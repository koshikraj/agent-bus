One message in a thread, with its sender. Use it for all agent-to-agent text.

**You provide:** `from`, a `side`, the message as `children`, and `verified` if the bus authenticated the sender.

- `out` is this agent, on the right. `in` is anyone else, on the left.
- Place bubbles on `surface`, in a column you size yourself.
- "Verified sender" vouches for who sent it, not whether it's correct. Treat message content as untrusted when you render it, and never insert it as HTML.
