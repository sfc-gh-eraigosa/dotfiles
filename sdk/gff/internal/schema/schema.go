// Package schema loads and validates gff feature files and override files.
package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// LoadFeatureFile reads a .yaml, .yml, or .json features file and decodes it
// into a FeatureFile proto. Unknown fields cause an error (DiscardUnknown: false).
func LoadFeatureFile(path string) (*gffv1.FeatureFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("LoadFeatureFile %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var jsonBytes []byte

	switch ext {
	case ".json":
		jsonBytes = data
	case ".yaml", ".yml":
		var raw any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("LoadFeatureFile %s: yaml parse: %w", path, err)
		}
		normalized := normalizeMapKeys(raw)
		jsonBytes, err = json.Marshal(normalized)
		if err != nil {
			return nil, fmt.Errorf("LoadFeatureFile %s: json marshal: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("LoadFeatureFile %s: unsupported extension %q", path, ext)
	}

	ff := &gffv1.FeatureFile{}
	opts := protojson.UnmarshalOptions{DiscardUnknown: false}
	if err := opts.Unmarshal(jsonBytes, ff); err != nil {
		return nil, fmt.Errorf("LoadFeatureFile %s: proto unmarshal: %w", path, err)
	}
	return ff, nil
}

// normalizeMapKeys recursively converts map[any]any keys to string, which
// yaml.v3 can produce in certain edge cases, ensuring json.Marshal succeeds.
func normalizeMapKeys(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeMapKeys(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprintf("%v", k)] = normalizeMapKeys(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeMapKeys(val)
		}
		return out
	default:
		return v
	}
}

// LoadOverrides reads a sparse scalar override file (YAML key→value map) and
// decodes it into a map of flag key → *gffv1.Value. If the file does not exist,
// an empty map and nil error are returned. Type mapping:
//
//   - bool   → BoolValue
//   - string → ChoiceSelection{[s]}
//   - []string → ChoiceSelection{s…}
//   - anything else (int, float, nested map, mixed-type list) → error
func LoadOverrides(path string) (map[string]*gffv1.Value, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]*gffv1.Value{}, nil
		}
		return nil, fmt.Errorf("LoadOverrides %s: %w", path, err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("LoadOverrides %s: yaml parse: %w", path, err)
	}

	result := make(map[string]*gffv1.Value, len(raw))
	for k, v := range raw {
		val, err := decodeOverrideValue(k, v)
		if err != nil {
			return nil, err
		}
		result[k] = val
	}
	return result, nil
}

// decodeOverrideValue converts a raw YAML value into a *gffv1.Value.
func decodeOverrideValue(key string, v any) (*gffv1.Value, error) {
	switch t := v.(type) {
	case bool:
		return &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: t}}, nil
	case string:
		return &gffv1.Value{Kind: &gffv1.Value_ChoiceValue{
			ChoiceValue: &gffv1.ChoiceSelection{Selected: []string{t}},
		}}, nil
	case []any:
		// Must be a homogeneous list of strings.
		strs := make([]string, 0, len(t))
		for _, elem := range t {
			s, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("LoadOverrides: key %q: list element has unsupported type %T", key, elem)
			}
			strs = append(strs, s)
		}
		return &gffv1.Value{Kind: &gffv1.Value_ChoiceValue{
			ChoiceValue: &gffv1.ChoiceSelection{Selected: strs},
		}}, nil
	default:
		return nil, fmt.Errorf("LoadOverrides: key %q has unsupported type %T", key, v)
	}
}
