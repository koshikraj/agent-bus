package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"birdie"
	"birdie/internal/bus"
	"birdie/internal/store"
)

const (
	itemHeadless       = "grok-headless"
	itemHermesHeadless = "hermes-headless"
	itemBot            = "grok-bot"

	botAgent = "grok-bot"
)

// headlessAgent is an agent birdie starts for each message, over ACP.
type headlessAgent struct {
	key, label, tool, agent, command, folder, description string
}

var headlessAgents = []headlessAgent{
	{itemHeadless, "Grok CLI (headless)", "grok", "grok", "grok agent stdio", "grok-work", "Grok CLI: coding questions and quick research"},
	{itemHermesHeadless, "Hermes (headless)", "hermes", "hermes-headless", "hermes acp", "hermes-work", "Hermes Agent: answers with nothing open"},
}

func headlessFor(key string) (headlessAgent, bool) {
	for _, h := range headlessAgents {
		if h.key == key {
			return h, true
		}
	}
	return headlessAgent{}, false
}

// wizardItem is one row of the onboarding checklist.
type wizardItem struct {
	key, label, note string
	agent            string
	ticked           bool
	connected        bool
}

// botDetails are the answers for connecting or changing Grok Bot.
type botDetails struct {
	url       string
	keyFile   string
	pastedKey string // saved to keyFile when set
}

type wizardPlan struct {
	service             bool
	connect, disconnect []*wizardItem
	bot                 *botDetails
	headlessCwd         map[string]string
	hermesWake          *bool
}

func (p wizardPlan) empty() bool {
	return !p.service && len(p.connect) == 0 && len(p.disconnect) == 0
}

type wizard struct {
	ctx context.Context
	p   prompter
	out io.Writer
	svc serviceManager
	o   setupOptions
	url string
}

func onboard(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	db := fs.String("db", defaultDB(), "SQLite database path")
	url := fs.String("url", envOr("BIRDIE_URL", defaultURL), "birdie base URL")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	o, err := newSetupOptions(*db)
	if err != nil {
		return err
	}
	t, closeTerminal, err := openTerminal(ctx, os.Stdout)
	if err != nil {
		return errors.New("onboard asks questions, so it needs a terminal; in a script, use: birdie setup --all")
	}
	defer closeTerminal()
	w := &wizard{ctx: ctx, p: t, out: os.Stdout, svc: newServiceManager(), o: o, url: *url}
	err = w.run()
	if errors.Is(err, errCancelled) {
		fmt.Fprintln(w.out, "Cancelled, so nothing changed.")
		return nil
	}
	return err
}

func (w *wizard) run() error {
	fmt.Fprintln(w.out, "Let's connect your agents to birdie. Nothing changes until you confirm.")
	fmt.Fprintln(w.out)

	plan := wizardPlan{headlessCwd: map[string]string{}}
	var err error
	if plan.service, err = w.askService(); err != nil {
		return err
	}
	items, missing, err := w.items()
	if err != nil {
		return err
	}
	footer := ""
	if len(missing) > 0 {
		footer = "Not found on PATH: " + strings.Join(missing, ", ")
	}
	fmt.Fprintln(w.out)
	if err := w.p.choose("What should connect?", items, footer); err != nil {
		return err
	}
	for _, it := range items {
		switch {
		case it.ticked && !it.connected:
			plan.connect = append(plan.connect, it)
		case it.ticked && setupTools[it.key].apply != nil:
			plan.connect = append(plan.connect, it) // refresh the config
		case !it.ticked && it.connected:
			plan.disconnect = append(plan.disconnect, it)
		}
	}
	if err := w.askDetails(&plan, items); err != nil {
		return err
	}
	if plan.empty() {
		fmt.Fprintln(w.out, "\nNothing to change.")
	} else {
		w.preview(plan)
		ok, err := w.p.confirm("\nApply these changes?", false)
		if err != nil {
			return err
		}
		if !ok {
			return errCancelled
		}
	}
	failed := w.apply(plan)

	fmt.Fprintln(w.out, "\nHealth check:")
	printChecks(w.out, runChecks(w.ctx, w.o.db, w.url, w.o.home, w.svc))
	w.nextSteps(plan)
	if len(failed) > 0 {
		return fmt.Errorf("these steps failed: %s", strings.Join(failed, ", "))
	}
	return nil
}

