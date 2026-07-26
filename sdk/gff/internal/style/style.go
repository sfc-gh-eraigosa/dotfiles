// Package style owns gff's color truth and theme resolution, ported from
// sdk/gsl's internal/theme + internal/style split: detection yields a palette
// NAME (dark / light / dark8), and a palette struct holds the colors. Both the
// TUI and the styled `gff list` table draw from here so the two surfaces
// always match.
//
// Unlike gsl (whose status line renders to a pipe and must infer the theme
// from settings files), gff's styled surfaces run on a real TTY — so the
// terminal's own background is the primary signal, queried once via
// lipgloss/termenv (OSC-11 with a COLORFGBG fallback).
//
// Resolution precedence:
//  1. GFF_THEME env override: "dark" | "light" | "dark8" (anything else
//     falls through — explicit user control, same spirit as gsl's toolCtx).
//  2. Low-color terminal (no 256color/truecolor): "dark8" — basic ANSI
//     indices are themed BY the terminal, so they follow the shell theme
//     on any background (gsl's terminalPalette makes the same call).
//  3. Terminal background query: dark background → "dark", light → "light".
package style

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// Colors holds the semantic color slots used by the TUI and list table.
type Colors struct {
	Grey   lipgloss.Color // dim/help text, default markers
	Green  lipgloss.Color // true / enabled
	Blue   lipgloss.Color // repo-live layer
	Orange lipgloss.Color // user-override layer
	Red    lipgloss.Color // false / errors
	Purple lipgloss.Color // header emphasis
	Text   lipgloss.Color // emphasized foreground (selected rows)
	Border lipgloss.Color // table borders
}

// palettes: "dark" is the original gff palette; "light" ports gsl's
// mid-luminance light-theme indices (blue 26, green 34, purple 92, amber 136)
// plus legible dark tones for the gff-specific slots; "dark8" uses basic ANSI
// indices that every terminal theme recolors itself.
var palettes = map[string]Colors{
	"dark": {
		Grey: "245", Green: "42", Blue: "39", Orange: "214",
		Red: "203", Purple: "63", Text: "255", Border: "240",
	},
	"light": {
		Grey: "241", Green: "34", Blue: "26", Orange: "136",
		Red: "124", Purple: "92", Text: "16", Border: "247",
	},
	"dark8": {
		Grey: "7", Green: "2", Blue: "4", Orange: "3",
		Red: "1", Purple: "5", Text: "7", Border: "7",
	},
}

// Palette returns the named palette. ok=false for unknown names.
func Palette(name string) (Colors, bool) { p, ok := palettes[name]; return p, ok }

// Resolve returns the palette name for the given environment and background
// probe. Pure — env and hasDarkBg are injected for testability.
func Resolve(env func(string) string, hasDarkBg func() bool) string {
	if name := env("GFF_THEME"); name != "" {
		if _, ok := palettes[name]; ok {
			return name
		}
	}
	ct := strings.ToLower(env("COLORTERM"))
	truecolor := ct == "truecolor" || ct == "24bit"
	if !truecolor && !strings.Contains(strings.ToLower(env("TERM")), "256color") {
		return "dark8"
	}
	// Inside tmux/screen the OSC-11 background query does not pass through, so
	// hasDarkBg silently reports the termenv default (dark) — near-invisible
	// text on light terminals. With no explicit signal (GFF_THEME handled
	// above, COLORFGBG below), prefer basic ANSI: the terminal's own theme
	// recolors those indices, so contrast is correct on any background.
	inTmux := env("TMUX") != "" ||
		strings.HasPrefix(env("TERM"), "tmux") || strings.HasPrefix(env("TERM"), "screen")
	if inTmux && env("COLORFGBG") == "" {
		return "dark8"
	}
	if hasDarkBg() {
		return "dark"
	}
	return "light"
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
