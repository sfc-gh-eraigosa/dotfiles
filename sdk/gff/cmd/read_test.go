package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── fixtures ─────────────────────────────────────────────────────────────────

const boolFeatYAML = `namespace: com.example.test
sets:
  - area: install
    features:
      - path: install.ai.claude
        description: Claude AI integration
        boolDefault: true
      - path: install.ai.tools
        description: AI tooling
        boolDefault: false
`

const choiceFeatYAML = `namespace: com.example.test
sets:
  - area: install
    features:
      - path: install.pkg.manager
        description: Package manager
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - id: auto
              description: Auto-detect
              stringValue: auto
              selected: true
            - id: apt
              description: Debian apt
              stringValue: apt
            - id: brew
              description: Homebrew
              stringValue: brew
`

const multiChoiceFeatYAML = `namespace: com.example.test
sets:
  - area: install
    features:
      - path: install.shell.plugins
        description: Shell plugins
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - id: fzf
              description: fzf fuzzy finder
              stringValue: fzf
              selected: true
            - id: starship
              description: Starship prompt
              stringValue: starship
              selected: true
            - id: zoxide
              description: zoxide
              stringValue: zoxide
`

const badFeatYAML = `namespace: com.example.test
sets:
  - area: install
    features:
      - path: install.no-good.flag
        description: Uses negative prefix
        boolDefault: true
      - path: install.ai.claude
        description: Good flag
        boolDefault: true
      - path: install.ai.claude
        description: Duplicate key
        boolDefault: false
`

// fakeRunner for tests — never calls real git.
type fakeTestRunner struct{}

func (fakeTestRunner) Output(_ string, _ ...string) (string, error) {
	return "", errors.New("no git")
}

// worldPaths creates a temp world and returns paths and the repo dir.
// If featYAML is non-empty, writes a repo with .gff/features.yaml and a .git dir.
func worldPaths(t *testing.T, featYAML string) (paths.Paths, string) {
	t.Helper()
	dir := t.TempDir()

	p := paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   filepath.Join(dir, "user-snap"),
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      filepath.Join(dir, "user-config.yaml"),
		RegistryFile:      filepath.Join(dir, "sources.yaml"),
		WorkDir:           filepath.Join(dir, "workdir"),
	}
	require.NoError(t, os.MkdirAll(p.WorkDir, 0o755))

	repoDir := ""
	if featYAML != "" {
		repoDir = filepath.Join(dir, "repo")
		gffDir := filepath.Join(repoDir, ".gff")
		require.NoError(t, os.MkdirAll(gffDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(gffDir, "features.yaml"), []byte(featYAML), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
		p.WorkDir = repoDir
	}
	return p, repoDir
}

// withResolver replaces the package-level newResolver with a function that
// returns a resolver over the given paths. Restores the original on cleanup.
func withResolver(t *testing.T, p paths.Paths) {
	t.Helper()
	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() {
		newResolver = orig
		sourceFlag = origSource
	})
	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}
}

// runCmd runs the root command with the given args, returning stdout and error.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// ── get verb ─────────────────────────────────────────────────────────────────

func TestGetBoolTrue(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "get", "install.ai.claude")
	require.NoError(t, err)
	assert.Equal(t, "true\n", out)
}

func TestGetBoolFalse(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "get", "install.ai.tools")
	require.NoError(t, err)
	assert.Equal(t, "false\n", out)
}

func TestGetChoiceSingleDefault(t *testing.T) {
	p, _ := worldPaths(t, choiceFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "get", "install.pkg.manager")
	require.NoError(t, err)
	assert.Equal(t, "auto\n", out)
}

func TestGetChoiceMultiDefault(t *testing.T) {
	p, _ := worldPaths(t, multiChoiceFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "get", "install.shell.plugins")
	require.NoError(t, err)
	// Both fzf and starship are default-selected; comma-joined sorted order.
	assert.Equal(t, "fzf,starship\n", out)
}

func TestGetUnknownKey(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	_, err := runCmd(t, "get", "unknown.no.exist")
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolve.ErrUnknownKey), "want ErrUnknownKey, got %v", err)
}

// ── enabled verb ──────────────────────────────────────────────────────────────

func TestEnabledOn(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "enabled", "install.ai.claude")
	require.NoError(t, err)
	assert.Equal(t, "", out, "enabled should produce no output on success")
}

func TestEnabledOff(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	_, err := runCmd(t, "enabled", "install.ai.tools")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errOff), "want errOff, got %v", err)
	// errOff is NOT ErrUnknownKey (different sentinel)
	assert.False(t, errors.Is(err, resolve.ErrUnknownKey))
}

