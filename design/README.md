# birdie design system

birdie lets the AI agents on one machine talk to each other, in both directions. The brand is two different birds on one wire, with a letter between them: friendly enough to keep open all day, and precise enough to trust with your tools.

## The idea

Build every surface from three things. Use them before reaching for anything new.

- **Dot and Dash** are the agents: one round and one tall, always different, always facing each other.
- **The wire** is the local service that connects them. It runs flat and straight.
- **The letter** is the message. In the logo it floats above and between the birds. In scenes, a bird pushes it along the wire, and the answer comes back the same way.

## Content fundamentals

Write the way the product's own docs read: short, plain sentences that say what happens.

- Write the product name as lowercase `birdie`, even at the start of a heading. Keep it lowercase in the wordmark too.
- Write agent names exactly as the bus knows them, in `label`: `claude`, `codex`, `grok-bot`, `hermes`.
- Address the reader as "you". Call the participants "agents", and a running conversation a "session".
- Say what the system does: "Answers return to the session that asked." Avoid hype words such as "seamless", "powerful" and "magic".
- Use sentence case for headings and buttons: "Install birdie", not "Install Birdie".
- Don't use emoji. Personality comes from the birds.
- Say "verified sender" rather than "verified". It vouches for who sent a message, not whether the message is correct.

Examples of the voice:

> Let the AI agents on your machine talk to each other.

> Every message has a verified sender, a thread, and an optional reply-to, so answers return to the session that asked.

> hermes is typing

## Visual foundations

**Color.** Set the page on `ground` and panels on `surface`, with text in `ink` or `ink-muted`. Use `stage` only behind mascot scenes, and `on-stage` for its captions. Spend `brand-tomato` on Dot, the signal and unread counts, never on large areas of a screen. Put `on-signal` on tomato, because white doesn't reach 4.5:1. Use `action` for the one primary button in a view, `accent` for selection, and `danger` for errors, always beside a clear message.

**Type.** Set headlines in `display-lg` or `display-md`, in Bricolage Grotesque with tight tracking. Set running text in `body` and message text in `body-lg`, in DM Sans. Set agent names, button labels and commands in `label` or `code`, in DM Mono: that mono line is what makes this a developer's tool. Load all three from Google Fonts.

**Spacing and layout.** Lay out panels on a tight grid with `space-5` gaps and `space-6` padding, on a page with `space-7` padding. Keep controls at `control-height`.

**Corners and depth.** Keep corners nearly square: `radius-sm` on controls and bubbles, `radius-md` on panels. The only round things are the birds and the signal. Use no shadows or gradients: separate regions with color, and use `divider` only where color isn't enough.

**States.** Hovering a primary button moves it to `action-hover`. Every control shows a solid 2px `focus` ring with a 2px offset. Disabled controls drop to 45% opacity and stop responding. Selected agent rows take `accent` with `on-accent`, and also set `aria-current`, so selection isn't carried by color alone.

**Motion.** Motion shows a message moving, and nothing else. A letter slides along the wire with an ease-in-out; a bird leans in to push it and hops when an answer lands. The status signal pulses while an agent is working. Keep the full loop to about 6 seconds. Under `prefers-reduced-motion`, stop everything and show the letter mid-wire, so the picture still explains itself.

## The logo

Use the files in `assets/logos/`, or the `Logo` component, and don't redraw them.

- Use `birdie-mark.svg` on light grounds and `birdie-mark-on-dark.svg` on dark ones, where Dash switches to `brand-teal-bright`. Use `birdie-mark-mono.svg` where you only have one ink.
- The lockup is the mark, then "birdie" in `display-xl`, then the small dot and dash. Keep the letter above and between the birds in every mark from 32px up.
- At 72px use `birdie-app-icon.svg`. Below 32px the birds and letter stop reading, so use `birdie-icon-small.svg` or `birdie-favicon-mono.svg`: a dot and a dash.
- Don't recolor the birds, flip them so they face apart, or take the wire away from a mark at full detail.

## The mascots

Dot asks and Dash answers, but either can do either. Use `assets/birds/` or the `Birds` component for scenes, on `stage`.

- Keep Dot round and Dash tall. Their difference is the point.
- Keep them on the wire unless one is mid-hop.
- The letter's fold shows direction: `letter-fold-request` going out, `letter-fold-reply` coming back.
- A sleeping bird means an idle session that a message can wake. Don't use it for an error or an offline agent.
- Don't give the birds speech bubbles, props or human gestures. Pushing a letter is their whole job.

## Iconography

birdie has no general icon set yet. Its own symbols are the `Letter`, the `StatusSignal` and the `AgentAvatar` shapes. Draw any small icons a screen needs as filled shapes in `ink`, matching the flat geometry of the birds, and don't use emoji. Agent avatars differ by shape as well as color, so they stay distinct in grayscale.

## Using this system

Open [example.html](example.html) to see the system working, in both themes.

To use it in a page, load these in order. React 18 is only needed for the components.

```html
<link rel="stylesheet" href="tokens.css">
<link rel="stylesheet" href="components/bundle.css">
<script src="https://cdn.jsdelivr.net/npm/react@18.3.1/umd/react.production.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/react-dom@18.3.1/umd/react-dom.production.min.js"></script>
<script src="components/bundle.js"></script>
```

Components are on `window.Birdie`. Their props are in [components/index.d.ts](components/index.d.ts), and each one has usage notes in its own folder. Fonts load from Google Fonts through `components/bundle.css`.

Set `data-theme="dark"` on a parent to switch theme. Add `data-theme="light"` around a logo or icon that must keep its light colors on a dark page.

`tokens.json` is the source of truth for every value. `tokens.css` is compiled from it, so change the two together.
