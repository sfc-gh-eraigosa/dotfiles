package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wenlock/dotfiles/gss/internal/git"
	"github.com/wenlock/dotfiles/gss/internal/status"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the repository",
	Run: func(cmd *cobra.Command, args []string) {
		path := getRepoPath()
		// Delegate to internal/status (PR-14): same git status --porcelain
		// rendering, now unit-tested and byte-identical to the classic output.
		out, err := status.NewService(git.NewSystemRunner()).Status(context.Background(), path)
		if err != nil {
			fmt.Printf("Error running git status in %s: %v\n", path, err)
			return
		}
		fmt.Print(out)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
