// Package adapter holds the protocol glue between the bus and the outside
// world. Outbound adapters implement bus.Deliverer; inbound adapters (A2A
// server, ACP agent, ...) call bus.Send/Receive the way httpapi does.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"birdie/internal/bus"
)

// Webhook delivers messages by POSTing JSON to the agent's configured URL.
// The body keeps Grok Bot's source/task/context/wanted shape, plus an
// birdie block carrying the IDs a remote agent needs to reply.
//
// Agent config keys: url (required), auth_file, auth_header (default
// Authorization, sent as "Bearer <token>").
type Webhook struct {
	HTTP *http.Client
	// PublicURL, when set, is advertised as the address to reply to.
	PublicURL string
}

func (w *Webhook) Kind() string { return "webhook" }

func (w *Webhook) Deliver(ctx context.Context, to bus.Agent, m bus.Message) error {
	target := to.Config["url"]
	if target == "" {
		return errors.New("webhook agent has no url configured")
	}
	envelope := map[string]any{
		"message_id": m.ID,
		"thread_id":  m.ThreadID,
		"from":       m.From,
		"to":         m.To,
	}
	if m.InReplyTo != "" {
		envelope["in_reply_to"] = m.InReplyTo
	}
	if w.PublicURL != "" {
		envelope["reply_to"] = map[string]string{
			"url":  strings.TrimRight(w.PublicURL, "/") + "/v1/messages",
			"body": fmt.Sprintf(`{"to": %q, "in_reply_to": %q, "content": "<your reply>"}`, m.From, m.ID),
			"auth": "Authorization: Bearer <your birdie token>",
		}
	}
	body, err := json.Marshal(map[string]any{
		"source":    m.From,
		"task":      m.Content,
		"context":   m.Meta["context"],
		"wanted":    m.Meta["wanted"],
		"birdie":    envelope,
		"agent_bus": envelope, // routines pasted before the rename read this
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if f := to.Config["auth_file"]; f != "" {
		secret, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read auth_file: %w", err)
		}
		header, value := to.Config["auth_header"], strings.TrimSpace(string(secret))
		if header == "" || strings.EqualFold(header, "Authorization") {
			header, value = "Authorization", "Bearer "+value
		}
		req.Header.Set(header, value)
	}

	client := w.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
