// Package registry_test exercises the source registry (sources.yaml keyed by
// reverse-DNS namespace) and the user-snapshot mechanism.
package registry_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempPaths returns a Paths with all mutable locations pointing inside dir.
func tempPaths(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	return paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   filepath.Join(dir, "user-snap"),
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      filepath.Join(dir, "config.yaml"),
		RegistryFile:      filepath.Join(dir, "sources.yaml"),
		WorkDir:           dir,
	}
}

// minimalFF builds a minimal FeatureFile with one bool flag.
func minimalFF(namespace string) *gffv1.FeatureFile {
	return &gffv1.FeatureFile{
		Namespace: namespace,
		Sets: []*gffv1.FeatureSet{
			{
				Area: "install",
				Features: []*gffv1.Feature{
					{
						Path:        "install.ai.claude",
						Description: "Claude CLI",
						Default:     &gffv1.Feature_BoolDefault{BoolDefault: true},
					},
				},
			},
		},
	}
}

// writeSourceFile creates a fake source features.yaml in a temp repo dir and
// returns (repoRoot, featurePath, rawBytes).
func writeSourceFile(t *testing.T, content string) (repoRoot, featurePath string, raw []byte) {
	t.Helper()
	repoRoot = t.TempDir()
	featureDir := filepath.Join(repoRoot, ".github", "gff")
	require.NoError(t, os.MkdirAll(featureDir, 0o755))
	featurePath = filepath.Join(featureDir, "features.yaml")
	raw = []byte(content)
	require.NoError(t, os.WriteFile(featurePath, raw, 0o644))
	return repoRoot, featurePath, raw
}

// TestInstallFresh verifies that a first install writes sources.yaml with the
// {namespace, url, commit} entry AND a snapshot at
// <UserSnapshotDir>/<namespace>.yaml that is BYTE-IDENTICAL to the source file.
func TestInstallFresh(t *testing.T) {
	p := tempPaths(t)
	reg := &registry.Registry{P: p}

	const (
		ns     = "com.example.demo"
		url    = "https://github.com/example/demo"
		commit = "abc1234"
	)
	content := "namespace: com.example.demo\nsets: []\n"
	repoRoot, _, raw := writeSourceFile(t, content)

	ff := minimalFF(ns)
	err := reg.Install(repoRoot, ns, url, commit, ff)
	require.NoError(t, err)

	// sources.yaml must exist and contain the entry.
	srcs, err := reg.Sources()
	require.NoError(t, err)
	require.Len(t, srcs, 1)
	assert.Equal(t, ns, srcs[0].Namespace)
	assert.Equal(t, url, srcs[0].Url)
	assert.Equal(t, commit, srcs[0].Commit)

	// Snapshot must exist at <UserSnapshotDir>/<namespace>.yaml and be BYTE-IDENTICAL.
	snapshotPath := filepath.Join(p.UserSnapshotDir, ns+".yaml")
	got, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	assert.Equal(t, raw, got, "snapshot bytes must be identical to source file bytes")
}

// TestInstallRefresh verifies that re-installing the same namespace + same url
// updates the commit and snapshot but does NOT create a duplicate registry entry.
func TestInstallRefresh(t *testing.T) {
	p := tempPaths(t)
	reg := &registry.Registry{P: p}

	const (
		ns     = "com.example.demo"
		url    = "https://github.com/example/demo"
		commit1 = "abc1234"
		commit2 = "def5678"
	)

	content1 := "namespace: com.example.demo\nsets: []\n"
	repoRoot, _, _ := writeSourceFile(t, content1)
	ff := minimalFF(ns)
	require.NoError(t, reg.Install(repoRoot, ns, url, commit1, ff))

	// Second install: different commit, different content.
	content2 := "namespace: com.example.demo\nsets: []\n# refreshed\n"
	featurePath := filepath.Join(repoRoot, ".github", "gff", "features.yaml")
	require.NoError(t, os.WriteFile(featurePath, []byte(content2), 0o644))

	require.NoError(t, reg.Install(repoRoot, ns, url, commit2, ff))

	// Still exactly one entry.
	srcs, err := reg.Sources()
	require.NoError(t, err)
	require.Len(t, srcs, 1, "re-install must not duplicate registry entry")
	assert.Equal(t, commit2, srcs[0].Commit, "commit must be refreshed")

	// Snapshot refreshed.
	snapshotPath := filepath.Join(p.UserSnapshotDir, ns+".yaml")
	got, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	assert.Equal(t, []byte(content2), got, "snapshot must reflect new content")
}

// TestInstallNamespaceTaken verifies that installing a DIFFERENT url for an
// already-registered namespace returns ErrNamespaceTaken whose error text
// contains the EXISTING url, and leaves the registry file unchanged.
func TestInstallNamespaceTaken(t *testing.T) {
	p := tempPaths(t)
	reg := &registry.Registry{P: p}

	const (
		ns      = "com.example.demo"
		url1    = "https://github.com/example/demo"
		url2    = "https://github.com/OTHER/demo"
		commit  = "abc1234"
	)

	content := "namespace: com.example.demo\nsets: []\n"
	repoRoot, _, _ := writeSourceFile(t, content)
	ff := minimalFF(ns)
	require.NoError(t, reg.Install(repoRoot, ns, url1, commit, ff))

	// Read the registry bytes before the conflict attempt.
	before, err := os.ReadFile(p.RegistryFile)
	require.NoError(t, err)

	// Attempt to register a different url for the same namespace.
	err = reg.Install(repoRoot, ns, url2, commit, ff)
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrNamespaceTaken),
		"error must wrap ErrNamespaceTaken, got: %v", err)
	assert.Contains(t, err.Error(), url1, "error text must contain the existing url")

	// Registry file must be unchanged.
	after, err := os.ReadFile(p.RegistryFile)
	require.NoError(t, err)
	assert.Equal(t, before, after, "registry must be byte-identical after failed install")
}

