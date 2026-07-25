package paths_test

import (
	"os"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	p, err := paths.Default()
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(p.SystemSnapshotDir, "/opt/conf/gff"),
		"SystemSnapshotDir should end with /opt/conf/gff, got %q", p.SystemSnapshotDir)

	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg != "" {
		assert.True(t, strings.HasPrefix(p.UserSnapshotDir, xdg),
			"UserSnapshotDir should start with XDG_DATA_HOME=%q when set, got %q", xdg, p.UserSnapshotDir)
	} else {
		assert.True(t, strings.HasPrefix(p.UserSnapshotDir, home+"/.local/share"),
			"UserSnapshotDir should start with $HOME/.local/share when XDG_DATA_HOME is unset, got %q", p.UserSnapshotDir)
	}
	assert.True(t, strings.HasSuffix(p.UserSnapshotDir, "/gff/snapshots"),
		"UserSnapshotDir should end with /gff/snapshots, got %q", p.UserSnapshotDir)

	assert.Equal(t, "/var/opt/conf/gff/config.yaml", p.SystemOverride)

	assert.Equal(t, home+"/.config/gff/config.yaml", p.UserOverride)

	assert.Equal(t, home+"/.config/gff/sources.yaml", p.RegistryFile)

	assert.NotEmpty(t, p.WorkDir, "WorkDir should be set")
}

func TestDefaultXDGDataHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	p, err := paths.Default()
	require.NoError(t, err)

	assert.Equal(t, dir+"/gff/snapshots", p.UserSnapshotDir,
		"UserSnapshotDir should use XDG_DATA_HOME when set")
}

func TestDefaultNoHomeErrors(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := paths.Default(); err == nil {
		t.Fatal("want error when HOME is empty")
	}
}
