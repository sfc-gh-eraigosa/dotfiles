package hookguard

import (
	"bytes"
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// env makes git hermetic for one test: a temp global config (holding the
// global core.hooksPath the privacy hooks would use), no system config, a
// temp HOME, and a fixed identity so commits work on any host. Returns the
// temp root and the global hooks dir.
func env(t *testing.T, withGlobal bool) (root, globalHooks string) {
	t.Helper()
	root = t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	globalHooks = filepath.Join(root, "global-hooks")
	cfg := filepath.Join(root, "gitconfig")
	body := ""
	if withGlobal {
		body = "[core]\n\thooksPath = " + globalHooks + "\n"
	}
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", root)
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "gss-test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "gss-test")
	}
	return root, globalHooks
}

func gitc(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v: %s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitc(t, dir, "init", "-q", "-b", "main")
	return dir
}

func commit(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitc(t, dir, "add", "f.txt")
	gitc(t, dir, "commit", "-q", "-m", "c")
}

func TestInspectClassifies(t *testing.T) {
	root, globalHooks := env(t, true)
	cases := []struct {
		name  string
		value func(repo string) string // "" = leave unset
		want  Kind
	}{
		{"unset", func(string) string { return "" }, OK},
		{"same as global", func(string) string { return globalHooks }, OK},
		{"same as global via ~", func(string) string { return "~/global-hooks" }, OK},
		{"own .git/hooks absolute", func(r string) string { return filepath.Join(r, ".git", "hooks") }, Redundant},
		{"own .git/hooks relative", func(string) string { return ".git/hooks" }, Redundant},
		{"husky", func(string) string { return ".husky/_" }, Foreign},
		{"dev null", func(string) string { return "/dev/null" }, Foreign},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepo(t, filepath.Join(root, "r", string(rune('a'+i))))
			if v := tc.value(repo); v != "" {
				gitc(t, repo, "config", "--local", Key, v)
			}
			res, err := Inspect(context.Background(), git.NewSystemRunner(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if res.Kind != tc.want {
				t.Fatalf("kind = %v, want %v (%+v)", res.Kind, tc.want, res)
			}
			if tc.want == Redundant && !res.Fixable() {
				t.Fatalf("redundant local value should be fixable: %+v", res)
			}
		})
	}
}

