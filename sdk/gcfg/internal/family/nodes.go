package family

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// The helpers here read and build the yaml.Node fragments families exchange
// with the engine. Working on nodes (not decoded structs) is what lets an
// export keep comments and a diff know whether a key was declared at all.

// Field returns the value node for key in a mapping node, and whether the
// key was declared. A nil or non-mapping node simply has no fields.
func Field(n *yaml.Node, key string) (*yaml.Node, bool) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1], true
		}
	}
	return nil, false
}

// Bool decodes a declared boolean.
func Bool(n *yaml.Node, key string) (bool, bool) {
	v, ok := Field(n, key)
	if !ok {
		return false, false
	}
	var b bool
	if err := v.Decode(&b); err != nil {
		return false, false
	}
	return b, true
}

// Str decodes a declared string.
func Str(n *yaml.Node, key string) (string, bool) {
	v, ok := Field(n, key)
	if !ok {
		return "", false
	}
	var s string
	if err := v.Decode(&s); err != nil {
		return "", false
	}
	return s, true
}

// Strings decodes a declared string list.
func Strings(n *yaml.Node, key string) ([]string, bool) {
	v, ok := Field(n, key)
	if !ok {
		return nil, false
	}
	var ss []string
	if err := v.Decode(&ss); err != nil {
		return nil, false
	}
	return ss, true
}

// Map builds a mapping node from ordered key/value pairs, skipping any pair
// whose value node is nil so an absent setting stays absent.
func Map(pairs ...any) *yaml.Node {
	if len(pairs)%2 != 0 {
		panic("family.Map: odd number of arguments")
	}
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i < len(pairs); i += 2 {
		key, _ := pairs[i].(string)
		val, _ := pairs[i+1].(*yaml.Node)
		if val == nil {
			continue
		}
		n.Content = append(n.Content, Scalar(key), val)
	}
	if len(n.Content) == 0 {
		return nil
	}
	return n
}

// Scalar builds a scalar node for a string, bool, or int.
func Scalar(v any) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode}
	switch t := v.(type) {
	case string:
		n.Tag, n.Value = "!!str", t
	case bool:
		n.Tag, n.Value = "!!bool", fmt.Sprintf("%t", t)
	case int:
		n.Tag, n.Value = "!!int", fmt.Sprintf("%d", t)
	default:
		n.Tag, n.Value = "!!str", fmt.Sprintf("%v", t)
	}
	return n
}

// Seq builds a sequence node of strings; an empty list still renders, so a
// declared-empty stays distinguishable from absent.
func Seq(items []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, s := range items {
		n.Content = append(n.Content, Scalar(s))
	}
	return n
}

// SameStrings compares two string lists as sets in order-insensitive form —
// GitHub returns topics and events in its own order.
func SameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}
