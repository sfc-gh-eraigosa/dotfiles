package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// updateHost now runs the default plan through the executor. Its precheck
// step is PrecheckScript, whose output format is "state=<x> branch=<y>" —
// not the raw `git status --porcelain` these fixtures used pre-executor.

func TestUpdateSkipsDirtyCloneByDefault(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": "state=dirty branch=main"}}, log: &sent}
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
	r := runner.Fake{Out: map[string]string{"alpha": "state=clean branch=main"}}
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
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": "state=dirty branch=main"}}, log: &sent}
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
	for _, want := range []string{"git fetch origin main", "git checkout main", "merge --ff-only FETCH_HEAD", "./install.sh"} {
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
		!strings.Contains(s, "git fetch origin feature/fleet/build") {
		t.Fatalf("update script did not target the ref:\n%s", s)
	}
	if strings.Contains(s, " main ") {
		t.Fatalf("update script leaked the main default:\n%s", s)
	}
}

func TestUpdateHostUsesTheRequestedRef(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": "state=clean branch=main"}}, log: &sent}
	if _, err := updateHost(r, "alpha", "feature/x", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(sent, " "), "feature/x") {
		t.Fatalf("updateHost ignored the ref: %v", sent)
	}
}

// The ref is interpolated into a remote shell command, so it must be a real
// git ref charset — anything else is rejected, not run. validRef is now an
// alias for updplan.ValidRef.
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

// The update must contact the remote exactly ONCE. `git pull` runs its own
// fetch, so the old `fetch && pull` form went to the network twice for data
// the first call already had — doubling the exposure to a transient fault.
// Observed on a real host: DNS answered for the fetch and failed for the pull
// seconds later, failing an update that had everything it needed locally.
func TestUpdateMakesExactlyOneNetworkCall(t *testing.T) {
	s := remoteUpdateScript("main")
	if n := strings.Count(s, "git fetch"); n != 1 {
		t.Fatalf("expected exactly one fetch, got %d:\n%s", n, s)
	}
	// `git pull` is banned here precisely because it re-fetches.
	if strings.Contains(s, "git pull") {
		t.Fatalf("git pull re-contacts the remote; merge the fetched ref instead:\n%s", s)
	}
	// The merge must be local: FETCH_HEAD is already on disk after the fetch.
	if !strings.Contains(s, "merge --ff-only FETCH_HEAD") {
		t.Fatalf("the fast-forward must be local:\n%s", s)
	}
	// Still fast-forward only — an update must never create a merge commit on
	// a host, which would leave it permanently divergent.
	if !strings.Contains(s, "--ff-only") {
		t.Fatalf("must stay fast-forward-only:\n%s", s)
	}
}

// The whole run — precheck + sync, for the default one-repo plan — must make
// exactly one `git fetch`, and never a `git pull` (which re-fetches).
func TestUpdateDefaultPlanSendsExactlyOneFetchPerSyncStep(t *testing.T) {
	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": "state=clean branch=main"}}, log: &sent}
	if _, err := updateHost(r, "alpha", "main", false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(sent, " ")
	if n := strings.Count(joined, "git fetch"); n != 1 {
		t.Fatalf("expected exactly one git fetch across the whole run, got %d:\n%v", n, sent)
	}
	if strings.Contains(joined, "git pull") {
		t.Fatalf("git pull re-contacts the remote: %v", sent)
	}
}

// --force is exactly an alias for --local rescue: giving both consistently
// is fine, giving them inconsistently is an error, and --local alone still
// works without --force.
func TestForceIsAnAliasForLocalRescue(t *testing.T) {
	local, err := resolveLocalPolicy("", true)
	if err != nil {
		t.Fatal(err)
	}
	if local != updplan.LocalRescue {
		t.Fatalf("--force must resolve to local:rescue, got %q", local)
	}

	local, err = resolveLocalPolicy("rescue", true)
	if err != nil {
		t.Fatal(err)
	}
	if local != updplan.LocalRescue {
		t.Fatalf("--force + --local rescue must agree, got %q", local)
	}

	if _, err := resolveLocalPolicy("carry", true); err == nil {
		t.Fatal("--force + --local carry disagree and must be an error")
	}

	local, err = resolveLocalPolicy("carry", false)
	if err != nil {
		t.Fatal(err)
	}
	if local != updplan.LocalCarry {
		t.Fatalf("--local carry alone must resolve to carry, got %q", local)
	}

	if _, err := resolveLocalPolicy("bogus", false); err == nil {
		t.Fatal("an unrecognised --local value must be rejected")
	}
}

// --timeout and --no-retry must reach the Executor unchanged.
func TestTimeoutAndNoRetryFlagsReachTheExecutor(t *testing.T) {
	old := flagUpdateTimeout
	oldRetry := flagUpdateNoRetry
	oldReset := flagUpdateReset
	oldRestore := flagUpdateNoRestore
	defer func() {
		flagUpdateTimeout = old
		flagUpdateNoRetry = oldRetry
		flagUpdateReset = oldReset
		flagUpdateNoRestore = oldRestore
	}()
	flagUpdateTimeout = 90 * time.Second
	flagUpdateNoRetry = true
	flagUpdateReset = true
	flagUpdateNoRestore = true

	ex := buildExecutor(runner.Fake{}, nil, updplan.LocalSkip)
	if ex.Timeout != 90*time.Second {
		t.Fatalf("Timeout = %v, want 90s", ex.Timeout)
	}
	if !ex.NoRetry {
		t.Fatal("NoRetry must reach the executor")
	}
	if !ex.Reset {
		t.Fatal("Reset must reach the executor")
	}
	if !ex.NoRestore {
		t.Fatal("NoRestore must reach the executor")
	}
	if ex.Local != updplan.LocalSkip {
		t.Fatalf("Local = %q, want skip", ex.Local)
	}
}
