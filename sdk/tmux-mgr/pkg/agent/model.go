package agent

import (
	"regexp"
	"strconv"
	"strings"
)

// Model tiers used to translate Ollama-style model hints into hosted Claude /
// Antigravity equivalents. Sized brackets are deliberately loose because the
// frontmatter Model field is a context clue, not a precise spec.
const (
	ModelTierSmall  = "small"
	ModelTierMedium = "medium"
	ModelTierLarge  = "large"
)

// claudeModels lists known-good Claude model IDs per tier. Keep this in sync
// with the latest Claude model family (currently Claude 4.X).
var claudeModels = map[string]string{
	ModelTierSmall:  "claude-haiku-4-5-20251001",
	ModelTierMedium: "claude-sonnet-4-6",
	ModelTierLarge:  "claude-opus-4-7",
}

// antigravityModels lists model IDs per tier for the Antigravity CLI (which
// serves Google's Gemini model family). The tier ladder is shallower than
// Claude's, so medium and large currently share the same model.
var antigravityModels = map[string]string{
	ModelTierSmall:  "gemini-2.5-flash",
	ModelTierMedium: "gemini-2.5-pro",
	ModelTierLarge:  "gemini-2.5-pro",
}

// parseOllamaSize extracts a parameter count (in billions) from an
// Ollama-style model string like `qwen2.5:1.5b` or `smollm:360m`. The last
// `<num><unit>` token wins so suffixes such as `:1.5b-q4_K_M` still parse.
func parseOllamaSize(model string) (float64, bool) {
	re := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([bm])\b`)
	matches := re.FindAllStringSubmatch(model, -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	val, err := strconv.ParseFloat(last[1], 64)
	if err != nil {
		return 0, false
	}
	if strings.ToLower(last[2]) == "m" {
		val /= 1000
	}
	return val, true
}

// tierForSize maps a parameter count (in billions) to a tier. Sub-3B is small,
// 3–8B is medium, 8B+ is large.
func tierForSize(billions float64) string {
	switch {
	case billions < 3:
		return ModelTierSmall
	case billions < 8:
		return ModelTierMedium
	default:
		return ModelTierLarge
	}
}

// SelectModel chooses a concrete model ID for the given assistant based on the
// definition's Model: hint. Returns "" when no decision can be made — callers
// MUST omit the --model flag in that case so the assistant inherits whichever
// model its CLI session was launched with.
//
// Special case: a nil definition (generalist with no .md file) always resolves
// to the cheapest tier because generalist work is meant to be token-efficient.
func SelectModel(def *Definition, host Assistant) string {
	tier := ""
	if def == nil {
		tier = ModelTierSmall
	} else if size, ok := parseOllamaSize(def.Model); ok {
		tier = tierForSize(size)
	} else {
		return ""
	}
	switch NormalizeAssistant(host) {
	case AssistantClaude:
		return claudeModels[tier]
	case AssistantAntigravity:
		return antigravityModels[tier]
	default:
		return ""
	}
}
