package cmd

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// layoutModel is a settled five-host fleet at a fixed terminal size, the shape
// an operator actually watches an update from.
func layoutModel(h, w int) tuiModel {
	m := newTUIModel(hosts("host-desktop", "host-nano", "host-pi", "host-edge", "host-lab"),
		nil, fakeBaseline{head: "72392c9"}, testNow, "main", 2, updplan.Default())
	m.vp = viewport{height: h, width: w}
	for _, r := range []Row{
		{Alias: "host-desktop", Class: "up-to-date", Commit: "72392c9", Age: testNow.Add(-2 * time.Hour), Branch: "main", InstalledBranch: "main"},
		{Alias: "host-nano", Class: "behind", Behind: 24, Commit: "9484943", Age: testNow.Add(-16 * 24 * time.Hour), Branch: "main", InstalledBranch: "main"},
		{Alias: "host-pi", Class: "unreachable"},
		{Alias: "host-edge", Class: "unknown", Note: "corrupt stamp"},
		{Alias: "host-lab", Class: "divergent", Commit: "abc1234", Age: testNow.Add(-3 * time.Hour), Branch: "feature/gff", InstalledBranch: "main"},
	} {
		m.setRow(r)
		delete(m.pending, r.Alias)
	}
	m.resort()
	return m
}

// TestViewNeverExceedsTerminalHeight is the guard for the scrolled-header bug.
//
// bubbletea's standard renderer drops lines from the TOP of the frame when the
// view is taller than the terminal ("we can't navigate the cursor into the
// terminal's scrollback buffer"), so one line of overflow silently eats the
// banner. Every state below must therefore fit in vp.height EXACTLY.
func TestViewNeverExceedsTerminalHeight(t *testing.T) {
	long := strings.Repeat("x", 300)
	states := []struct {
		name string
		mut  func(m *tuiModel)
	}{
		{"idle, log pane closed", func(m *tuiModel) { m.logOpen = false }},
		{"log pane open but empty", func(m *tuiModel) { m.logOpen = true }},
		{"update streaming, short lines", func(m *tuiModel) {
			m.logOpen = true
			m.streams = map[string]stream{"host-nano": {}}
			for i := 0; i < 60; i++ {
				m.appendLog("host-nano", fmt.Sprintf("Installing package %d...", i))
			}
		}},
		{"update streaming from five hosts at once", func(m *tuiModel) {
			m.logOpen = true
			m.streams = map[string]stream{"host-nano": {}, "host-pi": {},
				"host-edge": {}, "host-lab": {}, "host-desktop": {}}
			for i := 0; i < 60; i++ {
				m.appendLog("host-nano", fmt.Sprintf("Installing package %d...", i))
			}
		}},
		{"update streaming with line-wrapping output", func(m *tuiModel) {
			m.logOpen = true
			m.streams = map[string]stream{"host-nano": {}, "host-pi": {}}
			for i := 0; i < 60; i++ {
				m.appendLog("host-nano", long)
				m.appendLog("host-pi", long)
			}
		}},
		{"confirm gate over a running update", func(m *tuiModel) {
			m.logOpen = true
			m.mode = modeConfirm
			m.ans = answers{sudoSecret: "xxxx", windows: "s", gemini: "keep", reset: "y"}
			m.selected = map[string]bool{"host-nano": true, "host-pi": true}
			for i := 0; i < 60; i++ {
				m.appendLog("host-nano", long)
			}
		}},
		{"answers form over a running update", func(m *tuiModel) {
			m.logOpen = true
			m.mode = modeAnswers
			for i := 0; i < 60; i++ {
				m.appendLog("host-nano", long)
			}
		}},
		{"long-lined update, log focused and scrolled", func(m *tuiModel) {
			m.logOpen = true
			m.logFocus = true
			m.logFollow = false
			m.logTop = 5
			for i := 0; i < 60; i++ {
				m.appendLog("host-nano", long)
			}
		}},
	}
	sizes := [][2]int{{24, 80}, {26, 100}, {40, 100}, {50, 120}, {14, 60}}

	for _, s := range states {
		for _, sz := range sizes {
			m := layoutModel(sz[0], sz[1])
			s.mut(&m)
			out := m.View()
			if got := lipgloss.Height(out); got > m.vp.height {
				t.Errorf("%s at %dx%d: frame is %d lines for a %d-line terminal "+
					"(%d would be scrolled off the top)",
					s.name, sz[1], sz[0], got, m.vp.height, got-m.vp.height)
			}
			for _, line := range strings.Split(out, "\n") {
				if wdt := lipgloss.Width(line); wdt > m.vp.width {
					t.Errorf("%s at %dx%d: line too wide (%d): %q",
						s.name, sz[1], sz[0], wdt, stripANSI(line))
				}
			}
		}
	}
}

// TestViewFitsAcrossEveryTerminalSize sweeps the sizes an operator can actually
// drag a window to, in the state that produced the report: several hosts
// streaming at once, with output long enough to wrap. Every frame must fit.
func TestViewFitsAcrossEveryTerminalSize(t *testing.T) {
	// Run under a REAL colour profile, not the Ascii one init() pins. Under
	// Ascii every style is a no-op, so the frame carries no escape bytes at
	// all — which is how an ANSI-accounting bug in trunc reached main under a
	// 1219-size sweep that was measuring styleless output.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	for h := 8; h <= 60; h++ {
		for w := 40; w <= 200; w += 7 {
			m := layoutModel(h, w)
			m.logOpen = true
			m.streams = map[string]stream{"host-nano": {}, "host-pi": {}, "host-edge": {}}
			for i := 0; i < 40; i++ {
				m.appendLog("host-nano", strings.Repeat("x", 250))
				m.appendLog("host-pi", fmt.Sprintf("Installing package %d...", i))
			}
			out := m.View()
			if got := lipgloss.Height(out); got > h {
				t.Fatalf("%dx%d: frame is %d lines, %d would be scrolled off the top",
					w, h, got, got-h)
			}
			for _, line := range strings.Split(out, "\n") {
				if wd := lipgloss.Width(line); wd > w {
					t.Fatalf("%dx%d: line too wide (%d): %q", w, h, wd, stripANSI(line))
				}
			}
		}
	}
}

// A style must never be applied to already-rendered text. lipgloss re-styles
// character by character, so nesting strands the inner escape bytes: the log
// pane printed a literal "[38;5;33mhost-nano[0m" in its streaming legend, and
// those orphaned codes counted as visible cells — which is how a wave across
// several hosts pushed the title onto a second row and grew the frame.
func TestLogTitleNeverNestsStyles(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := layoutModel(40, 100)
	m.logOpen = true
	m.streams = map[string]stream{"host-nano": {}, "host-pi": {}, "host-edge": {}}
	m.appendLog("host-nano", "Installing 28 core packages via apt...")

	// stripANSI removes well-formed SGR runs; anything colour-code-shaped left
	// behind was printed as literal text by a nested Render.
	if got := stripANSI(m.logView()); strings.Contains(got, "38;5;") {
		t.Fatalf("nested style leaked escape codes as text:\n%q", got)
	}
	if !strings.Contains(stripANSI(m.logView()), "streaming: host-edge, host-nano, host-pi") {
		t.Fatalf("the streaming legend must name every live host:\n%s", stripANSI(m.logView()))
	}
}
