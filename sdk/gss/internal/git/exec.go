// Package git wraps the `git` CLI for the gss codebase.
//
// Every package that needs to invoke git MUST go through this package's
// Runner interface. Direct use of `os/exec` to call git from any layer
// above internal/git/ is forbidden by design (sdk/gss/docs/design.md →
// "Test seams"; resolution #5) and will be enforced by a grep check in
// CI per the future PR-50.
//
// Two implementations are provided:
//
//   - SystemRunner — shells out to the real `git` binary on $PATH.
//     Construct with NewSystemRunner() in production code; pass into
//     whatever Service struct needs git access.
//   - fake.Runner (sub-package internal/git/fake) — recording fake with
//     scriptable responses, used in tests.
//
// Higher packages never see *exec.Cmd. They depend on the Runner
// interface, accept a Runner via constructor injection, and read the
// returned []byte / inspect the returned error.
//
// # Error semantics
//
// When git exits non-zero, Run returns (combinedOutput, *exec.ExitError).
// Callers wanting the exit code use errors.As(err, &ee) and ee.ExitCode().
// When the binary cannot be found, Run returns an *exec.Error
// (or wrapping path error). When the context is cancelled or times out,
// Run returns an error wrapping context.Canceled / context.DeadlineExceeded
// per os/exec's CommandContext contract.
package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

// nonInteractiveEnv is layered on top of the caller's environment for every
// git invocation. gss git operations are automated — they run in hooks,
// background jobs, and worktrees with no controlling terminal — so a git
// subprocess must NEVER block on a human prompt. Without this, a git command
// that wants an editor (a merge commit recreated during a rebase, an
// autosquash, a commit that needs a message) or a credential/passphrase prompt
// opens the user's interactive editor or a terminal prompt and blocks forever
// with no TTY. Worse, while it blocks it holds `.git/index.lock`, which then
// wedges every subsequent git operation in the repository until the hung
// process is killed by hand (observed as `gss feature checkpoint` hanging on a
// rebase and leaving a stale index.lock behind).
//
//   - GIT_EDITOR=true / GIT_SEQUENCE_EDITOR=true make git's editor a no-op
//     that "succeeds" immediately, accepting the prepared default message
//     instead of opening vi/nano. A rebase that would have opened an editor
//     either completes with the default message or fails non-zero (which
//     callers already handle, e.g. checkpoint's rebase --abort) — never hangs.
//   - GIT_TERMINAL_PROMPT=0 makes git fail fast instead of prompting on the
//     terminal for credentials.
//
// These are appended after the inherited environment so they win over any
// value the caller happened to export (last occurrence wins; verified against
// git's `var GIT_EDITOR`).
var nonInteractiveEnv = []string{
	"GIT_EDITOR=true",
	"GIT_SEQUENCE_EDITOR=true",
	"GIT_TERMINAL_PROMPT=0",
}

// Runner is the single entry point for git invocations in the gss
// codebase. The interface signature is pinned by sdk/gss/docs/design.md
// → "Test seams" and must not change without a design review.
//
// name is the git subcommand (e.g. "status", "rev-parse", "worktree");
// implementations are responsible for prepending the git binary path.
// args is forwarded verbatim — callers compose `-C <path>` and other
// flags themselves; the Runner does not set cmd.Dir.
//
// The returned []byte is the combined stdout+stderr of the invocation,
// even on error (mirroring exec.Cmd.CombinedOutput).
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// SystemRunner is the production implementation of Runner. It shells
// out to the git binary on $PATH (or to a custom Path when set).
//
// Concurrency: a single SystemRunner is safe to share across goroutines;
// each Run call constructs its own *exec.Cmd.
type SystemRunner struct {
	// Path overrides the git binary location. When empty (the
	// zero-value case), Run uses "git", which goes through $PATH
	// lookup via exec.LookPath. Setting Path to an absolute path
	// bypasses $PATH entirely — useful for tests that want to point
	// at a specific git build, but production code should leave this
	// blank so the user's preferred git on PATH wins.
	Path string
}

// NewSystemRunner returns a SystemRunner configured to use "git" from
// $PATH. This is the canonical production constructor; callers wire
// the result into Service structs that depend on the Runner interface.
func NewSystemRunner() *SystemRunner {
	return &SystemRunner{Path: "git"}
}

// Run invokes `<Path> <name> <args...>` with the given context.
// Stdout and Stderr are merged into a single buffer to give callers
// the same view they'd get from `2>&1` at the shell. The buffer is
// returned even when the command exits non-zero so that callers can
// surface git's own error message verbatim.
//
// Cancellation: if ctx is cancelled mid-run, exec.CommandContext sends
// SIGKILL to the child and Run returns whatever error exec produces —
// typically wrapping ctx.Err(). If ctx is already cancelled before Run
// is called, exec.Cmd.Run returns the context error immediately.
func (r *SystemRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	bin := r.Path
	if bin == "" {
		bin = "git"
	}
	// Prepend the subcommand to args; this matches the design contract
	// that `name` is a git subcommand, not the binary.
	full := make([]string, 0, 1+len(args))
	full = append(full, name)
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, bin, full...)
	// Run git non-interactively so it can never block on an editor or a
	// terminal prompt (and thus never wedge the repo holding index.lock).
	cmd.Env = append(os.Environ(), nonInteractiveEnv...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}
