package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/backup"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a local safety branch of current changes",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()

		// Delegate to internal/backup (PR-11). The branch name (backup/gss-
		// <timestamp>) is byte-identical to the classic command; Create now
		// also appends a monotonic -N suffix if the name collides, so a rerun
		// in the same second succeeds instead of failing.
		name, err := backup.NewService(git.NewSystemRunner(), config.SystemClock{}).Create(context.Background(), path)
		if err != nil {
			fmt.Printf("Error creating backup branch: %s\n", err)
			return
		}
		fmt.Printf("Creating safety backup branch in %s: %s\n", path, name)
		fmt.Println("Backup branch created successfully.")
	},
}

func init() {
	rootCmd.AddCommand(backupCmd)
}
