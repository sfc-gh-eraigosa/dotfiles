package feature_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/feature"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// Cross-machine sync edge cases (roadmap.md → "Failure modes you should
// expect when syncing"). Each of the five rows of that table is reproduced as
// a registry fixture + an Observer reflecting host B's reality, then asserted.
// The invariant pinned throughout: OBSERVABLE STATE WINS OVER THE REGISTRY —
// audit only ever makes the registry match observed reality, and never
// performs a fix that needs human judgement (choosing a winner, rewriting git).

// loadFixture copies a testdata registry into a writable temp store (so
// --repair can persist) and returns it.
func loadFixture(t *testing.T, name string) *registry.Store {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "audit", "xmachine", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return registry.NewStore(path)
}

func xmObs() *fakeObserver {
	return &fakeObserver{
		missingWorktrees: map[string]bool{}, missingBranches: map[string]bool{},
		unreachable: map[string]bool{}, prs: map[int]gh.PR{},
	}
}

// Mode 1: a registry row points at a worktree directory that doesn't exist
// locally (created on host A, never synced). --repair drops the orphan row.
func TestXMachine_OrphanWorktreeDropped(t *testing.T) {
	store := loadFixture(t, "orphan_worktree.json")
	obs := xmObs()
	obs.missingWorktrees["/synced/from/hostA/api"] = true
	svc := &feature.Service{Store: store, Observe: obs}

	rep, err := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if f := findFinding(rep.Findings, "worktree-missing"); f == nil || f.Severity != feature.SevError {
		t.Fatalf("want worktree-missing error; got %+v", rep.Findings)
	}
	if reg, _ := store.Load(); len(reg.Features[0].Workers) != 0 {
		t.Error("observable state wins: a row at a vanished worktree must be dropped")
	}
}

// Mode 2: the registry's pr_url references a PR this host can't see (created
// by another host / wrong creds). --repair clears pr_url+pr_state, keeps the
// rest of the row so the next checkpoint opens a fresh PR.
func TestXMachine_PRNotFoundCleared(t *testing.T) {
	store := loadFixture(t, "pr_not_found.json")
	obs := xmObs() // prs empty => PR #77 is a 404
	svc := &feature.Service{Store: store, Observe: obs}

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	if findFinding(rep.Findings, "pr-404") == nil {
		t.Fatalf("want pr-404; got %+v", rep.Findings)
	}
	reg, _ := store.Load()
	if len(reg.Features[0].Workers) != 1 {
		t.Fatal("the worker row itself must be preserved")
	}
	if w := reg.Features[0].Workers[0]; w.PRURL != "" || w.PRState != "" {
		t.Errorf("observable state wins: unreachable PR must be cleared; got url=%q state=%q", w.PRURL, w.PRState)
	}
}

// Mode 3: two rows claim the same branch (both hosts ran `worker add` against
// the same NWO+name). Flagged error; --repair must NOT pick a winner.
func TestXMachine_DuplicateBranchFlaggedNotResolved(t *testing.T) {
	store := loadFixture(t, "dup_branch.json")
	svc := &feature.Service{Store: store, Observe: xmObs()}

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	f := findFinding(rep.Findings, "duplicate-branch")
	if f == nil || f.Severity != feature.SevError {
		t.Fatalf("want duplicate-branch error; got %+v", rep.Findings)
	}
	if f.Repaired {
		t.Error("duplicate-branch needs human judgement; --repair must not choose a winner")
	}
	if reg, _ := store.Load(); len(reg.Features[0].Workers) != 2 {
		t.Error("--repair must not auto-drop either duplicate row")
	}
}

// Mode 4: `git push --force-with-lease` rejects because another host advanced
// the branch — observed as a base the local branch can no longer reach. Audit
// flags error but never rewrites git refs (requires human / restack).
func TestXMachine_DivergedBranchNeedsHuman(t *testing.T) {
	store := loadFixture(t, "diverged_branch.json")
	obs := xmObs()
	obs.unreachable["feature/auth/erai/ui"] = true
	obs.prs[43] = gh.PR{Number: 43, Base: "main", State: "OPEN", IsDraft: true}
	svc := &feature.Service{Store: store, Observe: obs}

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	f := findFinding(rep.Findings, "base-unreachable")
	if f == nil || f.Severity != feature.SevError {
		t.Fatalf("want base-unreachable error; got %+v", rep.Findings)
	}
	if f.Repaired {
		t.Error("a diverged branch requires human judgement; --repair never rewrites git refs")
	}
	if reg, _ := store.Load(); len(reg.Features[0].Workers) != 1 {
		t.Error("--repair must not drop a diverged-branch row")
	}
}

// Mode 5: auto-promote "fired for nothing" — host A already flipped the PR to
// ready; host B's registry still says draft. Benign + idempotent: info
// severity, and --repair reconciles pr_state to the observed value.
func TestXMachine_StalePRStateReconciled(t *testing.T) {
	store := loadFixture(t, "stale_pr_state.json")
	obs := xmObs()
	obs.prs[55] = gh.PR{Number: 55, Base: "main", State: "OPEN", IsDraft: false} // promoted on host A
	svc := &feature.Service{Store: store, Observe: obs}

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	f := findFinding(rep.Findings, "pr-state-stale")
	if f == nil {
		t.Fatalf("want pr-state-stale; got %+v", rep.Findings)
	}
	if f.Severity != feature.SevInfo {
		t.Errorf("a stale pr_state is benign; want info severity, got %q", f.Severity)
	}
	if rep.HasErrors() {
		t.Error("mode 5 is benign; it must not raise an error-severity finding")
	}
	if got := reg(t, store).Features[0].Workers[0].PRState; got != "open" {
		t.Errorf("observable state wins: pr_state should reconcile draft->open; got %q", got)
	}
}

func reg(t *testing.T, s *registry.Store) registry.Registry {
	t.Helper()
	r, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return r
}
