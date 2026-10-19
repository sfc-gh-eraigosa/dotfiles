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
	Args:         cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runView(args)
	},
}

func init() {
	f := rootCmd.Flags()
	f.StringVar(&vf.dashboard, "dashboard", "", "draw this dashboard (uid or exact title) instead of a named view")
	f.StringVar(&vf.profile, "profile", "", "Grafana profile (default: the view's, else default_profile)")
	f.StringVar(&vf.rng, "range", "", "time range, e.g. 15m, 1h, 7d")
	f.StringVar(&vf.refresh, "refresh", "", "refresh interval, e.g. 30s (floor 5s)")
}

// Execute runs the CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
