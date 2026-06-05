package feature_test

import (
	"context"
	stderrors "errors"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

type fakeApprover struct {
	err   error
	calls int
}

func (f *fakeApprover) Verify(_ context.Context, _ string, _ bool) error { f.calls++; return f.err }

// readyService seeds a feature (optionally a stacked parent) and wires the
// gh fake + an approver.
func readyService(t *testing.T, ap *fakeApprover, workers []registry.Worker, ghc *ghfake.Client) (*feature.Service, *registry.Store) {
	t.Helper()
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main", Workers: workers,
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return &feature.Service{Store: store, GH: ghc, Approval: ap}, store
}

func bottomWorker() registry.Worker {
	return registry.Worker{User: "erai", Purpose: "api", Branch: "feature/auth/erai/api", BaseBranch: "main",
		Description: "api", PRURL: "https://github.com/o/r/pull/42", PRState: "draft"}
}

func ghWithReadyable(num int) *ghfake.Client {
	c := ghfake.NewClient()
	c.SeedPR(gh.PR{Number: num, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true, URL: "https://github.com/o/r/pull/42"})
	return c
}

func TestPromoteReady_RefusesWithoutToken(t *testing.T) {
	ap := &fakeApprover{err: errors.ErrApprovalTokenMissing}
	ghc := ghWithReadyable(42)
	svc, _ := readyService(t, ap, []registry.Worker{bottomWorker()}, ghc)

	err := svc.PromoteReady(context.Background(), feature.ReadyOpts{WorkerRef: "auth/erai/api"})
	if !stderrors.Is(err, errors.ErrPRReadyNeedsToken) {
		t.Fatalf("err = %v; want ErrPRReadyNeedsToken", err)
	}
	// Must NOT have promoted.
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPRReady {
			t.Error("promotion happened without a token")
		}
	}
}

func TestPromoteReady_NoApproverConfigured(t *testing.T) {
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	_ = store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{Name: "auth", DefaultBaseBranch: "main", Workers: []registry.Worker{bottomWorker()}}}}
		return nil
	})
	svc := &feature.Service{Store: store, GH: ghWithReadyable(42)} // no Approval
	if err := svc.PromoteReady(context.Background(), feature.ReadyOpts{WorkerRef: "auth/erai/api"}); !stderrors.Is(err, errors.ErrPRReadyNeedsToken) {
		t.Errorf("no approver: err = %v; want ErrPRReadyNeedsToken", err)
	}
}

func TestPromoteReady_PromotesWithToken(t *testing.T) {
	ap := &fakeApprover{}
	ghc := ghWithReadyable(42)
	svc, store := readyService(t, ap, []registry.Worker{bottomWorker()}, ghc)

	if err := svc.PromoteReady(context.Background(), feature.ReadyOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatalf("PromoteReady: %v", err)
	}
	if ap.calls != 1 {
		t.Errorf("approval calls = %d; want 1 (token consumed)", ap.calls)
	}
	promoted := false
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPRReady && c.Num == 42 {
			promoted = true
		}
	}
	if !promoted {
		t.Error("expected gh PRReady(42)")
	}
	reg, _ := store.Load()
	if reg.Features[0].Workers[0].PRState != "open" {
		t.Errorf("pr_state = %q; want open after promote", reg.Features[0].Workers[0].PRState)
	}
}

func stackedWorkers() []registry.Worker {
	return []registry.Worker{
		{User: "erai", Purpose: "api", Branch: "feature/auth/erai/api", BaseBranch: "main",
			Description: "api", PRURL: "https://github.com/o/r/pull/42", PRState: "draft"},
		{User: "erai", Purpose: "ui", Branch: "feature/auth/erai/ui", BaseBranch: "feature/auth/erai/api",
			Description: "ui", PRURL: "https://github.com/o/r/pull/43", PRState: "draft"},
	}
}

func TestPromoteReady_RefusesNonBottomWithDraftParent(t *testing.T) {
	ap := &fakeApprover{}
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 43, Head: "feature/auth/erai/ui", State: "OPEN", IsDraft: true, URL: "https://github.com/o/r/pull/43"})
	svc, _ := readyService(t, ap, stackedWorkers(), ghc)

	// ui's parent (api) is still draft → refuse without --force.
	err := svc.PromoteReady(context.Background(), feature.ReadyOpts{WorkerRef: "auth/erai/ui"})
	if err == nil {
		t.Fatal("non-bottom with draft parent: want refusal")
	}
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPRReady {
			t.Error("must not promote while parent is draft")
		}
	}
}

func TestPromoteReady_ForceOverridesParentGuard(t *testing.T) {
	ap := &fakeApprover{}
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 43, Head: "feature/auth/erai/ui", State: "OPEN", IsDraft: true, URL: "https://github.com/o/r/pull/43"})
	svc, _ := readyService(t, ap, stackedWorkers(), ghc)

	if err := svc.PromoteReady(context.Background(), feature.ReadyOpts{WorkerRef: "auth/erai/ui", Force: true}); err != nil {
		t.Fatalf("PromoteReady --force: %v", err)
	}
	promoted := false
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPRReady && c.Num == 43 {
			promoted = true
		}
	}
	if !promoted {
		t.Error("--force should promote despite draft parent")
	}
}

func TestPromoteReady_NoPR(t *testing.T) {
	ap := &fakeApprover{}
	w := bottomWorker()
	w.PRURL = ""
	svc, _ := readyService(t, ap, []registry.Worker{w}, ghfake.NewClient())
	if err := svc.PromoteReady(context.Background(), feature.ReadyOpts{WorkerRef: "auth/erai/api"}); err == nil {
		t.Error("worker without PR: want error")
	}
}
