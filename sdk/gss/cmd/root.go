package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

// getDefaultBranch detects the remote default branch (main, master, etc).
// Uses the local symbolic-ref cache first (no network); falls back to
// `git remote show origin` if the ref is not yet initialised.
func getDefaultBranch() string {
	path := getRepoPath()

	out, err := exec.Command("git", "-C", path, "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		if idx := strings.LastIndex(ref, "/"); idx >= 0 {
			return ref[idx+1:]
		}
	}

	out, err = exec.Command("git", "-C", path, "remote", "show", "origin").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "HEAD branch:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "HEAD branch:"))
			}
		}
	}

	return "main"
}