// askService decides whether to install, or refresh, the background service.
func (w *wizard) askService() (bool, error) {
	if !w.svc.available() {
		if !busUp(w.ctx, w.url) {
			fmt.Fprintln(w.out, "This system has no background service birdie can use, so keep `birdie serve` running yourself.")
		}
		return false, nil
	}
	installed, active := w.svc.state()
	if installed && active {
		return true, nil // restarted so it runs this version
	}
	if busUp(w.ctx, w.url) {
		fmt.Fprintf(w.out, "A bus is already running at %s outside the service, so I'll leave it alone.\n", w.url)
		return false, nil
	}
	return w.p.confirm("Keep birdie running in the background, starting at login?", true)
}

// items lists what can be connected and what's connected already.
func (w *wizard) items() ([]*wizardItem, []string, error) {
	found, _ := detectTools()
	onPath := map[string]bool{}
	for _, name := range found {
		onPath[name] = true
	}
	var items []*wizardItem
	var notFound []string
	for _, name := range setupOrder {
		tool := setupTools[name]
		agent, connected := connectedAs(name, w.o.home)
		if !onPath[name] && !connected {
			notFound = append(notFound, tool.label)
			continue
		}
		it := &wizardItem{key: name, label: tool.label, agent: tool.agent, ticked: true, connected: connected, note: "found"}
		if connected {
			it.agent, it.note = agent, "connected as "+agent
		}
		items = append(items, it)
	}

	st, err := store.Open(w.o.db)
	if err != nil {
		return nil, nil, err
	}
	defer st.Close()
	has := func(name, delivery string) bool {
		a, err := st.GetAgent(w.ctx, name)
		return err == nil && a.Delivery == delivery
	}
	for _, h := range headlessAgents {
		if connected := has(h.agent, "acp"); onPath[h.tool] || connected {
			it := &wizardItem{key: h.key, label: h.label, agent: h.agent, ticked: connected, connected: connected, note: "answers with nothing open"}
			if connected {
				it.note = "connected as " + h.agent
			}
			items = append(items, it)
		}
	}
	connected := has(botAgent, "webhook")
	it := &wizardItem{key: itemBot, label: "Grok Bot", agent: botAgent, ticked: connected, connected: connected, note: "needs its webhook URL and key"}
	if connected {
		it.note = "connected as " + botAgent
	}
	items = append(items, it)
	return items, notFound, nil
}

// askDetails collects what the ticked items still need.
func (w *wizard) askDetails(plan *wizardPlan, items []*wizardItem) error {
	for _, it := range items {
		if !it.ticked {
			continue
		}
		if h, ok := headlessFor(it.key); ok {
			if it.connected {
				continue
			}
			fmt.Fprintf(w.out, "\n%s\n", h.label)
			dir, err := w.p.line("Folder it works in", filepath.Join(w.o.home, h.folder))
			if err != nil {
				return err
			}
			plan.headlessCwd[h.key] = expandHome(dir, w.o.home)
			continue
		}
		switch it.key {
		case "hermes":
			current, _ := readHermesSettings(hermesHome(w.o.home))
			fmt.Fprintln(w.out, "\nHermes")
			wake, err := w.p.confirm("Let other agents' messages start turns in Hermes on their own? That uses your model account", current.Wake)
			if err != nil {
				return err
			}
			plan.hermesWake = &wake
		case itemBot:
			fmt.Fprintln(w.out, "\nGrok Bot")
			if it.connected {
				change, err := w.p.confirm("Change its webhook URL or key?", false)
				if err != nil {
					return err
				}
				if !change {
					continue
				}
				plan.connect = append(plan.connect, it)
			}
			details, err := w.askBot()
			if err != nil {
				return err
			}
			plan.bot = details
		}
	}
	return nil
}

