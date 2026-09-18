package cmd

import (
	"strings"
	"testing"
)

// The ASCII frames exist for a client that renders box-drawing glyphs badly
// (over Terminus on iPadOS the log pane's top border showed as a row of "?"
// with every line below it shifted). newTheme(true) must swap EVERY
// box-drawing glyph the frames emit — the panel and dialog borders AND the
// log column separator — not just the border, or the pane rows shift again
// relative to a frame that no longer does.
func TestNewTheme_ASCIISwapsBordersAndColumnSeparator(t *testing.T) {
	th := newTheme(true)
	for name, b := range map[string]struct{ top, left string }{
		"panel":  {th.panel.GetBorderStyle().Top, th.panel.GetBorderStyle().Left},
		"dialog": {th.dialog.GetBorderStyle().Top, th.dialog.GetBorderStyle().Left},
	} {
		if b.top != "-" || b.left != "|" {
			t.Errorf("%s border = top %q left %q; want ASCII \"-\" / \"|\"", name, b.top, b.left)
		}
	}
	if th.colSep != "|" {
		t.Errorf("colSep = %q; want ASCII \"|\"", th.colSep)
	}
}

// The default is the rounded set, and it is NOT read from the environment:
// the package-level theme every View() test renders through must be the same
// on a developer machine that exports FLEET_ASCII_BORDERS=1 in its shell.
func TestNewTheme_RoundedByDefaultRegardlessOfEnvironment(t *testing.T) {
	t.Setenv("FLEET_ASCII_BORDERS", "1")
	th := newTheme(false)
	b := th.panel.GetBorderStyle()
	if b.Top != "─" || b.TopLeft != "╭" {
		t.Errorf("panel border = top %q top-left %q; want the rounded default \"─\" / \"╭\"", b.Top, b.TopLeft)
	}
	if th.colSep != "│" {
		t.Errorf("colSep = %q; want \"│\"", th.colSep)
	}
}

// wantASCIIBorders: the gff flag is the host default; FLEET_ASCII_BORDERS is
// a per-session override in BOTH directions, and only "1"/"0" count — the
// first cut used `!= ""`, which made `FLEET_ASCII_BORDERS=0` turn ASCII on.
func TestWantASCIIBorders_EnvOverridesFlagInBothDirections(t *testing.T) {
	for _, tc := range []struct {
		env  string
		flag bool
		want bool
	}{
		{"", false, false},
		{"", true, true},
		{"1", false, true},
		{"1", true, true},
		{"0", true, false},
		{"0", false, false},
		{"yes", true, true}, // not an explicit value: defer to the flag
		{"false", false, false},
	} {
		t.Setenv("FLEET_ASCII_BORDERS", tc.env)
		if got := wantASCIIBorders(tc.flag); got != tc.want {
			t.Errorf("env=%q flag=%v: got %v, want %v", tc.env, tc.flag, got, tc.want)
		}
	}
}

// End to end through View(): with the ASCII theme installed the way `fleet
// tui` installs it, a rendered frame with a live log pane contains no
// box-drawing glyph at all.
func TestASCIIThemeRendersNoBoxDrawingThroughView(t *testing.T) {
	saved := th
	th = newTheme(true)
	t.Cleanup(func() { th = saved })

	m := ansiModel(30, 100)
	m.appendLog("host-nano", "working")
	out := m.View()

	for _, glyph := range []string{"╭", "╮", "╰", "╯", "─", "│"} {
		if strings.Contains(out, glyph) {
			t.Errorf("frame still contains box-drawing %q:\n%s", glyph, out)
		}
	}
	if !strings.Contains(out, "+-") {
		t.Errorf("frame has no ASCII corner/edge:\n%s", out)
	}
	if !strings.Contains(out, "host-nano     |") {
		t.Errorf("log column separator is not ASCII \"|\":\n%s", out)
	}
}
