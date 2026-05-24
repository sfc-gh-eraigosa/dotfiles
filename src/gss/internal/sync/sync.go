// Package sync implements `gss sync`: fetch origin, then rebase the current
// branch onto its upstream (design.md → "Safety primitives"). It reproduces
// the classic cmd/sync.go flow — fetch always precedes pull — but surfaces
// a rebase failure as the errors.ErrRebaseConflict sentinel so callers and
// the exit-code map can recognise it, while still carrying git's own output
// for display.
package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/git"
)

// Service runs the fetch+rebase sequence via an injected git.Runner.
type Service struct {
	Git git.Runner
}

// NewService wires the git runner.
func NewService(gitr git.Runner) *Service { return &Service{Git: gitr} }

// Result carries the resolved branch and git's combined output for display.
type Result struct {
	Branch string
	Output string
}

// Sync resolves the current branch (defaulting to main), runs
// `git fetch origin`, then `git pull --rebase origin <branch>`. A fetch
// error returns plainly; a pull/rebase error wraps ErrRebaseConflict with
// git's output appended for diagnostics.
func (s *Service) Sync(ctx context.Context, repoPath string) (Result, error) {
	branch := s.currentBranch(ctx, repoPath)

	if out, err := s.Git.Run(ctx, "-C", repoPath, "fetch", "origin"); err != nil {
		return Result{Branch: branch}, fmt.Errorf("sync: fetch origin: %w: %s", err, strings.TrimSpace(string(out)))
	}

	out, err := s.Git.Run(ctx, "-C", repoPath, "pull", "--rebase", "origin", branch)
	res := Result{Branch: branch, Output: string(out)}
	if err != nil {
		return res, fmt.Errorf("%w: rebase onto origin/%s failed: %s", errors.ErrRebaseConflict, branch, strings.TrimSpace(string(out)))
	}
	return res, nil
}

// currentBranch returns `git rev-parse --abbrev-ref HEAD`, defaulting to
// "main" when it can't be determined (matching classic cmd/sync.go).
func (s *Service) currentBranch(ctx context.Context, repoPath string) string {
	out, err := s.Git.Run(ctx, "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "main"
	}
	if b := strings.TrimSpace(string(out)); b != "" {
		return b
	}
	return "main"
}
