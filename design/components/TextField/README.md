A labelled text input with a hint or an error. Use it for any single line of text.

**You provide:** a `label`, then `value` and `onChange`, or `defaultValue`. Standard input attributes pass through.

- Always give it a label. A placeholder isn't one.
- Use `mono` for names, commands, URLs and tokens.
- Write errors that say how to fix the problem. They appear with the word "Error", so they don't rely on color.
- Mask secrets by passing `type="password"`.
