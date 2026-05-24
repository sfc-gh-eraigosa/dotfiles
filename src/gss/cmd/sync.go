package cmd

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/git"
	"github.com/wenlock/dotfiles/gss/internal/sync"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull latest changes and rebase if necessary",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()

		fmt.Printf("Fetching latest changes for %s...\n", path)

		// Delegate to internal/sync (PR-12): fetch origin then
		// pull --rebase. A rebase failure is the ErrRebaseConflict sentinel;
		// anything else is a fetch/transport error.
		res, err := sync.NewService(git.NewSystemRunner()).Sync(context.Background(), path)
		if err != nil {
			if stderrors.Is(err, errors.ErrRebaseConflict) {
				fmt.Printf("Conflict detected or error during rebase:\n%v\n", err)
				fmt.Println("\nPlease resolve conflicts manually or use 'git rebase --abort'.")
			} else {
				fmt.Printf("Error fetching: %v\n", err)
			}
			return
		}
		fmt.Printf("Successfully synchronized %s with origin/%s.\n", path, res.Branch)
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
