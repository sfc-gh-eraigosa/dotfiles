package cmd

import (
	"errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// canary is the token value these tests put in the environment; it must
// never reach any output.
var canary = strings.Join([]string{"ghs", "FIXTURE_TOKEN_DO_NOT_PRINT"}, "_")

func isolateCreds(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	t.Setenv("GH_CONFIG_DIR", t.TempDir()) // no gh login
	// Never let a cmd test reach this machine's gh credential.
	gh.StubAskGHForTests(t)
}

func TestAuthStatusNamesTheSourceAndNeverTheToken(t *testing.T) {
	isolateCreds(t)
	t.Setenv("GH_TOKEN", canary)
	out, errb, err := run("auth", "status", "-R", "sfc-gh-eraigosa/dotfiles")
	if err != nil {
		t.Fatalf("%v\n%s%s", err, out, errb)
	}
	if !strings.Contains(out, "GH_TOKEN") {
		t.Errorf("want the source named, got:\n%s", out)
	}
	if !strings.Contains(out, "sfc-gh-eraigosa/dotfiles") {
		t.Errorf("want the target named, got:\n%s", out)
	}
	if strings.Contains(out+errb, canary) {
		t.Fatal("auth status leaked the token value")
	}
	// A redacted fingerprint is fine and useful; the value is not.
	if !strings.Contains(out, "***") {
		t.Errorf("want a redaction marker, got:\n%s", out)
	}
}

func TestAuthStatusWithNoCredentialIsUsage(t *testing.T) {
	isolateCreds(t)
	out, _, err := run("auth", "status", "-R", "o/r")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v\n%s", err, out)
	}
	if !strings.Contains(err.Error(), "GH_TOKEN") || !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("the error should say how to fix it: %v", err)
	}
}

func TestAuthStatusHonoursTheAuthFlag(t *testing.T) {
	isolateCreds(t)
	t.Setenv("GH_TOKEN", canary)
	dir := t.TempDir()
	// Built from parts so no source line reads like a credential literal.
	body := "github.com:\n    " + "oauth_" + "token" + ": from-gh\n"
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_CONFIG_DIR", dir)
	out, _, err := run("auth", "status", "-R", "o/r", "--auth", "gh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "gh login") {
		t.Errorf("want the pinned source, got:\n%s", out)
	}
	if _, _, err := run("auth", "status", "-R", "o/r", "--auth", "nonsense"); !errors.Is(err, ErrUsage) {
		t.Fatalf("unknown --auth: want ErrUsage, got %v", err)
	}
}

// Without -R the target comes from the checkout's origin remote.
func TestTargetFallsBackToTheGitRemote(t *testing.T) {
	isolateCreds(t)
	t.Setenv("GH_TOKEN", canary)
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q")
	mustGit(t, dir, "remote", "add", "origin", "git@github.com:sfc-gh-eraigosa/dotfiles.git")
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	out, _, err := run("auth", "status")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "sfc-gh-eraigosa/dotfiles") {
		t.Errorf("want the remote's target, got:\n%s", out)
	}
}

func TestTargetOutsideARepoIsAUsageError(t *testing.T) {
	isolateCreds(t)
	t.Setenv("GH_TOKEN", canary)
	dir := t.TempDir() // no git repo here
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	_, _, err := run("auth", "status")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
	if !strings.Contains(err.Error(), "-R") {
		t.Errorf("the error should point at -R: %v", err)
	}
}
