package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

var (
	runes = func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
	space = tea.KeyMsg{Type: tea.KeySpace}
	enter = tea.KeyMsg{Type: tea.KeyEnter}
	esc   = tea.KeyMsg{Type: tea.KeyEscape}
)

// fakeEnv records saves and renders a line that names what it was given.
type fakeEnv struct {
	saved     []config.Config
	saveErr   error
	refreshed int
}

func (f *fakeEnv) env() Env {
	return Env{
		Path: "/tmp/gsl/config.json",
		Save: func(_ string, c config.Config) error {
			if f.saveErr != nil {
				return f.saveErr
			}
			f.saved = append(f.saved, c)
			return nil
		},
		Line: func(c config.Config, cols int) string {
			var on []string
			for _, s := range c.Segments {
				if s.Enabled {
					on = append(on, s.Type)
				}
			}
			line := fmt.Sprintf("[%s|%s|%s] %s", c.Style, c.Palette, c.Links, strings.Join(on, ","))
			if term.DisplayWidth(line) > cols {
				line = line[:cols]
			}
			return line
		},
		MCP: func() mcp.Status {
			return mcp.Status{Configured: 12, Connected: 6, Failed: 5, NeedsAuth: 1, FetchedAt: time.Now().Add(-2 * time.Minute),
				Servers: []mcp.ServerStatus{{Name: "alpha", State: mcp.StateFailed}, {Name: "beta", State: mcp.StateNeedsAuth}}}
		},
		Refresh: func() error { f.refreshed++; return nil },
		Styles:  []string{"emoji", "powerline"},
	}
}

func newModel(t *testing.T) (Model, *fakeEnv) {
	t.Helper()
	f := &fakeEnv{}
	m := New(config.Default(), f.env())
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	return m, f
}

// step applies one message and runs any synchronous command it returns,
// feeding the result back (as the bubbletea runtime would), once.
func step(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		if out := cmd(); out != nil {
			if _, quit := out.(tea.QuitMsg); quit {
				return m, cmd
			}
			if b, ok := out.(tea.BatchMsg); ok {
				for _, c := range b {
					if c != nil {
						if o := c(); o != nil {
							m, _ = step(m, o)
						}
					}
				}
				return m, nil
			}
			m, _ = step(m, out)
		}
	}
	return m, cmd
}

