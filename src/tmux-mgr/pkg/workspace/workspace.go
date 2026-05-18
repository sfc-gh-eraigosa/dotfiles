package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	worktreesDir = ".config/tmux-mgr/worktrees"
)

// GetWorktreesDir returns the absolute path to the worktrees directory.
func GetWorktreesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, worktreesDir), nil
}

// CreateWorkspace sets up a new git worktree for an agent session.
func CreateWorkspace(agentName string) (string, string, error) {
	worktreesPath, err := GetWorktreesDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get worktrees directory: %w", err)
	}

	sessionID := fmt.Sprintf("%s-%d", agentName, time.Now().UnixNano())
	workspacePath := filepath.Join(worktreesPath, sessionID)

	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Get the root of the current git repository
	gitRootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	gitRootBytes, err := gitRootCmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to find git repository root: %w", err)
	}
	gitRoot := string(gitRootBytes)
	gitRoot = gitRoot[:len(gitRoot)-1] // remove trailing newline

	// Create the worktree
	// We are creating a new branch for the worktree.
	cmd := exec.Command("git", "worktree", "add", "-b", sessionID, workspacePath)
	cmd.Dir = gitRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("failed to create git worktree: %w, Output: %s", err, string(output))
	}

	fmt.Printf("Created worktree at: %s\n", workspacePath)
	return workspacePath, sessionID, nil
}
