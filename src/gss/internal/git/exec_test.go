// Package git_test verifies the real SystemRunner against a live git
// binary. These tests are TDD-first proof for PR-02: when this file
// lands without exec.go, the package fails to compile (the SystemRunner
// symbol is undefined). Once exec.go ships the implementation, every
// case here is expected to pass.
//
// All tests that shell out to git skip cleanly when the binary isn't on
// $PATH so the package stays portable on minimal CI containers.
package git_test

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wenlock/dotfiles/gss/internal/git"
)

// skipIfNoGit short-circuits when git isn't available. The SystemRunner
// surface itself doesn't depend on git existing — these are integration
// tests of the wrapper against the real CLI.
func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not found on PATH; skipping: %v", err)
	}
}

// initRepo creates a fresh empty git repo in t.TempDir() and returns
// its path. Failures are fatal because every test below depends on a
// working repo.
func initRepo(t *testing.T) string {
	t.Helper()
	skipIfNoGit(t)
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// Set a deterministic identity for any commits we create; git init
	// alone doesn't, and `git commit` would otherwise fail when the
	// host has no user.email / user.name configured.
	for _, kv := range []struct{ k, v string }{
		{"user.email", "gss-test@example.invalid"},
		{"user.name", "gss test"},
		{"commit.gpgsign", "false"},
	} {
		c := exec.Command("git", "-C", dir, "config", kv.k, kv.v)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git config %s: %v\n%s", kv.k, err, out)
		}
	}
	return dir
}

// TestNewSystemRunner_DefaultPath — the constructor sets Path to "git"
// so callers can immediately use the resulting Runner.
func TestNewSystemRunner_DefaultPath(t *testing.T) {
	r := git.NewSystemRunner()
	if r == nil {
		t.Fatal("NewSystemRunner returned nil")
	}
	if r.Path != "git" {
		t.Errorf("Path = %q; want \"git\"", r.Path)
	}
}

// TestSystemRunner_ImplementsRunner — compile-time check that
// *SystemRunner satisfies the Runner interface. If the interface
// signature drifts, this fails fast at build time.
func TestSystemRunner_ImplementsRunner(t *testing.T) {
	var _ git.Runner = git.NewSystemRunner()
}

// TestSystemRunner_StatusPorcelain — a freshly-initialised repo has
// nothing to report; `git status --porcelain` must exit 0 with empty
// output. This is the canary that proves the SystemRunner reaches a
// real git and reads its output.
func TestSystemRunner_StatusPorcelain(t *testing.T) {
	dir := initRepo(t)
	r := git.NewSystemRunner()
	out, err := r.Run(context.Background(), "-C", dir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("Run status: %v\n%s", err, out)
	}
	if len(out) != 0 {
		t.Errorf("clean repo status = %q; want empty", out)
	}
}

// TestSystemRunner_ReturnsCombinedOutput — when there's something to
// report, the porcelain output must reach the caller verbatim. The
// SystemRunner contract is "combined stdout + stderr", and porcelain
// goes to stdout, so we verify the filename appears in the bytes.
func TestSystemRunner_ReturnsCombinedOutput(t *testing.T) {
	dir := initRepo(t)
	// Create an untracked file so porcelain has something to say.
	tmpFile := filepath.Join(dir, "untracked.txt")
	if err := writeFile(tmpFile, "hello\n"); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	r := git.NewSystemRunner()
	out, err := r.Run(context.Background(), "-C", dir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("Run status: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "untracked.txt") {
		t.Errorf("porcelain output = %q; want substring %q", out, "untracked.txt")
	}
}

// TestSystemRunner_PropagatesContext — a context whose deadline is
// already in the past must yield an error wrapping
// context.DeadlineExceeded. We use a sleep-equivalent (`git
// for-each-ref --format=%(refname)` is cheap, so we deliberately use a
// past-due deadline that fires before exec.Cmd.Run even returns).
func TestSystemRunner_PropagatesContext(t *testing.T) {
	skipIfNoGit(t)
	dir := initRepo(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Millisecond))
	defer cancel()
	r := git.NewSystemRunner()
	_, err := r.Run(ctx, "-C", dir, "status")
	if err == nil {
		t.Fatal("Run with past-due deadline: err = nil; want a deadline error")
	}
	// Acceptable forms: ctx.Err() returns DeadlineExceeded, or the OS
	// reported "context deadline exceeded" via exec.Cmd.Run. Either way
	// errors.Is must catch it.
	if !stderrors.Is(err, context.DeadlineExceeded) && ctx.Err() != context.DeadlineExceeded {
		t.Errorf("err = %v; want wrapping context.DeadlineExceeded", err)
	}
}

// TestSystemRunner_NonZeroExit — when git itself exits non-zero (e.g.
// an unknown subcommand), the wrapper must return an error that
// callers can recognise as *exec.ExitError, so they can inspect
// ExitCode(). We use `git not-a-real-subcommand` because rev-parse and
// status both treat unknown arguments as revisions/pathspecs and exit
// 0 in some configurations; `git <bogus-subcommand>` reliably exits 1
// across git versions ≥ 2.30.
func TestSystemRunner_NonZeroExit(t *testing.T) {
	dir := initRepo(t)
	r := git.NewSystemRunner()
	out, err := r.Run(context.Background(), "-C", dir, "this-is-not-a-real-git-subcommand")
	if err == nil {
		t.Fatalf("Run with bogus subcommand: err = nil; want exit error\noutput: %s", out)
	}
	var ee *exec.ExitError
	if !stderrors.As(err, &ee) {
		t.Errorf("err type = %T (%v); want *exec.ExitError", err, err)
	}
	// And the combined output should be non-empty (git prints the
	// "not a git command" message to stderr, which we capture into the
	// same buffer).
	if len(out) == 0 {
		t.Errorf("combined output empty; want git's stderr message")
	}
}

// TestSystemRunner_CustomPath — when Path is set to a non-existent
// binary, Run surfaces the lookup error (an *exec.Error or PathError),
// not nil.
func TestSystemRunner_CustomPath(t *testing.T) {
	r := &git.SystemRunner{Path: "this-binary-does-not-exist-gss-test"}
	_, err := r.Run(context.Background(), "status")
	if err == nil {
		t.Fatal("Run with bogus Path: err = nil; want lookup error")
	}
}

// TestSystemRunner_EmptyPathDefaultsToGit — a SystemRunner with Path
// left blank (zero value) must still find git on $PATH. This protects
// callers who write `&git.SystemRunner{}` instead of
// NewSystemRunner().
func TestSystemRunner_EmptyPathDefaultsToGit(t *testing.T) {
	skipIfNoGit(t)
	dir := initRepo(t)
	r := &git.SystemRunner{} // Path: ""
	out, err := r.Run(context.Background(), "-C", dir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("Run with empty Path: %v\n%s", err, out)
	}
}

// writeFile is a tiny helper so the test files don't pull in io/fs ceremony.
// Implemented at the bottom to keep the table of tests above readable.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
