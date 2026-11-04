package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// serviceManager keeps `birdie serve` running in the background.
type serviceManager interface {
	name() string
	available() bool
	state() (installed, active bool)
	install(exe, db string) error
	remove() error
}

func newServiceManager() serviceManager {
	switch runtime.GOOS {
	case "linux":
		return systemdService{}
	case "darwin":
		return launchdService{}
	}
	return noService{}
}

func serviceCmd(ctx context.Context, args []string) error {
	const use = "usage: birdie service install|remove|status [--db PATH]"
	if len(args) == 0 {
		return errors.New(use)
	}
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	db := fs.String("db", defaultDB(), "SQLite database path")
	if _, err := parse(fs, args[1:]); err != nil {
		return err
	}
	svc := newServiceManager()
	switch args[0] {
	case "install":
		if !svc.available() {
			return fmt.Errorf("no background service manager here; run `birdie serve` yourself")
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if err := svc.install(exe, *db); err != nil {
			return err
		}
		fmt.Printf("birdie now runs in the background through %s.\n", svc.name())
	case "remove":
		if err := svc.remove(); err != nil {
			return err
		}
		fmt.Println("removed the background service.")
	case "status":
		installed, active := svc.state()
		fmt.Printf("%s: installed=%v running=%v\n", svc.name(), installed, active)
	default:
		return errors.New(use)
	}
	return nil
}

// servicePath is the PATH the service runs with. Service managers start
// programs with a minimal PATH, which would hide commands agents run through,
// such as grok in ~/.local/bin.
func servicePath(exe string) string {
	return filepath.Dir(exe) + string(os.PathListSeparator) + os.Getenv("PATH")
}

func runQuiet(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".birdie-tmp"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// systemd (Linux)

const systemdUnit = "birdie.service"

type systemdService struct{}

func (systemdService) name() string { return "systemd" }

func (systemdService) path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "systemd", "user", systemdUnit)
}

func (systemdService) available() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return exec.Command("systemctl", "--user", "show-environment").Run() == nil
}

func (s systemdService) state() (bool, bool) {
	_, err := os.Stat(s.path())
	active := exec.Command("systemctl", "--user", "is-active", "--quiet", systemdUnit).Run() == nil
	return err == nil, active
}

func (s systemdService) install(exe, db string) error {
	if err := writeFileAtomic(s.path(), []byte(systemdUnitText(exe, db, servicePath(exe))), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", systemdUnit},
		{"--user", "restart", systemdUnit},
	} {
		if err := runQuiet("systemctl", args...); err != nil {
			return err
		}
	}
	return nil
}

func (s systemdService) remove() error {
	if _, err := os.Stat(s.path()); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := runQuiet("systemctl", "--user", "disable", "--now", systemdUnit); err != nil {
		return err
	}
	if err := os.Remove(s.path()); err != nil {
		return err
	}
	return runQuiet("systemctl", "--user", "daemon-reload")
}

func systemdUnitText(exe, db, path string) string {
	return fmt.Sprintf(`[Unit]
Description=birdie local agent message bus

[Service]
ExecStart=%s serve --db %s
Environment=%s
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, systemdArg(exe), systemdArg(db), systemdArg("PATH="+path))
}

// systemdArg quotes one word of a unit file, including its % and $ expansions.
func systemdArg(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`).Replace(s)
	return `"` + s + `"`
}

// launchd (macOS)

const launchdLabel = "dev.birdie.serve"

type launchdService struct{}

func (launchdService) name() string { return "launchd" }

func (launchdService) path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func (launchdService) target() string { return fmt.Sprintf("gui/%d/%s", os.Getuid(), launchdLabel) }

func (launchdService) available() bool {
	_, err := exec.LookPath("launchctl")
	return err == nil
}

func (s launchdService) state() (bool, bool) {
	_, err := os.Stat(s.path())
	active := exec.Command("launchctl", "print", s.target()).Run() == nil
	return err == nil, active
}

func (s launchdService) install(exe, db string) error {
	logFile := filepath.Join(configDir(), "serve.log")
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	if err := writeFileAtomic(s.path(), []byte(launchdPlist(exe, db, servicePath(exe), logFile)), 0o644); err != nil {
		return err
	}
	exec.Command("launchctl", "bootout", s.target()).Run() // fine if it wasn't loaded
	return runQuiet("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), s.path())
}

func (s launchdService) remove() error {
	if _, err := os.Stat(s.path()); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	exec.Command("launchctl", "bootout", s.target()).Run()
	return os.Remove(s.path())
}

func launchdPlist(exe, db, path, logFile string) string {
	esc := func(s string) string {
		var b bytes.Buffer
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>serve</string><string>--db</string><string>%s</string></array>
  <key>EnvironmentVariables</key>
  <dict><key>PATH</key><string>%s</string></dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, launchdLabel, esc(exe), esc(db), esc(path), esc(logFile), esc(logFile))
}

// noService is used where birdie has no service support, such as Windows.
type noService struct{}

func (noService) name() string              { return "no service manager" }
func (noService) available() bool           { return false }
func (noService) state() (bool, bool)       { return false, false }
func (noService) install(_, _ string) error { return errors.New("no service manager here") }
func (noService) remove() error             { return nil }
