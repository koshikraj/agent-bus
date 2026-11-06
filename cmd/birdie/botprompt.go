package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"birdie"
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
