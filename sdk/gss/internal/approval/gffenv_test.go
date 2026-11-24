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
		"[ \"$*\" = \"enabled " + FlagKey + "\" ] || exit 2\n" +
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

	if p, err := lookPath("gff"); err != nil || p != filepath.Join(bin, "gff") {
		t.Fatalf("lookPath(gff) = %q, %v; want the gff under the user's home", p, err)
	}
	if !TokenRequired() {
		t.Fatal("TokenRequired = false: the command's PATH/HOME/XDG reached gff")
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
