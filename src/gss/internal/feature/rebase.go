package feature

import (
	"context"
	"fmt"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/identity"
)

// RebaseOpts configures `feature rebase`.
type RebaseOpts struct {
	WorkerRef string
}

// Rebase is the convenience rebase (design.md → "gss feature rebase"):
// fetch + rebase the worker on its current base_branch and force-push,
// then update the PR's base WITHOUT re-rendering the body (no description
// rewrite — that's what makes it lighter than a full checkpoint). A rebase
// conflict aborts cleanly and returns errors.ErrRebaseConflict.
func (s *Service) Rebase(ctx context.Context, opts RebaseOpts) error {
	ref, err := identity.ParseWorkerRef(opts.WorkerRef)
	if err != nil {
		return err
	}
	reg, err := s.Store.Load()
	if err != nil {
		return err
	}
	fi, wi := findWorker(reg, ref)
	if fi < 0 {
		return fmt.Errorf("%w: no such worker %q", errors.ErrInvalidIdent, opts.WorkerRef)
	}
	w := reg.Features[fi].Workers[wi]

	if out, err := s.Git.Run(ctx, "-C", w.Worktree, "fetch", "origin"); err != nil {
		return fmt.Errorf("feature rebase: fetch: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := s.Git.Run(ctx, "-C", w.Worktree, "rebase", "origin/"+w.BaseBranch); err != nil {
		_, _ = s.Git.Run(ctx, "-C", w.Worktree, "rebase", "--abort")
		return fmt.Errorf("%w: rebase onto origin/%s: %s", errors.ErrRebaseConflict, w.BaseBranch, strings.TrimSpace(string(out)))
	}
	if out, err := s.Git.Run(ctx, "-C", w.Worktree, "push", "--force-with-lease", "origin", w.Branch); err != nil {
		return fmt.Errorf("feature rebase: force-push: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// Update the PR base only (no body render — pure rebase).
	if w.PRURL != "" {
		if err := s.GH.PREdit(ctx, prNumber(w.PRURL), gh.PREditOpts{Base: w.BaseBranch}); err != nil {
			return fmt.Errorf("feature rebase: pr edit: %w", err)
		}
	}
	return nil
}
