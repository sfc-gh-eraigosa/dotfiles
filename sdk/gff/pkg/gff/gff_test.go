package gff_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── fixtures ─────────────────────────────────────────────────────────────────

// WithRoot layer derivation (documented in gff.go):
//
//	<root>/system-snapshots/     SystemSnapshotDir
//	<root>/user-snapshots/       UserSnapshotDir
//	<root>/system-config.yaml    SystemOverride
//	<root>/user-config.yaml      UserOverride
//	<root>/sources.yaml          RegistryFile
//	<root>/work/                 WorkDir (a sub-dir so real-git probes don't escape)

const boolOnlyFeatYAML = `namespace: com.example.sdk
sets:
  - area: install
    features:
      - path: install.ai.claude
        description: Claude AI integration
        boolDefault: true
      - path: install.ai.tools
        description: AI tooling (off by default)
        boolDefault: false
`

const choiceFeatYAML = `namespace: com.example.sdk
sets:
  - area: install
    features:
      - path: install.pkg.manager
        description: Package manager (single-select)
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

const intChoiceFeatYAML = `namespace: com.example.sdk
sets:
  - area: install
    features:
      - path: install.cpu.cores
        description: CPU core count
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - id: two
              description: 2 cores
              intValue: 2
              selected: true
            - id: four
              description: 4 cores
              intValue: 4
`

const floatChoiceFeatYAML = `namespace: com.example.sdk
sets:
  - area: install
    features:
      - path: install.perf.threshold
        description: Perf threshold
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - id: low
              description: Low
              floatValue: 0.5
              selected: true
            - id: high
              description: High
              floatValue: 0.9
              selected: true
`

const boolChoiceFeatYAML = `namespace: com.example.sdk
sets:
  - area: install
    features:
      - path: install.debug.flags
        description: Debug flags
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - id: verbose
              description: Verbose mode
              boolValue: true
              selected: true
            - id: trace
              description: Trace mode
              boolValue: false
