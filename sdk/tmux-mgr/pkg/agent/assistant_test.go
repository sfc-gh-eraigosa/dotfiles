package agent

import "testing"

func TestDetectHost(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Assistant
	}{
		{"claude code present", map[string]string{"CLAUDECODE": "1"}, AssistantClaude},
		{"claude code empty string", map[string]string{"CLAUDECODE": ""}, AssistantAntigravity},
		{"claude code zero", map[string]string{"CLAUDECODE": "0"}, AssistantAntigravity},
		{"claude code unset", map[string]string{}, AssistantAntigravity},
		{"claude wins when both set", map[string]string{"CLAUDECODE": "1", "GEMINI_CLI": "1"}, AssistantClaude},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(k string) string { return tc.env[k] }
			got := DetectHost(getenv)
			if got != tc.want {
				t.Errorf("DetectHost() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeAssistant(t *testing.T) {
	cases := []struct {
		in   Assistant
		want Assistant
	}{
		{AssistantClaude, AssistantClaude},
		{AssistantAntigravity, AssistantAntigravity},
		// Legacy Gemini CLI identifier normalizes to its Antigravity successor.
		{AssistantGemini, AssistantAntigravity},
		{Assistant(""), Assistant("")},
	}
	for _, tc := range cases {
		if got := NormalizeAssistant(tc.in); got != tc.want {
			t.Errorf("NormalizeAssistant(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAssistantBinary(t *testing.T) {
	cases := []struct {
		in   Assistant
		want string
	}{
		{AssistantClaude, "claude"},
		{AssistantAntigravity, "agy"},
		// Legacy identifier still resolves to the Antigravity binary.
		{AssistantGemini, "agy"},
	}
	for _, tc := range cases {
		if got := tc.in.Binary(); got != tc.want {
			t.Errorf("(%q).Binary() = %q, want %q", tc.in, got, tc.want)
		}
	}
}
