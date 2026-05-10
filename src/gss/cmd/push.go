package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Safely push changes to origin",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()

		// Get current branch
		branchOut, _ := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output()
		currentBranch := strings.TrimSpace(string(branchOut))
		if currentBranch == "" { currentBranch = "main" }

		// 1. Safety Backup
		fmt.Println("Step 1: Creating safety backup...")
		backupCmd.Run(cmd, []string{})

		// 2. Sync
		fmt.Printf("\nStep 2: Syncing %s with origin/%s...\n", path, currentBranch)
		syncCmd.Run(cmd, []string{})

		// 3. Push
		fmt.Printf("\nStep 3: Pushing %s to origin/%s...\n", path, currentBranch)
		out, err := exec.Command("git", "-C", path, "push", "origin", currentBranch).CombinedOutput()
		if err != nil {
			fmt.Printf("Error pushing: %s\n", string(out))
			return
		}
		fmt.Println("Successfully pushed changes!")
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
