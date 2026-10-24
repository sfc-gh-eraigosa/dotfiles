package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/render"
)

type fakeFetcher struct {
	calls atomic.Int32
	width atomic.Int32 // the width of the last fetch
	err   error
	data  Data
	rng   time.Duration
}

func (f *fakeFetcher) Fetch(_ context.Context, width int, rng time.Duration, _ bool) (Data, error) {
	f.calls.Add(1)
	f.width.Store(int32(width))
	f.rng = rng
	if f.err != nil {
		return Data{}, f.err
	}
	return f.data, nil
}

func sample() Data {
	v := 0.42
	return Data{Title: "Sample", Panels: []PanelData{{
		Panel:  dash.Panel{Kind: dash.KindMeter, Title: "Pass rate", Unit: "percentunit"},
		Series: []grafana.Series{{Name: "x", Times: []time.Time{time.Unix(0, 0)}, Values: []float64{v}}},
	}}}
}

// instantTick yields the tick message at once; run() drops it, so a test
// never sleeps through a refresh interval.
func instantTick(time.Duration) tea.Cmd { return func() tea.Msg { return tickMsg{} } }

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newModel(f Fetcher, refresh string) (*Model, *clock) {
	c := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	m := New(Options{Fetcher: f, Range: "1h", Refresh: refresh, View: "sample", Render: render.Opts{}, Now: c.now, Tick: instantTick})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	return m, c
}

// run executes a command and feeds the messages it yields back into the model
// (a tea.BatchMsg is expanded; ticks are dropped — tests send them by hand).
func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			run(m, c)
		}
	case tickMsg, nil:
	default:
		_, next := m.Update(msg)
		run(m, next)
	}
}

func TestFirstFetchDraws(t *testing.T) {
	f := &fakeFetcher{data: sample()}
	m, _ := newModel(f, "30s")
	run(m, m.Init())
	out := m.View()
	if !strings.Contains(out, "Pass rate") || !strings.Contains(out, "42.0%") {
		t.Fatalf("frame:\n%s", out)
	}
	if !strings.Contains(out, "updated 12:00:00") || !strings.Contains(out, "every 30s") {
		t.Fatalf("status row:\n%s", out)
	}
}

// E7: a failure after a good fetch keeps the frame and says how stale it is.
func TestStaleKeepsLastGoodData(t *testing.T) {
	f := &fakeFetcher{data: sample()}
	m, c := newModel(f, "30s")
	run(m, m.Init())
	f.err = errors.New("grafana /api/ds/query: HTTP 502")
	c.t = c.t.Add(2*time.Minute + 14*time.Second)
	_, cmd := m.Update(tickMsg{})
	run(m, cmd)
	out := m.View()
	if !strings.Contains(out, "Pass rate") {
		t.Fatalf("the last good frame was dropped:\n%s", out)
	}
	if !strings.Contains(out, "stale 2m14s · grafana /api/ds/query: HTTP 502") {
		t.Fatalf("no stale row:\n%s", out)
	}
}

func TestFirstFetchFailsShowsErrorAndCountdown(t *testing.T) {
	f := &fakeFetcher{err: errors.New("dial tcp 192.0.2.128:30300: connection refused")}
	m, c := newModel(f, "30s")
	run(m, m.Init())
	c.t = c.t.Add(10 * time.Second)
	out := m.View()
	if !strings.Contains(out, "connection refused") || !strings.Contains(out, "retry in 20s") {
		t.Fatalf("frame:\n%s", out)
	}
	if strings.TrimSpace(strings.ReplaceAll(out, "\n", "")) == "" {
		t.Fatal("blank screen")
	}
}

// E8: a tick that finds a fetch still running is skipped and counted.
func TestRefreshSkipIfBusy(t *testing.T) {
	f := &fakeFetcher{data: sample()}
	m, _ := newModel(f, "30s")
	_, cmd := m.Update(tickMsg{}) // starts a fetch; do not run it yet
	if cmd == nil || !m.busy {
		t.Fatal("tick did not start a fetch")
	}
	m.Update(tickMsg{})
	m.Update(tickMsg{})
	if m.skipped != 2 {
		t.Fatalf("skipped = %d, want 2", m.skipped)
	}
	run(m, cmd)
	if f.calls.Load() != 1 || m.busy {
		t.Fatalf("calls %d busy %v", f.calls.Load(), m.busy)
	}
}

