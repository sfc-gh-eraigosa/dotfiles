package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/version"
	"github.com/spf13/cobra"
)

var versionJSON bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Args:  cobra.NoArgs,
	RunE: func(c *cobra.Command, _ []string) error {
		v := version.Get()
		if versionJSON {
			b, _ := json.Marshal(map[string]string{"version": v.Version, "commit": v.Commit, "build_date": v.BuildDate, "dirty": v.Dirty})
			fmt.Fprintln(c.OutOrStdout(), string(b))
			return nil
		}
		fmt.Fprintf(c.OutOrStdout(), "gdash v%s\n  Commit:      %s\n  Build Date:  %s\n  Dirty:       %s\n", v.Version, v.Commit, v.BuildDate, v.Dirty)
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "print as JSON")
	rootCmd.AddCommand(versionCmd)
}
