package render

import (
	"strconv"
	"strings"
)

// optBool reads a boolean option from a config Segment.Options map, returning
// def when the key is absent or not a bool. Config JSON decodes booleans into
// Go bool already, but we tolerate string "true"/"false" defensively.
func optBool(opts map[string]any, key string, def bool) bool {
	if opts == nil {
		return def
	}
	v, ok := opts[key]
	if !ok {
		return def
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		switch b {
		case "true":
			return true
		case "false":
			return false
		}
	}
	return def
}

// optString reads a string option from opts, returning def when absent or not
// a (non-empty) string.
func optString(opts map[string]any, key, def string) string {
	if opts == nil {
		return def
	}
	v, ok := opts[key]
	if !ok {
		return def
	}
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

// optInt reads an integer option from opts, returning def when absent or not a
// number. Config JSON decodes numbers into float64; an int and a numeric string
// ("8", the way the sibling bool options may be hand-written) are tolerated too.
func optInt(opts map[string]any, key string, def int) int {
	if opts == nil {
		return def
	}
	v, ok := opts[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i
		}
	}
	return def
}
