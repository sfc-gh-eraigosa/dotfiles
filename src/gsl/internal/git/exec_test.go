package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/git"
	"github.com/wenlock/dotfiles/gsl/internal/observe"
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
	// A bogus subcommand always exits non-zero regardless of environment.
	out, err := r.Run(context.Background(), "definitely-not-a-command")
	if err == nil {
		t.Fatal("expected error from bogus git subcommand, got nil")
	}
	_ = out // output may be non-empty; just confirm it doesn't panic
}

// TestSystemRunner_LogsOnFailure asserts the seam emits a structured
// git.subprocess_error record when the underlying command fails. The gh/mcp
// seams follow the same pattern; this covers the shared mechanism.
func TestSystemRunner_LogsOnFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gsl.log")
	t.Setenv("GSL_LOG_FILE", logPath)
	observe.ResetDefaultForTest()
	t.Cleanup(observe.ResetDefaultForTest)

	r := git.NewSystemRunner()
	_, err := r.Run(context.Background(), "definitely-not-a-command")
	if err == nil {
		t.Fatal("expected error from bogus git subcommand")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), `"event":"git.subprocess_error"`) {
		t.Fatalf("expected git.subprocess_error in log, got: %s", data)
	}
}
