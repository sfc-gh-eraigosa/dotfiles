package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Push current branch and open a pull request (or create a new feature branch if on default)",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()

		branchOut, _ := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output()
		currentBranch := strings.TrimSpace(string(branchOut))
		if currentBranch == "" {
			currentBranch = "main"
		}
		defaultBranch := getDefaultBranch()

		if currentBranch == defaultBranch {
			// On the default branch — create a timestamped feature branch first.
			timestamp := time.Now().Format("20060102-150405")
			currentBranch = fmt.Sprintf("feature/gss-%s", timestamp)
			fmt.Printf("On default branch; creating feature branch in %s: %s\n", path, currentBranch)
			if out, err := exec.Command("git", "-C", path, "checkout", "-b", currentBranch).CombinedOutput(); err != nil {
				fmt.Printf("Error creating branch: %s\n", string(out))
				return
			}
		} else {
			fmt.Printf("Using current branch: %s\n", currentBranch)
		}

		fmt.Printf("Pushing %s to origin...\n", currentBranch)
		if out, err := exec.Command("git", "-C", path, "push", "-u", "origin", currentBranch).CombinedOutput(); err != nil {
			fmt.Printf("Error pushing branch: %s\n", string(out))
			return
		}

		if _, err := exec.LookPath("gh"); err == nil {
			// Check for an existing PR first.
			viewCmd := exec.Command("gh", "pr", "view", "--json", "url", "-q", ".url")
			viewCmd.Dir = path
			prURL, err := viewCmd.CombinedOutput()
			prURLStr := strings.TrimSpace(string(prURL))
			if err == nil && prURLStr != "" {
				fmt.Printf("Pull Request already exists: %s\n", prURLStr)
				return
			}

			fmt.Println("Creating Pull Request via gh CLI...")
			createCmd := exec.Command("gh", "pr", "create", "--fill")
			createCmd.Dir = path
			out, err := createCmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Error creating PR: %s\n", string(out))
			} else {
				fmt.Printf("Pull Request created: %s", string(out))
			}
		} else {
			fmt.Println("'gh' CLI not found. Please create the PR manually on GitHub.")
		}
	},
}

func init() {
	rootCmd.AddCommand(prCmd)
}
