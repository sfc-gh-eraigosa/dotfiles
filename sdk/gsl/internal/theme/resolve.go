package theme

import (
	"strings"

	sdktheme "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/theme"
)

// Resolve returns the palette name, by the sdk's one rule
// (sdk/libs/tui/theme.Resolve) with the host tool's theme setting as its hint:
//
//  1. GSL_THEME, then SDK_THEME, when it names a palette.
//
//  2. The host-tool settings file (the hint):
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
//     A missing antigravity file, or no tool, gives no hint.
//
//  3. The terminal (no hint): inside tmux/screen without COLORFGBG, or
//     without 256 colors → "dark8"; else COLORFGBG's background, dark when
//     unknown → "dark" / "light".
//
// env and home are injected for testability. The production caller should
// pass os.Getenv and the result of os.UserHomeDir (or $HOME) respectively.
//
// Resolve contains no I/O itself — all file access is via the settings helpers
// in settings.go, which degrade gracefully on any error.
func Resolve(toolCtx string, env func(string) string, home string) string {
	hint := func() string {
		switch toolCtx {
		case "claude":
			return claudeEnumToPalette(readClaudeTheme(home))
		case "antigravity":
			if raw := readAntigravityTheme(home); raw != "" {
				return antigravityKeywordToPalette(raw)
			}
		}
		return "" // missing/unreadable antigravity settings, or no tool: terminal
	}
	return sdktheme.Resolve("gsl", env, sdktheme.EnvDarkBackground(env), hint)
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
