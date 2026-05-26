// Package git wraps the `git` CLI for the gsl codebase.
//
// Every package that needs to invoke git MUST go through this package's
// Runner interface. Direct use of `os/exec` to call git from any layer
// above internal/git/ is forbidden by design and is enforced by
// scripts/check-deps.sh.
//
// Two implementations are provided:
//
//   - SystemRunner — shells out to the real `git` binary on $PATH.
//     Construct with NewSystemRunner() in production code.
//   - fake.Runner (sub-package internal/git/fake) — recording fake with
//     scriptable responses, used in tests.
//
// CP1 note: this package defines only the interface + real shell-out wrapper.
// Actual git status parsing logic is added in CP2.
package git

import (
	"bytes"
	"context"
	"os/exec"
)

// Runner is the single entry point for git invocations in the gsl codebase.
// The interface signature mirrors src/gss/internal/git.Runner for consistency.
//
// name is the git subcommand (e.g. "status", "rev-parse"); implementations
// prepend the git binary path. args is forwarded verbatim.
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
	// Path overrides the git binary location. When empty, Run uses "git"
	// from $PATH. Setting to an absolute path bypasses $PATH lookup.
	Path string
}

// NewSystemRunner returns a SystemRunner configured to use "git" from $PATH.
func NewSystemRunner() *SystemRunner {
	return &SystemRunner{Path: "git"}
}

// Run invokes `<Path> <name> <args...>` with the given context.
// Stdout and Stderr are merged into a single buffer. The buffer is returned
// even when the command exits non-zero so that callers can surface git's
// own error message verbatim.
func (r *SystemRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	bin := r.Path
	if bin == "" {
		bin = "git"
	}
	full := make([]string, 0, 1+len(args))
	full = append(full, name)
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, bin, full...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}
