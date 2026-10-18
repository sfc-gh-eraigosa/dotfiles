// Package configui is `mono config` with no arguments: a screen listing the
// prefix, the title bar and every pane (tool, size, refresh, whether it can
// run on this host), with a live preview of the stacked layout. Edits go
// through the same config.Set/Add/Remove/Move as the CLI verbs, and every
// : command mirrors a `mono config` verb. Built on sdk/libs/tui.
package configui

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/cmdline"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/nav"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/layout"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/probe"
)

// Screen actions on top of keymap.Vim.
const (
	ActSize     keymap.Action = "size"
	ActAdd      keymap.Action = "add"
	ActDelete   keymap.Action = "delete"
	ActMoveDown keymap.Action = "move-down"
	ActMoveUp   keymap.Action = "move-up"
)

// Keys is the config screen's key table.
func Keys() keymap.Map {
	return keymap.Vim.Without(
		keymap.PageLeft, keymap.PageRight, keymap.Search, keymap.NextMatch,
		keymap.PrevMatch, keymap.ClearHighlight, keymap.Select,
	).Merge(
		keymap.Binding{Action: keymap.Confirm, Keys: []string{"enter"}, Help: "edit the row (prefix, title size, or the pane's command)", Short: "edit", Header: true},
		keymap.Binding{Action: ActSize, Keys: []string{"s"}, Help: "edit the pane's size", Short: "size", Header: true},
		keymap.Binding{Action: ActAdd, Keys: []string{"a"}, Help: "add a pane", Short: "add", Header: true},
		keymap.Binding{Action: ActDelete, Keys: []string{"d"}, Help: "remove the pane (asks first)", Short: "remove", Header: true},
		keymap.Binding{Action: ActMoveDown, Keys: []string{"J"}, Help: "move the pane down", Short: "reorder", Group: "reorder", Header: true},
		keymap.Binding{Action: ActMoveUp, Keys: []string{"K"}, Help: "move the pane up", Short: "reorder", Group: "reorder", Header: true},
		keymap.Binding{Action: keymap.Command, Keys: []string{":"}, Help: "command line (:set :add :remove :move :w :q)", Short: "command", Header: true},
		keymap.Binding{Action: keymap.Quit, Keys: []string{"q", "ctrl+c"}, Help: "quit (asks when there are unsaved changes)", Short: "quit", Header: true},
	)
}

type mode int

const (
	modeList mode = iota
	modeCommand
	modeHelp
	modeConfirm
)

// Model is the config screen.
type Model struct {
	Path string
	Env  probe.Env
	Save func(path string, c config.Config) error

	cfg     config.Config
	saved   config.Config
	choices []probe.Choice
	keys    keymap.Map
	cur     nav.Cursor
	mode    mode
	cmd     cmdline.State
	reg     cmdline.Registry
	confirm overlay.Confirm
	onYes   func() tea.Cmd
	msg     string
	isErr   bool
	width   int
	height  int
}

type probedMsg []probe.Choice

// New builds the screen for cfg as loaded from path.
func New(path string, cfg config.Config, env probe.Env) *Model {
	if env == nil {
		env = probe.System{}
	}
	m := &Model{Path: path, Env: env, Save: config.Save, cfg: cfg, saved: cfg, keys: Keys(), width: 80, height: 24}
	m.cur.SetLen(m.rowCount())
	m.cur.SetHeight(10)
	m.register()
	return m
}

func (m *Model) rowCount() int { return 2 + len(m.cfg.Panes) }

// Dirty reports unsaved changes.
func (m *Model) Dirty() bool { return !reflect.DeepEqual(m.cfg, m.saved) }

// Config is the working copy.
func (m *Model) Config() config.Config { return m.cfg }

func (m *Model) probe() tea.Cmd {
	cfg, env := m.cfg, m.Env
	return func() tea.Msg { return probedMsg(probe.Choose(context.Background(), env, cfg.Panes)) }
}

