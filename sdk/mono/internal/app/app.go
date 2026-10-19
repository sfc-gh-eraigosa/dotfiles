// Package app is the mono dashboard: a title bar over a stack of panes, the
// prefix-key state machine, the help overlay and the : command line. The core
// (HandleBytes, Resize, Frame) is terminal-free so it can be tested; Run wires
// it to a real terminal.
package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/cmdline"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/input"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/pane"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/probe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/screen"
)

type mode int

const (
	modeNormal mode = iota
	modeHelp
	modeCommand
	modeModal
)

// Options configure an App.
type Options struct {
	Config     config.Config
	ConfigPath string                                   // for :reload and after :config
	Env        probe.Env                                // tool checks; nil = probe.System{}
	Title      string                                   // left side of the title bar, e.g. "mono v0.1.0 — a1b2c3d"
	Dir        string                                   // default working directory for panes
	ConfigArgv func() []string                          // the `mono config` command for :config; nil disables it
	OnChange   func()                                   // called when a redraw is due (from any goroutine)
	Load       func(path string) (config.Config, error) // nil = config.Load
	Now        func() time.Time                         // nil = time.Now (double-esc timing)
}

// App is the dashboard state.
type App struct {
	opts   Options
	cfg    config.Config
	keys   keymap.Map
	router input.Router

	mu         sync.Mutex
	choices    []probe.Choice
	slots      []*slot // usable panes, top to bottom (deck.go)
	focus      int
	zoom       string          // the pane zoomed by hand; "" = none (spec F10)
	noAuto     map[string]bool // a zoom: true tool's auto-zoom turned off by z
	nav        bool            // nav mode (spec F9)
	lastEsc    time.Time       // a lone esc just forwarded, for double-esc
	navPartial []byte          // an escape sequence split across reads in nav mode
	mode       mode
	cmd        cmdline.State
	reg        cmdline.Registry
	modal      *pane.Pane
	msg        string
	width      int
	height     int
	quit       bool

	applyMu  sync.Mutex     // one apply at a time
	pending  *config.Config // the newest config waiting to be applied
	stopping sync.WaitGroup // programs a view switch is stopping (Stop waits)
	closed   bool           // Stop ran: apply installs nothing new
}

// New builds the dashboard; panes start on the first Resize.
func New(o Options) (*App, error) {
	if o.Env == nil {
		o.Env = probe.System{}
	}
	if o.Load == nil {
		o.Load = config.Load
	}
	if o.OnChange == nil {
		o.OnChange = func() {}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	pb, err := config.PrefixByte(o.Config.Prefix)
	if err != nil {
		return nil, err
	}
	a := &App{opts: o, cfg: o.Config, keys: Keys(o.Config.Prefix, o.Config.Nav()), router: input.Router{Prefix: pb}, noAuto: map[string]bool{}}
	a.registerCommands()
	return a, nil
}

func (a *App) registerCommands() {
	onHelp := func() { a.mu.Lock(); a.mode = modeHelp; a.mu.Unlock() }
	for _, s := range cmdline.Standard(onHelp, func(string) {}) {
		if s.Name != "search" { // mono has nothing to search
			a.reg.Register(s)
		}
	}
	paneNames := func(_ int, prefix string) []string {
		var out []string
		for _, s := range a.slots {
			for _, path := range s.paths() {
				if strings.HasPrefix(path, prefix) {
					out = append(out, path)
				}
			}
		}
		return out
	}
	a.reg.Register(
		cmdline.Spec{Name: "config", Help: "edit panes and sizes (mono config), then reload",
			Run: func([]string) (tea.Cmd, error) { return nil, a.openConfig() }},
		cmdline.Spec{Name: "restart", Help: "restart [pane]  restart a pane's program (default: focused)", Complete: paneNames,
			Run: func(args []string) (tea.Cmd, error) { return nil, a.restart(args) }},
		cmdline.Spec{Name: "focus", Help: "focus <pane>[/<tool>[/<view>]]", Complete: paneNames,
			Run: func(args []string) (tea.Cmd, error) { return nil, a.focusNamed(args) }},
		cmdline.Spec{Name: "reload", Help: "re-read the config file and re-check tools",
			Run: func([]string) (tea.Cmd, error) { return nil, a.reload() }},
	)
}

// Choices reports what the tool check decided for every configured pane.
func (a *App) Choices() []probe.Choice {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]probe.Choice(nil), a.choices...)
}

// Quit reports whether the user asked to leave.
func (a *App) Quit() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.quit
}

// PanesNames lists the running panes, top to bottom (for tests and doctor).
func (a *App) PaneNames() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, s := range a.slots {
		out = append(out, s.name)
	}
	return out
}

