package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Definition is an agent persona loaded from a `.ai/agents/<name>.md` file.
// Only fields that map to tmux-mgr behavior are populated; other frontmatter
// keys (GPU-Layers, Context-Window, etc.) are intentionally ignored.
type Definition struct {
	Name       string   // canonical name (filename without .md)
	Persona    string   // free-text label for the pane / list output
	Model      string   // raw Model: field, used as a context clue for tier mapping
	Aliases    []string // alternate names that resolve to this definition
	Symbol     string   // emoji prepended to the pane title
	SourcePath string   // absolute path to the .md file
}

// agentDirs returns the lookup paths in precedence order: repo-local first,
// then $HOME-global. Missing directories are skipped silently.
func agentDirs() []string {
	var dirs []string
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, ".ai", "agents"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".ai", "agents"))
	}
	return dirs
}

// LoadDefinition looks up an agent definition by name across the configured
// lookup dirs. The match is tried as:
//
//  1. <dir>/<name>.md (exact filename)
//  2. any *.md whose Aliases: field contains <name>
//
// Returns (nil, nil) when no definition matches — callers should treat this as
// "use generalist defaults" rather than an error.
func LoadDefinition(name string) (*Definition, error) {
	if name == "" {
		return nil, nil
	}
	for _, dir := range agentDirs() {
		path := filepath.Join(dir, name+".md")
		if _, err := os.Stat(path); err == nil {
			return parseDefinitionFile(path)
		}
	}
	for _, dir := range agentDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			def, err := parseDefinitionFile(filepath.Join(dir, entry.Name()))
			if err != nil || def == nil {
				continue
			}
			if slices.Contains(def.Aliases, name) {
				return def, nil
			}
		}
	}
	return nil, nil
}

// parseDefinitionFile reads frontmatter from an agent .md file. Frontmatter is
// the leading run of `# Key: Value` lines; parsing stops at the first non-empty
// non-comment line (or comment line without a `Key: Value` shape).
func parseDefinitionFile(path string) (*Definition, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open agent definition %s: %w", path, err)
	}
	// Read-only handle: a Close failure cannot affect the bytes already read.
	defer func() { _ = f.Close() }()

	def := &Definition{
		SourcePath: path,
		Name:       strings.TrimSuffix(filepath.Base(path), ".md"),
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			// Blank line before frontmatter ends — allow it.
			if def.Persona == "" && def.Model == "" && len(def.Aliases) == 0 && def.Symbol == "" {
				continue
			}
			break
		}
		if !strings.HasPrefix(trimmed, "#") {
			break
		}

		body := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		rawKey, rawVal, ok := strings.Cut(body, ":")
		if !ok {
			break
		}
		applyFrontmatter(def, strings.TrimSpace(rawKey), strings.TrimSpace(rawVal))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan agent definition %s: %w", path, err)
	}
	return def, nil
}

func applyFrontmatter(def *Definition, key, val string) {
	switch strings.ToLower(key) {
	case "persona":
		def.Persona = val
	case "model":
		def.Model = val
	case "aliases":
		for part := range strings.SplitSeq(val, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				def.Aliases = append(def.Aliases, trimmed)
			}
		}
	case "symbol":
		def.Symbol = val
	}
}
