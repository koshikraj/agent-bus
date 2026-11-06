import type { ReactNode, ButtonHTMLAttributes, InputHTMLAttributes } from "react";

export interface LogoProps {
  /** "lockup" is the mark, wordmark and signal. "icon" adapts its detail to the size. */
  variant?: "lockup" | "mark" | "icon";
  /** The height of the mark, or the width of the icon, in pixels. */
  size?: number;
  /** Draws the mark in one ink. Lockup and mark only. */
  mono?: boolean;
  /** Set false to drop the small dot and dash after the wordmark. */
  signal?: boolean;
  className?: string;
}

export interface BirdsProps {
  /** "loop" is the animated exchange. The others are stills. */
  pose?: "ask" | "idle" | "reply" | "loop";
  /** Set false to draw the scene without its `stage` panel. */
  stage?: boolean;
  className?: string;
}

export interface LetterProps {
  /** "request" has a tomato fold, "reply" a teal fold, and "solid" matches the logo. */
  tone?: "request" | "reply" | "solid";
  /** Width in pixels. */
  size?: number;
  /** Accessible name. Leave out when the letter is decorative. */
  label?: string;
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  kind?: "primary" | "secondary";
  /** Renders a link instead of a button. */
  href?: string;
  children: ReactNode;
}

export interface InstallCommandProps {
  /** Defaults to the birdie install script. */
  command?: string;
  className?: string;
}

export interface TextFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  hint?: string;
  /** Shown as "Error: ..." in place of the hint. */
  error?: string;
  /** Sets the value in DM Mono, for commands, URLs and tokens. */
  mono?: boolean;
}

export interface AgentAvatarProps {
  shape?: "round" | "pill" | "arch" | "square";
  /** Width in pixels. */
  size?: number;
  /** The agent's name, read by screen readers. */
  name?: string;
  /** Closes the eye for an idle session. */
  asleep?: boolean;
}

export interface AgentRowProps {
  name: string;
  shape?: "round" | "pill" | "arch" | "square";
  selected?: boolean;
  /** Number of unread letters. */
  unread?: number;
  asleep?: boolean;
  onClick?: () => void;
  className?: string;
}

export interface MessageBubbleProps {
  /** The sender's agent name. */
  from: string;
  /** "in" is from another agent, "out" is from this one. */
  side?: "in" | "out";
  verified?: boolean;
  children: ReactNode;
  className?: string;
}

export interface StatusSignalProps {
  /** What's happening, such as "hermes is typing". */
  label: string;
  /** Set false to stop the pulse. */
  live?: boolean;
  className?: string;
}
