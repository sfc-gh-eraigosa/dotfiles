package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp"
)

// writeJSON writes v as JSON to path, creating parent dirs as needed.
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeHome points HOME at a temp dir (and clears CLAUDE_CONFIG_DIR) and returns
// it with a fresh cwd.
func fakeHome(t *testing.T) (home, cwd string) {
	t.Helper()
	home, cwd = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return home, cwd
}

func srv() map[string]any { return map[string]any{"type": "http", "url": "https://example.invalid"} }

func byName(t *testing.T, cwd string) map[string]mcp.Scope {
	t.Helper()
	got, err := mcp.ConfiguredServers(cwd)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]mcp.Scope{}
	for _, s := range got {
		if _, dup := m[s.Name]; dup {
			t.Errorf("%s listed twice", s.Name)
		}
		m[s.Name] = s.Scope
	}
	return m
}

func names(m map[string]mcp.Scope) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// E10: a name in several scopes counts once, at its highest scope
// (local .mcp.json > project > user > plugin).
func TestConfiguredServers_Dedup(t *testing.T) {
	home, cwd := fakeHome(t)
	writeJSON(t, filepath.Join(home, ".claude.json"), map[string]any{
		"mcpServers": map[string]any{"shared": srv(), "user-only": srv()},
		"projects": map[string]any{cwd: map[string]any{
			"mcpServers": map[string]any{"shared": srv(), "proj-only": srv()},
		}},
	})
	writeJSON(t, filepath.Join(cwd, ".mcp.json"), map[string]any{"mcpServers": map[string]any{"shared": srv()}})
	got := byName(t, cwd)
	want := map[string]mcp.Scope{"shared": mcp.ScopeLocal, "user-only": mcp.ScopeUser, "proj-only": mcp.ScopeProject}
	if len(got) != len(want) {
		t.Fatalf("servers = %v, want %v", names(got), want)
	}
	for n, sc := range want {
		if got[n] != sc {
			t.Errorf("%s scope = %v, want %v", n, got[n], sc)
		}
	}
}

// .mcp.json servers: disabledMcpjsonServers is subtracted; with
// enableAllProjectMcpServers off, an enabledMcpjsonServers list is the
// allow-list. The keys may sit in ~/.claude.json's project entry or in the
// project's .claude/settings*.json.
func TestConfiguredServers_McpJSONApproval(t *testing.T) {
	home, cwd := fakeHome(t)
	writeJSON(t, filepath.Join(cwd, ".mcp.json"), map[string]any{"mcpServers": map[string]any{"a": srv(), "b": srv(), "c": srv()}})
	writeJSON(t, filepath.Join(home, ".claude.json"), map[string]any{
		"projects": map[string]any{cwd: map[string]any{"disabledMcpjsonServers": []string{"b"}}},
	})
	if got := names(byName(t, cwd)); len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("disabled b: got %v", got)
	}
	writeJSON(t, filepath.Join(cwd, ".claude", "settings.local.json"), map[string]any{"enabledMcpjsonServers": []string{"a", "b"}})
	if got := names(byName(t, cwd)); len(got) != 1 || got[0] != "a" {
		t.Fatalf("allow-list a,b minus disabled b: got %v", got)
	}
	writeJSON(t, filepath.Join(cwd, ".claude", "settings.local.json"), map[string]any{"enabledMcpjsonServers": []string{"a"}, "enableAllProjectMcpServers": true})
	if got := names(byName(t, cwd)); len(got) != 2 {
		t.Fatalf("enableAll (still minus disabled b): got %v", got)
	}
}

// Plugin servers come from the plugin cache, only for enabled plugins, under
// the CLI's own "plugin:<plugin>:<server>" names; both .mcp.json shapes
// (wrapped and flat) are read, and several cached versions count once.
func TestConfiguredServers_Plugins(t *testing.T) {
	home, cwd := fakeHome(t)
	writeJSON(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"enabledPlugins": map[string]any{"aws@market": true, "github@market": true, "off@market": false},
	})
	cache := filepath.Join(home, ".claude", "plugins", "cache", "market")
	writeJSON(t, filepath.Join(cache, "aws", "1.0.0", ".mcp.json"), map[string]any{"mcpServers": map[string]any{"aws-mcp": srv()}})
	writeJSON(t, filepath.Join(cache, "aws", "1.1.0", ".mcp.json"), map[string]any{"mcpServers": map[string]any{"aws-mcp": srv()}})
	writeJSON(t, filepath.Join(cache, "github", "abc123", ".mcp.json"), map[string]any{"github": srv()})
	writeJSON(t, filepath.Join(cache, "off", "1.0.0", ".mcp.json"), map[string]any{"mcpServers": map[string]any{"nope": srv()}})
	got := byName(t, cwd)
	if len(got) != 2 || got["plugin:aws:aws-mcp"] != mcp.ScopePlugin || got["plugin:github:github"] != mcp.ScopePlugin {
		t.Fatalf("plugins = %v", names(got))
	}
}

// CLAUDE_CONFIG_DIR replaces ~/.claude and holds .claude.json.
func TestConfiguredServers_ClaudeConfigDir(t *testing.T) {
	_, cwd := fakeHome(t)
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	writeJSON(t, filepath.Join(dir, ".claude.json"), map[string]any{"mcpServers": map[string]any{"u": srv()}})
	if got := byName(t, cwd); got["u"] != mcp.ScopeUser {
		t.Fatalf("CLAUDE_CONFIG_DIR: %v", names(got))
	}
}

// Missing files are not errors; malformed JSON is.
func TestConfiguredServers_MissingAndMalformed(t *testing.T) {
	home, cwd := fakeHome(t)
	if got, err := mcp.ConfiguredServers(cwd); err != nil || len(got) != 0 {
		t.Fatalf("nothing configured: %v %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mcp.ConfiguredServers(cwd); err == nil {
		t.Fatal("malformed ~/.claude.json must be an error")
	}
}
