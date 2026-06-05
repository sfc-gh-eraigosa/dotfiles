package agent

import "testing"

func TestParseOllamaSize(t *testing.T) {
	cases := []struct {
		in    string
		want  float64
		valid bool
	}{
		{"internlm2:1.8b", 1.8, true},
		{"qwen2.5:1.5b", 1.5, true},
		{"qwen2.5-coder:1.5b", 1.5, true},
		{"qwen2.5:0.5b", 0.5, true},
		{"llama3.2:1b", 1.0, true},
		{"smollm:360m", 0.36, true},
		{"smollm:360m-q4_K_M", 0.36, true}, // suffix ignored
		{"llama3.1:70b", 70, true},
		{"foo:bar", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := parseOllamaSize(tc.in)
			if ok != tc.valid {
				t.Fatalf("ok = %v, want %v", ok, tc.valid)
			}
			if ok && got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTierForSize(t *testing.T) {
	cases := []struct {
		size float64
		want string
	}{
		{0.36, ModelTierSmall},
		{1.5, ModelTierSmall},
		{2.9, ModelTierSmall},
		{3, ModelTierMedium},
		{7, ModelTierMedium},
		{8, ModelTierLarge},
		{70, ModelTierLarge},
	}
	for _, tc := range cases {
		if got := tierForSize(tc.size); got != tc.want {
			t.Errorf("tierForSize(%v) = %q, want %q", tc.size, got, tc.want)
		}
	}
}

func TestSelectModel_GeneralistFallsBackToCheapest(t *testing.T) {
	if got := SelectModel(nil, AssistantClaude); got != claudeModels[ModelTierSmall] {
		t.Errorf("generalist Claude = %q, want %q", got, claudeModels[ModelTierSmall])
	}
	if got := SelectModel(nil, AssistantGemini); got != geminiModels[ModelTierSmall] {
		t.Errorf("generalist Gemini = %q, want %q", got, geminiModels[ModelTierSmall])
	}
}

func TestSelectModel_NamedAgentMapsBySize(t *testing.T) {
	cases := []struct {
		name  string
		model string
		host  Assistant
		want  string
	}{
		{"captain ollama 1.8b → claude small", "internlm2:1.8b", AssistantClaude, claudeModels[ModelTierSmall]},
		{"researcher 360m → gemini small", "smollm:360m", AssistantGemini, geminiModels[ModelTierSmall]},
		{"hypothetical 7b → claude medium", "llama3.1:7b", AssistantClaude, claudeModels[ModelTierMedium]},
		{"hypothetical 70b → claude large", "llama3.1:70b", AssistantClaude, claudeModels[ModelTierLarge]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SelectModel(&Definition{Model: tc.model}, tc.host)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelectModel_UnparseableInherits(t *testing.T) {
	// When the Model: field doesn't carry a recognizable size, we return ""
	// so the caller omits --model and the spawned CLI inherits its default.
	if got := SelectModel(&Definition{Model: "some-custom-model"}, AssistantClaude); got != "" {
		t.Errorf("expected empty (inherit), got %q", got)
	}
	if got := SelectModel(&Definition{Model: ""}, AssistantGemini); got != "" {
		t.Errorf("expected empty (inherit) for missing Model, got %q", got)
	}
}
