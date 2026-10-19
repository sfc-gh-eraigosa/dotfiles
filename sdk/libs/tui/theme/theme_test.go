package theme

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestEveryPaletteHasEveryRole(t *testing.T) {
	for _, name := range Names() {
		p, ok := Get(name)
		if !ok || p.Name != name {
			t.Fatalf("Get(%s)", name)
		}
		roles := map[string]string{"ok": p.OK, "warn": p.Warn, "crit": p.Crit, "info": p.Info, "accent": p.Accent, "dim": p.Dim, "border": p.Border}
		for role, c := range roles {
			if c == "" {
				t.Errorf("%s.%s is empty", name, role)
			}
		}
		if len(p.Series) < 6 {
			t.Errorf("%s has %d series colors, want ≥ 6", name, len(p.Series))
		}
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("unknown palette found")
	}
}

// dark-daltonism must not rely on red vs green for ok/crit.
func TestDaltonismAvoidsRedGreen(t *testing.T) {
	p, _ := Get(DarkDaltonism)
	for _, c := range []string{p.OK, p.Crit} {
		if c == "2" || c == "1" || c == "42" || c == "196" || c == "203" {
			t.Fatalf("daltonism uses red/green %s", c)
		}
	}
}

// One resolve rule for every sdk tool (gsl and gff's precedence).
func TestResolve(t *testing.T) {
	dark := func() bool { return true }
	light := func() bool { return false }
	cases := []struct {
		name string
		env  map[string]string
		bg   func() bool
		want string
	}{
		{"tool override", map[string]string{"GDASH_THEME": "light", "SDK_THEME": "dark8", "TERM": "xterm-256color"}, dark, Light},
		{"sdk override", map[string]string{"SDK_THEME": "dark-daltonism", "TERM": "xterm-256color"}, dark, DarkDaltonism},
		{"unknown override falls through", map[string]string{"GDASH_THEME": "neon", "TERM": "xterm-256color"}, dark, Dark},
		{"tmux → dark8", map[string]string{"TMUX": "/tmp/tmux-1/default,1,0", "TERM": "tmux-256color"}, dark, Dark8},
		{"8-color terminal → dark8", map[string]string{"TERM": "xterm"}, dark, Dark8},
		{"truecolor, dark background", map[string]string{"COLORTERM": "truecolor", "TERM": "xterm"}, dark, Dark},
		{"256color, light background", map[string]string{"TERM": "xterm-256color"}, light, Light},
	}
	for _, tc := range cases {
		if got := Resolve("gdash", envOf(tc.env), tc.bg); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestEnabled(t *testing.T) {
	if Enabled(envOf(map[string]string{"NO_COLOR": "1", "TERM": "xterm-256color"})) {
		t.Fatal("NO_COLOR ignored")
	}
	if Enabled(envOf(map[string]string{"TERM": "dumb"})) {
		t.Fatal("TERM=dumb colored")
	}
	if !Enabled(envOf(map[string]string{"TERM": "xterm-256color"})) {
		t.Fatal("a normal terminal is not colored")
	}
}

func TestEnvDarkBackground(t *testing.T) {
	cases := map[string]bool{"": true, "15;0": true, "0;15": false, "15;default;7": false, "garbage": true}
	for v, want := range cases {
		if got := EnvDarkBackground(envOf(map[string]string{"COLORFGBG": v}))(); got != want {
			t.Errorf("COLORFGBG=%q → dark %v, want %v", v, got, want)
		}
	}
}

func TestFG(t *testing.T) {
	cases := map[string]string{"": "", "2": "\x1b[32m", "8": "\x1b[90m", "15": "\x1b[97m", "42": "\x1b[38;5;42m", "green": "\x1b[32m", "red": "\x1b[31m"}
	for c, want := range cases {
		if got := FG(c); got != want {
			t.Errorf("FG(%q) = %q, want %q", c, got, want)
		}
	}
	if s := Paint("x", "42"); !strings.HasPrefix(s, "\x1b[38;5;42m") || !strings.HasSuffix(s, Reset) {
		t.Fatalf("Paint = %q", s)
	}
	if Paint("x", "") != "x" {
		t.Fatal("Paint with no color must leave the text alone")
	}
}

func TestSeriesWraps(t *testing.T) {
	p, _ := Get(Dark)
	if p.SeriesColor(0) != p.Series[0] || p.SeriesColor(len(p.Series)) != p.Series[0] || p.SeriesColor(-1) != p.Series[0] {
		t.Fatal("SeriesColor must wrap and never panic")
	}
}

func TestFor(t *testing.T) {
	p, on := For("mono", envOf(map[string]string{"MONO_THEME": Light, "TERM": "xterm-256color"}))
	if p.Name != Light || !on {
		t.Fatalf("For = %q, %v", p.Name, on)
	}
	if _, on := For("mono", envOf(map[string]string{"NO_COLOR": "1", "TERM": "xterm-256color"})); on {
		t.Fatal("NO_COLOR still enabled")
	}
}

func TestChrome(t *testing.T) {
	c := ChromeFor("mono", envOf(map[string]string{"MONO_THEME": Dark, "TERM": "xterm-256color"}))
	if got := c.Accent("x"); got != Paint("x", c.P.Accent) {
		t.Fatalf("Accent = %q", got)
	}
	if got := c.Err("x"); got != Paint("x", c.P.Crit) {
		t.Fatalf("Err = %q", got)
	}
	off := ChromeFor("mono", envOf(map[string]string{"NO_COLOR": "1"}))
	for _, got := range []string{off.Dim("x"), off.Bold("x"), off.Accent("x"), off.Err("x")} {
		if got != "x" {
			t.Fatalf("NO_COLOR painted %q", got)
		}
	}
}
