package theme

import "strings"

// Resolve returns the palette name to use based on the host tool context,
// settings files, and terminal environment variables.
//
// Priority order:
//
//  1. Host-tool settings file:
//     - toolCtx == "claude": read <home>/.claude/settings.json field "theme".
//     Enum values: "dark" → "dark"; "light" → "light";
//     "dark-daltonism" → "dark-daltonism"; "system"/absent → "dark".
//     - toolCtx == "antigravity": read the
//     "ui.theme" field (free-form string) from
//     <home>/.gemini/antigravity-cli/settings.json, falling back to the
//     legacy <home>/.gemini/settings.json only when that file is absent.
//     Keyword bridge:
//     contains "light" → "light";
//     contains "daltonism" or "colorblind" → "dark-daltonism";
//     any other non-empty value (including unknown themes) → "dark".
//     (Unknown non-empty antigravity theme → "dark", NOT terminal fallthrough.)
//
//  2. Terminal fallback (only when toolCtx=="" OR the settings file is
//     missing/unreadable AND toolCtx==""):
//     env("COLORTERM") == "truecolor" or "24bit" → "dark";
//     env("TERM") contains "256color" → "dark";
//     else → "dark8" (8-color named-color palette).
//
// env and home are injected for testability. The production caller should
// pass os.Getenv and the result of os.UserHomeDir (or $HOME) respectively.
//
// Resolve contains no I/O itself — all file access is via the settings helpers
// in settings.go, which degrade gracefully on any error.
func Resolve(toolCtx string, env func(string) string, home string) string {
	switch toolCtx {
	case "claude":
		raw := readClaudeTheme(home)
		return claudeEnumToPalette(raw)

	case "antigravity":
		raw := readAntigravityTheme(home)
		if raw == "" {
			// Missing/unreadable → terminal fallback.
			return terminalPalette(env)
		}
		return antigravityKeywordToPalette(raw)

	default:
		// toolCtx == "" (or unknown): terminal fallback only.
		return terminalPalette(env)
	}
}

// claudeEnumToPalette maps the Claude settings.json "theme" enum to a palette
// name. "system" and absent/empty both map to "dark".
func claudeEnumToPalette(raw string) string {
	switch raw {
	case "light":
		return "light"
	case "dark-daltonism":
		return "dark-daltonism"
	case "dark":
		return "dark"
	default:
		// "system", "", or any unrecognized value → dark.
		return "dark"
	}
}

// antigravityKeywordToPalette maps a free-form Antigravity ui.theme string to
// a palette name using keyword matching.
//
// Rule: contains "light" → "light"; contains "daltonism" or "colorblind" →
// "dark-daltonism"; anything else non-empty → "dark".
//
// An empty input should never reach here (Resolve handles the empty case
// before calling this), but "" returns "dark" defensively.
func antigravityKeywordToPalette(raw string) string {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "light") {
		return "light"
	}
	if strings.Contains(lower, "daltonism") || strings.Contains(lower, "colorblind") {
		return "dark-daltonism"
	}
	// Non-empty unknown theme → dark (NOT terminal fallthrough per spec).
	return "dark"
}

// terminalPalette resolves a palette from terminal environment variables.
//
//   - inside tmux/screen ($TMUX set, or $TERM tmux*/screen*) → "dark8":
//     the true background is unknowable from env alone there, and basic ANSI
//     indices are recolored by the outer terminal's own theme, so contrast
//     stays correct on any background (mirrors gff's rule, dotfiles#187)
//   - $COLORTERM == "truecolor" or "24bit" → "dark"
//   - $TERM contains "256color"            → "dark"
//   - otherwise                            → "dark8"
func terminalPalette(env func(string) string) string {
	term := strings.ToLower(env("TERM"))
	if env("TMUX") != "" || strings.HasPrefix(term, "tmux") || strings.HasPrefix(term, "screen") {
		return "dark8"
	}
	ct := strings.ToLower(env("COLORTERM"))
	if ct == "truecolor" || ct == "24bit" {
		return "dark"
	}
	if strings.Contains(term, "256color") {
		return "dark"
	}
	return "dark8"
}
