package feature_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// startedFeature creates a service with feature "auth" already started.
func startedFeature(t *testing.T) (*feature.Service, *fakeBackend, *registry.Store) {
	t.Helper()
	svc, be, store := newService(t)
	if _, err := svc.Start(context.Background(), feature.StartOpts{Name: "auth", Description: "feature", BaseBranch: "main"}); err != nil {
		t.Fatalf("seed Start: %v", err)
	}
	return svc, be, store
}

func TestWorkerAdd_CreatesWorker(t *testing.T) {
	svc, be, store := startedFeature(t)
	res, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{
		Feature: "auth", Purpose: "api", Description: "Implement endpoints",
	})
	if err != nil {
		t.Fatalf("WorkerAdd: %v", err)
	}
	if res.Ref.String() != "auth/erai/api" {
		t.Errorf("ref = %q; want auth/erai/api", res.Ref.String())
	}
	if res.Branch != "feature/auth/erai/api" {
		t.Errorf("branch = %q; want feature/auth/erai/api", res.Branch)
	}
	// Registry row present.
	reg, _ := store.Load()
	if n := len(reg.Features[0].Workers); n != 1 {
		t.Fatalf("workers = %d; want 1", n)
	}
	// Worktree materialized via backend.
	if len(be.created) != 1 || be.created[0].Branch != res.Branch {
		t.Errorf("backend.Create = %+v; want one with branch %s", be.created, res.Branch)
	}
	// WORKER.md is seeded OUTSIDE the worktree (issue #132): present at the
	// meta path, absent from the worktree root so it can never appear in the
	// consumer repo's git status.
	if _, err := os.Stat(feature.WorkerMetaPath(res.Worktree)); err != nil {
		t.Errorf("WORKER.md not written at meta path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(res.Worktree, "WORKER.md")); !os.IsNotExist(err) {
		t.Errorf("WORKER.md must NOT exist in the worktree root (#132); stat err=%v", err)
	}
}

// TestWorkerAdd_SiblingLeavesDistinct is the regression guard that proves the
// leaf-keyed location (Option B) over the issue's shared-parent proposal
// (Option A): two workers under the same (feature,user) get DISTINCT WORKER.md
// files that do not clobber each other.
func TestWorkerAdd_SiblingLeavesDistinct(t *testing.T) {
	svc, _, _ := startedFeature(t)
	ctx := context.Background()
	a, err := svc.WorkerAdd(ctx, feature.WorkerAddOpts{Feature: "auth", Purpose: "api", Description: "first"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	b, err := svc.WorkerAdd(ctx, feature.WorkerAddOpts{Feature: "auth", Purpose: "api", Description: "second"})
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	pa, pb := feature.WorkerMetaPath(a.Worktree), feature.WorkerMetaPath(b.Worktree)
	if pa == pb {
		t.Fatalf("sibling workers share a WORKER.md path %q — Option A collision", pa)
	}
	for _, p := range []string{pa, pb} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("sibling WORKER.md missing at %q: %v", p, err)
		}
	}
}

func TestWorkerAdd_RequiresDescription(t *testing.T) {
	svc, _, _ := startedFeature(t)
	if _, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{Feature: "auth", Purpose: "api"}); err == nil {
		t.Error("missing --description: want error")
	}
}

func TestWorkerAdd_PersistsSpawnedByVerbatim(t *testing.T) {
	svc, _, store := startedFeature(t)
	sb := &registry.SpawnedBy{Engine: "claude", SessionID: "c1a2b3", PaneID: "%17", TmuxMgrSession: "coder-1", StartedAt: "2026-05-21T12:00:00Z"}
	if _, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{
		Feature: "auth", Purpose: "ui", Description: "wire ui", SpawnedBy: sb,
	}); err != nil {
		t.Fatalf("WorkerAdd: %v", err)
	}
	reg, _ := store.Load()
	got := reg.Features[0].Workers[0].SpawnedBy
	if got == nil || got.Engine != "claude" || got.SessionID != "c1a2b3" || got.PaneID != "%17" {
		t.Errorf("spawned_by not persisted verbatim: %+v", got)
	}
}

