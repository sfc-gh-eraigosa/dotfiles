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

// claudeEnumToPalette maps the Claude settings.json "theme" value to a palette
// name.
//
// This used to be an exact-match switch over {"light","dark","dark-daltonism"},
// which meant every OTHER value Claude ships collapsed to "dark" — including
// "light-ansi" (a LIGHT theme rendered dark), "auto", and the whole
// "*-daltonized" family. It is now the same substring bridge the Antigravity
// path uses, so an unknown-but-descriptive theme name resolves sensibly instead
// of silently defaulting.
//
// "system", "auto", "" and any value with no light/daltonism keyword → "dark".
func claudeEnumToPalette(raw string) string {
	return keywordToPalette(raw)
}

// antigravityKeywordToPalette maps a free-form Antigravity colorScheme (or
// legacy ui.theme) string to a palette name.
func antigravityKeywordToPalette(raw string) string {
	return keywordToPalette(raw)
}

// keywordToPalette is the shared substring bridge from a free-form theme name
// to one of the four palettes internal/style actually defines: "dark",
// "light", "dark-daltonism", "dark8".
//
// Rule (order matters):
//
//	contains "light"                        → "light"
//	contains "dalton" or "colorblind"       → "dark-daltonism"
//	otherwise (incl. "", "system", "auto")  → "dark"
//
// "light" is tested FIRST, so "light-daltonized" resolves to "light". That is
// deliberate: there is no light-daltonism palette, and returning
// "dark-daltonism" would invert the user's background. The accessibility gap is
// a MISSING PALETTE, not a mapping bug — naming one here that internal/style
// does not define would resolve to nothing at all.
//
// Matching "dalton" (not "daltonism") covers both the "-daltonism" and the
// "-daltonized" spellings the two CLIs use.
func keywordToPalette(raw string) string {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "light") {
		return "light"
	}
	if strings.Contains(lower, "dalton") || strings.Contains(lower, "colorblind") {
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
