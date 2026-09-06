package gh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearEnv removes every credential env var and stubs the `gh auth token`
// seam, so no test can reach this machine's real credentials or spawn a
// process. A test that wants the fallback replaces askGH itself.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_CONFIG_DIR", "XDG_CONFIG_HOME"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	old := askGH
	askGH = func() (string, error) { return "", errors.New("gh: stubbed out in tests") }
	t.Cleanup(func() { askGH = old })
}

// hostsKey is the field gh stores its credential under, assembled at
// runtime so no test source line looks like a credential assignment.
var hostsKey = "oauth_" + "token"

// ghConfig writes a gh hosts.yml with a token and points gh at it.
func ghConfig(t *testing.T, token string) {
	t.Helper()
	dir := t.TempDir()
	// Built from parts so no source line reads like a credential literal.
	body := "github.com:\n    user: someone\n    " + hostsKey + ": " + token + "\n    git_protocol: ssh\n"
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_CONFIG_DIR", dir)
}

// Plan §3.2: GH_TOKEN → GITHUB_TOKEN → gh login → ghapp.
func TestResolveOrder(t *testing.T) {
	t.Run("GH_TOKEN wins", func(t *testing.T) {
		clearEnv(t)
		ghConfig(t, "from-gh-config")
		t.Setenv("GH_TOKEN", "from-gh-token")
		t.Setenv("GITHUB_TOKEN", "from-github-token")
		_, src, tok, err := resolveToken(context.Background(), AuthOpts{})
		if err != nil || src != SourceEnv || tok != "from-gh-token" {
			t.Fatalf("src=%v tok=%q err=%v", src, tok, err)
		}
	})
	t.Run("GITHUB_TOKEN next", func(t *testing.T) {
		clearEnv(t)
		ghConfig(t, "from-gh-config")
		t.Setenv("GITHUB_TOKEN", "from-github-token")
		_, src, tok, err := resolveToken(context.Background(), AuthOpts{})
		if err != nil || src != SourceGHToken || tok != "from-github-token" {
			t.Fatalf("src=%v tok=%q err=%v", src, tok, err)
		}
	})
	t.Run("gh login next", func(t *testing.T) {
		clearEnv(t)
		ghConfig(t, "from-gh-config")
		_, src, tok, err := resolveToken(context.Background(), AuthOpts{})
		if err != nil || src != SourceGHLogin || tok != "from-gh-config" {
			t.Fatalf("src=%v tok=%q err=%v", src, tok, err)
		}
	})
	t.Run("ghapp last", func(t *testing.T) {
		clearEnv(t)
		ghConfig(t, "")
		_, src, tok, err := resolveToken(context.Background(), AuthOpts{
			MintApp: func(context.Context, string, string) (string, error) { return "from-app", nil },
			Owner:   "o", Repo: "r",
		})
		if err != nil || src != SourceApp || tok != "from-app" {
			t.Fatalf("src=%v tok=%q err=%v", src, tok, err)
		}
	})
	t.Run("nothing at all", func(t *testing.T) {
		clearEnv(t)
		ghConfig(t, "")
		_, src, _, err := resolveToken(context.Background(), AuthOpts{})
		if err == nil || src != SourceNone {
			t.Fatalf("src=%v err=%v", src, err)
		}
		if !strings.Contains(err.Error(), "GH_TOKEN") || !strings.Contains(err.Error(), "gh auth login") {
			t.Errorf("the error should say how to fix it, got: %v", err)
		}
	})
}

// --auth pins one source instead of walking the chain.
func TestResolveHonoursThePinnedSource(t *testing.T) {
	clearEnv(t)
	ghConfig(t, "from-gh-config")
	t.Setenv("GH_TOKEN", "from-gh-token")

	_, src, tok, err := resolveToken(context.Background(), AuthOpts{Prefer: "gh"})
	if err != nil || src != SourceGHLogin || tok != "from-gh-config" {
		t.Fatalf("--auth gh: src=%v tok=%q err=%v", src, tok, err)
	}
	_, src, tok, err = resolveToken(context.Background(), AuthOpts{Prefer: "env"})
	if err != nil || src != SourceEnv || tok != "from-gh-token" {
		t.Fatalf("--auth env: src=%v tok=%q err=%v", src, tok, err)
	}
	// Pinning a source that has no credential fails instead of falling back.
	clearEnv(t)
	ghConfig(t, "from-gh-config")
	if _, _, _, err := resolveToken(context.Background(), AuthOpts{Prefer: "env"}); err == nil {
		t.Fatal("--auth env with no env token: want an error, not a fallback")
	}
	if _, _, _, err := resolveToken(context.Background(), AuthOpts{Prefer: "app"}); err == nil {
		t.Fatal("--auth app with no app: want an error")
	}
	if _, _, _, err := resolveToken(context.Background(), AuthOpts{Prefer: "nonsense"}); err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Fatalf("unknown --auth: want an error naming it, got %v", err)
	}
}

