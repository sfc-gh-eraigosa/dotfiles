package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// sgrRE matches one SGR (Select Graphic Rendition) escape — the only escape
// class the TUI emits.
var sgrRE = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// openStyleAtEOL reports whether a line finishes with a style still open. That
// is the defect: lipgloss's wrap-and-pad pass carries an unterminated SGR onto
// the NEXT line, so a truncated host colour repaints the row below it.
func openStyleAtEOL(line string) bool {
	open := false
	for _, m := range sgrRE.FindAllStringSubmatch(line, -1) {
		// "\x1b[0m" and the shorthand "\x1b[m" both reset to default.
		if m[1] == "" || m[1] == "0" {
			open = false
			continue
		}
		open = true
	}
	return open
}

func ansiModel(h, w int) tuiModel {
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

// trunc must close whatever style it cuts through. Truncating a styled run in
// the middle drops its trailing reset, and lipgloss then re-opens that colour
// on the following line — a host's colour from the log legend ends up painting
// the next row's timestamp.
func TestTruncClosesTheStyleItCutsThrough(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	for _, tc := range []struct{ name, s string }{
		{"cut inside a styled run", "head " + blue.Render(strings.Repeat("x", 60))},
		{"cut a wholly styled run", blue.Render(strings.Repeat("x", 60))},
		{"cut inside styled CJK", "wide " + blue.Render("日本語テキストですよ")},
	} {
		got := trunc(tc.s, 20)
		if openStyleAtEOL(got) {
			t.Errorf("%s: trunc left a style open: %q", tc.name, got)
		}
		if w := lipgloss.Width(got); w != 20 {
			t.Errorf("%s: width %d, want 20: %q", tc.name, w, got)
		}
	}
}

// trunc must not gain a reset it never needed, or every plain row in the table
// grows escape bytes for nothing.
func TestTruncLeavesUnstyledAndShortStringsAlone(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	if got := trunc("abc", 20); got != "abc" {
		t.Errorf("a string that already fits must pass through unchanged, got %q", got)
	}
	if got := trunc(strings.Repeat("x", 60), 20); strings.Contains(got, "\x1b") {
		t.Errorf("an unstyled cut must stay unstyled, got %q", got)
	}
	if got := lipgloss.Width(trunc(strings.Repeat("x", 60), 1)); got != 1 {
		t.Errorf("n=1 must yield width 1, got %d", got)
	}
}

// The end-to-end guard, and the symptom from the report. lipgloss closes a
// line for padding but RE-OPENS the still-open style at the start of the next
// one, so a legend truncated inside a host's colour repaints the row below it:
// the first log row's timestamp came out tinted with that host's colour
// instead of dim, while every row under it was correct.
//
// Asserting that all timestamps share one prefix catches that generically,
// without pinning which escape the dim style happens to compile to.
func TestLeakedStyleNeverRepaintsTheFollowingRow(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	stampRE := regexp.MustCompile(`((?:\x1b\[[0-9;]*m)*)\d\d:\d\d:\d\d`)

	for _, w := range []int{40, 47, 54, 61, 68, 75, 100, 140} {
		m := ansiModel(30, w)
		m.logOpen = true
		m.streams = map[string]stream{"host-nano": {}, "host-pi": {},
			"host-edge": {}, "host-lab": {}, "host-desktop": {}}
		for i := 0; i < 12; i++ {
			m.appendLog("host-nano", fmt.Sprintf("Installing package %d...", i))
		}
		for i, line := range strings.Split(m.View(), "\n") {
			mm := stampRE.FindStringSubmatch(line)
			if mm == nil {
				continue
			}
			// The timestamp wears exactly one style, dim. A second escape in
			// front of it was carried over from the line above.
			if n := len(sgrRE.FindAllString(mm[1], -1)); n > 1 {
				t.Errorf("w=%d line %d: timestamp wears %d styles, want 1 — %q was leaked from the line above",
					w, i, n, mm[1])
			}
		}
	}
}
