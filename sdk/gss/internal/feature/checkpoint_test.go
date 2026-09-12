package feature_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// checkpointService seeds one worker and wires git/gh fakes.
func checkpointService(t *testing.T, prURL string, gitScript []gitfake.Response, ghc *ghfake.Client) (*feature.Service, *registry.Store, *gitfake.Runner) {
	t.Helper()
	return checkpointServiceAt(t, "/wt/api", prURL, gitScript, ghc)
}

// checkpointServiceAt is checkpointService with the worker's worktree at wt,
// so a test can plant a WORKER.md at feature.WorkerMetaPath(wt).
func checkpointServiceAt(t *testing.T, wt, prURL string, gitScript []gitfake.Response, ghc *ghfake.Client) (*feature.Service, *registry.Store, *gitfake.Runner) {
	t.Helper()
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{{
				User: "erai", Purpose: "api", Branch: "feature/auth/erai/api",
				Worktree: wt, BaseBranch: "main", Description: "endpoints", PRURL: prURL,
			}},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	gitr := &gitfake.Runner{Script: gitScript}
	return &feature.Service{Store: store, Git: gitr, GH: ghc}, store, gitr
}

// plantWorkerMD writes a WORKER.md for the worker whose worktree is wt.
func plantWorkerMD(t *testing.T, wt, md string) {
	t.Helper()
	p := feature.WorkerMetaPath(wt)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

const seededWorkerMD = "# auth: api\n\n- **Description**: endpoints\n\n## Goal\nShip the endpoints.\n\n## Decisions & notes\n<!-- append freely -->\n- chose REST\n\n## Open questions\n"

// TestCheckpoint_FirstCreateSeedsTitleAndBodyFromWorkerMD: the draft PR
// opens with the worker description as its title and WORKER.md's Goal +
// notes as its body, and the stack block is re-rendered with the new PR
// number instead of "(no PR yet)" for the PR's own row. Before this, a
// first checkpoint produced "<feature>: <purpose>" over a one-line body and
// an unnumbered self-row, which every author then fixed by hand with gh.
func TestCheckpoint_FirstCreateSeedsTitleAndBodyFromWorkerMD(t *testing.T) {
	wt := filepath.Join(t.TempDir(), "auth", "erai", "api")
	plantWorkerMD(t, wt, seededWorkerMD)
	ghc := ghfake.NewClient()
	svc, _, _ := checkpointServiceAt(t, wt, "", []gitfake.Response{{}, {}, {}}, ghc)

	res, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	c := lastPRCreate(ghc)
	if c == nil {
		t.Fatal("PRCreate was never called")
	}
	if c.CreateOpts.Title != "endpoints" {
		t.Errorf("title = %q; want the worker description", c.CreateOpts.Title)
	}
	for _, want := range []string{"Ship the endpoints.", "## Decisions & notes", "- chose REST", "<!-- gss:stack-begin -->"} {
		if !strings.Contains(c.CreateOpts.Body, want) {
			t.Errorf("created body missing %q:\n%s", want, c.CreateOpts.Body)
		}
	}
	if strings.Contains(c.CreateOpts.Body, "append freely") {
		t.Errorf("template comment leaked into the PR body:\n%s", c.CreateOpts.Body)
	}
	// One follow-up edit numbers the PR's own stack row.
	e := lastPREdit(ghc)
	num := prNumberOf(res.PRURL)
	if e == nil || e.Num != num {
		t.Fatalf("expected a PREdit on the new PR #%d after create; got %+v", num, e)
	}
	if !strings.Contains(e.EditOpts.Body, fmt.Sprintf("**#%d — erai/api (base: `main`)** ← you are here", num)) {
		t.Errorf("stack row not renumbered after create:\n%s", e.EditOpts.Body)
	}
	if strings.Contains(e.EditOpts.Body, "(no PR yet)") {
		t.Errorf("edited body still says (no PR yet):\n%s", e.EditOpts.Body)
	}
	if !strings.Contains(e.EditOpts.Body, "Ship the endpoints.") || strings.Count(e.EditOpts.Body, "<!-- gss:stack-begin -->") != 1 {
		t.Errorf("renumbering must keep the prose and exactly one stack block:\n%s", e.EditOpts.Body)
	}
}

// TestCheckpoint_FirstCreateWithoutWorkerMDFallsBack: no WORKER.md and no
// description → the old "<feature>: <purpose>" title and description body.
func TestCheckpoint_FirstCreateWithoutWorkerMDFallsBack(t *testing.T) {
	ghc := ghfake.NewClient()
	svc, store, _ := checkpointService(t, "", []gitfake.Response{{}, {}, {}}, ghc)
	if err := store.Update(func(r *registry.Registry) error {
		r.Features[0].Workers[0].Description = ""
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	c := lastPRCreate(ghc)
	if c == nil || c.CreateOpts.Title != "auth: api" {
		t.Errorf("PRCreate = %+v; want fallback title \"auth: api\"", c)
	}
}

// TestCheckpoint_ExistingPRKeepsItsBodyOverWorkerMD: WORKER.md seeds only
// the first body. A later checkpoint must preserve what is on GitHub.
func TestCheckpoint_ExistingPRKeepsItsBodyOverWorkerMD(t *testing.T) {
	wt := filepath.Join(t.TempDir(), "auth", "erai", "api")
	plantWorkerMD(t, wt, seededWorkerMD)
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 7, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true,
		URL: "https://github.com/o/r/pull/7", Body: "human prose written on GitHub"})
	svc, _, _ := checkpointServiceAt(t, wt, "https://github.com/o/r/pull/7", []gitfake.Response{{}, {}, {}}, ghc)

	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	e := lastPREdit(ghc)
	if e == nil || e.Num != 7 {
		t.Fatalf("expected PREdit on #7; got %+v", e)
	}
	if !strings.Contains(e.EditOpts.Body, "human prose written on GitHub") {
		t.Errorf("existing PR body was not preserved:\n%s", e.EditOpts.Body)
	}
	if strings.Contains(e.EditOpts.Body, "Ship the endpoints.") {
		t.Errorf("WORKER.md prose overwrote an existing PR body:\n%s", e.EditOpts.Body)
	}
}

func TestCheckpoint_FirstTimeCreatesDraftPR(t *testing.T) {
	// fetch ok, rebase ok, push ok.
	svc, store, gitr := checkpointService(t, "",
		[]gitfake.Response{{}, {}, {}}, ghfake.NewClient())

	res, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if !res.Created || res.PRState != "draft" {
		t.Errorf("result = %+v; want Created draft", res)
	}
	// git: fetch then rebase then push, in order. The push must precede
	// gh pr create so origin actually has the head SHA when GitHub looks it
	// up — without it, gh pr create fails with "Head sha can't be blank".
	if len(gitr.Calls) != 3 ||
		!argsHasFC(gitr.Calls[0].Args, "fetch") ||
		!argsHasFC(gitr.Calls[1].Args, "rebase") ||
		!argsHasFC(gitr.Calls[2].Args, "push") {
		t.Errorf("git calls = %+v; want fetch, rebase, push", gitr.Calls)
	}
	// PR created draft, head=branch, body has stack section.
	c := lastPRCreate(svc.GH.(*ghfake.Client))
	if c == nil || !c.CreateOpts.Draft || c.CreateOpts.Head != "feature/auth/erai/api" {
		t.Errorf("PRCreate = %+v; want draft head=feature/auth/erai/api", c)
	}
	if !strings.Contains(c.CreateOpts.Body, "<!-- gss:stack-begin -->") {
		t.Errorf("PR body missing stack section:\n%s", c.CreateOpts.Body)
	}
	// registry updated with pr_url.
	reg, _ := store.Load()
	if reg.Features[0].Workers[0].PRURL == "" {
		t.Error("registry pr_url not updated after create")
	}
}

// TestCheckpoint_FirstTimePushUsesForceWithLeaseAndSetsUpstream guards
// against a regression where the create path called gh pr create before
// pushing the worker branch to origin. Symptoms in the wild: gh failed with
// "Head sha can't be blank ... No commits between <base> and <head>" and the
// registry never got a pr_url, leaving the worker stuck half-checkpointed.
//
// The fix mirrors the update path: push with --force-with-lease so a divergent
// remote branch (e.g. from a half-completed previous checkpoint) doesn't
// strand the worker on a non-fast-forward error, plus --set-upstream so the
// branch has a tracking ref before the next checkpoint runs.
func TestCheckpoint_FirstTimePushUsesForceWithLeaseAndSetsUpstream(t *testing.T) {
	ghc := ghfake.NewClient()
	svc, _, gitr := checkpointService(t, "",
		[]gitfake.Response{{}, {}, {}}, ghc)

	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"}); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	pushIdx := -1
	for i, call := range gitr.Calls {
		if argsHasFC(call.Args, "push") && argsHasFC(call.Args, "origin") {
			pushIdx = i
			break
		}
	}
	if pushIdx < 0 {
		t.Fatalf("expected a git push to origin before PRCreate; calls=%+v", gitr.Calls)
	}
	args := gitr.Calls[pushIdx].Args
	if !argsHasFC(args, "feature/auth/erai/api") {
		t.Errorf("push did not target worker branch; args=%v", args)
	}
	if !argsHasFC(args, "--force-with-lease") {
		t.Errorf("push must use --force-with-lease (mirroring update path); args=%v", args)
	}
	if !argsHasFC(args, "-u") && !argsHasFC(args, "--set-upstream") {
		t.Errorf("push must set upstream tracking; args=%v", args)
	}
	if lastPRCreate(ghc) == nil {
		t.Fatalf("PRCreate was never called")
	}
}

// TestCheckpoint_FirstTimePushFailureSurfaces ensures a failed initial push
// short-circuits the checkpoint with a clear error rather than silently
// dropping through to gh pr create (which would fail opaquely with "Head sha
// can't be blank") and leave the registry in a partial state.
func TestCheckpoint_FirstTimePushFailureSurfaces(t *testing.T) {
	pushErr := stderrors.New("non-fast-forward")
	// fetch ok, rebase ok, push fails.
	svc, _, _ := checkpointService(t, "",
		[]gitfake.Response{{}, {}, {Err: pushErr}}, ghfake.NewClient())

	_, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"})
	if err == nil {
		t.Fatalf("push failure: want error, got nil")
	}
	if !strings.Contains(err.Error(), "push") {
		t.Errorf("error should mention push; got: %v", err)
	}
	// PRCreate must NOT have been called when push failed.
	if lastPRCreate(svc.GH.(*ghfake.Client)) != nil {
		t.Error("PRCreate was called despite push failure; should short-circuit")
	}
}

func TestCheckpoint_ExistingPRForcePushesAndEdits(t *testing.T) {
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 7, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true, URL: "https://github.com/o/r/pull/7"})
	// fetch ok, rebase ok, force-push ok.
	svc, _, gitr := checkpointService(t, "https://github.com/o/r/pull/7",
		[]gitfake.Response{{}, {}, {}}, ghc)

	res, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if res.Created {
		t.Error("existing PR: result.Created = true; want false (edit path)")
	}
	// A force-with-lease push happened.
	found := false
	for _, call := range gitr.Calls {
		if argsHasFC(call.Args, "push") && argsHasFC(call.Args, "--force-with-lease") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected git push --force-with-lease; calls=%+v", gitr.Calls)
	}
	// PREdit called, not PRCreate.
	if lastPRCreate(ghc) != nil {
		t.Error("existing PR must be edited, not created")
	}
	edited := false
	for _, call := range ghc.Calls() {
		if call.Verb == ghfake.VerbPREdit && call.Num == 7 {
			edited = true
		}
	}
	if !edited {
		t.Error("expected gh PREdit on #7")
	}
}

