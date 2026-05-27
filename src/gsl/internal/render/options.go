package render

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
