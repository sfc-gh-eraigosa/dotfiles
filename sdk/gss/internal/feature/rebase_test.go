package feature_test

import (
	"context"
	stderrors "errors"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

func rebaseService(t *testing.T, prURL string, script []gitfake.Response, ghc *ghfake.Client) (*feature.Service, *gitfake.Runner) {
	t.Helper()
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{
			Name: "auth", DefaultBaseBranch: "main",
			Workers: []registry.Worker{{
				User: "erai", Purpose: "ui", Branch: "feature/auth/erai/ui",
				Worktree: "/wt/ui", BaseBranch: "feature/auth/erai/api", Description: "ui", PRURL: prURL,
			}},
		}}}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	gitr := &gitfake.Runner{Script: script}
	return &feature.Service{Store: store, Git: gitr, GH: ghc}, gitr
}

func TestRebase_RebasesForcePushesUpdatesBaseNoBody(t *testing.T) {
	ghc := ghfake.NewClient()
	ghc.SeedPR(gh.PR{Number: 43, Head: "feature/auth/erai/ui", State: "OPEN", IsDraft: true, URL: "https://github.com/o/r/pull/43"})
	// fetch, rebase, push.
	svc, gitr := rebaseService(t, "https://github.com/o/r/pull/43",
		[]gitfake.Response{{}, {}, {}}, ghc)

	if err := svc.Rebase(context.Background(), feature.RebaseOpts{WorkerRef: "auth/erai/ui"}); err != nil {
		t.Fatalf("Rebase: %v", err)
	}
	// force-with-lease push happened.
	pushed := false
	for _, c := range gitr.Calls {
		if argsHasFC(c.Args, "push") && argsHasFC(c.Args, "--force-with-lease") {
			pushed = true
		}
	}
	if !pushed {
		t.Errorf("expected force-with-lease push; calls=%+v", gitr.Calls)
	}
	// PREdit updated base, but rendered NO body (pure rebase).
	var edit *ghfake.Call
	for i := range ghc.Calls() {
		if ghc.Calls()[i].Verb == ghfake.VerbPREdit {
			c := ghc.Calls()[i]
			edit = &c
		}
	}
	if edit == nil {
		t.Fatal("expected a PREdit")
	}
	if edit.EditOpts.Base != "feature/auth/erai/api" {
		t.Errorf("PREdit base = %q; want the worker base", edit.EditOpts.Base)
	}
	if edit.EditOpts.Body != "" || edit.EditOpts.BodyFile != "" {
		t.Errorf("rebase must NOT rewrite the body; got Body=%q BodyFile=%q", edit.EditOpts.Body, edit.EditOpts.BodyFile)
	}
	// No PRCreate.
	if lastPRCreate(ghc) != nil {
		t.Error("rebase must not create a PR")
	}
}

func TestRebase_ConflictAborts(t *testing.T) {
	// fetch ok, rebase fails, abort ok.
	svc, gitr := rebaseService(t, "",
		[]gitfake.Response{{}, {Err: stderrors.New("CONFLICT")}, {}}, ghfake.NewClient())
	if err := svc.Rebase(context.Background(), feature.RebaseOpts{WorkerRef: "auth/erai/ui"}); !stderrors.Is(err, errors.ErrRebaseConflict) {
		t.Fatalf("err = %v; want ErrRebaseConflict", err)
	}
	aborted := false
	for _, c := range gitr.Calls {
		if argsHasFC(c.Args, "rebase") && argsHasFC(c.Args, "--abort") {
			aborted = true
		}
	}
	if !aborted {
		t.Error("conflict must abort cleanly")
	}
}

func TestRebase_NoPRStillRebases(t *testing.T) {
	ghc := ghfake.NewClient()
	svc, _ := rebaseService(t, "", []gitfake.Response{{}, {}, {}}, ghc)
	if err := svc.Rebase(context.Background(), feature.RebaseOpts{WorkerRef: "auth/erai/ui"}); err != nil {
		t.Fatalf("Rebase: %v", err)
	}
	// No PR → no gh edit attempted.
	for _, c := range ghc.Calls() {
		if c.Verb == ghfake.VerbPREdit {
			t.Error("no PR: must not PREdit")
		}
	}
}

func TestRebase_UnknownWorker(t *testing.T) {
	svc, _ := rebaseService(t, "", nil, ghfake.NewClient())
	if err := svc.Rebase(context.Background(), feature.RebaseOpts{WorkerRef: "auth/erai/ghost"}); err == nil {
		t.Error("unknown worker: want error")
	}
}
