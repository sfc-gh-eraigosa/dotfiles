package git_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
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
//
// The level is Debug (not Warn) because a non-zero exit here is the ROUTINE
// case — see TestSystemRunner_ExpectedExit_LogsAtDebug for why that matters,
// and TestSystemRunner_MissingBinary_LogsAtWarn for the failures that stay
// loud. Hence GSL_LOG_LEVEL=debug: the record still exists, it is just no
// longer shouted.
func TestSystemRunner_LogsOnFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gsl.log")
	t.Setenv("GSL_LOG_FILE", logPath)
	t.Setenv("GSL_LOG_LEVEL", "debug")
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

// ─── E18 / F12 — subprocess containment ──────────────────────────────────────

// deadlineBudget is the wall-clock ceiling for a Run whose child has exited but
// whose GRANDCHILD still holds the stdout pipe open:
//
//	ctx deadline (800ms) + WaitDelay (200ms) + slack (500ms)
//
// The slack absorbs process spawn + scheduler jitter on a loaded CI box. It is
// nowhere near the 5s the leaked grandchild sleeps for, so the assertion cannot
// pass by accident.
const (
	e18Deadline = 800 * time.Millisecond
	e18Budget   = 1500 * time.Millisecond
	// e18Script exits IMMEDIATELY but backgrounds a grandchild that inherits
	// (and holds open) the stdout pipe for 5s. Because SystemRunner points
	// cmd.Stdout at a *bytes.Buffer, os/exec allocates an os.Pipe and Wait()
	// blocks until EVERY writer closes it — the grandchild included. Without
	// WaitDelay + a process-group kill, Wait blocks the full 5s and the
	// context deadline is DEFEATED.
	e18Script = "( sleep 5 ) & exit 0"
)

// TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe is E18 for the git seam.
func TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics; not applicable on Windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), e18Deadline)
	defer cancel()

	// Path is the shell, so name+args become `sh -c "<script>"`.
	r := &git.SystemRunner{Path: "/bin/sh"}

	start := time.Now()
	_, _ = r.Run(ctx, "-c", e18Script)
	elapsed := time.Since(start)

	if elapsed > e18Budget {
		t.Fatalf("git.Run took %v with an %v deadline; want <= %v. "+
			"The orphaned grandchild is holding the stdout pipe and Wait() is "+
			"blocking on it — the deadline is not being enforced (E18/F12).",
			elapsed.Round(time.Millisecond), e18Deadline, e18Budget)
	}
	t.Logf("git.Run returned in %v (deadline %v, budget %v)",
		elapsed.Round(time.Millisecond), e18Deadline, e18Budget)
}

// TestRun_KillsProcessGroup_OnContextCancel proves the kill targets the whole
// process GROUP, not just the direct child.
//
// The child (sh) blocks for 30s, so ctx cancellation is what ends it. Its
// backgrounded grandchild sleeps 1s and then touches a marker file. With only
// the direct child killed (os/exec's default Cancel), the grandchild survives
// its parent and creates the marker. With Setpgid + a negative-pid kill, the
// grandchild dies with the group and the marker is NEVER created.
func TestRun_KillsProcessGroup_OnContextCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process groups; not applicable on Windows")
	}

	marker := filepath.Join(t.TempDir(), "grandchild-survived")
	script := "( sleep 1; touch " + marker + " ) & sleep 30"

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	r := &git.SystemRunner{Path: "/bin/sh"}
	start := time.Now()
	_, err := r.Run(ctx, "-c", script)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected an error from the cancelled command, got nil")
	}
	if elapsed > e18Budget {
		t.Fatalf("Run took %v after a 300ms cancel; want <= %v", elapsed.Round(time.Millisecond), e18Budget)
	}

	// Give the (hopefully dead) grandchild well past its 1s sleep to prove it
	// is gone rather than merely slow.
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("grandchild survived context cancellation and wrote %s: "+
			"only the direct child was killed. Cancel must kill the PROCESS GROUP (E18/F12).", marker)
	}
}

// TestSystemRunner_ExpectedExit_LogsAtDebug pins the logging-hygiene rule: an
// EXPECTED non-zero exit (not-a-repo, no-PR) is Debug, not Warn. Warn is
// reserved for genuine operational failures (timeout, missing binary) so that
// real records are not drowned by ~1.6k routine ones.
func TestSystemRunner_ExpectedExit_LogsAtDebug(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gsl.log")
	t.Setenv("GSL_LOG_FILE", logPath)
	t.Setenv("GSL_LOG_LEVEL", "debug")
	observe.ResetDefaultForTest()
	t.Cleanup(observe.ResetDefaultForTest)

	r := git.NewSystemRunner()
	if _, err := r.Run(context.Background(), "definitely-not-a-command"); err == nil {
		t.Fatal("expected error from bogus git subcommand")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"event":"git.subprocess_error"`) {
		t.Fatalf("expected git.subprocess_error in log, got: %s", got)
	}
	if !strings.Contains(got, `"level":"debug"`) {
		t.Errorf("expected the expected-exit record at level=debug; got: %s", got)
	}
	if strings.Contains(got, `"level":"warning"`) {
		t.Errorf("expected non-zero exit must NOT be logged at warn; got: %s", got)
	}
}

// TestSystemRunner_MissingBinary_LogsAtWarn is the counterweight to the Debug
// demotion: demoting the ROUTINE exits must not silence the REAL ones. A missing
// binary is a genuine misconfiguration and must still be visible at the DEFAULT
// log level (no GSL_LOG_LEVEL set — i.e. info), which Debug records are not.
func TestSystemRunner_MissingBinary_LogsAtWarn(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gsl.log")
	t.Setenv("GSL_LOG_FILE", logPath)
	// Deliberately NOT setting GSL_LOG_LEVEL: the default (info) must show this.
	observe.ResetDefaultForTest()
	t.Cleanup(observe.ResetDefaultForTest)

	r := &git.SystemRunner{Path: "gsl-definitely-not-a-real-binary-xyz"}
	if _, err := r.Run(context.Background(), "status"); err == nil {
		t.Fatal("expected a lookup error for a missing binary")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"level":"warning"`) {
		t.Errorf("a missing binary must still be logged at Warn (visible at the default "+
			"level); got: %q", got)
	}
}
