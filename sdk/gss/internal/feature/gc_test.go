package feature_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	wtgit "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/worktree/git"
)

const gcNWO = "octo/proj"

// Tiny stdlib stand-ins for testify (not a dependency of this module).
type must struct{}

var require must

func (must) NoError(t *testing.T, err error, msg ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v %v", err, msg)
	}
}

func (must) Error(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

type soft struct{}

var assert soft

func (soft) True(t *testing.T, v bool, msg ...any) {
	t.Helper()
	if !v {
		t.Errorf("want true %v", msg)
	}
}
func (soft) False(t *testing.T, v bool, msg ...any) {
	t.Helper()
	if v {
		t.Errorf("want false %v", msg)
	}
}
func (soft) Equal(t *testing.T, want, got any, msg ...any) {
	t.Helper()
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Errorf("want %v, got %v %v", want, got, msg)
	}
}
func (soft) Contains(t *testing.T, got, sub string, msg ...any) {
	t.Helper()
	if !strings.Contains(got, sub) {
		t.Errorf("%q does not contain %q %v", got, sub, msg)
	}
}
func (soft) Empty(t *testing.T, v any, msg ...any) {
	t.Helper()
	if rv := fmt.Sprint(v); rv != "[]" && rv != "map[]" && rv != "" {
		t.Errorf("want empty, got %v %v", v, msg)
	}
}
func (soft) Greater(t *testing.T, a, b int64, msg ...any) {
	t.Helper()
	if a <= b {
		t.Errorf("want %d > %d %v", a, b, msg)
	}
}
func (soft) ElementsMatch(t *testing.T, want, got []string) {
	t.Helper()
	w, g := append([]string(nil), want...), append([]string(nil), got...)
	sort.Strings(w)
	sort.Strings(g)
	if fmt.Sprint(w) != fmt.Sprint(g) {
		t.Errorf("want elements %v, got %v", w, g)
	}
}

// gcEnv is a real temp git repo (with a bare origin), real worktrees, a real
// registry file and a fake gh — nothing outside t.TempDir() is touched.
type gcEnv struct {
	t     *testing.T
	root  string // worktrees root
	main  string // main checkout
	store *registry.Store
	gh    *ghfake.Client
	svc   *feature.Service
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=gss-test",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=gss-test",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func newGCEnv(t *testing.T) *gcEnv {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	mainDir := filepath.Join(base, "main")
	require.NoError(t, os.MkdirAll(origin, 0o755))
	gitIn(t, origin, "init", "--bare", "-b", "main")
	require.NoError(t, os.MkdirAll(mainDir, 0o755))
	gitIn(t, mainDir, "init", "-b", "main")
	gitIn(t, mainDir, "remote", "add", "origin", origin)
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "README"), []byte("hi\n"), 0o644))
	gitIn(t, mainDir, "add", "README")
	gitIn(t, mainDir, "commit", "-m", "init")
	gitIn(t, mainDir, "push", "-u", "origin", "main")

	root := filepath.Join(base, "wt")
	store := registry.NewStore(filepath.Join(base, "registry.json"))
	require.NoError(t, store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", StartedAt: "2026-05-21T12:00:00Z", DefaultBaseBranch: "main", Description: "login work",
		}}}
		return nil
	}))
	ghc := ghfake.NewClient()
	runner := git.NewSystemRunner()
	return &gcEnv{t: t, root: root, main: mainDir, store: store, gh: ghc, svc: &feature.Service{
		Store: store, Backend: wtgit.NewBackend(mainDir, runner), Git: runner, GH: ghc,
		WorktreeRoot: root, NWO: gcNWO,
	}}
}

// addWorker creates a pushed worktree for feature auth, registers it with
// the cached state, and seeds the PR on the fake with ghState.
func (e *gcEnv) addWorker(leaf, base string, pr int, cached, ghState string) string {
	e.t.Helper()
	branch := "feature/auth/erai/" + leaf
	wt := filepath.Join(e.root, gcNWO, "auth", "erai", leaf)
	require.NoError(e.t, os.MkdirAll(filepath.Dir(wt), 0o755))
	gitIn(e.t, e.main, "worktree", "add", "-b", branch, wt, "main")
	require.NoError(e.t, os.WriteFile(filepath.Join(wt, leaf+".txt"), []byte(leaf), 0o644))
	gitIn(e.t, wt, "add", ".")
	gitIn(e.t, wt, "commit", "-m", leaf)
	gitIn(e.t, wt, "push", "-u", "origin", branch)
	url := fmt.Sprintf("https://github.com/%s/pull/%d", gcNWO, pr)
	e.register(registry.Worker{User: "erai", Purpose: leaf, Branch: branch, Worktree: wt, BaseBranch: base, PRURL: url, PRState: cached})
	e.seed(pr, ghState)
	return wt
}

