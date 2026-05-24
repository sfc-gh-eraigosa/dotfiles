package feature_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/feature"
	"github.com/wenlock/dotfiles/gss/internal/registry"
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
	// Worktree materialized via backend + WORKER.md written.
	if len(be.created) != 1 || be.created[0].Branch != res.Branch {
		t.Errorf("backend.Create = %+v; want one with branch %s", be.created, res.Branch)
	}
	if _, err := os.Stat(filepath.Join(res.Worktree, "WORKER.md")); err != nil {
		t.Errorf("WORKER.md not written: %v", err)
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
