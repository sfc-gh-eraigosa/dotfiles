package repo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh/fake"
)

// ghJSONResponse builds a fake gh pr view JSON response.
func ghJSONResponse(number int, state string) []byte {
	b, _ := json.Marshal(map[string]interface{}{"number": number, "state": state})
	return b
}

// isolateGhCache redirects XDG_CACHE_HOME to a fresh temp dir for the
// duration of the test. This prevents gh.PR's on-disk file cache from
// serving stale hits that would suppress the gh.Runner call we want to
// observe, and prevents tests from writing cache entries that pollute
// each other (or subsequent test runs).
func isolateGhCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

// ---------------------------------------------------------------------------
// Registry-hit path: gh should NOT be consulted
// ---------------------------------------------------------------------------

func TestPR_RegistryHit_GhNotCalled(t *testing.T) {
	ghSpy := &ghfake.Runner{}
	// The registry fixture has a worker with pr_url → PR#21, OPEN.
	toplevel := "/home/wenlock/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gsl/edward-raigosa/impl"
	branch := "feature/gsl/edward-raigosa/impl"

	info, err := PR(context.Background(), ghSpy, branch, toplevel, testdataPath("registry.json"))
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("PR: expected non-nil RepoInfo")
	}
	if info.PRNumber != 21 {
		t.Errorf("PRNumber: want 21, got %d", info.PRNumber)
	}
	if info.PRState != "OPEN" {
		t.Errorf("PRState: want OPEN, got %q", info.PRState)
	}
	if info.FeatureName != "gsl" {
		t.Errorf("FeatureName: want gsl, got %q", info.FeatureName)
	}

	// CRITICAL: gh must not have been called (registry-hit avoids any network).
	if ghSpy.CallCount() != 0 {
		t.Errorf("gh.Runner was called %d time(s); want 0 (registry-hit path)", ghSpy.CallCount())
	}
}

// ---------------------------------------------------------------------------
// Worker with no pr_url → gh fallback
// ---------------------------------------------------------------------------

func TestPR_WorkerNoPRUrl_GhFallback(t *testing.T) {
	isolateGhCache(t)
	ghRunner := &ghfake.Runner{
		Default: ghfake.Response{Stdout: ghJSONResponse(99, "DRAFT")},
	}
	// Use a temp registry with a worker that has no pr_url so we exercise the
	// fallback path. The branch name is arbitrary — isolateGhCache ensures no
	// stale cache entry can short-circuit the Runner call.
	tmpDir := t.TempDir()
	branch := "feature/gsl/another-user/review-noprurltest"
	toplevel := "/home/wenlock/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gsl/another-user/review"
	regPath := tmpDir + "/registry.json"
	regJSON := fmt.Sprintf(`{
  "schema_version": 1,
  "features": [{
    "name": "gsl",
    "workers": [{
      "branch": %q,
      "worktree": %q
    }]
  }]
}`, branch, toplevel)
	if err := os.WriteFile(regPath, []byte(regJSON), 0o644); err != nil {
		t.Fatalf("write temp registry: %v", err)
	}

	info, err := PR(context.Background(), ghRunner, branch, toplevel, regPath)
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("PR: expected non-nil RepoInfo from gh fallback")
	}
	if info.PRNumber != 99 {
		t.Errorf("PRNumber: want 99 (from gh), got %d", info.PRNumber)
	}
	if info.PRState != "DRAFT" {
		t.Errorf("PRState: want DRAFT (from gh), got %q", info.PRState)
	}
	// gh should have been consulted (registry match but no HasPR).
	if ghRunner.CallCount() == 0 {
		t.Error("gh.Runner was NOT called; want at least 1 call for fallback")
	}
	// Feature name propagated from the registry match.
	if info.FeatureName != "gsl" {
		t.Errorf("FeatureName: want gsl (from registry match), got %q", info.FeatureName)
	}
}

// ---------------------------------------------------------------------------
// Bumped schema_version → registry ignored → gh fallback
// ---------------------------------------------------------------------------

