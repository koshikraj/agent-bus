// Package bus is the protocol-neutral core of agent-bus: agents, messages,
// threads, inboxes and outbound delivery. Every transport (HTTP, MCP, A2A,
// ACP, webhooks) is a thin adapter over this package.
package bus

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalid      = errors.New("invalid request")
)

const (
	StatusQueued    = "queued"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"

	// DeliveryPull agents fetch their own inbox (MCP, HTTP long-poll).
	// Any other delivery kind is pushed by the Deliverer registered for it.
	DeliveryPull = "pull"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// ValidName reports whether name is usable as an agent name. Names double as
// token file names, so the pattern is deliberately strict.
func ValidName(name string) bool { return namePattern.MatchString(name) }

type Agent struct {
	Name string `json:"name"`
	// Description tells other agents what this agent is for. It is kept in
	// Config["description"].
	Description string            `json:"description,omitempty"`
	TokenHash   string            `json:"-"`
	Delivery    string            `json:"delivery"`
	Config      map[string]string `json:"-"`
	CreatedAt   time.Time         `json:"created_at"`
}

type Message struct {
	ID        string            `json:"id"`
	ThreadID  string            `json:"thread_id"`
	InReplyTo string            `json:"in_reply_to,omitempty"`
	From      string            `json:"from"`
	To        string            `json:"to"`
	Content   string            `json:"content"`
	Meta      map[string]string `json:"meta,omitempty"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`

	Attempts int `json:"-"`
}

type SendInput struct {
	To        string            `json:"to"`
	Content   string            `json:"content"`
	ThreadID  string            `json:"thread_id,omitempty"`
	InReplyTo string            `json:"in_reply_to,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// Outbound is a queued message for a push-delivered agent.
type Outbound struct {
	Message Message
	Agent   Agent
}

type Store interface {
	CreateAgent(ctx context.Context, a Agent) error
	DeleteAgent(ctx context.Context, name string) error
	GetAgent(ctx context.Context, name string) (Agent, error)
	AgentByTokenHash(ctx context.Context, hash string) (Agent, error)
	ListAgents(ctx context.Context) ([]Agent, error)

	InsertMessage(ctx context.Context, m Message) error
	GetMessage(ctx context.Context, id string) (Message, error)
	// ThreadAccess reports whether the thread has any messages and whether
	// agent sent or received one of them.
	ThreadAccess(ctx context.Context, threadID, agent string) (exists, member bool, err error)
	// Thread returns the thread's messages that agent sent or received.
	Thread(ctx context.Context, threadID, agent string) ([]Message, error)
	// ClaimInbox marks up to limit queued messages for agent as delivered and
	// returns them, oldest first. threadID filters when non-empty.
	ClaimInbox(ctx context.Context, agent, threadID string, limit int) ([]Message, error)

	DueOutbound(ctx context.Context, now time.Time, limit int) ([]Outbound, error)
	MarkDelivered(ctx context.Context, id string, at time.Time) error
	RecordFailure(ctx context.Context, id, errMsg string, next time.Time, failed bool) error

	Close() error
}

// Deliverer pushes messages to agents whose Delivery equals Kind(). This is
// the outbound glue point for webhooks, A2A clients, ACP clients and so on.
type Deliverer interface {
	Kind() string
	Deliver(ctx context.Context, to Agent, m Message) error
}