// Resize sets the terminal size; the first call starts the panes.
func (a *App) Resize(width, height int) {
	a.mu.Lock()
	first := a.width == 0 && a.height == 0
	a.width, a.height = max(width, 1), max(height, 1)
	a.mu.Unlock()
	if first {
		a.applyMu.Lock()
		a.apply(a.cfg)
		a.applyMu.Unlock()
		return
	}
	a.mu.Lock()
	starts := a.relayoutLocked()
	if a.modal != nil {
		a.modal.Resize(a.width, a.height)
	}
	a.mu.Unlock()
	startAll(starts)
	a.opts.OnChange()
}

// apply reconciles the running panes with cfg: unchanged panes keep running,
// changed or removed ones stop, new ones start. A deck keeps the tool and
// view it was showing when they are still usable. Callers hold applyMu, so
// two applies never interleave (a pane resized before it started, or an
// older config landing last).
func (a *App) apply(cfg config.Config) {
	choices := probe.Choose(context.Background(), a.opts.Env, cfg.Effective())
	a.mu.Lock()
	if a.closed { // mono is quitting: a late reload starts nothing
		a.mu.Unlock()
		return
	}
	old := map[string]*slot{}
	for _, s := range a.slots {
		old[s.name] = s
	}
	var slots []*slot
	for _, c := range choices {
		if !c.Usable() {
			continue
		}
		s := newSlot(c)
		if o, ok := old[s.name]; ok {
			s.keepPlace(o)
		}
		spec := a.specFor(s)
		if o, ok := old[s.name]; ok && o.p != nil && o.p.Spec == spec {
			s.p = o.p
			delete(old, s.name)
		} else {
			s.p = pane.New(spec, a.opts.OnChange)
			s.fresh = true
		}
		slots = append(slots, s)
	}
	focused := ""
	if a.focus < len(a.slots) {
		focused = a.slots[a.focus].name
	}
	a.cfg, a.choices, a.slots = cfg, choices, slots
	a.focus = 0
	for i, s := range slots {
		if s.name == focused {
			a.focus = i
		}
	}
	if a.zoom != "" && a.slotIndexLocked(a.zoom) < 0 {
		a.zoom = ""
	}
	if pb, err := config.PrefixByte(cfg.Prefix); err == nil {
		a.router.Prefix = pb
		a.keys = Keys(cfg.Prefix, cfg.Nav())
	}
	starts := a.relayoutLocked()
	a.mu.Unlock()

	var wg sync.WaitGroup
	for _, s := range old {
		if s.p == nil {
			continue
		}
		wg.Add(1)
		go func(p *pane.Pane) { defer wg.Done(); p.Stop() }(s.p)
	}
	startAll(starts)
	wg.Wait()
	a.opts.OnChange()
}

// Stop ends every pane (in parallel; each gets KillGrace).
func (a *App) Stop() {
	a.applyMu.Lock() // no apply in flight; none after (closed)
	defer a.applyMu.Unlock()
	a.mu.Lock()
	a.closed = true
	var ps []*pane.Pane
	for _, s := range a.slots {
		if s.p != nil {
			ps = append(ps, s.p)
		}
	}
	if a.modal != nil {
		ps = append(ps, a.modal)
	}
	a.mu.Unlock()
	var wg sync.WaitGroup
	for _, p := range ps {
		wg.Add(1)
		go func(p *pane.Pane) { defer wg.Done(); p.Stop() }(p)
	}
	wg.Wait()
	a.stopping.Wait() // and every program a view switch was stopping
}

// HandleBytes routes one read from the keyboard.
func (a *App) HandleBytes(b []byte) {
	a.mu.Lock()
	m := a.mode
	modal := a.modal
	a.mu.Unlock()
	switch m {
	case modeModal:
		if modal != nil {
			modal.Write(b)
		}
		return
	case modeHelp:
		a.mu.Lock()
		a.mode = modeNormal // any key closes help; that key is not passed on
		a.mu.Unlock()
		if _, n := input.Decode(b); n < len(b) {
			a.HandleBytes(b[n:])
			return
		}
		a.opts.OnChange()
		return
	case modeCommand:
		for len(b) > 0 {
			k, n := input.Decode(b)
			b = b[n:]
			a.commandKey(k)
			a.mu.Lock()
			still := a.mode == modeCommand
			a.mu.Unlock()
			if !still {
				a.HandleBytes(b)
				return
			}
		}
		a.opts.OnChange()
		return
	}
	a.mu.Lock()
	nav := a.nav
	a.mu.Unlock()
	if nav {
		a.navBytes(b)
		return
	}
	a.mu.Lock()
	events, rest := a.router.Feed(b)
	a.mu.Unlock()
	for i, ev := range events {
		if ev.Key == nil {
			fwd, entered := a.doubleEsc(ev.Forward)
			if len(fwd) > 0 {
				a.focused(func(p *pane.Pane) { p.Write(fwd) })
			}
			if entered {
				var more []byte
				for _, e := range events[i+1:] {
					more = append(more, e.Forward...)
				}
				a.navBytes(append(more, rest...))
				return
			}
			continue
		}
		a.prefixKey(*ev.Key)
	}
	if len(rest) > 0 {
		a.HandleBytes(rest) // in whatever mode that key left us
		return
	}
	a.opts.OnChange()
}