func TestCheckpoint_AdoptsExistingOpenPRWhenRegistryHasNoURL(t *testing.T) {
	// The registry row has NO pr_url, but an open PR already exists on GitHub
	// for the head branch (e.g. created on another machine, or the registry
	// lost the url). Checkpoint must adopt that PR — push + edit — instead of
	// calling gh pr create (which would fail "a pull request ... already
	// exists" and silently drop the commit).
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 9, Head: "feature/auth/erai/api", State: "OPEN", IsDraft: true, URL: "https://github.com/o/r/pull/9"})
	// fetch ok, rebase ok, force-push ok.
	svc, store, gitr := checkpointService(t, "",
		[]gitfake.Response{{}, {}, {}}, ghc)

	res, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if res.Created {
		t.Error("existing open PR: result.Created = true; want false (adopt + edit path)")
	}
	if res.PRURL != "https://github.com/o/r/pull/9" {
		t.Errorf("res.PRURL = %q; want the adopted PR url", res.PRURL)
	}
	// PRCreate must NOT have been called.
	if lastPRCreate(ghc) != nil {
		t.Error("an existing open PR must be adopted, not re-created")
	}
	// A push must have happened — the whole point is the commit reaches origin.
	pushed := false
	for _, call := range gitr.Calls {
		if argsHasFC(call.Args, "push") {
			pushed = true
		}
	}
	if !pushed {
		t.Errorf("expected a git push to the existing PR's branch; calls=%+v", gitr.Calls)
	}
	// PREdit on the adopted PR number.
	edited := false
	for _, call := range ghc.Calls() {
		if call.Verb == ghfake.VerbPREdit && call.Num == 9 {
			edited = true
		}
	}
	if !edited {
		t.Error("expected gh PREdit on the adopted PR #9")
	}
	// Registry backfilled with the adopted pr_url/pr_state.
	reg, _ := store.Load()
	w := reg.Features[0].Workers[0]
	if w.PRURL != "https://github.com/o/r/pull/9" {
		t.Errorf("registry pr_url = %q; want the adopted PR url", w.PRURL)
	}
	if w.PRState == "" {
		t.Error("registry pr_state not backfilled after adopting the PR")
	}
}

