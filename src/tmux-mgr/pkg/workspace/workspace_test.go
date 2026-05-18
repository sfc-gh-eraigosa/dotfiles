package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreateWorkspace(t *testing.T) {
	// Setup a temporary git repository to act as the project root
	tempRepoDir, err := os.MkdirTemp("", "testrepo-*")
	if err != nil {
		t.Fatalf("Failed to create temp repo dir: %v", err)
	}
	defer os.RemoveAll(tempRepoDir)

	// Initialize the git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tempRepoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git init: %v", err)
	}

	// Create an initial commit so we can branch off it
	exec.Command("git", "-C", tempRepoDir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", tempRepoDir, "config", "user.name", "Test User").Run()
	os.WriteFile(filepath.Join(tempRepoDir, "test.txt"), []byte("test"), 0644)
	exec.Command("git", "-C", tempRepoDir, "add", ".").Run()
	exec.Command("git", "-C", tempRepoDir, "commit", "-m", "init").Run()

	// Change working directory to the temp repo so CreateWorkspace finds it via rev-parse
	originalWd, _ := os.Getwd()
	os.Chdir(tempRepoDir)
	defer os.Chdir(originalWd)

	// Test positive case
	agentName := "testagent"
	workspacePath, sessionID, err := CreateWorkspace(agentName)
	if err != nil {
		t.Errorf("CreateWorkspace failed: %v", err)
	}

	// Verify outputs
	if sessionID == "" {
		t.Error("Expected a non-empty session ID")
	}
	if workspacePath == "" {
		t.Error("Expected a non-empty workspace path")
	}

	// Verify the directory exists
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		t.Errorf("Expected workspace directory to exist at %s", workspacePath)
	}

	// Verify it's a git worktree by checking for a .git file (worktrees use a file, not a dir)
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); os.IsNotExist(err) {
		t.Errorf("Expected .git file in worktree")
	}

	// Cleanup the created worktree directory so it doesn't pollute the user's home dir
	os.RemoveAll(workspacePath)
}

func TestGetWorktreesDir(t *testing.T) {
	dir, err := GetWorktreesDir()
	if err != nil {
		t.Errorf("GetWorktreesDir failed: %v", err)
	}
	if dir == "" {
		t.Error("Expected non-empty directory path")
	}
}
