package feature_test

import (
	"context"
	stderrors "errors"
	"path/filepath"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/feature"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	ghfake "github.com/wenlock/dotfiles/gss/internal/gh/fake"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
	"github.com/wenlock/dotfiles/gss/internal/registry"
	"github.com/wenlock/dotfiles/gss/internal/stack"
)

// restackService seeds a 2-deep stack (api base main <- ui base api) and
// wires the git/gh fakes.
func restackService(t *testing.T, script []gitfake.Response, ghc *ghfake.Client) (*feature.Service, *registry.Store) {
	t.Helper()
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{
				{User: "erai", Purpose: "api", Branch: "feature/auth/erai/api", Worktree: "/wt/api", BaseBranch: "main",
					Description: "api", PRURL: "https://github.com/o/r/pull/42"},
				{User: "erai", Purpose: "ui", Branch: "feature/auth/erai/ui", Worktree: "/wt/ui", BaseBranch: "feature/auth/erai/api",
					Description: "ui", PRURL: "https://github.com/o/r/pull/43"},
			},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return &feature.Service{Store: store, Git: &gitfake.Runner{Script: script}, GH: ghc}, store
}

func ghRestack() *ghfake.Client {
	c := ghfake.NewClient()
	c.SeedPR(gh.PR{Number: 42, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true, URL: "https://github.com/o/r/pull/42"})
	return c
}

func TestRestackIncrementsCount(t *testing.T) {
	// rebase --onto ok, push ok.
	svc, store := restackService(t, []gitfake.Response{{}, {}}, ghRestack())

	if err := svc.Restack(context.Background(), feature.RestackOpts{WorkerRef: "auth/erai/api", Onto: "develop"}); err != nil {
		t.Fatalf("Restack: %v", err)
	}
	reg, _ := store.Load()
	api := reg.Features[0].Workers[0]
	ui := reg.Features[0].Workers[1]
	if api.BaseBranch != "develop" {
		t.Errorf("api base = %q; want develop", api.BaseBranch)
	}
	if api.RestackCount != 1 {
		t.Errorf("api restack_count = %d; want 1", api.RestackCount)
	}
	// ui is a descendant whose effective base moved → also incremented.
	if ui.RestackCount != 1 {
		t.Errorf("ui (descendant) restack_count = %d; want 1", ui.RestackCount)
	}
}

func TestRestack_OnlyIncrementsNeverDecrements(t *testing.T) {
	ghc := ghRestack()
	svc, store := restackService(t, []gitfake.Response{{}, {}, {}, {}}, ghc)
	ctx := context.Background()

	if err := svc.Restack(ctx, feature.RestackOpts{WorkerRef: "auth/erai/api", Onto: "develop"}); err != nil {
		t.Fatalf("Restack 1: %v", err)
	}
	// Restack back to the original base — must NOT decrement.
	if err := svc.Restack(ctx, feature.RestackOpts{WorkerRef: "auth/erai/api", Onto: "main"}); err != nil {
		t.Fatalf("Restack 2: %v", err)
	}
	reg, _ := store.Load()
	if got := reg.Features[0].Workers[0].RestackCount; got != 2 {
		t.Errorf("restack_count = %d; want 2 (increments only, never decrements)", got)
	}
}

func TestRestack_ConflictAborts(t *testing.T) {
	// rebase --onto fails, abort ok.
	svc, _ := restackService(t, []gitfake.Response{{Err: stderrors.New("CONFLICT")}, {}}, ghRestack())
	if err := svc.Restack(context.Background(), feature.RestackOpts{WorkerRef: "auth/erai/api", Onto: "develop"}); !stderrors.Is(err, errors.ErrRebaseConflict) {
		t.Fatalf("err = %v; want ErrRebaseConflict", err)
	}
}

func TestRestack_UnknownWorker(t *testing.T) {
	svc, _ := restackService(t, nil, ghRestack())
	if err := svc.Restack(context.Background(), feature.RestackOpts{WorkerRef: "auth/erai/ghost", Onto: "develop"}); err == nil {
		t.Error("unknown worker: want error")
	}
}

// Ensure stack.ErrCycle is reachable through Restack on a cyclic stack.
func TestRestack_Cycle(t *testing.T) {
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	_ = store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{
				{User: "erai", Purpose: "api", Branch: "b/api", Worktree: "/wt/api", BaseBranch: "b/ui", Description: "a"},
				{User: "erai", Purpose: "ui", Branch: "b/ui", Worktree: "/wt/ui", BaseBranch: "b/api", Description: "u"},
			},
		}}}
		return nil
	})
	svc := &feature.Service{Store: store, Git: &gitfake.Runner{}, GH: ghfake.NewClient()}
	if err := svc.Restack(context.Background(), feature.RestackOpts{WorkerRef: "auth/erai/api", Onto: "main"}); !stderrors.Is(err, stack.ErrCycle) {
		t.Errorf("cyclic stack: err = %v; want stack.ErrCycle", err)
	}
}
