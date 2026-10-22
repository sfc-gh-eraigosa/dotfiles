package cmd

import (
	"fmt"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gsl",
	Short: "gsl is a Go Status Line tool",
	Long: `gsl renders a powerline-style status line for Claude Code (piped a JSON
payload on stdin after every assistant turn) and an on-demand line for
Antigravity CLI / plain shell use.

Segments: host, dirgit, repo, ai, time — configurable via ~/.config/gsl/config.json`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// colorFlag and glyphsFlag override the terminal detection for render and
// status (plan F10): --color auto|never|16|256|truecolor, --glyphs auto|ascii.
var colorFlag, glyphsFlag string

func init() {
	rootCmd.PersistentFlags().StringVar(&colorFlag, "color", "auto", "colour: auto (NO_COLOR, TERM, COLORTERM), never, 16, 256 or truecolor")
	rootCmd.PersistentFlags().StringVar(&glyphsFlag, "glyphs", "auto", "glyphs: auto (the style's) or ascii")
}

// renderFlags validates --color / --glyphs: the colour mode (from the
// environment when auto) and whether to force ASCII glyphs.
func renderFlags(env func(string) string) (style.ColorMode, bool, error) {
	if _, err := style.ParseColorMode(colorFlag); err != nil {
		return style.ModeAuto, false, err
	}
	switch glyphsFlag {
	case "", "auto":
	case "ascii":
	default:
		return style.ModeAuto, false, fmt.Errorf("--glyphs: %q is not one of auto, ascii", glyphsFlag)
	}
	return style.DetectColorMode(env, colorFlag), glyphsFlag == "ascii", nil
}

// Execute runs the root command. Called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
