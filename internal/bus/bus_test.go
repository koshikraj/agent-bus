package bus_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"agentbus/internal/adapter"
	"agentbus/internal/bus"
	"agentbus/internal/store"
)

func newBus(t *testing.T, ds ...bus.Deliverer) (*bus.Bus, *store.SQLite) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "bus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return bus.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), ds...), st
}

func addAgent(t *testing.T, st *store.SQLite, name string, cfg map[string]string) {
	t.Helper()
	a := bus.Agent{Name: name, TokenHash: bus.HashToken(name + "-token"), Delivery: bus.DeliveryPull, CreatedAt: time.Now()}
	if cfg != nil {
		a.Delivery, a.Config = "webhook", cfg
	}
	if err := st.CreateAgent(context.Background(), a); err != nil {
		t.Fatal(err)
	}
}

func TestRequestReplyThread(t *testing.T) {
	ctx := context.Background()
	b, st := newBus(t)
	addAgent(t, st, "alice", nil)
	addAgent(t, st, "bob", nil)

	req, err := b.Send(ctx, "alice", bus.SendInput{To: "bob", Content: "ping", Meta: map[string]string{"wanted": "pong"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Receive(ctx, "bob", "", 0)
	if err != nil || len(got) != 1 || got[0].Content != "ping" || got[0].Meta["wanted"] != "pong" {
		t.Fatalf("bob inbox = %+v, %v", got, err)
	}
	if again, _ := b.Receive(ctx, "bob", "", 0); len(again) != 0 {
		t.Fatalf("message delivered twice: %+v", again)
	}

	reply, err := b.Send(ctx, "bob", bus.SendInput{To: "alice", Content: "pong", InReplyTo: req.ID})
	if err != nil {
		t.Fatal(err)
	}
	if reply.ThreadID != req.ThreadID {
		t.Fatalf("reply thread %q, want %q", reply.ThreadID, req.ThreadID)
	}
	got, err = b.Receive(ctx, "alice", req.ThreadID, 0)
	if err != nil || len(got) != 1 || got[0].InReplyTo != req.ID {
		t.Fatalf("alice inbox = %+v, %v", got, err)
	}
	thread, err := b.Thread(ctx, req.ThreadID, "alice")
	if err != nil || len(thread) != 2 {
		t.Fatalf("thread = %+v, %v", thread, err)
	}
}

func TestLongPollWakesOnSend(t *testing.T) {
	ctx := context.Background()
	b, st := newBus(t)
	addAgent(t, st, "alice", nil)
	addAgent(t, st, "bob", nil)

	go func() {
		time.Sleep(100 * time.Millisecond)
		b.Send(ctx, "alice", bus.SendInput{To: "bob", Content: "hello"})
	}()
	start := time.Now()
	got, err := b.Receive(ctx, "bob", "", 10*time.Second)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("long poll took %v; should wake on send", elapsed)
	}
}

func TestOutsidersCannotJoinThread(t *testing.T) {
	ctx := context.Background()
	b, st := newBus(t)
	for _, n := range []string{"alice", "bob", "mallory"} {
		addAgent(t, st, n, nil)
	}
	m, err := b.Send(ctx, "alice", bus.SendInput{To: "bob", Content: "secret plan"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Send(ctx, "mallory", bus.SendInput{To: "alice", Content: "x", InReplyTo: m.ID}); !errors.Is(err, bus.ErrForbidden) {
		t.Fatalf("reply by outsider: err = %v, want ErrForbidden", err)
	}
	if _, err := b.Send(ctx, "mallory", bus.SendInput{To: "alice", Content: "x", ThreadID: m.ThreadID}); !errors.Is(err, bus.ErrForbidden) {
		t.Fatalf("thread post by outsider: err = %v, want ErrForbidden", err)
	}
	if _, err := b.Thread(ctx, m.ThreadID, "mallory"); !errors.Is(err, bus.ErrNotFound) {
		t.Fatalf("thread read by outsider: err = %v, want ErrNotFound", err)
	}
	if _, err := b.Send(ctx, "alice", bus.SendInput{To: "nobody", Content: "x"}); !errors.Is(err, bus.ErrNotFound) {
		t.Fatalf("unknown recipient: err = %v", err)
	}
}

func TestWebhookDelivery(t *testing.T) {
	type hit struct {
		auth string
		body map[string]any
	}
	hits := make(chan hit, 4)
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail { // first attempt fails to exercise retry
			fail = false
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		hits <- hit{r.Header.Get("Authorization"), body}
	}))
	defer srv.Close()

	keyFile := filepath.Join(t.TempDir(), "key")
	if err := writeFile(keyFile, "s3cret\n"); err != nil {
		t.Fatal(err)
	}
	b, st := newBus(t, &adapter.Webhook{PublicURL: "https://bus.example"})
	addAgent(t, st, "claude", nil)
	addAgent(t, st, "grok", map[string]string{"url": srv.URL, "auth_file": keyFile})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.RunDelivery(ctx)

	m, err := b.Send(ctx, "claude", bus.SendInput{To: "grok", Content: "summarize repo", Meta: map[string]string{"context": "README.md", "wanted": "3 bullets"}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case h := <-hits:
		if h.auth != "Bearer s3cret" {
			t.Errorf("auth header = %q", h.auth)
		}
		if h.body["task"] != "summarize repo" || h.body["context"] != "README.md" || h.body["wanted"] != "3 bullets" || h.body["source"] != "claude" {
			t.Errorf("body = %v", h.body)
		}
		env := h.body["agent_bus"].(map[string]any)
		if env["message_id"] != m.ID || env["reply_to"] == nil {
			t.Errorf("envelope = %v", env)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("webhook never delivered after retry")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := st.GetMessage(ctx, m.ID)
		if got.Status == bus.StatusDelivered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status = %q, want delivered", got.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
