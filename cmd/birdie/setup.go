package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"birdie"
	"birdie/internal/bus"
	"birdie/internal/mcpserver"
	"birdie/internal/store"
)

// legacyServer is the project's old name. Setup replaces what it left in a
// tool's config.
const legacyServer = "agent-bus"

// setupTool wires one agent program to the bus.
type setupTool struct {
	label       string
	agent       string // default agent name
	description string
	apply       func(*setupRun) error
}

var setupTools = map[string]setupTool{
	"claude": {"Claude Code", "claude", "Claude Code on this machine: code, repos, files", setupClaude},
	"codex":  {"Codex", "codex", "Codex on this machine: code, repos, files", setupCodex},
	"grok":   {"Grok CLI", "grok-tui", "Grok CLI on this machine: coding questions and quick research", setupGrok},
}

// setupOrder is the order tools are listed in. Each name is also the command
// looked for on PATH.
var setupOrder = []string{"claude", "codex", "grok"}

// setupRun carries one setup and records what it changed.
type setupRun struct {
	exe, agent string
	remove     bool
	dry        bool
	stamp      string
	home       string
	said       int
}

// setupOptions are the settings shared by every tool in one setup command.
type setupOptions struct {
	as, db, exe, home, stamp string
	remove, dry              bool
	quiet                    bool // leave closing advice to the caller
}

func setup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	as := fs.String("as", "", "agent name on the bus (default: claude, codex or grok-tui); needs a tool name")
	remove := fs.Bool("remove", false, "undo the setup; the agent and its token stay")
	dry := fs.Bool("dry-run", false, "print what would change")
	all := fs.Bool("all", false, "set up every tool found on PATH, without asking")
	db := fs.String("db", defaultDB(), "SQLite database path")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 1 || (len(pos) == 0) != *all {
		return errors.New("usage: birdie setup claude|codex|grok|--all [--as NAME] [--remove] [--dry-run]\nFor a guided setup, run: birdie onboard")
	}
	o, err := newSetupOptions(*db)
	if err != nil {
		return err
	}
	o.as, o.remove, o.dry = *as, *remove, *dry
	if *all {
		return setupAll(ctx, o, os.Stdout)
	}
	if _, ok := setupTools[pos[0]]; !ok {
		return fmt.Errorf("can't set up %q; choose claude, codex or grok", pos[0])
	}
	return setupOne(ctx, pos[0], o, o.dry)
}

