package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/identity"
	"github.com/wenlock/dotfiles/gss/internal/registry"
	"github.com/wenlock/dotfiles/gss/internal/tmpl"
	"github.com/wenlock/dotfiles/gss/internal/worktree"
)

// WorkerAddOpts configures `feature worker add`. Description is required.
type WorkerAddOpts struct {
	Feature     string
	Purpose     string
	Description string
	BaseBranch  string // empty → feature's default base
	User        string // empty → resolved via UserSources
	ForceSuffix bool
	Goal        string
	SpawnedBy   *registry.SpawnedBy // persisted verbatim
}

// WorkerResult reports the created worker.
type WorkerResult struct {
	Ref      identity.WorkerRef
	Branch   string
	Worktree string
	Base     string
}

// WorkerAdd validates inputs (PR-08), resolves the user (PR-08), allocates
// a unique worker_ref (PR-07) under the registry lock (PR-18), persists the
// row (with spawned_by verbatim), then materializes the worktree and writes
// WORKER.md. Returns the worker_ref and paths.
func (s *Service) WorkerAdd(ctx context.Context, opts WorkerAddOpts) (WorkerResult, error) {
	if strings.TrimSpace(opts.Description) == "" {
		return WorkerResult{}, errors.NewValidationError("description", "a worker requires --description")
	}
	cleanDesc, err := identity.ValidateDescription(opts.Description)
	if err != nil {
		return WorkerResult{}, err
	}
	if err := identity.ValidatePurpose(opts.Purpose); err != nil {
		return WorkerResult{}, err
	}
	src := s.UserSources
	src.Override = opts.User
	user, err := identity.ResolveUser(src)
	if err != nil {
		return WorkerResult{}, err
	}

	rng := identity.NewSystemRNG(identity.Words())
	var res WorkerResult

	err = s.Store.Update(func(r *registry.Registry) error {
		fi := -1
		for i := range r.Features {
			if r.Features[i].Name == opts.Feature {
				fi = i
				break
			}
		}
		if fi < 0 {
			return fmt.Errorf("%w: no such feature %q", errors.ErrInvalidIdent, opts.Feature)
		}
		f := &r.Features[fi]
		base := opts.BaseBranch
		if base == "" {
			base = f.DefaultBaseBranch
		}
		if base == "" {
			base = "main"
		}

		taken := func(ref identity.WorkerRef) bool {
			for _, w := range f.Workers {
				if w.User == ref.User && w.Purpose == ref.Purpose && w.Suffix == ref.Suffix {
					return true
				}
			}
			return false
		}
		ref, err := identity.AllocateRef(rng, identity.WorkerRef{Feature: opts.Feature, User: user, Purpose: opts.Purpose}, opts.ForceSuffix, taken)
		if err != nil {
			return err
		}

		leaf := opts.Purpose
		if ref.Suffix != "" {
			leaf = opts.Purpose + "-" + ref.Suffix
		}
		branch := s.branchPrefix() + "/" + opts.Feature + "/" + user + "/" + leaf
		wtPath := filepath.Join(s.WorktreeRoot, s.NWO, opts.Feature, user, leaf)

		f.Workers = append(f.Workers, registry.Worker{
			User: user, Purpose: opts.Purpose, Suffix: ref.Suffix,
			Branch: branch, Worktree: wtPath, BaseBranch: base,
			Backend: s.Backend.Name(), StartedAt: s.now(),
			Description: cleanDesc, SpawnedBy: opts.SpawnedBy,
		})
		res = WorkerResult{Ref: ref, Branch: branch, Worktree: wtPath, Base: base}
		return nil
	})
	if err != nil {
		return WorkerResult{}, err
	}

	// Materialize the worktree (the backend owns the inode) then seed
	// WORKER.md. A failure here leaves a registry row that reconcile
	// (PR-19) will drop.
	if _, err := s.Backend.Create(worktree.CreateReq{Path: res.Worktree, Branch: res.Branch, BaseBranch: res.Base}); err != nil {
		return WorkerResult{}, fmt.Errorf("feature: create worktree: %w", err)
	}
	content, err := tmpl.RenderEmbeddedWorker(tmpl.WorkerData{
		Feature: opts.Feature, User: user, Purpose: opts.Purpose, Suffix: res.Ref.Suffix,
		Branch: res.Branch, BaseBranch: res.Base, Description: cleanDesc, Goal: opts.Goal,
	})
	if err != nil {
		return WorkerResult{}, err
	}
	if err := os.WriteFile(filepath.Join(res.Worktree, "WORKER.md"), []byte(content), 0o644); err != nil {
		return WorkerResult{}, fmt.Errorf("feature: write WORKER.md: %w", err)
	}
	return res, nil
}
