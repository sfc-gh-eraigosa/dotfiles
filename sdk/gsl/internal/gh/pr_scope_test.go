package gh_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh/fake"
)

// mkRepo creates a directory that repoRoot will recognise as a repository.
// Only the presence of .git matters — the key derivation is a filesystem walk,
// never a `git` subprocess (that would put a fork back on the hot render path).
func mkRepo(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir repo %s: %v", name, err)
	}
	return dir
}

// TestPR_NegativeCacheIsScopedToRepo is the regression guard for a cross-repo
// cache-poisoning bug.
//
// The PR cache lives in ONE global directory but was keyed on the branch name
// alone. Negative caching (E19/F13) then made the common case — "this branch has
// no PR" — write an entry under that shared key. Result: repo A having no PR on
// a branch SUPPRESSED the real PR of repo B on a branch of the same name for the
// full 60 s TTL.
//
// Reproduced end-to-end before the fix: repo A cached {"number":0} under
// pr-shared-branch.json, and repo B's open PR #42 was then never even looked up
// (gh was not invoked at all — the poisoned entry was served instead).
//
// Against the branch-only key this test fails on BOTH assertions below: the
// runner is never called (CallCount 0) and the real PR comes back nil.
func TestPR_NegativeCacheIsScopedToRepo(t *testing.T) {
	setupCacheDir(t)

	repoA := mkRepo(t, "repoA")
	repoB := mkRepo(t, "repoB")
	const branch = "shared-branch"

	// ── repo A: no PR on `branch`. gh signals that by EXITING 1. ──
	t.Chdir(repoA)
	noPR := &fake.Runner{Default: fake.Response{
		Stderr:   []byte("no pull requests found for branch \"" + branch + "\"\n"),
		ExitCode: 1,
		Err:      errors.New("exit status 1"),
	}}
	info, err := gh.PR(context.Background(), noPR, branch)
	if err != nil {
		t.Fatalf("repoA: unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("repoA: expected nil PRInfo (no PR), got %+v", info)
	}
	// Sanity: the negative WAS cached (that is the E19 behaviour we want kept).
	if _, err := os.Stat(gh.CacheFilePathForTest(branch)); err != nil {
		t.Fatalf("repoA: the negative result should still be cached (E19/F13): %v", err)
	}

	// ── repo B: SAME branch name, but here it really does have an open PR. ──
	t.Chdir(repoB)
	hasPR := &fake.Runner{Default: fake.Response{
		Stdout: []byte(`{"number":42,"state":"OPEN"}`),
	}}
	info, err = gh.PR(context.Background(), hasPR, branch)
	if err != nil {
		t.Fatalf("repoB: unexpected error: %v", err)
	}

	if got := hasPR.CallCount(); got != 1 {
		t.Errorf("repoB: CallCount() = %d; want 1 — gh was never asked, because "+
			"repoA's NEGATIVE cache entry for the same branch name was served "+
			"instead (the cache key must be scoped to the repository)", got)
	}
	if info == nil {
		t.Fatalf("repoB: PR #42 came back nil — repoA's 'no PR' answer for an " +
			"unrelated repository suppressed it (cross-repo cache poisoning)")
	}
	if info.Number != 42 || info.State != "OPEN" {
		t.Errorf("repoB: got PR %+v; want {Number:42 State:OPEN}", info)
	}
}

// TestPR_PositiveCacheDoesNotLeakAcrossRepos is the mirror image: repo A's real
// PR number must not be FABRICATED into repo B, which has no PR at all. This
// direction predates negative caching (the success path always wrote the cache),
// so it is the older half of the same key bug.
func TestPR_PositiveCacheDoesNotLeakAcrossRepos(t *testing.T) {
	setupCacheDir(t)

	repoA := mkRepo(t, "repoA")
	repoB := mkRepo(t, "repoB")
	const branch = "shared-branch"

	t.Chdir(repoA)
	hasPR := &fake.Runner{Default: fake.Response{
		Stdout: []byte(`{"number":7,"state":"OPEN"}`),
	}}
	info, err := gh.PR(context.Background(), hasPR, branch)
	if err != nil || info == nil || info.Number != 7 {
		t.Fatalf("repoA: want PR #7, got %+v (err %v)", info, err)
	}

	t.Chdir(repoB)
	noPR := &fake.Runner{Default: fake.Response{
		Stderr:   []byte("no pull requests found\n"),
		ExitCode: 1,
		Err:      errors.New("exit status 1"),
	}}
	info, err = gh.PR(context.Background(), noPR, branch)
	if err != nil {
		t.Fatalf("repoB: unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("repoB: has NO PR, but got %+v — repo A's cached PR number "+
			"leaked across the shared branch-only cache key", info)
	}
}
