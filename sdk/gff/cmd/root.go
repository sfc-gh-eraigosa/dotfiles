// Package cmd wires the gff CLI verbs. Each verb lives in its own file and
// self-registers on the root command via init() + rootCmd.AddCommand, the
// same pattern sdk/gsl uses.
package cmd

import (
	"fmt"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/gitx"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// sourceFlag holds the value of the persistent --source flag.
var sourceFlag string

// newResolver is the test seam: tests swap this to inject a pre-built Resolver
// over temp-world fixtures. Production code uses defaultResolver.
var newResolver = defaultResolver

// defaultResolver builds a Resolver from Default() paths wired to the real registry.
func defaultResolver() (*resolve.Resolver, error) {
	p, err := paths.Default()
	if err != nil {
		return nil, fmt.Errorf("gff: resolving paths: %w", err)
	}
	r := resolve.New(p, gitx.ExecRunner{}, sourceFlag)
	r.S = &registry.Registry{P: p}
	return r, nil
}

var rootCmd = newRoot()

func newRoot() *cobra.Command {
	c := &cobra.Command{
		Use:           "gff",
		Short:         "git fast features — layered feature flags persisted in git",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Bare `gff` with no args on a TTY runs the TUI; non-TTY prints help.
		RunE: func(cmd *cobra.Command, args []string) error {
			if term.IsTerminal(int(os.Stdout.Fd())) {
				return runTUI(cmd, args)
			}
			return cmd.Help()
		},
	}
	c.PersistentFlags().StringVar(&sourceFlag, "source", "", "resolve flags from a registered source name or local repo path")
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
