package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
