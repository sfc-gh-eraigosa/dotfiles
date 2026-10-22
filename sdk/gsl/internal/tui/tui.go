// Package tui is `gsl config` with no arguments: a config studio over the
// status line. Rows for the style, palette, links, time settings and every
// segment (toggle with space, reorder with J/K); a live preview of the line at
// an adjustable width with a ruler; an MCP health panel; save with w. Every
// : command mirrors a `gsl config` verb. Built on sdk/libs/tui.
//
// The model is VALUE-semantic (plan E22): Update never changes the model it
// was called on — config edits copy the segment slice, and commands run
// against the local copy that Update returns.
package tui

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/cmdline"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/nav"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
)

// Env is what the studio needs from the outside world; cmd wires the real
// ones, tests fakes.
type Env struct {
	Path    string // config.json, for the title and Save
	Save    func(path string, c config.Config) error
	Line    func(c config.Config, cols int) string // the status line c renders at cols
	MCP     func() mcp.Status                      // the cached MCP status (never probes)
	Refresh func() error                           // the out-of-band probe (r)
	Styles  []string                               // style names to offer, in order
}

// Studio actions on top of keymap.Vim.
const (
	ActMoveDown keymap.Action = "move-down"
	ActMoveUp   keymap.Action = "move-up"
	ActNarrower keymap.Action = "narrower"
	ActWider    keymap.Action = "wider"
	ActRefresh  keymap.Action = "refresh"
	ActSave     keymap.Action = "save"
)

