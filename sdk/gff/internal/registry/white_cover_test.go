package registry

import "testing"

// White-box coverage of the unexported yaml-normalize helper.
func TestNormalizeMapKeysVariants(t *testing.T) {
	in := map[any]any{
		1:      "a",
		"list": []any{map[any]any{2: "b"}},
		"map":  map[string]any{"k": 3},
	}
	out, ok := normalizeMapKeys(in).(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any, got %T", normalizeMapKeys(in))
	}
	if out["1"] != "a" {
		t.Fatal("int key not stringified")
	}
	if s := normalizeMapKeys(42); s != 42 {
		t.Fatal("scalar passthrough broken")
	}
}
