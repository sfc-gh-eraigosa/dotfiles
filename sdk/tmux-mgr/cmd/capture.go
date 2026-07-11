package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "capture [target]",
		Short: "Capture pane content",
		Run: func(cmd *cobra.Command, args []string) {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			out, err := Tmgr.Capture(target)
			if err != nil {
				log.Printf("Error capturing pane %s: %v", target, err)
				fmt.Println("Error capturing pane")
				return
			}
			fmt.Print(out)
		},
	})
}