func (e *gcEnv) register(w registry.Worker) {
	e.t.Helper()
	require.NoError(e.t, e.store.Update(func(r *registry.Registry) error {
		r.Features[0].Workers = append(r.Features[0].Workers, w)
		return nil
	}))
}

func (e *gcEnv) seed(num int, state string) {
	pr := gh.PR{Number: num, State: strings.ToUpper(state)}
	if state == "draft" {
		pr.State, pr.IsDraft = "OPEN", true
	}
	e.gh.SeedPR(pr)
}

func (e *gcEnv) rows() map[string]registry.Worker {
	e.t.Helper()
	reg, err := e.store.Load()
	require.NoError(e.t, err)
	out := map[string]registry.Worker{}
	for _, f := range reg.Features {
		for _, w := range f.Workers {
			out[w.Purpose] = w
		}
	}
	return out
}

func (e *gcEnv) branchExists(b string) bool {
	cmd := exec.Command("git", "-C", e.main, "rev-parse", "--verify", "--quiet", "refs/heads/"+b)
	return cmd.Run() == nil
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func (e *gcEnv) prViews() []int {
	var nums []int
	for _, c := range e.gh.Calls() {
		if c.Verb == ghfake.VerbPRView {
			nums = append(nums, c.Num)
		}
	}
	return nums
}

func TestDone_MergedPRWithStaleOpenCacheNeedsNoForce(t *testing.T) {
	e := newGCEnv(t)
	wt := e.addWorker("api", "main", 11, "open", "merged")

	res, err := e.svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	require.NoError(t, err)
	assert.True(t, res.BranchDeleted)
	assert.False(t, exists(wt), "worktree removed")
	assert.False(t, e.branchExists("feature/auth/erai/api"), "local branch of a merged PR deleted")
	assert.Empty(t, e.rows())
	assert.Equal(t, []int{11}, e.prViews())
}

func TestDone_OpenPRStillRefusedAndStateRefreshed(t *testing.T) {
	e := newGCEnv(t)
	wt := e.addWorker("api", "main", 12, "merged-looking", "draft")

	_, err := e.svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"draft"`)
	assert.True(t, exists(wt))
	assert.True(t, e.branchExists("feature/auth/erai/api"), "open PR's branch untouched")
	assert.Equal(t, "draft", e.rows()["api"].PRState, "observed state persisted")
}

func TestDone_ClosedPRAllowedButBranchKept(t *testing.T) {
	e := newGCEnv(t)
	wt := e.addWorker("api", "main", 13, "open", "closed")

	res, err := e.svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	require.NoError(t, err)
	assert.False(t, res.BranchDeleted)
	assert.False(t, exists(wt))
	assert.True(t, e.branchExists("feature/auth/erai/api"), "closed-unmerged branch is not deleted")
}

func TestDone_GitHubUnreachableKeepsRefusing(t *testing.T) {
	e := newGCEnv(t)
	wt := e.addWorker("api", "main", 14, "open", "merged")
	e.gh.ScriptError(ghfake.VerbPRView, fmt.Errorf("network down"))

	_, err := e.svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not be reached")
	assert.True(t, exists(wt))
}

func TestDone_DirtyTreeStillRefusedForMergedPR(t *testing.T) {
	e := newGCEnv(t)
	wt := e.addWorker("api", "main", 15, "open", "merged")
	require.NoError(t, os.WriteFile(filepath.Join(wt, "scratch"), []byte("x"), 0o644))

	_, err := e.svc.Done(context.Background(), feature.DoneOpts{WorkerRef: "auth/erai/api"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dirty")
	assert.True(t, exists(wt))
}

// gcFixture builds the full matrix: one worker per outcome plus a row that
// belongs to another repo in the same registry.
func gcFixture(t *testing.T) *gcEnv {
	e := newGCEnv(t)
	e.addWorker("merged", "main", 21, "open", "merged")
	e.addWorker("closed", "main", 22, "open", "closed")
	e.addWorker("openpr", "main", 23, "open", "open")
	dirty := e.addWorker("dirty", "main", 24, "open", "merged")
	require.NoError(t, os.WriteFile(filepath.Join(dirty, "scratch"), []byte("x"), 0o644))
	unpushed := e.addWorker("unpushed", "main", 25, "open", "merged")
	require.NoError(t, os.WriteFile(filepath.Join(unpushed, "more.txt"), []byte("m"), 0o644))
	gitIn(t, unpushed, "add", ".")
	gitIn(t, unpushed, "commit", "-m", "local only")
	e.addWorker("parent", "main", 26, "open", "merged")
	e.addWorker("child", "feature/auth/erai/parent", 27, "open", "open")
	// A row of ANOTHER repo in the shared registry: its PR number is merged
	// in the fake, so acting on it would be visible.
	other := filepath.Join(e.root, "octo", "other", "auth", "erai", "foreign")
	e.register(registry.Worker{User: "erai", Purpose: "foreign", Branch: "feature/auth/erai/foreign", Worktree: other,
		BaseBranch: "main", PRURL: "https://github.com/octo/other/pull/99", PRState: "open"})
	e.seed(99, "merged")
	return e
}

func skipReasons(r feature.GCResult) map[string]string {
	m := map[string]string{}
	for _, s := range r.Skipped {
		m[s.Worker] = s.Reason
	}
	return m
}

func TestGC_DryRunReportsAndChangesNothing(t *testing.T) {
	e := gcFixture(t)
	res, err := e.svc.GC(context.Background(), feature.GCOpts{})
	require.NoError(t, err)

	var cands []string
	for _, c := range res.Candidates {
		cands = append(cands, c.Worker)
		assert.Greater(t, c.Bytes, int64(0))
	}
	assert.ElementsMatch(t, []string{"auth/erai/merged", "auth/erai/closed"}, cands)
	assert.Empty(t, res.Removed)

	skips := skipReasons(res)
	assert.Contains(t, skips["auth/erai/openpr"], "is open")
	assert.Contains(t, skips["auth/erai/dirty"], "uncommitted")
	assert.Contains(t, skips["auth/erai/unpushed"], "not pushed")
	assert.Contains(t, skips["auth/erai/child"], "is open")
	assert.Contains(t, skips["auth/erai/parent"], "auth/erai/child still bases")
	if _, ok := skips["auth/erai/foreign"]; ok {
		t.Error("other repo's rows are out of scope, not even reported")
	}
	for _, n := range e.prViews() {
		if n == 99 {
			t.Error("asked GitHub about another repo's PR #99")
		}
	}

	// A dry run may record observed pr_state, but every worktree, branch and row stays.
	for _, leaf := range []string{"merged", "closed", "dirty", "unpushed", "openpr", "parent", "child"} {
		assert.True(t, exists(filepath.Join(e.root, gcNWO, "auth", "erai", leaf)), leaf+" worktree untouched")
		assert.True(t, e.branchExists("feature/auth/erai/"+leaf), leaf+" branch untouched")
	}
	assert.Equal(t, 8, len(e.rows()))
}

func TestGC_ApplyRemovesOnlyCandidates(t *testing.T) {
	e := gcFixture(t)
	res, err := e.svc.GC(context.Background(), feature.GCOpts{Apply: true})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"auth/erai/merged", "auth/erai/closed"}, res.Removed)
	assert.Equal(t, 0, len(res.Failed))
	assert.Greater(t, res.FreedBytes, int64(0))

	rows := e.rows()
	for _, gone := range []string{"merged", "closed"} {
		if _, ok := rows[gone]; ok {
			t.Errorf("%s row should be gone", gone)
		}
		assert.False(t, exists(filepath.Join(e.root, gcNWO, "auth", "erai", gone)), gone)
	}
	for _, kept := range []string{"openpr", "dirty", "unpushed", "parent", "child", "foreign"} {
		if _, ok := rows[kept]; !ok {
			t.Errorf("%s row must survive", kept)
		}
	}
	assert.False(t, e.branchExists("feature/auth/erai/merged"), "merged branch deleted")
	assert.True(t, e.branchExists("feature/auth/erai/closed"), "closed-unmerged branch kept")
	assert.True(t, e.branchExists("feature/auth/erai/openpr"), "open PR branch kept")
	assert.Equal(t, "open", rows["foreign"].PRState, "other repo row byte-for-byte untouched")
	assert.Equal(t, "merged", rows["parent"].PRState, "refreshed state is persisted even when skipped")

	// Second pass: the parent's dependent is still open, so nothing more goes.
	res2, err := e.svc.GC(context.Background(), feature.GCOpts{Apply: true})
	require.NoError(t, err)
	assert.Equal(t, 0, len(res2.Removed))
}

func TestGC_FeatureScope(t *testing.T) {
	e := gcFixture(t)
	_, err := e.svc.GC(context.Background(), feature.GCOpts{Feature: "nope"})
	require.Error(t, err)
	res, err := e.svc.GC(context.Background(), feature.GCOpts{Feature: "auth"})
	require.NoError(t, err)
	assert.Equal(t, 2, len(res.Candidates))
}

func TestGC_JSONShape(t *testing.T) {
	e := gcFixture(t)
	res, err := e.svc.GC(context.Background(), feature.GCOpts{})
	require.NoError(t, err)
	data, err := res.JSON()
	require.NoError(t, err)
	for _, key := range []string{`"removed": []`, `"skipped"`, `"worker"`, `"reason"`, `"freed_bytes"`} {
		assert.Contains(t, string(data), key)
	}
}
