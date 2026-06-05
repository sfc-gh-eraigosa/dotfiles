// Package sync_test verifies the fetch+rebase service per
// src/gss/docs/plan.md PR-12: fetch precedes pull, a rebase conflict
// surfaces as errors.ErrRebaseConflict, and the current branch defaults to
// main.
package sync_test

import (
	stderrors "errors"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
	"github.com/wenlock/dotfiles/gss/internal/sync"
)

func resp(stdout string) gitfake.Response { return gitfake.Response{Stdout: []byte(stdout)} }
func errResp(stdout string) gitfake.Response {
	return gitfake.Response{Stdout: []byte(stdout), Err: stderrors.New("git failed")}
}

func argsContain(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestSync_SuccessFetchPrecedesPull(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		resp("feature\n"),           // rev-parse --abbrev-ref HEAD
		resp(""),                    // fetch origin
		resp("abc123\n"),            // rev-parse --verify refs/remotes/origin/feature (exists)
		resp("Already up to date."), // pull --rebase
	}}
	s := sync.NewService(gitr)

	res, err := s.Sync(t.Context(), "/repo")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Branch != "feature" {
		t.Errorf("branch = %q; want feature", res.Branch)
	}
	if res.NewBranch {
		t.Errorf("res.NewBranch = true; want false when origin/feature exists")
	}
	if len(gitr.Calls) != 4 {
		t.Fatalf("calls = %d; want 4 (rev-parse, fetch, verify, pull)", len(gitr.Calls))
	}
	if !argsContain(gitr.Calls[1].Args, "fetch") {
		t.Errorf("call[1] not fetch: %+v", gitr.Calls[1])
	}
	if !argsContain(gitr.Calls[3].Args, "pull") || !argsContain(gitr.Calls[3].Args, "feature") {
		t.Errorf("call[3] not `pull … feature`: %+v", gitr.Calls[3])
	}
}

func TestSync_FetchErrorSkipsPull(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		resp("main\n"),
		errResp("could not reach origin"),
	}}
	s := sync.NewService(gitr)

	if _, err := s.Sync(t.Context(), "/repo"); err == nil {
		t.Fatal("fetch error: err = nil; want error")
	}
	if len(gitr.Calls) != 2 {
		t.Errorf("calls = %d; want 2 (pull must not run after fetch fails)", len(gitr.Calls))
	}
}

func TestSync_RebaseConflict(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		resp("main\n"),
		resp(""),
		resp("abc123\n"), // rev-parse --verify refs/remotes/origin/main (exists)
		errResp("CONFLICT (content): Merge conflict in a.go"),
	}}
	s := sync.NewService(gitr)

	_, err := s.Sync(t.Context(), "/repo")
	if err == nil {
		t.Fatal("rebase conflict: err = nil; want ErrRebaseConflict")
	}
	if !stderrors.Is(err, errors.ErrRebaseConflict) {
		t.Errorf("err = %v; want wrapping ErrRebaseConflict", err)
	}
}

// A brand-new branch has no origin/<branch> counterpart yet. Sync must NOT
// try to rebase onto a non-existent remote ref (which fails with "couldn't
// find remote ref" and, in the push flow, burns the approval token and forces
// a second confirmation prompt). Instead it reports NewBranch so the caller's
// push creates the branch with --set-upstream.
func TestSync_NewBranchSkipsRebase(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		resp("feature\n"), // rev-parse --abbrev-ref HEAD
		resp(""),          // fetch origin
		errResp(""),       // rev-parse --verify refs/remotes/origin/feature → MISSING (exit 1)
	}}
	s := sync.NewService(gitr)

	res, err := s.Sync(t.Context(), "/repo")
	if err != nil {
		t.Fatalf("Sync on a new branch should not error: %v", err)
	}
	if !res.NewBranch {
		t.Errorf("res.NewBranch = false; want true for a branch with no origin counterpart")
	}
	if len(gitr.Calls) != 3 {
		t.Fatalf("calls = %d; want 3 (rev-parse, fetch, verify) — pull must be skipped", len(gitr.Calls))
	}
	for _, c := range gitr.Calls {
		if argsContain(c.Args, "pull") {
			t.Errorf("pull must be skipped for a new branch, but it ran: %+v", c)
		}
	}
}

func TestSync_DefaultsToMain(t *testing.T) {
	gitr := &gitfake.Runner{Script: []gitfake.Response{
		errResp(""),      // rev-parse --abbrev-ref HEAD fails → default main
		resp(""),         // fetch
		resp("abc123\n"), // rev-parse --verify refs/remotes/origin/main (exists)
		resp(""),         // pull
	}}
	s := sync.NewService(gitr)

	res, err := s.Sync(t.Context(), "/repo")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Branch != "main" {
		t.Errorf("branch = %q; want main (rev-parse failed)", res.Branch)
	}
	if !argsContain(gitr.Calls[3].Args, "main") {
		t.Errorf("pull should target main: %+v", gitr.Calls[3])
	}
}
