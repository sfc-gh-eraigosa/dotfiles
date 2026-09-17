package feature_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/tmpl"
)

// doneService seeds a single-worker feature with FEATURE.md on disk and
// returns the service + store + the feature dir. featureMD is written
// verbatim to <root>/octo/proj/auth/FEATURE.md.
func doneService(t *testing.T, featureMD string, status string, workers []registry.Worker) (*feature.Service, *registry.Store, string) {
	t.Helper()
	root := t.TempDir()
	featDir := filepath.Join(root, "octo/proj", "auth")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "FEATURE.md"), []byte(featureMD), 0o644); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(filepath.Join(root, "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", StartedAt: "2026-05-21T12:00:00Z", DefaultBaseBranch: "main", Description: "login work",
			Workers: workers,
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := &feature.Service{
		Store: store, Backend: &fakeBackend{},
		Git:          &gitfake.Runner{Default: gitfake.Response{Stdout: []byte(status)}},
		WorktreeRoot: root, NWO: "octo/proj",
	}
	return svc, store, featDir
}

func cleanFeatureMD(t *testing.T) string {
	t.Helper()
	md, err := tmpl.RenderEmbeddedFeature(tmpl.FeatureData{Name: "auth", Description: "login work", StartedAt: "2026-05-21T12:00:00Z", BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	return md
}

func oneWorker() []registry.Worker {
	return []registry.Worker{{User: "erai", Purpose: "api", Branch: "feature/auth/erai/api", Worktree: "/wt/api", BaseBranch: "main", Description: "api", PRState: "merged"}}
}

// TestDone_RemovesMetaDir proves teardown cleans up the worker's external
// .gss-meta/<leaf>/ dir (issue #132) without disturbing a sibling worker's
// meta dir under the same (feature,user).
func TestDone_RemovesMetaDir(t *testing.T) {
	root := t.TempDir()
	wtA := filepath.Join(root, "octo/proj/auth/erai/api")
	wtB := filepath.Join(root, "octo/proj/auth/erai/impl")
	for _, wt := range []string{wtA, wtB} {
		if err := os.MkdirAll(filepath.Dir(feature.WorkerMetaPath(wt)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(feature.WorkerMetaPath(wt), []byte("# worker\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	featDir := filepath.Join(root, "octo/proj", "auth")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "FEATURE.md"), []byte(cleanFeatureMD(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(filepath.Join(root, "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", StartedAt: "2026-05-21T12:00:00Z", DefaultBaseBranch: "main", Description: "login work",
			Workers: []registry.Worker{
				{User: "erai", Purpose: "api", Branch: "feature/auth/erai/api", Worktree: wtA, BaseBranch: "main", PRState: "merged"},
				{User: "erai", Purpose: "impl", Branch: "feature/auth/erai/impl", Worktree: wtB, BaseBranch: "main", PRState: "merged"},
			},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := &feature.Service{
		Store: store, Backend: &fakeBackend{},
		Git:          &gitfake.Runner{Default: gitfake.Response{Stdout: []byte("")}}, // clean porcelain
		WorktreeRoot: root, NWO: "octo/proj",
	}
	if _, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatalf("Done: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(feature.WorkerMetaPath(wtA))); !os.IsNotExist(err) {
		t.Errorf("api meta dir should be removed after done; stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Dir(feature.WorkerMetaPath(wtB))); err != nil {
		t.Errorf("sibling impl meta dir must survive; got err=%v", err)
	}
}

func TestDoneOnEmptyFeatureMatchingTemplate(t *testing.T) {
	svc, store, featDir := doneService(t, cleanFeatureMD(t), "", oneWorker())
	res, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if !res.FeatureDeleted {
		t.Error("matching template → feature should be deleted")
	}
	reg, _ := store.Load()
	if len(reg.Features) != 0 {
		t.Errorf("feature row not removed: %+v", reg.Features)
	}
	if _, err := os.Stat(filepath.Join(featDir, "FEATURE.md")); !os.IsNotExist(err) {
		t.Error("FEATURE.md should have been deleted")
	}
}

func TestDoneOnEmptyFeatureWithEdits(t *testing.T) {
	edited := cleanFeatureMD(t) + "\n## Decisions & notes\n- we chose X over Y\n"
	svc, store, featDir := doneService(t, edited, "", oneWorker())
	res, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if res.FeatureDeleted {
		t.Error("edited FEATURE.md → feature must be retained")
	}
	if res.RetainedNotice == "" {
		t.Error("expected a stderr notice naming the orphaned feature")
	}
	reg, _ := store.Load()
	if len(reg.Features) != 1 || len(reg.Features[0].Workers) != 0 {
		t.Errorf("feature should be retained with 0 workers: %+v", reg.Features)
	}
	if _, err := os.Stat(filepath.Join(featDir, "FEATURE.md")); err != nil {
		t.Error("edited FEATURE.md should be preserved")
	}
}

func TestDoneWhitespaceOnlyDiffStillDeletes(t *testing.T) {
	// Same template content but with trailing whitespace + extra final newline.
	svc, _, _ := doneService(t, cleanFeatureMD(t)+"   \n\n\n", "", oneWorker())
	res, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Done: %v", err)
	}
	if !res.FeatureDeleted {
		t.Error("whitespace-only difference must not count as an edit")
	}
}

func TestDone_RefusesDirty(t *testing.T) {
	svc, _, _ := doneService(t, cleanFeatureMD(t), " M a.go\n", oneWorker())
	if _, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"}); !stderrors.Is(err, errors.ErrDirtyWorktree) {
		t.Errorf("dirty worktree: err = %v; want ErrDirtyWorktree", err)
	}
}

func TestDone_RefusesDependents(t *testing.T) {
	workers := []registry.Worker{
		{User: "erai", Purpose: "api", Branch: "feature/auth/erai/api", Worktree: "/wt/api", BaseBranch: "main", Description: "api", PRState: "merged"},
		{User: "erai", Purpose: "ui", Branch: "feature/auth/erai/ui", Worktree: "/wt/ui", BaseBranch: "feature/auth/erai/api", Description: "ui"},
	}
	svc, _, _ := doneService(t, cleanFeatureMD(t), "", workers)
	if _, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"}); err == nil {
		t.Error("worker with dependents: want refusal without --force")
	}
}

func TestDone_ForceRemovesDirty(t *testing.T) {
	svc, store, _ := doneService(t, cleanFeatureMD(t), " M a.go\n", oneWorker())
	if _, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api", Force: true}); err != nil {
		t.Fatalf("Done --force: %v", err)
	}
	reg, _ := store.Load()
	if len(reg.Features) != 0 {
		t.Errorf("--force should remove worker (and empty feature); got %+v", reg.Features)
	}
}

// TestDone_RegistryUpdateFailureAfterWorktreeRemovalIsDiagnosable pins the
// dotfiles#98 fix.
//
// The issue's primary suggestion was to swap the order (registry first,
// then worktree), reasoning that a registry-only removal followed by a
// failed Backend.Remove "can be cleaned up manually or by reconcile". That
// reasoning doesn't hold: registry.Reconcile only detects a row pointing at
// a MISSING worktree (its stale-worktree-dropped check) — never the
// reverse. Swapping the order would trade the current, self-healing
// failure mode (a stale row `audit --repair` already drops) for a
// non-recoverable one (a live worktree, still attached to git, that gss no
// longer tracks at all and no command can find again).
//
// So the order stays (worktree, then registry) and this test pins the
// issue's own documented fallback instead: when Store.Update fails AFTER
// the worktree is already gone, the returned error must say so explicitly
// — the caller must not mistake a registry-only failure for "teardown
// failed entirely" when the worktree in fact already vanished.
func TestDone_RegistryUpdateFailureAfterWorktreeRemovalIsDiagnosable(t *testing.T) {
	svc, store, _ := doneService(t, cleanFeatureMD(t), "", oneWorker())
	// Force the LATER Store.Update (registry row removal) — not the
	// EARLIER Store.Load at the top of Done — to fail with ErrLockHeld.
	// Grabbing the lock externally for the whole test would block Load
	// too (it also takes the lock, shared), so the competing holder is
	// acquired precisely inside Backend.Remove's fake, synchronously,
	// right after the worktree removal is recorded and right before Done
	// reaches Store.Update.
	store.LockTimeout = 50 * time.Millisecond
	holder := flock.New(store.LockPath)
	be := svc.Backend.(*fakeBackend)
	be.afterRemove = func() {
		if ok, err := holder.TryLock(); err != nil || !ok {
			t.Fatalf("test setup: could not hold the registry lock externally: ok=%v err=%v", ok, err)
		}
	}
	defer func() { _ = holder.Unlock() }()

	_, err := svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	if err == nil {
		t.Fatal("Store.Update contended: want an error")
	}
	if !stderrors.Is(err, errors.ErrLockHeld) {
		t.Errorf("err = %v; want it to still wrap ErrLockHeld", err)
	}
	if !strings.Contains(err.Error(), "already removed") {
		t.Errorf("err = %v; want it to say the worktree was already removed (partial-teardown diagnostic)", err)
	}
	if len(be.removed) != 1 || be.removed[0] != "/wt/api" {
		t.Errorf("backend.Remove calls = %v; want exactly one for /wt/api — the worktree really was removed despite the registry error", be.removed)
	}
}
