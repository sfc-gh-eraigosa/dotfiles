// Package style is gff's adapter onto the sdk's one color scheme,
// sdk/libs/tui/theme: the palettes, their colors and the rule for picking
// one live there, shared with fleet, gsl, mono and gdash. This package gives
// gff the roles as lipgloss colors so the TUI and the styled `gff list`
// table draw from one place and always match.
//
// Resolution is theme.Resolve("gff", …):
//  1. GFF_THEME, then SDK_THEME: "dark" | "light" | "dark8" |
//     "dark-daltonism" (anything else falls through);
//  2. inside tmux/screen without COLORFGBG, or a terminal without 256
//     colors: "dark8" — basic ANSI, themed BY the terminal, so it follows
//     the shell theme on any background (tmux does not pass the background
//     query through);
//  3. the terminal background, queried once via lipgloss/termenv (OSC-11
//     with a COLORFGBG fallback): dark → "dark", light → "light". gff's
//     surfaces run on a real TTY, so the query is the primary signal.
package style

import (
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
)

// Colors holds the color slots used by the TUI and list table: the shared
// scheme's roles, under the same names (under dark-daltonism OK is teal and
// Crit orange, so the slots are named for meaning, not hue).
type Colors struct {
	Dim    lipgloss.Color // help text, default markers
	OK     lipgloss.Color // true / enabled
	Info   lipgloss.Color // repo-live layer
	Warn   lipgloss.Color // user-override layer
	Crit   lipgloss.Color // false / errors
	Accent lipgloss.Color // header emphasis
	Text   lipgloss.Color // emphasized foreground ("" = terminal default)
	Border lipgloss.Color // table borders
}

// Palette returns the named shared palette as gff's slots. ok=false for
// unknown names.
func Palette(name string) (Colors, bool) {
	p, ok := theme.Get(name)
	if !ok {
		return Colors{}, false
	}
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	return Colors{
		Dim: c(p.Dim), OK: c(p.OK), Info: c(p.Info), Warn: c(p.Warn),
		Crit: c(p.Crit), Accent: c(p.Accent), Text: c(p.Text), Border: c(p.Border),
	}, true
}

// Resolve returns the palette name for the given environment and background
// probe. Pure — env and hasDarkBg are injected for testability.
func Resolve(env func(string) string, hasDarkBg func() bool) string {
	return theme.Resolve("gff", env, hasDarkBg)
}

// darkBg caches the terminal background probe: the OSC-11 query is a real
// terminal round-trip (with timeout), so ask at most once per process.
var (
	bgOnce sync.Once
	darkBg bool
)

func hasDarkBackground() bool {
	bgOnce.Do(func() { darkBg = lipgloss.HasDarkBackground() })
	return darkBg
}

// Active resolves and returns the palette for the current process
// environment and terminal.
func Active() Colors {
	p, _ := Palette(Resolve(os.Getenv, hasDarkBackground))
	return p
}
