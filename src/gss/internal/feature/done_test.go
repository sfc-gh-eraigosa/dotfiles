package feature_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/feature"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
	"github.com/wenlock/dotfiles/gss/internal/registry"
	"github.com/wenlock/dotfiles/gss/internal/tmpl"
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
