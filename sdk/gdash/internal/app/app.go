// Package app is gdash's terminal UI: it lays a fetched dashboard out in one
// or two columns, scrolls it, refreshes it on a timer (skipping a tick while
// a fetch is still running), and keeps the last good frame when a fetch
// fails — stale, never blank (spec F4, F5).
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/render"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/overlay"
)

// The refresh floor and the per-fetch timeout.
const (
	MinRefresh   = 5 * time.Second
	FetchTimeout = 30 * time.Second
	twoColumnsAt = 120
)

// Ranges are the steps + and - move through (spec F5).
var Ranges = []time.Duration{
	5 * time.Minute, 15 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour,
	12 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour,
}

// gdash's own actions, added to the shared table.
const (
	actRefresh  keymap.Action = "refresh"
	actWider    keymap.Action = "range-wider"
	actNarrower keymap.Action = "range-narrower"
)

// Keys is gdash's key table: refresh and range first (they are gdash's own,
// and short screens clip the help overlay), then the shared vim map without
// the actions gdash has no use for. Help renders from it.
func Keys() keymap.Map {
	own := keymap.Map{
		{Action: actRefresh, Keys: []string{"r"}, Help: "refresh now", Short: "refresh", Header: true},
		{Action: actWider, Keys: []string{"+", "="}, Help: "wider time range", Short: "range", Group: "range", Header: true},
		{Action: actNarrower, Keys: []string{"-"}, Help: "narrower time range", Short: "range", Group: "range", Header: true},
	}
	return own.Merge(keymap.Vim.Without(keymap.PageLeft, keymap.PageRight, keymap.Search, keymap.NextMatch,
		keymap.PrevMatch, keymap.ClearHighlight, keymap.Command, keymap.Select, keymap.Confirm)...)
}

// Options configure a Model.
type Options struct {
	Fetcher Fetcher
	View    string // the view's name, for the status row
	Range   string // "1h"; empty = the dashboard's own (Fetch with rng 0)
	Refresh string // "30s"
	Render  render.Opts
	Now     func() time.Time
	// Tick schedules the next refresh; tests swap in one that does not sleep.
	Tick func(d time.Duration) tea.Cmd
}

type tickMsg struct{}

type dataMsg struct {
	data Data
	err  error
}

// Model is the bubbletea model.
type Model struct {
	opt     Options
	keys    keymap.Map
	refresh time.Duration
	rangeAt int
	auto    bool // no range chosen yet: the first answer's Range is taken

	w, h    int
	offset  int
	help    bool
	pendG   bool
	busy    bool
	pending bool // a range or width change arrived while busy: fetch again after
	inited  bool // Init ran: the refresh clock is going
	fetchW  int  // the chart width the last fetch asked for; 0 = none yet
	skipped int

	data      *Data
	gotAt     time.Time
	err       error
	attempted time.Time
}

// New builds a Model. A refresh under MinRefresh is raised to it.
func New(o Options) *Model {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Tick == nil {
		o.Tick = func(d time.Duration) tea.Cmd {
			return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
		}
	}
	m := &Model{opt: o, keys: Keys(), rangeAt: 2}
	m.refresh = 30 * time.Second
	if d, err := config.ParseDuration(o.Refresh); err == nil {
		m.refresh = d
	}
	if m.refresh < MinRefresh {
		m.refresh = MinRefresh
	}
	m.auto = o.Range == ""
	if m.auto {
		o.Range = config.DefaultRange
	}
	if d, err := config.ParseDuration(o.Range); err == nil {
		m.rangeAt = nearest(d)
	}
	return m
}

