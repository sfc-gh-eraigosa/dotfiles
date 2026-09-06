package schema

import (
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
)

// White-box coverage of unexported helpers.

func TestNormalizeMapKeysVariants(t *testing.T) {
	in := map[any]any{
		1:      map[any]any{true: "x"},
		"list": []any{map[any]any{2: "y"}, "z"},
	}
	out, ok := normalizeMapKeys(in).(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any, got %T", normalizeMapKeys(in))
	}
	if _, ok := out["1"]; !ok {
		t.Fatal("int key not stringified")
	}
	if s := normalizeMapKeys("scalar"); s != "scalar" {
		t.Fatalf("scalar passthrough broken: %v", s)
	}
	if m, ok := normalizeMapKeys(map[string]any{"a": []any{1}}).(map[string]any); !ok || len(m) != 1 {
		t.Fatal("map[string]any branch broken")
	}
}

func TestValueTypeNameAllArms(t *testing.T) {
	cases := []struct {
		opt  *gffv1.ChoiceOption
		want string
	}{
		{&gffv1.ChoiceOption{Value: &gffv1.ChoiceOption_IntValue{IntValue: 1}}, "int"},
		{&gffv1.ChoiceOption{Value: &gffv1.ChoiceOption_FloatValue{FloatValue: 1.5}}, "float"},
		{&gffv1.ChoiceOption{Value: &gffv1.ChoiceOption_StringValue{StringValue: "s"}}, "string"},
		{&gffv1.ChoiceOption{Value: &gffv1.ChoiceOption_BoolValue{BoolValue: true}}, "bool"},
		{&gffv1.ChoiceOption{}, "none"},
	}
	for _, c := range cases {
		if got := valueTypeName(c.opt); got != c.want {
			t.Errorf("valueTypeName(%v) = %q, want %q", c.opt, got, c.want)
		}
	}
}

// A feature that declares neither boolDefault nor choiceDefault is meaningless
// — lint must flag it (plan §7.2 IA-3 wants default-less features rejected).
func TestLintMissingDefault(t *testing.T) {
	f := &gffv1.FeatureFile{
		Namespace: "com.example.demo",
		Sets: []*gffv1.FeatureSet{{
			Area:     "install",
			Features: []*gffv1.Feature{{Path: "install.ai.claude", Description: "no default"}},
		}},
	}
	findings := Lint(f)
	found := false
	for _, fd := range findings {
		if fd.Rule == "missing-default" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want missing-default finding, got %v", findings)
	}
}
