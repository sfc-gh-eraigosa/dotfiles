package hookguard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// A local value is shared by every worktree of a repo: checking the main
// checkout and a linked worktree reports it once, under the main checkout,
// and a --fix that reaches it through the worktree first names the main
// checkout too.
func TestLocalValueReportedOnceAcrossWorktrees(t *testing.T) {
	root, _ := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	main := newRepo(t, filepath.Join(root, "z", "main"))
	commit(t, main)
	wt := filepath.Join(root, "a", "wt") // sorts before main
	gitc(t, main, "worktree", "add", "-q", "-b", "w", wt)
	gitc(t, main, "config", "extensions.worktreeConfig", "true")

	gitc(t, main, "config", "--local", Key, ".husky/_")
	var out strings.Builder
	s := CheckAll(ctx, r, []string{wt, main}, false, &out)
	if s.Remaining != 1 || strings.Count(out.String(), "foreign    ") != 1 || !strings.Contains(out.String(), "foreign    "+main+":") {
		t.Fatalf("foreign local value: want one line naming %s, got %+v\n%s", main, s, out.String())
	}

	gitc(t, main, "config", "--local", Key, ".git/hooks")
	out.Reset()
	s = CheckAll(ctx, r, []string{wt, main}, true, &out)
	if s.Fixed != 1 || s.Remaining != 0 || !strings.Contains(out.String(), "fixed      "+main+":") || strings.Contains(out.String(), wt+":") {
		t.Fatalf("redundant local value: want one fix naming %s, got %+v\n%s", main, s, out.String())
	}
}

// Fix unsets every value in the scope; each one gets a line and a count.
func TestCheckAllCountsEveryValueFixed(t *testing.T) {
	root, _ := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	repo := newRepo(t, filepath.Join(root, "r"))
	gitc(t, repo, "config", "--local", "--add", Key, ".git/hooks")
	gitc(t, repo, "config", "--local", "--add", Key, filepath.Join(repo, ".git", "hooks"))
	var out strings.Builder
	s := CheckAll(ctx, r, []string{repo}, true, &out)
	if s.Fixed != 2 || strings.Count(out.String(), "fixed      ") != 2 || !strings.Contains(out.String(), `".git/hooks"`) {
		t.Fatalf("want 2 values fixed, each on its own line: %+v\n%s", s, out.String())
	}
}

// Inspecting a non-bare repo through its .git dir (git -C <repo>/.git, or a
// push with --git-dir) resolves a relative value against the worktree, as
// git does when the hook runs.
func TestInspectFromTheGitDir(t *testing.T) {
	root, _ := env(t, true)
	repo := newRepo(t, filepath.Join(root, "r"))
	gitc(t, repo, "config", "--local", Key, ".git/hooks")
	res, err := Inspect(context.Background(), git.NewSystemRunner(), filepath.Join(repo, ".git"))
	if err != nil || res.Kind != Redundant {
		t.Fatalf("want redundant, got %+v %v", res, err)
	}
}

// A path through a symlinked parent matches the real one even when its last
// part does not exist yet: the repo's own hooks dir removed, or a global
// hooks dir not created.
func TestMissingLeafThroughSymlinkedParent(t *testing.T) {
	root, globalHooks := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	repo := newRepo(t, filepath.Join(root, "real"))
	if err := os.RemoveAll(filepath.Join(repo, ".git", "hooks")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	gitc(t, repo, "config", "--local", Key, filepath.Join(link, ".git", "hooks"))
	if res, err := Inspect(ctx, r, repo); err != nil || res.Kind != Redundant {
		t.Fatalf("own hooks dir through a symlink: want redundant, got %+v %v", res, err)
	}

	rootLink := filepath.Join(root, "rootlink")
	if err := os.Symlink(root, rootLink); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(globalHooks); err == nil {
		t.Fatal("test wants the global hooks dir missing")
	}
	gitc(t, repo, "config", "--local", Key, filepath.Join(rootLink, filepath.Base(globalHooks)))
	if res, err := Inspect(ctx, r, repo); err != nil || res.Kind != OK {
		t.Fatalf("global hooks dir through a symlink: want ok, got %+v %v", res, err)
	}
}

// A foreign value from outside the repo's own config is not unset with
// git config --local; the advice says where it is set.
func TestAdviceForForeignOutsideTheRepo(t *testing.T) {
	root, _ := env(t, true)
	repo := newRepo(t, filepath.Join(root, "r"))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", Key)
	t.Setenv("GIT_CONFIG_VALUE_0", "/dev/null")
	res, err := Inspect(context.Background(), git.NewSystemRunner(), repo)
	if err != nil || res.Kind != Foreign || res.Scope != "command" {
		t.Fatalf("want foreign from command scope, got %+v %v", res, err)
	}
	if a := Advice(res); strings.Contains(a, "--local") || !strings.Contains(a, "command config") || !strings.Contains(a, "GIT_CONFIG_*") {
		t.Fatalf("command scope advice: %s", a)
	}
	sys := Result{Repo: repo, Kind: Foreign, Scope: "system", Value: "/dev/null", HooksDir: filepath.Join(repo, ".git", "hooks")}
	if a := Advice(sys); strings.Contains(a, "--local") || !strings.Contains(a, "system config") {
		t.Fatalf("system scope advice: %s", a)
	}
}

// A linked worktree of a submodule is listed as the submodule's checkout,
// not as the worktree itself.
func TestDiscoverSubmoduleWorktreeNamesTheCheckout(t *testing.T) {
	root, _ := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	sub := newRepo(t, filepath.Join(root, "sub"))
	commit(t, sub)
	super := newRepo(t, filepath.Join(root, "super"))
	gitc(t, super, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "s")
	wt := filepath.Join(root, "subwt")
	gitc(t, filepath.Join(super, "s"), "worktree", "add", "-q", "-b", "w", wt)
	got := Discover(ctx, r, []string{wt}, nil)
	if want := filepath.Join(super, "s"); len(got) != 1 || got[0] != want {
		t.Fatalf("Discover = %v, want [%s]", got, want)
	}
}
