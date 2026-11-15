package hookguard

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// A -c core.hooksPath on the push itself is refused even when the repo's own
// config is clean.
func TestRunnerRefusesCommandLineOverride(t *testing.T) {
	root, _ := env(t, true)
	repo := newRepo(t, filepath.Join(root, "repo"))
	g := Runner{Inner: git.NewSystemRunner()}
	_, err := g.Run(context.Background(), "-C", repo, "-c", "core.hooksPath=/dev/null", "push", "origin", "main")
	if !stderrors.Is(err, ErrOverride) || !strings.Contains(err.Error(), "command line") {
		t.Fatalf("want a command-line refusal, got %v", err)
	}
}

// Two redundant local values: Fix removes both (a plain --unset would fail on
// a multi-valued key).
func TestFixRemovesEveryRedundantValue(t *testing.T) {
	root, _ := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	repo := newRepo(t, filepath.Join(root, "r"))
	gitc(t, repo, "config", "--local", "--add", Key, ".git/hooks")
	gitc(t, repo, "config", "--local", "--add", Key, filepath.Join(repo, ".git", "hooks"))
	res, err := Inspect(ctx, r, repo)
	if err != nil || !res.Fixable() || len(res.Values) != 2 {
		t.Fatalf("two own values: want fixable with 2 values, got %+v %v", res, err)
	}
	after, err := Fix(ctx, r, res)
	if err != nil || after.Kind != OK {
		t.Fatalf("Fix: %+v %v", after, err)
	}
}

// A foreign value hidden behind a redundant one is not Fixable: unsetting the
// key would remove the foreign hooks too.
func TestMixedValuesAreNotFixable(t *testing.T) {
	root, _ := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	repo := newRepo(t, filepath.Join(root, "r"))
	gitc(t, repo, "config", "--local", "--add", Key, ".husky/_")
	gitc(t, repo, "config", "--local", "--add", Key, ".git/hooks")
	res, err := Inspect(ctx, r, repo)
	if err != nil || res.Kind != Redundant || res.Fixable() {
		t.Fatalf("want redundant and not fixable, got %+v %v", res, err)
	}
	if a := Advice(res); !strings.Contains(a, "2 core.hooksPath values") || !strings.Contains(a, ".husky/_") {
		t.Fatalf("advice should name both values: %s", a)
	}
	if after, _ := Fix(ctx, r, res); after.Kind != Redundant || gitc(t, repo, "config", "--local", "--get-all", Key) != ".husky/_\n.git/hooks" {
		t.Fatalf("Fix must leave a mixed scope alone: %+v", after)
	}
}

// A redundant value from a scope gss does not edit is reported with the scope
// to fix it in.
func TestAdviceForRedundantOutsideTheRepo(t *testing.T) {
	res := Result{Repo: "/r", Kind: Redundant, Scope: "system", Value: "/r/.git/hooks", Values: []string{"/r/.git/hooks"}, ownOnly: true}
	if res.Fixable() {
		t.Fatal("a system value is not Fixable")
	}
	if a := Advice(res); !strings.Contains(a, "system config") {
		t.Fatalf("advice: %s", a)
	}
}

// With extensions.worktreeConfig a linked worktree can carry its own value:
// Inspect sees it, Fix unsets it with --worktree, and Discover lists the
// worktree beside its main checkout.
func TestWorktreeScope(t *testing.T) {
	root, _ := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	main := newRepo(t, filepath.Join(root, "main"))
	commit(t, main)
	wt := filepath.Join(root, "wt")
	gitc(t, main, "worktree", "add", "-q", "-b", "w", wt)
	gitc(t, main, "config", "extensions.worktreeConfig", "true")
	gitc(t, wt, "config", "--worktree", Key, filepath.Join(main, ".git", "hooks"))

	if repos := Discover(ctx, r, []string{wt}, nil); len(repos) != 2 {
		t.Fatalf("Discover = %v, want the main checkout and the worktree", repos)
	}
	res, err := Inspect(ctx, r, wt)
	if err != nil || res.Scope != "worktree" || !res.Fixable() {
		t.Fatalf("want a fixable worktree value, got %+v %v", res, err)
	}
	if after, err := Fix(ctx, r, res); err != nil || after.Kind != OK {
		t.Fatalf("Fix: %+v %v", after, err)
	}
}

// A bare repo's relative value resolves against the git dir; a symlink to the
// repo's own hooks dir is still its own hooks dir.
func TestBareAndSymlinkedOwnHooks(t *testing.T) {
	root, _ := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	bare := filepath.Join(root, "b.git")
	gitc(t, root, "init", "-q", "--bare", bare)
	gitc(t, bare, "config", Key, "hooks")
	if res, err := Inspect(ctx, r, bare); err != nil || res.Kind != Redundant {
		t.Fatalf("bare: %+v %v", res, err)
	}

	repo := newRepo(t, filepath.Join(root, "r"))
	link := filepath.Join(root, "own-hooks")
	if err := os.Symlink(filepath.Join(repo, ".git", "hooks"), link); err != nil {
		t.Fatal(err)
	}
	gitc(t, repo, "config", "--local", Key, link)
	if res, err := Inspect(ctx, r, repo); err != nil || res.Kind != Redundant {
		t.Fatalf("symlink: %+v %v", res, err)
	}
}

