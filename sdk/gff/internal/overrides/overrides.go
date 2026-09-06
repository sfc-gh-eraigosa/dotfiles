// Package overrides provides atomic read-modify-write operations on the sparse
// user override file (~/.config/gff/config.yaml).
//
// The override file is a plain YAML scalar map (bool or string/string-list
// values only) that is decoded and re-encoded without going through protojson,
// so it stays hand-editable. The P3 TUI and cmd/set use this same writer.
package overrides

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/schema"
	"gopkg.in/yaml.v3"
)

// Write sets key to v in the user override file (p.UserOverride), creating it
// if it does not exist. The file is written with mode 0600.
//
// v must be a *gffv1.Value with either BoolValue or ChoiceValue set.
// The file is written atomically: concurrent readers never see a partial write.
func Write(p paths.Paths, key string, v *gffv1.Value) error {
	// Load existing overrides (absent file == empty map).
	m, err := schema.LoadOverrides(p.UserOverride)
	if err != nil {
		return fmt.Errorf("overrides.Write: load: %w", err)
	}

	// Set the key.
	m[key] = v

	return marshalAndWrite(p.UserOverride, m)
}

// Unset removes key from the user override file. If the key is not present,
// or the file does not exist, Unset is a no-op (returns nil).
func Unset(p paths.Paths, key string) error {
	m, err := schema.LoadOverrides(p.UserOverride)
	if err != nil {
		return fmt.Errorf("overrides.Unset: load: %w", err)
	}

	if _, ok := m[key]; !ok {
		return nil // key not present — nothing to do
	}
	delete(m, key)

	return marshalAndWrite(p.UserOverride, m)
}

// WriteFileAtomic writes data to path atomically (temp file + rename) with the
// given permissions. It creates parent directories as needed. This is the
// shared primitive used by both this package and internal/registry.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Unique temp name so concurrent writers never clobber each other's temp file.
	tmp, err := os.CreateTemp(dir, ".config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	success = true
	return nil
}

// marshalAndWrite serializes m back to the plain scalar-map YAML form and
// writes it atomically with mode 0600.
//
// Encoding rules (the inverse of schema.LoadOverrides / decodeOverrideValue):
//
//   - BoolValue  → bare YAML bool
//   - ChoiceValue with 1 id → bare YAML string
//   - ChoiceValue with 0 or ≥2 ids → YAML string list
func marshalAndWrite(path string, m map[string]*gffv1.Value) error {
	// Build a plain map[string]any for yaml.Marshal.
	plain := make(map[string]any, len(m))
	for k, v := range m {
		switch kind := v.GetKind().(type) {
		case *gffv1.Value_BoolValue:
			plain[k] = kind.BoolValue
		case *gffv1.Value_ChoiceValue:
			sel := kind.ChoiceValue.GetSelected()
			if len(sel) == 1 {
				plain[k] = sel[0]
			} else {
				// Zero or multi: store as a list.
				out := make([]string, len(sel))
				copy(out, sel)
				plain[k] = out
			}
		default:
			// Defensive: skip unknown kinds (shouldn't happen after validation).
			continue
		}
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(plain))
	for k := range plain {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build an ordered yaml.Node so the output is stable.
	doc := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range keys {
		doc.Content = append(doc.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: k},
			toYAMLNode(plain[k]),
		)
	}

	// Handle the empty-map case: produce empty YAML ({}), not null.
	var data []byte
	if len(plain) == 0 {
		data = []byte("{}\n")
	} else {
		root := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{doc}}
		var err error
		data, err = yaml.Marshal(root)
		if err != nil {
			return fmt.Errorf("overrides: yaml marshal: %w", err)
		}
	}

	return WriteFileAtomic(path, data, 0o600)
}

// toYAMLNode converts a plain Go value (bool, string, []string) into a yaml.Node.
func toYAMLNode(v any) *yaml.Node {
	switch t := v.(type) {
	case bool:
		s := "false"
		if t {
			s = "true"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: s}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: t}
	case []string:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, s := range t {
			seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s})
		}
		return seq
	default:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%v", v)}
	}
}
