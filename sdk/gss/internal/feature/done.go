package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/tmpl"
)

// DoneOpts configures `feature done`.
type DoneOpts struct {
	WorkerRef string
	Force     bool
}

// DoneResult reports what teardown did.
type DoneResult struct {
	Removed        string // worker_ref removed
	FeatureDeleted bool   // empty-feature cleanup removed the feature row + FEATURE.md
	RetainedNotice string // one-line stderr notice when an orphaned feature is retained
}

// Done tears down a worker (design.md → "gss feature done"): it refuses on a
// dirty worktree, an open/unmerged PR, or remaining dependents (unless
// --force), then removes the worktree (via the backend) and the registry
// row. This is the non-interactive (worker-mode) path: when the removal
// empties the feature, it compares the on-disk FEATURE.md against the
// rendered template (whitespace-normalised) — byte-identical means no human
// edits, so the feature row + FEATURE.md are deleted; any substantive edit
// retains them with a stderr notice (resolution #19).
func (s *Service) Done(ctx context.Context, opts DoneOpts) (DoneResult, error) {
	ref, err := identity.ParseWorkerRef(opts.WorkerRef)
	if err != nil {
		return DoneResult{}, err
	}
	reg, err := s.Store.Load()
	if err != nil {
		return DoneResult{}, err
	}
	fi, wi := findWorker(reg, ref)
	if fi < 0 {
		return DoneResult{}, fmt.Errorf("%w: no such worker %q", errors.ErrInvalidIdent, opts.WorkerRef)
	}
	feat := reg.Features[fi]
	w := feat.Workers[wi]
	res := DoneResult{Removed: ref.String()}

	if !opts.Force {
		if out, _ := s.gitOut(ctx, w.Worktree, "status", "--porcelain"); strings.TrimSpace(out) != "" {
			return res, fmt.Errorf("%w: worktree %s is dirty; commit or pass --force", errors.ErrDirtyWorktree, w.Worktree)
		}
		for _, other := range feat.Workers {
			if other.Branch != w.Branch && other.BaseBranch == w.Branch {
				return res, fmt.Errorf("feature done: %s still bases on %s; re-target it or pass --force", workerRef(feat.Name, other), ref.String())
			}
		}
		if w.PRURL != "" && w.PRState != "" && w.PRState != "merged" {
			return res, fmt.Errorf("feature done: PR for %s is %q (unmerged); pass --force", ref.String(), w.PRState)
		}
	}

	// Tear down the worktree (the backend owns the inode).
	if err := s.Backend.Remove(w.Worktree, opts.Force); err != nil {
		return res, fmt.Errorf("feature done: remove worktree: %w", err)
	}
	// Tear down the worker's external WORKER.md home (issue #132). Nothing else
	// touches the <feature>/<user>/.gss-meta tree, so without this the meta dir
	// would accrete forever under the worktrees root. Best-effort-remove the now
	// possibly-empty .gss-meta parent too (fails harmlessly if a sibling remains).
	_ = os.RemoveAll(workerMetaDir(w.Worktree))
	_ = os.Remove(filepath.Join(filepath.Dir(w.Worktree), ".gss-meta"))

	// Decide empty-feature cleanup BEFORE mutating the registry.
	willEmpty := len(feat.Workers) == 1
	deleteFeature := false
	featDir := filepath.Join(s.WorktreeRoot, s.NWO, feat.Name)
	if willEmpty {
		if s.templateClean(feat) {
			deleteFeature = true
		} else {
			res.RetainedNotice = fmt.Sprintf("orphaned feature %q retained (FEATURE.md has edits): %s", feat.Name, filepath.Join(featDir, "FEATURE.md"))
		}
	}

	err = s.Store.Update(func(r *registry.Registry) error {
		f, wIdx := findWorker(*r, ref)
		if f < 0 {
			return nil
		}
		// remove the worker row
		ws := r.Features[f].Workers
		r.Features[f].Workers = append(ws[:wIdx], ws[wIdx+1:]...)
		// re-target eager dependents on --force
		if opts.Force {
			for i := range r.Features[f].Workers {
				if r.Features[f].Workers[i].BaseBranch == w.Branch {
					r.Features[f].Workers[i].BaseBranch = w.BaseBranch
				}
			}
		}
		if deleteFeature && len(r.Features[f].Workers) == 0 {
			r.Features = append(r.Features[:f], r.Features[f+1:]...)
		}
		return nil
	})
	if err != nil {
		return res, err
	}

	if deleteFeature {
		_ = os.Remove(filepath.Join(featDir, "FEATURE.md"))
		_ = os.Remove(featDir) // best-effort (only if now empty)
		res.FeatureDeleted = true
	}
	return res, nil
}

