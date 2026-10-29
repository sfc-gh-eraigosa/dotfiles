package feature

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
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
	BranchDeleted  bool   // the local branch was deleted (PR confirmed merged)
	BranchNotice   string // one-line stderr notice when branch deletion was attempted and failed
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
	prStateNow := w.PRState // refined below when we ask GitHub

	if !opts.Force {
		if out, _ := s.gitOut(ctx, w.Worktree, "status", "--porcelain"); strings.TrimSpace(out) != "" {
			return res, fmt.Errorf("%w: worktree %s is dirty; commit or pass --force", errors.ErrDirtyWorktree, w.Worktree)
		}
		for _, other := range feat.Workers {
			if other.Branch != w.Branch && other.BaseBranch == w.Branch {
				return res, fmt.Errorf("feature done: %s still bases on %s; re-target it or pass --force", workerRef(feat.Name, other), ref.String())
			}
		}
		if w.PRState != "merged" && (w.PRURL != "" || s.GH != nil) {
			// The cached pr_state goes stale the moment the PR merges on its
			// own (Mergify), so ask GitHub before refusing (dotfiles#438).
			pr, observed, oerr := s.observePR(ctx, w)
			switch {
			case oerr == nil:
				prStateNow = observed
				s.persistPRState(ref, pr, observed)
				if observed != "merged" && observed != "closed" {
					return res, fmt.Errorf("feature done: PR for %s is %q (unmerged); pass --force", ref.String(), observed)
				}
			case oerr == errNoPR || (w.PRURL == "" && w.PRState == ""):
				// never had a PR: nothing to wait for
			case w.PRState != "":
				return res, fmt.Errorf("feature done: PR for %s is cached as %q and GitHub could not be reached to refresh it (%v); pass --force", ref.String(), w.PRState, oerr)
			}
		}
	}

	// Resolve the main repo now: once the worktree is gone there is nothing
	// left to ask, and the local branch has to be deleted from the repo.
	repoDir := ""
	if prStateNow == "merged" {
		repoDir = s.repoDirFor(ctx, w.Worktree)
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
		// dotfiles#98: by this point Backend.Remove above already succeeded
		// — the worktree is genuinely gone — so a bare registry error here
		// would read as "teardown failed entirely" when only the
		// bookkeeping half did. Say so explicitly.
		//
		// This was also the point the issue's own primary suggestion
		// (swap the order: registry first, then worktree) targeted, but
		// that trade is worse, not better: registry.Reconcile only detects
		// a row pointing at a MISSING worktree (its stale-worktree-dropped
		// check) — never the reverse. Swapping the order would trade this
		// self-healing failure mode (`gss feature audit --repair` already
		// drops the stale row) for a non-recoverable one — a live worktree,
		// still attached to git, that gss would no longer track at all.
		// The order stays; only the diagnostic changes.
		return res, fmt.Errorf("feature done: worktree %s already removed, but the registry update failed — the stale row can be cleaned up with `gss feature audit --repair`: %w", w.Worktree, err)
	}

	if deleteFeature {
		_ = os.Remove(filepath.Join(featDir, "FEATURE.md"))
		_ = os.Remove(featDir) // best-effort (only if now empty)
		res.FeatureDeleted = true
	}

	// GitHub already deleted the remote branch of a merged PR; drop the local
	// one too. Only when the PR is confirmed merged — never an open PR's.
	if prStateNow == "merged" {
		if err := s.deleteLocalBranch(ctx, repoDir, w); err != nil {
			res.BranchNotice = fmt.Sprintf("worker removed, but local branch %s was not deleted: %v", w.Branch, err)
		} else {
			res.BranchDeleted = true
		}
	}
	return res, nil
}

// errNoPR means GitHub has no PR for the worker's branch.
var errNoPR = fmt.Errorf("no PR found on GitHub")

// observePR asks GitHub for the worker's PR: by recorded URL, or — for a row
// with no pr_url — by head branch. Several PRs for a branch: an open one wins
// (so a worker with a live PR is never treated as finished), else the most
// recent. It returns the PR and its state in the registry's vocabulary.
func (s *Service) observePR(ctx context.Context, w registry.Worker) (gh.PR, string, error) {
	if s.GH == nil {
		return gh.PR{}, "", fmt.Errorf("no GitHub client")
	}
	var pr gh.PR
	if w.PRURL != "" {
		if s.NWO != "" && !strings.Contains(w.PRURL, "/"+s.NWO+"/pull/") {
			return gh.PR{}, "", fmt.Errorf("PR %s does not belong to %s", w.PRURL, s.NWO)
		}
		num := registry.PRNumberFromURL(w.PRURL)
		if num <= 0 {
			return gh.PR{}, "", fmt.Errorf("no PR number in %q", w.PRURL)
		}
		var err error
		if pr, err = s.GH.PRView(ctx, num); err != nil {
			return gh.PR{}, "", err
		}
		if pr.URL == "" {
			pr.URL = w.PRURL
		}
	} else {
		if w.Branch == "" {
			return gh.PR{}, "", errNoPR
		}
		prs, err := s.GH.PRList(ctx, gh.PRFilter{State: "all", Head: w.Branch, Limit: 5})
		if err != nil {
			return gh.PR{}, "", err
		}
		found := false
		for _, c := range prs {
			if s.NWO != "" && c.URL != "" && !strings.Contains(c.URL, "/"+s.NWO+"/pull/") {
				continue
			}
			open := func(p gh.PR) bool { st := registry.PRStateOf(p); return st == "open" || st == "draft" }
			switch {
			case !found, open(c) && !open(pr), open(c) == open(pr) && c.Number > pr.Number:
				pr, found = c, true
			}
		}
		if !found {
			return gh.PR{}, "", errNoPR
		}
	}
	st := registry.PRStateOf(pr)
	if st == "" {
		return gh.PR{}, "", fmt.Errorf("GitHub returned no state for PR #%d", pr.Number)
	}
	return pr, st, nil
}

// persistPRState records an observed pr_state on one worker row under the
// store lock (best-effort: the caller already has the answer it needs).
func (s *Service) persistPRState(ref identity.WorkerRef, pr gh.PR, state string) {
	_ = s.Store.Update(func(r *registry.Registry) error {
		if f, wi := findWorker(*r, ref); f >= 0 {
			w := &r.Features[f].Workers[wi]
			w.PRState = state
			if w.PRURL == "" {
				w.PRURL = pr.URL
			}
		}
		return nil
	})
}

// repoDirFor returns the main repo's git dir for a worker worktree: the
// configured RepoPath, else the worktree's common git dir. "" when unknown.
func (s *Service) repoDirFor(ctx context.Context, worktree string) string {
	if s.RepoPath != "" {
		return s.RepoPath
	}
	out, err := s.gitOut(ctx, worktree, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || out == "" || !filepath.IsAbs(out) {
		return ""
	}
	return out
}

// deleteLocalBranch removes the worker's branch from the main repo. Callers
// must have confirmed the PR merged: the commits were squashed, so -D (not -d)
// is the only form that works.
func (s *Service) deleteLocalBranch(ctx context.Context, repoDir string, w registry.Worker) error {
	if repoDir == "" {
		return fmt.Errorf("main repository not found")
	}
	if w.Branch == "" || w.Branch == w.BaseBranch || w.Branch == "main" || w.Branch == "master" {
		return fmt.Errorf("refusing to delete protected branch %q", w.Branch)
	}
	if _, err := s.Git.Run(ctx, "-C", repoDir, "branch", "-D", w.Branch); err != nil {
		return err
	}
	return nil
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