func nearest(d time.Duration) int {
	best := 0
	for i, r := range Ranges {
		if absDur(r-d) < absDur(Ranges[best]-d) {
			best = i
		}
	}
	return best
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// Range is the current time range.
func (m *Model) Range() time.Duration { return Ranges[m.rangeAt] }

// Init starts the refresh clock, and fetches at once when the size is already
// known. A program usually starts before its first WindowSizeMsg; fetching
// then would ask for a 1-cell-wide chart (two points per series) and leave
// that degraded frame up until the next refresh, so the first fetch waits for
// the size (Update).
func (m *Model) Init() tea.Cmd {
	m.inited = true
	if m.w == 0 {
		return m.tick()
	}
	return tea.Batch(m.fetch(false), m.tick())
}

// dataWidth is the chart width a fetch asks Grafana for.
func (m *Model) dataWidth() int { return max(m.colWidth()-8, 1) }

func (m *Model) tick() tea.Cmd {
	return m.opt.Tick(m.refresh)
}

func (m *Model) colWidth() int {
	if m.w >= twoColumnsAt {
		return (m.w - 1) / 2
	}
	return m.w
}

func (m *Model) fetch(reload bool) tea.Cmd {
	m.busy = true
	m.attempted = m.opt.Now()
	f, width, rng := m.opt.Fetcher, m.dataWidth(), m.Range()
	if m.auto {
		rng = 0
	}
	m.fetchW = width
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), FetchTimeout)
		defer cancel()
		d, err := f.Fetch(ctx, width, rng, reload)
		return dataMsg{data: d, err: err}
	}
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clamp()
		// The first size, or a new width: the data on screen was fetched for
		// another width (or not at all), so fetch again at this one.
		if m.inited && m.w > 0 && m.dataWidth() != m.fetchW {
			if m.busy {
				m.pending = true
				return m, nil
			}
			return m, m.fetch(false)
		}
	case tickMsg:
		if m.busy {
			m.skipped++
			return m, m.tick()
		}
		return m, tea.Batch(m.fetch(false), m.tick())
	case dataMsg:
		if m.auto && msg.err == nil {
			m.auto = false
			if msg.data.Range > 0 {
				m.rangeAt = nearest(msg.data.Range)
			}
		}
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			d := msg.data
			m.data, m.gotAt, m.err = &d, m.opt.Now(), nil
			m.clamp()
		}
		if m.pending {
			m.pending = false
			return m, m.fetch(false)
		}
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.help {
		if s := msg.String(); s == "?" || s == "esc" || s == "q" || s == "f1" {
			m.help = false
		}
		return m, nil
	}
	if msg.String() == "g" {
		if m.pendG {
			m.pendG, m.offset = false, 0
		} else {
			m.pendG = true
		}
		return m, nil
	}
	m.pendG = false
	a, ok := m.keys.Lookup(msg)
	if !ok {
		return m, nil
	}
	page := max(m.h-1, 1)
	switch a {
	case keymap.Quit:
		return m, tea.Quit
	case keymap.Help:
		m.help = true
	case keymap.Down:
		m.offset++
	case keymap.Up:
		m.offset--
	case keymap.HalfDown:
		m.offset += page / 2
	case keymap.HalfUp:
		m.offset -= page / 2
	case keymap.PageDown:
		m.offset += page
	case keymap.PageUp:
		m.offset -= page
	case keymap.Bottom:
		m.offset = 1 << 30
	case actRefresh:
		if !m.busy {
			return m, m.fetch(true)
		}
	case actWider, actNarrower:
		before := m.rangeAt
		if a == actWider && m.rangeAt < len(Ranges)-1 {
			m.rangeAt++
		}
		if a == actNarrower && m.rangeAt > 0 {
			m.rangeAt--
		}
		if m.rangeAt != before {
			m.auto = false
			if m.busy {
				m.pending = true
				return m, nil
			}
			return m, m.fetch(false)
		}
	}
	m.clamp()
	return m, nil
}

func (m *Model) clamp() {
	maxOff := max(len(m.body())-(m.h-1), 0)
	m.offset = min(max(m.offset, 0), maxOff)
}

// body is every line of the dashboard at the current width.
func (m *Model) body() []string {
	if m.data == nil || m.w <= 0 {
		return nil
	}
	return Layout(*m.data, m.w, m.opt.View, m.opt.Render)
}

// View implements tea.Model: exactly h lines, the last one the status row.
func (m *Model) View() string {
	if m.h <= 0 || m.w <= 0 {
		return ""
	}
	if m.help {
		return clip(overlay.Help(overlay.Plain{}, "gdash", m.keys, "? or esc closes"), m.w, m.h)
	}
	rows := m.h - 1
	body := m.body()
	var out []string
	switch {
	case m.data == nil && m.err == nil:
		out = []string{fit("loading " + m.opt.View + "…")}
	case m.data == nil:
		out = []string{}
	default:
		end := min(m.offset+rows, len(body))
		out = append(out, body[m.offset:end]...)
	}
	for len(out) < rows {
		out = append(out, "")
	}
	for i := range out {
		out[i] = trunc(out[i], m.w)
	}
	status := render.Status(m.statusText(), m.w, m.opt.Render)
	return strings.Join(append(out, status...), "\n")
}

