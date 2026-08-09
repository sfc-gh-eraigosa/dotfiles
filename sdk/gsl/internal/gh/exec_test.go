package gh_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh"
)

// TestNewSystemRunnerImplementsRunner is a compile-time check that
// *SystemRunner satisfies the Runner interface.
func TestNewSystemRunnerImplementsRunner(t *testing.T) {
	var _ gh.Runner = gh.NewSystemRunner()
}

// writeStubScript writes a tiny shell script that echoes the given line to
// stdout, the given line to stderr, then exits with the given code. It
// returns the script path. The test skips gracefully on platforms without a
// POSIX shell (e.g. Windows).
func writeStubScript(t *testing.T, stdoutLine, stderrLine string, exitCode int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub shell script not portable to windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no POSIX sh available: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "stub.sh")
	body := "#!/bin/sh\n" +
		"echo '" + stdoutLine + "'\n" +
		"echo '" + stderrLine + "' 1>&2\n" +
		"exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	_ = sh // presence already verified; script runs via its own shebang
	return path
}

// itoa avoids importing strconv for a single small conversion.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// TestSystemRunnerRunSuccess exercises the real SystemRunner.Run against a
// controlled stub binary: it verifies that stdout and stderr are both
// captured into the returned bytes and that a zero exit yields a nil error.
func TestSystemRunnerRunSuccess(t *testing.T) {
	script := writeStubScript(t, "hello-out", "hello-err", 0)

	r := &gh.SystemRunner{Path: script}
	out, err := r.Run(context.Background(), "ignored-subcommand", "arg1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "hello-out") {
		t.Errorf("stdout not captured; got %q", s)
	}
	if !strings.Contains(s, "hello-err") {
		t.Errorf("stderr not captured; got %q", s)
	}
}

// TestSystemRunnerRunNonZeroExit verifies that a non-zero exit returns BOTH
// the captured output bytes AND a non-nil error.
func TestSystemRunnerRunNonZeroExit(t *testing.T) {
	script := writeStubScript(t, "out-line", "err-line", 3)

	r := &gh.SystemRunner{Path: script}
	out, err := r.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected non-nil error for non-zero exit, got nil")
	}
	s := string(out)
	if !strings.Contains(s, "out-line") || !strings.Contains(s, "err-line") {
		t.Errorf("output not captured on failure; got %q", s)
	}
}

// TestSystemRunnerEmptyPathDefaultsToGh verifies the empty-Path fallback uses
// "gh". We don't require gh to be installed: if it's absent the invocation
// fails with an exec lookup error (proving the binary name resolved to "gh"),
// which is an acceptable, hermetic assertion.
func TestSystemRunnerEmptyPathDefaultsToGh(t *testing.T) {
	if _, err := exec.LookPath("gh"); err == nil {
		// gh exists on PATH; a benign subcommand should run without a
		// "executable file not found" style error.
		r := &gh.SystemRunner{Path: ""}
		_, err := r.Run(context.Background(), "--version")
		if err != nil && strings.Contains(err.Error(), "executable file not found") {
			t.Fatalf("empty Path did not fall back to gh: %v", err)
		}
		return
	}

	// gh is NOT installed: the empty-Path fallback must still attempt to run
	// "gh" and surface a lookup error mentioning gh — proving the default.
	r := &gh.SystemRunner{Path: ""}
	_, err := r.Run(context.Background(), "--version")
	if err == nil {
		t.Skip("gh unexpectedly runnable; cannot assert lookup-failure path")
	}
	if !strings.Contains(err.Error(), "gh") {
		t.Errorf("error did not reference the gh fallback binary; got %v", err)
	}
}

// ─── E18 / F12 — subprocess containment ──────────────────────────────────────

const (
	e18Deadline = 800 * time.Millisecond
	e18Budget   = 1500 * time.Millisecond
	// e18Script exits IMMEDIATELY but leaves a grandchild holding the inherited
	// stdout pipe for 5s. SystemRunner sets cmd.Stdout to a *bytes.Buffer, so
	// os/exec allocates an os.Pipe and Wait() blocks until every writer closes
	// it. Without WaitDelay + a process-group kill the deadline is DEFEATED.
	e18Script = "( sleep 5 ) & exit 0"
)

// TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe is E18 for the gh seam.
func TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics; not applicable on Windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), e18Deadline)
	defer cancel()

	r := &gh.SystemRunner{Path: "/bin/sh"}

	start := time.Now()
	_, _ = r.Run(ctx, "-c", e18Script)
	elapsed := time.Since(start)

	if elapsed > e18Budget {
		t.Fatalf("gh.Run took %v with an %v deadline; want <= %v. "+
			"An orphaned grandchild holding the stdout pipe defeats the deadline (E18/F12).",
			elapsed.Round(time.Millisecond), e18Deadline, e18Budget)
	}
	t.Logf("gh.Run returned in %v (deadline %v, budget %v)",
		elapsed.Round(time.Millisecond), e18Deadline, e18Budget)
}
