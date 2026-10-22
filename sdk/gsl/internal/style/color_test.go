package style

import (
	"strings"
	"testing"
)

func TestParseColor(t *testing.T) {
	cases := []struct {
		in     string
		ok     bool
		zero   bool
		index  int
		rgb    RGB
		hasRGB bool
	}{
		{in: "", ok: true, zero: true},
		{in: "default", ok: true, zero: true},
		{in: "blue", ok: true, index: 4},
		{in: "12", ok: true, index: 12},
		{in: "255", ok: true, index: 255},
		{in: "38;5;214", ok: true, index: 214},
		{in: "48;5;26", ok: true, index: 26},
		{in: "#00ff00", ok: true, hasRGB: true, rgb: RGB{0, 255, 0}},
		{in: "#ABCDEF", ok: true, hasRGB: true, rgb: RGB{0xab, 0xcd, 0xef}},
		{in: "38;2;250;250;210", ok: true, hasRGB: true, rgb: RGB{250, 250, 210}},
		{in: "nope", ok: false},
		{in: "256", ok: false},
		{in: "-1", ok: false},
		{in: "#12345", ok: false},
		{in: "1;31", ok: false},
		{in: "38;2;300;0;0", ok: false},
		{in: "\x1b[31m", ok: false},
	}
	for _, c := range cases {
		got, ok := ParseColor(c.in)
		if ok != c.ok {
			t.Errorf("ParseColor(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		switch {
		case c.zero && !got.IsZero():
			t.Errorf("ParseColor(%q) = %+v, want zero", c.in, got)
		case c.hasRGB && got.RGB() != c.rgb:
			t.Errorf("ParseColor(%q) rgb = %v, want %v", c.in, got.RGB(), c.rgb)
		case !c.zero && !c.hasRGB && (got.index != c.index):
			t.Errorf("ParseColor(%q) index = %d, want %d", c.in, got.index, c.index)
		}
	}
}

// Emission follows the capability ladder; 0–15 stay terminal-themed indices.
func TestColorSGR(t *testing.T) {
	green := FromRGB(RGB{0, 255, 0})
	cases := []struct {
		c    Color
		bg   bool
		mode ColorMode
		want string
	}{
		{Index(4), false, Mode256, "\x1b[38;5;4m"},
		{Index(4), true, Mode256, "\x1b[48;5;4m"},
		{Index(4), false, Mode16, "\x1b[34m"},
		{Index(12), true, Mode16, "\x1b[104m"},
		{Index(214), false, ModeTruecolor, "\x1b[38;5;214m"},
		{green, false, ModeTruecolor, "\x1b[38;2;0;255;0m"},
		{green, true, Mode256, "\x1b[48;5;46m"},
		{green, false, Mode16, "\x1b[92m"},
		{Index(214), false, Mode16, "\x1b[33m"}, // (255,175,0) is nearest the basic yellow (205,205,0)
		{Index(4), false, ModeNever, ""},
		{Color{}, false, Mode256, ""},
	}
	for _, c := range cases {
		if got := c.c.SGR(c.bg, c.mode); got != c.want {
			t.Errorf("%+v bg=%v mode=%v = %q, want %q", c.c, c.bg, c.mode, got, c.want)
		}
	}
}

func TestContrastRatio(t *testing.T) {
	if r := ContrastRatio(RGB{0, 0, 0}, RGB{255, 255, 255}); r < 20.9 || r > 21.01 {
		t.Errorf("black/white = %.2f, want 21", r)
	}
	if r := ContrastRatio(RGB{255, 255, 255}, RGB{255, 255, 0}); r > 1.1 {
		t.Errorf("white on yellow = %.2f, want ~1.07", r)
	}
	if ContrastRatio(RGB{10, 20, 30}, RGB{200, 100, 50}) != ContrastRatio(RGB{200, 100, 50}, RGB{10, 20, 30}) {
		t.Error("contrast must be symmetric")
	}
}

// E14: every block resolves to a concrete pair, whatever the theme holds.
func TestResolveBlockColors_AlwaysConcrete(t *testing.T) {
	for _, v := range []string{"default", "#00ff00", "nope", "", "214"} {
		st := Style{Theme: map[string]string{"dirgit": v, "fg": "default"}}
		bg, fg := ResolveBlockColors(st, RoleDirGit)
		if bg.IsZero() || fg.IsZero() {
			t.Errorf("theme dirgit=%q → bg %+v fg %+v, want both concrete", v, bg, fg)
		}
	}
	bg, fg := ResolveBlockColors(Style{}, Role("no-such-role"))
	if bg.IsZero() || fg.IsZero() {
		t.Error("an unknown role must still resolve")
	}
	// The user's fg always wins.
	_, fg = ResolveBlockColors(Style{Theme: map[string]string{"dirgit": "214", "fg": "#ff0000"}}, RoleDirGit)
	if fg.RGB() != (RGB{255, 0, 0}) {
		t.Errorf("user fg lost: %+v", fg)
	}
}

// E15: every builtin palette × role, the computed text clears WCAG 4.5:1.
// dark8 is terminal-themed (its RGB is the terminal's, not ours), so it is
// excluded; its text is white, as the terminal theme intends.
func TestContrast_AllBuiltinPalettes(t *testing.T) {
	for _, name := range []string{"dark", "light", "dark-daltonism"} {
		p, _ := Palette(name)
		for role, v := range p {
			st := Style{Theme: map[string]string{role: v, "fg": "default"}}
			bg, fg := ResolveBlockColors(st, Role(role))
			if r := ContrastRatio(bg.RGB(), fg.RGB()); r < 4.5 {
				t.Errorf("%s.%s: bg %s text contrast %.2f < 4.5", name, role, v, r)
			}
		}
	}
	// White text on yellow is exactly what must not happen.
	bg, fg := ResolveBlockColors(Style{Theme: map[string]string{"time": "#ffff00"}}, RoleTime)
	if fg.RGB() == (RGB{255, 255, 255}) || ContrastRatio(bg.RGB(), fg.RGB()) < 4.5 {
		t.Error("white on yellow chosen")
	}
}

// E16: the capability ladder.
func TestColorMode_Ladder(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		env  map[string]string
		flag string
		want ColorMode
	}{
		{map[string]string{"NO_COLOR": "1", "COLORTERM": "truecolor"}, "", ModeNever},
		{map[string]string{"TERM": "dumb"}, "", ModeNever},
		{map[string]string{"COLORTERM": "truecolor", "TERM": "xterm"}, "", ModeTruecolor},
		{map[string]string{"COLORTERM": "24bit"}, "", ModeTruecolor},
		{map[string]string{"TERM": "xterm-256color"}, "", Mode256},
		// A bare TERM (xterm over ssh, screen, tmux) has meant 256 colours for
		// gsl all along; only terminals known to have 16 drop to them.
		{map[string]string{"TERM": "xterm"}, "", Mode256},
		{map[string]string{"TERM": "screen"}, "", Mode256},
		{map[string]string{"TERM": "linux"}, "", Mode16},
		{map[string]string{"TERM": "vt100"}, "", Mode16},
		{map[string]string{}, "", Mode256}, // a pipe under Claude Code: no TERM
		{map[string]string{"TERM": "linux"}, "truecolor", ModeTruecolor},
		{map[string]string{"NO_COLOR": "1"}, "256", Mode256}, // an explicit flag wins
		{map[string]string{"TERM": "xterm-256color"}, "never", ModeNever},
		{map[string]string{"TERM": "xterm-256color"}, "auto", Mode256},
	}
	for _, c := range cases {
		if got := DetectColorMode(env(c.env), c.flag); got != c.want {
			t.Errorf("env %v flag %q = %v, want %v", c.env, c.flag, got, c.want)
		}
	}
	if _, err := ParseColorMode("rainbow"); err == nil {
		t.Error("unknown --color value accepted")
	}
}

