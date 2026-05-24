package feature_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/feature"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// conflictService seeds a feature with the given workers and a git fake
// scripted with one diff response per worker (in registry order).
func conflictService(t *testing.T, workers []registry.Worker, diffs ...string) *feature.Service {
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
	script := make([]gitfake.Response, len(diffs))
	for i, d := range diffs {
		script[i] = gitfake.Response{Stdout: []byte(d)}
	}
	return &feature.Service{Store: store, Git: &gitfake.Runner{Script: script}}
}

func twoWorkers() []registry.Worker {
	return []registry.Worker{
		{User: "erai", Purpose: "api", Branch: "b/api", Worktree: "/wt/api", BaseBranch: "main", Description: "api"},
		{User: "erai", Purpose: "ui", Branch: "b/ui", Worktree: "/wt/ui", BaseBranch: "main", Description: "ui"},
	}
}

func TestConflicts_DetectsOverlap(t *testing.T) {
	svc := conflictService(t, twoWorkers(), "a.go\nb.go\n", "b.go\nc.go\n")
	rep, err := svc.Conflicts(context.Background(), feature.ConflictsOpts{Feature: "auth"})
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	if len(rep.Conflicts) != 1 {
		t.Fatalf("conflicts = %d; want 1\n%+v", len(rep.Conflicts), rep.Conflicts)
	}
	c := rep.Conflicts[0]
	if c.WorkerA != "auth/erai/api" || c.WorkerB != "auth/erai/ui" {
		t.Errorf("pair = %s <-> %s; want api <-> ui", c.WorkerA, c.WorkerB)
	}
	if len(c.Files) != 1 || c.Files[0] != "b.go" {
		t.Errorf("overlap files = %v; want [b.go]", c.Files)
	}
}

func TestConflicts_NoOverlap(t *testing.T) {
	svc := conflictService(t, twoWorkers(), "a.go\n", "c.go\n")
	rep, err := svc.Conflicts(context.Background(), feature.ConflictsOpts{Feature: "auth"})
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	if len(rep.Conflicts) != 0 {
		t.Errorf("conflicts = %+v; want none", rep.Conflicts)
	}
	if !strings.Contains(rep.Text(), "No file conflicts") {
		t.Errorf("Text() = %q; want 'No file conflicts'", rep.Text())
	}
}

func TestConflicts_NeverResolves(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{{Stdout: []byte("a.go\n")}, {Stdout: []byte("a.go\n")}}}
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	_ = store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{Name: "auth", DefaultBaseBranch: "main", Workers: twoWorkers()}}}
		return nil
	})
	svc := &feature.Service{Store: store, Git: gitr}
	if _, err := svc.Conflicts(context.Background(), feature.ConflictsOpts{Feature: "auth"}); err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	// Only read-only `diff --name-only` calls — no merge/checkout/write.
	for _, call := range gitr.Calls {
		joined := call.Name + " " + strings.Join(call.Args, " ")
		if !strings.Contains(joined, "diff") || !strings.Contains(joined, "--name-only") {
			t.Errorf("non-read git call during conflict scan: %q", joined)
		}
		for _, forbidden := range []string{"merge", "checkout", "rebase", "push", "commit", "reset"} {
			if strings.Contains(joined, forbidden) {
				t.Errorf("conflict scan must never %s: %q", forbidden, joined)
			}
		}
	}
}

func TestConflicts_JSON(t *testing.T) {
	svc := conflictService(t, twoWorkers(), "a.go\nb.go\n", "b.go\n")
	rep, err := svc.Conflicts(context.Background(), feature.ConflictsOpts{Feature: "auth"})
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	js, err := rep.MarshalReport()
	if err != nil {
		t.Fatalf("MarshalReport: %v", err)
	}
	for _, want := range []string{`"feature": "auth"`, `"worker_a": "auth/erai/api"`, `"files"`, `"b.go"`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("json missing %q:\n%s", want, js)
		}
	}
}

func TestConflicts_UnknownFeature(t *testing.T) {
	svc := conflictService(t, twoWorkers())
	if _, err := svc.Conflicts(context.Background(), feature.ConflictsOpts{Feature: "ghost"}); err == nil {
		t.Error("unknown feature: want error")
	}
}
