// Package repo composes the git and gh seams to produce a unified
// view of the current repository state for the gsl status line.
//
// This package is NOT a seam: it routes all subprocess calls through
// the git.Runner / gh.Runner interfaces injected by the caller, and
// must not directly shell out.
package repo

import (
	"context"
	"errors"

	"github.com/wenlock/dotfiles/gsl/internal/git"
)

// Location describes where in the git working tree the current directory sits.
type Location struct {
	// IsWorktree is true when the current checkout is a linked worktree
	// (git-dir != git-common-dir), false when it is the canonical repo root.
	IsWorktree bool
	// Toplevel is the absolute path returned by `git rev-parse --show-toplevel`.
	Toplevel string
	// WorktreeCount is the total number of worktrees (including the main one)
	// as reported by `git worktree list --porcelain`.
	WorktreeCount int
}

// ErrNotARepo is returned by Locate when the working directory is not
// inside a git repository (i.e. the git.Runner failed in a way that
// indicates "no repo here").
var ErrNotARepo = errors.New("repo: not inside a git repository")

// Locate inspects the git state of dir (passed to Runner calls as a
// hint, but the Runner is free to ignore it — the SystemRunner always
// uses the process working directory). It returns a Location, or
// ErrNotARepo when the directory is not tracked by git.
//
// dir is accepted as a parameter so callers can contextualise the call;
// the underlying git seam does not currently honour it (all three
// worktree helpers pass _ for the dir argument), so passing "" is fine.
func Locate(ctx context.Context, r git.Runner, dir string) (Location, error) {
	isLinked, err := git.IsLinked(ctx, r, dir)
	if err != nil {
		return Location{}, ErrNotARepo
	}

	toplevel, err := git.Toplevel(ctx, r, dir)
	if err != nil {
		return Location{}, ErrNotARepo
	}

	count, err := git.Count(ctx, r, dir)
	if err != nil {
		// Non-fatal: we have the essential info; default count to 1.
		count = 1
	}

	return Location{
		IsWorktree:    isLinked,
		Toplevel:      toplevel,
		WorktreeCount: count,
	}, nil
}