// TestSnapshotLookup verifies Snapshot() returns the path and ok=true for a
// registered namespace, and ("", false) for an unknown namespace.
func TestSnapshotLookup(t *testing.T) {
	p := tempPaths(t)
	reg := &registry.Registry{P: p}

	const (
		ns     = "com.example.demo"
		url    = "https://github.com/example/demo"
		commit = "abc1234"
	)
	content := "namespace: com.example.demo\nsets: []\n"
	repoRoot, _, _ := writeSourceFile(t, content)
	ff := minimalFF(ns)
	require.NoError(t, reg.Install(repoRoot, ns, url, commit, ff))

	// Known namespace.
	path, ok := reg.Snapshot(ns)
	assert.True(t, ok, "Snapshot must return ok=true for a registered namespace")
	expectedPath := filepath.Join(p.UserSnapshotDir, ns+".yaml")
	assert.Equal(t, expectedPath, path)

	// Unknown namespace.
	path2, ok2 := reg.Snapshot("com.unknown.ns")
	assert.False(t, ok2, "Snapshot must return ok=false for an unknown namespace")
	assert.Empty(t, path2)
}

// TestSourcesMissingRegistry verifies Sources() on a missing registry file
// returns an empty slice and nil error.
func TestSourcesMissingRegistry(t *testing.T) {
	p := tempPaths(t)
	reg := &registry.Registry{P: p}

	// RegistryFile does not exist.
	srcs, err := reg.Sources()
	require.NoError(t, err)
	assert.Empty(t, srcs, "Sources on missing registry must return empty slice")
}

// TestInstallMissingSourceFile verifies Install returns an error when the
// source features file cannot be read (repoRoot points at an empty temp dir).
func TestInstallMissingSourceFile(t *testing.T) {
	p := tempPaths(t)
	reg := &registry.Registry{P: p}

	// repoRoot exists but has no features file at either probe path.
	emptyRepo := t.TempDir()
	ff := minimalFF("com.example.nope")

	err := reg.Install(emptyRepo, "com.example.nope", "https://example.com", "abc", ff)
	require.Error(t, err, "Install must error when source file is unreadable")
}

// TestSourcesCorruptRegistry verifies Sources() returns an error when the
// registry file contains invalid YAML (not an error if YAML parses but
// protojson rejects the shape).
func TestSourcesCorruptRegistry(t *testing.T) {
	p := tempPaths(t)
	reg := &registry.Registry{P: p}

	// Write a registry file that is valid YAML but invalid as SourceRegistry
	// (a top-level string instead of a map causes protojson to error after
	// the YAML→JSON step).
	require.NoError(t, os.MkdirAll(filepath.Dir(p.RegistryFile), 0o755))
	require.NoError(t, os.WriteFile(p.RegistryFile, []byte("sources:\n  - {namespace: x\n    badfield: z\n"), 0o644))

	// Sources must propagate the parse error; it must not return nil, nil.
	_, err := reg.Sources()
	require.Error(t, err)
}

// TestSnapshotUnknownAfterRegistryError verifies Snapshot returns ("", false)
// when the registry file is unreadable (e.g., permission denied path), rather
// than panicking or returning a stale path.
func TestSnapshotUnknownAfterRegistryError(t *testing.T) {
	p := tempPaths(t)
	// Point RegistryFile at a directory so ReadFile fails.
	require.NoError(t, os.MkdirAll(p.RegistryFile, 0o755))
	reg := &registry.Registry{P: p}

	path, ok := reg.Snapshot("com.example.any")
	assert.False(t, ok)
	assert.Empty(t, path)
}

// TestInstallMultipleNamespaces verifies that multiple distinct namespaces can
// be registered independently (Sources returns all of them, Snapshot resolves each).
func TestInstallMultipleNamespaces(t *testing.T) {
	p := tempPaths(t)
	reg := &registry.Registry{P: p}

	specs := []struct {
		ns     string
		url    string
		commit string
	}{
		{"com.example.alpha", "https://github.com/example/alpha", "aaa1111"},
		{"com.example.beta", "https://github.com/example/beta", "bbb2222"},
	}

	for _, sp := range specs {
		content := "namespace: " + sp.ns + "\nsets: []\n"
		repoRoot, _, _ := writeSourceFile(t, content)
		ff := minimalFF(sp.ns)
		require.NoError(t, reg.Install(repoRoot, sp.ns, sp.url, sp.commit, ff))
	}

	srcs, err := reg.Sources()
	require.NoError(t, err)
	assert.Len(t, srcs, 2, "two distinct namespaces must produce two registry entries")

	for _, sp := range specs {
		snapPath, ok := reg.Snapshot(sp.ns)
		assert.True(t, ok)
		assert.Equal(t, filepath.Join(p.UserSnapshotDir, sp.ns+".yaml"), snapPath)
	}
}