func keys(m Model, ks ...tea.Msg) Model {
	for _, k := range ks {
		m, _ = step(m, k)
	}
	return m
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// E22: Update is value-semantic — the caller's model never sees a change.
func TestModel_Update_IsValueSemantic(t *testing.T) {
	m, _ := newModel(t)
	before := m.Config()
	segs := append([]config.Segment(nil), before.Segments...)
	for _, k := range []tea.Msg{runes("G"), space, runes("J"), runes("K"), runes("gg"), space, runes(">"), runes("<")} {
		next, _ := m.Update(k)
		_ = next
		if m.Config().Style != before.Style || m.Config().Palette != before.Palette {
			t.Fatalf("%v changed the caller's config", k)
		}
		for i, s := range m.Config().Segments {
			if s.Type != segs[i].Type || s.Enabled != segs[i].Enabled {
				t.Fatalf("%v changed the caller's segment %d: %+v", k, i, s)
			}
		}
	}
}

// Every known segment is listed, even one an old config lacks, without making
// the config dirty.
func TestNew_ListsEveryKnownSegment(t *testing.T) {
	cfg := config.Default()
	cfg.Segments = cfg.Segments[:2]
	m := New(cfg, (&fakeEnv{}).env())
	if len(m.Config().Segments) != len(config.KnownSegmentTypes) || m.Dirty() {
		t.Fatalf("segments %d dirty %v", len(m.Config().Segments), m.Dirty())
	}
}

// space cycles style / palette / links and toggles a segment; the preview
// follows at once.
func TestKeys_SpaceCyclesAndToggles(t *testing.T) {
	m, _ := newModel(t)
	m = keys(m, runes("j")) // past the width row, onto style
	style0 := m.Config().Style
	m = keys(m, space) // row 0: style
	if m.Config().Style == style0 || !m.Dirty() {
		t.Fatalf("style did not cycle: %s", m.Config().Style)
	}
	m = keys(m, runes("j"), space) // palette: auto → dark
	if m.Config().Palette != "dark" {
		t.Fatalf("palette = %q", m.Config().Palette)
	}
	m = keys(m, runes("j"), space) // links: → next
	if m.Config().Links == "" || m.Config().Links == config.LinksUnderline {
		t.Fatalf("links = %q", m.Config().Links)
	}
	m = keys(m, runes("G"), space) // last segment off
	last := m.Config().Segments[len(m.Config().Segments)-1]
	if last.Enabled {
		t.Fatalf("last segment still on: %+v", last)
	}
	if strings.Contains(m.View(), ","+last.Type) || strings.HasSuffix(strings.Split(m.View(), "\n")[0], last.Type) {
		t.Errorf("preview still shows %s", last.Type)
	}
}

// J/K reorder segments (only on segment rows), and the cursor follows.
func TestKeys_Reorder(t *testing.T) {
	m, _ := newModel(t)
	m = keys(m, runes("j")) // past the width row, onto style
	m = keys(m, runes("K")) // on the style row: nothing
	if m.Dirty() {
		t.Fatal("K on a setting row changed something")
	}
	first := m.Config().Segments[0].Type
	m = keys(m, runes("j"), runes("j"), runes("j"), runes("j"), runes("j"), runes("j"), runes("J")) // first segment row
	if m.Config().Segments[1].Type != first {
		t.Fatalf("J: %+v", m.Config().Segments)
	}
	m = keys(m, runes("K"))
	if m.Config().Segments[0].Type != first {
		t.Fatalf("K back: %+v", m.Config().Segments)
	}
}

// enter on a setting opens the command line pre-filled with its :set; the
// command runs on enter and is validated.
func TestKeys_EnterEditsAndValidates(t *testing.T) {
	m, _ := newModel(t)
	m = keys(m, runes("j"))                                // past the width row, onto style
	m = keys(m, runes("j"), runes("j"), runes("j"), enter) // timezone
	if !strings.Contains(m.View(), ":set timezone ") {
		t.Fatalf("no prompt:\n%s", m.View())
	}
	m = keys(m, tea.KeyMsg{Type: tea.KeyCtrlU}, runes("set timezone Mars/Base"), enter)
	if m.Config().Timezone == "Mars/Base" || !strings.Contains(m.View(), "Mars/Base") {
		t.Fatalf("a bad timezone was accepted or not reported:\n%s", m.View())
	}
	m = keys(m, runes(":"), runes("set timezone UTC"), enter)
	if m.Config().Timezone != "UTC" {
		t.Fatalf("timezone = %q", m.Config().Timezone)
	}
	m = keys(m, runes(":"), runes("set style neon"), enter)
	if m.Config().Style == "neon" {
		t.Fatal("an unknown style was accepted")
	}
}

// Mode owns the key: typing inside the prompt quits or toggles nothing.
func TestPromptOwnsKeys(t *testing.T) {
	m, _ := newModel(t)
	m = keys(m, runes(":"))
	var cmd tea.Cmd
	for _, r := range "qjk wJ" {
		m, cmd = step(m, runes(string(r)))
		if quits(cmd) {
			t.Fatal("q inside the prompt quit")
		}
	}
	if m.Dirty() {
		t.Fatal("keys inside the prompt changed the config")
	}
	m = keys(m, esc)
	if strings.Contains(m.View(), ":qjk") {
		t.Fatal("esc did not cancel the prompt")
	}
}

// w saves; the saved config is what the next render reads.
func TestKeys_SaveRoundTrip(t *testing.T) {
	m, f := newModel(t)
	m = keys(m, runes("j")) // past the width row, onto style
	m = keys(m, runes("j"), space, runes("w"))
	if len(f.saved) != 1 || f.saved[0].Palette != "dark" || m.Dirty() {
		t.Fatalf("saved %+v dirty %v", f.saved, m.Dirty())
	}
	f.saveErr = errors.New("disk full")
	m = keys(m, space, runes("w"))
	if !m.Dirty() || !strings.Contains(m.View(), "disk full") {
		t.Fatalf("a failed save must say so and stay dirty:\n%s", m.View())
	}
}

// q quits at once when clean; with unsaved changes it asks, and n keeps you.
func TestKeys_QuitAsksWhenDirty(t *testing.T) {
	m, _ := newModel(t)
	m = keys(m, runes("j")) // past the width row, onto style
	if _, cmd := step(m, runes("q")); !quits(cmd) {
		t.Fatal("clean q did not quit")
	}
	m = keys(m, space)
	m, cmd := step(m, runes("q"))
	if quits(cmd) || !strings.Contains(m.View(), "unsaved") {
		t.Fatalf("dirty q must ask:\n%s", m.View())
	}
	m, cmd = step(m, runes("n"))
	if quits(cmd) || !m.Dirty() {
		t.Fatal("n must keep the changes and stay")
	}
	m, _ = step(m, runes("q"))
	if _, cmd = step(m, runes("y")); !quits(cmd) {
		t.Fatal("y must quit")
	}
	if _, cmd = step(keys(m, runes(":")), runes("q!")); quits(cmd) {
		t.Fatal("not submitted yet")
	}
}

// < and > change the preview width, shown on the ruler; r refreshes MCP.
func TestKeys_WidthAndRefresh(t *testing.T) {
	m, f := newModel(t)
	m = keys(m, runes("<"), runes("<"))
	if !strings.Contains(m.View(), "80 cols") {
		t.Fatalf("width after << :\n%s", m.View())
	}
	m = keys(m, runes(":"), runes("width 40"), enter)
	if !strings.Contains(m.View(), "40 cols") {
		t.Fatalf(":width 40:\n%s", m.View())
	}
	m = keys(m, runes("r"))
	if f.refreshed != 1 || !strings.Contains(m.View(), "6✓ 5✗ 1!") {
		t.Fatalf("refresh %d:\n%s", f.refreshed, m.View())
	}
	if !strings.Contains(m.View(), "alpha") || !strings.Contains(m.View(), "beta") {
		t.Errorf("MCP panel should name the servers that need a look:\n%s", m.View())
	}
}

// No line of the screen exceeds the terminal width (40/80/200 columns), in
// every mode.
func TestView_FitsWidth(t *testing.T) {
	for _, w := range []int{40, 80, 200} {
		m := New(config.Default(), (&fakeEnv{}).env())
		m, _ = step(m, tea.WindowSizeMsg{Width: w, Height: 30})
		for _, extra := range [][]tea.Msg{nil, {runes("?")}, {runes(":"), runes(strings.Repeat("x", 300))}, {space, runes("q")}} {
			v := keys(m, extra...).View()
			for i, line := range strings.Split(v, "\n") {
				if dw := term.DisplayWidth(line); dw > w {
					t.Errorf("width %d, keys %v: line %d is %d wide: %q", w, extra, i, dw, line)
				}
			}
		}
	}
}

// The help overlay, footer and README render from the one key table.
func TestKeyTablesAgree(t *testing.T) {
	m, _ := newModel(t)
	help := keys(m, runes("?")).View()
	for _, b := range Keys() {
		if !strings.Contains(help, b.Help) {
			t.Errorf("help overlay lacks %q", b.Help)
		}
	}
	if !strings.Contains(m.View(), m.hints().HeaderHint("  ")) {
		t.Errorf("footer is not the key table's hint")
	}
}

// The README's studio key table is KeyTable(), verbatim.
func TestREADMEKeyTable(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), KeyTable()) {
		t.Errorf("README key table is out of date; paste:\n%s", KeyTable())
	}
}

