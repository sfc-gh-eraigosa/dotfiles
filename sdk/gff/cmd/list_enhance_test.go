package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListTableHasHeaderAndAlignment(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "list")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "header + 2 rows")
	for _, col := range []string{"PATH", "TYPE", "VALUE", "LAYER", "DESCRIPTION"} {
		assert.Contains(t, lines[0], col, "header row")
	}
	// Columns line up: TYPE starts at the same offset in header and rows.
	idx := strings.Index(lines[0], "TYPE")
	require.Greater(t, idx, 0)
	assert.Equal(t, "bool", lines[1][idx:idx+4], "TYPE column aligned under header")
}

func TestListJSONPrettyPrinted(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML)
	withResolver(t, p)

	out, err := runCmd(t, "list", "--json")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "[\n  {"), "indented jq-style output, got %q", out[:min(len(out), 20)])

	var rows []resolve.ResolvedJSON
	require.NoError(t, json.Unmarshal([]byte(out), &rows), "pretty output still unmarshals")
	assert.Len(t, rows, 2)
}

func TestListFilterGlobAndPrefix(t *testing.T) {
	p, _ := worldPaths(t, boolFeatYAML) // install.ai.claude + install.ai.tools
	withResolver(t, p)

	// Glob on the full key.
	out, err := runCmd(t, "list", "install.ai.*")
	require.NoError(t, err)
	assert.Contains(t, out, "install.ai.claude")
	assert.Contains(t, out, "install.ai.tools")

	// Glob narrowing to one key.
	out, err = runCmd(t, "list", "*.claude")
	require.NoError(t, err)
	assert.Contains(t, out, "install.ai.claude")
	assert.NotContains(t, out, "install.ai.tools")

	// Bare prefix (no glob chars) matches by segment prefix.
	out, err = runCmd(t, "list", "install.ai")
	require.NoError(t, err)
	assert.Contains(t, out, "install.ai.claude")

	// Filter applies to --json too; no match => empty array, exit 0.
	out, err = runCmd(t, "list", "--json", "no.such.prefix")
	require.NoError(t, err)
	var rows []resolve.ResolvedJSON
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	assert.Empty(t, rows)
}
