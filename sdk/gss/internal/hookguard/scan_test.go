package hookguard

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
)

// A temp HOME laid out like a real one: ~/github/<org>/<repo>, ~/git/<repo>,
// a plain directory, and a registry worktree of one of the repos.
func TestDiscoverAndCheckAll(t *testing.T) {
	root, _ := env(t, true)
	ctx := context.Background()
	r := git.NewSystemRunner()

	red := newRepo(t, filepath.Join(root, "github", "org", "red"))
	commit(t, red)
	gitc(t, red, "config", "--local", Key, filepath.Join(red, ".git", "hooks"))
	foreign := newRepo(t, filepath.Join(root, "git", "foreign"))
	gitc(t, foreign, "config", "--local", Key, ".husky/_")
	clean := newRepo(t, filepath.Join(root, "git", "clean"))
	if err := os.MkdirAll(filepath.Join(root, "github", "org", "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, "worktrees", "red-w")
	gitc(t, red, "worktree", "add", "-q", "-b", "w", wt)

	parents := []string{filepath.Join(root, "github", "org"), filepath.Join(root, "git"), filepath.Join(root, "missing")}
	repos := Discover(ctx, r, []string{wt, filepath.Join(root, "gone")}, parents)
	want := []string{foreign, clean, red}
	if strings.Join(repos, ",") != strings.Join(sorted(want), ",") {
		t.Fatalf("Discover = %v, want %v (worktree folded into its main checkout, plain dir skipped)", repos, sorted(want))
	}

	var out bytes.Buffer
	s := CheckAll(ctx, r, repos, false, &out)
	if s.Checked != 3 || s.Remaining != 2 || s.Fixed != 0 {
		t.Fatalf("check: %+v\n%s", s, out.String())
	}
	if strings.Contains(out.String(), clean+":") {
		t.Errorf("clean repo printed:\n%s", out.String())
	}
	assertLine(t, out.String(), "redundant  "+red)
	assertLine(t, out.String(), "foreign    "+foreign)

	out.Reset()
	s = CheckAll(ctx, r, repos, true, &out)
	if s.Fixed != 1 || s.Remaining != 1 {
		t.Fatalf("--fix: %+v\n%s", s, out.String())
	}
	assertLine(t, out.String(), "fixed      "+red)
	assertLine(t, out.String(), "foreign    "+foreign)

	out.Reset()
	s = CheckAll(ctx, r, repos, true, &out)
	if s.Fixed != 0 || s.Remaining != 1 {
		t.Fatalf("second --fix is a no-op for the fixed repo: %+v\n%s", s, out.String())
	}
}

func TestCheckAllReportsBadPathAndMissingGlobal(t *testing.T) {
	root, _ := env(t, false)
	repo := newRepo(t, filepath.Join(root, "repo"))
	var out bytes.Buffer
	s := CheckAll(context.Background(), git.NewSystemRunner(), []string{repo, filepath.Join(root, "nope")}, false, &out)
	if s.Errors != 1 || s.Remaining != 0 {
		t.Fatalf("%+v\n%s", s, out.String())
	}
	assertLine(t, out.String(), "error      "+filepath.Join(root, "nope"))
	assertLine(t, out.String(), "note: the global core.hooksPath is not set")
}

func assertLine(t *testing.T, out, prefix string) {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, prefix) {
			return
		}
	}
	t.Errorf("no line starting %q in:\n%s", prefix, out)
}

func sorted(s []string) []string {
	c := append([]string(nil), s...)
	for i := range c {
		for j := i + 1; j < len(c); j++ {
			if c[j] < c[i] {
				c[i], c[j] = c[j], c[i]
			}
		}
	}
	return c
}
