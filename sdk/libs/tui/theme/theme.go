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
	Series []string // chart lines, hosts — in order
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
		Series: []string{"26", "130", "92", "30", "136", "125", "31", "28"}},
	// dark8: basic ANSI, recolored by the terminal's own theme (tmux, 8-color).
	Dark8: {Name: Dark8, OK: "2", Warn: "3", Crit: "1", Info: "4", Accent: "5", Dim: "8", Text: "", Border: "8",
		Series: []string{"4", "3", "5", "6", "2", "1"}},
	// dark-daltonism: red-green safe — ok is teal and crit orange, as in gsl.
	DarkDaltonism: {Name: DarkDaltonism, OK: "73", Warn: "178", Crit: "208", Info: "69", Accent: "99", Dim: "245", Text: "255", Border: "240",
		Series: []string{"69", "208", "44", "178", "99", "73", "172", "147"}},
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
//  2. inside tmux/screen, or on a terminal without 256 colors: dark8, so the
//     terminal's own theme colors it (gsl and gff's rule);
//  3. otherwise the background: dark → dark, light → light.
func Resolve(tool string, env func(string) string, darkBackground func() bool) string {
	for _, k := range []string{strings.ToUpper(strings.ReplaceAll(tool, "-", "_")) + "_THEME", "SDK_THEME"} {
		if v := env(k); v != "" {
			if _, ok := palettes[v]; ok {
				return v
			}
		}
	}
	term := strings.ToLower(env("TERM"))
	if env("TMUX") != "" || strings.HasPrefix(term, "tmux") || strings.HasPrefix(term, "screen") {
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
