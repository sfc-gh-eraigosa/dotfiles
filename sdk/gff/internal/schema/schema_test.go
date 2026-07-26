package schema_test

import (
	"os"
	"path/filepath"
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/schema"
)

// writeFile writes content to a temp file and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFeatureFileYAML(t *testing.T) {
	p := writeFile(t, "features.yaml", `
namespace: com.example.demo
sets:
  - area: install
    features:
      - {path: install.ai.claude, description: Claude CLI, boolDefault: true}
      - path: install.pkg.manager
        description: Package manager
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: auto, description: Auto-detect, stringValue: auto, selected: true}
            - {id: apt, description: Debian/Ubuntu apt, stringValue: apt}
`)
	ff, err := schema.LoadFeatureFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := ff.Sets[0].Features[0].GetBoolDefault(); !got {
		t.Fatal("bool default")
	}
	cd := ff.Sets[0].Features[1].GetChoiceDefault()
	if cd.Mode != gffv1.ChoiceMode_CHOICE_MODE_SINGLE {
		t.Fatal("mode")
	}
	if cd.Options[1].Id != "apt" || cd.Options[1].GetStringValue() != "apt" {
		t.Fatal("choice option")
	}
	if !cd.Options[0].Selected {
		t.Fatal("default selection")
	}
}

func TestLoadFeatureFileJSON(t *testing.T) {
	p := writeFile(t, "features.json", `{
  "namespace": "com.example.demo",
  "sets": [{
    "area": "install",
    "features": [{
      "path": "install.ai.claude",
      "description": "Claude CLI",
      "boolDefault": true
    }]
  }]
}`)
	ff, err := schema.LoadFeatureFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !ff.Sets[0].Features[0].GetBoolDefault() {
		t.Fatal("json bool default")
	}
}

func TestLoadFeatureFileYML(t *testing.T) {
	p := writeFile(t, "features.yml", `
namespace: com.example.demo
sets:
  - area: install
    features:
      - {path: install.ai.claude, description: Claude CLI, boolDefault: true}
`)
	ff, err := schema.LoadFeatureFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if ff.Namespace != "com.example.demo" {
		t.Fatalf("namespace: got %q", ff.Namespace)
	}
}

func TestLoadFeatureFileNotFound(t *testing.T) {
	_, err := schema.LoadFeatureFile(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFeatureFileUnknownField(t *testing.T) {
	// DiscardUnknown: false means unknown fields should cause an error.
	p := writeFile(t, "features.yaml", `
namespace: com.example.demo
sets: []
unknownField: bad
`)
	_, err := schema.LoadFeatureFile(p)
	if err == nil {
		t.Fatal("expected error for unknown field with DiscardUnknown:false")
	}
}

func TestLoadOverrides(t *testing.T) {
	p := writeFile(t, "config.yaml",
		"install.ai.claude: false\ninstall.pkg.manager: apt\nshell.zsh.plugins: [fzf, starship]\n")
	o, err := schema.LoadOverrides(p)
	if err != nil {
		t.Fatal(err)
	}
	if o["install.ai.claude"].GetBoolValue() != false {
		t.Fatal("bool override")
	}
	if got := o["install.pkg.manager"].GetChoiceValue().Selected; len(got) != 1 || got[0] != "apt" {
		t.Fatal("single choice override")
	}
	if got := o["shell.zsh.plugins"].GetChoiceValue().Selected; len(got) != 2 {
		t.Fatal("multi choice override")
	}
}

func TestLoadOverridesBoolTrue(t *testing.T) {
	p := writeFile(t, "config.yaml", "install.ai.claude: true\n")
	o, err := schema.LoadOverrides(p)
	if err != nil {
		t.Fatal(err)
	}
	if o["install.ai.claude"].GetBoolValue() != true {
		t.Fatal("bool true override")
	}
}

func TestLoadOverridesMissingFile(t *testing.T) { // sparse layer absent => empty, no error
	o, err := schema.LoadOverrides(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || len(o) != 0 {
		t.Fatalf("want empty/nil, got %v/%v", o, err)
	}
}

func TestLoadOverridesInvalidType(t *testing.T) {
	// int, float, nested map, mixed-type list => parse error
	p := writeFile(t, "config.yaml", "install.ai.claude: 42\n")
	_, err := schema.LoadOverrides(p)
	if err == nil {
		t.Fatal("expected error for integer override value")
	}
}

func TestLoadOverridesNestedMap(t *testing.T) {
	p := writeFile(t, "config.yaml", "install.ai.claude:\n  nested: value\n")
	_, err := schema.LoadOverrides(p)
	if err == nil {
		t.Fatal("expected error for nested map override value")
	}
}

func TestLoadOverridesMixedTypeList(t *testing.T) {
	// A list with mixed types (string and int) is invalid.
	p := writeFile(t, "config.yaml", "shell.zsh.plugins: [fzf, 42]\n")
	_, err := schema.LoadOverrides(p)
	if err == nil {
		t.Fatal("expected error for mixed-type list override value")
	}
}

func TestLoadFeatureFileChoiceMultiSelect(t *testing.T) {
	p := writeFile(t, "features.yaml", `
namespace: com.example.demo
sets:
  - area: shell
    features:
      - path: shell.zsh.plugins
        description: ZSH plugins
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - {id: fzf, description: FZF fuzzy finder, stringValue: fzf, selected: true}
            - {id: starship, description: Starship prompt, stringValue: starship, selected: true}
            - {id: zoxide, description: Zoxide, stringValue: zoxide}
`)
	ff, err := schema.LoadFeatureFile(p)
	if err != nil {
		t.Fatal(err)
	}
	cd := ff.Sets[0].Features[0].GetChoiceDefault()
	if cd.Mode != gffv1.ChoiceMode_CHOICE_MODE_MULTI {
		t.Fatalf("expected MULTI mode, got %v", cd.Mode)
	}
	if len(cd.Options) != 3 {
		t.Fatalf("expected 3 options, got %d", len(cd.Options))
	}
}
