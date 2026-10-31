// Package client is a Go client for the agent-bus HTTP API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agentbus/internal/bus"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{}}
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("agent-bus: %s (HTTP %d)", e.Message, e.Status) }

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) {
			return fmt.Errorf("agent-bus daemon not reachable at %s (is `agent-bus serve` running?): %w", c.BaseURL, err)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return &APIError{Status: resp.StatusCode, Message: e.Error}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) Me(ctx context.Context) (bus.Agent, error) {
	var a bus.Agent
	return a, c.do(ctx, http.MethodGet, "/v1/me", nil, &a)
}

func (c *Client) Agents(ctx context.Context) ([]bus.Agent, error) {
	var out struct{ Agents []bus.Agent }
	return out.Agents, c.do(ctx, http.MethodGet, "/v1/agents", nil, &out)
}

func (c *Client) Send(ctx context.Context, in bus.SendInput) (bus.Message, error) {
	var m bus.Message
	return m, c.do(ctx, http.MethodPost, "/v1/messages", in, &m)
}

// Inbox claims queued messages, long-polling up to wait (the server caps it).
func (c *Client) Inbox(ctx context.Context, threadID string, wait time.Duration) ([]bus.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, wait+30*time.Second)
	defer cancel()
	q := url.Values{"wait": {strconv.FormatFloat(wait.Seconds(), 'f', -1, 64)}}
	if threadID != "" {
		q.Set("thread_id", threadID)
	}
	var out struct{ Messages []bus.Message }
	return out.Messages, c.do(ctx, http.MethodGet, "/v1/inbox?"+q.Encode(), nil, &out)
}

func (c *Client) Thread(ctx context.Context, threadID string) ([]bus.Message, error) {
	var out struct{ Messages []bus.Message }
	return out.Messages, c.do(ctx, http.MethodGet, "/v1/threads/"+url.PathEscape(threadID), nil, &out)
}
