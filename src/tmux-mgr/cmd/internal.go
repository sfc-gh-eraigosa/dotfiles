package cmd

import "github.com/spf13/cobra"

// internalCmd groups commands not meant for direct user interaction — shims,
// hooks, and migrators that tmux-mgr invokes on itself. Hidden from the main
// help so it does not clutter the user-facing surface.
var internalCmd = &cobra.Command{
	Use:    "internal",
	Short:  "Internal shims/hooks (not for direct use)",
	Hidden: true,
}

func init() {
	rootCmd.AddCommand(internalCmd)
}
