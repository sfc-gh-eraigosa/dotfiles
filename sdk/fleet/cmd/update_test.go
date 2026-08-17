package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

func TestUpdateSkipsDirtyCloneByDefault(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": " M install.sh"}}, log: &sent}
	res, err := updateHost(r, "alpha", "main", false)
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
	res, err := updateHost(r, "alpha", "main", false)
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
	if _, err := updateHost(r, "alpha", "main", true); err != nil {
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
	if _, err := updateHost(r, "dead", "main", false); err == nil {
		t.Fatal("an unreachable host must surface an error")
	}
}

// The default target is `main`, matching the original hardcoded behaviour:
// fetch, switch to main, fast-forward it, re-run install.sh.
func TestUpdateScriptDefaultsToMain(t *testing.T) {
	s := remoteUpdateScript("main")
	for _, want := range []string{"git fetch origin", "git checkout main", "pull --ff-only origin main", "./install.sh"} {
		if !strings.Contains(s, want) {
			t.Fatalf("default update script missing %q:\n%s", want, s)
		}
	}
}

// A non-default ref lets an operator point a host at a branch or tag — e.g. to
// validate a change (like the install-stamp) before it lands on main.
func TestUpdateScriptTargetsGivenRef(t *testing.T) {
	s := remoteUpdateScript("feature/fleet/build")
	if !strings.Contains(s, "git checkout feature/fleet/build") ||
		!strings.Contains(s, "pull --ff-only origin feature/fleet/build") {
		t.Fatalf("update script did not target the ref:\n%s", s)
	}
	if strings.Contains(s, " main ") {
		t.Fatalf("update script leaked the main default:\n%s", s)
	}
}

func TestUpdateHostUsesTheRequestedRef(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": ""}}, log: &sent}
	if _, err := updateHost(r, "alpha", "feature/x", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(sent, " "), "feature/x") {
		t.Fatalf("updateHost ignored the ref: %v", sent)
	}
}

// The ref is interpolated into a remote shell command, so it must be a real
// git ref charset — anything else is rejected, not run.
func TestValidRefRejectsShellInjection(t *testing.T) {
	for _, good := range []string{"main", "feature/fleet/build", "v0.1.0", "release-1.2"} {
		if !validRef(good) {
			t.Errorf("valid ref rejected: %q", good)
		}
	}
	for _, bad := range []string{"", "main; rm -rf ~", "main && echo pwned", "$(whoami)", "a b", "main`id`"} {
		if validRef(bad) {
			t.Errorf("dangerous ref accepted: %q", bad)
		}
	}
}
