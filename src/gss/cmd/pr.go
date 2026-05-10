package cmd

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Create a branch and a pull request",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()
		timestamp := time.Now().Format("20060102-150405")
		branchName := fmt.Sprintf("feature/gss-%s", timestamp)

		fmt.Printf("Creating feature branch in %s: %s\n", path, branchName)
		if out, err := exec.Command("git", "-C", path, "checkout", "-b", branchName).CombinedOutput(); err != nil {
			fmt.Printf("Error creating branch: %s\n", string(out))
			return
		}

		fmt.Println("Pushing feature branch to origin...")
		if out, err := exec.Command("git", "-C", path, "push", "-u", "origin", branchName).CombinedOutput(); err != nil {
			fmt.Printf("Error pushing branch: %s\n", string(out))
			return
		}

		fmt.Println("Attempting to create Pull Request via 'gh' CLI...")
		if _, err := exec.LookPath("gh"); err == nil {
			// Using -C for gh as well if supported, otherwise cd
			c := exec.Command("gh", "pr", "create", "--fill")
			c.Dir = path
			out, err := c.CombinedOutput()
			if err != nil {
				fmt.Printf("Error creating PR: %s\n", string(out))
			} else {
				fmt.Println("Pull Request created successfully!")
			}
		} else {
			fmt.Println("'gh' CLI not found. Please create the PR manually on GitHub.")
		}
	},
}

func init() {
	rootCmd.AddCommand(prCmd)
}