// Keys is the studio's key table: the help overlay, the footer and the
// README key table all render from it.
func Keys() keymap.Map {
	return keymap.Vim.Without(
		keymap.PageLeft, keymap.PageRight, keymap.Search, keymap.NextMatch,
		keymap.PrevMatch, keymap.ClearHighlight,
	).Merge(
		keymap.Binding{Action: keymap.Select, Keys: []string{"space"}, Help: "on a segment: turn it on/off; on style / palette / links: the next value", Short: "toggle", Header: true},
		keymap.Binding{Action: keymap.Confirm, Keys: []string{"enter"}, Help: "edit the setting on the command line (on a segment: on/off)", Short: "edit"},
		keymap.Binding{Action: ActMoveDown, Keys: []string{"J"}, Help: "move the segment right (later on the line)", Short: "reorder", Group: "reorder"},
		keymap.Binding{Action: ActMoveUp, Keys: []string{"K"}, Help: "move the segment left (earlier on the line)", Short: "reorder", Group: "reorder"},
		keymap.Binding{Action: ActNarrower, Keys: []string{"<"}, Help: "narrow the preview by 10 columns", Short: "width", Group: "width"},
		keymap.Binding{Action: ActWider, Keys: []string{">"}, Help: "widen the preview by 10 columns", Short: "width", Group: "width"},
		keymap.Binding{Action: ActRefresh, Keys: []string{"r"}, Help: "probe the MCP servers now (a few seconds)", Short: "mcp"},
		keymap.Binding{Action: ActSave, Keys: []string{"w"}, Help: "save to config.json", Short: "save", Header: true},
		keymap.Binding{Action: keymap.Command, Keys: []string{":"}, Help: "command line (:set :toggle :width :refresh :w :wq :q)", Short: "command", Header: true},
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

// Setting rows, then one row per segment.
const (
	rowStyle = iota
	rowPalette
	rowLinks
	rowTimezone
	rowTime
	rowDate
	settingRows
)

var palettes = []string{"", "dark", "light", "dark8", "dark-daltonism"}
var linkModes = []string{config.LinksUnderline, config.LinksPlain, config.LinksOff}

// The options space cycles through (and :set hints and Tab completes) for the
// free-form settings. Any IANA zone or Go layout can still be typed.
var (
	timezones = []string{"", "UTC", "America/Los_Angeles", "America/Denver", "America/Chicago",
		"America/New_York", "America/Sao_Paulo", "Europe/London", "Europe/Berlin",
		"Asia/Kolkata", "Asia/Shanghai", "Asia/Tokyo", "Australia/Sydney"}
	timeFormats = []string{"15:04", "15:04:05", "3:04 PM", "3:04:05 PM"}
	dateFormats = []string{"2006-01-02", "Mon 01-02", "Mon Jan 2", "01/02", "Jan 2"}
)

// settingKeys are the :set keys, in row order.
var settingKeys = []string{"style", "palette", "links", "timezone", "time_format", "date_format"}

// choices are the options for a :set key, with the current value kept in
// the cycle when it is a custom one. "" (auto / local) is shown as its word.
func (m Model) choices(key string) []string {
	withCurrent := func(list []string, cur string) []string {
		if slices.Contains(list, cur) {
			return list
		}
		return append(append([]string(nil), list...), cur)
	}
	switch key {
	case "style":
		return m.env.Styles
	case "palette":
		return palettes
	case "links":
		return linkModes
	case "timezone":
		return withCurrent(timezones, m.cfg.Timezone)
	case "time_format":
		return withCurrent(timeFormats, m.cfg.TimeFormat)
	case "date_format":
		return withCurrent(dateFormats, m.cfg.DateFormat)
	}
	return nil
}

// current is the value of a :set key, as cycling compares it.
func (m Model) current(key string) string {
	switch key {
	case "style":
		return m.cfg.Style
	case "palette":
		return m.cfg.Palette
	case "links":
		return orDefault(m.cfg.Links, config.LinksUnderline)
	case "timezone":
		return m.cfg.Timezone
	case "time_format":
		return m.cfg.TimeFormat
	}
	return m.cfg.DateFormat
}

// shown is how an option reads: "" is auto (palette) or local (timezone).
func shown(key, v string) string {
	if v != "" {
		return v
	}
	if key == "timezone" {
		return "local"
	}
	return "auto"
}

// Model is the studio. Use New; Update returns a changed copy.
type Model struct {
	env         Env
	cfg, saved  config.Config
	keys        keymap.Map
	cur         nav.Cursor
	mode        mode
	cmd         cmdline.State
	confirm     overlay.Confirm
	onYes       func(Model) (Model, tea.Cmd)
	msg         string
	isErr       bool
	width       int
	height      int
	previewCols int // 0 = the window width
	mcp         mcp.Status
}

type mcpMsg struct {
	s   mcp.Status
	err error
}

// New builds the studio for cfg. Every known segment gets a row — one an old
// config lacks is added, disabled — and that normalised config counts as saved.
func New(cfg config.Config, env Env) Model {
	cfg = withAllSegments(cfg)
	m := Model{env: env, cfg: cfg, saved: cfg, keys: Keys(), width: 80, height: 24}
	m.cur.SetLen(m.rowCount())
	m.cur.SetHeight(10)
	if env.MCP != nil {
		m.mcp = env.MCP()
	}
	return m
}

func withAllSegments(cfg config.Config) config.Config {
	segs := append([]config.Segment(nil), cfg.Segments...)
	for _, t := range config.KnownSegmentTypes {
		if !slices.ContainsFunc(segs, func(s config.Segment) bool { return s.Type == t }) {
			segs = append(segs, config.Segment{Type: t})
		}
	}
	cfg.Segments = segs
	return cfg
}

// Config is the working copy; Dirty reports unsaved changes.
func (m Model) Config() config.Config { return m.cfg }
func (m Model) Dirty() bool           { return !reflect.DeepEqual(m.cfg, m.saved) }

func (m Model) rowCount() int { return settingRows + len(m.cfg.Segments) }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.cur.SetHeight(max(m.height-16, 3))
		return m, nil
	case mcpMsg:
		m.mcp = msg.s
		if msg.err != nil {
			m.say("mcp refresh: "+msg.err.Error(), true)
		} else {
			m.say("MCP probed", false)
		}
		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) say(s string, isErr bool) { m.msg, m.isErr = s, isErr }

// set applies one `gsl config set`-style change; a rejected one changes nothing.
func (m *Model) set(key, value string) error {
	c := m.cfg
	switch key {
	case "style":
		if !slices.Contains(m.env.Styles, value) {
			return fmt.Errorf("style %q: not one of %s", value, strings.Join(m.env.Styles, ", "))
		}
		c.Style = value
	case "palette":
		if value == "auto" {
			value = ""
		}
		if !slices.Contains(palettes, value) {
			return fmt.Errorf("palette %q: want auto, dark, light, dark8 or dark-daltonism", value)
		}
		c.Palette = value
	case "links":
		if !slices.Contains(linkModes, value) {
			return fmt.Errorf("links %q: want underline, plain or off", value)
		}
		c.Links = value
	case "timezone":
		if value == "local" {
			value = ""
		}
		if _, err := time.LoadLocation(value); err != nil {
			return fmt.Errorf("timezone %q: not an IANA zone", value)
		}
		c.Timezone = value
	case "time_format":
		c.TimeFormat = value
	case "date_format":
		c.DateFormat = value
	default:
		return fmt.Errorf("set: unknown key %q (style, palette, links, timezone, time_format, date_format)", key)
	}
	m.cfg = c
	m.say("changed — w to save", false)
	return nil
}

// toggle flips segment typ, on a copy of the slice.
func (m *Model) toggle(typ string) error {
	i := slices.IndexFunc(m.cfg.Segments, func(s config.Segment) bool { return s.Type == typ })
	if i < 0 {
		return fmt.Errorf("toggle: no segment %q", typ)
	}
	segs := append([]config.Segment(nil), m.cfg.Segments...)
	segs[i].Enabled = !segs[i].Enabled
	m.cfg.Segments = segs
	m.say("changed — w to save", false)
	return nil
}

// move shifts segment i by d, on a copy, and keeps the cursor on it.
func (m *Model) move(i, d int) {
	j := i + d
	if i < 0 || j < 0 || j >= len(m.cfg.Segments) {
		return
	}
	segs := append([]config.Segment(nil), m.cfg.Segments...)
	segs[i], segs[j] = segs[j], segs[i]
	m.cfg.Segments = segs
	m.cur.To(settingRows + j)
	m.say("changed — w to save", false)
}

func next(list []string, cur string) string {
	i := slices.Index(list, cur)
	return list[(i+1)%len(list)]
}

func (m *Model) write() {
	if err := m.env.Save(m.env.Path, m.cfg); err != nil {
		m.say("save: "+err.Error(), true)
		return
	}
	m.saved = m.cfg
	m.say("saved "+m.env.Path, false)
}

func (m Model) refresh() tea.Cmd {
	env := m.env
	return func() tea.Msg {
		var err error
		if env.Refresh != nil {
			err = env.Refresh()
		}
		var s mcp.Status
		if env.MCP != nil {
			s = env.MCP()
		}
		return mcpMsg{s: s, err: err}
	}
}

func (m *Model) setWidth(n int) {
	m.previewCols = min(max(n, 20), 300)
}

func (m Model) cols() int {
	if m.previewCols > 0 {
		return m.previewCols
	}
	return max(m.width, 20)
}

// registry is the : command set, bound to this (local) model value.
func (m *Model) registry() *cmdline.Registry {
	var reg cmdline.Registry
	onHelp := func() { m.mode = modeHelp }
	for _, s := range cmdline.Standard(onHelp, func(string) {}) {
		switch s.Name {
		case "search":
		case "q":
			reg.Register(cmdline.Spec{Name: "q", Aliases: []string{"quit"}, Help: "quit (refuses with unsaved changes; :q! discards)",
				Run: func([]string) (tea.Cmd, error) {
					if m.Dirty() {
						return nil, fmt.Errorf("unsaved changes: w to save, :wq to save and quit, :q! to discard")
					}
					return tea.Quit, nil
				}})
		default:
			reg.Register(s)
		}
	}
	keyComplete := func(i int, prefix string) []string {
		var list []string
		switch i {
		case 0:
			list = settingKeys
		case 1: // the value: the options for the key already typed
			f := strings.Fields(m.cmd.Input.String())
			if len(f) < 2 {
				return nil
			}
			for _, v := range m.choices(f[1]) {
				list = append(list, shown(f[1], v))
			}
		}
		var out []string
		for _, k := range list {
			if strings.HasPrefix(k, prefix) {
				out = append(out, k)
			}
		}
		return out
	}
	segComplete := func(i int, prefix string) []string {
		var out []string
		for _, s := range m.cfg.Segments {
			if i == 0 && strings.HasPrefix(s.Type, prefix) {
				out = append(out, s.Type)
			}
		}
		return out
	}
	reg.Register(
		cmdline.Spec{Name: "q!", Help: "quit without saving", Run: func([]string) (tea.Cmd, error) { return tea.Quit, nil }},
		cmdline.Spec{Name: "w", Help: "save", Run: func([]string) (tea.Cmd, error) { m.write(); return nil, nil }},
		cmdline.Spec{Name: "wq", Aliases: []string{"x"}, Help: "save and quit", Run: func([]string) (tea.Cmd, error) {
			m.write()
			if m.isErr {
				return nil, nil
			}
			return tea.Quit, nil
		}},
		cmdline.Spec{Name: "set", Help: "set <key> <value>  (gsl config set)", Complete: keyComplete, Run: func(args []string) (tea.Cmd, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("usage: set <key> <value>")
			}
			return nil, m.set(args[0], strings.Join(args[1:], " "))
		}},
		cmdline.Spec{Name: "toggle", Help: "toggle <segment>  (gsl config toggle)", Complete: segComplete, Run: func(args []string) (tea.Cmd, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("usage: toggle <segment>")
			}
			return nil, m.toggle(args[0])
		}},
		cmdline.Spec{Name: "width", Help: "width <n>|auto  (the preview's columns)", Run: func(args []string) (tea.Cmd, error) {
			if len(args) == 1 && args[0] == "auto" {
				m.previewCols = 0
				return nil, nil
			}
			n, err := strconv.Atoi(strings.Join(args, ""))
			if err != nil {
				return nil, fmt.Errorf("usage: width <n>|auto")
			}
			m.setWidth(n)
			return nil, nil
		}},
		cmdline.Spec{Name: "refresh", Help: "probe the MCP servers now", Run: func([]string) (tea.Cmd, error) { return m.refresh(), nil }},
	)
	return &reg
}

