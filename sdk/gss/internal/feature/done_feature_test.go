package feature_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// seedFeature writes a feature row with the given workers plus a FEATURE.md
// whose contents are supplied by md, and returns the service + feature dir.
func seedFeature(t *testing.T, md string, workers []registry.Worker) (*feature.Service, *registry.Store, string) {
	t.Helper()
	root := t.TempDir()
	featDir := filepath.Join(root, "octo/proj", "auth")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "FEATURE.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(filepath.Join(root, "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", StartedAt: "2026-05-21T12:00:00Z", DefaultBaseBranch: "main",
			Description: "login work", Workers: workers,
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return &feature.Service{Store: store, WorktreeRoot: root, NWO: "octo/proj"}, store, featDir
}

func featureCount(t *testing.T, store *registry.Store) int {
	t.Helper()
	reg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return len(reg.Features)
}

// TestDoneFeature_RemovesEmptyTemplateCleanFeature is the gap this closes:
// `gss feature start` with no worker had no inverse, so an abandoned feature
// could only be cleared by hand-editing registry.json.
func TestDoneFeature_RemovesEmptyTemplateCleanFeature(t *testing.T) {
	svc, store, featDir := seedFeature(t, cleanFeatureMD(t), nil)

	res, err := svc.DoneFeature("auth", false)
	if err != nil {
		t.Fatalf("DoneFeature: %v", err)
	}
	if !res.FeatureDeleted {
		t.Error("FeatureDeleted = false; want true")
	}
	if n := featureCount(t, store); n != 0 {
		t.Errorf("features remaining = %d; want 0", n)
	}
	if _, err := os.Stat(filepath.Join(featDir, "FEATURE.md")); !os.IsNotExist(err) {
		t.Error("FEATURE.md still present")
	}
}

// TestDoneFeature_RefusesWhenWorkersRemain keeps worktree-owning teardown in
// `feature done <worker-ref>`, which is the verb that removes the inode.
func TestDoneFeature_RefusesWhenWorkersRemain(t *testing.T) {
	svc, store, _ := seedFeature(t, cleanFeatureMD(t), oneWorker())

	if _, err := svc.DoneFeature("auth", false); err == nil {
		t.Fatal("DoneFeature with workers: want error")
	}
	if n := featureCount(t, store); n != 1 {
		t.Errorf("feature was removed despite refusal (count=%d)", n)
	}
}

// TestDoneFeature_RefusesEditedFeatureMD protects the decisions recorded in
// FEATURE.md — the same guard Done applies on empty-feature cleanup.
func TestDoneFeature_RefusesEditedFeatureMD(t *testing.T) {
	svc, store, _ := seedFeature(t, cleanFeatureMD(t)+"\n\nWe chose X over Y.\n", nil)

	if _, err := svc.DoneFeature("auth", false); err == nil {
		t.Fatal("DoneFeature with edited FEATURE.md: want error")
	}
	if n := featureCount(t, store); n != 1 {
		t.Errorf("feature removed despite edits (count=%d)", n)
	}
}

func TestDoneFeature_ForceOverridesBothGuards(t *testing.T) {
	svc, store, _ := seedFeature(t, cleanFeatureMD(t)+"\n\nedited.\n", oneWorker())

	if _, err := svc.DoneFeature("auth", true); err != nil {
		t.Fatalf("DoneFeature --force: %v", err)
	}
	if n := featureCount(t, store); n != 0 {
		t.Errorf("features remaining = %d; want 0", n)
	}
}

func TestDoneFeature_UnknownFeature(t *testing.T) {
	svc, _, _ := seedFeature(t, cleanFeatureMD(t), nil)
	if _, err := svc.DoneFeature("nope", false); err == nil {
		t.Fatal("DoneFeature(nope): want error")
	}
}
