package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

func TestUpdateSkipsDirtyCloneByDefault(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": " M install.sh"}}, log: &sent}
	res, err := updateHost(r, "alpha", false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Fatal("a dirty clone must be skipped without --force")
	}
	if res.Reason == "" {
		t.Fatal("a skip must state a reason")
	}
	for _, c := range sent {
		if strings.Contains(c, "install.sh") || strings.Contains(c, "pull") {
			t.Fatalf("a skipped host must not be mutated: %q", c)
		}
	}
}

func TestUpdateProceedsOnCleanClone(t *testing.T) {
	r := runner.Fake{Out: map[string]string{"alpha": ""}}
	res, err := updateHost(r, "alpha", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped {
		t.Fatalf("a clean clone must not be skipped: %s", res.Reason)
	}
}

// --force must PRESERVE the dirty work in a rescue worktree, never discard it.
func TestForceRescuesDirtyWorkBeforePulling(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": " M install.sh"}}, log: &sent}
	if _, err := updateHost(r, "alpha", true); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(sent, " ")
	if strings.Contains(joined, "reset --hard") || strings.Contains(joined, "checkout -- ") {
		t.Fatalf("--force must never discard local work: %v", sent)
	}
	for _, want := range []string{"git add -A", "worktree add"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("--force must preserve work via %q: %v", want, sent)
		}
	}
}

// Verified on git 2.47.3: `git branch <n> stash@{0}` loses UNTRACKED files
// (a stash commit's tree excludes them). The rescue must use `git add -A` so
// an operator inspecting the rescue worktree finds everything.
func TestRescuePreservesUntrackedWork(t *testing.T) {
	if !strings.Contains(rescueWorktree, "git add -A") {
		t.Fatal("rescue must stage untracked files (git add -A), or they are lost")
	}
	if strings.Contains(rescueWorktree, "stash@{0}") {
		t.Fatal("branching from stash@{0} silently drops untracked files")
	}
	if !strings.Contains(rescueWorktree, "worktree add") {
		t.Fatal("rescued work must be materialised as an inspectable worktree")
	}
}

func TestUpdateSurfacesProbeFailure(t *testing.T) {
	r := runner.Fake{Err: map[string]error{"dead": runner.ErrFake}}
	if _, err := updateHost(r, "dead", false); err == nil {
		t.Fatal("an unreachable host must surface an error")
	}
}