func (m Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeHelp:
		m.mode = modeList
		return m, nil
	case modeConfirm:
		m.mode = modeList
		if m.confirm.Key(k) == overlay.Yes && m.onYes != nil {
			return m.onYes(m)
		}
		m.say("cancelled", false)
		return m, nil
	case modeCommand:
		reg := m.registry()
		ev := m.cmd.Key(k, reg)
		switch ev.Kind {
		case cmdline.Cancelled:
			m.mode = modeList
		case cmdline.Submitted:
			m.mode = modeList
			m.msg, m.isErr = "", false
			cmd, err := reg.Run(ev.Command)
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
	seg := m.cur.Pos - settingRows
	switch act {
	case keymap.Quit:
		if !m.Dirty() {
			return m, tea.Quit
		}
		m.confirm = overlay.Confirm{Title: "Quit without saving?", Lines: []string{"unsaved changes to " + m.env.Path}, YesLabel: "discard"}
		m.onYes = func(m Model) (Model, tea.Cmd) { return m, tea.Quit }
		m.mode = modeConfirm
	case keymap.Help:
		m.mode = modeHelp
	case keymap.Command:
		m.prompt("")
	case ActSave:
		m.write()
	case ActRefresh:
		m.say("probing MCP servers…", false)
		return m, m.refresh()
	case ActNarrower:
		m.setWidth(m.cols() - 10)
	case ActWider:
		m.setWidth(m.cols() + 10)
	case ActMoveDown, ActMoveUp:
		d := 1
		if act == ActMoveUp {
			d = -1
		}
		m.move(seg, d)
	case keymap.Select:
		var err error
		if seg >= 0 {
			err = m.toggle(m.cfg.Segments[seg].Type)
		} else {
			key := settingKeys[m.cur.Pos]
			err = m.set(key, shown(key, next(m.choices(key), m.current(key))))
		}
		if err != nil {
			m.say(err.Error(), true)
		}
	case keymap.Confirm:
		if seg >= 0 {
			if err := m.toggle(m.cfg.Segments[seg].Type); err != nil {
				m.say(err.Error(), true)
			}
		} else {
			m.prompt(m.setLine())
		}
	}
	return m, nil
}

// hints is the key table as the footer shows it for the row under the
// cursor: on a setting, space is "next" and enter "edit"; on a segment,
// space is "on/off" and J/K "reorder". Still the one key table — only which
// bindings show, and their short labels, change.
func (m Model) hints() keymap.Map {
	onSegment := m.cur.Pos >= settingRows
	out := make(keymap.Map, 0, len(m.keys))
	for _, b := range m.keys {
		switch b.Action {
		case keymap.Select:
			b.Short = "next"
			if onSegment {
				b.Short = "on/off"
			}
		case keymap.Confirm:
			b.Header = !onSegment
		case ActMoveDown, ActMoveUp:
			b.Header = onSegment
		}
		out = append(out, b)
	}
	return out
}

// setHint is the line above a ":set …" prompt: the keys, then the options
// for the key being set (Tab cycles them).
func (m Model) setHint() string {
	text := m.cmd.Input.String()
	f := strings.Fields(text)
	if len(f) == 0 || f[0] != "set" {
		return ""
	}
	if len(f) == 1 || (len(f) == 2 && !strings.HasSuffix(text, " ")) {
		return "keys: " + strings.Join(settingKeys, " · ") + "   (Tab completes)"
	}
	var opts []string
	for _, v := range m.choices(f[1]) {
		opts = append(opts, shown(f[1], v))
	}
	if len(opts) == 0 {
		return ""
	}
	// The "or anything" note leads: a long list is cut at the screen edge.
	var lead string
	switch f[1] {
	case "timezone":
		lead = "options (Tab cycles; or any IANA zone)"
	case "time_format", "date_format":
		lead = "options (Tab cycles; or any Go layout)"
	default:
		return "options: " + strings.Join(opts, " · ") + "   (Tab cycles)"
	}
	return lead + ": " + strings.Join(opts, " · ")
}

// setLine is the :set pre-filled for the setting under the cursor.
func (m Model) setLine() string {
	switch m.cur.Pos {
	case rowStyle:
		return "set style " + m.cfg.Style
	case rowPalette:
		return "set palette " + orAuto(m.cfg.Palette)
	case rowLinks:
		return "set links " + m.cfg.Links
	case rowTimezone:
		return "set timezone " + m.cfg.Timezone
	case rowTime:
		return "set time_format " + m.cfg.TimeFormat
	}
	return "set date_format " + m.cfg.DateFormat
}

func orAuto(s string) string {
	if s == "" {
		return "auto"
	}
	return s
}

func (m *Model) prompt(text string) {
	m.mode = modeCommand
	m.cmd = cmdline.State{}
	m.cmd.Input.SetText(text)
	m.msg = ""
}

// pal paints the studio's own text from the sdk's shared scheme.
var pal = theme.ChromeFor("gsl", os.Getenv)

// fit cuts a line to w columns (ANSI-aware), so no line can wrap.
func fit(s string, w int) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// View implements tea.Model.
func (m Model) View() string {
	w := max(m.width, 20)
	var lines []string
	add := func(s string) { lines = append(lines, fit(s, w)) }

	if m.mode == modeHelp {
		for _, l := range strings.Split(overlay.Help(pal, "❓ gsl config keys", m.keys, "any key to close",
			overlay.Section{Title: "Commands", Lines: m.commandLines()}), "\n") {
			add(l)
		}
		return strings.Join(lines, "\n")
	}
	if m.mode == modeConfirm {
		for _, l := range strings.Split(m.confirm.Render(pal), "\n") {
			add(l)
		}
		return strings.Join(lines, "\n")
	}

	title := "gsl config — " + m.env.Path
	if m.Dirty() {
		title += "  [modified]"
	}
	add(pal.Bold(title))
	add("")
	now := time.Now()
	if loc, err := time.LoadLocation(m.cfg.Timezone); err == nil {
		now = now.In(loc)
	}
	rows := []string{
		row("style", m.cfg.Style),
		row("palette", orAuto(m.cfg.Palette)),
		row("links", orDefault(m.cfg.Links, config.LinksUnderline)),
		row("timezone", orDefault(m.cfg.Timezone, "local")),
		row("time", m.cfg.TimeFormat) + pal.Dim("  e.g. "+now.Format(m.cfg.TimeFormat)),
		row("date", m.cfg.DateFormat) + pal.Dim("  e.g. "+now.Format(m.cfg.DateFormat)),
	}
	for _, s := range m.cfg.Segments {
		box := "[ ]"
		if s.Enabled {
			box = "[x]"
		}
		rows = append(rows, box+" "+s.Type)
	}
	top, bottom := m.cur.Visible()
	if top > 0 {
		add(pal.Dim(fmt.Sprintf("  … %d more above", top)))
	}
	for i := top; i < bottom && i < len(rows); i++ {
		if i == settingRows {
			add(pal.Dim("  segments — left → right on the line (J/K to move, space on/off)"))
		}
		mark, hint := "  ", ""
		if i == m.cur.Pos {
			mark = "> "
			hint = "   ‹space: next · enter: type›"
			if i >= settingRows {
				hint = "   ‹space: on/off · J/K: move›"
			}
		}
		add(mark + rows[i] + pal.Dim(hint))
	}
	if bottom < len(rows) {
		add(pal.Dim(fmt.Sprintf("  … %d more below", len(rows)-bottom)))
	}

	cols := m.cols()
	add("")
	add(pal.Bold("Preview") + pal.Dim(fmt.Sprintf("  (%d cols — < > to change)", cols)))
	if m.env.Line != nil {
		add(m.env.Line(m.cfg, min(cols, w)))
	}
	add(ruler(min(cols, w)))

	add("")
	add(pal.Bold("MCP") + "  " + mcpSummary(m.mcp, time.Now()))
	if names := needsLook(m.mcp); names != "" {
		add("  " + names)
	}

	add("")
	switch {
	case m.mode == modeCommand:
		if h := m.setHint(); h != "" {
			add(pal.Dim(h))
		}
		add(":" + m.cmd.Input.Render("█"))
	case m.msg != "" && m.isErr:
		add(pal.Err(m.msg))
	case m.msg != "":
		add(pal.Dim(m.msg))
	default:
		add(pal.Dim(m.hints().HeaderHint("  ")))
	}
	return strings.Join(lines, "\n")
}

func row(k, v string) string { return fmt.Sprintf("%-9s %s", k, v) }

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// ruler marks every 10th column under the preview: "····|····|" with the
// tens digit at each mark.
func ruler(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		switch {
		case i%10 == 0:
			d := strconv.Itoa(i / 10 % 10)
			b.WriteString(d)
		case i%5 == 0:
			b.WriteString("+")
		default:
			b.WriteString("·")
		}
	}
	return pal.Dim(b.String())
}

