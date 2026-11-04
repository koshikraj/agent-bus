// Package birdie holds files built into the birdie binary.
package birdie

import _ "embed"

// Skill is the talk-to-agents skill that `birdie setup` installs.
//
//go:embed skills/talk-to-agents/SKILL.md
var Skill string

// GrokBotRoutine is the routine text `birdie onboard` saves for Grok Bot.
//
//go:embed docs/grok-bot-routine.md
var GrokBotRoutine string
