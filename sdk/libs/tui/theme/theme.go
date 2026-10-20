// Package theme is the sdk's one color scheme: named palettes of semantic
// roles (ok, warn, crit, info, accent, dim, text, border) plus an ordered
// series list for chart lines and hosts, and one rule for picking a palette.
//
// It gathers what the tools had each grown on their own: gsl's palette names
// and light-theme indices (internal/style), gff's semantic slots and terminal
// rule (internal/style, itself ported from gsl), and fleet's host colors as
// the dark series. New tools draw from here; existing ones move over as they
// are touched (mono-gdash plan E1).
//
// The package has no dependencies: colors are ANSI strings ("42", "2",
// "green"), FG turns one into an escape, and lipgloss users pass the same
// string to lipgloss.Color.
package theme

import (
	"math"
	"strconv"
	"strings"
)

// Palette names (gsl's four).
const (
	Dark          = "dark"
	Light         = "light"
	Dark8         = "dark8"
	DarkDaltonism = "dark-daltonism"
)

// Reset ends any color FG started.
const Reset = "\x1b[0m"

// Palette is one scheme. Text "" means the terminal's own foreground.
type Palette struct {
	Name   string
	OK     string   // good / enabled / under threshold
	Warn   string   // needs attention
	Crit   string   // failed / over threshold
	Info   string   // neutral emphasis (running, links)
	Accent string   // headers, focus
	Dim    string   // secondary text, help
	Text   string   // emphasized foreground
	Border string   // rules and boxes
	Series []string // chart lines, hosts — in order; never the Crit color
}

// SeriesColor is the i-th series color, wrapping; never panics.
func (p Palette) SeriesColor(i int) string {
	if len(p.Series) == 0 {
		return ""
	}
	if i < 0 {
		i = 0
	}
	return p.Series[i%len(p.Series)]
}

var palettes = map[string]Palette{
	// dark: gff's dark slots; fleet's host colors as the series.
	Dark: {Name: Dark, OK: "42", Warn: "214", Crit: "203", Info: "39", Accent: "63", Dim: "245", Text: "255", Border: "240",
		Series: []string{"33", "208", "141", "37", "178", "169", "45", "113"}},
	// light: gsl's mid-luminance light indices (legible on light and dark).
	Light: {Name: Light, OK: "34", Warn: "136", Crit: "124", Info: "26", Accent: "92", Dim: "241", Text: "16", Border: "247",
		Series: []string{"26", "130", "55", "30", "136", "90", "31", "28"}},
	// dark8: basic ANSI, recolored by the terminal's own theme (tmux, 8-color);
	// its series skips red (crit) and adds bright blue.
	Dark8: {Name: Dark8, OK: "2", Warn: "3", Crit: "1", Info: "4", Accent: "5", Dim: "8", Text: "", Border: "8",
		Series: []string{"4", "3", "5", "6", "2", "12"}},
	// dark-daltonism: red-green safe — ok is teal and crit orange, as in gsl;
	// so its series holds no orange at all (172 read as failing).
	DarkDaltonism: {Name: DarkDaltonism, OK: "73", Warn: "178", Crit: "208", Info: "69", Accent: "99", Dim: "245", Text: "255", Border: "240",
		Series: []string{"69", "75", "147", "44", "99", "73", "110", "152"}},
}

// Names lists the palettes.
func Names() []string { return []string{Dark, Light, Dark8, DarkDaltonism} }

// Get returns a palette by name.
func Get(name string) (Palette, bool) {
	p, ok := palettes[name]
	if ok {
		p.Series = append([]string(nil), p.Series...)
	}
	return p, ok
}

// Enabled reports whether color is wanted at all: NO_COLOR unset (any
// value disables, per no-color.org) and a terminal that isn't "dumb".
func Enabled(env func(string) string) bool {
	return env("NO_COLOR") == "" && env("TERM") != "dumb"
}

// Resolve picks the palette for a tool, the same way for every sdk tool:
//  1. <TOOL>_THEME, then SDK_THEME, when it names a palette;
//  2. each hint in order, when it names one — a tool's own signal, such as
//     gsl reading the host AI tool's theme setting;
//  3. inside tmux/screen without COLORFGBG, or on a terminal without 256
//     colors: dark8, so the terminal's own theme colors it (tmux does not
//     pass a background query through; COLORFGBG does reach it);
//  4. otherwise the background: dark → dark, light → light.
func Resolve(tool string, env func(string) string, darkBackground func() bool, hints ...func() string) string {
	for _, k := range []string{strings.ToUpper(strings.ReplaceAll(tool, "-", "_")) + "_THEME", "SDK_THEME"} {
		if v := env(k); v != "" {
			if _, ok := palettes[v]; ok {
				return v
			}
		}
	}
	for _, h := range hints {
		if h == nil {
			continue
		}
		if v := h(); v != "" {
			if _, ok := palettes[v]; ok {
				return v
			}
		}
	}
	term := strings.ToLower(env("TERM"))
	inTmux := env("TMUX") != "" || strings.HasPrefix(term, "tmux") || strings.HasPrefix(term, "screen")
	if inTmux && env("COLORFGBG") == "" {
		return Dark8
	}
	ct := strings.ToLower(env("COLORTERM"))
	if ct != "truecolor" && ct != "24bit" && !strings.Contains(term, "256color") {
		return Dark8
	}
	if darkBackground != nil && !darkBackground() {
		return Light
	}
	return Dark
}

