# agent-bus

Lets the AI agents on your machine talk to each other, in both directions.
Ask Claude Code to "check with Grok Bot", have Grok Bot ask Claude to review a
PR, or let Grok answer questions headlessly, all through one small local
service.

```
  Claude Code ──MCP──┐                         ┌──webhook──► Grok Bot
  Codex / Grok ──MCP─┤                         │
                     ▼                         │
              agent-bus serve  (127.0.0.1:7777, SQLite)
                     ▲                         │
  Grok Bot ───HTTP───┘                         └──ACP──────► grok (headless)
```

Every message has a verified sender, a thread, and an optional "reply to",
so questions and answers stay paired.

## 1. Install

You need [Go](https://go.dev/dl/) 1.27 or newer.

```sh
git clone https://github.com/koshikraj/agent-bus && cd agent-bus
make install            # puts agent-bus in ~/.local/bin (make sure that's on your PATH)
```

## 2. Run the service

```sh
agent-bus serve         # listens on 127.0.0.1:7777; keep it running
```

To start it automatically on Linux (systemd):

```sh
cp deploy/agent-bus.service ~/.config/systemd/user/
systemctl --user daemon-reload && systemctl --user enable --now agent-bus
```

Restart the service after upgrading the binary.

## 3. Set up Claude Code

```sh
# Register Claude on the bus (its token is saved in ~/.config/agent-bus/tokens/claude)
agent-bus agent add claude --description "Claude Code on this machine: code, repos, files"

# Give every Claude session the tools to message other agents
claude mcp add -s user agent-bus -- agent-bus mcp --as claude --channel

# Optional: a skill that tells Claude (and the Grok CLI, which reads the same folder) when to use them
mkdir -p ~/.claude/skills && cp -r skills/talk-to-agents ~/.claude/skills/
```

Now any Claude session can do things like *"ask Grok Bot if I have meetings
today"*; you don't have to mention agent-bus.

**Letting other agents wake Claude up.** So that Grok Bot (or anyone) can
start a conversation with Claude, start one *listening* session:

```sh
claude --dangerously-load-development-channels server:agent-bus
```

Pick "I am using this for local development" when asked. Messages to `claude`
now appear in that session on their own, and Claude answers them. Other
Claude sessions leave incoming messages alone: the `--channel` bridge only
takes messages in a session started with that flag. Run one listening session
at a time; with several, each message goes to one of them.

If you registered the server under a different name, pass it to the bridge
too: `agent-bus mcp --as claude --channel --server-name NAME`.

## 4. Set up Grok Bot

```sh
agent-bus agent add grok-bot \
  --description "Grok Bot: personal assistant (email, calendar, web); answers in its own time" \
  --webhook https://api2.cursor.sh/automations/webhook/<your-webhook-id> \
  --auth-file ~/.config/grok-bot/sender-key
```

- `--webhook` is the Grok Bot routine's webhook URL; `--auth-file` is a file
  holding its secret key. Messages to `grok-bot` are POSTed there (retried if
  it's down).
- In the Grok Bot routine, add two secrets: `AGENT_BUS_URL` =
  `http://127.0.0.1:7777`, and `AGENT_BUS_TOKEN` = the contents of
  `~/.config/agent-bus/tokens/grok-bot`.
- Paste the routine text from [docs/grok-bot-routine.md](docs/grok-bot-routine.md).
  It covers starting conversations, answering messages, and not replying to
  answers (so bots don't loop).

## 5. Try it

In any Claude session:

> Ask Grok Bot what the weather is in Bangalore.

From Grok Bot's chat, with the listening Claude session running:

> Ask Claude how many open PRs koshikraj/logr has.

Or from a terminal:

```sh
agent-bus agent add me
agent-bus send --as me --to claude "What's 12 × 12?"
agent-bus inbox --as me --wait 60s
```

## Other agents

**Grok CLI, answering headlessly (ACP).** No window needs to be open: for each
message, agent-bus runs Grok, keeps one Grok session per thread, and posts its
answer back.

```sh
agent-bus agent add grok --acp "grok agent stdio" --cwd ~/grok-work \
  --description "Grok CLI: coding questions and quick research"
```

| Flag | Default | Meaning |
|---|---|---|
| `--cwd` | home folder | Folder Grok works in; `grok -r <session>` there opens a conversation |
| `--permission` | `reject` | Tool calls that need approval: `reject` or `allow` |
| `--timeout` | `20m` | Longest one answer may take |
| `--max-replies` | `10` | Answers per thread before it stops (stops bots looping); `0` = no limit |

Any agent that speaks the [Agent Client Protocol](https://agentclientprotocol.com)
works the same way, e.g. Codex via `codex-acp` once installed.

**Grok CLI or Codex, interactively (MCP).** Give an open session the same
tools Claude has, under its own name:

```sh
agent-bus agent add grok-tui
grok mcp add agent-bus agent-bus -- mcp --as grok-tui

agent-bus agent add codex
# ~/.codex/config.toml
#   [mcp_servers.agent-bus]
#   command = "agent-bus"
#   args = ["mcp", "--as", "codex"]
cp -r skills/talk-to-agents ~/.codex/skills/
```

These sessions can send and wait for answers, but can't be woken by an
incoming message the way the listening Claude session can.

## Commands

```
agent-bus serve [--addr 127.0.0.1:7777]
agent-bus agent add NAME [--description TEXT] [--webhook URL --auth-file PATH | --acp "COMMAND" ...]
agent-bus agent describe NAME TEXT
agent-bus agent list | rm NAME
agent-bus send  --as NAME --to AGENT MESSAGE
agent-bus reply --as NAME MSG_ID MESSAGE
agent-bus inbox --as NAME [--wait 30s]
agent-bus mcp   --as NAME [--channel [--server-name agent-bus]]
```

MCP tools: `send_message`, `wait_for_reply`, `check_inbox`, `get_thread`,
`list_agents`.

HTTP API (Bearer token): `POST /v1/messages`, `GET /v1/inbox?wait=30s`,
`GET /v1/threads/{id}`, `GET /v1/agents`, `GET /v1/me`.

## Troubleshooting

| Problem | Check |
|---|---|
| `connection refused` | Is `agent-bus serve` running? |
| Claude never wakes up | The listening session was started with `--dangerously-load-development-channels server:agent-bus`, and the server name matches your `claude mcp add` name |
| Grok Bot gets nothing | `agent-bus agent list` shows `grok-bot` as `webhook`; the service log shows delivery errors |
| Headless Grok doesn't answer | The service log shows `acp: turn failed`; the sender also gets a reply explaining why |

## Security

- Listens on 127.0.0.1 only. Each agent has its own token; only a hash is stored.
- The sender of every message is set by the bus from the token, so it can't be faked.
- Only the agents in a thread can read it or post to it.
- Messages from other agents are treated as requests, not as your
  instructions: agents are told not to share secrets or personal information,
  or do anything destructive, without checking with you.

## How it fits together

```
cmd/agent-bus       CLI
internal/bus        messages, threads, inboxes, delivery loop (no transport code)
internal/store      SQLite storage
internal/httpapi    HTTP API
internal/mcpserver  MCP server (+ Claude Code channel push)
internal/adapter    outbound delivery: webhook, ACP (A2A next)
skills/             talk-to-agents skill for Claude, Grok and Codex
docs/               Grok Bot routine
```

New protocols plug in as adapters: outbound ones implement `bus.Deliverer`
(`Kind()` + `Deliver()`) and are picked by an agent's delivery kind; inbound
ones call `Bus.Send` / `Bus.Receive`, as the HTTP and MCP servers do.
