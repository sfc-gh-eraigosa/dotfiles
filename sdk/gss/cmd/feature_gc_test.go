package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/worktree"
	wtgit "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/worktree/git"
)

func gcGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=gss-test",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=gss-test",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gcCmdEnv wires the real gc command to a temp repo + registry + fake gh with
// one removable (merged) and one open-PR worker. It returns the exit codes the
// command asked for.
func gcCmdEnv(t *testing.T) *[]int {
	t.Helper()
	base := t.TempDir()
	origin, mainDir, root := filepath.Join(base, "o.git"), filepath.Join(base, "main"), filepath.Join(base, "wt")
	for _, d := range []string{origin, mainDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gcGit(t, origin, "init", "--bare", "-b", "main")
	gcGit(t, mainDir, "init", "-b", "main")
	gcGit(t, mainDir, "remote", "add", "origin", origin)
	_ = os.WriteFile(filepath.Join(mainDir, "R"), []byte("r"), 0o644)
	gcGit(t, mainDir, "add", "R")
	gcGit(t, mainDir, "commit", "-m", "init")
	gcGit(t, mainDir, "push", "-u", "origin", "main")

	store := registry.NewStore(filepath.Join(base, "registry.json"))
	ghc := ghfake.NewClient()
	var workers []registry.Worker
	for i, c := range []struct{ leaf, state string }{{"done1", "MERGED"}, {"live1", "OPEN"}} {
		branch := "feature/auth/erai/" + c.leaf
		wt := filepath.Join(root, "octo/proj/auth/erai", c.leaf)
		_ = os.MkdirAll(filepath.Dir(wt), 0o755)
		gcGit(t, mainDir, "worktree", "add", "-b", branch, wt, "main")
		gcGit(t, wt, "push", "-u", "origin", branch)
		num := 10 + i
		ghc.SeedPR(gh.PR{Number: num, State: c.state})
		workers = append(workers, registry.Worker{User: "erai", Purpose: c.leaf, Branch: branch, Worktree: wt,
			BaseBranch: "main", PRState: "open", PRURL: fmt.Sprintf("https://github.com/octo/proj/pull/%d", num)})
	}
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1, Features: []registry.Feature{{Name: "auth", DefaultBaseBranch: "main", Workers: workers}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runner := git.NewSystemRunner()
	oldF, oldE := gcServiceFactory, gcExit
	t.Cleanup(func() { gcServiceFactory, gcExit = oldF, oldE })
	gcServiceFactory = func() (*feature.Service, error) {
		return &feature.Service{Store: store, Backend: wtgit.NewBackend(mainDir, runner), Git: runner, GH: ghc,
			WorktreeRoot: root, NWO: "octo/proj", RepoPath: mainDir}, nil
	}
	codes := &[]int{}
	gcExit = func(c int) { *codes = append(*codes, c) }
	return codes
}

func runGCCmd(t *testing.T, flags ...string) string {
	t.Helper()
	for _, name := range []string{"apply", "json", "feature"} {
		f := featureGCCmd.Flags().Lookup(name)
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	for i := 0; i < len(flags); i++ {
		k := strings.TrimPrefix(flags[i], "--")
		v := "true"
		if k == "feature" {
			i++
			v = flags[i]
		}
		if err := featureGCCmd.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	featureGCCmd.SetOut(&out)
	featureGCCmd.Run(featureGCCmd, nil)
	return out.String()
}

func TestFeatureGC_Wired(t *testing.T) {
	if !hasSubcommand(featureCmd, "gc") {
		t.Fatal("gss feature gc not wired")
	}
	for _, name := range []string{"feature", "apply", "json"} {
		if featureGCCmd.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s", name)
		}
	}
	if featureGCCmd.Flags().Lookup("apply").DefValue != "false" {
		t.Error("--apply must default to a dry run")
	}
}

func TestFeatureGC_DryRunText(t *testing.T) {
	codes := gcCmdEnv(t)
	out := runGCCmd(t)
	for _, want := range []string{"would remove", "auth/erai/done1", "PR #10 merged", "skip", "auth/erai/live1", "PR #11 is open", "dry run"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(*codes) != 0 {
		t.Errorf("dry run exited %v", *codes)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(gcWT(t)), "done1")); err != nil {
		t.Errorf("dry run removed a worktree: %v", err)
	}
}

// gcWT returns the worker parent dir of the active gc service.
func gcWT(t *testing.T) string {
	svc, _ := gcServiceFactory()
	return filepath.Join(svc.WorktreeRoot, "octo/proj/auth/erai", "x")
}

func TestFeatureGC_ApplyJSON(t *testing.T) {
	codes := gcCmdEnv(t)
	out := runGCCmd(t, "--apply", "--json")
	var got struct {
		Removed    []string                          `json:"removed"`
		Skipped    []struct{ Worker, Reason string } `json:"skipped"`
		FreedBytes int64                             `json:"freed_bytes"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got.Removed) != 1 || got.Removed[0] != "auth/erai/done1" {
		t.Errorf("removed = %v", got.Removed)
	}
	if len(got.Skipped) != 1 || got.Skipped[0].Worker != "auth/erai/live1" || got.Skipped[0].Reason == "" {
		t.Errorf("skipped = %+v", got.Skipped)
	}
	if got.FreedBytes <= 0 {
		t.Errorf("freed_bytes = %d", got.FreedBytes)
	}
	if len(*codes) != 0 {
		t.Errorf("exit codes %v on success", *codes)
	}
}

func TestFeatureGC_ExitNonZeroWhenRemovalFails(t *testing.T) {
	codes := gcCmdEnv(t)
	svc, _ := gcServiceFactory()
	// Make the removal fail after the checks pass: a backend that errors.
	svc.Backend = failingBackend{svc.Backend}
	gcServiceFactory = func() (*feature.Service, error) { return svc, nil }
	out := runGCCmd(t, "--apply")
	if !strings.Contains(out, "FAILED") {
		t.Errorf("want a FAILED line:\n%s", out)
	}
	if len(*codes) != 1 || (*codes)[0] != 1 {
		t.Errorf("exit codes = %v, want [1]", *codes)
	}
}

type worktreeBackend = worktree.Backend

type failingBackend struct{ worktreeBackend }

func (failingBackend) Remove(string, bool) error { return fmt.Errorf("boom") }
