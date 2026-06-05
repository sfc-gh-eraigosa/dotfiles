package feature

import (
	"context"
	"fmt"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/stack"
)

// approver is the approval-token gate (satisfied by *approval.Verifier).
type approver interface {
	Verify(ctx context.Context, repoPath string, forceAutonomous bool) error
}

// ReadyOpts configures `feature pr --ready`.
type ReadyOpts struct {
	WorkerRef string
	Force     bool // override the parent-still-draft guard (not the token gate)
}

// PromoteReady promotes a worker's draft PR to ready-for-review (design.md
// → "gss feature pr --ready"; resolution #11). Promotion is the trust
// boundary, so it is ALWAYS gated on a valid approval token — a missing or
// invalid token returns errors.ErrPRReadyNeedsToken and never promotes.
// By default it also refuses to promote a non-bottom PR while its parent is
// still draft (merge bottom-up); --force overrides only that guard, never
// the token gate.
func (s *Service) PromoteReady(ctx context.Context, opts ReadyOpts) error {
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
	if w.PRURL == "" {
		return fmt.Errorf("feature pr --ready: %s has no PR to promote", ref.String())
	}

	// 1. Approval gate — always required (never bypassed, even with --force).
	if s.Approval == nil {
		return fmt.Errorf("%w: no approval verifier configured", errors.ErrPRReadyNeedsToken)
	}
	if err := s.Approval.Verify(ctx, w.Worktree, false); err != nil {
		return fmt.Errorf("%w: %v", errors.ErrPRReadyNeedsToken, err)
	}

	// 2. Parent-still-draft guard (overridable with --force).
	if !opts.Force {
		if pref, draft := parentDraft(reg.Features[fi], ref); draft {
			return fmt.Errorf("feature pr --ready: parent %s is still draft; merge bottom-up first or pass --force", pref)
		}
	}

	// 3. Promote.
	if err := s.GH.PRReady(ctx, prNumber(w.PRURL)); err != nil {
		return fmt.Errorf("feature pr --ready: %w", err)
	}
	return s.Store.Update(func(r *registry.Registry) error {
		f, wIdx := findWorker(*r, ref)
		if f >= 0 {
			r.Features[f].Workers[wIdx].PRState = "open"
		}
		return nil
	})
}

// parentDraft reports the worker's in-stack parent ref and whether that
// parent's PR is still draft. A bottom worker (no in-set parent) is never
// blocked.
func parentDraft(f registry.Feature, ref identity.WorkerRef) (string, bool) {
	nodes := make([]stack.Node, len(f.Workers))
	var here stack.Node
	for i, w := range f.Workers {
		n := stack.Node{Ref: workerRef(f.Name, w), Branch: w.Branch, BaseBranch: w.BaseBranch}
		nodes[i] = n
		if w.User == ref.User && w.Purpose == ref.Purpose && w.Suffix == ref.Suffix {
			here = n
		}
	}
	parent, ok := stack.Parent(nodes, here)
	if !ok {
		return "", false // bottom of the stack
	}
	for _, w := range f.Workers {
		if w.Branch == parent.Branch {
			return parent.Ref, w.PRState == "" || w.PRState == "draft"
		}
	}
	return "", false
}
