package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHooksLogWired(t *testing.T) {
	if !hasSubcommand(hooksCmd, "log") {
		t.Fatal("gss hooks log is not wired")
	}
}

func TestWriteHookLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	t.Setenv("GSS_HOOK_LOG", path)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := writeHookLog([]string{"pre-push", "1"}, now); err != nil {
		t.Fatalf("writeHookLog: %v", err)
	}
	if err := writeHookLog([]string{"pre-push"}, now); err == nil {
		t.Error("one arg must be an error")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"hook":"pre-push","result":"refused","exit":1`) {
		t.Errorf("log line = %s", raw)
	}
}

// TestHooksCallTheLog pins the one-line calls in the privacy git hooks: an
// EXIT trap in each hook and a "chained" line before the hand-off. The
// behaviour is covered by ai/githooks/githooks_test.sh.
func TestHooksCallTheLog(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "ai", "githooks")
	for _, h := range []string{"pre-commit", "commit-msg", "pre-push"} {
		src, err := os.ReadFile(filepath.Join(dir, h))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), "hooks log "+h+` "$`) || !strings.Contains(string(src), "trap '") {
			t.Errorf("%s has no EXIT-trap call to gss hooks log", h)
		}
	}
	src, err := os.ReadFile(filepath.Join(dir, "_privacy_common.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `hooks log "$name" chained`) {
		t.Error("_privacy_common.sh does not log before the hand-off")
	}
}

// TestHooksLogDoesNotChangeOutcomes runs the real privacy hooks in a
// throwaway repo with a stub gss that records its args, prints noise and
// exits 7. A clean commit and a push handed to a repo-local hook still pass, a home path is still refused,
// and every run leaves its line.
func TestHooksLogDoesNotChangeOutcomes(t *testing.T) {
	for _, bin := range []string{"git", "bash"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	hooks, err := filepath.Abs(filepath.Join("..", "..", "..", "ai", "githooks"))
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	home := filepath.Join(work, "home")
	stubDir := filepath.Join(work, "bin")
	calls := filepath.Join(work, "calls.txt")
	repo := filepath.Join(work, "repo")
	origin := filepath.Join(work, "origin.git")
	for _, d := range []string{home, stubDir, repo} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stub := "#!/bin/sh\necho \"$*\" >> " + calls + "\necho noise; echo noise >&2\nexit 7\n"
	if err := os.WriteFile(filepath.Join(stubDir, "gss"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	// git takes any string as user.email; a bare name keeps this fixture
	// free of anything shaped like an address.
	gcfg := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gcfg, []byte("[user]\n\tname = Alice\n\temail = alice\n[init]\n\tdefaultBranch = main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home, "USER=alice", "HOSTNAME=alicebox", "GIT_CONFIG_GLOBAL=" + gcfg,
		"PRIVACY_GUARD_GITLEAKS=0",
	}
	run := func(dir string, args ...string) error {
		c := exec.Command("git", append([]string{"-c", "core.hooksPath=" + hooks}, args...)...)
		c.Dir, c.Env = dir, env
		out, err := c.CombinedOutput()
		if err != nil {
			t.Logf("git %v: %s", args, out)
		}
		return err
	}
	if err := run(work, "init", "-q", "--bare", origin); err != nil {
		t.Fatal(err)
	}
	if err := run(repo, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := run(repo, "remote", "add", "origin", origin); err != nil {
		t.Fatal(err)
	}

	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ok.txt", "plain text\n")
	if err := run(repo, "add", "ok.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repo, "commit", "-q", "-m", "clean"); err != nil {
		t.Fatalf("a clean commit was refused with the log stub in place: %v", err)
	}
	// A repo-local pre-push makes the guard hand off (exec), which skips the
	// EXIT trap; the "chained" line is what records that run.
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-push"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repo, "push", "-q", "origin", "HEAD"); err != nil {
		t.Fatalf("a clean push was refused with the log stub in place: %v", err)
	}
	write("bad.txt", "see "+home+"/notes\n")
	if err := run(repo, "add", "bad.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repo, "commit", "-q", "-m", "leak"); err == nil {
		t.Fatal("a home path was committed: logging changed the guard's outcome")
	}

	raw, err := os.ReadFile(calls)
	if err != nil {
		t.Fatalf("no hook logged a run: %v", err)
	}
	got := string(raw)
	for _, want := range []string{"hooks log pre-commit 0", "hooks log commit-msg 0", "hooks log pre-push chained", "hooks log pre-commit 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("calls missing %q:\n%s", want, got)
		}
	}
}
