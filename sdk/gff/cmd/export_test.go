package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	envparse "github.com/hashicorp/go-envparse"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ── golden world fixtures ─────────────────────────────────────────────────────

// goldenWorldYAML is the normative feature file for the golden export test.
// 3 flags: bool defaulting true (claude), choice defaulting auto (pkg.manager),
// bool defaulting true (wispr-flow — overridden false in goldenWorld).
const goldenWorldYAML = `namespace: com.example.golden
sets:
  - area: install
    features:
      - path: install.ai.claude
        description: Claude AI integration $(rm -rf /tmp/pwned)
        boolDefault: true
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
      - path: install.windows.wispr-flow
        description: Wispr Flow MSI
        boolDefault: true
`

// goldenWorld creates the export test world: repo with goldenWorldYAML and a
// user override that sets install.windows.wispr-flow=false.
func goldenWorld(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	gffDir := filepath.Join(repoDir, ".gff")
	require.NoError(t, os.MkdirAll(gffDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gffDir, "features.yaml"), []byte(goldenWorldYAML), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))

	cfgDir := filepath.Join(dir, "cfg")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	userOverride := filepath.Join(cfgDir, "config.yaml")
	// Override wispr-flow to false so the golden shows false.
	require.NoError(t, os.WriteFile(userOverride, []byte("install.windows.wispr-flow: false\n"), 0o600))

	return paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   filepath.Join(dir, "user-snap"),
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      userOverride,
		RegistryFile:      filepath.Join(dir, "sources.yaml"),
		WorkDir:           repoDir,
	}
}

// installExportResolver wires newResolver for export tests and resets all
// export flag vars on cleanup so package-level state doesn't leak between tests.
func installExportResolver(t *testing.T, p paths.Paths) {
	t.Helper()
	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() {
		newResolver = orig
		sourceFlag = origSource
		resetExportFlags()
	})
	resetExportFlags()
	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}
}

// ── mangling ──────────────────────────────────────────────────────────────────

func TestEnvMangling(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"install.windows.wispr-flow", "GFF_INSTALL_WINDOWS_WISPR_FLOW"},
		{"install.ai.claude", "GFF_INSTALL_AI_CLAUDE"},
		{"install.pkg.manager", "GFF_INSTALL_PKG_MANAGER"},
		{"install.shell.default-zsh", "GFF_INSTALL_SHELL_DEFAULT_ZSH"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, mangleKey(tc.key), "mangleKey(%q)", tc.key)
	}
}

// ── golden test ───────────────────────────────────────────────────────────────

func TestExportShellGolden(t *testing.T) {
	p := goldenWorld(t)
	installExportResolver(t, p)

	out, err := runCmd(t, "export", "--format", "shell")
	require.NoError(t, err)

	golden, err := os.ReadFile("testdata/export.golden")
	require.NoError(t, err)

	assert.Equal(t, string(golden), out,
		"export --format shell output must match golden file exactly")
}

// ── injection safety ──────────────────────────────────────────────────────────

// TestExportNoInjection asserts that the shell and dotenv formats (which are
// eval'd directly) never emit the description field — only KEY=value lines
// containing bool literals or lint-constrained kebab option ids. json/yaml
// carry the full ResolvedJSON (including description) per §3.3 contract, so
// they are not checked for description suppression here.
func TestExportNoInjection(t *testing.T) {
	p := goldenWorld(t)
	installExportResolver(t, p)

	for _, format := range []string{"shell", "dotenv"} {
		resetExportFlags()
		out, err := runCmd(t, "export", "--format", format)
		require.NoError(t, err, "format %s", format)
		assert.NotContains(t, out, "$(rm -rf", "format %s must not contain injection payload in values", format)
		assert.NotContains(t, out, "/tmp/pwned", "format %s must not contain injection payload in values", format)
		// Every line must be KEY=value where value is bool or kebab ids only.
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			require.Len(t, parts, 2, "each line must be KEY=value, got %q", line)
			value := parts[1]
			// Values must be only bool literals or comma-joined kebab ids.
			assert.Regexp(t, `^(true|false|[a-z0-9]+(-[a-z0-9]+)*(,[a-z0-9]+(-[a-z0-9]+)*)*)$`,
				value, "value %q in format %s must be bool or kebab ids only", value, format)
		}
	}
}

// ── --shell alias ─────────────────────────────────────────────────────────────

func TestExportShellAlias(t *testing.T) {
	p := goldenWorld(t)
	installExportResolver(t, p)

	outAlias, err := runCmd(t, "export", "--shell")
	require.NoError(t, err)

	outFormat, err := runCmd(t, "export", "--format", "shell")
	require.NoError(t, err)

	assert.Equal(t, outFormat, outAlias, "--shell alias must produce identical output to --format shell")
}

// ── dotenv format ─────────────────────────────────────────────────────────────

func TestExportDotenvFile(t *testing.T) {
	p := goldenWorld(t)
	installExportResolver(t, p)

	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")

	_, err := runCmd(t, "export", "--format", "dotenv", "-o", envFile)
	require.NoError(t, err)

	data, err := os.ReadFile(envFile)
	require.NoError(t, err)

	// Must parse with go-envparse (hashicorp).
	parsed, err := envparse.Parse(strings.NewReader(string(data)))
	require.NoError(t, err, "dotenv output must round-trip through go-envparse")

	assert.Equal(t, "true", parsed["GFF_INSTALL_AI_CLAUDE"])
	assert.Equal(t, "auto", parsed["GFF_INSTALL_PKG_MANAGER"])
	assert.Equal(t, "false", parsed["GFF_INSTALL_WINDOWS_WISPR_FLOW"])

	// Same content as shell — compare trimmed lines (dotenv file has no trailing newline difference).
	resetExportFlags()
	outShell, err := runCmd(t, "export", "--format", "shell")
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(outShell), strings.TrimSpace(string(data)),
		"dotenv and shell must produce same lines")
}

