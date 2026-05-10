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
		if currentBranch == "" {
			currentBranch = "main"
		}

		// Get remote URL to construct diff link
		remoteOut, _ := exec.Command("git", "-C", path, "remote", "get-url", "origin").Output()
		remoteURL := strings.TrimSpace(string(remoteOut))
		remoteURL = strings.TrimSuffix(remoteURL, ".git")
		// Convert SSH to HTTPS if necessary
		remoteURL = strings.Replace(remoteURL, "git@github.com:", "https://github.com/", 1)

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

		// 4. Generate Diff Link
		if strings.Contains(remoteURL, "github.com") {
			fmt.Printf("\nView changes on GitHub: %s/compare/%s...%s\n", remoteURL, "main", currentBranch)
			// Note: 'main' is used as base for simplicity, could be refined to detect default branch
		}
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
