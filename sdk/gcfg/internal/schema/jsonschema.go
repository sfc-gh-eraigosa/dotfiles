package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// JSONSchema renders a JSON Schema (draft 2020-12) for File, generated from
// the same structs the loader uses — so an editor completing gcfg.yaml and
// `gcfg lint` agree by construction. `gcfg schema` writes it to
// .github/gcfg.schema.json and CI fails if regenerating it drifts.
//
// The mapping: exported fields become properties keyed by their json tag;
// pointers and `omitempty` are optional; a `jsonschema:"required"` tag adds
// the field to `required`; `jsonschema:"enum=<key>"` pulls the value set
// from Enums; any other jsonschema text becomes the description.
// additionalProperties is false everywhere, mirroring the strict loader.
func JSONSchema() ([]byte, error) {
	defs := map[string]any{}
	root := schemaFor(reflect.TypeOf(File{}), defs)
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected root schema %T", root)
	}
	out := map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         "https://raw.githubusercontent.com/sfc-gh-eraigosa/dotfiles/main/.github/gcfg.schema.json",
		"title":       "gcfg.yaml",
		"description": "GitHub repository and organization settings as code (gcfg).",
	}
	for k, v := range obj {
		out[k] = v
	}
	if len(defs) > 0 {
		out["$defs"] = defs
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding schema: %w", err)
	}
	return append(b, '\n'), nil
}

// schemaFor builds the schema for t, registering named struct types in defs
// and referencing them, so recursive or repeated types stay small.
func schemaFor(t reflect.Type, defs map[string]any) any {
	switch t.Kind() {
	case reflect.Pointer:
		return schemaFor(t.Elem(), defs)
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schemaFor(t.Elem(), defs)}
	case reflect.Map:
		// Open-ended maps (rule parameters) accept anything.
		return map[string]any{"type": "object"}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		return structSchema(t, defs)
	default:
		return map[string]any{}
	}
}

// listElem returns the element type when t is a List[T].
func listElem(t reflect.Type) (reflect.Type, bool) {
	if t.Kind() != reflect.Struct || !strings.HasPrefix(t.Name(), "List[") {
		return nil, false
	}
	f, ok := t.FieldByName("Items")
	if !ok {
		return nil, false
	}
	return f.Type.Elem(), true
}

func structSchema(t reflect.Type, defs map[string]any) any {
	// A List[T] is either a bare array of T or the {ownership, items} form.
	if elem, ok := listElem(t); ok {
		item := schemaFor(elem, defs)
		return map[string]any{
			"description": "a list, or {ownership, items} when this family needs its own ownership",
			"oneOf": []any{
				map[string]any{"type": "array", "items": item},
				map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"ownership": map[string]any{"type": "string", "enum": toAny(Ownerships)},
						"items":     map[string]any{"type": "array", "items": item},
					},
				},
			},
		}
	}
	props := map[string]any{}
	var required []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, opts := jsonName(f)
		if name == "" || name == "-" {
			continue
		}
		s, isRequired := fieldSchema(f, defs)
		props[name] = s
		if isRequired && !opts.omitempty {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	out := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		out["required"] = toAny(required)
	}
	return out
}

type jsonOpts struct{ omitempty bool }

func jsonName(f reflect.StructField) (string, jsonOpts) {
	tag := f.Tag.Get("json")
	if tag == "" {
		return strings.ToLower(f.Name), jsonOpts{}
	}
	parts := strings.Split(tag, ",")
	o := jsonOpts{}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			o.omitempty = true
		}
	}
	return parts[0], o
}

// fieldSchema renders one field, honouring the jsonschema tag.
func fieldSchema(f reflect.StructField, defs map[string]any) (any, bool) {
	var required bool
	var enumKey, desc string
	for _, part := range strings.Split(f.Tag.Get("jsonschema"), ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case part == "required":
			required = true
		case strings.HasPrefix(part, "enum="):
			enumKey = strings.TrimPrefix(part, "enum=")
		default:
			desc = part
		}
	}
	s := schemaFor(f.Type, defs)
	m, ok := s.(map[string]any)
	if !ok {
		m = map[string]any{}
	}
	if enumKey != "" {
		if values, has := Enums[enumKey]; has {
			m["enum"] = toAny(values)
		}
	}
	if desc != "" {
		m["description"] = desc
	}
	return m, required
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
