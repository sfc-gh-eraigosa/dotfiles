package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/input"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/layout"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/pane"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/probe"
)

// A slot is one pane on screen: a plain pane, or a deck of tools × views of
// which exactly one view's program runs (spec F8). Hidden by zoom, a slot
// keeps running but gets no rows (spec F10).

type toolState struct {
	name  string
	zoom  bool
	views []probe.ViewChoice
}

type slot struct {
	name     string
	deck     bool
	zoomable bool
	pct      int
	tools    []toolState
	tool     int // the tool on screen
	view     int // its view on screen
	p        *pane.Pane
	fresh    bool // p is new and has not been started
}

func newSlot(c probe.Choice) *slot {
	pct, _ := config.ParseSize(c.Pane.Size)
	s := &slot{name: c.Pane.Name, deck: c.Pane.IsDeck(), zoomable: c.Pane.CanZoom(), pct: pct}
	for _, v := range c.Views {
		if n := len(s.tools); n == 0 || s.tools[n-1].name != v.Tool {
			s.tools = append(s.tools, toolState{name: v.Tool, zoom: v.Zoom})
		}
		s.tools[len(s.tools)-1].views = append(s.tools[len(s.tools)-1].views, v)
	}
	return s
}

// keepPlace carries the tool and view on screen over from the old slot when
// they are still usable.
func (s *slot) keepPlace(o *slot) {
	if o.tool >= len(o.tools) {
		return
	}
	tn, vn := o.tools[o.tool].name, o.tools[o.tool].views[o.view].View
	for ti, t := range s.tools {
		if t.name != tn {
			continue
		}
		for vi, v := range t.views {
			if v.View == vn {
				s.tool, s.view = ti, vi
			}
		}
	}
}

func (s *slot) current() probe.ViewChoice { return s.tools[s.tool].views[s.view] }

// path is pane, or pane/tool/view for a deck.
func (s *slot) path() string {
	if !s.deck {
		return s.name
	}
	v := s.current()
	return s.name + "/" + v.Tool + "/" + v.View
}

// paths lists every name :focus accepts for this slot.
func (s *slot) paths() []string {
	out := []string{s.name}
	if !s.deck {
		return out
	}
	for _, t := range s.tools {
		out = append(out, s.name+"/"+t.name)
		for _, v := range t.views {
			out = append(out, s.name+"/"+t.name+"/"+v.View)
		}
	}
	return out
}

// label is the title bar's "where you are": mid ◀ gdash ▶ · ci 2/4, with
// arrows only toward another tool (spec F8).
func (s *slot) label(ascii bool) string {
	if !s.deck {
		return s.name
	}
	l, r := "◀ ", " ▶"
	if ascii {
		l, r = "< ", " >"
	}
	if s.tool == 0 {
		l = ""
	}
	if s.tool == len(s.tools)-1 {
		r = ""
	}
	t := s.tools[s.tool]
	out := s.name + " " + l + t.name + r + " · " + t.views[s.view].View
	if len(t.views) > 1 {
		out += " " + strconv.Itoa(s.view+1) + "/" + strconv.Itoa(len(t.views))
	}
	return out
}

func (a *App) specFor(s *slot) pane.Spec {
	v := s.current()
	every, _ := time.ParseDuration(v.Every)
	dir := v.Cwd
	if dir == "" {
		dir = a.opts.Dir
	}
	return pane.Spec{Name: s.path(), Cmd: v.Cmd, Every: every, Dir: dir}
}

// launch is a pane to start, outside a.mu.
type launch struct {
	p    *pane.Pane
	w, h int
}

func startAll(ss []launch) {
	for _, s := range ss {
		_ = s.p.Start(s.w, s.h)
	}
}

// heightsLocked is [title, slot...] rows for the current size and zoom: the
// zoomed slot takes every middle row, the other middle slots none; the title
// and non-zoomable slots keep theirs (spec F10).
func (a *App) heightsLocked() []int {
	rows := a.splitHeightsLocked()
	z := a.zoomIndexLocked()
	if z < 0 {
		return rows
	}
	total := 0
	for i, s := range a.slots {
		if s.zoomable {
			total += rows[i+1]
			rows[i+1] = 0
		}
	}
	rows[z+1] = total
	return rows
}

func (a *App) splitHeightsLocked() []int {
	title, _ := config.ParseSize(a.cfg.Title.Size)
	pcts := []int{title}
	for _, s := range a.slots {
		pcts = append(pcts, s.pct)
	}
	return layout.Rows(a.height, pcts)
}

// zoomIndexLocked is the zoomed slot: the one zoomed by hand, else the first
// whose tool on screen is zoom: true (and not turned off with z); -1 = none.
func (a *App) zoomIndexLocked() int {
	if a.zoom != "" {
		if i := a.slotIndexLocked(a.zoom); i >= 0 && a.slots[i].zoomable {
			return i
		}
	}
	for i, s := range a.slots {
		if s.zoomable && s.tools[s.tool].zoom && !a.noAuto[s.name] {
			return i
		}
	}
	return -1
}

