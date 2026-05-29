// Package mcp wraps MCP server detection for the gsl codebase.
//
// Every package that needs to detect or interrogate MCP servers MUST go
// through this package's Runner interface. Direct use of `os/exec` outside
// this package is forbidden and enforced by scripts/check-deps.sh.
//
// Two implementations are provided:
//
//   - SystemRunner — shells out to the real binary on $PATH.
//     Construct with NewSystemRunner() in production code.
//   - fake.Runner (sub-package internal/mcp/fake) — recording fake for tests.
//
// CP1 note: this package defines only the interface + real shell-out wrapper.
// Actual MCP detection logic is added in CP2.
package mcp

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/sirupsen/logrus"
	"github.com/wenlock/dotfiles/gsl/internal/observe"
)

// Runner is the single entry point for MCP subprocess invocations in gsl.
// The signature mirrors internal/git.Runner for consistency.
//
// Unlike the git and gh seams — where name is a subcommand appended to a
// fixed "git"/"gh" binary — the mcp Runner treats name as the executable
// itself (resolved via $PATH or SystemRunner.Path). args are passed
// directly to that executable with no subcommand prepended.
// The returned []byte is combined stdout+stderr.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// SystemRunner is the production implementation of Runner.
// It is safe to share across goroutines.
type SystemRunner struct {
	// Path overrides the binary path. When empty, Run uses name directly
	// (i.e. $PATH lookup via exec.LookPath).
	Path string
}

// NewSystemRunner returns a SystemRunner that defers binary lookup to $PATH.
func NewSystemRunner() *SystemRunner {
	return &SystemRunner{}
}

// Run invokes the executable named by name (or Path if set) with the
// given args and context. name is the full executable, not a subcommand
// — unlike the git/gh seams which prepend a fixed binary. Stdout and
// Stderr are merged into a single buffer.
func (r *SystemRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	bin := name
	if r.Path != "" {
		bin = r.Path
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		observe.Default().WithFields(logrus.Fields{
			"event":   "mcp.subprocess_error",
			"command": bin,
			"args":    args,
			"error":   err.Error(),
		}).Warn("mcp subprocess failed")
		return buf.Bytes(), err
	}
	return buf.Bytes(), nil
}
