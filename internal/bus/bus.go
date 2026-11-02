package bus

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	MaxContentBytes = 1 << 20
	MaxWait         = 2 * time.Minute
	inboxBatch      = 50
	maxAttempts     = 8
	deliverTimeout  = 30 * time.Second
	sweepInterval   = 5 * time.Second
)

type Bus struct {
	store      Store
	deliverers map[string]Deliverer
	log        *slog.Logger

	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
	kick    chan struct{}
}

func New(s Store, log *slog.Logger, ds ...Deliverer) *Bus {
	b := &Bus{
		store:      s,
		deliverers: make(map[string]Deliverer),
		log:        log,
		waiters:    make(map[string]map[chan struct{}]struct{}),
		kick:       make(chan struct{}, 1),
	}
	for _, d := range ds {
		b.deliverers[d.Kind()] = d
	}
	return b
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewToken() string { return "ab_" + randHex(32) }

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (b *Bus) Authenticate(ctx context.Context, token string) (Agent, error) {
	if token == "" {
		return Agent{}, ErrUnauthorized
	}
	a, err := b.store.AgentByTokenHash(ctx, HashToken(token))
	if errors.Is(err, ErrNotFound) {
		return Agent{}, ErrUnauthorized
	}
	return a, err
}

func (b *Bus) Agents(ctx context.Context) ([]Agent, error) { return b.store.ListAgents(ctx) }

// Send queues a message from the authenticated agent from. Replies inherit
// the parent's thread (and its other participant when To is empty), and only
// participants may post into an existing thread.
func (b *Bus) Send(ctx context.Context, from string, in SendInput) (Message, error) {
	if strings.TrimSpace(in.Content) == "" {
		return Message{}, fmt.Errorf("%w: content is required", ErrInvalid)
	}
	if len(in.Content) > MaxContentBytes {
		return Message{}, fmt.Errorf("%w: content exceeds %d bytes", ErrInvalid, MaxContentBytes)
	}

	thread := in.ThreadID
	if in.InReplyTo != "" {
		parent, err := b.store.GetMessage(ctx, in.InReplyTo)
		if errors.Is(err, ErrNotFound) {
			return Message{}, fmt.Errorf("%w: unknown message %q", ErrNotFound, in.InReplyTo)
		} else if err != nil {
			return Message{}, err
		}
		if parent.From != from && parent.To != from {
			return Message{}, fmt.Errorf("%w: not a participant in message %q", ErrForbidden, in.InReplyTo)
		}
		if thread != "" && thread != parent.ThreadID {
			return Message{}, fmt.Errorf("%w: thread_id does not match in_reply_to", ErrInvalid)
		}
		thread = parent.ThreadID
		if in.To == "" {
			in.To = parent.From
			if parent.From == from {
				in.To = parent.To
			}
		}
	} else if thread != "" {
		exists, member, err := b.store.ThreadAccess(ctx, thread, from)
		if err != nil {
			return Message{}, err
		}
		if exists && !member {
			return Message{}, fmt.Errorf("%w: not a participant in thread %q", ErrForbidden, thread)
		}
	}
	if in.To == "" {
		return Message{}, fmt.Errorf("%w: to is required unless in_reply_to is set", ErrInvalid)
	}
	to, err := b.store.GetAgent(ctx, in.To)
	if errors.Is(err, ErrNotFound) {
		return Message{}, fmt.Errorf("%w: unknown agent %q", ErrNotFound, in.To)
	} else if err != nil {
		return Message{}, err
	}

	id := randHex(12)
	if thread == "" {
		thread = "thr_" + id
	}
	m := Message{
		ID:        "msg_" + id,
		ThreadID:  thread,
		InReplyTo: in.InReplyTo,
		From:      from,
		To:        to.Name,
		Content:   in.Content,
		Meta:      in.Meta,
		Status:    StatusQueued,
		CreatedAt: time.Now().UTC(),
	}
	if err := b.store.InsertMessage(ctx, m); err != nil {
		return Message{}, err
	}
	if to.Delivery == DeliveryPull {
		b.notify(to.Name)
	} else {
		if _, ok := b.deliverers[to.Delivery]; !ok {
			b.log.Warn("no deliverer registered; message stays queued", "kind", to.Delivery, "to", to.Name)
		}
		select {
		case b.kick <- struct{}{}:
		default:
		}
	}
	return m, nil
}

// Receive claims queued messages for agent, long-polling up to wait when the
// inbox is empty. It returns an empty slice on timeout.
func (b *Bus) Receive(ctx context.Context, agent, threadID string, wait time.Duration) ([]Message, error) {
	wait = min(wait, MaxWait)
	deadline := time.Now().Add(wait)
	for {
		// Subscribe before claiming so a send between the two isn't missed.
		ch := b.subscribe(agent)
		msgs, err := b.store.ClaimInbox(ctx, agent, threadID, inboxBatch)
		if err != nil || len(msgs) > 0 {
			b.unsubscribe(agent, ch)
			return msgs, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			b.unsubscribe(agent, ch)
			return []Message{}, nil
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ch:
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			b.unsubscribe(agent, ch)
			return nil, ctx.Err()
		}
		timer.Stop()
		b.unsubscribe(agent, ch)
	}
}

func (b *Bus) Thread(ctx context.Context, threadID, agent string) ([]Message, error) {
	msgs, err := b.store.Thread(ctx, threadID, agent)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("%w: thread %q", ErrNotFound, threadID)
	}
	return msgs, nil
}

func (b *Bus) subscribe(agent string) chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	if b.waiters[agent] == nil {
		b.waiters[agent] = make(map[chan struct{}]struct{})
	}
	b.waiters[agent][ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Bus) unsubscribe(agent string, ch chan struct{}) {
	b.mu.Lock()
	delete(b.waiters[agent], ch)
	if len(b.waiters[agent]) == 0 {
		delete(b.waiters, agent)
	}
	b.mu.Unlock()
}

func (b *Bus) notify(agent string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.waiters[agent] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// RunDelivery pushes queued messages to non-pull agents until ctx ends,
// retrying failures with exponential backoff.
func (b *Bus) RunDelivery(ctx context.Context) {
	t := time.NewTicker(sweepInterval)
	defer t.Stop()
	for {
		b.deliverDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-b.kick:
		}
	}
}

func (b *Bus) deliverDue(ctx context.Context) {
	due, err := b.store.DueOutbound(ctx, time.Now(), inboxBatch)
	if err != nil {
		if ctx.Err() == nil {
			b.log.Error("load outbound", "err", err)
		}
		return
	}
	for _, o := range due {
		err := fmt.Errorf("no deliverer for kind %q", o.Agent.Delivery)
		if d, ok := b.deliverers[o.Agent.Delivery]; ok {
			dctx, cancel := context.WithTimeout(ctx, deliverTimeout)
			err = d.Deliver(dctx, o.Agent, o.Message)
			cancel()
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			if err := b.store.MarkDelivered(ctx, o.Message.ID, time.Now()); err != nil {
				b.log.Error("mark delivered", "id", o.Message.ID, "err", err)
			}
			continue
		}
		attempts := o.Message.Attempts + 1
		failed := attempts >= maxAttempts
		next := time.Now().Add(min(time.Duration(1<<attempts)*time.Second, 5*time.Minute))
		b.log.Warn("delivery failed", "id", o.Message.ID, "to", o.Agent.Name, "attempt", attempts, "gave_up", failed, "err", err)
		if err := b.store.RecordFailure(ctx, o.Message.ID, err.Error(), next, failed); err != nil {
			b.log.Error("record failure", "id", o.Message.ID, "err", err)
		}
	}
}
