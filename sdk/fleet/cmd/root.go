package cmd

import (
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

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	home, _ := os.UserHomeDir()
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", filepath.Join(home, ".ssh", "config"), "ssh config path")
	rootCmd.PersistentFlags().StringVar(&flagMarker, "marker", "#fleet", "comment marking a host as in-fleet")
	rootCmd.PersistentFlags().StringVar(&flagRepo, "repo", filepath.Join(home, "git", "dotfiles"), "local dotfiles repo used for the baseline")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "machine-readable output")
}
