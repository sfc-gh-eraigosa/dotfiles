package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHooksCheckWired pins `gss hooks check` and its --fix flag.
func TestHooksCheckWired(t *testing.T) {
	if !hasSubcommand(hooksCmd, "check") {
		t.Fatal("gss hooks check is not wired")
	}
	if hooksCheckCmd.Flags().Lookup("fix") == nil {
		t.Fatal("gss hooks check has no --fix flag")
	}
}

// TestPushPathsUseHookGuard pins the wiring that makes every push gss makes
// refuse a repo whose own core.hooksPath switches off the global privacy
// hooks: each command file that builds a pushing service wraps its git
// runner and its approval gate in hookguard. Behaviour is tested in
// internal/hookguard; this catches a future edit that drops the wrapper.
func TestPushPathsUseHookGuard(t *testing.T) {
	for _, f := range []string{"push.go", "pr.go", "feature_service.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"hookguard.Runner{Inner: sys}", "hookguard.Approver{"} {
			if !strings.Contains(string(src), want) {
				t.Errorf("cmd/%s does not contain %q", f, want)
			}
		}
	}
}

// hermeticGit points git at a temp global config that sets the global hooks
// path, with no system config and a temp HOME.
func hermeticGit(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	cfg := filepath.Join(root, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[core]\n\thooksPath = "+filepath.Join(root, "global-hooks")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", root)
	return root
}

// `gss hooks check --fix <path>` repairs a redundant override and says so.
func TestHooksCheckFixesAPath(t *testing.T) {
	root := hermeticGit(t)
	repo := filepath.Join(root, "r")
	for _, args := range [][]string{{"init", "-q", repo}, {"-C", repo, "config", "--local", "core.hooksPath", ".git/hooks"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	hooksFix = true
	t.Cleanup(func() { hooksFix = false })
	var out bytes.Buffer
	hooksCheckCmd.SetOut(&out)
	t.Cleanup(func() { hooksCheckCmd.SetOut(nil) })
	hooksCheckCmd.Run(hooksCheckCmd, []string{repo})
	if !strings.Contains(out.String(), "fixed      "+repo) || !strings.Contains(out.String(), "1 fixed, 0 overrides remain") {
		t.Fatalf("output:\n%s", out.String())
	}
}

// scanParents lists every org under ~/github and ~/GitHub, then ~/git.
func TestScanParents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, d := range []string{"github/a", "GitHub/b"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := scanParents()
	want := []string{filepath.Join(home, "github", "a"), filepath.Join(home, "GitHub", "b"), filepath.Join(home, "git")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("scanParents = %v, want %v", got, want)
	}
}
