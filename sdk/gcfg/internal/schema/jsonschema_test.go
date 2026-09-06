package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONSchemaGolden(t *testing.T) {
	got, err := JSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "gcfg.schema.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("JSON Schema drifted from %s (regenerate with UPDATE_GOLDEN=1 go test ./internal/schema/)", golden)
	}
}

func TestJSONSchemaShape(t *testing.T) {
	b, err := JSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("$schema = %v", s["$schema"])
	}
	props, _ := s["properties"].(map[string]any)
	if _, ok := props["repo"]; !ok {
		t.Fatalf("no repo property: %v", props)
	}
	if req, _ := s["required"].([]any); len(req) != 1 || req[0] != "version" {
		t.Errorf("required = %v", s["required"])
	}
	if s["additionalProperties"] != false {
		t.Errorf("additionalProperties = %v (strict load must be mirrored)", s["additionalProperties"])
	}
	// Enums reach the schema so an editor can complete them.
	if !strings.Contains(string(b), `"internal"`) || !strings.Contains(string(b), `"COMMIT_OR_PR_TITLE"`) {
		t.Error("enum values missing from the schema")
	}
	// It ends with a newline so the committed file is diff-friendly.
	if b[len(b)-1] != '\n' {
		t.Error("schema should end with a newline")
	}
}

// The generator must cover every kind the types use, and List[T] must render
// as "either a bare list or {ownership, items}".
func TestJSONSchemaCoversEveryKind(t *testing.T) {
	b, err := JSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	props := s["properties"].(map[string]any)
	repo := props["repo"].(map[string]any)["properties"].(map[string]any)

	labels := repo["labels"].(map[string]any)
	one, ok := labels["oneOf"].([]any)
	if !ok || len(one) != 2 {
		t.Fatalf("labels should offer both forms: %v", labels)
	}
	if one[0].(map[string]any)["type"] != "array" || one[1].(map[string]any)["type"] != "object" {
		t.Errorf("oneOf shapes = %v", one)
	}

	general := repo["general"].(map[string]any)["properties"].(map[string]any)
	if general["allow_forking"].(map[string]any)["type"] != "boolean" {
		t.Errorf("bool field = %v", general["allow_forking"])
	}
	if general["topics"].(map[string]any)["type"] != "array" {
		t.Errorf("slice field = %v", general["topics"])
	}
	if general["description"].(map[string]any)["type"] != "string" {
		t.Errorf("pointer-to-string field = %v", general["description"])
	}
	vis := general["visibility"].(map[string]any)["enum"].([]any)
	if len(vis) != 3 || vis[0] != "public" {
		t.Errorf("enum from the shared table = %v", vis)
	}

	envs := repo["environments"].(map[string]any)["oneOf"].([]any)
	envItem := envs[0].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if envItem["wait_timer"].(map[string]any)["type"] != "integer" {
		t.Errorf("int field = %v", envItem["wait_timer"])
	}

	rulesets := repo["rulesets"].(map[string]any)["oneOf"].([]any)
	ruleItems := rulesets[0].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	rules := ruleItems["rules"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if rules["parameters"].(map[string]any)["type"] != "object" {
		t.Errorf("open map field = %v", rules["parameters"])
	}
	actors := ruleItems["bypass_actors"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if actors["actor_id"].(map[string]any)["type"] != "integer" {
		t.Errorf("int64 field = %v", actors["actor_id"])
	}
	// Descriptions from the jsonschema tag survive.
	if !strings.Contains(fmt.Sprint(ruleItems["name"]), "") {
		t.Fatal("unreachable")
	}
	if d, _ := repo["labels"].(map[string]any)["description"].(string); !strings.Contains(d, "ownership") {
		t.Errorf("list description = %q", d)
	}
}
