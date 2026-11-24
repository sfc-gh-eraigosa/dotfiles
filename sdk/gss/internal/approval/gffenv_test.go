package approval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A command that points gss at another PATH, HOME or XDG dir must not change
// gff's answer (#472): gss asks the gff under the real home, with only HOME
// and a fixed PATH set.
func TestGFFIgnoresTheCommandEnv(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "opt", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// The real gff says on only when asked the exact question with a clean env.
	real := "#!/bin/sh\n" +
		"[ \"$HOME\" = \"" + home + "\" ] || exit 3\n" +
		"[ -z \"$XDG_DATA_HOME$XDG_CONFIG_HOME\" ] || exit 4\n" +
		"[ \"$(pwd -P)\" = / ] || exit 5\n" +
		"[ \"$*\" = \"--source " + Source + " enabled " + FlagKey + "\" ] || exit 2\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gff"), []byte(real), 0o755); err != nil {
		t.Fatal(err)
	}
	// A decoy gff on the command's PATH says off.
	decoy := t.TempDir()
	if err := os.WriteFile(filepath.Join(decoy, "gff"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer func(h func() string) { userHome = h }(userHome)
	userHome = func() string { return home }
	t.Setenv(EnvOverride, "")
	t.Setenv("PATH", decoy+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "/nonexistent")
	t.Setenv("XDG_CONFIG_HOME", "/nonexistent")
	// gss runs from a repo whose own flag file is broken; gff must not read it
	// (reviews of #473): it runs in / and asks the dotfiles source.
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".github", "gff"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".github", "gff", "features.yaml"), []byte("garbage: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)

	if p, err := lookPath("gff"); err != nil || p != filepath.Join(bin, "gff") {
		t.Fatalf("lookPath(gff) = %q, %v; want the gff under the user's home", p, err)
	}
	if !TokenRequired() {
		t.Fatal("TokenRequired = false: the command's PATH/HOME/XDG reached gff")
	}
}

// The real userHome comes from the password database, not $HOME (review of
// #473): os.UserHomeDir would let `HOME=/x gss ...` pick gff's config.
func TestUserHomeIgnoresHOME(t *testing.T) {
	fake := t.TempDir()
	t.Setenv("HOME", fake)
	if h := userHome(); h == "" || h == fake {
		t.Fatalf("userHome() = %q with HOME=%q; want the account's home", h, fake)
	}
}

func TestGFFEnvIsOnlyHomeAndPath(t *testing.T) {
	defer func(h func() string) { userHome = h }(userHome)
	userHome = func() string { return "/home/u" }
	env := gffEnv()
	if len(env) != 2 || env[1] != "HOME=/home/u" || !strings.HasPrefix(env[0], "PATH=/home/u/opt/bin") {
		t.Fatalf("gffEnv = %q", env)
	}
	userHome = func() string { return "" }
	if env := gffEnv(); len(env) != 1 || env[0] != "PATH=/usr/local/bin:/usr/bin:/bin" {
		t.Fatalf("gffEnv with no home = %q", env)
	}
}