func mcpSummary(s mcp.Status, now time.Time) string {
	if s.FetchedAt.IsZero() {
		return fmt.Sprintf("%d configured, not probed yet — r to probe", s.Configured)
	}
	return fmt.Sprintf("%d✓ %d✗ %d! · %d configured · probed %s ago", s.Connected, s.Failed, s.NeedsAuth, s.Configured, now.Sub(s.FetchedAt).Round(time.Second))
}

// needsLook names the servers that failed or need authentication.
func needsLook(s mcp.Status) string {
	var failed, auth []string
	for _, sv := range s.Servers {
		switch sv.State {
		case mcp.StateFailed:
			failed = append(failed, sv.Name)
		case mcp.StateNeedsAuth:
			auth = append(auth, sv.Name)
		}
	}
	var parts []string
	if len(failed) > 0 {
		parts = append(parts, "failed: "+strings.Join(failed, ", "))
	}
	if len(auth) > 0 {
		parts = append(parts, "needs auth: "+strings.Join(auth, ", "))
	}
	return strings.Join(parts, " · ")
}

func (m Model) commandLines() []string {
	var out []string
	for _, s := range m.registry().Specs() {
		out = append(out, fmt.Sprintf(":%-8s %s", s.Name, s.Help))
	}
	return out
}

// Style names for Env.Styles: the builtins, then the config's own, sorted.
func StyleNames(cfg config.Config) []string {
	names := make([]string, 0)
	for n := range style.Builtins() {
		names = append(names, n)
	}
	for n := range cfg.Styles {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}

// KeyTable is the README's markdown key table, from the same key table as
// the help overlay and the footer (TestREADMEKeyTable pins the copy).
func KeyTable() string {
	var b strings.Builder
	b.WriteString("| Keys | Does |\n| :-- | :-- |\n")
	for _, k := range Keys() {
		keys := make([]string, len(k.Keys))
		for i, s := range k.Keys {
			keys[i] = "`" + s + "`"
		}
		b.WriteString("| " + strings.Join(keys, " ") + " | " + k.Help + " |\n")
	}
	return b.String()
}
