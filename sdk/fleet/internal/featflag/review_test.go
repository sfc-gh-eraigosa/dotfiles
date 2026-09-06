package featflag

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"
)

// Findings from the leaf C code review (PR #270).

// fixtureRepo makes an isolated git repo whose feature file pins the two
// fleet.update flags to NON-default values, plus an isolated gff root, so a
// test can tell "read the repo I named" from "read whatever cwd is in".
func fixtureRepo(t *testing.T, features string) (repoDir string, opts []gff.Option) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	repoDir = t.TempDir()
	if out, err := exec.Command("git", "-C", repoDir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init unavailable: %v %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".gff"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".gff", "features.yaml"), []byte(features), 0o644); err != nil {
		t.Fatal(err)
	}
	return repoDir, []gff.Option{gff.WithRoot(root)}
}

const pinnedFeatures = `namespace: com.example.fixture
sets:
  - area: fleet
    features:
      - path: fleet.update.enabled
        description: pinned off by the fixture
        boolDefault: false
      - path: fleet.update.config
        description: pinned to repo by the fixture
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: home, description: home, stringValue: "~/.config/fleet/fleet.yaml"}
            - {id: repo, description: repo, stringValue: "opt/etc/fleet/fleet.yaml", selected: true}
`

// The adapter must read the checkout named by --repo, not the process cwd
// and not a stale registered snapshot: this test runs with cwd inside the
// real dotfiles checkout (enabled=true, home) and must still see the fixture.
func TestGFFScopesToTheRepoPathNotTheCwd(t *testing.T) {
	repoDir, opts := fixtureRepo(t, pinnedFeatures)
	g := &GFF{Repo: repoDir, Opts: opts}
	on, err := g.Bool(KeyEnabled)
	if err != nil || on {
		t.Fatalf("Bool = %v, %v; want false from the fixture repo", on, err)
	}
	sel, err := g.Strings(KeyConfig)
	if err != nil || len(sel) != 1 || sel[0] != "repo" {
		t.Fatalf("Strings = %v, %v; want the selected option ID [repo]", sel, err)
	}
}

// Strings returns option IDs (what `gff set` stores), not stringValue
// payloads — the fixture's payloads are deliberately different from the ids.
func TestGFFStringsReturnsOptionIDsNotPayloads(t *testing.T) {
	repoDir, opts := fixtureRepo(t, pinnedFeatures)
	g := &GFF{Repo: repoDir, Opts: opts}
	sel, err := g.Strings(KeyConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sel {
		if strings.Contains(s, "/") {
			t.Fatalf("Strings leaked a stringValue payload: %v", sel)
		}
	}
}

// A repo that does not define the key yields ErrUnknownKey — not a silent
// fallback to some other checkout's definition.
func TestGFFUnknownKeyInTheNamedRepoIsReported(t *testing.T) {
	repoDir, opts := fixtureRepo(t, "namespace: com.example.empty\nsets: []\n")
	g := &GFF{Repo: repoDir, Opts: opts}
	if _, err := g.Bool(KeyEnabled); !errors.Is(err, gff.ErrUnknownKey) {
		t.Fatalf("want ErrUnknownKey, got %v", err)
	}
}

// A --repo that is not a git repository is an unknown source; the adapter then
// falls back to the unscoped lookup exactly once (cwd discovery), so a
// workstation without a clone still resolves something sensible.
func TestGFFFallsBackToUnscopedWhenRepoIsNotAGitRepo(t *testing.T) {
	var calls []string
	g := &GFF{Repo: t.TempDir()}
	g.boolFn = func(key string, opts ...gff.Option) (bool, error) {
		calls = append(calls, key)
		if len(calls) == 1 {
			return false, gff.ErrUnknownSource
		}
		return true, nil
	}
	on, err := g.Bool(KeyEnabled)
	if err != nil || !on || len(calls) != 2 {
		t.Fatalf("on=%v err=%v calls=%d; want the unscoped retry", on, err, len(calls))
	}
}

// A typed-nil *GFF stored in the Source interface must be as safe as an
// untyped nil: no code path may panic on the "gff unavailable" case.
func TestResolveTypedNilGFFIsFailOpen(t *testing.T) {
	var g *GFF
	s := Resolve(g, "/h", "/r")
	if !s.Enabled || s.ConfigPath != "" || s.Note == "" {
		t.Fatalf("typed nil: %+v", s)
	}
}

// More than one selection is a misconfigured choice; take nothing, say so.
func TestResolveMultipleSelectionsIsFailOpenWithANote(t *testing.T) {
	s := Resolve(Static{Bools: map[string]bool{KeyEnabled: true}, Strs: map[string][]string{KeyConfig: {"home", "repo"}}}, "/h", "/r")
	if s.ConfigPath != "" || !strings.Contains(s.Note, "selection") {
		t.Fatalf("multiple selections: %+v", s)
	}
}

// "repo" without a usable repoDir must not produce a cwd-relative path.
func TestResolveRepoLocationNeedsAnAbsoluteRepoDir(t *testing.T) {
	src := Static{Bools: map[string]bool{KeyEnabled: true}, Strs: map[string][]string{KeyConfig: {"repo"}}}
	for _, bad := range []string{"", "git/dotfiles"} {
		s := Resolve(src, "/h", bad)
		if s.ConfigPath != "" || !strings.Contains(s.Note, "repo") {
			t.Errorf("repoDir %q: %+v", bad, s)
		}
	}
	if s := Resolve(src, "/h", "/abs/dotfiles"); s.ConfigPath != "/abs/dotfiles/opt/etc/fleet/fleet.yaml" {
		t.Fatalf("absolute repoDir: %+v", s)
	}
}
