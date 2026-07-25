package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── fixtures ─────────────────────────────────────────────────────────────────

const writeWorldBoolYAML = `namespace: com.example.test
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

const writeWorldChoiceYAML = `namespace: com.example.test
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

const writeWorldMultiChoiceYAML = `namespace: com.example.test
sets:
  - area: install
    features:
      - path: install.shell.plugins
        description: Shell plugins
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - id: fzf
              description: fzf
              stringValue: fzf
              selected: true
            - id: starship
              description: Starship
              stringValue: starship
              selected: true
            - id: zoxide
              description: zoxide
              stringValue: zoxide
`

// writeWorld builds a temp paths world with a repo containing featYAML.
func writeWorld(t *testing.T, featYAML string) paths.Paths {
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

	if featYAML != "" {
		repoDir := filepath.Join(dir, "repo")
		gffDir := filepath.Join(repoDir, ".gff")
		require.NoError(t, os.MkdirAll(gffDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(gffDir, "features.yaml"), []byte(featYAML), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
		p.WorkDir = repoDir
	} else {
		require.NoError(t, os.MkdirAll(p.WorkDir, 0o755))
	}

	return p
}

// installResolver wires the newResolver hook for write tests.
func installResolver(t *testing.T, p paths.Paths) {
	t.Helper()
	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() { newResolver = orig; sourceFlag = origSource })
	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}
}

// ── set tests ─────────────────────────────────────────────────────────────────

// TestSetBoolFalseCreatesFile: `set install.ai.claude false` creates config.yaml
// with mode 0600 containing only that key.
func TestSetBoolFalseCreatesFile(t *testing.T) {
	p := writeWorld(t, writeWorldBoolYAML)
	installResolver(t, p)

	_, err := runCmd(t, "set", "install.ai.claude", "false")
	require.NoError(t, err)

	// File must exist.
	info, err := os.Stat(p.UserOverride)
	require.NoError(t, err, "config.yaml must exist after set")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "config.yaml must be mode 0600")

	// Read back and assert content has only the one key.
	data, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "install.ai.claude")
	assert.Contains(t, content, "false")
	// Must NOT contain other keys.
	assert.NotContains(t, content, "install.ai.tools")
}

// TestSetBoolTrue: set a bool to true.
func TestSetBoolTrue(t *testing.T) {
	p := writeWorld(t, writeWorldBoolYAML)
	installResolver(t, p)

	_, err := runCmd(t, "set", "install.ai.tools", "true")
	require.NoError(t, err)

	data, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err)
	assert.Contains(t, string(data), "true")
}

// TestSetUnknownKey: `set` with a key not in any definition layer => ErrUnknownKey,
// override file byte-identical before/after (i.e. must not exist afterward).
func TestSetUnknownKey(t *testing.T) {
	p := writeWorld(t, writeWorldBoolYAML)
	installResolver(t, p)

	// File does not exist before.
	_, statErr := os.Stat(p.UserOverride)
	require.True(t, os.IsNotExist(statErr), "override file should not exist yet")

	_, err := runCmd(t, "set", "unknown.no.exist", "true")
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolve.ErrUnknownKey), "want ErrUnknownKey, got %v", err)

	// File still must not exist.
	_, statErr = os.Stat(p.UserOverride)
	assert.True(t, os.IsNotExist(statErr), "override file must not be created on unknown-key error")
}

// TestSetChoiceUnknownOptionLeaveFileUntouched: unknown option id => error,
// file byte-identical before/after.
func TestSetChoiceUnknownOptionLeaveFileUntouched(t *testing.T) {
	p := writeWorld(t, writeWorldChoiceYAML)
	installResolver(t, p)

	// Pre-seed with a known good value.
	_, err := runCmd(t, "set", "install.pkg.manager", "apt")
	require.NoError(t, err)

	before, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err)

	// Now try to set with a bad option id.
	_, err = runCmd(t, "set", "install.pkg.manager", "nonexistent")
	require.Error(t, err)

	after, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err)
	assert.Equal(t, before, after, "override file must be byte-identical after unknown-id error")
}

// TestSetChoiceTwoIdsOnSingleMode: two ids on CHOICE_MODE_SINGLE => error,
// file untouched.
func TestSetChoiceTwoIdsOnSingleMode(t *testing.T) {
	p := writeWorld(t, writeWorldChoiceYAML)
	installResolver(t, p)

	// Pre-seed.
	_, err := runCmd(t, "set", "install.pkg.manager", "auto")
	require.NoError(t, err)

	before, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err)

	// Provide two ids for a single-select flag.
	_, err = runCmd(t, "set", "install.pkg.manager", "auto,apt")
	require.Error(t, err)

	after, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err)
	assert.Equal(t, before, after, "override file must be byte-identical after single-mode-two-ids error")
}

