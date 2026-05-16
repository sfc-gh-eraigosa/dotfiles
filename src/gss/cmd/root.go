package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var repoPath string

var rootCmd = &cobra.Command{
	Use:   "gss",
	Short: "gss is a Git Safe Sync tool",
	Long:  "A specialized tool for safely synchronizing and pushing changes to git repositories with automated backups.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&repoPath, "repo", "r", ".", "Path to the git repository")
}

func getRepoPath() string {
	return repoPath
}

func getDefaultBranch() string {
	// Try to detect the default branch (main or master)
	return "main" // Simplified for now, can be improved with 'git remote show origin'
}
