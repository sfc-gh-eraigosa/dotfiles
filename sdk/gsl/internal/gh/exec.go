// Package gh wraps the GitHub CLI (`gh`) for the gsl codebase.
//
// Every package that needs to interact with GitHub MUST go through this
// package's Runner interface. Direct use of `os/exec` outside this package is
// forbidden and enforced by scripts/check-deps.sh.
//
// Two implementations are provided:
//
//   - SystemRunner — shells out to the real `gh` binary on $PATH.
//     Construct with NewSystemRunner() in production code.
//   - fake.Runner (sub-package internal/gh/fake) — recording fake for tests.
//
// CP1 note: this package defines only the interface + real shell-out wrapper.
// Actual PR-lookup logic is added in CP2.
package gh

import (
	"bytes"
	"context"
	"os/exec"

	gslexec "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/exec"
)

// Runner is the single entry point for gh invocations in gsl.
// The signature mirrors internal/git.Runner for consistency.
//
// name is the gh subcommand (e.g. "pr", "repo"); args is forwarded verbatim.
// The returned []byte is the combined stdout+stderr of the invocation.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// SystemRunner is the production implementation of Runner. It shells out to
// the `gh` binary on $PATH (or to a custom Path when set).
// It is safe to share across goroutines.
type SystemRunner struct {
	// Path overrides the gh binary location. When empty, Run uses "gh"
	// from $PATH.
	Path string
}

// NewSystemRunner returns a SystemRunner configured to use "gh" from $PATH.
func NewSystemRunner() *SystemRunner {
	return &SystemRunner{Path: "gh"}
}

// Run invokes `<Path> <name> <args...>` with the given context.
// Stdout and Stderr are merged into a single buffer.
//
// Containment (F12/E18): hardened via internal/exec — own process group,
// group-wide kill on cancellation, WaitDelay bound on the I/O pipes. This seam
// needs it most: `gh` forks git subprocesses of its own (remote -v, config,
// remote get-url), any of which can outlive it holding the stdout pipe.
func (r *SystemRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	bin := r.Path
	if bin == "" {
		bin = "gh"
	}
	full := make([]string, 0, 1+len(args))
	full = append(full, name)
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, bin, full...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	gslexec.Harden(cmd)
	if err := cmd.Run(); err != nil {
		gslexec.LogSubprocessError("gh", bin, full, err)
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}
