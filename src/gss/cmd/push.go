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
		remoteURL = strings.Replace(remoteURL, "git@github.com:", "https://github.com/", 1)

		// Get current remote SHA before we push
		exec.Command("git", "-C", path, "fetch", "origin").Run()
		remoteSHAOut, _ := exec.Command("git", "-C", path, "rev-parse", fmt.Sprintf("origin/%s", currentBranch)).Output()
		oldRemoteSHA := strings.TrimSpace(string(remoteSHAOut))

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

		// Get new local SHA after push
		newSHAOut, _ := exec.Command("git", "-C", path, "rev-parse", "HEAD").Output()
		newSHA := strings.TrimSpace(string(newSHAOut))

		fmt.Println("Successfully pushed changes!")

		// 4. Generate Diff Link
		if strings.Contains(remoteURL, "github.com") {
			if oldRemoteSHA != "" && newSHA != "" && oldRemoteSHA != newSHA {
				fmt.Printf("\nView changes on GitHub: %s/compare/%s...%s\n", remoteURL, oldRemoteSHA, newSHA)
			} else {
				// Fallback if SHAs match or failed to retrieve
				fmt.Printf("\nView changes on GitHub: %s/compare/%s...%s\n", remoteURL, "main", currentBranch)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