func (a *App) focused(fn func(*pane.Pane)) {
	a.mu.Lock()
	var p *pane.Pane
	if a.focus < len(a.slots) {
		p = a.slots[a.focus].p
	}
	a.mu.Unlock()
	if p != nil {
		fn(p)
	}
}

func (a *App) prefixKey(k tea.KeyMsg) {
	a.mu.Lock()
	act, ok := a.keys.Lookup(k)
	a.msg = ""
	if !ok {
		a.msg = fmt.Sprintf("%s %s is not a mono key — %s ? for help", a.cfg.Prefix, keymap.KeyName(k), a.cfg.Prefix)
		a.mu.Unlock()
		return
	}
	var starts []launch
	switch act {
	case keymap.Down:
		a.cyclePaneLocked(1)
	case keymap.Up:
		a.cyclePaneLocked(-1)
	case keymap.PageLeft:
		starts = a.stepToolLocked(-1)
	case keymap.PageRight:
		starts = a.stepToolLocked(1)
	case ActZoom:
		starts = a.toggleZoomLocked()
	case ActNav:
		a.nav = true
	case keymap.Help:
		a.mode = modeHelp
	case keymap.Command:
		a.mode = modeCommand
		a.cmd = cmdline.State{}
	case keymap.Quit:
		a.quit = true
	case ActRestart:
		if a.focus < len(a.slots) {
			s := a.slots[a.focus]
			p := s.p
			go func() { _ = p.Restart() }()
			if s.deck {
				a.mu.Unlock()
				_ = a.reload() // a deck also re-checks its tools
				return
			}
		}
	case ActSendPrefix:
		if a.focus < len(a.slots) {
			a.slots[a.focus].p.Write([]byte{a.router.Prefix})
		}
	case keymap.Back:
	}
	a.mu.Unlock()
	startAll(starts)
}

func (a *App) commandKey(k tea.KeyMsg) {
	a.mu.Lock()
	ev := a.cmd.Key(k, &a.reg)
	switch ev.Kind {
	case cmdline.Cancelled:
		a.mode = modeNormal
		a.mu.Unlock()
		return
	case cmdline.Submitted:
		a.mode = modeNormal
		a.mu.Unlock()
		cmd, err := a.reg.Run(ev.Command)
		a.mu.Lock()
		if err != nil {
			a.msg = err.Error()
		} else if cmd != nil {
			if _, isQuit := cmd().(tea.QuitMsg); isQuit {
				a.quit = true
			}
		}
	}
	a.mu.Unlock()
}

func (a *App) restart(args []string) error {
	a.mu.Lock()
	var target *pane.Pane
	if len(args) == 0 {
		if a.focus < len(a.slots) {
			target = a.slots[a.focus].p
		}
	} else {
		for _, s := range a.slots {
			if s.name == args[0] {
				target = s.p
			}
		}
	}
	a.mu.Unlock()
	if target == nil {
		return fmt.Errorf("no running pane named %q", strings.Join(args, " "))
	}
	go func() { _ = target.Restart() }()
	return nil
}

// focusNamed is :focus <pane>[/<tool>[/<view>]] (spec E19).
func (a *App) focusNamed(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: focus <pane>[/<tool>[/<view>]]")
	}
	a.mu.Lock()
	starts, err := a.focusPathLocked(args[0])
	a.mu.Unlock()
	startAll(starts)
	a.opts.OnChange()
	return err
}

func (a *App) reload() error {
	cfg, err := a.opts.Load(a.opts.ConfigPath)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.pending = &cfg
	a.msg = "reloaded " + a.opts.ConfigPath
	a.mu.Unlock()
	go func() {
		a.applyMu.Lock()
		defer a.applyMu.Unlock()
		a.mu.Lock()
		next := a.pending
		a.pending = nil
		a.mu.Unlock()
		if next != nil { // nil: a later reload already applied the newest
			a.apply(*next)
		}
	}()
	return nil
}

