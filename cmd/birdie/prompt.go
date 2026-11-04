package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

var errCancelled = errors.New("cancelled, so nothing changed")

// prompter asks the questions of `birdie onboard`.
type prompter interface {
	confirm(question string, def bool) (bool, error)
	line(question, def string) (string, error)
	secret(question string) (string, error)
	choose(title string, items []*wizardItem, footer string) error
}

// terminal is a prompter on a real terminal. It reads the terminal itself, so
// it still works under `curl ... | sh`, where stdin is the script.
type terminal struct {
	ctx context.Context
	in  *os.File
	out io.Writer
}

func openTerminal(ctx context.Context, out io.Writer) (*terminal, func(), error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return &terminal{ctx, os.Stdin, out}, func() {}, nil
	}
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	if !term.IsTerminal(int(f.Fd())) {
		f.Close()
		return nil, nil, errors.New("not a terminal")
	}
	return &terminal{ctx, f, out}, func() { f.Close() }, nil
}

// read waits for input without blocking a cancelled context.
func (t *terminal) read(p []byte) (int, error) {
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := t.in.Read(p)
		done <- result{n, err}
	}()
	select {
	case r := <-done:
		return r.n, r.err
	case <-t.ctx.Done():
		return 0, errCancelled
	}
}

func (t *terminal) readLine() (string, error) {
	buf := make([]byte, 4096)
	var line []byte
	for {
		n, err := t.read(buf)
		line = append(line, buf[:n]...)
		if i := strings.IndexByte(string(line), '\n'); i >= 0 {
			return strings.TrimSpace(string(line[:i])), nil
		}
		if errors.Is(err, errCancelled) {
			return "", err
		}
		if err != nil {
			return "", errCancelled // the terminal closed
		}
	}
}

func (t *terminal) confirm(question string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	for {
		fmt.Fprintf(t.out, "%s %s ", question, hint)
		a, err := t.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(a) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(t.out, "Please answer y or n.")
	}
}

func (t *terminal) line(question, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(t.out, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(t.out, "%s: ", question)
	}
	a, err := t.readLine()
	if err != nil {
		return "", err
	}
	if a == "" {
		return def, nil
	}
	return a, nil
}

// secret reads without echoing, and restores the terminal if it's interrupted.
func (t *terminal) secret(question string) (string, error) {
	fd := int(t.in.Fd())
	state, err := term.GetState(fd)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(t.out, "%s: ", question)
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, err := term.ReadPassword(fd)
		done <- result{b, err}
	}()
	select {
	case r := <-done:
		fmt.Fprintln(t.out)
		if r.err != nil {
			return "", r.err
		}
		return strings.TrimSpace(string(r.b)), nil
	case <-t.ctx.Done():
		term.Restore(fd, state)
		fmt.Fprintln(t.out)
		return "", errCancelled
	}
}

// choose runs the checklist in raw mode: arrows or j/k move, space toggles.
func (t *terminal) choose(title string, items []*wizardItem, footer string) error {
	fd := int(t.in.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer term.Restore(fd, state)

	c := &checklist{items: items}
	drawn := 0
	buf := make([]byte, 16)
	for {
		if drawn > 0 {
			fmt.Fprintf(t.out, "\x1b[%dA\r\x1b[J", drawn)
		}
		width, _, err := term.GetSize(fd)
		if err != nil {
			width = 0
		}
		lines := c.render(title, footer, width)
		fmt.Fprint(t.out, strings.Join(lines, "\r\n")+"\r\n")
		drawn = len(lines)

		n, err := t.read(buf)
		if err != nil {
			return errCancelled
		}
		for _, k := range splitKeys(string(buf[:n])) {
			switch c.key(k) {
			case keyDone:
				return nil
			case keyCancel:
				return errCancelled
			}
		}
	}
}

// splitKeys separates keys that arrive together, as they do when an arrow key
// is held down.
func splitKeys(s string) []string {
	var keys []string
	for len(s) > 0 {
		n := 1
		if len(s) >= 3 && s[0] == '\x1b' && (s[1] == '[' || s[1] == 'O') {
			n = 3
		}
		keys = append(keys, s[:n])
		s = s[n:]
	}
	return keys
}

type keyResult int

const (
	keyContinue keyResult = iota
	keyDone
	keyCancel
)

type checklist struct {
	items  []*wizardItem
	cursor int
}

func (c *checklist) key(k string) keyResult {
	switch k {
	case "\x1b[A", "\x1bOA", "k":
		c.cursor = (c.cursor - 1 + len(c.items)) % len(c.items)
	case "\x1b[B", "\x1bOB", "j":
		c.cursor = (c.cursor + 1) % len(c.items)
	case " ", "x":
		c.items[c.cursor].ticked = !c.items[c.cursor].ticked
	case "\r", "\n":
		return keyDone
	case "q", "\x03", "\x04":
		return keyCancel
	}
	return keyContinue
}

func (c *checklist) render(title, footer string, width int) []string {
	lines := []string{title, ""}
	for i, it := range c.items {
		cursor, mark := " ", " "
		if i == c.cursor {
			cursor = ">"
		}
		if it.ticked {
			mark = "x"
		}
		line := fmt.Sprintf("%s [%s] %-22s %s", cursor, mark, it.label, it.note)
		lines = append(lines, fitWidth(strings.TrimRight(line, " "), width))
	}
	lines = append(lines, "")
	if footer != "" {
		lines = append(lines, fitWidth(footer, width))
	}
	return append(lines, fitWidth("↑/↓ move, space to tick or untick, Enter to continue, q to cancel", width))
}

// fitWidth cuts a line so it can't wrap, which would break redrawing the list.
func fitWidth(s string, width int) string {
	r := []rune(s)
	if width > 1 && len(r) >= width {
		return string(r[:width-1])
	}
	return s
}
