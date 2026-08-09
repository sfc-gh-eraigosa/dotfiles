package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
)

// TestNewSystemRunnerImplementsRunner is a compile-time check that
// *SystemRunner satisfies the Runner interface.
func TestNewSystemRunnerImplementsRunner(t *testing.T) {
	var _ mcp.Runner = mcp.NewSystemRunner()
}

// TestSystemRunner_RunCapturesOutput exercises the real SystemRunner.Run path
// against a binary we control. It verifies the empty-Path default resolves the
// executable by name via $PATH and that stdout is captured.
func TestSystemRunner_RunCapturesOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on a POSIX echo binary; skipping on Windows")
	}
	// Empty Path means Run looks the binary up by name on $PATH.
	r := mcp.NewSystemRunner()
	out, err := r.Run(context.Background(), "echo", "hello", "mcp")
	if err != nil {
		t.Fatalf("Run(echo): unexpected error: %v (out=%q)", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "hello mcp" {
		t.Errorf("Run(echo) output = %q; want %q", got, "hello mcp")
	}
}

// TestSystemRunner_RunPathOverride verifies that a non-empty Path is used as
// the executable, bypassing the name argument.
func TestSystemRunner_RunPathOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on a POSIX echo binary; skipping on Windows")
	}
	echoPath, err := execLookPath("echo")
	if err != nil {
		t.Skipf("echo not found on PATH: %v", err)
	}
	r := &mcp.SystemRunner{Path: echoPath}
	// name is deliberately a non-existent binary; Path must win.
	out, err := r.Run(context.Background(), "this-binary-does-not-exist", "ok")
	if err != nil {
		t.Fatalf("Run with Path override: unexpected error: %v (out=%q)", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "ok" {
		t.Errorf("Run with Path override output = %q; want %q", got, "ok")
	}
}

// TestSystemRunner_RunNonZeroExit verifies that a non-zero exit returns BOTH
// the captured bytes AND a non-nil error. We write a tiny shell script we
// control so the output and exit code are deterministic.
func TestSystemRunner_RunNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a /bin/sh script; skipping on Windows")
	}
	sh, err := execLookPath("sh")
	if err != nil {
		t.Skipf("sh not found on PATH: %v", err)
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "fail.sh")
	const body = "#!/bin/sh\necho boom\nexit 3\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}

	r := &mcp.SystemRunner{Path: sh}
	out, runErr := r.Run(context.Background(), sh, script)
	if runErr == nil {
		t.Fatalf("Run of failing script: want non-nil error, got nil (out=%q)", out)
	}
	if got := strings.TrimSpace(string(out)); got != "boom" {
		t.Errorf("Run of failing script output = %q; want %q (bytes must be returned alongside error)", got, "boom")
	}
}

// execLookPath is a thin indirection so this test file does not need to import
// os/exec directly (the seam gate confines os/exec to exec.go). exec.LookPath
// is a harmless read-only PATH lookup; we replicate it via PATH scanning.
func execLookPath(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

// ─── E18 / F12 — subprocess containment ──────────────────────────────────────

const (
	e18Deadline = 800 * time.Millisecond
	e18Budget   = 1500 * time.Millisecond
	// e18Script exits IMMEDIATELY but leaves a grandchild holding the inherited
	// stdout pipe for 5s. SystemRunner sets cmd.Stdout to a *bytes.Buffer, so
	// os/exec allocates an os.Pipe and Wait() blocks until every writer closes
	// it. Without WaitDelay + a process-group kill the deadline is DEFEATED.
	//
	// This is the `claude mcp list` shape: the CLI dials every server and can
	// leave transport children behind.
	e18Script = "( sleep 5 ) & exit 0"
)

// TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe is E18 for the mcp seam.
func TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics; not applicable on Windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), e18Deadline)
	defer cancel()

	// The mcp seam treats `name` as the executable itself.
	r := mcp.NewSystemRunner()

	start := time.Now()
	_, _ = r.Run(ctx, "/bin/sh", "-c", e18Script)
	elapsed := time.Since(start)

	if elapsed > e18Budget {
		t.Fatalf("mcp.Run took %v with an %v deadline; want <= %v. "+
			"An orphaned grandchild holding the stdout pipe defeats the deadline (E18/F12).",
			elapsed.Round(time.Millisecond), e18Deadline, e18Budget)
	}
	t.Logf("mcp.Run returned in %v (deadline %v, budget %v)",
		elapsed.Round(time.Millisecond), e18Deadline, e18Budget)
}
