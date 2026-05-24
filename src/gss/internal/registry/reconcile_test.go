// Package registry_test verifies reconciliation per src/gss/docs/plan.md
// PR-19: stale rows dropped when the worktree is gone, pr_state refreshed
// from gh, read-only by default (observable state wins), and the --repair
// write-back path. Driven by the gh/git fakes + a temp Store.
package registry_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/gh"
	ghfake "github.com/wenlock/dotfiles/gss/internal/gh/fake"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// gitWorktrees fakes `git worktree list --porcelain` listing the given paths.
func gitWorktrees(paths ...string) *gitfake.Runner {
	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "worktree %s\nHEAD abc123\n\n", p)
	}
	return &gitfake.Runner{Default: gitfake.Response{Stdout: []byte(b.String())}}
}

func twoWorkerReg() registry.Registry {
	return registry.Registry{
		SchemaVersion: 1,
		Features: []registry.Feature{{
			Name: "feat", DefaultBaseBranch: "main",
			Workers: []registry.Worker{
				{User: "u", Purpose: "api", Branch: "b/api", Worktree: "/wt/api", BaseBranch: "main", Backend: "git", Description: "d",
					PRURL: "https://github.com/o/r/pull/42", PRState: "draft"},
				{User: "u", Purpose: "ui", Branch: "b/ui", Worktree: "/wt/ui", BaseBranch: "main", Backend: "git", Description: "d"},
			},
		}},
	}
}

func TestReconcile_DropsStaleWorktree(t *testing.T) {
	reg := twoWorkerReg()
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 42, State: "OPEN", IsDraft: true}) // matches stored "draft" → no PR change
	rc := registry.NewReconciler(ghc, gitWorktrees("/main", "/wt/api"))

	out, changes, err := rc.Reconcile(t.Context(), "/main", reg)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if n := len(out.Features[0].Workers); n != 1 {
		t.Fatalf("workers after reconcile = %d; want 1 (ui dropped)", n)
	}
	if out.Features[0].Workers[0].Purpose != "api" {
		t.Errorf("surviving worker = %q; want api", out.Features[0].Workers[0].Purpose)
	}
	if len(changes) != 1 || changes[0].Kind != registry.ChangeStaleWorktree {
		t.Errorf("changes = %+v; want one stale-worktree drop", changes)
	}
}

func TestReconcile_RefreshesPRState(t *testing.T) {
	reg := twoWorkerReg()
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 42, State: "OPEN", IsDraft: false}) // observed "open" != stored "draft"
	rc := registry.NewReconciler(ghc, gitWorktrees("/main", "/wt/api", "/wt/ui"))

	out, changes, err := rc.Reconcile(t.Context(), "/main", reg)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := out.Features[0].Workers[0].PRState; got != "open" {
		t.Errorf("api pr_state = %q; want open (observable wins)", got)
	}
	found := false
	for _, c := range changes {
		if c.Kind == registry.ChangePRState {
			found = true
		}
	}
	if !found {
		t.Errorf("changes = %+v; want a pr-state-refreshed entry", changes)
	}
}

func TestReconcile_ReadOnly(t *testing.T) {
	reg := twoWorkerReg()
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 42, State: "OPEN", IsDraft: false})
	rc := registry.NewReconciler(ghc, gitWorktrees("/main", "/wt/api")) // ui stale

	_, _, err := rc.Reconcile(t.Context(), "/main", reg)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// Input registry must be untouched (audit-style).
	if n := len(reg.Features[0].Workers); n != 2 {
		t.Errorf("input workers = %d; want 2 (Reconcile must not mutate input)", n)
	}
	if reg.Features[0].Workers[0].PRState != "draft" {
		t.Errorf("input pr_state mutated to %q; want draft unchanged", reg.Features[0].Workers[0].PRState)
	}
}

func TestRepair_WritesBack(t *testing.T) {
	reg := twoWorkerReg()
	store := registry.NewStore(t.TempDir() + "/registry.json")
	if err := store.Update(func(r *registry.Registry) error { *r = reg; return nil }); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 42, State: "OPEN", IsDraft: true})
	rc := registry.NewReconciler(ghc, gitWorktrees("/main", "/wt/api")) // ui stale

	changes, err := rc.Repair(t.Context(), "/main", store)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("changes = %+v; want 1", changes)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if n := len(got.Features[0].Workers); n != 1 {
		t.Errorf("persisted workers = %d; want 1 (stale dropped + written back)", n)
	}
}