// failConfig fails every `git config` read the way an unreadable file does
// (an exit code other than 1).
type failConfig struct{ git.Runner }

func (f failConfig) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	for _, a := range args {
		if a == "config" {
			return []byte("fatal: bad config"), exec.Command("sh", "-c", "exit 128").Run()
		}
	}
	return f.Runner.Run(ctx, name, args...)
}

// Check refuses when the config cannot be read, and still passes a directory
// that is not a repo (the push fails there on its own).
func TestCheckFailsClosedOnUnreadableConfig(t *testing.T) {
	root, _ := env(t, true)
	ctx := context.Background()
	repo := newRepo(t, filepath.Join(root, "r"))
	if err := Check(ctx, failConfig{git.NewSystemRunner()}, repo); err == nil || !strings.Contains(err.Error(), "cannot tell") {
		t.Fatalf("unreadable config: want a refusal, got %v", err)
	}
	if err := Check(ctx, git.NewSystemRunner(), filepath.Join(root, "not-a-repo")); err != nil {
		t.Fatalf("not a repo: want nil, got %v", err)
	}
}

// A checkout reached through a symlink under a scan parent is scanned.
func TestDiscoverFollowsSymlinkedCheckouts(t *testing.T) {
	root, _ := env(t, true)
	repo := newRepo(t, filepath.Join(root, "elsewhere", "r"))
	parent := filepath.Join(root, "parent")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repo, filepath.Join(parent, "r")); err != nil {
		t.Fatal(err)
	}
	if repos := Discover(context.Background(), git.NewSystemRunner(), nil, []string{parent}); len(repos) != 1 {
		t.Fatalf("Discover = %v, want the symlinked checkout", repos)
	}
}

// An explicit empty value still replaces the global hooks path.
func TestEmptyValueIsAnOverride(t *testing.T) {
	root, _ := env(t, true)
	repo := newRepo(t, filepath.Join(root, "r"))
	gitc(t, repo, "config", "--local", Key, "")
	res, err := Inspect(context.Background(), git.NewSystemRunner(), repo)
	if err != nil || res.Kind != Foreign || res.Scope != "local" || res.Value != "" {
		t.Fatalf("want a foreign local empty value, got %+v %v", res, err)
	}
}

// Two global values (a second global file or an include) are not an error:
// git uses the last.
func TestTwoGlobalValues(t *testing.T) {
	root, globalHooks := env(t, true)
	cfg := os.Getenv("GIT_CONFIG_GLOBAL")
	if err := os.WriteFile(cfg, []byte("[core]\n\thooksPath = /elsewhere\n\thooksPath = "+globalHooks+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := newRepo(t, filepath.Join(root, "r"))
	res, err := Inspect(context.Background(), git.NewSystemRunner(), repo)
	if err != nil || res.Kind != OK || res.Global != globalHooks {
		t.Fatalf("want ok with the last global value, got %+v %v", res, err)
	}
	if err := Check(context.Background(), git.NewSystemRunner(), repo); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

// With extensions.worktreeConfig, a scan of the parent dir also finds a
// linked worktree gss never registered, and one --fix pass clears a
// redundant value in both the worktree and the local scope.
func TestScanFindsUnregisteredWorktreeAndFixesBothScopes(t *testing.T) {
	root, _ := env(t, true)
	ctx, r := context.Background(), git.NewSystemRunner()
	parent := filepath.Join(root, "git")
	main := newRepo(t, filepath.Join(parent, "r"))
	commit(t, main)
	wt := filepath.Join(root, "elsewhere", "wt")
	gitc(t, main, "worktree", "add", "-q", "-b", "w", wt)
	gitc(t, main, "config", "extensions.worktreeConfig", "true")
	gitc(t, main, "config", "--local", Key, filepath.Join(main, ".git", "hooks"))
	gitc(t, wt, "config", "--worktree", Key, filepath.Join(main, ".git", "hooks"))

	repos := Discover(ctx, r, nil, []string{parent})
	found := false
	for _, p := range repos {
		found = found || p == wt
	}
	if !found {
		t.Fatalf("Discover = %v, want it to include %s", repos, wt)
	}
	var out strings.Builder
	s := CheckAll(ctx, r, []string{wt}, true, &out)
	if s.Fixed != 1 || s.Remaining != 0 {
		t.Fatalf("one pass: %+v\n%s", s, out.String())
	}
}

// Approver lets a directory that is not a repo through to the token gate, as
// Check does: the push fails there on its own.
func TestApproverPassesANonRepo(t *testing.T) {
	root, _ := env(t, true)
	inner := &fakeVerifier{}
	a := Approver{Inner: inner, Git: git.NewSystemRunner()}
	if err := a.Verify(context.Background(), filepath.Join(root, "nope"), false); err != nil || inner.calls != 1 {
		t.Fatalf("non-repo: err=%v calls=%d", err, inner.calls)
	}
}
