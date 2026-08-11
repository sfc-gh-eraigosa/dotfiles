package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	flagConfig string
	flagMarker string
	flagRepo   string
	flagJSON   bool
)

var rootCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Report and manage dotfiles install status across your hosts",
}

// Execute runs the root command. An exitError carries a deliberate exit code
// (e.g. `status` finding a stale host) and is NOT an operational failure, so
// it exits quietly; anything else prints as an error.
func Execute() {
	err := rootCmd.Execute()
	if err == nil {
		return
	}
	var ee exitError
	if errors.As(err, &ee) {
		// A deliberate exit code (e.g. `status` found a stale host). Not an
		// operational failure, so nothing is printed.
		os.Exit(ee.code)
	}
	// SilenceErrors is set on commands that return exitError, so real errors
	// would otherwise vanish. Print them here.
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

func init() {
	home, _ := os.UserHomeDir()
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", filepath.Join(home, ".ssh", "config"), "ssh config path")
	rootCmd.PersistentFlags().StringVar(&flagMarker, "marker", "#fleet", "comment marking a host as in-fleet")
	rootCmd.PersistentFlags().StringVar(&flagRepo, "repo", filepath.Join(home, "git", "dotfiles"), "local dotfiles repo used for the baseline")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable output")
}
