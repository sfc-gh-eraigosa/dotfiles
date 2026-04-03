package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "restore [name]",
		Short: "Restore a saved window layout",
		Run: func(cmd *cobra.Command, args []string) {
			name := "default"
			if len(args) > 0 {
				name = args[0]
			}
			if err := Tmgr.RestoreLayout(name); err != nil {
				log.Printf("Error restoring layout: %v", err)
				fmt.Printf("Error restoring layout %s\n", name)
				return
			}
			fmt.Printf("Layout %s restored\n", name)
		},
	})
}