func TestRefreshFloor(t *testing.T) {
	m, _ := newModel(&fakeFetcher{}, "1s")
	if m.refresh != 5*time.Second {
		t.Fatalf("refresh = %v, want the 5s floor", m.refresh)
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// E9: keys come from the binding table: scroll, refresh, range, quit, help.
func TestKeys(t *testing.T) {
	d := sample()
	for i := 0; i < 30; i++ { // enough rows to scroll
		d.Panels = append(d.Panels, d.Panels[0])
	}
	f := &fakeFetcher{data: d}
	m, _ := newModel(f, "30s")
	run(m, m.Init())
	m.Update(key("j"))
	if m.offset != 1 {
		t.Fatalf("j: offset %d", m.offset)
	}
	m.Update(key("G"))
	bottom := m.offset
	if bottom == 0 {
		t.Fatal("G did not scroll")
	}
	m.Update(key("j"))
	if m.offset != bottom {
		t.Fatal("scrolled past the end")
	}
	m.Update(key("g"))
	m.Update(key("g"))
	if m.offset != 0 {
		t.Fatalf("gg: offset %d", m.offset)
	}
	m.Update(key("ctrl+d"))
	if m.offset == 0 {
		t.Fatal("ctrl+d did not scroll")
	}
	m.Update(key("ctrl+u"))
	if m.offset != 0 {
		t.Fatalf("ctrl+u: offset %d", m.offset)
	}
	before := f.calls.Load()
	_, cmd := m.Update(key("r"))
	run(m, cmd)
	if f.calls.Load() != before+1 {
		t.Fatal("r did not refresh")
	}
	_, cmd = m.Update(key("+"))
	run(m, cmd)
	if f.rng != 3*time.Hour {
		t.Fatalf("+ from 1h: %v", f.rng)
	}
	for i := 0; i < 10; i++ {
		_, cmd = m.Update(key("+"))
		run(m, cmd)
	}
	if m.Range() != 7*24*time.Hour {
		t.Fatalf("+ past 7d: %v", m.Range())
	}
	for i := 0; i < 20; i++ {
		m.Update(key("-"))
	}
	if m.Range() != 5*time.Minute {
		t.Fatalf("- past 5m: %v", m.Range())
	}
	m.Update(key("?"))
	if !strings.Contains(m.View(), "refresh now") {
		t.Fatalf("help overlay missing:\n%s", m.View())
	}
	m.Update(key("?"))
	if _, cmd := m.Update(key("q")); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q did not quit")
	}
}

func TestHelpRendersFromTheTable(t *testing.T) {
	for _, b := range Keys() {
		if len(b.Keys) == 0 || b.Help == "" {
			t.Fatalf("binding %s has no key or help", b.Action)
		}
	}
	if !Keys().Has("r") || !Keys().Has("+") || Keys().Has("/") {
		t.Fatal("gdash keymap: want r and +, not / (no search)")
	}
}

// A program starts before it knows its size (bubbletea sends the first
// WindowSizeMsg after Init). Fetching then asked Grafana for a 1-cell-wide
// chart — two points per series — and that degraded frame stayed up until
// the next refresh or `r`. Under mono every view switch starts gdash fresh,
// so every switch showed it. The first fetch now waits for the size.
func TestFirstFetchWaitsForTheSize(t *testing.T) {
	f := &fakeFetcher{data: sample()}
	m := New(Options{Fetcher: f, Range: "1h", Refresh: "30s", View: "sample", Now: time.Now, Tick: instantTick})
	run(m, m.Init())
	if n := f.calls.Load(); n != 0 {
		t.Fatalf("fetched %d time(s) before the size was known (width %d)", n, f.width.Load())
	}
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	run(m, cmd)
	if f.calls.Load() != 1 || f.width.Load() != 72 {
		t.Fatalf("after the size: calls %d width %d, want 1 fetch at width 72", f.calls.Load(), f.width.Load())
	}
	if !strings.Contains(m.View(), "Pass rate") {
		t.Fatalf("no frame after the first fetch:\n%s", m.View())
	}
}

// A width change redraws at the new width (mono's zoom, a terminal resize);
// a height-only change does not refetch.
func TestResizeRefetchesOnlyWhenTheWidthChanges(t *testing.T) {
	f := &fakeFetcher{data: sample()}
	m, _ := newModel(f, "30s")
	run(m, m.Init())
	if f.calls.Load() != 1 || f.width.Load() != 72 {
		t.Fatalf("init: calls %d width %d", f.calls.Load(), f.width.Load())
	}
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	run(m, cmd)
	if f.calls.Load() != 1 {
		t.Fatalf("a height-only resize refetched (calls %d)", f.calls.Load())
	}
	_, cmd = m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	run(m, cmd)
	if f.calls.Load() != 2 || f.width.Load() != 91 { // two columns: (200-1)/2 - 8
		t.Fatalf("width change: calls %d width %d, want 2 at width 91", f.calls.Load(), f.width.Load())
	}
}

// A resize while a fetch is in flight queues one more at the new width.
func TestResizeWhileBusyFetchesAgainAfter(t *testing.T) {
	f := &fakeFetcher{data: sample()}
	m, _ := newModel(f, "30s")
	cmd := m.Init() // the fetch is in flight; do not run it yet
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	if !m.pending {
		t.Fatal("a resize while busy did not queue a refetch")
	}
	run(m, cmd)
	if f.calls.Load() != 2 || f.width.Load() != 92 {
		t.Fatalf("calls %d width %d, want the queued fetch at width 92", f.calls.Load(), f.width.Load())
	}
}

// In two columns, a table too wide for its half takes the whole row (a
// half-width table cuts every cell); one that fits stays beside its
// neighbour.
func TestWideTableTakesTheRow(t *testing.T) {
	label := strings.Repeat("x", 80)
	tbl := func(title, v string) PanelData {
		return PanelData{
			Panel:  dash.Panel{Kind: dash.KindTable, Title: title, Targets: []dash.Target{{RefID: "A"}}, Table: &dash.Table{}},
			Series: []grafana.Series{{RefID: "A", Labels: map[string]string{"task": v}, Times: []time.Time{time.Unix(0, 0)}, Values: []float64{1}}},
		}
	}
	meter := PanelData{Panel: dash.Panel{Kind: dash.KindMeter, Title: "Funnel"}}
	out := Layout(Data{Panels: []PanelData{meter, tbl("Wide", label), tbl("Narrow", "t-1"), meter}}, 140, "v", render.Opts{})
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, label) {
		t.Fatalf("wide table was cut:\n%s", joined)
	}
	if strings.Contains(out[0], "Wide") {
		t.Fatalf("wide table shares the meter's row:\n%s", joined)
	}
	found := false
	for _, l := range out {
		if strings.HasPrefix(l, "Narrow") && strings.Contains(l, "Funnel") {
			found = true
		}
	}
	if !found {
		t.Fatalf("narrow table should sit beside the next panel:\n%s", joined)
	}
}