func TestInspectNoGlobalIsOK(t *testing.T) {
	root, _ := env(t, false)
	repo := newRepo(t, filepath.Join(root, "r"))
	gitc(t, repo, "config", "--local", Key, "/dev/null")
	res, err := Inspect(context.Background(), git.NewSystemRunner(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != OK || res.Global != "" {
		t.Fatalf("no global hooks => nothing switched off; got %+v", res)
	}
}

// The shape that leaked a feature, seen from a linked worktree: the main checkout's own
// config points at <main>/.git/hooks, which is the common dir's hooks dir.
func TestInspectFromLinkedWorktree(t *testing.T) {
	root, _ := env(t, true)
	main := newRepo(t, filepath.Join(root, "main"))
	commit(t, main)
	wt := filepath.Join(root, "wt")
	gitc(t, main, "worktree", "add", "-q", "-b", "w", wt)
	gitc(t, main, "config", "--local", Key, filepath.Join(main, ".git", "hooks"))

	res, err := Inspect(context.Background(), git.NewSystemRunner(), wt)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != Redundant || res.Scope != "local" {
		t.Fatalf("got %+v, want redundant/local", res)
	}
}

func TestCheckRefusalMessage(t *testing.T) {
	root, globalHooks := env(t, true)
	ctx := context.Background()
	r := git.NewSystemRunner()

	red := newRepo(t, filepath.Join(root, "red"))
	gitc(t, red, "config", "--local", Key, filepath.Join(red, ".git", "hooks"))
	err := Check(ctx, r, red)
	if !stderrors.Is(err, ErrOverride) {
		t.Fatalf("want ErrOverride, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{filepath.Join(red, ".git", "hooks"), globalHooks, "privacy guard", "will not run", "gss hooks check --fix " + red} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}

	foreign := newRepo(t, filepath.Join(root, "foreign"))
	gitc(t, foreign, "config", "--local", Key, ".husky/_")
	msg = Check(ctx, r, foreign).Error()
	for _, want := range []string{".husky/_", "privacy guard", "exec that file after the privacy check", "config --local --unset core.hooksPath"} {
		if !strings.Contains(msg, want) {
			t.Errorf("foreign message lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "gss hooks check --fix") {
		t.Errorf("foreign values are never auto-fixed; message must not offer --fix:\n%s", msg)
	}

	clean := newRepo(t, filepath.Join(root, "clean"))
	if err := Check(ctx, r, clean); err != nil {
		t.Fatalf("clean repo refused: %v", err)
	}
	if err := Check(ctx, r, filepath.Join(root, "not-a-repo")); err != nil {
		t.Fatalf("non-repo is git's error to report, not ours: %v", err)
	}
}

// The refusal itself: a real push through Runner is stopped before it
// leaves, and goes through once the override is gone.
func TestRunnerRefusesPush(t *testing.T) {
	root, _ := env(t, true)
	ctx := context.Background()
	remote := filepath.Join(root, "remote.git")
	gitc(t, root, "init", "-q", "--bare", remote)
	repo := newRepo(t, filepath.Join(root, "repo"))
	commit(t, repo)
	gitc(t, repo, "remote", "add", "origin", remote)
	gitc(t, repo, "config", "--local", Key, filepath.Join(repo, ".git", "hooks"))

	g := Runner{Inner: git.NewSystemRunner()}
	if _, err := g.Run(ctx, "-C", repo, "status", "--short"); err != nil {
		t.Fatalf("non-push calls pass through: %v", err)
	}
	_, err := g.Run(ctx, "-C", repo, "push", "--force-with-lease", "--set-upstream", "origin", "main")
	if !stderrors.Is(err, ErrOverride) {
		t.Fatalf("push with an override: want ErrOverride, got %v", err)
	}
	if out, err := exec.Command("git", "-C", remote, "rev-parse", "--verify", "-q", "refs/heads/main").CombinedOutput(); err == nil {
		t.Fatalf("refused push still reached the remote: %s", out)
	}

	gitc(t, repo, "config", "--local", "--unset", Key)
	if out, err := g.Run(ctx, "-C", repo, "push", "origin", "main"); err != nil {
		t.Fatalf("push without an override: %v: %s", err, out)
	}
	gitc(t, remote, "rev-parse", "--verify", "refs/heads/main")
}

type fakeVerifier struct{ calls int }

func (f *fakeVerifier) Verify(context.Context, string, bool) error { f.calls++; return nil }

// The token gate is not reached (so the token is not consumed) while an
// override is in place.
func TestApproverChecksBeforeToken(t *testing.T) {
	root, _ := env(t, true)
	repo := newRepo(t, filepath.Join(root, "repo"))
	inner := &fakeVerifier{}
	a := Approver{Inner: inner, Git: git.NewSystemRunner()}

	gitc(t, repo, "config", "--local", Key, "/dev/null")
	if err := a.Verify(context.Background(), repo, false); !stderrors.Is(err, ErrOverride) {
		t.Fatalf("want ErrOverride, got %v", err)
	}
	if inner.calls != 0 {
		t.Fatalf("token gate ran despite the override")
	}
	gitc(t, repo, "config", "--local", "--unset", Key)
	if err := a.Verify(context.Background(), repo, false); err != nil || inner.calls != 1 {
		t.Fatalf("clean repo: err=%v calls=%d", err, inner.calls)
	}
}

func TestInvocation(t *testing.T) {
	cases := []struct {
		name               string
		args               []string
		dir, sub, override string
	}{
		{"-C", []string{"/r", "push", "origin", "b"}, "/r", "push", ""},
		{"push", nil, ".", "push", ""},
		{"-C", []string{"/r", "-c", "core.hooksPath=/x", "push"}, "/r", "push", "core.hooksPath=/x"},
		{"-C", []string{"/r", "-c", "user.name=x", "push"}, "/r", "push", ""},
		{"-ccore.hookspath=/dev/null", []string{"push"}, ".", "push", "core.hookspath=/dev/null"},
		{"--config-env=core.hooksPath=HP", []string{"push"}, ".", "push", "core.hooksPath=HP"},
		{"-C", []string{"/r", "-C", "sub", "status"}, "/r/sub", "status", ""},
		{"--no-pager", []string{"-C", "/r", "log"}, "/r", "log", ""},
		{"-C", []string{"/r", "config", "remote.pushDefault"}, "/r", "config", ""},
		{"--git-dir", []string{"/r/.git", "--work-tree", "/r", "push"}, "/r", "push", ""},
		{"--git-dir=/r/.git", []string{"push"}, "/r/.git", "push", ""},
		{"-C", []string{"/a", "--work-tree=w", "push"}, "/a/w", "push", ""},
		{"--exec-path=/x", []string{"push"}, ".", "push", ""},
	}
	for _, tc := range cases {
		c := invocation(tc.name, tc.args)
		if c.dir != tc.dir || c.sub != tc.sub || c.override != tc.override {
			t.Errorf("invocation(%q, %v) = %+v, want dir %q sub %q override %q", tc.name, tc.args, c, tc.dir, tc.sub, tc.override)
		}
	}
}

func TestFixOnlyRedundantAndOnlyLocal(t *testing.T) {
	root, _ := env(t, true)
	ctx := context.Background()
	r := git.NewSystemRunner()
	globalBefore, err := os.ReadFile(os.Getenv("GIT_CONFIG_GLOBAL"))
	if err != nil {
		t.Fatal(err)
	}

	red := newRepo(t, filepath.Join(root, "red"))
	gitc(t, red, "config", "--local", Key, ".git/hooks")
	sample := filepath.Join(red, ".git", "hooks", "pre-push.sample")
	res, _ := Inspect(ctx, r, red)
	after, err := Fix(ctx, r, res)
	if err != nil || after.Kind != OK {
		t.Fatalf("fix redundant: kind=%v err=%v", after.Kind, err)
	}
	if out, err := exec.Command("git", "-C", red, "config", "--local", "--get", Key).CombinedOutput(); err == nil {
		t.Fatalf("local value still set: %s", out)
	}
	if _, err := os.Stat(sample); err != nil {
		t.Fatalf("fix must not touch hook files: %v", err)
	}

	foreign := newRepo(t, filepath.Join(root, "foreign"))
	gitc(t, foreign, "config", "--local", Key, "/dev/null")
	res, _ = Inspect(ctx, r, foreign)
	after, err = Fix(ctx, r, res)
	if err != nil || after.Kind != Foreign {
		t.Fatalf("foreign must be left alone: kind=%v err=%v", after.Kind, err)
	}
	if got := gitc(t, foreign, "config", "--local", "--get", Key); got != "/dev/null" {
		t.Fatalf("foreign value changed to %q", got)
	}

	globalAfter, _ := os.ReadFile(os.Getenv("GIT_CONFIG_GLOBAL"))
	if !bytes.Equal(globalBefore, globalAfter) {
		t.Fatalf("global config changed:\n%s", globalAfter)
	}
}
