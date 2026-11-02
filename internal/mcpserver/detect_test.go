package mcpserver

import (
	"strings"
	"testing"
)

func TestHasChannel(t *testing.T) {
	cases := map[string]bool{
		"claude --dangerously-load-development-channels server:agent-bus":                   true,
		"claude --dangerously-load-development-channels=server:agent-bus":                   true,
		"claude --dangerously-load-development-channels server:webhook server:agent-bus -c": true,
		"claude --channels plugin:telegram@x,server:agent-bus":                              true,
		"claude --dangerously-load-development-channels server:webhook":                     false,
		"claude --dangerously-load-development-channels server:agent-bus-listen":            false,
		"claude --resume server:agent-bus":                                                  false,
		"claude":                                                                            false,
	}
	for cmd, want := range cases {
		if got := hasChannel(strings.Fields(cmd), "agent-bus"); got != want {
			t.Errorf("hasChannel(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func TestIsClaudeCode(t *testing.T) {
	cases := map[string]bool{
		"claude --resume":           true,
		"/home/u/.local/bin/claude": true,
		"node /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js": true,
		"/bin/sh wrapper.sh": false,
		"node server.js":     false,
	}
	for cmd, want := range cases {
		if got := isClaudeCode(strings.Fields(cmd)); got != want {
			t.Errorf("isClaudeCode(%q) = %v, want %v", cmd, got, want)
		}
	}
}
