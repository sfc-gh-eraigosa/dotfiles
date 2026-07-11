package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "save [name]",
		Short: "Save current window layout",
		Run: func(cmd *cobra.Command, args []string) {
			name := "default"
			if len(args) > 0 {
				name = args[0]
			}
			if err := Tmgr.SaveLayout(name); err != nil {
				log.Printf("Error saving layout: %v", err)
				fmt.Printf("Error saving layout %s\n", name)
				return
			}
			fmt.Printf("Layout %s saved\n", name)
		},
	})
}
