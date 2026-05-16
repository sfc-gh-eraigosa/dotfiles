package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the repository",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()
		out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
		if err != nil {
			fmt.Printf("Error running git status in %s: %v\n", path, err)
			return
		}
		lines := strings.Split(string(out), "\n")
		dirty := false
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				dirty = true
				break
			}
		}

		if !dirty {
			fmt.Printf("No changes detected in %s.\n", path)
			return
		}

		fmt.Printf("Changes in %s:\n", path)
		for _, line := range lines {
			if line == "" {
				continue
			}
			fmt.Printf(" - %s\n", line)
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
