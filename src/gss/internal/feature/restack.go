package feature

import (
	"context"
	"fmt"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/identity"
	"github.com/wenlock/dotfiles/gss/internal/registry"
	"github.com/wenlock/dotfiles/gss/internal/stack"
)

// RestackOpts configures `feature restack`.
type RestackOpts struct {
	WorkerRef string
	Onto      string // new base branch
}

// Restack re-targets a worker's branch onto a new base (design.md → "gss
// feature restack --onto"; resolution #17): it rebases --onto the new base,
// force-pushes with lease, updates the PR's base, and increments
// restack_count on the worker AND every descendant whose effective base
// moved. restack_count only ever increments — restacking back to the
// original base does NOT decrement it (the laundering mitigation that
// disqualifies the worker from auto-promote). A rebase conflict aborts and
// returns ErrRebaseConflict; a cyclic stack returns stack.ErrCycle.
func (s *Service) Restack(ctx context.Context, opts RestackOpts) error {
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
	feat := reg.Features[fi]
	w := feat.Workers[wi]

	nodes, here := stackNodes(feat, ref)
	_, affected, err := stack.RestackOnto(nodes, here, opts.Onto)
	if err != nil {
		return err // e.g. stack.ErrCycle
	}

	// Rebase the worker's commits onto the new base.
	if out, err := s.Git.Run(ctx, "-C", w.Worktree, "rebase", "--onto", opts.Onto, w.BaseBranch); err != nil {
		_, _ = s.Git.Run(ctx, "-C", w.Worktree, "rebase", "--abort")
		return fmt.Errorf("%w: restack onto %s: %s", errors.ErrRebaseConflict, opts.Onto, strings.TrimSpace(string(out)))
	}
	if out, err := s.Git.Run(ctx, "-C", w.Worktree, "push", "--force-with-lease", "origin", w.Branch); err != nil {
		return fmt.Errorf("feature restack: force-push: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if w.PRURL != "" {
		if err := s.GH.PREdit(ctx, prNumber(w.PRURL), gh.PREditOpts{Base: opts.Onto}); err != nil {
			return fmt.Errorf("feature restack: pr edit: %w", err)
		}
	}

	// Persist: worker's new base + restack_count++ on every affected ref.
	affectedSet := make(map[string]bool, len(affected))
	for _, r := range affected {
		affectedSet[r] = true
	}
	return s.Store.Update(func(r *registry.Registry) error {
		f, wIdx := findWorker(*r, ref)
		if f < 0 {
			return nil
		}
		r.Features[f].Workers[wIdx].BaseBranch = opts.Onto
		for i := range r.Features[f].Workers {
			if affectedSet[workerRef(r.Features[f].Name, r.Features[f].Workers[i])] {
				r.Features[f].Workers[i].RestackCount++
			}
		}
		return nil
	})
}

// stackNodes builds the stack.Node slice for a feature and returns the node
// matching ref.
func stackNodes(f registry.Feature, ref identity.WorkerRef) ([]stack.Node, stack.Node) {
	nodes := make([]stack.Node, len(f.Workers))
	var here stack.Node
	for i, w := range f.Workers {
		n := stack.Node{Ref: workerRef(f.Name, w), Branch: w.Branch, BaseBranch: w.BaseBranch}
		nodes[i] = n
		if w.User == ref.User && w.Purpose == ref.Purpose && w.Suffix == ref.Suffix {
			here = n
		}
	}
	return nodes, here
}
