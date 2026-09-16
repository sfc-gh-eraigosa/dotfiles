package feature

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/stack"
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
	// Validate --onto BEFORE it can reach git or the registry (dotfiles#96):
	// this value is persisted verbatim as the worker's BaseBranch and later
	// replayed as a bare positional to `git rebase`, so a flag-like value
	// (e.g. "--exec=<cmd>") would poison the registry for a later exploit.
	if err := identity.ValidateBranchRef(opts.Onto); err != nil {
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

	// Rebase the worker's commits onto the new base. The "--" end-of-options
	// separator (dotfiles#96) is defence-in-depth: opts.Onto and w.BaseBranch
	// are validated above/at their write sites, but this stops the bare
	// positional upstream ref from EVER being reinterpreted as a git flag,
	// even for a value that reached the registry some other way (e.g. a
	// hand-edited registry.json).
	if out, err := s.Git.Run(ctx, "-C", w.Worktree, "rebase", "--onto", opts.Onto, "--", w.BaseBranch); err != nil {
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
