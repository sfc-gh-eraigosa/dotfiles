package style

// Theme handling ported from sdk/gsl (internal/theme + internal/style):
// a palette NAME is resolved first (dark / light / dark8), and a palette
// struct owns all color truth. gff's TUI/list run on a real TTY, so the
// terminal background is detectable (lipgloss/termenv OSC-11 + COLORFGBG);
// gsl's status line renders to a pipe and never could.

import "testing"

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolve_ExplicitOverrideWins(t *testing.T) {
	for _, name := range []string{"dark", "light", "dark8"} {
		got := Resolve(envOf(map[string]string{"GFF_THEME": name, "TERM": "xterm-256color"}), func() bool { return true })
		if got != name {
			t.Errorf("GFF_THEME=%q: got %q, want %q", name, got, name)
		}
	}
}

func TestResolve_UnknownOverrideFallsThrough(t *testing.T) {
	got := Resolve(envOf(map[string]string{"GFF_THEME": "sparkle", "TERM": "xterm-256color"}), func() bool { return false })
	if got != "light" {
		t.Errorf("unknown GFF_THEME with light background: got %q, want light", got)
	}
}

func TestResolve_LowColorTerminalUsesANSINamed(t *testing.T) {
	// Basic ANSI colors are themed BY the terminal, so dark8 is safe on any
	// background — gsl's terminalPalette makes the same call.
	got := Resolve(envOf(map[string]string{"TERM": "xterm"}), func() bool { return true })
	if got != "dark8" {
		t.Errorf("low-color TERM: got %q, want dark8", got)
	}
}

func TestResolve_TruecolorHonorsBackground(t *testing.T) {
	env := envOf(map[string]string{"COLORTERM": "truecolor", "TERM": "xterm"})
	if got := Resolve(env, func() bool { return true }); got != "dark" {
		t.Errorf("truecolor+dark bg: got %q, want dark", got)
	}
	if got := Resolve(env, func() bool { return false }); got != "light" {
		t.Errorf("truecolor+light bg: got %q, want light", got)
	}
}

func TestPalette_AllNamesExistAndDiffer(t *testing.T) {
	for _, name := range []string{"dark", "light", "dark8"} {
		p, ok := Palette(name)
		if !ok {
			t.Fatalf("Palette(%q) not found", name)
		}
		if p.Blue == "" || p.Grey == "" || p.Orange == "" || p.Text == "" {
			t.Errorf("Palette(%q) has empty slots: %+v", name, p)
		}
	}
	d, _ := Palette("dark")
	l, _ := Palette("light")
	if d.Blue == l.Blue || d.Grey == l.Grey {
		t.Error("dark and light palettes must differ (light uses mid-luminance tones per gsl)")
	}
}

func TestPalette_UnknownName(t *testing.T) {
	if _, ok := Palette("nonexistent"); ok {
		t.Error("Palette(nonexistent) must return ok=false")
	}
}

func TestActiveFollowsGFFTheme(t *testing.T) {
	t.Setenv("GFF_THEME", "light")
	want, _ := Palette("light")
	if got := Active(); got != want {
		t.Errorf("Active() with GFF_THEME=light = %+v, want light palette", got)
	}
}

// Owner-reported: under tmux the OSC-11 background query does not pass
// through and COLORFGBG is absent, so the probe silently defaulted to dark —
// near-white text on light terminals. Inside tmux with no explicit signal,
// resolve to the ANSI palette: the terminal's own theme recolors basic ANSI,
// so contrast is always correct.
func TestResolve_TmuxWithoutSignalsUsesANSI(t *testing.T) {
	for _, env := range []map[string]string{
		{"TMUX": "/tmp/tmux-1000/default,123,0", "TERM": "tmux-256color"},
		{"TERM": "screen-256color"}, // tmux/screen TERM even without $TMUX
	} {
		got := Resolve(envOf(env), func() bool { return true })
		if got != "dark8" {
			t.Errorf("env %v: got %q, want dark8 (terminal-themed ANSI)", env, got)
		}
	}
}

func TestResolve_TmuxExplicitSignalsStillWin(t *testing.T) {
	// GFF_THEME override wins inside tmux…
	got := Resolve(envOf(map[string]string{"TMUX": "x", "TERM": "tmux-256color", "GFF_THEME": "light"}), func() bool { return true })
	if got != "light" {
		t.Errorf("GFF_THEME in tmux: got %q, want light", got)
	}
	// …and an explicit COLORFGBG means the background IS known: honor it.
	got = Resolve(envOf(map[string]string{"TMUX": "x", "TERM": "tmux-256color", "COLORFGBG": "0;15"}), func() bool { return false })
	if got != "light" {
		t.Errorf("COLORFGBG in tmux: got %q, want light", got)
	}
}