func TestCheckpoint_RebaseConflictAborts(t *testing.T) {
	// fetch ok, rebase fails, abort ok.
	svc, _, gitr := checkpointService(t, "",
		[]gitfake.Response{{}, {Err: stderrors.New("CONFLICT")}, {}}, ghfake.NewClient())

	_, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"})
	if !stderrors.Is(err, errors.ErrRebaseConflict) {
		t.Fatalf("err = %v; want ErrRebaseConflict", err)
	}
	// rebase --abort must have been issued.
	aborted := false
	for _, call := range gitr.Calls {
		if argsHasFC(call.Args, "rebase") && argsHasFC(call.Args, "--abort") {
			aborted = true
		}
	}
	if !aborted {
		t.Error("rebase conflict must abort cleanly (rebase --abort)")
	}
}

func TestCheckpoint_UnknownWorker(t *testing.T) {
	svc, _, _ := checkpointService(t, "", nil, ghfake.NewClient())
	if _, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/ghost"}); err == nil {
		t.Error("unknown worker: want error")
	}
}

func argsHasFC(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func lastPREdit(c *ghfake.Client) *ghfake.Call {
	calls := c.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Verb == ghfake.VerbPREdit {
			return &calls[i]
		}
	}
	return nil
}

// prNumberOf mirrors the package's unexported prNumber for assertions.
func prNumberOf(url string) int {
	n, _ := strconv.Atoi(url[strings.LastIndex(url, "/")+1:])
	return n
}

func lastPRCreate(c *ghfake.Client) *ghfake.Call {
	calls := c.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Verb == ghfake.VerbPRCreate {
			return &calls[i]
		}
	}
	return nil
}
