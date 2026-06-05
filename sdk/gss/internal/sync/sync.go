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

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// Service runs the fetch+rebase sequence via an injected git.Runner.
type Service struct {
	Git git.Runner
}

// NewService wires the git runner.
func NewService(gitr git.Runner) *Service { return &Service{Git: gitr} }

// Result carries the resolved branch and git's combined output for display.
// NewBranch is true when the branch has no origin/<branch> counterpart yet, so
// the rebase was skipped (there is nothing to rebase onto) and the caller's
// push must create it with --set-upstream.
type Result struct {
	Branch    string
	Output    string
	NewBranch bool
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

	// A brand-new branch has no origin/<branch> counterpart, so there is nothing
	// to rebase onto. Rebasing anyway fails with git's "couldn't find remote
	// ref" — which, in the push flow, aborts after the approval token is already
	// consumed and forces a second confirmation. Detect the missing remote ref
	// (exit-code based, robust to git's wording/locale) and skip the rebase; the
	// caller's `push -u` will create the branch.
	if _, err := s.Git.Run(ctx, "-C", repoPath, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch); err != nil {
		return Result{Branch: branch, NewBranch: true, Output: "new branch — no origin counterpart yet; skipping rebase"}, nil
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
