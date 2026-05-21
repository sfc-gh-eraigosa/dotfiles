package registry

import (
	"context"
	"regexp"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/git"
)

// Change kinds reported by Reconcile.
const (
	// ChangeStaleWorktree — a worker whose worktree no longer exists in
	// `git worktree list` was dropped.
	ChangeStaleWorktree = "stale-worktree-dropped"
	// ChangePRState — a worker's pr_state was refreshed from gh.
	ChangePRState = "pr-state-refreshed"
)

// Change records one reconciliation action (for audit output).
type Change struct {
	Worker string // worker_ref (feature/user/purpose[-suffix])
	Kind   string
	Detail string
}

// Reconciler compares a registry against observable state — the live git
// worktrees and gh PR states — and reports drift (design.md → "Filesystem
// layout", "Conflict protection"). Observable state always wins over the
// recorded registry.
type Reconciler struct {
	GH  gh.Client
	Git git.Runner
}

// NewReconciler wires the dependencies.
func NewReconciler(ghc gh.Client, gitr git.Runner) *Reconciler {
	return &Reconciler{GH: ghc, Git: gitr}
}

// Reconcile returns a reconciled copy of reg plus the changes it represents.
// It is READ-ONLY: it neither mutates reg nor writes to disk (audit-style).
// Persisting the result is the caller's choice (the --repair path; see
// Repair). Rules:
//   - a worker whose worktree path is absent from `git worktree list` is
//     dropped as stale;
//   - a surviving worker whose gh PR state differs from its recorded
//     pr_state is refreshed to the observed state.
func (rc *Reconciler) Reconcile(ctx context.Context, repoPath string, reg Registry) (Registry, []Change, error) {
	live, err := rc.liveWorktrees(ctx, repoPath)
	if err != nil {
		return Registry{}, nil, err
	}

	out := Registry{SchemaVersion: reg.SchemaVersion}
	var changes []Change

	for _, f := range reg.Features {
		nf := f
		nf.Workers = nil
		for _, w := range f.Workers { // w is a copy; reg is never mutated
			if w.Worktree != "" && !live[w.Worktree] {
				changes = append(changes, Change{Worker: workerRef(f.Name, w), Kind: ChangeStaleWorktree, Detail: w.Worktree})
				continue // drop stale row
			}
			if w.PRURL != "" {
				if num := prNumberFromURL(w.PRURL); num > 0 {
					if pr, err := rc.GH.PRView(ctx, num); err == nil {
						if observed := prState(pr); observed != "" && observed != w.PRState {
							changes = append(changes, Change{Worker: workerRef(f.Name, w), Kind: ChangePRState, Detail: w.PRState + " -> " + observed})
							w.PRState = observed
						}
					}
				}
			}
			nf.Workers = append(nf.Workers, w)
		}
		out.Features = append(out.Features, nf)
	}
	return out, changes, nil
}

// Repair runs Reconcile and writes the result back under the store's lock
// (the --repair path). The returned changes describe what was applied.
func (rc *Reconciler) Repair(ctx context.Context, repoPath string, store *Store) ([]Change, error) {
	var changes []Change
	err := store.Update(func(r *Registry) error {
		reconciled, ch, err := rc.Reconcile(ctx, repoPath, *r)
		if err != nil {
			return err
		}
		*r = reconciled
		changes = ch
		return nil
	})
	return changes, err
}

// liveWorktrees returns the set of worktree paths reported by
// `git worktree list --porcelain`.
func (rc *Reconciler) liveWorktrees(ctx context.Context, repoPath string) (map[string]bool, error) {
	out, err := rc.Git.Run(ctx, "-C", repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			live[strings.TrimSpace(p)] = true
		}
	}
	return live, nil
}

func workerRef(feature string, w Worker) string {
	ref := feature + "/" + w.User + "/" + w.Purpose
	if w.Suffix != "" {
		ref += "-" + w.Suffix
	}
	return ref
}

var prURLRe = regexp.MustCompile(`/pull/(\d+)`)

func prNumberFromURL(url string) int {
	m := prURLRe.FindStringSubmatch(url)
	if m == nil {
		return 0
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n
}

// prState maps a gh.PR to the registry's pr_state vocabulary: a draft is
// "draft"; otherwise the lower-cased State (open/closed/merged).
func prState(pr gh.PR) string {
	if pr.IsDraft {
		return "draft"
	}
	return strings.ToLower(pr.State)
}