// openConfig runs `mono config` full screen; when it exits mono reloads.
func (a *App) openConfig() error {
	if a.opts.ConfigArgv == nil {
		return fmt.Errorf("config editor unavailable; run `mono config` in a shell")
	}
	argv := a.opts.ConfigArgv()
	if len(argv) == 0 {
		return fmt.Errorf("config editor unavailable; run `mono config` in a shell")
	}
	quoted := make([]string, len(argv))
	for i, s := range argv {
		quoted[i] = "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	a.mu.Lock()
	w, h := a.width, a.height
	a.mu.Unlock()
	var m *pane.Pane
	m = pane.New(pane.Spec{Name: "config", Cmd: strings.Join(quoted, " "), Dir: a.opts.Dir}, func() {
		if exited, _ := m.Exited(); exited {
			go a.closeModal(m)
		}
		a.opts.OnChange()
	})
	a.mu.Lock()
	a.modal, a.mode = m, modeModal
	a.mu.Unlock()
	return m.Start(w, h)
}

func (a *App) closeModal(m *pane.Pane) {
	a.mu.Lock()
	if a.modal != m {
		a.mu.Unlock()
		return
	}
	a.modal, a.mode = nil, modeNormal
	a.mu.Unlock()
	m.Stop()
	if err := a.reload(); err != nil {
		a.mu.Lock()
		a.msg = err.Error()
		a.mu.Unlock()
	}
	a.opts.OnChange()
}

// Frame composes the screen: exactly height rows of exactly width columns.
// It reads mono's state under a.mu, then snapshots the panes without it, so a
// pane whose emulator is busy never stalls key handling.
func (a *App) Frame() ([]string, screen.Cursor) {
	a.mu.Lock()
	w, h := a.width, a.height
	if a.mode == modeModal && a.modal != nil {
		m := a.modal
		a.mu.Unlock()
		v := m.Snapshot(w, h)
		return v.Rows, v.Cursor
	}
	heights := a.heightsLocked()
	var panes []*pane.Pane
	for _, s := range a.slots {
		panes = append(panes, s.p)
	}
	md, focus, prefix, msg := a.mode, a.focus, a.cfg.Prefix, a.msg
	cmdText := a.cmd.Input.String()
	bar := a.titleBarLocked(w)
	var help []string
	if md == modeHelp {
		help = a.helpBodyLocked()
	}
	a.mu.Unlock()

	rows := make([]string, 0, h)
	var cur screen.Cursor
	// Title region: the bar, then the message line, then blanks.
	for i := 0; i < heights[0]; i++ {
		switch {
		case i == 0 && md == modeCommand:
			line := ":" + cmdText
			rows = append(rows, pad(line, w))
			cur = screen.Cursor{X: min(utf8.RuneCountInString(line), w-1), Y: 0, Show: true}
		case i == 0:
			rows = append(rows, bar)
		case i == 1: // with a one-row title the message shows on the bar instead
			rows = append(rows, pad(msg, w))
		default:
			rows = append(rows, pad("", w))
		}
	}

	top := heights[0]
	for i, p := range panes {
		if i+1 >= len(heights) {
			break
		}
		ph := heights[i+1]
		if ph == 0 || p == nil { // hidden by zoom: keeps running, not drawn
			continue
		}
		v := p.Snapshot(w, ph)
		if v.Exited && ph > 0 {
			v.Rows[ph-1] = "\x1b[0;7m" + pad(fmt.Sprintf(" %s %s — %s r restart ", p.Spec.Name, v.ExitReason, prefix), w) + "\x1b[0m"
		}
		rows = append(rows, v.Rows...)
		if i == focus && md == modeNormal && p.Interactive() && !v.Exited && v.Cursor.Show {
			cur = screen.Cursor{X: v.Cursor.X, Y: top + v.Cursor.Y, Show: true}
		}
		top += ph
	}
	for len(rows) < h {
		rows = append(rows, pad("", w))
	}
	rows = rows[:h]

	if md == modeHelp {
		drawHelp(rows, help, w, h)
		cur = screen.Cursor{}
	}
	return rows, cur
}

func (a *App) titleBarLocked(w int) string {
	left := " " + a.opts.Title
	if a.focus < len(a.slots) {
		left += "   ● " + a.slots[a.focus].label(os.Getenv("FLEET_ASCII_BORDERS") == "1")
	}
	if a.nav {
		left += "   NAV"
	}
	if a.zoomIndexLocked() >= 0 {
		left += "   ZOOM"
	}
	right := a.cfg.Prefix + " ? help "
	style := "\x1b[0;7m"
	if a.router.Pending() {
		// The hint is what matters while the prefix waits: it takes the bar.
		return "\x1b[0;1;7m" + pad(" "+a.cfg.Prefix+": "+a.keys.HeaderHint("  "), w) + "\x1b[0m"
	} else if a.msg != "" {
		right = a.msg + " "
	}
	gap := w - width(left) - width(right)
	if gap < 1 {
		return style + pad(truncate(left+" "+right, w), w) + "\x1b[0m"
	}
	return style + left + strings.Repeat(" ", gap) + right + "\x1b[0m"
}

// palette paints mono's own text (help overlay) from the sdk's shared
// scheme (sdk/libs/tui/theme), so it matches gdash, fleet and gsl.
type palette struct{}

func scheme() theme.Palette {
	p, _ := theme.Get(theme.Resolve("mono", os.Getenv, theme.EnvDarkBackground(os.Getenv)))
	return p
}

func (palette) Dim(s string) string    { return "\x1b[2m" + s + "\x1b[22m" }
func (palette) Bold(s string) string   { return "\x1b[1m" + s + "\x1b[22m" }
func (palette) Accent(s string) string { return theme.Paint(s, scheme().Accent) }
func (palette) Err(s string) string    { return theme.Paint(s, scheme().Crit) }

// HelpText is the help overlay body (shared with `mono --help`).
func HelpText(keys keymap.Map, prefix string, panes []string, p overlay.Palette) string {
	cmds := []string{
		":config            edit panes, tools and sizes, then reload",
		":restart [pane]    restart a pane's program",
		":focus <pane>      focus a pane by name",
		":reload            re-read the config and re-check tools",
		":q                 quit",
	}
	sections := []overlay.Section{{Title: "Commands", Lines: cmds}}
	if len(panes) > 0 {
		sections = append(sections, overlay.Section{Title: "Panes", Lines: panes})
	}
	return overlay.Help(p, "❓ keys — press "+prefix+" first; every other key goes to the focused program", keys, "any key to close", sections...)
}

func (a *App) helpBodyLocked() []string {
	var panes []string
	for _, c := range a.choices {
		if c.Usable() {
			panes = append(panes, fmt.Sprintf("%-8s %s", c.Pane.Name, c.Cmd))
		} else {
			panes = append(panes, fmt.Sprintf("%-8s (not shown: %s)", c.Pane.Name, c.Reason))
		}
	}
	return strings.Split(HelpText(a.keys, a.cfg.Prefix, panes, palette{}), "\n")
}

// drawHelp boxes body in the middle of rows. Below 4 columns there is no room
// for a box, and the panes stay visible instead.
func drawHelp(rows, body []string, w, h int) {
	if w < 4 || h < 3 {
		return
	}
	inner := 0
	for _, l := range body {
		inner = max(inner, width(l))
	}
	inner = min(inner+2, w-2)
	if len(body)+2 > h {
		keep := max(h-3, 1)
		hidden := len(body) - keep
		body = append(body[:keep:keep], fmt.Sprintf("… %d more — make the window taller to see them", hidden))
	}
	box := []string{"┌" + strings.Repeat("─", inner) + "┐"}
	for _, l := range body {
		box = append(box, "│ "+pad(truncate(l, inner-1), inner-1)+"│")
	}
	box = append(box, "└"+strings.Repeat("─", inner)+"┘")
	top := max((h-len(box))/2, 0)
	left := max((w-inner-2)/2, 0)
	for i, l := range box {
		if top+i >= h {
			break
		}
		rows[top+i] = "\x1b[0m" + strings.Repeat(" ", left) + l + "\x1b[0m" + strings.Repeat(" ", max(w-left-inner-2, 0))
	}
}

// width is the display width of s in terminal columns, ignoring ANSI
// escapes (wide runes such as emoji count two), matching screen.Row.
func width(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e || j <= i+1) {
				j++
			}
			i = j + 1
			continue
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		n += runewidth.RuneWidth(r)
		i += sz
	}
	return n
}

func pad(s string, w int) string {
	if d := w - width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return truncate(s, w)
}

func truncate(s string, w int) string {
	if width(s) <= w {
		return s
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e || j <= i+1) {
				j++
			}
			b.WriteString(s[i:min(j+1, len(s))])
			i = j + 1
			continue
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		cw := width(s[i : i+sz])
		if n+cw > w {
			break
		}
		b.WriteString(s[i : i+sz])
		n += cw
		i += sz
	}
	return b.String() + strings.Repeat(" ", max(w-n, 0))
}
