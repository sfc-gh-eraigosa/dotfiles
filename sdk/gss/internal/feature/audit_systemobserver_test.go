// White-box tests for the PRODUCTION systemObserver against a real git
// binary — deliberately NOT using fakeObserver, since the dotfiles#336 bug
// lives entirely in systemObserver's wiring (fakeObserver has no concept
// of "repoPath" at all, so it could never have caught this).
package feature

import (
	"context"
	"os/exec"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not found on PATH; skipping: %v", err)
	}
}

// initRepoWithBranch creates a fresh git repo in t.TempDir(), commits once
// on its default branch, then creates (and checks out) branchName as a
// second branch. Returns the repo's absolute path.
func initRepoWithBranch(t *testing.T, branchName string) string {
	t.Helper()
	skipIfNoGit(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--quiet")
	run("config", "user.email", "gss-test"+"@example.invalid")
	run("config", "user.name", "gss test")
	run("config", "commit.gpgsign", "false")
	run("commit", "--allow-empty", "-m", "seed")
	run("checkout", "-b", branchName)
	return dir
}

// TestSystemObserver_BranchExists_ScopesToWorkersOwnRepo pins the root
// cause of dotfiles#336: BranchExists used to run `git -C repoPath
// rev-parse --verify refs/heads/<branch>`, scoped to whatever repo the
// audit command was INVOKED from/against — applied blindly to every
// worker in the registry. But the registry is shared across every repo on
// the host (RegistryDir defaults to one global path under
// ~/.config/gss/worktrees, not a per-repo one), so a worker belonging to a
// DIFFERENT repo than repoPath always looked branch-missing — even though
// its branch, worktree, and PR were all perfectly healthy in its OWN
// repo — and `audit --repair` silently dropped its row.
//
// Two independent real repos both create a branch of the SAME name, so
// this can't pass by accident: the check must be scoped to the worktree
// actually passed in, not any fixed repoPath.
func TestSystemObserver_BranchExists_ScopesToWorkersOwnRepo(t *testing.T) {
	const branch = "shared-branch-name"
	repoA := initRepoWithBranch(t, branch)
	repoB := initRepoWithBranch(t, branch) // has the same branch name, independently

	obs := &systemObserver{git: git.NewSystemRunner()}
	ctx := context.Background()

	if !obs.BranchExists(ctx, repoA, branch) {
		t.Errorf("BranchExists(repoA, %q) = false; want true (repoA created this branch itself)", branch)
	}
	if !obs.BranchExists(ctx, repoB, branch) {
		t.Errorf("BranchExists(repoB, %q) = false; want true (repoB created this branch itself)", branch)
	}
	// The branch this worker's OWN repo genuinely never created must still
	// be reported missing — proves the check is actually scoped to the
	// worktree, not hard-coded to always return true.
	if obs.BranchExists(ctx, repoA, "never-created-anywhere") {
		t.Error("BranchExists(repoA, \"never-created-anywhere\") = true; want false")
	}
}

func TestSystemObserver_BaseReachable_ScopesToWorkersOwnRepo(t *testing.T) {
	const branch = "shared-branch-name"
	repoA := initRepoWithBranch(t, branch)
	repoB := initRepoWithBranch(t, branch)

	obs := &systemObserver{git: git.NewSystemRunner()}
	ctx := context.Background()

	// Each repo's own branch is trivially reachable from its own default
	// branch (one linear history, no divergence).
	if !obs.BaseReachable(ctx, repoA, branch, "master") && !obs.BaseReachable(ctx, repoA, branch, "main") {
		t.Error("BaseReachable(repoA): want true against repoA's own default branch")
	}
	if !obs.BaseReachable(ctx, repoB, branch, "master") && !obs.BaseReachable(ctx, repoB, branch, "main") {
		t.Error("BaseReachable(repoB): want true against repoB's own default branch")
	}
}
