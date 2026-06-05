package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsDirty(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "gss-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize a git repo
	exec.Command("git", "-C", tempDir, "init").Run()
	// Configure git for the temp repo to avoid failures due to missing global config
	exec.Command("git", "-C", tempDir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", tempDir, "config", "user.name", "Test User").Run()
	exec.Command("git", "-C", tempDir, "config", "commit.gpgsign", "false").Run()

	// Case 1: Clean repo (initially empty)
	if isDirty(tempDir) {
		out, _ := exec.Command("git", "-C", tempDir, "status", "--porcelain").CombinedOutput()
		t.Errorf("Expected clean repo initially, but isDirty returned true. Status:\n%s", string(out))
	}

	// Case 2: Dirty repo (new untracked file)
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	if !isDirty(tempDir) {
		out, _ := exec.Command("git", "-C", tempDir, "status", "--porcelain").CombinedOutput()
		t.Errorf("Expected dirty repo after creating file, but isDirty returned false. Status:\n%s", string(out))
	}

	// Case 3: Clean repo again (staged and committed)
	exec.Command("git", "-C", tempDir, "add", ".").Run()
	if err := exec.Command("git", "-C", tempDir, "commit", "-m", "test").Run(); err != nil {
		out, _ := exec.Command("git", "-C", tempDir, "status").CombinedOutput()
		t.Fatalf("Failed to commit: %v. Status:\n%s", err, string(out))
	}

	if isDirty(tempDir) {
		out, _ := exec.Command("git", "-C", tempDir, "status", "--porcelain").CombinedOutput()
		t.Errorf("Expected clean repo after commit, but isDirty returned true. Status:\n%s", string(out))
	}
}
