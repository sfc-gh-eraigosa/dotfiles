package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// List is a family whose value is a sequence — labels, rulesets, webhooks.
// It accepts two YAML shapes (plan §3.1):
//
//	labels: [ {…}, {…} ]                 # bare sequence
//	labels: {ownership: full, items: […]} # when the family needs its own ownership
//
// Both land in Items; the second also sets Ownership.
type List[T any] struct {
	Ownership Ownership `yaml:"ownership,omitempty" json:"ownership,omitempty" jsonschema:"enum=ownership"`
	Items     []T       `yaml:"items,omitempty" json:"items,omitempty"`
}

// Own is the list's effective ownership.
func (l *List[T]) Own(fallback Ownership) Ownership {
	if l == nil {
		return resolve("", fallback)
	}
	return resolve(l.Ownership, fallback)
}

// Len is the number of declared items (nil-safe).
func (l *List[T]) Len() int {
	if l == nil {
		return 0
	}
	return len(l.Items)
}

// UnmarshalYAML accepts either shape and keeps the strict unknown-key check
// for both.
func (l *List[T]) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.SequenceNode:
		var items []T
		if err := decodeStrict(n, &items); err != nil {
			return err
		}
		l.Items = items
		return nil
	case yaml.MappingNode:
		// Alias the type so this method is not called recursively.
		type wrapped struct {
			Ownership Ownership `yaml:"ownership,omitempty"`
			Items     []T       `yaml:"items,omitempty"`
		}
		var w wrapped
		if err := decodeStrict(n, &w); err != nil {
			return err
		}
		if w.Ownership != "" && w.Ownership != Declared && w.Ownership != Full {
			return fmt.Errorf("line %d: ownership %q must be one of declared, full", n.Line, w.Ownership)
		}
		l.Ownership, l.Items = w.Ownership, w.Items
		return nil
	default:
		return fmt.Errorf("line %d: expected a list or a {ownership, items} mapping", n.Line)
	}
}

// MarshalYAML writes the bare sequence unless an ownership override needs
// the mapping form — so an exported file stays as plain as it can be.
func (l List[T]) MarshalYAML() (any, error) {
	if l.Ownership == "" {
		return l.Items, nil
	}
	return struct {
		Ownership Ownership `yaml:"ownership"`
		Items     []T       `yaml:"items"`
	}{l.Ownership, l.Items}, nil
}