// DoneFeature tears down a whole feature: the registry row, FEATURE.md, and
// the feature directory.
//
// Done removes a *worker* and only deletes the feature as a side effect of
// removing its last one. That left `gss feature start` with no inverse: a
// feature created and never populated — the common outcome of a mistyped or
// abandoned start — had no supported teardown at all, and cleaning it up
// meant hand-editing registry.json.
//
// The guards mirror Done's: refuse while workers remain, and refuse when
// FEATURE.md carries edits (that text is the only record of the decisions
// behind the feature). --force overrides both, and is required for the
// workers case because their worktrees are removed by `feature done`, not
// here — this verb never touches a worktree.
func (s *Service) DoneFeature(name string, force bool) (DoneResult, error) {
	res := DoneResult{}
	if strings.TrimSpace(name) == "" {
		return res, fmt.Errorf("%w: feature name is required", errors.ErrInvalidIdent)
	}
	reg, err := s.Store.Load()
	if err != nil {
		return res, err
	}
	fi := -1
	for i := range reg.Features {
		if reg.Features[i].Name == name {
			fi = i
			break
		}
	}
	if fi < 0 {
		return res, fmt.Errorf("%w: no such feature %q", errors.ErrInvalidIdent, name)
	}
	feat := reg.Features[fi]

	if !force {
		if n := len(feat.Workers); n > 0 {
			return res, fmt.Errorf("feature done: %q still has %d worker(s); remove them with 'gss feature done <worker-ref>' first, or pass --force", name, n)
		}
		if !s.templateClean(feat) {
			return res, fmt.Errorf("feature done: FEATURE.md for %q has edits; pass --force to discard them", name)
		}
	}

	if err := s.Store.Update(func(r *registry.Registry) error {
		for i := range r.Features {
			if r.Features[i].Name == name {
				r.Features = append(r.Features[:i], r.Features[i+1:]...)
				return nil
			}
		}
		return nil
	}); err != nil {
		return res, err
	}

	featDir := filepath.Join(s.WorktreeRoot, s.NWO, name)
	_ = os.Remove(filepath.Join(featDir, "FEATURE.md"))
	_ = os.Remove(featDir) // best-effort (only if now empty)
	res.FeatureDeleted = true
	return res, nil
}

// templateClean reports whether the feature's on-disk FEATURE.md is
// byte-identical (whitespace-normalised) to the rendered template — i.e. it
// carries no human/agent edits.
func (s *Service) templateClean(feat registry.Feature) bool {
	rendered, err := tmpl.RenderEmbeddedFeature(tmpl.FeatureData{
		Name: feat.Name, Description: feat.Description, StartedAt: feat.StartedAt, BaseBranch: feat.DefaultBaseBranch,
	})
	if err != nil {
		return false
	}
	onDisk, err := os.ReadFile(filepath.Join(s.WorktreeRoot, s.NWO, feat.Name, "FEATURE.md"))
	if err != nil {
		// No FEATURE.md on disk → nothing to retain.
		return true
	}
	return normalizeWS(string(onDisk)) == normalizeWS(rendered)
}

// normalizeWS trims trailing whitespace per line and trailing blank lines,
// so only substantive edits count as a difference.
func normalizeWS(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