// The footer follows the row: on a setting, space picks the next value and
// enter edits; on a segment, space turns it on/off and J/K reorder it. A
// heading says the segment rows are the line's left-to-right order.
func TestFooterFollowsTheRow(t *testing.T) {
	m, _ := newModel(t)
	m = keys(m, runes("j")) // palette
	foot := lastLine(m.View())
	if !strings.Contains(foot, "space next") || !strings.Contains(foot, "enter edit") || strings.Contains(foot, "J/K") {
		t.Errorf("setting row footer: %q", foot)
	}
	m = keys(m, runes("G")) // a segment
	foot = lastLine(m.View())
	if !strings.Contains(foot, "space on/off") || !strings.Contains(foot, "J/K reorder") {
		t.Errorf("segment row footer: %q", foot)
	}
	if !strings.Contains(m.View(), "left → right on the line") {
		t.Errorf("no segment-order heading:\n%s", m.View())
	}
	help := keys(m, runes("?")).View()
	for _, want := range []string{"move the segment right (later on the line)", "move the segment left (earlier on the line)"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}

func lastLine(v string) string {
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
	return lines[len(lines)-1]
}

var tab = tea.KeyMsg{Type: tea.KeyTab}

// space cycles every setting — timezone and the time/date formats too —
// through its known options; a custom current value stays in the cycle.
func TestSpaceCyclesEverySetting(t *testing.T) {
	m, _ := newModel(t)
	m = keys(m, runes("j"))                         // past the width row, onto style
	m = keys(m, runes("j"), runes("j"), runes("j")) // timezone
	tz0 := m.Config().Timezone
	m = keys(m, space)
	if m.Config().Timezone == tz0 {
		t.Fatalf("space did not change the timezone (%q)", tz0)
	}
	seen := map[string]bool{}
	for i := 0; i < 40 && !seen[m.Config().Timezone]; i++ {
		seen[m.Config().Timezone] = true
		m = keys(m, space)
	}
	if !seen[tz0] || !seen[""] || !seen["UTC"] {
		t.Errorf("the timezone cycle should hold the original %q, local and UTC: %v", tz0, seen)
	}
	m = keys(m, runes("j")) // time format
	f0 := m.Config().TimeFormat
	if m = keys(m, space); m.Config().TimeFormat == f0 {
		t.Error("space did not change the time format")
	}
	m = keys(m, runes("j")) // date format
	d0 := m.Config().DateFormat
	if m = keys(m, space); m.Config().DateFormat == d0 {
		t.Error("space did not change the date format")
	}
}

// The selected row says what space and enter do there, and a format row
// shows what the format looks like now.
func TestRowHints(t *testing.T) {
	m, _ := newModel(t)
	if v := m.View(); !strings.Contains(v, "space: next") || !strings.Contains(v, "enter: type") {
		t.Errorf("setting row hint missing:\n%s", v)
	}
	m = keys(m, runes("G"))
	if v := m.View(); !strings.Contains(v, "space: on/off") || !strings.Contains(v, "J/K: move") {
		t.Errorf("segment row hint missing:\n%s", v)
	}
	if v := m.View(); !strings.Contains(v, "e.g. ") {
		t.Errorf("format rows should show an example:\n%s", v)
	}
}

// :set shows the keys, then the options for the key being set, and Tab
// completes them.
func TestSetHintsAndCompletion(t *testing.T) {
	m, _ := newModel(t)
	m = keys(m, runes(":"), runes("set "))
	if v := m.View(); !strings.Contains(v, "keys: style · palette · links · timezone · time_format · date_format") {
		t.Errorf("no key hint:\n%s", v)
	}
	m = keys(m, runes("palette "))
	if v := m.View(); !strings.Contains(v, "options: auto · dark · light · dark8 · dark-daltonism") {
		t.Errorf("no options hint:\n%s", v)
	}
	m = keys(m, runes("li"), tab, enter)
	if m.Config().Palette != "light" {
		t.Fatalf("Tab did not complete li → light: %q", m.Config().Palette)
	}
	m = keys(m, runes(":"), runes("set timezone "))
	if v := m.View(); !strings.Contains(v, "UTC") || !strings.Contains(v, "any IANA zone") {
		t.Errorf("timezone options hint:\n%s", v)
	}
}

// The width row comes first: space cycles the preview width (auto, 60 … 200)
// and the ruler follows; it changes the view, not the config.
func TestWidthRow(t *testing.T) {
	m, _ := newModel(t)
	if v := m.View(); !strings.Contains(v, "> width") || !strings.Contains(v, "auto (100)") {
		t.Fatalf("width row:\n%s", v)
	}
	m = keys(m, space)
	if !strings.Contains(m.View(), "60 cols") || m.Dirty() {
		t.Fatalf("space → 60, not saved:\n%s", m.View())
	}
	for i := 0; i < 6; i++ {
		m = keys(m, space)
	}
	if !strings.Contains(m.View(), "auto (100)") {
		t.Fatalf("the cycle wraps back to auto:\n%s", m.View())
	}
	m = keys(m, enter)
	if !strings.Contains(m.View(), ":width 100") {
		t.Fatalf("enter types :width:\n%s", m.View())
	}
}
