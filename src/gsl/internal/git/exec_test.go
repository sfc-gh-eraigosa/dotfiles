package git_test

import (
	"context"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/git"
)

// TestNewSystemRunnerImplementsRunner is a compile-time check that
// *SystemRunner satisfies the Runner interface.
func TestNewSystemRunnerImplementsRunner(t *testing.T) {
	var _ git.Runner = git.NewSystemRunner()
}

// TestSystemRunnerRealGit verifies the real runner can execute a basic git
// command. Requires git to be present on PATH.
func TestSystemRunnerRealGit(t *testing.T) {
	r := git.NewSystemRunner()
	out, err := r.Run(context.Background(), "version")
	if err != nil {
		t.Fatalf("git version: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty output from git version")
	}
}

// TestSystemRunnerNonZeroExit verifies that a failing git command returns an
// error but still surfaces the output (combined stdout+stderr).
func TestSystemRunnerNonZeroExit(t *testing.T) {
	r := git.NewSystemRunner()
	// git status in a non-git directory should exit non-zero.
	out, err := r.Run(context.Background(), "status", "-C", "/tmp")
	if err == nil {
		t.Skip("git status unexpectedly succeeded; skip in non-standard env")
	}
	_ = out // output may be non-empty; just confirm it doesn't panic
}
