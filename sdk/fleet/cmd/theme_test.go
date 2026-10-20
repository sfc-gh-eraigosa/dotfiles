package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
	sdktheme "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
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

// Colors come from the sdk's shared scheme (libs/tui/theme): each status is a
// role, each host a series color, so the same palette looks the same in
// fleet, gff, gsl, mono and gdash. No color number is chosen here.
func TestThemeDrawsFromSharedScheme(t *testing.T) {
	for _, name := range sdktheme.Names() {
		p, _ := sdktheme.Get(name)
		th := newThemeFor(false, p)
		fg := func(s lipgloss.Style) string { c, _ := s.GetForeground().(lipgloss.Color); return string(c) }
		roles := map[string][2]string{
			"ok":          {fg(th.ok), p.OK},
			"fail":        {fg(th.fail), p.Crit},
			"warn":        {fg(th.warn), p.Warn},
			"running":     {fg(th.running), p.Info},
			"title":       {fg(th.title), p.Accent},
			"selected":    {fg(th.selected), p.Accent},
			"up-to-date":  {fg(th.byClass[string(drift.UpToDate)]), p.OK},
			"behind":      {fg(th.byClass[string(drift.Behind)]), p.Info},
			"unreachable": {fg(th.byClass[string(drift.Unreachable)]), p.Crit},
			"auth-failed": {fg(th.byClass[string(drift.AuthFailed)]), p.Warn},
		}
		for role, got := range roles {
			if got[0] != got[1] {
				t.Errorf("%s %s = %q, want %q", name, role, got[0], got[1])
			}
		}
		if c, _ := th.panel.GetBorderBottomForeground().(lipgloss.Color); string(c) != p.Border {
			t.Errorf("%s panel border = %q, want %q", name, c, p.Border)
		}
		if len(th.logHosts) != len(p.Series) {
			t.Fatalf("%s: %d host colors, want the %d series colors", name, len(th.logHosts), len(p.Series))
		}
		for i, s := range th.logHosts {
			if fg(s) != p.Series[i] {
				t.Errorf("%s host %d = %q, want %q", name, i, fg(s), p.Series[i])
			}
		}
	}
	// The package default every View() test renders through is dark, and the
	// dark series keeps fleet's original host colors.
	if fg := th.logHosts[0].GetForeground().(lipgloss.Color); fg != "33" {
		t.Errorf("default first host color = %q, want 33", fg)
	}
}

// fleet tui resolves its palette the same way every sdk tool does:
// FLEET_THEME, then SDK_THEME, tmux → dark8, else the background.
func TestFleetPaletteResolves(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	dark := func() bool { return true }
	if got := fleetPalette(env(map[string]string{"FLEET_THEME": "light", "TERM": "xterm-256color"}), dark).Name; got != "light" {
		t.Errorf("FLEET_THEME=light → %s", got)
	}
	if got := fleetPalette(env(map[string]string{"TMUX": "x", "TERM": "tmux-256color"}), dark).Name; got != "dark8" {
		t.Errorf("tmux → %s", got)
	}
}

// Every drift class reads differently by color, not only by weight: behind
// (an update waits) is Info, auth-failed (look at your keys) is Warn,
// unreachable Crit — "the colour difference is the whole point".
func TestDriftClassesDifferByColor(t *testing.T) {
	for _, name := range sdktheme.Names() {
		p, _ := sdktheme.Get(name)
		th := newThemeFor(false, p)
		seen := map[string]string{}
		for _, cl := range []drift.Class{drift.UpToDate, drift.Behind, drift.Divergent, drift.Unreachable, drift.AuthFailed} {
			c, _ := th.byClass[string(cl)].GetForeground().(lipgloss.Color)
			if other, dup := seen[string(c)]; dup {
				t.Errorf("%s: %s and %s are both %q", name, other, cl, c)
			}
			seen[string(c)] = string(cl)
		}
		// "You are here" is none of the class colors, and readable on its
		// palette's background (light gets a dark pink, not dark's 213).
		local, _ := th.local.GetForeground().(lipgloss.Color)
		if cl, dup := seen[string(local)]; dup {
			t.Errorf("%s: local color %q is %s's", name, local, cl)
		}
		if name == sdktheme.Light && local == "213" {
			t.Errorf("light: local is the dark-background pink 213")
		}
	}
}
