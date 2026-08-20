package cmd

import (
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathsDefault(t *testing.T) {
	// Test that paths.Default() works correctly
	p, err := paths.Default()
	require.NoError(t, err)

	// UserOverride should be under .config/gff
	assert.Contains(t, p.UserOverride, ".config/gff")

	// RegistryFile should be under .config/gff
	assert.Contains(t, p.RegistryFile, ".config/gff")

	// UserSnapshotDir should be under XDG_DATA_HOME or .local/share
	assert.True(t,
		contains(p.UserSnapshotDir, "gff/snapshots") ||
			contains(p.UserSnapshotDir, ".local/share/gff/snapshots"),
		"UserSnapshotDir should contain gff/snapshots",
	)
}

func TestPathsDefaultWithXDGDataHome(t *testing.T) {
	tmpdir := t.TempDir()
	// t.Setenv restores the previous value when the test finishes.
	t.Setenv("XDG_DATA_HOME", tmpdir)

	p, err := paths.Default()
	require.NoError(t, err)

	// UserSnapshotDir should use the XDG_DATA_HOME value
	expected := filepath.Join(tmpdir, "gff", "snapshots")
	assert.Equal(t, expected, p.UserSnapshotDir)
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
