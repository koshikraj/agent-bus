package mcpserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentbus/internal/bus"
	"agentbus/internal/client"
	"agentbus/internal/httpapi"
	"agentbus/internal/mcpserver"
	"agentbus/internal/store"
)

// session drives a Server over pipes the way an MCP client would.
type session struct {
	t   *testing.T
	in  *io.PipeWriter
	out *bufio.Scanner
	id  int
}

func (s *session) send(v map[string]any) {
	b, _ := json.Marshal(v)
	s.in.Write(append(b, '\n'))
}

// next returns the next line written by the server.
func (s *session) next() map[string]any {
	s.t.Helper()
	line := make(chan string, 1)
	go func() {
		if s.out.Scan() {
			line <- s.out.Text()
		}
		close(line)
	}()
	select {
	case l, ok := <-line:
		if !ok {
			s.t.Fatal("server closed output")
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			s.t.Fatalf("bad JSON from server: %q", l)
		}
		return m
	case <-time.After(10 * time.Second):
		s.t.Fatal("timed out waiting for server output")
	}
	return nil
}

func (s *session) call(method string, params any) map[string]any {
	s.t.Helper()
	s.id++
	s.send(map[string]any{"jsonrpc": "2.0", "id": s.id, "method": method, "params": params})
	resp := s.next()
	if e, ok := resp["error"]; ok {
		s.t.Fatalf("%s: error %v", method, e)
	}
	return resp["result"].(map[string]any)
}

func toolJSON(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	if result["isError"] == true {
		t.Fatalf("tool error: %v", result["content"])
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("tool text not JSON: %q", text)
	}
	return out
}

func setup(t *testing.T, channel bool) (*session, *client.Client) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "bus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, n := range []string{"claude", "cursor"} {
		st.CreateAgent(context.Background(), bus.Agent{Name: n, TokenHash: bus.HashToken(n + "-tok"), Delivery: bus.DeliveryPull, CreatedAt: time.Now()})
	}
	b := bus.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	api := httptest.NewServer(httpapi.New(b, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(api.Close)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := mcpserver.New(client.New(api.URL, "claude-tok"), "claude", "test", channel, log.New(io.Discard, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Run(ctx, inR, outW)
	t.Cleanup(func() { inW.Close(); cancel() })

	s := &session{t: t, in: inW, out: bufio.NewScanner(outR)}
	init := s.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}})
	if init["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize = %v", init)
	}
	_, hasChannel := init["capabilities"].(map[string]any)["experimental"]
	if hasChannel != channel {
		t.Fatalf("channel capability = %v, want %v", hasChannel, channel)
	}
	s.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return s, client.New(api.URL, "cursor-tok")
}

func TestToolsRoundTrip(t *testing.T) {
	s, cursor := setup(t, false)

	tools := s.call("tools/list", map[string]any{})["tools"].([]any)
	if len(tools) != 5 {
		t.Fatalf("got %d tools", len(tools))
	}

	sent := toolJSON(t, s.call("tools/call", map[string]any{"name": "send_message", "arguments": map[string]any{"to": "cursor", "content": "run the tests", "wanted": "pass/fail"}}))
	thread := sent["thread_id"].(string)

	// The other agent answers over plain HTTP while Claude waits.
	go func() {
		msgs, err := cursor.Inbox(context.Background(), "", 5*time.Second)
		if err != nil || len(msgs) != 1 {
			return
		}
		cursor.Send(context.Background(), bus.SendInput{To: "claude", Content: "all green", InReplyTo: msgs[0].ID})
	}()
	got := toolJSON(t, s.call("tools/call", map[string]any{"name": "wait_for_reply", "arguments": map[string]any{"thread_id": thread, "timeout_seconds": 10}}))
	msgs := got["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["content"] != "all green" {
		t.Fatalf("wait_for_reply = %v", got)
	}
}

func TestChannelPush(t *testing.T) {
	s, cursor := setup(t, true)

	m, err := cursor.Send(context.Background(), bus.SendInput{To: "claude", Content: "build broke on main", Meta: map[string]string{"from": "forged", "branch": "main"}})
	if err != nil {
		t.Fatal(err)
	}
	n := s.next()
	if n["method"] != "notifications/claude/channel" {
		t.Fatalf("got %v", n)
	}
	p := n["params"].(map[string]any)
	meta := p["meta"].(map[string]any)
	content := p["content"].(string)
	if !strings.HasPrefix(content, "build broke on main\n\n(Message from agent cursor") || !strings.Contains(content, m.ID) || meta["message_id"] != m.ID || meta["branch"] != "main" {
		t.Fatalf("notification = %v", p)
	}
	if meta["from"] != "cursor" {
		t.Fatalf("sender-supplied meta overrode authenticated from: %v", meta["from"])
	}
	if !strings.HasPrefix(meta["thread_id"].(string), "thr_") {
		t.Fatalf("thread_id = %v", meta["thread_id"])
	}
}
