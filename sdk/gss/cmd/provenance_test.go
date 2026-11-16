package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/provenance"
)

func TestProvenanceWired(t *testing.T) {
	for _, sub := range []string{"stamp", "show"} {
		if !hasSubcommand(provenanceCmd, sub) {
			t.Errorf("gss provenance %s is not wired", sub)
		}
	}
}

func TestStampMessage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	msg := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	if err := os.WriteFile(msg, []byte("subject\n\nbody\n\nCo-Authored-By: X <x>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info := provenance.Info{Harness: "claude", Session: "s-1", HostID: "h-0123456789"}
	ctx := context.Background()
	r := git.NewSystemRunner()
	for i := 0; i < 2; i++ { // twice: an amend must not stamp again
		if err := stampMessage(ctx, r, []string{msg}, info); err != nil {
			t.Fatalf("stamp: %v", err)
		}
	}
	raw, _ := os.ReadFile(msg)
	want := "Co-Authored-By: X <x>\nBuilt-By: claude\nBuilt-On: h-0123456789\nBuilt-Session: s-1\nHooks: global\n"
	if !strings.HasSuffix(string(raw), want) {
		t.Errorf("message =\n%s\nwant suffix\n%s", raw, want)
	}

	plain := filepath.Join(t.TempDir(), "msg")
	if err := os.WriteFile(plain, []byte("subject\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stampMessage(ctx, r, []string{plain}, provenance.Info{HostID: "h-1"}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(plain); string(raw) != "subject\n" {
		t.Errorf("no assistant, but the message changed: %q", raw)
	}
	if err := stampMessage(ctx, r, nil, info); err == nil {
		t.Error("no file argument must be an error")
	}
}

func TestShowProvenance(t *testing.T) {
	var b bytes.Buffer
	showProvenance(&b, provenance.Info{Harness: "claude", Session: "s-1", HostID: "h-1"}, nil)
	if got := b.String(); got != "built-by\tclaude\nbuilt-on\th-1\nsession\ts-1\n" {
		t.Errorf("show = %q", got)
	}
	b.Reset()
	showProvenance(&b, provenance.Info{}, []string{"box", "other"})
	if got := b.String(); got != provenance.HostID("box")+"\tbox\n"+provenance.HostID("other")+"\tother\n" {
		t.Errorf("show --host = %q", got)
	}
}

// TestCommitMsgHookStampsBeforeItJudges runs the real commit-msg hook with a
// stub gss standing in for `gss provenance stamp`. A stamp of plain text
// reaches the commit; a stamp that adds a home path is refused, which shows
// the check reads the message after the stamp.
func TestCommitMsgHookStampsBeforeItJudges(t *testing.T) {
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
	repo := filepath.Join(work, "repo")
	for _, d := range []string{home, stubDir, repo} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// git takes any string as user.email; a bare name keeps this fixture
	// free of anything shaped like an address.
	gcfg := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gcfg, []byte("[user]\n\tname = Alice\n\temail = alice\n[init]\n\tdefaultBranch = main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	trailerFile := filepath.Join(work, "trailer")
	stub := "#!/bin/sh\n[ \"$1 $2\" = \"provenance stamp\" ] || exit 0\nprintf '\\n%s\\n' \"$(cat " + trailerFile + ")\" >> \"$3\"\n"
	if err := os.WriteFile(filepath.Join(stubDir, "gss"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home, "USER=alice", "HOSTNAME=alicebox", "GIT_CONFIG_GLOBAL=" + gcfg,
		"PRIVACY_GUARD_GITLEAKS=0",
	}
	git := func(args ...string) (string, error) {
		c := exec.Command("git", append([]string{"-c", "core.hooksPath=" + hooks}, args...)...)
		c.Dir, c.Env = repo, env
		out, err := c.CombinedOutput()
		return string(out), err
	}
	if out, err := git("init", "-q"); err != nil {
		t.Fatal(out)
	}

	if err := os.WriteFile(trailerFile, []byte("Built-By: stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := git("commit", "-q", "--allow-empty", "-m", "clean"); err != nil {
		t.Fatalf("commit refused: %s", out)
	}
	if out, _ := git("log", "-1", "--format=%B"); !strings.Contains(out, "Built-By: stub") {
		t.Errorf("stamp did not reach the commit:\n%s", out)
	}

	if err := os.WriteFile(trailerFile, []byte("Built-By: "+home+"/x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := git("commit", "-q", "--allow-empty", "-m", "stamped leak"); err == nil {
		t.Fatalf("a stamped home path was committed; the check ran before the stamp:\n%s", out)
	}
}