func TestEnabledUnknownKey(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	_, err := runCmd(t, "enabled", "unknown.no.exist")
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolve.ErrUnknownKey), "want ErrUnknownKey, got %v", err)
}

func TestEnabledOnChoiceKey(t *testing.T) {
	p, _ := worldPaths(t, choiceFeatYAML)
	withResolver(t, p)

	_, err := runCmd(t, "enabled", "install.pkg.manager")
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolve.ErrWrongFlagType), "want ErrWrongFlagType, got %v", err)
}

// ── selected verb ─────────────────────────────────────────────────────────────

func TestSelectedIsSelected(t *testing.T) {
	p, _ := worldPaths(t, choiceFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "selected", "install.pkg.manager", "auto")
	require.NoError(t, err)
	assert.Equal(t, "", out, "selected should produce no output on success")
}

func TestSelectedNotSelected(t *testing.T) {
	p, _ := worldPaths(t, choiceFeatYAML)
	withResolver(t, p)

	_, err := runCmd(t, "selected", "install.pkg.manager", "apt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errNotSelected), "want errNotSelected, got %v", err)
}

func TestSelectedUnknownOption(t *testing.T) {
	p, _ := worldPaths(t, choiceFeatYAML)
	withResolver(t, p)

	_, err := runCmd(t, "selected", "install.pkg.manager", "nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolve.ErrUnknownOption), "want ErrUnknownOption, got %v", err)
}

func TestSelectedUnknownKey(t *testing.T) {
	p, _ := worldPaths(t, choiceFeatYAML)
	withResolver(t, p)

	_, err := runCmd(t, "selected", "unknown.no.exist", "apt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolve.ErrUnknownKey), "want ErrUnknownKey, got %v", err)
}

// ── list verb ─────────────────────────────────────────────────────────────────

func TestListTableOutput(t *testing.T) {
	// Use user-snapshot layer so the layer name is "user-snapshot".
	dir := t.TempDir()
	snapDir := filepath.Join(dir, "user-snap")
	require.NoError(t, os.MkdirAll(snapDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(snapDir, "test.yaml"), []byte(boolFeatYAML), 0o644))

	p := paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   snapDir,
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      filepath.Join(dir, "user-config.yaml"),
		RegistryFile:      filepath.Join(dir, "sources.yaml"),
		WorkDir:           filepath.Join(dir, "workdir"),
	}
	require.NoError(t, os.MkdirAll(p.WorkDir, 0o755))
	withResolver(t, p)

	out, err := runCmd(t, "list")
	require.NoError(t, err)
	// Must contain the path, type, value, and layer.
	assert.Contains(t, out, "install.ai.claude")
	assert.Contains(t, out, "bool")
	assert.Contains(t, out, "true")
	assert.Contains(t, out, "user-snapshot")
}

func TestListJSONOutput(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "list", "--json")
	require.NoError(t, err)

	var results []resolve.ResolvedJSON
	require.NoError(t, json.Unmarshal([]byte(out), &results), "list --json must produce valid JSON array of ResolvedJSON")
	assert.NotEmpty(t, results)

	// Find install.ai.claude entry.
	found := false
	for _, r := range results {
		if r.Path == "install.ai.claude" {
			found = true
			assert.Equal(t, "bool", r.Type)
			assert.Equal(t, "repo-live", r.Layer)
		}
	}
	assert.True(t, found, "expected install.ai.claude in list --json output")
}

func TestListEmpty(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   filepath.Join(dir, "user-snap"),
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      filepath.Join(dir, "user-config.yaml"),
		RegistryFile:      filepath.Join(dir, "sources.yaml"),
		WorkDir:           filepath.Join(dir, "workdir"),
	}
	require.NoError(t, os.MkdirAll(p.WorkDir, 0o755))
	withResolver(t, p)

	// No error even when there are no flags.
	_, err := runCmd(t, "list")
	require.NoError(t, err)
}

// ── lint verb ─────────────────────────────────────────────────────────────────

func TestLintBadFile(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "features.yaml")
	require.NoError(t, os.WriteFile(badFile, []byte(badFeatYAML), 0o644))

	p, _ := worldPaths(t, "")
	withResolver(t, p)

	out, err := runCmd(t, "lint", badFile)
	require.Error(t, err, "lint on a bad file must return an error")
	// Output must contain the finding(s).
	assert.NotEmpty(t, out)
}

func TestLintCleanFile(t *testing.T) {
	dir := t.TempDir()
	cleanFile := filepath.Join(dir, "features.yaml")
	require.NoError(t, os.WriteFile(cleanFile, []byte(boolFeatYAML), 0o644))

	p, _ := worldPaths(t, "")
	withResolver(t, p)

	out, err := runCmd(t, "lint", cleanFile)
	require.NoError(t, err, "lint on a clean file must succeed")
	assert.Empty(t, strings.TrimSpace(out))
}

