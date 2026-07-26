package cmd

// `gff sources` — owner-approved §3.4 extension (PR #187 review): the CLI
// twin of the TUI help overlay's SOURCES story. Lists registry entries AND
// the discovered current-repo origin, marking which is which.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sourcesRepoYAML = `
namespace: com.example.currepo
sets:
  - area: demo
    features:
      - {path: demo.ui.dash, description: Dashboard, boolDefault: true}
`

func writeRegistry(t *testing.T, registryFile, snapDir, ns, url, commit string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(registryFile), 0o755))
	body := "sources:\n  - namespace: " + ns + "\n    url: " + url + "\n    commit: " + commit + "\n"
	require.NoError(t, os.WriteFile(registryFile, []byte(body), 0o644))
	require.NoError(t, os.MkdirAll(snapDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(snapDir, ns+".yaml"), []byte("namespace: "+ns+"\nsets: []\n"), 0o644))
}

func TestSourcesListsRegisteredAndDiscovered(t *testing.T) {
	p, _ := worldPaths(t, sourcesRepoYAML)
	writeRegistry(t, p.RegistryFile, p.UserSnapshotDir, "com.example.registered", "https://example.com/reg.git", "abc123")
	withResolver(t, p)

	out, err := runCmd(t, "sources")
	require.NoError(t, err)
	assert.Contains(t, out, "com.example.registered")
	assert.Contains(t, out, "https://example.com/reg.git")
	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "registered")
	assert.Contains(t, out, "com.example.currepo", "the CWD repo's origin is listed too")
	assert.Contains(t, out, "discovered", "the unregistered current-repo origin is labeled")
	assert.Contains(t, out, "current repo", "the current repo is marked")
}

func TestSourcesJSON(t *testing.T) {
	p, _ := worldPaths(t, sourcesRepoYAML)
	writeRegistry(t, p.RegistryFile, p.UserSnapshotDir, "com.example.registered", "https://example.com/reg.git", "abc123")
	withResolver(t, p)

	out, err := runCmd(t, "sources", "--json")
	require.NoError(t, err)
	var rows []struct {
		Namespace   string `json:"namespace"`
		URL         string `json:"url"`
		Commit      string `json:"commit"`
		Registered  bool   `json:"registered"`
		CurrentRepo bool   `json:"currentRepo"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	require.Len(t, rows, 2)
	// Current-repo origin sorts first.
	assert.Equal(t, "com.example.currepo", rows[0].Namespace)
	assert.True(t, rows[0].CurrentRepo)
	assert.False(t, rows[0].Registered)
	assert.Equal(t, "com.example.registered", rows[1].Namespace)
	assert.True(t, rows[1].Registered)
	assert.False(t, rows[1].CurrentRepo)
}

func TestSourcesEmptyWorld(t *testing.T) {
	p, _ := worldPaths(t, "") // no repo, no registry
	withResolver(t, p)

	out, err := runCmd(t, "sources")
	require.NoError(t, err, "empty world is not an error (exit 0)")
	assert.Contains(t, out, "no sources")

	jout, err := runCmd(t, "sources", "--json")
	require.NoError(t, err)
	assert.Equal(t, "[]\n", jout, "empty JSON list, never null")
}

func TestSourcesRegisteredCurrentRepo(t *testing.T) {
	// The CWD repo's namespace is ALSO registered: one row, both markers.
	p, _ := worldPaths(t, sourcesRepoYAML)
	writeRegistry(t, p.RegistryFile, p.UserSnapshotDir, "com.example.currepo", "https://example.com/cur.git", "def456")
	withResolver(t, p)

	out, err := runCmd(t, "sources", "--json")
	require.NoError(t, err)
	var rows []struct {
		Namespace   string `json:"namespace"`
		Registered  bool   `json:"registered"`
		CurrentRepo bool   `json:"currentRepo"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	require.Len(t, rows, 1, "no duplicate row when registered == current repo")
	assert.True(t, rows[0].Registered)
	assert.True(t, rows[0].CurrentRepo)
}

func TestRenderSourcesTableWidthBound(t *testing.T) {
	rows := []sourceRow{
		{Namespace: "com.example.registered", URL: "https://example.com/a-rather-long-repository-url.git", Commit: "abc123", Registered: true},
		{Namespace: "com.example.currepo", CurrentRepo: true},
	}
	out := renderSourcesTable(rows, 80)
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("line %d exceeds 80 cols: %d", i, w)
		}
	}
	assert.Contains(t, out, "com.example.currepo")
	// The STATUS cell wraps within the column at narrow widths, so assert the
	// words independently rather than as one contiguous string.
	assert.Contains(t, out, "current")
	assert.Contains(t, out, "registered")
}
