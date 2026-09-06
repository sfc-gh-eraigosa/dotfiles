package agent

// Assistant identifies which AI CLI tmux-mgr should spawn inside each agent pane.
type Assistant string

const (
	AssistantClaude      Assistant = "claude"
	AssistantAntigravity Assistant = "antigravity"
	// AssistantGemini is the retired Gemini CLI (EOL June 2026), replaced by
	// Antigravity CLI. Kept only so legacy env values and session records still
	// parse; NormalizeAssistant maps it to AssistantAntigravity.
	AssistantGemini Assistant = "gemini"
)

// NormalizeAssistant maps legacy assistant identifiers to their current
// equivalents. Old records or env vars may still say "gemini"; Antigravity
// CLI is its replacement.
func NormalizeAssistant(a Assistant) Assistant {
	if a == AssistantGemini {
		return AssistantAntigravity
	}
	return a
}

// Binary returns the executable name for the assistant's CLI. The Antigravity
// CLI ships as `agy`, not `antigravity`; every other assistant's binary
// matches its identifier.
func (a Assistant) Binary() string {
	if NormalizeAssistant(a) == AssistantAntigravity {
		return "agy"
	}
	return string(a)
}

// DetectHost returns the assistant currently driving the user, based on env.
// Antigravity is the default — preserving the pre-EOL behavior where the
// Google CLI (then Gemini) was assumed when no host signal is present.
func DetectHost(getenv func(string) string) Assistant {
	if getenv("CLAUDECODE") == "1" {
		return AssistantClaude
	}
	return AssistantAntigravity
}