// ── json format ───────────────────────────────────────────────────────────────

func TestExportJSON(t *testing.T) {
	p := goldenWorld(t)
	installExportResolver(t, p)

	out, err := runCmd(t, "export", "--format", "json")
	require.NoError(t, err)

	var results []resolve.ResolvedJSON
	require.NoError(t, json.Unmarshal([]byte(out), &results), "json output must unmarshal into []ResolvedJSON")
	assert.Len(t, results, 3)

	// Find the choice key and verify it carries typed values + selected ids.
	found := false
	for _, r := range results {
		if r.Path == "install.pkg.manager" {
			found = true
			assert.Equal(t, "choice", r.Type)
			// Value must reference the selected id (auto).
			assert.NotEmpty(t, r.Value)
			// Feature must carry the options.
			assert.NotEmpty(t, r.Feature)
		}
	}
	assert.True(t, found, "install.pkg.manager must be in json output")
}

// ── yaml format ───────────────────────────────────────────────────────────────

func TestExportYAMLRoundTrip(t *testing.T) {
	p := goldenWorld(t)
	installExportResolver(t, p)

	outJSON, err := runCmd(t, "export", "--format", "json")
	require.NoError(t, err)

	outYAML, err := runCmd(t, "export", "--format", "yaml")
	require.NoError(t, err)

	var fromJSON []resolve.ResolvedJSON
	require.NoError(t, json.Unmarshal([]byte(outJSON), &fromJSON))

	// YAML decodes into the same structure.
	var fromYAML []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(outYAML), &fromYAML))

	// Both must have the same number of entries.
	assert.Equal(t, len(fromJSON), len(fromYAML), "json and yaml must have same entry count")

	// Re-encode both as JSON for equality comparison.
	jBytes, err := json.Marshal(fromJSON)
	require.NoError(t, err)
	yBytes, err := json.Marshal(fromYAML)
	require.NoError(t, err)

	assert.JSONEq(t, string(jBytes), string(yBytes),
		"yaml and json outputs must be semantically equal when marshaled back to JSON")
}

// ── -o unwritable target ──────────────────────────────────────────────────────

func TestExportUnwritableTarget(t *testing.T) {
	p := goldenWorld(t)
	installExportResolver(t, p)

	// Create a read-only directory.
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	require.NoError(t, os.MkdirAll(roDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) }) // restore for cleanup

	target := filepath.Join(roDir, "output.env")
	_, err := runCmd(t, "export", "--format", "dotenv", "-o", target)
	require.Error(t, err, "export to an unwritable target must return an error")

	// Target must not exist (no partial write).
	_, statErr := os.Stat(target)
	assert.True(t, os.IsNotExist(statErr), "target must not exist after failure")
}

// ── empty feature set ─────────────────────────────────────────────────────────

func TestExportEmptyWorld(t *testing.T) {
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
	installExportResolver(t, p)

	// All formats must succeed with empty/valid output.
	for _, format := range []string{"shell", "dotenv", "json", "yaml"} {
		out, err := runCmd(t, "export", "--format", format)
		require.NoError(t, err, "format %s on empty world must not error", format)
		if format == "json" {
			assert.Equal(t, "[]\n", out, "json empty world must be []")
		}
	}
}

// ── install verb ──────────────────────────────────────────────────────────────

func TestInstallInRepo(t *testing.T) {
	dir := t.TempDir()

	// Create a real git repo with a flag file.
	repoDir := filepath.Join(dir, "myrepo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))

	// git init.
	gitDir := filepath.Join(repoDir, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "objects"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644))

	// Write a valid flag file.
	gffDir := filepath.Join(repoDir, ".gff")
	require.NoError(t, os.MkdirAll(gffDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gffDir, "features.yaml"), []byte(goldenWorldYAML), 0o644))

	userSnapDir := filepath.Join(dir, "user-snap")
	p := paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   userSnapDir,
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      filepath.Join(dir, "user-config.yaml"),
		RegistryFile:      filepath.Join(dir, "sources.yaml"),
		WorkDir:           repoDir,
	}

	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() { newResolver = orig; sourceFlag = origSource })
	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}

	_, err := runCmd(t, "install")
	require.NoError(t, err)

	// Verify registry has the source.
	reg := &registry.Registry{P: p}
	sources, err := reg.Sources()
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "com.example.golden", sources[0].GetNamespace())

	// Snapshot file must exist.
	snapFile := filepath.Join(userSnapDir, "com.example.golden.yaml")
	_, err = os.Stat(snapFile)
	assert.NoError(t, err, "snapshot file must exist after install")
}

func TestInstallOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   filepath.Join(dir, "user-snap"),
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      filepath.Join(dir, "user-config.yaml"),
		RegistryFile:      filepath.Join(dir, "sources.yaml"),
		// WorkDir has no .git => not a repo.
		WorkDir: dir,
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

	_, err := runCmd(t, "install")
	require.Error(t, err, "install outside a git repo must return an error")
}
