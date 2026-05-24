package feature_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/feature"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// fakeObserver answers the audit's read-only queries. Everything is healthy
// by default; a test marks negatives by adding to the maps. PRs default to a
// base matching the worker (seeded by healthyObserver); absent => 404.
type fakeObserver struct {
	missingWorktrees map[string]bool // worktree path -> missing
	missingBranches  map[string]bool // branch -> missing
	unreachable      map[string]bool // branch -> base unreachable
	prs              map[int]gh.PR   // present PRs; absent => 404
}

func (o *fakeObserver) WorktreeExists(p string) bool                  { return !o.missingWorktrees[p] }
func (o *fakeObserver) BranchExists(_ context.Context, b string) bool { return !o.missingBranches[b] }
func (o *fakeObserver) BaseReachable(_ context.Context, b, _ string) bool {
	return !o.unreachable[b]
}
func (o *fakeObserver) PRView(_ context.Context, n int) (gh.PR, bool) {
	pr, ok := o.prs[n]
	return pr, ok
}

// healthyObserver seeds a clean observer: every worker's worktree+branch
// present, base reachable, and (if it has a PR) the PR exists with a matching
// base.
func healthyObserver(workers []registry.Worker) *fakeObserver {
	o := &fakeObserver{
		missingWorktrees: map[string]bool{}, missingBranches: map[string]bool{},
		unreachable: map[string]bool{}, prs: map[int]gh.PR{},
	}
	for _, w := range workers {
		if w.PRURL != "" {
			// IsDraft:true matches the worker's seeded pr_state "draft" so a
			// clean fixture produces no pr-state-stale reconcile finding.
			o.prs[prNum(w.PRURL)] = gh.PR{Number: prNum(w.PRURL), Base: w.BaseBranch, Head: w.Branch, State: "OPEN", IsDraft: true}
		}
	}
	return o
}

// auditService seeds a single feature (default base main) and wires the
// injected observer.
func auditService(t *testing.T, workers []registry.Worker, obs feature.Observer) (*feature.Service, *registry.Store) {
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
	return &feature.Service{Store: store, Observe: obs}, store
}

func aw(purpose, base, prurl string) registry.Worker {
	return registry.Worker{
		User: "erai", Purpose: purpose, Branch: "feature/auth/erai/" + purpose,
		Worktree: "/wt/" + purpose, BaseBranch: base, Description: purpose, PRURL: prurl, PRState: "draft",
	}
}

func findFinding(fs []feature.Finding, check string) *feature.Finding {
	for i := range fs {
		if fs[i].Check == check {
			return &fs[i]
		}
	}
	return nil
}

func TestAuditClean(t *testing.T) {
	workers := []registry.Worker{aw("api", "main", "https://github.com/o/r/pull/42")}
	svc, _ := auditService(t, workers, healthyObserver(workers))
	rep, err := svc.Audit(context.Background(), feature.AuditOpts{})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("clean registry → want no findings; got %+v", rep.Findings)
	}
	if rep.HasErrors() {
		t.Error("clean registry must not report errors")
	}
}

func TestAuditWorktreeMissingAndRepair(t *testing.T) {
	workers := []registry.Worker{aw("api", "main", "https://github.com/o/r/pull/42")}
	obs := healthyObserver(workers)
	obs.missingWorktrees["/wt/api"] = true
	svc, store := auditService(t, workers, obs)

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{})
	if f := findFinding(rep.Findings, "worktree-missing"); f == nil || f.Severity != feature.SevError {
		t.Fatalf("want error-severity worktree-missing; got %+v", rep.Findings)
	}
	// read-only must not have changed the registry.
	if reg, _ := store.Load(); len(reg.Features[0].Workers) != 1 {
		t.Error("read-only audit dropped a row")
	}
	// repair drops the row.
	rep2, err := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	if err != nil {
		t.Fatalf("Audit --repair: %v", err)
	}
	if rep2.Repaired != 1 {
		t.Errorf("repaired = %d; want 1", rep2.Repaired)
	}
	reg, _ := store.Load()
	if len(reg.Features[0].Workers) != 0 {
		t.Errorf("repair should drop the dead worker; got %+v", reg.Features[0].Workers)
	}
}

func TestAuditBranchMissingAndRepair(t *testing.T) {
	workers := []registry.Worker{aw("api", "main", "")}
	obs := healthyObserver(workers)
	obs.missingBranches["feature/auth/erai/api"] = true
	svc, store := auditService(t, workers, obs)

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	if f := findFinding(rep.Findings, "branch-missing"); f == nil || f.Severity != feature.SevError {
		t.Fatalf("want branch-missing error; got %+v", rep.Findings)
	}
	if reg, _ := store.Load(); len(reg.Features[0].Workers) != 0 {
		t.Error("repair should drop a worker whose branch is gone")
	}
}

func TestAuditBaseUnreachableNotRepaired(t *testing.T) {
	workers := []registry.Worker{aw("api", "main", "")}
	obs := healthyObserver(workers)
	obs.unreachable["feature/auth/erai/api"] = true
	svc, store := auditService(t, workers, obs)

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	f := findFinding(rep.Findings, "base-unreachable")
	if f == nil || f.Severity != feature.SevError {
		t.Fatalf("want base-unreachable error; got %+v", rep.Findings)
	}
	if f.Repaired {
		t.Error("base-unreachable must NOT be auto-repaired (needs restack)")
	}
	if reg, _ := store.Load(); len(reg.Features[0].Workers) != 1 {
		t.Error("repair must not drop a worker with an unreachable base")
	}
}