`

// buildRoot creates a root directory with the WithRoot layer layout and writes
// a repo feature file at <root>/work/.gff/features.yaml so the live layer resolves.
func buildRoot(t *testing.T, featYAML string) string {
	t.Helper()
	root := t.TempDir()

	// Create work dir as a git repo so RepoRoot recognises it.
	work := filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(work, ".git"), 0o755))

	if featYAML != "" {
		gffDir := filepath.Join(work, ".gff")
		require.NoError(t, os.MkdirAll(gffDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(gffDir, "features.yaml"), []byte(featYAML), 0o644))
	}
	return root
}

// ── Bool ─────────────────────────────────────────────────────────────────────

func TestBoolTrue(t *testing.T) {
	root := buildRoot(t, boolOnlyFeatYAML)
	got, err := gff.Bool("install.ai.claude", gff.WithRoot(root))
	require.NoError(t, err)
	assert.True(t, got)
}

func TestBoolFalse(t *testing.T) {
	root := buildRoot(t, boolOnlyFeatYAML)
	got, err := gff.Bool("install.ai.tools", gff.WithRoot(root))
	require.NoError(t, err)
	assert.False(t, got)
}

func TestBoolUnknownKey(t *testing.T) {
	root := buildRoot(t, boolOnlyFeatYAML)
	_, err := gff.Bool("unknown.no.exist", gff.WithRoot(root))
	require.Error(t, err)
}

func TestBoolWithUserOverride(t *testing.T) {
	root := buildRoot(t, boolOnlyFeatYAML)
	// Write a user override that flips claude to false.
	override := filepath.Join(root, "user-config.yaml")
	require.NoError(t, os.WriteFile(override, []byte("install.ai.claude: false\n"), 0o600))

	got, err := gff.Bool("install.ai.claude", gff.WithRoot(root))
	require.NoError(t, err)
	assert.False(t, got, "user override must flip the value to false")
}

// ── Selected ─────────────────────────────────────────────────────────────────

func TestSelectedDefault(t *testing.T) {
	root := buildRoot(t, choiceFeatYAML)
	ids, err := gff.Selected("install.pkg.manager", gff.WithRoot(root))
	require.NoError(t, err)
	assert.Equal(t, []string{"auto"}, ids)
}

func TestSelectedAfterOverride(t *testing.T) {
	root := buildRoot(t, choiceFeatYAML)
	override := filepath.Join(root, "user-config.yaml")
	require.NoError(t, os.WriteFile(override, []byte("install.pkg.manager: apt\n"), 0o600))

	ids, err := gff.Selected("install.pkg.manager", gff.WithRoot(root))
	require.NoError(t, err)
	assert.Equal(t, []string{"apt"}, ids)
}

// ── IsSelected ───────────────────────────────────────────────────────────────

func TestIsSelectedTrue(t *testing.T) {
	root := buildRoot(t, choiceFeatYAML)
	ok, err := gff.IsSelected("install.pkg.manager", "auto", gff.WithRoot(root))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIsSelectedFalse(t *testing.T) {
	root := buildRoot(t, choiceFeatYAML)
	ok, err := gff.IsSelected("install.pkg.manager", "apt", gff.WithRoot(root))
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestIsSelectedUnknownOption(t *testing.T) {
	root := buildRoot(t, choiceFeatYAML)
	_, err := gff.IsSelected("install.pkg.manager", "nonexistent", gff.WithRoot(root))
	require.Error(t, err)
}

// ── StringValues ─────────────────────────────────────────────────────────────

func TestStringValues(t *testing.T) {
	root := buildRoot(t, choiceFeatYAML)
	vals, err := gff.StringValues("install.pkg.manager", gff.WithRoot(root))
	require.NoError(t, err)
	// Default selection is "auto" with stringValue "auto".
	assert.Equal(t, []string{"auto"}, vals)
}

// ── IntValues ────────────────────────────────────────────────────────────────

func TestIntValues(t *testing.T) {
	root := buildRoot(t, intChoiceFeatYAML)
	vals, err := gff.IntValues("install.cpu.cores", gff.WithRoot(root))
	require.NoError(t, err)
	assert.Equal(t, []int64{2}, vals)
}

// ── FloatValues ──────────────────────────────────────────────────────────────

func TestFloatValues(t *testing.T) {
	root := buildRoot(t, floatChoiceFeatYAML)
	vals, err := gff.FloatValues("install.perf.threshold", gff.WithRoot(root))
	require.NoError(t, err)
	assert.InDelta(t, 0.5, vals[0], 1e-9)
	assert.InDelta(t, 0.9, vals[1], 1e-9)
}

// ── BoolValues ───────────────────────────────────────────────────────────────

func TestBoolValues(t *testing.T) {
	root := buildRoot(t, boolChoiceFeatYAML)
	vals, err := gff.BoolValues("install.debug.flags", gff.WithRoot(root))
	require.NoError(t, err)
	// Only "verbose" is default-selected (boolValue: true).
	assert.Equal(t, []bool{true}, vals)
}

// ── wrong-type accessor errors naming the actual type ────────────────────────

func TestIntValuesWrongType(t *testing.T) {
	// choiceFeatYAML uses stringValue; calling IntValues on it must error naming "string".
	root := buildRoot(t, choiceFeatYAML)
	_, err := gff.IntValues("install.pkg.manager", gff.WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string", "error must name the actual type")
}

func TestFloatValuesWrongType(t *testing.T) {
	root := buildRoot(t, choiceFeatYAML)
	_, err := gff.FloatValues("install.pkg.manager", gff.WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string")
}

func TestBoolValuesWrongType(t *testing.T) {
	root := buildRoot(t, choiceFeatYAML)
	_, err := gff.BoolValues("install.pkg.manager", gff.WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string")
}

func TestStringValuesWrongType(t *testing.T) {
	// intChoiceFeatYAML uses intValue; calling StringValues must error naming "int".
	root := buildRoot(t, intChoiceFeatYAML)
	_, err := gff.StringValues("install.cpu.cores", gff.WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "int", "error must name the actual type")
}

// ── WithSource ───────────────────────────────────────────────────────────────

func TestWithSource(t *testing.T) {
	// Build a separate source repo.
	src := t.TempDir()
	gffDir := filepath.Join(src, ".gff")
	require.NoError(t, os.MkdirAll(gffDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(gffDir, "features.yaml"), []byte(boolOnlyFeatYAML), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, ".git"), 0o755))

	// Main root has no flags of its own.
	root := buildRoot(t, "")

	got, err := gff.Bool("install.ai.claude",
		gff.WithRoot(root),
		gff.WithSource(src))
	require.NoError(t, err)
	assert.True(t, got)
}

// TestBuildResolverWithSourcePath covers buildResolver with WithSource(path).
func TestBuildResolverWithSourcePath(t *testing.T) {
	root := buildRoot(t, "")

	// Create a source repo with bool flags
	src2 := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src2, ".git"), 0o755))
	gffDir2 := filepath.Join(src2, ".gff")
	require.NoError(t, os.MkdirAll(gffDir2, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(gffDir2, "features.yaml"), []byte(boolOnlyFeatYAML), 0o644))

	// Use WithSource pointing to the second repo path
	got, err := gff.Bool("install.ai.claude",
		gff.WithRoot(root),
		gff.WithSource(src2))
	require.NoError(t, err)
	assert.True(t, got)
}

// TestBuildResolverErrorPath tests error handling in buildResolver.
func TestBuildResolverErrorPath(t *testing.T) {
	// Create a root without proper setup to trigger an error path
	root := buildRoot(t, "")

	// Try to resolve a key that doesn't exist anywhere
	_, err := gff.Bool("nonexistent.key.here",
		gff.WithRoot(root))
	require.Error(t, err, "resolving nonexistent key should error")
}
