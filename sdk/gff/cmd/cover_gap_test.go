package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultResolverConstructs covers the production resolver wiring. It only
// constructs (no resolution), so a hermetic HOME keeps it off real user state.
func TestDefaultResolverConstructs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r, err := defaultResolver()
	require.NoError(t, err)
	require.NotNil(t, r)
	require.NotNil(t, r.S, "registry SourceLookup must be wired")
}

// TestExecuteVersion covers cmd.Execute (the main entrypoint path).
func TestExecuteVersion(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"version"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs([]string{})
	})
	require.NoError(t, Execute())
	assert.Contains(t, out.String(), "gff")
}

// ── runLint branch coverage ───────────────────────────────────────────────────

func TestLintExplicitCleanFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "features.yaml")
	require.NoError(t, os.WriteFile(path, []byte(boolFeatYAML), 0o644))

	out, err := runCmd(t, "lint", path)
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(out))
}

func TestLintExplicitUnreadablePath(t *testing.T) {
	_, err := runCmd(t, "lint", filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lint:")
}

func TestLintNoArgDiscoversRepoFile(t *testing.T) {
	p, repoDir := worldPaths(t, boolFeatYAML)
	require.NotEmpty(t, repoDir)
	withResolver(t, p)

	out, err := runCmd(t, "lint")
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(out))
}

func TestLintNoArgOutsideRepo(t *testing.T) {
	p, _ := worldPaths(t, "") // no repo — WorkDir is a plain temp dir
	withResolver(t, p)

	_, err := runCmd(t, "lint")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not inside a git repository")
}

// ── runInstall branch coverage ────────────────────────────────────────────────

func TestInstallHappyNoRemote(t *testing.T) {
	p, repoDir := worldPaths(t, boolFeatYAML)
	require.NotEmpty(t, repoDir)
	withResolver(t, p)

	out, err := runCmd(t, "install")
	require.NoError(t, err)
	assert.Contains(t, out, "installed com.example.test")

	reg := &registry.Registry{P: p}
	sources, err := reg.Sources()
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "com.example.test", sources[0].GetNamespace())
}

func TestInstallOutsideRepoNoWorld(t *testing.T) {
	p, _ := worldPaths(t, "")
	withResolver(t, p)

	_, err := runCmd(t, "install")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not inside a git repository")
}

func TestInstallMissingFlagFile(t *testing.T) {
	p, repoDir := worldPaths(t, boolFeatYAML)
	require.NoError(t, os.Remove(filepath.Join(repoDir, ".gff", "features.yaml")))
	withResolver(t, p)

	_, err := runCmd(t, "install")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load feature file")
}

func TestInstallLintFindings(t *testing.T) {
	p, repoDir := worldPaths(t, badFeatYAML)
	require.NotEmpty(t, repoDir)
	withResolver(t, p)

	_, err := runCmd(t, "install")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lint finding")
}

// urlRunner fakes git: answers remote.origin.url and rev-parse, errors otherwise.
type urlRunner struct{ url, commit string }

func (u urlRunner) Output(_ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "remote.origin.url"):
		return u.url + "\n", nil
	case strings.Contains(joined, "rev-parse"):
		return u.commit + "\n", nil
	}
	return "", errors.New("no git")
}

func TestInstallOriginNamespaceMismatchWarns(t *testing.T) {
	p, repoDir := worldPaths(t, boolFeatYAML) // declares com.example.test
	require.NotEmpty(t, repoDir)

	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() { newResolver = orig; sourceFlag = origSource })
	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, urlRunner{url: "https://github.com/other/repo", commit: "abc1234"}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}

	out, err := runCmd(t, "install")
	require.NoError(t, err, "mismatch is a WARNING, not an error")
	assert.Contains(t, out, "WARNING")
	assert.Contains(t, out, "com.github.other.repo")
}

// ── runUnset / runList remaining branches ────────────────────────────────────

func TestUnsetAbsentKeyIsNoOp(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	// unset of an absent key is a documented no-op (plan §3.4 requires
	// ErrUnknownKey only for set); it must not error.
	_, err := runCmd(t, "unset", "install.zz.zz")
	require.NoError(t, err)
}

func TestListChoiceRow(t *testing.T) {
	p, _ := worldPaths(t, choiceFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "install.pkg.manager")
	assert.Contains(t, out, "choice")
	assert.Contains(t, out, "auto")
}

// TestVerbsResolverError covers every verb's newResolver-error arm in one table.
func TestVerbsResolverError(t *testing.T) {
	orig := newResolver
	t.Cleanup(func() { newResolver = orig })
	newResolver = func() (*resolve.Resolver, error) {
		return nil, errors.New("boom: no resolver")
	}

	for _, args := range [][]string{
		{"get", "a.b.c"}, {"enabled", "a.b.c"}, {"selected", "a.b.c", "x"},
		{"list"}, {"lint"}, {"set", "a.b.c", "true"}, {"unset", "a.b.c"},
		{"export"}, {"install"},
	} {
		_, err := runCmd(t, args...)
		assert.Error(t, err, "verb %v must propagate resolver construction error", args)
	}
}

func TestDefaultResolverNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := defaultResolver()
	require.Error(t, err)
}

func TestExportJSONAndYAMLToFile(t *testing.T) {
	p := goldenWorld(t)
	installExportResolver(t, p)
	dir := t.TempDir()

	for _, format := range []string{"json", "yaml"} {
		resetExportFlags()
		target := filepath.Join(dir, "out."+format)
		_, err := runCmd(t, "export", "--format", format, "-o", target)
		require.NoError(t, err, "format %s -o", format)
		data, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	}
}
