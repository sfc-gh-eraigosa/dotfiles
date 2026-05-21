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

// MergedOpts configures `feature merged`.
type MergedOpts struct {
	WorkerRef string
	// NoAutoReady disables the auto-promote leg for this call; the mechanical
	// re-targeting still runs.
	NoAutoReady bool
}

// MergedResult reports what `feature merged` did.
type MergedResult struct {
	Retargeted []string // worker refs whose base_branch (and PR base) were re-pointed
	Promoted   string   // the single child worker ref flipped draft->ready, or ""
	Notice     string   // one-line stderr notice when a bottom child was NOT auto-promoted
}

// Merged collapses one stack level after worker-ref's PR landed (design.md →
// "gss feature merged"; resolutions #16, #17). Every worker based DIRECTLY on
// the merged branch is mechanically re-targeted onto the merged worker's
// former base (its PR base via `gh pr edit`, plus the registry row). THEN —
// only when the merged worker was the stack bottom, the stack is linear (one
// direct child), and that child's restack_count is 0 — the single child draft
// is auto-promoted to ready (`gh pr ready`). The ordering is fixed:
// re-target before promote. Fan-outs and restacked children are re-targeted
// but never promoted; Merged returns a one-line Notice naming the
// disqualified children so the human can ratify with `gss feature pr --ready`.
// --no-auto-ready disables the promote leg entirely.
func (s *Service) Merged(ctx context.Context, opts MergedOpts) (MergedResult, error) {
	ref, err := identity.ParseWorkerRef(opts.WorkerRef)
	if err != nil {
		return MergedResult{}, err
	}
	reg, err := s.Store.Load()
	if err != nil {
		return MergedResult{}, err
	}
	fi, wi := findWorker(reg, ref)
	if fi < 0 {
		return MergedResult{}, fmt.Errorf("%w: no such worker %q", errors.ErrInvalidIdent, opts.WorkerRef)
	}
	feat := reg.Features[fi]
	mergedRef := workerRef(feat.Name, feat.Workers[wi])

	nodes, here := stackNodes(feat, ref)
	retargets := stack.RetargetOnMerge(nodes, here)

	// restack_count lookup, registry-backed — the lifetime invariant gate.
	restackOf := func(r string) int {
		for _, w := range feat.Workers {
			if workerRef(feat.Name, w) == r {
				return w.RestackCount
			}
		}
		return 0
	}
	promote, eligible := stack.AutoPromoteChild(nodes, here, feat.DefaultBaseBranch, restackOf)

	// 1) Mechanical re-target FIRST: re-point each direct child's PR base onto
	//    the merged worker's former base.
	var retargeted []string
	for _, rt := range retargets {
		if cw := workerByRef(feat, rt.Ref); cw != nil && cw.PRURL != "" {
			if err := s.GH.PREdit(ctx, prNumber(cw.PRURL), gh.PREditOpts{Base: rt.NewBase}); err != nil {
				return MergedResult{}, fmt.Errorf("feature merged: re-target %s: %w", rt.Ref, err)
			}
		}
		retargeted = append(retargeted, rt.Ref)
	}

	// 2) THEN auto-promote (ordering rule pinned): flip the single eligible
	//    child draft -> ready, unless --no-auto-ready.
	var promoted, notice string
	switch {
	case eligible && !opts.NoAutoReady:
		if cw := workerByRef(feat, promote.Ref); cw != nil && cw.PRURL != "" {
			if err := s.GH.PRReady(ctx, prNumber(cw.PRURL)); err != nil {
				return MergedResult{}, fmt.Errorf("feature merged: promote %s: %w", promote.Ref, err)
			}
		}
		promoted = promote.Ref
	case here.BaseBranch == feat.DefaultBaseBranch:
		notice = disqualifyNotice(nodes, here, restackOf)
	}

	// 3) Persist: merged worker -> "merged"; children re-pointed; promoted -> "open".
	newBase := make(map[string]string, len(retargets))
	for _, rt := range retargets {
		newBase[rt.Ref] = rt.NewBase
	}
	if err := s.Store.Update(func(r *registry.Registry) error {
		f, _ := findWorker(*r, ref)
		if f < 0 {
			return nil
		}
		for i := range r.Features[f].Workers {
			wr := workerRef(r.Features[f].Name, r.Features[f].Workers[i])
			if wr == mergedRef {
				r.Features[f].Workers[i].PRState = "merged"
			}
			if nb, ok := newBase[wr]; ok {
				r.Features[f].Workers[i].BaseBranch = nb
			}
			if wr == promoted {
				r.Features[f].Workers[i].PRState = "open"
			}
		}
		return nil
	}); err != nil {
		return MergedResult{}, err
	}

	return MergedResult{Retargeted: retargeted, Promoted: promoted, Notice: notice}, nil
}

// workerByRef returns a pointer to the worker in feat matching ref, or nil.
func workerByRef(feat registry.Feature, ref string) *registry.Worker {
	for i := range feat.Workers {
		if workerRef(feat.Name, feat.Workers[i]) == ref {
			return &feat.Workers[i]
		}
	}
	return nil
}

// disqualifyNotice builds the one-line stderr notice when the merged worker
// was the bottom but no child was auto-promoted: fan-out (>=2 children) or a
// single child that has been restacked.
func disqualifyNotice(nodes []stack.Node, merged stack.Node, restackOf func(string) int) string {
	kids := stack.Children(nodes, merged)
	switch {
	case len(kids) >= 2:
		refs := make([]string, len(kids))
		for i, k := range kids {
			refs[i] = k.Ref
		}
		return fmt.Sprintf("not auto-promoting %s (fan-out: %d children); ratify with `gss feature pr --ready --worker <ref>`",
			strings.Join(refs, ", "), len(kids))
	case len(kids) == 1 && restackOf(kids[0].Ref) > 0:
		return fmt.Sprintf("not auto-promoting %s (restacked %d times); ratify with `gss feature pr --ready --worker %s`",
			kids[0].Ref, restackOf(kids[0].Ref), kids[0].Ref)
	}
	return ""
}