// Init starts the tool check.
func (m *Model) Init() tea.Cmd { return m.probe() }

func (m *Model) register() {
	onHelp := func() { m.mode = modeHelp }
	for _, s := range cmdline.Standard(onHelp, func(string) {}) {
		switch s.Name {
		case "search":
		case "q":
			m.reg.Register(cmdline.Spec{Name: "q", Aliases: []string{"quit"}, Help: "quit (refuses with unsaved changes; :q! discards)",
				Run: func([]string) (tea.Cmd, error) {
					if m.Dirty() {
						return nil, fmt.Errorf("unsaved changes: :w to save, :wq to save and quit, :q! to discard")
					}
					return tea.Quit, nil
				}})
		default:
			m.reg.Register(s)
		}
	}
	keyComplete := func(i int, prefix string) []string {
		if i != 0 {
			return nil
		}
		var out []string
		for _, k := range m.cfg.Keys() {
			if strings.HasPrefix(k, prefix) {
				out = append(out, k)
			}
		}
		return out
	}
	paneComplete := func(i int, prefix string) []string {
		if i != 0 {
			return nil
		}
		var out []string
		for _, p := range m.cfg.Panes {
			if strings.HasPrefix(p.Name, prefix) {
				out = append(out, p.Name)
			}
		}
		return out
	}
	m.reg.Register(
		cmdline.Spec{Name: "q!", Help: "quit without saving", Run: func([]string) (tea.Cmd, error) { return tea.Quit, nil }},
		cmdline.Spec{Name: "w", Help: "save", Run: func([]string) (tea.Cmd, error) { return nil, m.write() }},
		cmdline.Spec{Name: "wq", Aliases: []string{"x"}, Help: "save and quit", Run: func([]string) (tea.Cmd, error) {
			if err := m.write(); err != nil {
				return nil, err
			}
			return tea.Quit, nil
		}},
		cmdline.Spec{Name: "set", Help: "set <key> <value>  (mono config set)", Complete: keyComplete, Run: func(args []string) (tea.Cmd, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("usage: set <key> <value>")
			}
			return m.edit(func(c config.Config) (config.Config, error) { return c.Set(args[0], strings.Join(args[1:], " ")) })
		}},
		cmdline.Spec{Name: "add", Help: "add <name> <size> <cmd...>  (mono config add)", Run: func(args []string) (tea.Cmd, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("usage: add <name> <size> <cmd...>")
			}
			at := -1
			if i := m.cur.Pos - 2; i >= 0 {
				at = i + 1
			}
			p := config.Pane{Name: args[0], Size: args[1], Cmd: strings.Join(args[2:], " ")}
			return m.edit(func(c config.Config) (config.Config, error) { return c.Add(p, at) })
		}},
		cmdline.Spec{Name: "remove", Help: "remove <pane>  (mono config remove)", Complete: paneComplete, Run: func(args []string) (tea.Cmd, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("usage: remove <pane>")
			}
			return m.edit(func(c config.Config) (config.Config, error) { return c.Remove(args[0]) })
		}},
		cmdline.Spec{Name: "move", Help: "move <pane> up|down  (mono config move)", Complete: paneComplete, Run: func(args []string) (tea.Cmd, error) {
			if len(args) != 2 || (args[1] != "up" && args[1] != "down") {
				return nil, fmt.Errorf("usage: move <pane> up|down")
			}
			d := 1
			if args[1] == "up" {
				d = -1
			}
			cmd, err := m.edit(func(c config.Config) (config.Config, error) { return c.Move(args[0], d) })
			if err == nil {
				m.cur.To(m.cfg.Find(args[0]) + 2)
			}
			return cmd, err
		}},
	)
}

func (m *Model) write() error {
	if err := m.Save(m.Path, m.cfg); err != nil {
		return err
	}
	m.saved = m.cfg
	m.say("saved "+m.Path, false)
	return nil
}

