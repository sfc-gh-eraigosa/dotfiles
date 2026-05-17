package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var forceAutonomous bool

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Safely push changes to origin",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()

		// 0. Handshake Verification (Technical Safeguard)
		if !forceAutonomous {
			home, _ := os.UserHomeDir()
			configDir := filepath.Join(home, ".config", "gss")
			approvalPath := filepath.Join(configDir, "approval.token")
			
			// Get current SHA to compare
			shaOut, _ := exec.Command("git", "-C", path, "rev-parse", "HEAD").Output()
			currentSHA := strings.TrimSpace(string(shaOut))

			content, err := os.ReadFile(approvalPath)
			if err != nil {
				fmt.Println("Error: Missing or invalid AI approval token. The agent must obtain user permission before pushing.")
				return
			}
			
			if strings.TrimSpace(string(content)) != currentSHA {
				fmt.Printf("Error: Invalid AI approval token (Expected %s, but token file was different).\n", currentSHA)
				return
			}
			
			// Consume token
			os.Remove(approvalPath)
		}

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

		// 4. Generate Summary and Diff Link
		if oldRemoteSHA != "" && newSHA != "" && oldRemoteSHA != newSHA {
			// Get stats
			statsOut, _ := exec.Command("git", "-C", path, "diff", "--stat", oldRemoteSHA, newSHA).Output()
			statsLines := strings.Split(strings.TrimSpace(string(statsOut)), "\n")
			
			if len(statsLines) > 0 {
				fmt.Printf("\nSummary of changes (%d files):\n", len(statsLines)-1)
				if len(statsLines)-1 < 10 {
					// Show detailed list if < 10 files
					fmt.Println(string(statsOut))
				} else {
					// Just show the final line (e.g., "15 files changed, 100 insertions(+), 50 deletions(-)")
					fmt.Println(statsLines[len(statsLines)-1])
				}
			}

			if strings.Contains(remoteURL, "github.com") {
				fmt.Printf("\nView changes on GitHub: %s/compare/%s...%s\n", remoteURL, oldRemoteSHA, newSHA)
			}
		}
	},
}

func init() {
	pushCmd.Flags().BoolVar(&forceAutonomous, "force-autonomous", false, "Skip the interactive confirmation prompt (Dangerous)")
	rootCmd.AddCommand(pushCmd)
}