func TestPR_BumpedSchema_GhFallback(t *testing.T) {
	isolateGhCache(t)
	ghRunner := &ghfake.Runner{
		Default: ghfake.Response{Stdout: ghJSONResponse(55, "OPEN")},
	}

	info, err := PR(context.Background(), ghRunner, "test-branch-bumped-schema", "/some/toplevel", testdataPath("registry_v2.json"))
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("PR: expected non-nil RepoInfo (gh fallback)")
	}
	if info.PRNumber != 55 {
		t.Errorf("PRNumber: want 55 (gh), got %d", info.PRNumber)
	}
	// CRITICAL: gh MUST have been consulted when the schema is bumped.
	if ghRunner.CallCount() == 0 {
		t.Error("gh.Runner was NOT called; want ≥1 call for bumped-schema fallback")
	}
}

// ---------------------------------------------------------------------------
// Missing registry → gh fallback
// ---------------------------------------------------------------------------

func TestPR_MissingRegistry_GhFallback(t *testing.T) {
	isolateGhCache(t)
	ghRunner := &ghfake.Runner{
		Default: ghfake.Response{Stdout: ghJSONResponse(77, "CLOSED")},
	}

	info, err := PR(context.Background(), ghRunner, "test-branch-missing-registry", "/some/toplevel", "/no/such/registry.json")
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("PR: expected non-nil RepoInfo from gh")
	}
	if info.PRNumber != 77 {
		t.Errorf("PRNumber: want 77 (gh), got %d", info.PRNumber)
	}
	if ghRunner.CallCount() == 0 {
		t.Error("gh.Runner was NOT called; want ≥1 call for missing-registry fallback")
	}
}

// ---------------------------------------------------------------------------
// No match in registry → gh fallback
// ---------------------------------------------------------------------------

func TestPR_NoRegistryMatch_GhFallback(t *testing.T) {
	isolateGhCache(t)
	ghRunner := &ghfake.Runner{
		Default: ghfake.Response{Stdout: ghJSONResponse(33, "OPEN")},
	}

	info, err := PR(context.Background(), ghRunner, "test-branch-no-registry-match", "/no/such/toplevel", testdataPath("registry.json"))
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("PR: expected non-nil RepoInfo from gh")
	}
	if info.PRNumber != 33 {
		t.Errorf("PRNumber: want 33, got %d", info.PRNumber)
	}
	if ghRunner.CallCount() == 0 {
		t.Error("gh.Runner NOT called; want ≥1 for no-match fallback")
	}
}

// ---------------------------------------------------------------------------
// gh returns nil (no PR) on fallback
// ---------------------------------------------------------------------------

func TestPR_GhReturnsNil(t *testing.T) {
	isolateGhCache(t)
	// gh returns empty output → gh.PR returns nil → repo.PR returns nil.
	ghRunner := &ghfake.Runner{
		Default: ghfake.Response{Stdout: nil},
	}

	info, err := PR(context.Background(), ghRunner, "no-branch-gh-nil", "/no/path", "/no/registry.json")
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("PR: want nil RepoInfo (no PR), got %+v", info)
	}
}

// ---------------------------------------------------------------------------
// Registry match by branch (not toplevel) when toplevel doesn't match
// ---------------------------------------------------------------------------

func TestPR_RegistryHit_ByBranch(t *testing.T) {
	ghSpy := &ghfake.Runner{}

	// Use a toplevel that doesn't match but a branch that does.
	info, err := PR(
		context.Background(), ghSpy,
		"feature/gsl/edward-raigosa/impl", // matching branch
		"/not/the/worktree/path",          // non-matching toplevel
		testdataPath("registry.json"),
	)
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("PR: expected non-nil RepoInfo (branch match)")
	}
	if info.PRNumber != 21 {
		t.Errorf("PRNumber: want 21, got %d", info.PRNumber)
	}
	// gh must NOT have been called (registry branch-hit).
	if ghSpy.CallCount() != 0 {
		t.Errorf("gh called %d time(s); want 0 (registry branch-hit)", ghSpy.CallCount())
	}
}

// ---------------------------------------------------------------------------
// DefaultRegistryPath smoke test (just checks it returns a non-empty string)
// ---------------------------------------------------------------------------