func newSetupOptions(db string) (setupOptions, error) {
	exe, err := os.Executable()
	if err != nil {
		return setupOptions{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return setupOptions{}, err
	}
	return setupOptions{db: db, exe: exe, home: home, stamp: time.Now().Format("20060102-150405")}, nil
}

// setupOne applies or previews the setup of one tool.
func setupOne(ctx context.Context, name string, o setupOptions, dry bool) error {
	tool := setupTools[name]
	r := &setupRun{exe: o.exe, agent: tool.agent, remove: o.remove, dry: dry, stamp: o.stamp, home: o.home}
	if o.as != "" {
		r.agent = o.as
	} else if agent, ok := connectedAs(name, o.home); ok {
		r.agent = agent // keep the name it's connected under
	}
	if !o.remove && !dry {
		if err := ensureAgent(ctx, o.db, r.agent, tool.description); err != nil {
			return err
		}
	}
	if err := tool.apply(r); err != nil {
		return err
	}
	switch {
	case dry:
		if r.said == 0 {
			fmt.Println("nothing to change")
		}
	case o.quiet:
	case o.remove:
		fmt.Printf("removed. The agent %s and its token stay; remove them with: birdie agent rm %s\n", r.agent, r.agent)
	default:
		fmt.Printf("done. Restart any open %s sessions, and keep `birdie serve` running.\n", name)
	}
	return nil
}

// setupAll sets up every tool found on PATH. A failure doesn't stop the rest.
func setupAll(ctx context.Context, o setupOptions, out io.Writer) error {
	if o.as != "" {
		return errors.New("--as names one agent, so it needs a tool name: birdie setup claude --as NAME")
	}
	if o.remove {
		return errors.New("--remove needs a tool name, so nothing is removed by accident")
	}
	found, missing := detectTools()
	if len(found) == 0 {
		return fmt.Errorf("found none of %s on PATH", strings.Join(setupOrder, ", "))
	}
	if len(missing) > 0 {
		fmt.Fprintf(out, "Not found on PATH: %s\n", strings.Join(missing, ", "))
	}
	var failed []string
	for _, name := range found {
		fmt.Fprintf(out, "\n%s:\n", setupTools[name].label)
		if err := setupOne(ctx, name, o, o.dry); err != nil {
			fmt.Fprintf(out, "failed: %v\n", err)
			failed = append(failed, name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("setup failed for %s", strings.Join(failed, ", "))
	}
	return nil
}

// detectTools splits the known tools into those on PATH and those not.
func detectTools() (found, missing []string) {
	for _, name := range setupOrder {
		if _, err := exec.LookPath(name); err == nil {
			found = append(found, name)
		} else {
			missing = append(missing, name)
		}
	}
	return found, missing
}

func ensureAgent(ctx context.Context, db, name, description string) error {
	st, err := store.Open(db)
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.GetAgent(ctx, name); err == nil {
		return nil
	} else if !errors.Is(err, bus.ErrNotFound) {
		return err
	}
	if err := createAgent(ctx, st, bus.Agent{
		Name: name, Delivery: bus.DeliveryPull, CreatedAt: time.Now().UTC(),
		Config: map[string]string{"description": description},
	}); err != nil {
		return err
	}
	fmt.Printf("added agent %s\n", name)
	return nil
}

func (r *setupRun) say(format string, args ...any) {
	r.said++
	prefix := ""
	if r.dry {
		prefix = "would "
	}
	fmt.Println(prefix + fmt.Sprintf(format, args...))
}

// backup copies a file about to change into the birdie config folder.
func (r *setupRun) backup(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	dir := filepath.Join(configDir(), "backups", r.stamp)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	dest := filepath.Join(dir, strings.TrimLeft(strings.ReplaceAll(strings.TrimPrefix(path, r.home), string(filepath.Separator), "_"), "_"))
	if err := os.WriteFile(dest, b, 0o600); err != nil {
		return err
	}
	fmt.Printf("backed up %s to %s\n", path, dest)
	return nil
}

// write replaces a file's content, with a backup, if it would change.
func (r *setupRun) write(path, content string) error {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && string(old) == content {
		return nil
	}
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	r.say("write %s", path)
	if r.dry {
		return nil
	}
	if err := r.backup(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".birdie-tmp"
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (r *setupRun) removeFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	r.say("remove %s", path)
	if r.dry {
		return nil
	}
	if err := r.backup(path); err != nil {
		return err
	}
	return os.Remove(path)
}

func (r *setupRun) skill(dir string) error {
	path := filepath.Join(dir, "talk-to-agents", "SKILL.md")
	if r.remove {
		if err := r.removeFile(path); err != nil {
			return err
		}
		if !r.dry {
			os.Remove(filepath.Dir(path)) // only if empty
		}
		return nil
	}
	return r.write(path, birdie.Skill)
}

// tomlServer edits the birdie MCP server in a TOML config file.
func (r *setupRun) tomlServer(path string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	block := ""
	if !r.remove {
		block = fmt.Sprintf(`[mcp_servers.birdie]
command = %q
args = ["mcp", "--as", %q]
enabled = true
tool_timeout_sec = 660
`, r.exe, r.agent)
	}
	text := setTOMLBlock(string(b), "mcp_servers."+legacyServer, "")
	return r.write(path, setTOMLBlock(text, "mcp_servers.birdie", block))
}

// setTOMLBlock replaces a table and its subtables, or adds it at the end. An
// empty block removes it. Everything else in the file stays as written.
func setTOMLBlock(text, table, block string) string {
	var kept []string
	skipping := false
	for _, line := range strings.SplitAfter(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			name := strings.Trim(t, "[] ")
			skipping = name == table || strings.HasPrefix(name, table+".")
		}
		if !skipping {
			kept = append(kept, line)
		}
	}
	out := strings.TrimRight(strings.Join(kept, ""), "\n")
	if out != "" {
		out += "\n"
	}
	if block != "" {
		if out != "" {
			out += "\n"
		}
		out += block
	}
	return out
}

func envHome(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}

func setupClaude(r *setupRun) error {
	dir := envHome("CLAUDE_CONFIG_DIR", filepath.Join(r.home, ".claude"))
	if err := r.skill(filepath.Join(dir, "skills")); err != nil {
		return err
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("the claude command isn't on PATH: %w", err)
	}
	r.say("update the birdie MCP server in Claude Code's user config")
	if r.dry {
		return nil
	}
	if err := r.backup(filepath.Join(envHome("CLAUDE_CONFIG_DIR", r.home), ".claude.json")); err != nil {
		return err
	}
	// Adding fails if it exists, so start clean. It's fine if there's nothing to remove.
	exec.Command("claude", "mcp", "remove", "-s", "user", legacyServer).Run()
	exec.Command("claude", "mcp", "remove", "-s", "user", "birdie").Run()
	if r.remove {
		return nil
	}
	cmd := exec.Command("claude", "mcp", "add", "-s", "user", "birdie", "--", r.exe, "mcp", "--as", r.agent)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("claude mcp add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func setupCodex(r *setupRun) error {
	dir := envHome("CODEX_HOME", filepath.Join(r.home, ".codex"))
	if err := r.skill(filepath.Join(dir, "skills")); err != nil {
		return err
	}
	return r.tomlServer(filepath.Join(dir, "config.toml"))
}

func setupGrok(r *setupRun) error {
	dir := envHome("GROK_HOME", filepath.Join(r.home, ".grok"))
	if err := r.skill(filepath.Join(dir, "skills")); err != nil {
		return err
	}
	if err := r.tomlServer(filepath.Join(dir, "config.toml")); err != nil {
		return err
	}
	path := mcpserver.GrokHooksPath()
	if err := r.removeFile(filepath.Join(filepath.Dir(path), legacyServer+".json")); err != nil {
		return err
	}
	if r.remove {
		return r.removeFile(path)
	}
	content, err := grokHooks(r.exe, r.agent)
	if err != nil {
		return err
	}
	return r.write(path, content)
}

func grokHooks(exe, agent string) (string, error) {
	group := func(event string, matcher bool) map[string]any {
		g := map[string]any{"hooks": []map[string]any{{
			"type":    "command",
			"command": fmt.Sprintf("%s hook grok %s --as %s", shellQuote(exe), event, shellQuote(agent)),
			"timeout": 15,
		}}}
		if matcher {
			g["matcher"] = "*"
		}
		return g
	}
	b, err := json.MarshalIndent(map[string]any{"hooks": map[string]any{
		"PostToolUse": []any{group("post-tool-use", true)},
		"Stop":        []any{group("stop", false)},
	}}, "", "  ")
	return string(b) + "\n", err
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
