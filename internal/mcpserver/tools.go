package mcpserver

import (
	"context"
	"encoding/json"
	"time"

	"agentbus/internal/bus"
)

type schema = map[string]any

func obj(required []string, props schema) schema {
	s := schema{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) schema { return schema{"type": "string", "description": desc} }

var tools = []schema{
	{
		"name":        "send_message",
		"description": "Message another AI agent or bot (for example Grok Bot, Claude, Codex or Grok): ask it something, hand it a task, or answer it. Returns message_id and thread_id. To answer a message, set in_reply_to to its message_id; to can then be left out.",
		"inputSchema": obj([]string{"content"}, schema{
			"to":          str("Recipient agent name from list_agents (e.g. grok-bot, claude). Optional when in_reply_to is set."),
			"content":     str("The message. For requests, make it self-contained: the recipient has none of your context."),
			"in_reply_to": str("message_id being answered; the reply joins that thread."),
			"thread_id":   str("Continue an existing thread without replying to a specific message."),
			"context":     str("Optional supporting files, links or notes."),
			"wanted":      str("Optional description of the result you want back."),
		}),
	},
	{
		"name":        "check_inbox",
		"description": "Fetch new messages other agents sent you. Each message is returned once.",
		"inputSchema": obj(nil, schema{
			"wait_seconds": schema{"type": "number", "description": "Wait up to this long (max 120) if the inbox is empty. Default 0."},
		}),
	},
	{
		"name":        "wait_for_reply",
		"description": "Wait for the answer to a message you sent: blocks until a message arrives on the thread. Returns the new messages, or nothing on timeout.",
		"inputSchema": obj([]string{"thread_id"}, schema{
			"thread_id":       str("thread_id returned by send_message."),
			"timeout_seconds": schema{"type": "number", "description": "Default 120, max 600."},
		}),
	},
	{
		"name":        "get_thread",
		"description": "Read the full history of a thread you are part of.",
		"inputSchema": obj([]string{"thread_id"}, schema{"thread_id": str("Thread to read.")}),
	},
	{
		"name":        "list_agents",
		"description": "List the other AI agents and bots you can message, with what each one is for.",
		"inputSchema": obj(nil, schema{}),
	},
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolArgs struct {
	To             string  `json:"to"`
	Content        string  `json:"content"`
	InReplyTo      string  `json:"in_reply_to"`
	ThreadID       string  `json:"thread_id"`
	Context        string  `json:"context"`
	Wanted         string  `json:"wanted"`
	WaitSeconds    float64 `json:"wait_seconds"`
	TimeoutSeconds float64 `json:"timeout_seconds"`
}

func (s *Server) callTool(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var p callParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &rpcError{-32602, "invalid params"}
	}
	var a toolArgs
	if len(p.Arguments) > 0 {
		if err := json.Unmarshal(p.Arguments, &a); err != nil {
			return toolText("invalid arguments: "+err.Error(), true), nil
		}
	}

	var out any
	var err error
	switch p.Name {
	case "send_message":
		meta := map[string]string{}
		if a.Context != "" {
			meta["context"] = a.Context
		}
		if a.Wanted != "" {
			meta["wanted"] = a.Wanted
		}
		var m bus.Message
		m, err = s.c.Send(ctx, bus.SendInput{To: a.To, Content: a.Content, InReplyTo: a.InReplyTo, ThreadID: a.ThreadID, Meta: meta})
		out = map[string]string{"message_id": m.ID, "thread_id": m.ThreadID, "status": m.Status}
	case "check_inbox":
		if s.channel {
			return toolText("Channel mode is on: new messages are pushed to you automatically as <channel> events.", false), nil
		}
		var msgs []bus.Message
		msgs, err = s.c.Inbox(ctx, "", seconds(a.WaitSeconds, 0, bus.MaxWait))
		out = map[string]any{"messages": msgs}
	case "wait_for_reply":
		if a.ThreadID == "" {
			return toolText("thread_id is required", true), nil
		}
		var msgs []bus.Message
		msgs, err = s.waitForReply(ctx, a.ThreadID, seconds(a.TimeoutSeconds, 120*time.Second, maxReplyWait))
		if err == nil && len(msgs) == 0 {
			note := "No reply yet. Check again later with wait_for_reply or check_inbox."
			if s.channel {
				note = "No reply yet. It will arrive as a <channel> event when it comes."
			}
			return toolText(note, false), nil
		}
		out = map[string]any{"messages": msgs}
	case "get_thread":
		var msgs []bus.Message
		msgs, err = s.c.Thread(ctx, a.ThreadID)
		out = map[string]any{"messages": msgs}
	case "list_agents":
		var agents []bus.Agent
		agents, err = s.c.Agents(ctx)
		out = map[string]any{"you": s.name, "agents": agents}
	default:
		return nil, &rpcError{-32602, "unknown tool: " + p.Name}
	}
	if err != nil {
		return toolText(err.Error(), true), nil
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return toolText(string(b), false), nil
}

func seconds(v float64, def, max time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return min(time.Duration(v*float64(time.Second)), max)
}

func toolText(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": isErr,
	}
}