func TestWorkerAdd_UniqueTupleGetsSuffix(t *testing.T) {
	svc, _, store := startedFeature(t)
	ctx := context.Background()
	a, err := svc.WorkerAdd(ctx, feature.WorkerAddOpts{Feature: "auth", Purpose: "api", Description: "first"})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	b, err := svc.WorkerAdd(ctx, feature.WorkerAddOpts{Feature: "auth", Purpose: "api", Description: "second"})
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if a.Ref.Suffix != "" {
		t.Errorf("first worker should have no suffix; got %q", a.Ref.Suffix)
	}
	if b.Ref.Suffix == "" {
		t.Errorf("second worker (same user/purpose) must get a suffix to stay unique")
	}
	if a.Ref.String() == b.Ref.String() {
		t.Errorf("refs collided: %q", a.Ref.String())
	}
	reg, _ := store.Load()
	if n := len(reg.Features[0].Workers); n != 2 {
		t.Errorf("workers = %d; want 2", n)
	}
}

func TestWorkerAdd_InvalidPurpose(t *testing.T) {
	svc, _, _ := startedFeature(t)
	// "moss" is a suffix wordlist word → rejected as a purpose.
	if _, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{Feature: "auth", Purpose: "moss", Description: "x"}); err == nil {
		t.Error("wordlist purpose: want rejection")
	}
}

func TestWorkerAdd_UnknownFeature(t *testing.T) {
	svc, _, _ := newService(t) // no feature started
	if _, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{Feature: "ghost", Purpose: "api", Description: "x"}); err == nil {
		t.Error("unknown feature: want error")
	}
}

// TestWorkerAdd_RejectsFlagLikeBase pins the dotfiles#96 fix at the other
// write path into BaseBranch: `worker add --base` flows unvalidated into
// the registry, from where it is later replayed as a bare positional to
// `git rebase` (checkpoint.go/rebase.go's "origin/"+base prefix defeats the
// injection there, but restack.go's positional does not — see
// TestRestack_RejectsFlagLikeOnto). No worktree must be created.
func TestWorkerAdd_RejectsFlagLikeBase(t *testing.T) {
	svc, be, _ := startedFeature(t)
	if _, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{
		Feature: "auth", Purpose: "api", Description: "x", BaseBranch: "--exec=touch /tmp/pwned",
	}); err == nil {
		t.Error("flag-like --base: want a validation error")
	}
	if len(be.created) != 0 {
		t.Errorf("backend.Create called %d times; want 0 (rejected before materializing)", len(be.created))
	}
}

// TestWorkerAdd_DetectsLeafCollisionAcrossDifferentSplit pins a finding from
// the PR #333 review: WorkerAdd's uniqueness check (`taken`, fed to
// identity.AllocateRef) compared candidate/stored refs component-wise, the
// same lossy comparison dotfiles#258 fixed findWorker to stop using. An
// existing worker stored as Purpose="apt", Suffix="pin" (as AllocateRef
// would draw it) and a new `--purpose apt-pin` request reconstruct to the
// identical leaf "apt-pin", but component-wise comparison never matches
// them — so `taken()` reported "free" and a second, textually-identical-leaf
// worker could be allocated, silently colliding with the first (later
// checkpoint/restack/done calls against ".../apt-pin" could operate on
// either worker's worktree/branch/PR, whichever findWorker's leaf-based
// match happens to hit first).
func TestWorkerAdd_DetectsLeafCollisionAcrossDifferentSplit(t *testing.T) {
	svc, _, store := startedFeature(t)
	if err := store.Update(func(r *registry.Registry) error {
		r.Features[0].Workers = append(r.Features[0].Workers, registry.Worker{
			User: "erai", Purpose: "apt", Suffix: "pin",
			Branch: "feature/auth/erai/apt-pin", Worktree: "/wt/apt-pin", BaseBranch: "main", Description: "existing",
		})
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := svc.WorkerAdd(context.Background(), feature.WorkerAddOpts{
		Feature: "auth", Purpose: "apt-pin", Description: "new",
	})
	if err != nil {
		t.Fatalf("WorkerAdd: %v", err)
	}
	if res.Ref.Suffix == "" {
		t.Errorf("expected a drawn suffix to disambiguate from the colliding leaf %q; got none (ref=%q)",
			"apt-pin", res.Ref.String())
	}
}