func fit(s string) string { return s }

func trunc(s string, w int) string {
	if runewidth.StringWidth(render.StripANSI(s)) <= w {
		return s
	}
	return runewidth.Truncate(render.StripANSI(s), w, "")
}

func clip(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = trunc(lines[i], w)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) statusText() string {
	now := m.opt.Now()
	switch {
	case m.data == nil && m.err != nil:
		left := m.refresh - now.Sub(m.attempted)
		if left < 0 {
			left = 0
		}
		return fmt.Sprintf("error: %v · retry in %s", m.err, left.Round(time.Second))
	case m.err != nil:
		return fmt.Sprintf("stale %s · %v", now.Sub(m.gotAt).Round(time.Second), m.err)
	case m.data == nil:
		return "loading…"
	}
	s := fmt.Sprintf("%s · last %s · updated %s · every %s · ? help",
		m.data.Title, formatRange(m.Range()), m.gotAt.Format("15:04:05"), formatRange(m.refresh))
	if m.skipped > 0 {
		s += fmt.Sprintf(" · %d skipped", m.skipped)
	}
	return s
}

func formatRange(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return fmt.Sprintf("%ds", d/time.Second)
}

// Layout draws every panel: rows full width, other panels in one column, or
// two side by side at twoColumnsAt cells and wider (spec F3). A table too
// wide for its column takes the whole row instead of cutting every cell.
func Layout(d Data, w int, view string, o render.Opts) []string {
	colW, two := w, w >= twoColumnsAt
	if two {
		colW = (w - 1) / 2
	}
	var out, left []string
	flush := func() {
		if left != nil {
			out = append(out, left...)
			left = nil
		}
	}
	for _, pd := range d.Panels {
		if pd.Panel.Kind == dash.KindRow {
			flush()
			out = append(out, render.Row(pd.Panel, w, o)...)
			continue
		}
		if two && pd.Panel.Kind == dash.KindTable && render.TableWidth(pd.Panel, pd.Series) > colW {
			flush()
			out = append(out, drawPanel(pd, w, view, o)...)
			continue
		}
		block := drawPanel(pd, colW, view, o)
		if !two {
			out = append(out, block...)
			continue
		}
		if left == nil {
			left = block
			continue
		}
		out = append(out, sideBySide(left, block, colW)...)
		left = nil
	}
	flush()
	return out
}

func drawPanel(pd PanelData, w int, view string, o render.Opts) []string {
	p := pd.Panel
	switch {
	case p.Kind == dash.KindUnsupported:
		return render.Placeholder(p, p.Reason, w)
	case pd.Reason != "":
		return render.Placeholder(p, pd.Reason, w)
	case len(pd.Missing) > 0:
		name := strings.SplitN(pd.Missing[0], ":", 2)[0]
		return render.Placeholder(p, fmt.Sprintf("needs var $%s (gdash config set views.%s.vars.%s <value>)", name, view, name), w)
	case pd.Err != "" && len(pd.Series) == 0:
		return render.Placeholder(p, "error: "+pd.Err, w)
	case p.Kind == dash.KindChart:
		return render.Chart(p, pd.Series, w, render.ChartHeight, o)
	case p.Kind == dash.KindTable:
		return render.TableView(p, pd.Series, w, o)
	}
	return render.Meter(p, pd.Series, w, o)
}

func sideBySide(a, b []string, colW int) []string {
	n := max(len(a), len(b))
	out := make([]string, n)
	for i := 0; i < n; i++ {
		var l, r string
		if i < len(a) {
			l = a[i]
		}
		if i < len(b) {
			r = b[i]
		}
		pad := colW - runewidth.StringWidth(render.StripANSI(l))
		if pad < 0 {
			pad = 0
		}
		out[i] = l + strings.Repeat(" ", pad) + " " + r
	}
	return out
}