func (w *wizard) askBot() (*botDetails, error) {
	d := &botDetails{}
	for {
		raw, err := w.p.line("Webhook URL of the Grok Bot routine", "")
		if err != nil {
			return nil, err
		}
		u, err := neturl.Parse(raw)
		if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			d.url = raw
			break
		}
		fmt.Fprintln(w.out, "That doesn't look like a web address. It should start with https://")
	}

	saved := defaultBotKeyPath()
	if keyFileOK(saved) {
		use, err := w.p.confirm("Use the key saved in "+saved+"?", true)
		if err != nil {
			return nil, err
		}
		if use {
			d.keyFile = saved
			return d, nil
		}
	}
	for {
		path, err := w.p.line("Path to a file holding its key, or press Enter to paste the key", "")
		if err != nil {
			return nil, err
		}
		if path != "" {
			abs, err := filepath.Abs(expandHome(path, w.o.home))
			if err == nil && keyFileOK(abs) {
				d.keyFile = abs
				return d, nil
			}
			fmt.Fprintf(w.out, "Can't read a key from %s.\n", path)
			continue
		}
		key, err := w.p.secret("Paste the key (it won't be shown)")
		if err != nil {
			return nil, err
		}
		if key != "" {
			d.keyFile, d.pastedKey = saved, key
			return d, nil
		}
		fmt.Fprintln(w.out, "No key entered.")
	}
}

func defaultBotKeyPath() string {
	return filepath.Join(filepath.Dir(configDir()), "grok-bot", "sender-key")
}

func keyFileOK(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(b)) != ""
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}

func routinePath() string { return filepath.Join(configDir(), "grok-bot-routine.md") }

func (w *wizard) toolOptions(it *wizardItem, plan wizardPlan, remove bool) setupOptions {
	o := w.o
	o.as, o.remove, o.quiet = it.agent, remove, true
	if it.key == "hermes" {
		o.wake = plan.hermesWake
	}
	return o
}

func (w *wizard) preview(plan wizardPlan) {
	fmt.Fprintln(w.out, "\nChanges to make:")
	if plan.service {
		fmt.Fprintf(w.out, "\nBackground service:\nwould install it through %s and restart it\n", w.svc.name())
	}
	for _, it := range plan.connect {
		fmt.Fprintf(w.out, "\n%s:\n", it.label)
		if h, ok := headlessFor(it.key); ok {
			fmt.Fprintf(w.out, "would add agent %s, running `%s` in %s\n", h.agent, h.command, plan.headlessCwd[h.key])
			continue
		}
		switch it.key {
		case itemBot:
			verb := "add agent"
			if it.connected {
				verb = "update agent"
			}
			fmt.Fprintf(w.out, "would %s %s, delivering to %s\n", verb, botAgent, urlHost(plan.bot.url))
			if plan.bot.pastedKey != "" {
				fmt.Fprintf(w.out, "would save the key you pasted to %s\n", plan.bot.keyFile)
			} else {
				fmt.Fprintf(w.out, "would read the key from %s\n", plan.bot.keyFile)
			}
			fmt.Fprintf(w.out, "would save the routine text to %s\n", routinePath())
		default:
			if err := setupOne(w.ctx, it.key, w.toolOptions(it, plan, false), true); err != nil {
				fmt.Fprintf(w.out, "can't preview: %v\n", err)
			}
		}
	}
	for _, it := range plan.disconnect {
		fmt.Fprintf(w.out, "\n%s (unticked):\n", it.label)
		if h, ok := headlessFor(it.key); ok {
			fmt.Fprintf(w.out, "would delete agent %s and its token\n", h.agent)
			continue
		}
		switch it.key {
		case itemBot:
			fmt.Fprintf(w.out, "would delete agent %s and its token; its routine would need a new token if you reconnect\n", botAgent)
		default:
			if err := setupOne(w.ctx, it.key, w.toolOptions(it, plan, true), true); err != nil {
				fmt.Fprintf(w.out, "can't preview: %v\n", err)
			}
		}
	}
}

