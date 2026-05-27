package feature_test

import (
	"context"
	stderrors "errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/feature"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	ghfake "github.com/wenlock/dotfiles/gss/internal/gh/fake"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// checkpointService seeds one worker and wires git/gh fakes.
func checkpointService(t *testing.T, prURL string, gitScript []gitfake.Response, ghc *ghfake.Client) (*feature.Service, *registry.Store, *gitfake.Runner) {
	t.Helper()
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{{
				User: "erai", Purpose: "api", Branch: "feature/auth/erai/api",
				Worktree: "/wt/api", BaseBranch: "main", Description: "endpoints", PRURL: prURL,
			}},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	gitr := &gitfake.Runner{Script: gitScript}
	return &feature.Service{Store: store, Git: gitr, GH: ghc}, store, gitr
}

func TestCheckpoint_FirstTimeCreatesDraftPR(t *testing.T) {
	// fetch ok, rebase ok.
	svc, store, gitr := checkpointService(t, "",
		[]gitfake.Response{{}, {}}, ghfake.NewClient())

	res, err := svc.Checkpoint(context.Background(), feature.CheckpointOpts{WorkerRef: "auth/erai/api"})
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if !res.Created || res.PRState != "draft" {
		t.Errorf("result = %+v; want Created draft", res)
	}
	// git: fetch then rebase, in order.
	if len(gitr.Calls) != 2 || !argsHasFC(gitr.Calls[0].Args, "fetch") || !argsHasFC(gitr.Calls[1].Args, "rebase") {
		t.Errorf("git calls = %+v; want fetch then rebase", gitr.Calls)
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

func lastPRCreate(c *ghfake.Client) *ghfake.Call {
	calls := c.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Verb == ghfake.VerbPRCreate {
			return &calls[i]
		}
	}
	return nil
}