// Theme values are config, not trusted escapes: nothing that could smuggle a
// sequence into the line parses (the old render.colorCode untrusted table).
func TestParseColor_RejectsInjection(t *testing.T) {
	for _, v := range []string{
		"0m\x1b[2J", "38;5;1;48;5;2", "; rm -rf", "\x1b[31m", "notacolor", "999",
		"38;2;256;0;0", "38;2;0;256;0", "38;2;0;0;256", "38;2;-1;0;0", "99;2;10;20;30",
	} {
		if c, ok := ParseColor(v); ok {
			t.Errorf("ParseColor(%q) accepted: %+v", v, c)
		}
	}
	// A parsed colour always emits exactly one well-formed SGR.
	for _, v := range []string{"0", "123", "255", "green", "white", "38;2;10;20;30", "48;2;0;128;255"} {
		c, ok := ParseColor(v)
		if !ok {
			t.Fatalf("%q rejected", v)
		}
		for _, m := range []ColorMode{Mode16, Mode256, ModeTruecolor} {
			seq := c.SGR(false, m)
			if strings.Count(seq, "\x1b") != 1 || !strings.HasSuffix(seq, "m") {
				t.Errorf("%q in %v → %q", v, m, seq)
			}
		}
	}
}

// Text on a terminal-themed 0–15 colour: dark on the light ones (white,
// yellow and the brights), white on the dark ones — a white MERGED chip on
// the powerline "accent: white" must not vanish.
func TestTextOn_ThemedColours(t *testing.T) {
	for _, light := range []int{3, 7, 10, 11, 14, 15} {
		if got := TextOn(Index(light)); got != Index(0) {
			t.Errorf("TextOn(%d) = %+v, want black (0)", light, got)
		}
	}
	for _, dark := range []int{0, 1, 4, 5, 8} {
		if got := TextOn(Index(dark)); got != Index(7) {
			t.Errorf("TextOn(%d) = %+v, want white (7)", dark, got)
		}
	}
}
