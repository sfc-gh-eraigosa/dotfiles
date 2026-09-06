package cmd

import (
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), version.String())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