// edit applies a change; a rejected change leaves the config untouched.
func (m *Model) edit(fn func(config.Config) (config.Config, error)) (tea.Cmd, error) {
	next, err := fn(m.cfg)
	if err != nil {
		return nil, err
	}
	m.cfg = next
	m.cur.SetLen(m.rowCount())
	m.say("changed — :w to save", false)
	return m.probe(), nil
}

func (m *Model) say(s string, isErr bool) { m.msg, m.isErr = s, isErr }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case probedMsg:
		m.choices = msg
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.cur.SetHeight(max(m.height-14, 3))
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeHelp:
		m.mode = modeList
		return m, nil
	case modeConfirm:
		m.mode = modeList
		if m.confirm.Key(k) == overlay.Yes && m.onYes != nil {
			return m, m.onYes()
		}
		m.say("cancelled", false)
		return m, nil
	case modeCommand:
		ev := m.cmd.Key(k, &m.reg)
		switch ev.Kind {
		case cmdline.Cancelled:
			m.mode = modeList
		case cmdline.Submitted:
			m.mode = modeList
			cmd, err := m.reg.Run(ev.Command)
			if err != nil {
				m.say(err.Error(), true)
			}
			return m, cmd
		}
		return m, nil
	}

	if m.cur.Key(k, m.keys) {
		return m, nil
	}
	act, ok := m.keys.Lookup(k)
	if !ok {
		return m, nil
	}
	pane := m.cur.Pos - 2
	switch act {
	case keymap.Quit:
		if !m.Dirty() {
			return m, tea.Quit
		}
		m.ask(overlay.Confirm{Title: "Quit without saving?", Lines: []string{"unsaved changes to " + m.Path}, YesLabel: "discard"}, func() tea.Cmd { return tea.Quit })
	case keymap.Help:
		m.mode = modeHelp
	case keymap.Command:
		m.prompt("")
	case keymap.Confirm:
		switch {
		case m.cur.Pos == 0:
			m.prompt("set prefix " + m.cfg.Prefix)
		case m.cur.Pos == 1:
			m.prompt("set title.size " + m.cfg.Title.Size)
		default:
			p := m.cfg.Panes[pane]
			m.prompt("set panes." + p.Name + ".cmd " + p.Cmd)
		}
	case ActSize:
		if pane >= 0 {
			p := m.cfg.Panes[pane]
			m.prompt("set panes." + p.Name + ".size " + p.Size)
		} else if m.cur.Pos == 1 {
			m.prompt("set title.size " + m.cfg.Title.Size)
		}
	case ActAdd:
		m.prompt("add ")
	case ActDelete:
		if pane >= 0 {
			name := m.cfg.Panes[pane].Name
			m.ask(overlay.Confirm{Title: "Remove pane " + name + "?", Lines: []string{m.cfg.Panes[pane].Cmd}, YesLabel: "remove"}, func() tea.Cmd {
				cmd, err := m.edit(func(c config.Config) (config.Config, error) { return c.Remove(name) })
				if err != nil {
					m.say(err.Error(), true)
				}
				return cmd
			})
		}
	case ActMoveDown, ActMoveUp:
		if pane >= 0 {
			d := 1
			if act == ActMoveUp {
				d = -1
			}
			name := m.cfg.Panes[pane].Name
			cmd, _ := m.edit(func(c config.Config) (config.Config, error) { return c.Move(name, d) })
			m.cur.To(m.cfg.Find(name) + 2)
			return m, cmd
		}
	}
	return m, nil
}

func (m *Model) prompt(text string) {
	m.mode = modeCommand
	m.cmd = cmdline.State{}
	m.cmd.Input.SetText(text)
	m.msg = ""
}

func (m *Model) ask(c overlay.Confirm, yes func() tea.Cmd) {
	m.confirm, m.onYes, m.mode = c, yes, modeConfirm
}

type pal struct{}