func (a *App) slotIndexLocked(name string) int {
	for i, s := range a.slots {
		if s.name == name {
			return i
		}
	}
	return -1
}

// relayoutLocked sizes every visible slot, returns the new panes to start
// (a hidden new pane starts at its split height), and keeps focus on a
// visible slot.
func (a *App) relayoutLocked() []launch {
	h := a.heightsLocked()
	split := a.splitHeightsLocked()
	var starts []launch
	for i, s := range a.slots {
		rows := h[i+1]
		if s.fresh {
			if rows == 0 {
				rows = max(split[i+1], 1)
			}
			starts = append(starts, launch{p: s.p, w: a.width, h: rows})
			s.fresh = false
			continue
		}
		if rows > 0 {
			s.p.Resize(a.width, rows)
		}
	}
	if a.focus < len(a.slots) && h[a.focus+1] == 0 {
		if z := a.zoomIndexLocked(); z >= 0 {
			a.focus = z
		}
	}
	return starts
}

func (a *App) visibleLocked() []bool {
	h := a.heightsLocked()
	out := make([]bool, len(a.slots))
	for i := range a.slots {
		out[i] = h[i+1] > 0
	}
	return out
}

// navigableLocked is visibleLocked minus the fixed panes (zoomable: false,
// the gsl footer): j/k never stop on them; :focus still can.
func (a *App) navigableLocked() []bool {
	out := a.visibleLocked()
	for i, s := range a.slots {
		out[i] = out[i] && s.zoomable
	}
	return out
}

// cyclePaneLocked is prefix j/k: the next or previous navigable pane,
// wrapping as before decks.
func (a *App) cyclePaneLocked(d int) {
	n := len(a.slots)
	vis := a.navigableLocked()
	for step := 1; step <= n; step++ {
		i := ((a.focus+d*step)%n + n) % n
		if vis[i] {
			a.focus = i
			return
		}
	}
}

// switchLocked puts tool t, view v of slot i on screen: the old program stops
// (HUP, then KILL after pane.KillGrace) and the new one starts fresh.
func (a *App) switchLocked(i, t, v int) []launch {
	s := a.slots[i]
	if s.tool == t && s.view == v {
		return nil
	}
	if s.tool != t {
		delete(a.noAuto, s.name)
	}
	old := s.p
	s.tool, s.view = t, v
	s.p = pane.New(a.specFor(s), a.opts.OnChange)
	s.fresh = true
	a.stopping.Add(1)
	go func() { defer a.stopping.Done(); old.Stop() }()
	return a.relayoutLocked()
}

func (a *App) stepToolLocked(d int) []launch {
	if a.focus >= len(a.slots) {
		return nil
	}
	s := a.slots[a.focus]
	t := s.tool + d
	if !s.deck || t < 0 || t >= len(s.tools) {
		return nil // the ends do not wrap
	}
	return a.switchLocked(a.focus, t, 0)
}

// navMoveLocked: the next view of the deck, past its last view the next
// navigable pane; the last pane stays (spec E21).
func (a *App) navMoveLocked(d int) []launch {
	if a.focus >= len(a.slots) {
		return nil
	}
	s := a.slots[a.focus]
	if s.deck {
		if v := s.view + d; v >= 0 && v < len(s.tools[s.tool].views) {
			return a.switchLocked(a.focus, s.tool, v)
		}
	}
	vis := a.navigableLocked()
	for i := a.focus + d; i >= 0 && i < len(a.slots); i += d {
		if vis[i] {
			a.focus = i
			return nil
		}
	}
	return nil
}

// toggleZoomLocked is z: zoom the focused pane over the middle, or give the
// rows back. A zoom: true tool's own zoom is turned off (and on) the same way.
func (a *App) toggleZoomLocked() []launch {
	if a.focus >= len(a.slots) {
		return nil
	}
	s := a.slots[a.focus]
	if !s.zoomable {
		a.msg = s.name + " is not part of the middle (zoomable: false)"
		return nil
	}
	auto := s.tools[s.tool].zoom
	switch {
	case a.zoom == s.name:
		a.zoom = ""
		if auto {
			a.noAuto[s.name] = true
		}
	case a.zoomIndexLocked() == a.focus: // auto-zoomed
		a.noAuto[s.name] = true
	default:
		a.zoom = s.name
		delete(a.noAuto, s.name)
	}
	return a.relayoutLocked()
}

