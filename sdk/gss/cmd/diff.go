package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show changes in the dotfiles repository",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := exec.Command("git", "diff").CombinedOutput()
		if err != nil {
			fmt.Printf("Error running git diff: %v\n", err)
			return
		}
		if len(out) == 0 {
			fmt.Println("No unstaged changes.")
		} else {
			fmt.Println(string(out))
		}

		out, err = exec.Command("git", "diff", "--cached").CombinedOutput()
		if err != nil {
			fmt.Printf("Error running git diff --cached: %v\n", err)
			return
		}
		if len(out) > 0 {
			fmt.Println("\nStaged changes:")
			fmt.Println(string(out))
		}
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