// For is a tool's palette, resolved once from the environment (Resolve with
// EnvDarkBackground), and whether color is wanted at all (Enabled). Tools
// call it at startup and keep the result, rather than resolving per paint.
func For(tool string, env func(string) string) (Palette, bool) {
	p, _ := Get(Resolve(tool, env, EnvDarkBackground(env)))
	return p, Enabled(env)
}

// EnvDarkBackground guesses the background from COLORFGBG ("fg;bg", set by
// rxvt, konsole and others): bg 7 or 9–15 is light. Unknown is dark, the
// common case. Tools on a real TTY may use a terminal query instead (gff
// asks lipgloss); this one needs nothing.
func EnvDarkBackground(env func(string) string) func() bool {
	return func() bool {
		parts := strings.Split(env("COLORFGBG"), ";")
		bg, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			return true
		}
		return bg != 7 && (bg < 9 || bg > 15)
	}
}

var named = map[string]int{"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4, "magenta": 5, "cyan": 6, "white": 7}

// BasicName is the color name ("blue") for a basic ANSI index "0"–"7", for
// renderers that spell those as names so no 256-color escape is emitted.
func BasicName(c string) (string, bool) {
	for name, n := range named {
		if strconv.Itoa(n) == c {
			return name, true
		}
	}
	return "", false
}

// TextOn is the text color that reads best on a fill of bg: "16" (black) when
// its contrast beats white's, else "7" (white). bg may be an ANSI-256 index,
// "38;5;N" / "48;5;N", or "38;2;r;g;b" / "48;2;r;g;b". Names and 0–15 are
// themed by the terminal, so their brightness is unknown: white, as before.
func TextOn(bg string) string {
	r, g, b, ok := rgbOf(strings.TrimSpace(bg))
	if !ok {
		return "7"
	}
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	l := 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
	if (l+0.05)/0.05 > 1.05/(l+0.05) {
		return "16"
	}
	return "7"
}

// rgbOf is a fill's RGB, for the forms TextOn takes; ok is false for names,
// 0–15 and anything malformed.
func rgbOf(c string) (r, g, b float64, ok bool) {
	parts := strings.Split(c, ";")
	switch {
	case len(parts) == 3 && (parts[0] == "38" || parts[0] == "48") && parts[1] == "5":
		c, parts = parts[2], nil
	case len(parts) == 5 && (parts[0] == "38" || parts[0] == "48") && parts[1] == "2":
		var v [3]float64
		for i, p := range parts[2:] {
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 || n > 255 {
				return 0, 0, 0, false
			}
			v[i] = float64(n)
		}
		return v[0], v[1], v[2], true
	case len(parts) != 1:
		return 0, 0, 0, false
	}
	n, err := strconv.Atoi(c)
	if err != nil || n < 16 || n > 255 {
		return 0, 0, 0, false
	}
	if n >= 232 {
		v := float64(8 + (n-232)*10)
		return v, v, v, true
	}
	level := []float64{0, 95, 135, 175, 215, 255}
	i := n - 16
	return level[i/36], level[i/6%6], level[i%6], true
}

// FG is the foreground escape for a color: "" → none, 0–7 or a name → the
// basic code (themed by the terminal), 8–15 → bright, else 256-color.
func FG(c string) string {
	if c == "" {
		return ""
	}
	if n, ok := named[c]; ok {
		return "\x1b[3" + strconv.Itoa(n) + "m"
	}
	n, err := strconv.Atoi(c)
	switch {
	case err != nil || n < 0 || n > 255:
		return ""
	case n < 8:
		return "\x1b[3" + strconv.Itoa(n) + "m"
	case n < 16:
		return "\x1b[9" + strconv.Itoa(n-8) + "m"
	}
	return "\x1b[38;5;" + c + "m"
}

// Paint wraps s in color c, or returns it unchanged when c is "".
func Paint(s, c string) string {
	if f := FG(c); f != "" {
		return f + s + Reset
	}
	return s
}

// Chrome paints a tool's own text (help overlays, dialogs) from one palette:
// Accent for headers, Crit for errors, SGR dim and bold. With Off set (color
// unwanted) every method returns its text unchanged. It satisfies
// libs/tui/overlay.Palette.
type Chrome struct {
	P   Palette
	Off bool
}

// ChromeFor is For as a Chrome, resolved once.
func ChromeFor(tool string, env func(string) string) Chrome {
	p, on := For(tool, env)
	return Chrome{P: p, Off: !on}
}

func (c Chrome) sgr(s, on, off string) string {
	if c.Off {
		return s
	}
	return on + s + off
}

func (c Chrome) Dim(s string) string  { return c.sgr(s, "\x1b[2m", "\x1b[22m") }
func (c Chrome) Bold(s string) string { return c.sgr(s, "\x1b[1m", "\x1b[22m") }

func (c Chrome) Accent(s string) string {
	if c.Off {
		return s
	}
	return Paint(s, c.P.Accent)
}

func (c Chrome) Err(s string) string {
	if c.Off {
		return s
	}
	return Paint(s, c.P.Crit)
}