func (pal) Dim(s string) string    { return "\x1b[2m" + s + "\x1b[22m" }
func (pal) Bold(s string) string   { return "\x1b[1m" + s + "\x1b[22m" }
func (pal) Accent(s string) string { return "\x1b[36m" + s + "\x1b[39m" }
func (pal) Err(s string) string    { return "\x1b[31m" + s + "\x1b[39m" }

// View implements tea.Model.
func (m *Model) View() string {
	p := pal{}
	if m.mode == modeHelp {
		return overlay.Help(p, "❓ mono config keys", m.keys, "any key to close",
			overlay.Section{Title: "Commands", Lines: m.commandLines()})
	}
	if m.mode == modeConfirm {
		return m.confirm.Render(p)
	}
	var b strings.Builder
	title := "mono config — " + m.Path
	if m.Dirty() {
		title += "  [modified]"
	}
	b.WriteString(p.Bold(title) + "\n\n")

	rows := []string{
		fmt.Sprintf("%-8s %s", "prefix", m.cfg.Prefix),
		fmt.Sprintf("%-8s %-5s %s", "title", m.cfg.Title.Size, "version, commit, focused pane, help hint"),
	}
	for i, pn := range m.cfg.Panes {
		every := ""
		if pn.Every != "" {
			every = "every " + pn.Every
		}
		cmd := pn.Cmd
		if len(pn.Fallbacks) > 0 {
			cmd += "  (else " + strings.Join(pn.Fallbacks, ", ") + ")"
		}
		status := p.Dim("checking…")
		if i < len(m.choices) && m.choices[i].Pane.Name == pn.Name {
			if c := m.choices[i]; c.Usable() {
				status = p.Accent("✓ " + c.Cmd)
			} else {
				status = p.Err("✗ not shown: " + c.Reason)
			}
		}
		rows = append(rows, fmt.Sprintf("%-8s %-5s %-9s %-28s %s", pn.Name, pn.Size, every, cmd, status))
	}
	top, bottom := m.cur.Visible()
	for i := top; i < bottom && i < len(rows); i++ {
		mark := "  "
		if i == m.cur.Pos {
			mark = "> "
		}
		b.WriteString(mark + rows[i] + "\n")
	}
	b.WriteString("\n" + p.Bold("Preview") + p.Dim(fmt.Sprintf("  (%d rows)", m.height)) + "\n")
	b.WriteString(m.preview())
	b.WriteString("\n")
	switch {
	case m.mode == modeCommand:
		b.WriteString(":" + m.cmd.Input.Render("█"))
	case m.msg != "" && m.isErr:
		b.WriteString(p.Err(m.msg))
	case m.msg != "":
		b.WriteString(p.Dim(m.msg))
	default:
		b.WriteString(p.Dim(m.keys.HeaderHint("  ")))
	}
	return b.String()
}

// preview draws one line per region with its row count at the current
// terminal height, using only the panes that pass the tool check.
func (m *Model) preview() string {
	title, _ := config.ParseSize(m.cfg.Title.Size)
	pcts := []int{title}
	var names []string
	for i, pn := range m.cfg.Panes {
		if len(m.choices) == len(m.cfg.Panes) && !m.choices[i].Usable() {
			continue
		}
		n, _ := config.ParseSize(pn.Size)
		pcts = append(pcts, n)
		names = append(names, pn.Name)
	}
	rows := layout.Rows(m.height, pcts)
	barW := max(min(m.width-24, 50), 10)
	var b strings.Builder
	line := func(name string, n int) {
		w := max(n*barW/max(m.height, 1), 1)
		fmt.Fprintf(&b, "  %-8s %3d rows %s\n", name, n, strings.Repeat("█", w))
	}
	line("title", rows[0])
	for i, n := range names {
		line(n, rows[i+1])
	}
	return b.String()
}

func (m *Model) commandLines() []string {
	var out []string
	for _, s := range m.reg.Specs() {
		out = append(out, fmt.Sprintf(":%-8s %s", s.Name, s.Help))
	}
	return out
}