// TestSetChoiceMultiIds: setting two valid ids on CHOICE_MODE_MULTI is ok.
func TestSetChoiceMultiIds(t *testing.T) {
	p := writeWorld(t, writeWorldMultiChoiceYAML)
	installResolver(t, p)

	_, err := runCmd(t, "set", "install.shell.plugins", "fzf,starship")
	require.NoError(t, err)

	data, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err)
	assert.Contains(t, string(data), "fzf")
	assert.Contains(t, string(data), "starship")
}

// ── unset tests ───────────────────────────────────────────────────────────────

// TestUnsetRemovesKey: unset removes the key and keeps others.
func TestUnsetRemovesKey(t *testing.T) {
	p := writeWorld(t, writeWorldBoolYAML)
	installResolver(t, p)

	// Set two keys.
	_, err := runCmd(t, "set", "install.ai.claude", "false")
	require.NoError(t, err)
	_, err = runCmd(t, "set", "install.ai.tools", "true")
	require.NoError(t, err)

	// Unset one.
	_, err = runCmd(t, "unset", "install.ai.claude")
	require.NoError(t, err)

	data, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "install.ai.claude", "unset key must be absent")
	assert.Contains(t, string(data), "install.ai.tools", "other keys must remain")
}

// TestUnsetNoopOnMissingKey: unset on a key that's not in the file is a no-op.
func TestUnsetNoopOnMissingKey(t *testing.T) {
	p := writeWorld(t, writeWorldBoolYAML)
	installResolver(t, p)

	// File does not exist yet.
	_, err := runCmd(t, "unset", "install.ai.claude")
	require.NoError(t, err)
}

// ── round-trip ────────────────────────────────────────────────────────────────

// TestRoundTripSetGet: set then get must agree.
func TestRoundTripSetGet(t *testing.T) {
	p := writeWorld(t, writeWorldBoolYAML)
	installResolver(t, p)

	_, err := runCmd(t, "set", "install.ai.claude", "false")
	require.NoError(t, err)

	out, err := runCmd(t, "get", "install.ai.claude")
	require.NoError(t, err)
	assert.Equal(t, "false\n", out)
}

// TestRoundTripSetChoiceGet: set choice then get must agree.
func TestRoundTripSetChoiceGet(t *testing.T) {
	p := writeWorld(t, writeWorldChoiceYAML)
	installResolver(t, p)

	_, err := runCmd(t, "set", "install.pkg.manager", "apt")
	require.NoError(t, err)

	out, err := runCmd(t, "get", "install.pkg.manager")
	require.NoError(t, err)
	assert.Equal(t, "apt\n", out)
}

// TestRoundTripUnsetRestoresDefault: unset restores the default value.
func TestRoundTripUnsetRestoresDefault(t *testing.T) {
	p := writeWorld(t, writeWorldBoolYAML)
	installResolver(t, p)

	// Set to false.
	_, err := runCmd(t, "set", "install.ai.claude", "false")
	require.NoError(t, err)

	out, err := runCmd(t, "get", "install.ai.claude")
	require.NoError(t, err)
	assert.Equal(t, "false\n", out)

	// Unset — default should be restored (boolDefault: true).
	_, err = runCmd(t, "unset", "install.ai.claude")
	require.NoError(t, err)

	out, err = runCmd(t, "get", "install.ai.claude")
	require.NoError(t, err)
	assert.Equal(t, "true\n", out)
}

// TestNoWritesOutsideTempDir: all writes stay inside t.TempDir().
// This is an invariant enforced by the test design: all paths come from
// writeWorld(t, ...) which always uses t.TempDir(). If newResolver is
// correctly replaced, there is no way for a write to escape.
// We additionally assert the real $HOME config dir was NOT touched.
func TestNoWritesOutsideTempDir(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	realConfig := filepath.Join(home, ".config", "gff", "config.yaml")

	// Capture mtime before (may not exist).
	statBefore, _ := os.Stat(realConfig)

	p := writeWorld(t, writeWorldBoolYAML)
	installResolver(t, p)

	_, err = runCmd(t, "set", "install.ai.claude", "false")
	require.NoError(t, err)

	// The real config file must not have been created or modified.
	statAfter, _ := os.Stat(realConfig)
	if statBefore == nil {
		assert.Nil(t, statAfter, "real config.yaml must not be created by tests")
	} else {
		assert.Equal(t, statBefore.ModTime(), statAfter.ModTime(), "real config.yaml must not be modified by tests")
	}
}
