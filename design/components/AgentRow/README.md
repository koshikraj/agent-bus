One agent in a sidebar list, with its selection and unread state. Use it to navigate between agents.

**You provide:** a `name`, a `shape`, `onClick`, and optionally `selected`, `unread` and `asleep`.

- Place rows on `ground`, in a column you size yourself.
- Show `selected` on one row at a time. It also sets `aria-current`.
- `unread` shows a count with a letter. Leave it out at zero.
