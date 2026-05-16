package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull latest changes and rebase if necessary",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()
		
		// Get current branch
		branchOut, _ := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output()
		currentBranch := strings.TrimSpace(string(branchOut))
		if currentBranch == "" { currentBranch = "main" }

		fmt.Printf("Fetching latest changes for %s...\n", path)
		if out, err := exec.Command("git", "-C", path, "fetch", "origin").CombinedOutput(); err != nil {
			fmt.Printf("Error fetching: %s\n", string(out))
			return
		}

		fmt.Printf("Attempting to pull/rebase onto %s...\n", currentBranch)
		out, err := exec.Command("git", "-C", path, "pull", "--rebase", "origin", currentBranch).CombinedOutput()
		if err != nil {
			fmt.Printf("Conflict detected or error during rebase:\n%s\n", string(out))
			fmt.Println("\nPlease resolve conflicts manually or use 'git rebase --abort'.")
			return
		}
		fmt.Printf("Successfully synchronized %s with origin/%s.\n", path, currentBranch)
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
