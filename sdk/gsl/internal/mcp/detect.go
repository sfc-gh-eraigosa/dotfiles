package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// claudeConfig is the minimal schema we need from ~/.claude.json.
// Only mcpServers and projects are read; all other keys are ignored.
type claudeConfig struct {
	// McpServers is the global list of configured MCP servers.
	McpServers map[string]json.RawMessage `json:"mcpServers"`
	// Projects maps absolute project directory paths to their per-project
	// settings. We only read the inner mcpServers sub-object.
	Projects map[string]claudeProject `json:"projects"`
}

type claudeProject struct {
	McpServers map[string]json.RawMessage `json:"mcpServers"`
}

// mcpJSON is the minimal schema for .mcp.json (per-project, in cwd).
type mcpJSON struct {
	McpServers map[string]json.RawMessage `json:"mcpServers"`
}

// claudeConfigPath returns the path to ~/.claude.json, honoring
// CLAUDE_CONFIG_DIR if set.
func claudeConfigPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

// ConfiguredCount counts how many MCP servers are configured for the given
// working directory by reading two JSON config files — no subprocess is
// involved.
//
// Sources:
//  1. ~/.claude.json (or $CLAUDE_CONFIG_DIR/.claude.json):
//     - Global mcpServers: keys at .mcpServers
//     - Per-project mcpServers: keys at .projects[cwd].mcpServers
//  2. .mcp.json inside cwd: keys at .mcpServers
//
// Deduplication policy: counts are SUMMED across the three sources without
// deduplication.  This keeps the logic simple and avoids the need to compare
// heterogeneous server-definition objects; callers should treat the result as
// an upper bound rather than a precise unique count.  The three sources
// typically don't overlap in practice because global servers, per-project
// overrides, and .mcp.json serve different scopes.
//
// Missing files contribute 0 and are NOT treated as errors.  Only genuine
// parse errors (non-empty, malformed JSON) are propagated.
func ConfiguredCount(cwd string) (int, error) {
	total := 0

	// ── Source 1: ~/.claude.json ──────────────────────────────────────────────
	cfgPath := claudeConfigPath()
	if cfgPath != "" {
		n, err := countClaudeJSON(cfgPath, cwd)
		if err != nil {
			return 0, err
		}
		total += n
	}

	// ── Source 2: <cwd>/.mcp.json ─────────────────────────────────────────────
	mcpPath := filepath.Join(cwd, ".mcp.json")
	n, err := countMcpJSON(mcpPath)
	if err != nil {
		return 0, err
	}
	total += n

	return total, nil
}

// countClaudeJSON reads ~/.claude.json (or whatever path is given) and
// returns the sum of global mcpServers keys plus per-cwd project
// mcpServers keys.  Returns (0, nil) when the file does not exist.
func countClaudeJSON(path, cwd string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}

	var cfg claudeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return 0, err
	}

	count := len(cfg.McpServers)
	if proj, ok := cfg.Projects[cwd]; ok {
		count += len(proj.McpServers)
	}
	return count, nil
}

// countMcpJSON reads a .mcp.json file and returns the number of keys under
// mcpServers.  Returns (0, nil) when the file does not exist.
func countMcpJSON(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}

	var m mcpJSON
	if err := json.Unmarshal(data, &m); err != nil {
		return 0, err
	}
	return len(m.McpServers), nil
}
