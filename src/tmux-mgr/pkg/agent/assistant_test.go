package agent

import "testing"

func TestDetectHost(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Assistant
	}{
		{"claude code present", map[string]string{"CLAUDECODE": "1"}, AssistantClaude},
		{"claude code empty string", map[string]string{"CLAUDECODE": ""}, AssistantGemini},
		{"claude code zero", map[string]string{"CLAUDECODE": "0"}, AssistantGemini},
		{"claude code unset", map[string]string{}, AssistantGemini},
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
