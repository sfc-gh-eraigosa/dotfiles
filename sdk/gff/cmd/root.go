// Package cmd wires the gff CLI verbs. Each verb lives in its own file and
// self-registers on the root command via init() + rootCmd.AddCommand, the
// same pattern sdk/gsl uses.
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = newRoot()

func newRoot() *cobra.Command {
	c := &cobra.Command{
		Use:           "gff",
		Short:         "git fast features — layered feature flags persisted in git",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return c
}

// NewRootCmd returns the root command with every registered verb attached.
// Tests build fresh buffers on it; main uses the shared rootCmd via Execute.
func NewRootCmd() *cobra.Command {
	return rootCmd
}

// Execute runs the root command. Called by main; error mapping to process
// exit codes lives only in main.go.
func Execute() error {
	return rootCmd.Execute()
}
