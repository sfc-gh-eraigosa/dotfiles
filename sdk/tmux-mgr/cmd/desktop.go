package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "desktop [list|switch] [target]",
		Short: "Navigate between windows",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 || args[0] == "list" {
				out, _ := Tmgr.Run("list-windows", "-F", "#I: #W")
				fmt.Print(out)
				return
			}
			target := args[0]
			if args[0] == "switch" && len(args) > 1 {
				target = args[1]
			}
			if !runTmux("select-window", "-t", target) {
				return
			}
			log.Printf("Switched to window %s", target)
		},
	})
}