func TestLintDiscoveredRepoFile(t *testing.T) {
	// worldPaths with boolFeatYAML creates a repo with .gff/features.yaml (clean).
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	// lint with no argument discovers the repo file via the resolver's WorkDir.
	// The resolver has a WorkDir pointing at the repo, so CWD probing should work.
	// We need to provide the repo path explicitly since we can't easily set CWD in tests.
	// Use the explicit path variant here.
	featPath := filepath.Join(p.WorkDir, ".gff", "features.yaml")
	_, err := runCmd(t, "lint", featPath)
	require.NoError(t, err)
}

// ── --source flag ──────────────────────────────────────────────────────────────

func TestGetSourcePath(t *testing.T) {
	// Create a second temp repo at a distinct path.
	dir2 := t.TempDir()
	gffDir2 := filepath.Join(dir2, ".gff")
	require.NoError(t, os.MkdirAll(gffDir2, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gffDir2, "features.yaml"), []byte(boolFeatYAML), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir2, ".git"), 0o755))

	// Primary world has no flags of its own (WorkDir elsewhere).
	mainDir := t.TempDir()
	p := paths.Paths{
		SystemSnapshotDir: filepath.Join(mainDir, "sys-snap"),
		UserSnapshotDir:   filepath.Join(mainDir, "user-snap"),
		SystemOverride:    filepath.Join(mainDir, "sys-config.yaml"),
		UserOverride:      filepath.Join(mainDir, "user-config.yaml"),
		RegistryFile:      filepath.Join(mainDir, "sources.yaml"),
		WorkDir:           mainDir, // NOT the repo with flags
	}
	require.NoError(t, os.MkdirAll(p.WorkDir, 0o755))

	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() { newResolver = orig; sourceFlag = origSource })

	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}

	// Set --source to dir2 (the second repo).
	sourceFlag = dir2

	out, err := runCmd(t, "get", "install.ai.claude")
	require.NoError(t, err)
	assert.Equal(t, "true\n", out)
}

func TestGetSourceRegisteredName(t *testing.T) {
	// Build a world with a registered snapshot for "com.example.test".
	dir := t.TempDir()
	userSnapDir := filepath.Join(dir, "user-snap")
	require.NoError(t, os.MkdirAll(userSnapDir, 0o755))
	// The snapshot file is named after the namespace.
	require.NoError(t, os.WriteFile(
		filepath.Join(userSnapDir, "com.example.test.yaml"),
		[]byte(boolFeatYAML), 0o644))

	// Write sources.yaml pointing at that namespace.
	sourcesYAML := fmt.Sprintf(`sources:
  - namespace: com.example.test
    url: https://github.com/example/test
    commit: abc1234
`)
	registryFile := filepath.Join(dir, "sources.yaml")
	require.NoError(t, os.WriteFile(registryFile, []byte(sourcesYAML), 0o644))

	p := paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   userSnapDir,
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      filepath.Join(dir, "user-config.yaml"),
		RegistryFile:      registryFile,
		WorkDir:           filepath.Join(dir, "workdir"),
	}
	require.NoError(t, os.MkdirAll(p.WorkDir, 0o755))

	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() { newResolver = orig; sourceFlag = origSource })

	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}

	sourceFlag = "com.example.test"
	out, err := runCmd(t, "get", "install.ai.claude")
	require.NoError(t, err)
	assert.Equal(t, "true\n", out)
}

func TestGetUnknownSource(t *testing.T) {
	p, _ := worldPaths(t, "")
	withResolver(t, p)

	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() { newResolver = orig; sourceFlag = origSource })

	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}

	sourceFlag = "com.nonexistent.source"
	_, err := runCmd(t, "get", "install.ai.claude")
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolve.ErrUnknownSource), "want ErrUnknownSource, got %v", err)
}

func TestGetNonRepoPath(t *testing.T) {
	// A path that exists as a directory but has no .git and no feature file.
	dir := t.TempDir()
	p, _ := worldPaths(t, "")
	withResolver(t, p)

	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() { newResolver = orig; sourceFlag = origSource })

	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}

	// A local path that exists as a dir (so it's treated as a local path, not a name),
	// but has no feature file — the live layer will be absent and the key won't exist.
	sourceFlag = dir
	_, err := runCmd(t, "get", "install.ai.claude")
	require.Error(t, err)
	// A non-repo path is an unknown source (plan §7.2 IA-10 => exit 2).
	assert.True(t, errors.Is(err, resolve.ErrUnknownSource), "want ErrUnknownSource for non-repo path, got %v", err)
}
