package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// mustGit runs a git command in dir and fails the test if it does not succeed.
// These setup commands used to run unchecked, so a failed `git init` or
// `git config` surfaced later as a confusing assertion failure about
// dirtiness rather than a clear setup error.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("setup: git %v: %v\n%s", args, err, out)
	}
}

func TestIsDirty(t *testing.T) {
	// t.TempDir cleans itself up when the test finishes.
	tempDir := t.TempDir()

	// Initialize a git repo
	mustGit(t, tempDir, "init")
	// Configure git for the temp repo to avoid failures due to missing global config
	mustGit(t, tempDir, "config", "user.email", "test@example.com")
	mustGit(t, tempDir, "config", "user.name", "Test User")
	mustGit(t, tempDir, "config", "commit.gpgsign", "false")

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
	mustGit(t, tempDir, "add", ".")
	if err := exec.Command("git", "-C", tempDir, "commit", "-m", "test").Run(); err != nil {
		out, _ := exec.Command("git", "-C", tempDir, "status").CombinedOutput()
		t.Fatalf("Failed to commit: %v. Status:\n%s", err, string(out))
	}

	if isDirty(tempDir) {
		out, _ := exec.Command("git", "-C", tempDir, "status", "--porcelain").CombinedOutput()
		t.Errorf("Expected clean repo after commit, but isDirty returned true. Status:\n%s", string(out))
	}
}
