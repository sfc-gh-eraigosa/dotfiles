package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Injected at build time via -ldflags (see build.sh). Names must stay
// exported and match the -X paths in build.sh or the injection silently
// no-ops and every binary reports "dev".
var (
	Version   = "dev"
	Commit    = "none"
	Dirty     = "false"
	BuildDate = "unknown"
)

func versionString() string { return fmt.Sprintf("fleet %s (%s)", Version, Commit) }

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, _ []string) {
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, versionString())
		fmt.Fprintf(out, "  Dirty:      %s\n", Dirty)
		fmt.Fprintf(out, "  Build Date: %s\n", BuildDate)
	},
}

func init() { rootCmd.AddCommand(versionCmd) }
