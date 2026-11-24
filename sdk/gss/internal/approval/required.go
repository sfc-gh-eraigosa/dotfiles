package approval

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// FlagKey is the gff flag that turns the approval token on. It is off by
// default (operator, 2026-11-23: the handshake added a round trip per PR
// and did not hold up in practice); `gff set gss.approval-token.enabled true`
// turns it back on. The safety_guard hook reads the same flag.
const FlagKey = "gss.approval-token.enabled"

// EnvOverride set to on/true/1 turns the token on without asking gff (tests,
// and a one-off `GSS_REQUIRE_APPROVAL=on gss ...`). It can only turn the check
// on: turning it off is the gff flag's alone, so no command line can switch
// the check off while the operator has it on.
const EnvOverride = "GSS_REQUIRE_APPROVAL"

// Source is the gff source that declares FlagKey: the dotfiles repo, as
// `gff install` registers it. gff is asked about it by name, never about the
// repo gss happens to run in.
const Source = "com.github.sfc-gh-eraigosa.dotfiles"

// gff is asked with an environment of its own (#472): the user's home from
// the password database and a fixed PATH, nothing inherited, run from that
// home with --source (review of #473). A command that sets PATH, HOME or
// XDG_* for gss, or a broken .github/gff/features.yaml in the repo it runs
// in, cannot make gff fail, which would read as off. An operator's own
// XDG_CONFIG_HOME is ignored too: gff reads ~/.config/gff. When the account
// has no home (user.Current fails), only the system dirs are searched, and
// gff missing or failing reads as off, the default.
//
// lookPath, runGFF and userHome are swapped in tests.
var (
	lookPath = cleanLookPath
	runGFF   = func(args ...string) error {
		p, err := cleanLookPath("gff")
		if err != nil {
			return err
		}
		cmd := exec.Command(p, args...)
		cmd.Env = gffEnv()
		cmd.Dir = "/"
		if h := userHome(); h != "" {
			cmd.Dir = h
		}
		return cmd.Run()
	}
	userHome = func() string {
		if u, err := user.Current(); err == nil {
			return u.HomeDir
		}
		return ""
	}
)

// gffPATH is where gff is looked for: the install dirs under the user's
// home, then the system dirs.
func gffPATH() string {
	var dirs []string
	if h := userHome(); h != "" {
		dirs = append(dirs, filepath.Join(h, "opt", "bin"), filepath.Join(h, ".local", "bin"), filepath.Join(h, "go", "bin"))
	}
	dirs = append(dirs, "/usr/local/bin", "/usr/bin", "/bin")
	return strings.Join(dirs, string(os.PathListSeparator))
}

// gffEnv is the whole environment gff runs with.
func gffEnv() []string {
	env := []string{"PATH=" + gffPATH()}
	if h := userHome(); h != "" {
		env = append(env, "HOME="+h)
	}
	return env
}

// cleanLookPath finds an executable on gffPATH, never on $PATH.
func cleanLookPath(name string) (string, error) {
	for _, d := range filepath.SplitList(gffPATH()) {
		p := filepath.Join(d, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// TokenRequired reports whether gated verbs need an approval token: on when
// the env override says on, else `gff enabled gss.approval-token.enabled` (exit
// 0 = on). No gff, an unknown flag, or any other answer means off, the
// default.
func TokenRequired() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvOverride))) {
	case "on", "true", "1":
		return true
	}
	if _, err := lookPath("gff"); err != nil {
		return false
	}
	return runGFF("--source", Source, "enabled", FlagKey) == nil
}
