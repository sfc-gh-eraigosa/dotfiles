package mcp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// detect.go — the configured-server scan (plan F6): which MCP servers are
// configured for a working directory, read from config files only. No
// subprocess, so it is safe on the render path.

// claudeDir is the Claude Code config directory: $CLAUDE_CONFIG_DIR, else
// ~/.claude. "" when neither resolves.
func claudeDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// claudeJSONPath is ~/.claude.json, or $CLAUDE_CONFIG_DIR/.claude.json.
func claudeJSONPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

// approval is the .mcp.json gate: settings keys that decide which project
// (.mcp.json) servers Claude Code actually loads.
type approval struct {
	EnableAll *bool    `json:"enableAllProjectMcpServers"`
	Enabled   []string `json:"enabledMcpjsonServers"`
	Disabled  []string `json:"disabledMcpjsonServers"`
}

type claudeJSON struct {
	McpServers map[string]json.RawMessage `json:"mcpServers"`
	Projects   map[string]struct {
		McpServers map[string]json.RawMessage `json:"mcpServers"`
		approval
	} `json:"projects"`
}

type settingsJSON struct {
	approval
	EnabledPlugins map[string]bool `json:"enabledPlugins"`
}

// readJSON decodes path into v. A missing or empty file is (false, nil): a
// config that does not exist contributes nothing and is not an error.
func readJSON(path string, v any) (bool, error) {
	if path == "" {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(strings.TrimSpace(string(data))) == 0) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, err
	}
	return true, nil
}

// mcpJSONServers reads a .mcp.json in either shape: {"mcpServers": {...}} or
// the flat {"<name>": {...}} some plugins ship.
func mcpJSONServers(path string) ([]string, error) {
	var raw map[string]json.RawMessage
	if ok, err := readJSON(path, &raw); !ok || err != nil {
		return nil, err
	}
	if inner, ok := raw["mcpServers"]; ok {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(inner, &m); err != nil {
			return nil, err
		}
		raw = m
	}
	var out []string
	for name, v := range raw {
		if t := strings.TrimSpace(string(v)); strings.HasPrefix(t, "{") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ConfiguredServers returns the DE-DUPLICATED set of servers configured for
// cwd, resolved across every scope (plan F6):
//   - a name in several scopes counts once, at its highest scope: Local
//     (<cwd>/.mcp.json) > Project (~/.claude.json projects[cwd]) > User
//     (~/.claude.json) > Plugin;
//   - Local servers pass the approval gate: names in disabledMcpjsonServers
//     are dropped; unless enableAllProjectMcpServers is on, a non-empty
//     enabledMcpjsonServers list is the allow-list. The keys are read from
//     ~/.claude.json's project entry, <cwd>/.claude/settings{,.local}.json
//     and the user settings.json;
//   - Plugin servers come from <claude dir>/plugins/cache/<market>/<plugin>/
//     <version>/.mcp.json for plugins enabled in the user settings.json,
//     under the CLI's own names, "plugin:<plugin>:<server>".
//
// A missing config file contributes nothing and is NOT an error; malformed
// JSON is.
func ConfiguredServers(cwd string) ([]ServerStatus, error) {
	found := map[string]Scope{}
	add := func(name string, sc Scope) {
		if cur, ok := found[name]; !ok || sc < cur { // lower Scope value = higher precedence
			found[name] = sc
		}
	}

	var cj claudeJSON
	if _, err := readJSON(claudeJSONPath(), &cj); err != nil {
		return nil, err
	}
	for name := range cj.McpServers {
		add(name, ScopeUser)
	}
	gate := approval{}
	merge := func(a approval) {
		if a.EnableAll != nil {
			gate.EnableAll = a.EnableAll
		}
		gate.Enabled = append(gate.Enabled, a.Enabled...)
		gate.Disabled = append(gate.Disabled, a.Disabled...)
	}
	if proj, ok := cj.Projects[cwd]; ok {
		for name := range proj.McpServers {
			add(name, ScopeProject)
		}
		merge(proj.approval)
	}

	var user settingsJSON
	if _, err := readJSON(filepath.Join(claudeDir(), "settings.json"), &user); err != nil {
		return nil, err
	}
	merge(user.approval)
	for _, f := range []string{"settings.json", "settings.local.json"} {
		var ps settingsJSON
		if _, err := readJSON(filepath.Join(cwd, ".claude", f), &ps); err != nil {
			return nil, err
		}
		merge(ps.approval)
	}

	local, err := mcpJSONServers(filepath.Join(cwd, ".mcp.json"))
	if err != nil {
		return nil, err
	}
	disabled, enabled := set(gate.Disabled), set(gate.Enabled)
	allowAll := (gate.EnableAll != nil && *gate.EnableAll) || len(enabled) == 0
	for _, name := range local {
		if disabled[name] || (!allowAll && !enabled[name]) {
			continue
		}
		add(name, ScopeLocal)
	}

	for key, on := range user.EnabledPlugins {
		plugin, market, ok := strings.Cut(key, "@")
		if !on || !ok {
			continue
		}
		versions, _ := filepath.Glob(filepath.Join(claudeDir(), "plugins", "cache", market, plugin, "*", ".mcp.json"))
		for _, path := range versions {
			servers, err := mcpJSONServers(path)
			if err != nil {
				continue // one broken plugin cache must not hide the rest
			}
			for _, name := range servers {
				add("plugin:"+plugin+":"+name, ScopePlugin)
			}
		}
	}

	out := make([]ServerStatus, 0, len(found))
	for name, sc := range found {
		out = append(out, ServerStatus{Name: name, Scope: sc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func set(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