func TestDefaultRegistryPath(t *testing.T) {
	p := DefaultRegistryPath()
	if p == "" {
		t.Error("DefaultRegistryPath: returned empty string")
	}
	// Should end with the canonical suffix.
	const suffix = "gss/worktrees/registry.json"
	if len(p) < len(suffix) || p[len(p)-len(suffix):] != suffix {
		t.Errorf("DefaultRegistryPath: want path ending in %q, got %q", suffix, p)
	}
}

// ---------------------------------------------------------------------------
// ghFallback: internal helper smoke tests
// ---------------------------------------------------------------------------

func TestGhFallback_NilResponse(t *testing.T) {
	isolateGhCache(t)
	r := &ghfake.Runner{Default: ghfake.Response{}}
	info, err := ghFallback(context.Background(), r, "test-branch-gh-nil-response")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("want nil, got %+v", info)
	}
}

func TestGhFallback_ValidResponse(t *testing.T) {
	isolateGhCache(t)
	r := &ghfake.Runner{Default: ghfake.Response{Stdout: ghJSONResponse(7, "OPEN")}}
	info, err := ghFallback(context.Background(), r, "test-branch-gh-valid-response")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("want non-nil RepoInfo")
	}
	if info.PRNumber != 7 {
		t.Errorf("PRNumber: want 7, got %d", info.PRNumber)
	}
}

// ---------------------------------------------------------------------------
// Malformed registry → stderr warning emitted, then gh fallback
// ---------------------------------------------------------------------------

// TestPR_MalformedRegistry_WarnsAndFallsBack proves that an unreadable/malformed
// registry.json no longer fails silently: a diagnostic is emitted to the warning
// writer, and the gh fallback still resolves the PR (behavior unchanged except
// for the added warning).
func TestPR_MalformedRegistry_WarnsAndFallsBack(t *testing.T) {
	isolateGhCache(t)

	var buf bytes.Buffer
	prev := prWarnWriter
	prWarnWriter = &buf
	t.Cleanup(func() { prWarnWriter = prev })

	ghRunner := &ghfake.Runner{
		Default: ghfake.Response{Stdout: ghJSONResponse(88, "OPEN")},
	}

	info, err := PR(
		context.Background(), ghRunner,
		"test-branch-malformed-registry", "/some/toplevel",
		testdataPath("registry_malformed.json"),
	)
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("PR: expected non-nil RepoInfo from gh fallback")
	}
	if info.PRNumber != 88 {
		t.Errorf("PRNumber: want 88 (gh fallback), got %d", info.PRNumber)
	}
	if ghRunner.CallCount() == 0 {
		t.Error("gh.Runner was NOT called; want ≥1 call for malformed-registry fallback")
	}
	if got := buf.String(); !strings.Contains(got, "registry") {
		t.Errorf("expected a stderr diagnostic mentioning the registry, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Registry match by toplevel for other-feature / PR #42
// ---------------------------------------------------------------------------

func TestPR_OtherFeature_RegistryHit(t *testing.T) {
	ghSpy := &ghfake.Runner{}

	toplevel := "/home/bob/.config/gss/worktrees/sfc-gh-bob/dotfiles/other-feature/bob/impl"
	info, err := PR(context.Background(), ghSpy, "", toplevel, testdataPath("registry.json"))
	if err != nil {
		t.Fatalf("PR: unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("PR: expected non-nil RepoInfo")
	}
	if info.PRNumber != 42 {
		t.Errorf("PRNumber: want 42, got %d", info.PRNumber)
	}
	if info.PRState != "MERGED" {
		t.Errorf("PRState: want MERGED, got %q", info.PRState)
	}
	if info.FeatureName != "other-feature" {
		t.Errorf("FeatureName: want other-feature, got %q", info.FeatureName)
	}
	if ghSpy.CallCount() != 0 {
		t.Errorf("gh called %d time(s); want 0 (registry-hit)", ghSpy.CallCount())
	}
}

// Ensure fmt and os are used (avoids "imported and not used" with the helper).
var _ = fmt.Sprintf
var _ = os.WriteFile
