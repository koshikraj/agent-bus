package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"birdie"
	"birdie/internal/bus"
)

// botPromptTemplate is what the user gives Grok Bot to finish its setup. It
// takes the bus URL, the token and the routine text.
const botPromptTemplate = `Connect a routine to birdie, which lets you talk to other AI agents on this machine. If you already have a routine for birdie, use it. Otherwise, create one that a webhook starts.

1. Add these two secrets to the routine's settings:

   BIRDIE_URL = %s
   BIRDIE_TOKEN = %s

2. Add everything between BEGIN and END to the routine's instructions:

BEGIN
%s
END

3. Check it works: call GET $BIRDIE_URL/v1/agents with the header Authorization: Bearer $BIRDIE_TOKEN, then tell me who you can reach. If the call can't connect, tell me that, because your runs may not be able to reach this machine.

4. Give me the routine's webhook URL and the key that webhook needs. My birdie setup is waiting for them in my terminal.

If you can't change the routine or get those two values yourself, tell me exactly where to do it in your settings. Don't repeat BIRDIE_TOKEN in your replies.`

func botPromptPath() string { return filepath.Join(configDir(), "grok-bot-prompt.md") }

// routineText returns the part of the routine doc meant for Grok Bot: the
// fenced block, without the notes around it.
func routineText() string {
	_, rest, ok := strings.Cut(birdie.GrokBotRoutine, "```text\n")
	if !ok {
		return birdie.GrokBotRoutine
	}
	text, _, _ := strings.Cut(rest, "\n```")
	return text
}

// botPrompt fills in the prompt with the bus URL and Grok Bot's token.
func botPrompt(url string) (string, error) {
	b, err := os.ReadFile(filepath.Join(tokenDir(), botAgent))
	if err != nil {
		return "", fmt.Errorf("read %s's token: %w", botAgent, err)
	}
	return fmt.Sprintf(botPromptTemplate, url, strings.TrimSpace(string(b)), routineText()), nil
}

// saveBotPrompt writes the prompt where only this user can read it. It removes
// any earlier copy first, because WriteFile would keep that file's mode.
func saveBotPrompt(prompt string) (string, error) {
	path := botPromptPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(path, []byte(prompt+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// setupBot is `birdie setup grok-bot`: the Grok Bot part of onboarding, on its
// own. With no terminal to ask in, p is nil and the webhook comes from flags.
func setupBot(ctx context.Context, o setupOptions, url, webhook, authFile string, p prompter, out io.Writer) error {
	w := &wizard{ctx: ctx, p: p, out: out, o: o, url: url}
	delivery := w.botDelivery()

	if o.remove {
		switch {
		case delivery == "":
			fmt.Fprintf(out, "%s isn't set up, so there's nothing to remove\n", botAgent)
			return nil
		case o.dry:
			fmt.Fprintf(out, "would delete agent %s and its token; its routine would need a new token if you reconnect\n", botAgent)
			return nil
		}
		return w.deleteAgent(botAgent)
	}

	var details *botDetails
	if webhook != "" || authFile != "" {
		if webhook == "" || authFile == "" {
			return errors.New("--webhook and --auth-file go together")
		}
		if !validWebhookURL(webhook) {
			return errors.New("--webhook doesn't look like a web address; it should start with https://")
		}
		abs, err := filepath.Abs(expandHome(authFile, o.home))
		if err != nil {
			return err
		}
		if !keyFileOK(abs) {
			return fmt.Errorf("can't read a key from %s", authFile)
		}
		details = &botDetails{url: webhook, keyFile: abs}
	}

	if o.dry {
		if delivery == "" {
			fmt.Fprintf(out, "would add agent %s and save its token to %s\n", botAgent, filepath.Join(tokenDir(), botAgent))
			fmt.Fprintf(out, "would save the routine text to %s\n", routinePath())
		}
		switch {
		case details != nil:
			fmt.Fprintf(out, "would deliver to %s, reading the key from %s\n", urlHost(details.url), details.keyFile)
		case delivery == "webhook":
			fmt.Fprintf(out, "%s already delivers to its webhook, so nothing would change\n", botAgent)
		default:
			fmt.Fprintln(out, "would give you a prompt for Grok Bot, and ask for the webhook URL and key it gives you")
		}
		return nil
	}

	if delivery == "webhook" && details == nil {
		fmt.Fprintf(out, "%s is already set up. To get its prompt: birdie agent prompt %s\n", botAgent, botAgent)
		fmt.Fprintf(out, "To change its webhook: birdie setup %s --webhook URL --auth-file PATH\n", botAgent)
		return nil
	}
	if delivery == "" {
		if err := w.registerBot(); err != nil {
			return err
		}
	}
	if details != nil {
		if err := w.connectBot(*details); err != nil {
			return err
		}
		if delivery != "webhook" {
			fmt.Fprintf(out, "If Grok Bot doesn't have its prompt yet, get it with: birdie agent prompt %s\n", botAgent)
		}
		return nil
	}
	if !busUp(ctx, url) {
		fmt.Fprintf(out, "The bus isn't running at %s, and Grok Bot's check needs it. Start it with: birdie service install, or birdie serve\n", url)
	}
	if p == nil {
		fmt.Fprintln(out, "There's no terminal to ask in, so finish in two steps:")
		fmt.Fprintf(out, "  1. Give Grok Bot its prompt, from: birdie agent prompt %s\n", botAgent)
		fmt.Fprintf(out, "  2. Enter what it gives you: birdie setup %s --webhook URL --auth-file PATH\n", botAgent)
		return nil
	}
	return w.finishBot(delivery == bus.DeliveryPull)
}