// apply carries out every step, and keeps going after a failure.
func (w *wizard) apply(plan wizardPlan) (failed []string) {
	step := func(name string, run func() error) {
		fmt.Fprintf(w.out, "\n%s:\n", name)
		if err := run(); err != nil {
			fmt.Fprintf(w.out, "failed: %v\n", err)
			failed = append(failed, name)
		}
	}
	for _, it := range plan.disconnect {
		switch it.key {
		case itemHeadless, itemHermesHeadless, itemBot:
			step(it.label, func() error { return w.deleteAgent(it.agent) })
		default:
			step(it.label, func() error { return setupOne(w.ctx, it.key, w.toolOptions(it, plan, true), false) })
		}
	}
	for _, it := range plan.connect {
		if h, ok := headlessFor(it.key); ok {
			step(it.label, func() error { return w.addHeadless(h, plan.headlessCwd[h.key]) })
			continue
		}
		switch it.key {
		case itemBot:
			step(it.label, func() error { return w.applyBot(*plan.bot, it.connected) })
		default:
			step(it.label, func() error { return setupOne(w.ctx, it.key, w.toolOptions(it, plan, false), false) })
		}
	}
	if plan.service {
		step("Background service", func() error {
			if err := w.svc.install(w.o.exe, w.o.db); err != nil {
				return err
			}
			if !waitBus(w.ctx, w.url) {
				return fmt.Errorf("the service started, but the bus didn't answer at %s", w.url)
			}
			fmt.Fprintln(w.out, "running")
			return nil
		})
	}
	return failed
}

func (w *wizard) deleteAgent(name string) error {
	st, err := store.Open(w.o.db)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.DeleteAgent(w.ctx, name); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(tokenDir(), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Fprintf(w.out, "deleted agent %s\n", name)
	return nil
}

func (w *wizard) addHeadless(h headlessAgent, cwd string) error {
	argv := strings.Fields(h.command)
	if _, err := exec.LookPath(argv[0]); err != nil {
		return err
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	st, err := store.Open(w.o.db)
	if err != nil {
		return err
	}
	defer st.Close()
	err = createAgent(w.ctx, st, bus.Agent{
		Name: h.agent, Delivery: "acp", CreatedAt: time.Now().UTC(),
		Config: map[string]string{
			"command": h.command, "permission": "reject", "cwd": abs,
			"description": h.description,
		},
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(w.out, "added agent %s\n", h.agent)
	return nil
}

func (w *wizard) applyBot(d botDetails, existing bool) error {
	r := &setupRun{stamp: w.o.stamp, home: w.o.home}
	if d.pastedKey != "" {
		if err := os.MkdirAll(filepath.Dir(d.keyFile), 0o700); err != nil {
			return err
		}
		if err := r.write(d.keyFile, d.pastedKey+"\n"); err != nil {
			return err
		}
	}
	st, err := store.Open(w.o.db)
	if err != nil {
		return err
	}
	defer st.Close()
	if existing {
		if err := st.SetAgentConfig(w.ctx, botAgent, "url", d.url); err != nil {
			return err
		}
		if err := st.SetAgentConfig(w.ctx, botAgent, "auth_file", d.keyFile); err != nil {
			return err
		}
		fmt.Fprintf(w.out, "updated agent %s\n", botAgent)
	} else {
		err := createAgent(w.ctx, st, bus.Agent{
			Name: botAgent, Delivery: "webhook", CreatedAt: time.Now().UTC(),
			Config: map[string]string{
				"url": d.url, "auth_file": d.keyFile,
				"description": "Grok Bot: personal assistant (email, calendar, web); answers in its own time",
			},
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(w.out, "added agent %s\n", botAgent)
	}
	return r.write(routinePath(), birdie.GrokBotRoutine)
}

func (w *wizard) nextSteps(plan wizardPlan) {
	var steps []string
	for _, it := range plan.connect {
		if _, isTool := setupTools[it.key]; isTool {
			steps = append(steps, "Restart any open sessions of the tools you connected.")
			break
		}
	}
	for _, it := range append(plan.connect[:len(plan.connect):len(plan.connect)], plan.disconnect...) {
		if it.key == "hermes" {
			steps = append(steps, "Restart Hermes to load the change: hermes gateway restart.")
			break
		}
	}
	if plan.bot != nil {
		steps = append(steps,
			"In the Grok Bot routine, add the secret BIRDIE_URL = "+defaultURL+".",
			"Add the secret BIRDIE_TOKEN, pasting the contents of "+filepath.Join(tokenDir(), botAgent)+".",
			"Paste the routine text from "+routinePath()+" into the routine.",
		)
	}
	if len(steps) == 0 {
		return
	}
	fmt.Fprintln(w.out, "\nWhat's next:")
	for i, s := range steps {
		fmt.Fprintf(w.out, "  %d. %s\n", i+1, s)
	}
}
