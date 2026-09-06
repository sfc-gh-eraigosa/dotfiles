package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
)

// writeJSON writes v as JSON to path, creating parent dirs as needed.
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestConfiguredCount_AllSources(t *testing.T) {
	// Build a fake cwd directory for the project.
	cwd := t.TempDir()

	// Build a fake HOME with a .claude.json that has:
	//   - 2 global mcpServers
	//   - 3 per-project mcpServers for this cwd
	fakeHome := t.TempDir()
	claudeJSON := map[string]any{
		"mcpServers": map[string]any{
			"global-server-1": map[string]any{"type": "http"},
			"global-server-2": map[string]any{"type": "http"},
		},
		"projects": map[string]any{
			cwd: map[string]any{
				"mcpServers": map[string]any{
					"proj-server-a": map[string]any{"type": "stdio"},
					"proj-server-b": map[string]any{"type": "stdio"},
					"proj-server-c": map[string]any{"type": "stdio"},
				},
			},
			"/some/other/project": map[string]any{
				"mcpServers": map[string]any{
					"other-server": map[string]any{"type": "stdio"},
				},
			},
		},
	}
	writeJSON(t, filepath.Join(fakeHome, ".claude.json"), claudeJSON)
	t.Setenv("CLAUDE_CONFIG_DIR", fakeHome)

	// Write a .mcp.json in cwd with 1 server.
	writeJSON(t, filepath.Join(cwd, ".mcp.json"), map[string]any{
		"mcpServers": map[string]any{
			"local-server": map[string]any{"command": "my-server"},
		},
	})

	// Expected: 2 (global) + 3 (per-cwd project) + 1 (.mcp.json) = 6
	count, err := mcp.ConfiguredCount(cwd)
	if err != nil {
		t.Fatalf("ConfiguredCount: unexpected error: %v", err)
	}
	if count != 6 {
		t.Errorf("ConfiguredCount = %d; want 6", count)
	}
}

func TestConfiguredCount_MissingFiles_NoError(t *testing.T) {
	// Point HOME at an empty temp dir so ~/.claude.json doesn't exist.
	fakeHome := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", fakeHome)

	// cwd also has no .mcp.json.
	cwd := t.TempDir()

	count, err := mcp.ConfiguredCount(cwd)
	if err != nil {
		t.Fatalf("ConfiguredCount with missing files: unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("ConfiguredCount = %d; want 0 when no config files exist", count)
	}
}

func TestConfiguredCount_GlobalOnly(t *testing.T) {
	fakeHome := t.TempDir()
	writeJSON(t, filepath.Join(fakeHome, ".claude.json"), map[string]any{
		"mcpServers": map[string]any{
			"s1": map[string]any{},
			"s2": map[string]any{},
		},
	})
	t.Setenv("CLAUDE_CONFIG_DIR", fakeHome)

	// cwd with no .mcp.json and no matching project entry.
	cwd := t.TempDir()
	count, err := mcp.ConfiguredCount(cwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("ConfiguredCount = %d; want 2", count)
	}
}

func TestConfiguredCount_MissingClaudeJSON_WithMcpJSON(t *testing.T) {
	fakeHome := t.TempDir() // no .claude.json
	t.Setenv("CLAUDE_CONFIG_DIR", fakeHome)

	cwd := t.TempDir()
	writeJSON(t, filepath.Join(cwd, ".mcp.json"), map[string]any{
		"mcpServers": map[string]any{
			"only-local": map[string]any{},
		},
	})

	count, err := mcp.ConfiguredCount(cwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("ConfiguredCount = %d; want 1", count)
	}
}

func TestConfiguredCount_NoProjectMatchForCwd(t *testing.T) {
	fakeHome := t.TempDir()
	writeJSON(t, filepath.Join(fakeHome, ".claude.json"), map[string]any{
		"mcpServers": map[string]any{
			"g": map[string]any{},
		},
		"projects": map[string]any{
			"/some/other": map[string]any{
				"mcpServers": map[string]any{
					"other": map[string]any{},
				},
			},
		},
	})
	t.Setenv("CLAUDE_CONFIG_DIR", fakeHome)

	cwd := t.TempDir()
	count, err := mcp.ConfiguredCount(cwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the global mcpServers key ("g") should be counted.
	if count != 1 {
		t.Errorf("ConfiguredCount = %d; want 1", count)
	}
}

func TestConfiguredCount_EmptyMcpServers(t *testing.T) {
	fakeHome := t.TempDir()
	writeJSON(t, filepath.Join(fakeHome, ".claude.json"), map[string]any{
		"mcpServers": map[string]any{},
	})
	t.Setenv("CLAUDE_CONFIG_DIR", fakeHome)

	cwd := t.TempDir()
	count, err := mcp.ConfiguredCount(cwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("ConfiguredCount = %d; want 0", count)
	}
}
