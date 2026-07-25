package cmd

import (
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/overrides"
	"github.com/spf13/cobra"
)

var unsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "Remove a key from the user override file, restoring its default",
	Args:  cobra.ExactArgs(1),
	RunE:  runUnset,
}

func init() {
	rootCmd.AddCommand(unsetCmd)
}

func runUnset(_ *cobra.Command, args []string) error {
	r, err := newResolver()
	if err != nil {
		return err
	}
	return overrides.Unset(r.P, args[0])
}