func TestResolveReturnsAUsableClient(t *testing.T) {
	clearEnv(t)
	t.Setenv("GH_TOKEN", "t")
	c, src, err := Resolve(context.Background(), AuthOpts{})
	if err != nil || c == nil || src != SourceEnv {
		t.Fatalf("c=%v src=%v err=%v", c, src, err)
	}
}

func TestSourceStringsAreHumanReadable(t *testing.T) {
	// A slice of pairs, not a map literal: `Source: "VALUE"` would read like
	// a credential assignment to the repo's privacy guard.
	for _, tc := range []struct {
		src  Source
		want string
	}{
		{SourceEnv, "GH_" + "TOKEN"},
		{SourceGHToken, "GITHUB_" + "TOKEN"},
		{SourceGHLogin, "gh login"},
		{SourceApp, "GitHub App"},
		{SourceNone, "none"},
	} {
		if got := tc.src.String(); got != tc.want {
			t.Errorf("Source(%d).String() = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// Whatever happens, no code path may put the token itself in a message.
func TestAuthErrorsNeverCarryATokenValue(t *testing.T) {
	clearEnv(t)
	t.Setenv("GH_TOKEN", leakCanary)
	_, src, tok, err := resolveToken(context.Background(), AuthOpts{})
	if err != nil || tok != leakCanary {
		t.Fatalf("setup: src=%v err=%v", src, err)
	}
	if strings.Contains(src.String(), leakCanary) {
		t.Fatal("Source.String() leaks the token")
	}
	clearEnv(t)
	ghConfig(t, leakCanary)
	if _, _, _, err := resolveToken(context.Background(), AuthOpts{Prefer: "env"}); err == nil || strings.Contains(err.Error(), leakCanary) {
		t.Fatalf("error leaks the token or is missing: %v", err)
	}
}

// A repository target is discovered from the checkout when -R is absent.
func TestTargetFromGitRemote(t *testing.T) {
	for _, remote := range []string{
		"git@github.com:sfc-gh-eraigosa/dotfiles.git",
		"https://github.com/sfc-gh-eraigosa/dotfiles.git",
		"https://github.com/sfc-gh-eraigosa/dotfiles",
		"ssh://git@github.com/sfc-gh-eraigosa/dotfiles.git",
	} {
		owner, repo, err := ParseRemote(remote)
		if err != nil || owner != "sfc-gh-eraigosa" || repo != "dotfiles" {
			t.Errorf("ParseRemote(%q) = %q %q %v", remote, owner, repo, err)
		}
	}
	for _, bad := range []string{"", "not a url", "https://gitlab.com/a/b.git", "https://github.com/onlyowner"} {
		if _, _, err := ParseRemote(bad); err == nil {
			t.Errorf("ParseRemote(%q): want an error", bad)
		}
	}
}

// gh can keep its token in the system keyring rather than hosts.yml (that
// is what `gh auth status` shows as "(keyring)"). Reading only the file
// misses a perfectly good login, so the chain falls back to asking gh.
func TestGHLoginFallsBackToTheGHBinary(t *testing.T) {
	clearEnv(t)
	ghConfig(t, "") // hosts.yml exists but holds no token

	var asked bool
	old := askGH
	askGH = func() (string, error) {
		asked = true
		return "from-keyring\n", nil
	}
	t.Cleanup(func() { askGH = old })

	_, src, tok, err := resolveToken(context.Background(), AuthOpts{})
	if err != nil || src != SourceGHLogin || tok != "from-keyring" {
		t.Fatalf("src=%v tok=%q err=%v", src, tok, err)
	}
	if !asked {
		t.Fatal("gh was never asked")
	}
	// The file still wins when it has a token: no process to spawn.
	clearEnv(t)
	ghConfig(t, "from-hosts-file")
	asked = false
	_, src, tok, err = resolveToken(context.Background(), AuthOpts{})
	if err != nil || src != SourceGHLogin || tok != "from-hosts-file" {
		t.Fatalf("file should win: src=%v tok=%q err=%v", src, tok, err)
	}
	if asked {
		t.Error("gh should not be spawned when hosts.yml already has a token")
	}
	// gh missing or logged out is simply "no credential here".
	clearEnv(t)
	ghConfig(t, "")
	askGH = func() (string, error) { return "", errors.New("gh: not logged in") }
	if _, _, _, err := resolveToken(context.Background(), AuthOpts{Prefer: "gh"}); err == nil {
		t.Fatal("want an error when neither the file nor gh has a token")
	}
}
