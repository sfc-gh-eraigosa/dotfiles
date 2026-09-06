// Package cmd wires the ghapp CLI verbs. Each verb lives in its own file and
// self-registers on the root command via init() + rootCmd.AddCommand, the
// same pattern sdk/gff uses. Exit-code mapping lives only in main.go.
package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

// ErrUsage marks a usage / no-credential error; main maps it to exit 2.
var ErrUsage = errors.New("usage")

var rootCmd = newRoot()

func newRoot() *cobra.Command {
	return &cobra.Command{
		Use:           "ghapp",
		Short:         "GitHub App credential toolkit — create an App by manifest, mint installation tokens",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Bare `ghapp` prints help; there is no TUI.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

// NewRootCmd returns the root command with every registered verb attached.
// Tests build fresh buffers on it; main uses the shared rootCmd via Execute.
func NewRootCmd() *cobra.Command {
	return rootCmd
}

// Execute runs the root command. Called by main.
func Execute() error {
	return rootCmd.Execute()
}
