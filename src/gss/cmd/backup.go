package cmd

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a local safety branch of current changes",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()
		timestamp := time.Now().Format("20060102-150405")
		branchName := fmt.Sprintf("backup/gss-%s", timestamp)

		fmt.Printf("Creating safety backup branch in %s: %s\n", path, branchName)
		
		out, err := exec.Command("git", "-C", path, "branch", branchName).CombinedOutput()
		if err != nil {
			fmt.Printf("Error creating backup branch: %s\n", string(out))
			return
		}
		fmt.Println("Backup branch created successfully.")
	},
}

func init() {
	rootCmd.AddCommand(backupCmd)
}