func TestAuditPR404AndRepair(t *testing.T) {
	workers := []registry.Worker{aw("api", "main", "https://github.com/o/r/pull/42")}
	obs := healthyObserver(workers)
	delete(obs.prs, 42) // 404
	svc, store := auditService(t, workers, obs)

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	if f := findFinding(rep.Findings, "pr-404"); f == nil || f.Severity != feature.SevWarn {
		t.Fatalf("want pr-404 warn; got %+v", rep.Findings)
	}
	reg, _ := store.Load()
	w := reg.Features[0].Workers[0]
	if w.PRURL != "" || w.PRState != "" {
		t.Errorf("repair should clear pr_url/pr_state; got %q/%q", w.PRURL, w.PRState)
	}
}

func TestAuditPRBaseDivergedAndRepair(t *testing.T) {
	workers := []registry.Worker{aw("api", "main", "https://github.com/o/r/pull/42")}
	obs := healthyObserver(workers)
	obs.prs[42] = gh.PR{Number: 42, Base: "develop", Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true}
	svc, store := auditService(t, workers, obs)

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	if f := findFinding(rep.Findings, "pr-base-diverged"); f == nil {
		t.Fatalf("want pr-base-diverged; got %+v", rep.Findings)
	}
	if reg, _ := store.Load(); reg.Features[0].Workers[0].BaseBranch != "develop" {
		t.Errorf("repair should adopt the PR base develop; got %q", reg.Features[0].Workers[0].BaseBranch)
	}
}

func TestAuditStaleBaseAndRepair(t *testing.T) {
	// ui's base points at a branch no worker owns and isn't the default.
	workers := []registry.Worker{aw("ui", "feature/auth/erai/ghost", "")}
	svc, store := auditService(t, workers, healthyObserver(workers))

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	if f := findFinding(rep.Findings, "stale-base"); f == nil || f.Severity != feature.SevWarn {
		t.Fatalf("want stale-base warn; got %+v", rep.Findings)
	}
	if reg, _ := store.Load(); reg.Features[0].Workers[0].BaseBranch != "main" {
		t.Errorf("repair should reset stale base to default main; got %q", reg.Features[0].Workers[0].BaseBranch)
	}
}

// Resolution #8 forbids branching on spawned_by, so audit deliberately does
// NOT validate it. A worker with an empty spawned_by engine produces no
// finding (TestSpawnedByInformationalOnly guards the no-branch rule itself).
func TestAuditIgnoresSpawnedBy(t *testing.T) {
	w := aw("api", "main", "")
	w.SpawnedBy = &registry.SpawnedBy{Engine: ""}
	workers := []registry.Worker{w}
	svc, _ := auditService(t, workers, healthyObserver(workers))

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{})
	if findFinding(rep.Findings, "spawned-by-invalid") != nil {
		t.Error("audit must not branch on spawned_by (resolution #8)")
	}
}

func TestAuditSchemaNewerSurfacedNotRepaired(t *testing.T) {
	// Write a registry.json with a schema version newer than supported; the
	// store rejects it at load, and audit surfaces it as an error finding.
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":99,"features":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &feature.Service{Store: registry.NewStore(path), Observe: &fakeObserver{}}

	rep, err := svc.Audit(context.Background(), feature.AuditOpts{Repair: true})
	if err != nil {
		t.Fatalf("Audit should surface (not error out) on newer schema: %v", err)
	}
	if f := findFinding(rep.Findings, "schema-newer"); f == nil || f.Severity != feature.SevError {
		t.Fatalf("want schema-newer error finding; got %+v", rep.Findings)
	}
	if rep.Repaired != 0 {
		t.Error("a newer schema must never be repaired")
	}
	if !rep.HasErrors() {
		t.Error("schema-newer should make HasErrors true")
	}
}

func TestAuditFeatureFilter(t *testing.T) {
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	_ = store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{
			{Name: "auth", DefaultBaseBranch: "main", Workers: []registry.Worker{
				{User: "erai", Purpose: "api", Branch: "b/api", Worktree: "/wt/api", BaseBranch: "main", Description: "a"}}},
			{Name: "billing", DefaultBaseBranch: "main", Workers: []registry.Worker{
				{User: "erai", Purpose: "x", Branch: "b/x", Worktree: "/wt/x", BaseBranch: "main", Description: "x"}}},
		}}
		return nil
	})
	obs := &fakeObserver{
		missingWorktrees: map[string]bool{"/wt/api": true, "/wt/x": true},
		missingBranches:  map[string]bool{}, unreachable: map[string]bool{}, prs: map[int]gh.PR{},
	}
	svc := &feature.Service{Store: store, Observe: obs}

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{Feature: "auth"})
	for _, f := range rep.Findings {
		if !strings.HasPrefix(f.Worker, "auth/") {
			t.Errorf("--feature auth must only audit auth workers; got finding for %q", f.Worker)
		}
	}
	if len(rep.Findings) == 0 {
		t.Error("expected the auth worktree-missing finding")
	}
}

func TestAuditJSONRenders(t *testing.T) {
	workers := []registry.Worker{aw("api", "main", "https://github.com/o/r/pull/42")}
	obs := healthyObserver(workers)
	delete(obs.prs, 42)
	svc, _ := auditService(t, workers, obs)

	rep, _ := svc.Audit(context.Background(), feature.AuditOpts{})
	data, err := rep.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("audit JSON is not valid: %v\n%s", err, data)
	}
	if _, ok := got["findings"]; !ok {
		t.Error("audit JSON missing findings key")
	}
}