// focusPathLocked focuses pane[/tool[/view]]; a bad name lists valid ones.
func (a *App) focusPathLocked(path string) ([]launch, error) {
	parts := strings.Split(path, "/")
	i := a.slotIndexLocked(parts[0])
	if i < 0 {
		var names []string
		for _, s := range a.slots {
			names = append(names, s.name)
		}
		return nil, fmt.Errorf("no running pane named %q (panes: %s)", parts[0], strings.Join(names, ", "))
	}
	s := a.slots[i]
	if len(parts) > 1 && !s.deck {
		return nil, fmt.Errorf("pane %q runs one program, not a deck", s.name)
	}
	if len(parts) > 3 {
		return nil, fmt.Errorf("want <pane>[/<tool>[/<view>]], got %q", path)
	}
	t, v := s.tool, s.view
	if len(parts) > 1 {
		t = -1
		var names []string
		for ti, tl := range s.tools {
			names = append(names, tl.name)
			if tl.name == parts[1] {
				t = ti
			}
		}
		if t < 0 {
			return nil, fmt.Errorf("pane %q has no usable tool %q (tools: %s)", s.name, parts[1], strings.Join(names, ", "))
		}
		v = 0
	}
	if len(parts) > 2 {
		v = -1
		var names []string
		for vi, vc := range s.tools[t].views {
			names = append(names, vc.View)
			if vc.View == parts[2] {
				v = vi
			}
		}
		if v < 0 {
			return nil, fmt.Errorf("tool %q has no usable view %q (views: %s)", s.tools[t].name, parts[2], strings.Join(names, ", "))
		}
	}
	a.focus = i
	if a.heightsLocked()[i+1] == 0 { // hidden by zoom: show it, don't focus the invisible
		if z := a.zoomIndexLocked(); z >= 0 {
			if a.zoom == a.slots[z].name {
				a.zoom = ""
			} else {
				a.noAuto[a.slots[z].name] = true
			}
		}
	}
	starts := a.switchLocked(i, t, v)
	return append(starts, a.relayoutLocked()...), nil
}

// doubleEsc watches what is about to be forwarded: every esc is forwarded
// (mono never eats a key outside the prefix), and a second within
// nav_esc_ms also enters nav mode (spec F9). It returns what to forward and
// whether nav mode began.
func (a *App) doubleEsc(b []byte) ([]byte, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	window := time.Duration(a.cfg.EscMs()) * time.Millisecond
	if window == 0 {
		return b, false
	}
	now := a.opts.Now()
	switch string(b) {
	case "\x1b":
		if !a.lastEsc.IsZero() && now.Sub(a.lastEsc) <= window {
			a.lastEsc, a.nav = time.Time{}, true
			return b, true
		}
		a.lastEsc = now
		return b, false
	case "\x1b\x1b": // both escapes in one read
		a.lastEsc, a.nav = time.Time{}, true
		return b, true
	}
	a.lastEsc = time.Time{}
	return b, false
}

// navBytes handles keys in nav mode: hjkl and z move without the prefix and
// esc leaves. Any other key leaves and is handled as if nav mode were off —
// typed text reaches the program and the prefix starts a command — so
// nothing typed is lost.
func (a *App) navBytes(b []byte) {
	nav := NavKeys()
	a.mu.Lock()
	if len(a.navPartial) > 0 { // the rest of an escape sequence split across reads
		b = append(a.navPartial, b...)
		a.navPartial = nil
	}
	a.mu.Unlock()
	for len(b) > 0 {
		if input.IncompleteEscape(b) {
			a.mu.Lock()
			a.navPartial = append([]byte(nil), b...)
			a.mu.Unlock()
			return
		}
		k, n := input.Decode(b)
		raw := b[:n]
		b = b[n:]
		a.mu.Lock()
		act, ok := nav.Lookup(k)
		var starts []launch
		pass := false
		switch {
		case !ok:
			a.nav, pass = false, true
		case act == keymap.Back:
			a.nav = false
		case act == keymap.Down:
			starts = a.navMoveLocked(1)
		case act == keymap.Up:
			starts = a.navMoveLocked(-1)
		case act == keymap.PageLeft:
			starts = a.stepToolLocked(-1)
		case act == keymap.PageRight:
			starts = a.stepToolLocked(1)
		case act == ActZoom:
			starts = a.toggleZoomLocked()
		}
		still := a.nav
		a.mu.Unlock()
		startAll(starts)
		if !still {
			if pass {
				b = append(append([]byte(nil), raw...), b...)
			}
			if len(b) > 0 {
				a.HandleBytes(b)
				return
			}
			break
		}
	}
	a.opts.OnChange()
}

// Running lists what runs in each slot: pane, or pane/tool/view (tests, doctor).
func (a *App) Running() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, s := range a.slots {
		out = append(out, s.path())
	}
	return out
}

// Heights is [title, pane...] rows as drawn now.
func (a *App) Heights() []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.heightsLocked()
}

// Nav reports whether nav mode is on.
func (a *App) Nav() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.nav
}

// FocusedName is the focused pane's name.
func (a *App) FocusedName() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.focus < len(a.slots) {
		return a.slots[a.focus].name
	}
	return ""
}

// FocusedPath is the focused pane, or pane/tool/view for a deck.
func (a *App) FocusedPath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.focus < len(a.slots) {
		return a.slots[a.focus].path()
	}
	return ""
}
