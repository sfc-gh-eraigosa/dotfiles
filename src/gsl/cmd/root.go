package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gsl",
	Short: "gsl is a Go Status Line tool",
	Long: `gsl renders a powerline-style status line for Claude Code (piped a JSON
payload on stdin after every assistant turn) and an on-demand line for
Gemini/CLI.

Segments: dirgit, repo, ai, time — configurable via ~/.config/gsl/config.json`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command. Called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
