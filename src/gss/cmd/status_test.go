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

	// Case 1: Clean repo (initially empty)
	if isDirty(tempDir) {
		t.Error("Expected clean repo initially, but isDirty returned true")
	}

	// Case 2: Dirty repo (new untracked file)
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	if !isDirty(tempDir) {
		t.Error("Expected dirty repo after creating file, but isDirty returned false")
	}

	// Case 3: Clean repo again (staged and committed)
	exec.Command("git", "-C", tempDir, "add", ".").Run()
	exec.Command("git", "-C", tempDir, "commit", "-m", "test").Run()

	if isDirty(tempDir) {
		t.Error("Expected clean repo after commit, but isDirty returned true")
	}
}
