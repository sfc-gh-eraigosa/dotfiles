// Package cmd is gdash's CLI.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gdash [view]",
	Short: "Grafana dashboards as terminal graphs",
	Long: `gdash draws a Grafana dashboard's panels in the terminal: braille line
charts for time series and htop-style meters for single values, refreshed in
place. A view (see "gdash config") names a Grafana profile and a dashboard.`,
	SilenceUsage: true,
}

// Execute runs the CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
