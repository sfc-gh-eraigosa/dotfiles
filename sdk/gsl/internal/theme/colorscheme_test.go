package theme_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/theme"
)

// writeAgySettings writes a raw settings.json body to the Antigravity CLI path.
func writeAgySettings(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestResolve_Antigravity_TopLevelColorScheme is the ground-truth test.
//
// gsl read "ui.theme", but agy does not write that key. The REAL
// ~/.gemini/antigravity-cli/settings.json on a machine running agy v1.1.1 is:
//
//	{
//	  "colorScheme": "light",
//	  "enableTelemetry": false,
//	  "trustedWorkspaces": ["..."],
//	  "statusLine": { "type": "command", "command": "...", "enabled": true }
//	}
//
// colorScheme is TOP-LEVEL. Because gsl looked for a "ui" object that never
// exists, readAntigravityTheme returned "" for every real agy user and Resolve
// fell through to the terminal palette — so a user on a LIGHT agy theme got a
// DARK status line, permanently.
func TestResolve_Antigravity_TopLevelColorScheme(t *testing.T) {
	home := t.TempDir()
	// Byte-faithful to the real file (paths/flags aside).
	writeAgySettings(t, home, `{
  "colorScheme": "light",
  "enableTelemetry": false,
  "trustedWorkspaces": ["/home/user/git/dotfiles"],
  "statusLine": {
    "type": "command",
    "command": "bash ~/.gemini/config/statusline-command.sh",
    "enabled": true
  }
}`)
	if got, want := theme.Resolve("antigravity", noEnv(), home), "light"; got != want {
		t.Errorf("agy colorScheme=light: got %q, want %q "+
			"(gsl is reading ui.theme, which agy never writes)", got, want)
	}
}

// TestResolve_Antigravity_ColorSchemeKeywords runs the keyword bridge over the
// top-level colorScheme key.
func TestResolve_Antigravity_ColorSchemeKeywords(t *testing.T) {
	cases := []struct{ scheme, want string }{
		{"light", "light"},
		{"Ayu Light", "light"},
		{"dark", "dark"},
		{"Tokyo Night", "dark"},
		{"dark-daltonized", "dark-daltonism"},
		{"colorblind", "dark-daltonism"},
	}
	for _, tc := range cases {
		t.Run(tc.scheme, func(t *testing.T) {
			home := t.TempDir()
			writeAgySettings(t, home, `{"colorScheme": "`+tc.scheme+`"}`)
			if got := theme.Resolve("antigravity", noEnv(), home); got != tc.want {
				t.Errorf("colorScheme=%q: got %q, want %q", tc.scheme, got, tc.want)
			}
		})
	}
}

// TestResolve_Antigravity_ColorSchemeWinsOverLegacyUITheme — colorScheme is
// what agy actually writes, so it must win when both are somehow present.
func TestResolve_Antigravity_ColorSchemeWinsOverLegacyUITheme(t *testing.T) {
	home := t.TempDir()
	writeAgySettings(t, home, `{"colorScheme": "light", "ui": {"theme": "Tokyo Night"}}`)
	if got, want := theme.Resolve("antigravity", noEnv(), home), "light"; got != want {
		t.Errorf("colorScheme must win over legacy ui.theme: got %q, want %q", got, want)
	}
}

// TestResolve_Antigravity_LegacyUIThemeStillWorks — ui.theme remains a fallback
// for anyone whose settings file still carries it.
func TestResolve_Antigravity_LegacyUIThemeStillWorks(t *testing.T) {
	home := t.TempDir()
	writeAgySettings(t, home, `{"ui": {"theme": "Ayu Light"}}`)
	if got, want := theme.Resolve("antigravity", noEnv(), home), "light"; got != want {
		t.Errorf("legacy ui.theme fallback: got %q, want %q", got, want)
	}
}

// TestClaudeEnum_AutoAndVariants — claudeEnumToPalette used an exact-match
// switch, so every value it did not literally know collapsed to "dark".
// Claude ships "auto", "light-ansi", "dark-ansi" and the "*-daltonized"
// variants; all of them silently rendered a dark line, including on a LIGHT
// terminal. Substring matching fixes the whole family at once.
//
// Precedence is "light" BEFORE "daltonism", matching the Antigravity keyword
// bridge that already shipped. Note the consequence for "light-daltonized":
// internal/style defines exactly four palettes — dark, light, dark-daltonism,
// dark8 — and there is NO light-daltonism. Returning "light" keeps the
// background assumption correct (the user is on a light terminal); returning
// "dark-daltonism" would invert it. The accessibility gap is real but it is a
// MISSING PALETTE, not a mapping bug, and inventing a palette name here would
// resolve to nothing at all.
func TestClaudeEnum_AutoAndVariants(t *testing.T) {
	cases := []struct{ theme, want string }{
		// Known-good, must not regress.
		{"dark", "dark"},
		{"light", "light"},
		{"dark-daltonism", "dark-daltonism"},
		{"system", "dark"},
		{"", "dark"},
		// The family that collapsed to "dark" before.
		{"auto", "dark"},
		{"light-ansi", "light"},
		{"dark-ansi", "dark"},
		{"dark-daltonized", "dark-daltonism"},
		{"dark-daltonized-ansi", "dark-daltonism"},
		// light wins over daltonism (see the doc comment above).
		{"light-daltonized", "light"},
		{"light-daltonized-ansi", "light"},
	}
	for _, tc := range cases {
		t.Run(tc.theme, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".claude")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			body := `{"theme": "` + tc.theme + `"}`
			if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := theme.Resolve("claude", noEnv(), home); got != tc.want {
				t.Errorf("claude theme=%q: got %q, want %q", tc.theme, got, tc.want)
			}
		})
	}
}
