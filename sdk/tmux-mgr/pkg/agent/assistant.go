package agent

// Assistant identifies which AI CLI tmux-mgr should spawn inside each agent pane.
type Assistant string

const (
	AssistantClaude Assistant = "claude"
	AssistantGemini Assistant = "gemini"
)

// DetectHost returns the assistant currently driving the user, based on env.
// Gemini is the default — preserving existing behavior when no host signal is present.
func DetectHost(getenv func(string) string) Assistant {
	if getenv("CLAUDECODE") == "1" {
		return AssistantClaude
	}
	return AssistantGemini
}
