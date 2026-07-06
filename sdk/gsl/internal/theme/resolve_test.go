package theme_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/theme"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// noEnv returns a function that always returns "" (no environment variables).
func noEnv() func(string) string {
	return func(string) string { return "" }
}

// envMap builds an env function from a map.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// writeClaudeSettings writes <home>/.claude/settings.json with the given theme
// string. An empty theme omits the field.
func writeClaudeSettings(t *testing.T, home, theme string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	var obj map[string]any
	if theme != "" {
		obj = map[string]any{"theme": theme}
	} else {
		obj = map[string]any{}
	}
	data, _ := json.Marshal(obj)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// writeAntigravitySettings writes <home>/.gemini/antigravity-cli/settings.json
// with ui.theme set (the Antigravity CLI reuses the ~/.gemini directory).
func writeAntigravitySettings(t *testing.T, home, uiTheme string) {
	t.Helper()
	writeUIThemeSettings(t, filepath.Join(home, ".gemini", "antigravity-cli"), uiTheme)
}

// writeLegacyGeminiSettings writes the legacy Gemini CLI file
// <home>/.gemini/settings.json with ui.theme set.
func writeLegacyGeminiSettings(t *testing.T, home, uiTheme string) {
	t.Helper()
	writeUIThemeSettings(t, filepath.Join(home, ".gemini"), uiTheme)
}

// writeUIThemeSettings writes <dir>/settings.json with ui.theme set. An empty
// uiTheme omits the field.
func writeUIThemeSettings(t *testing.T, dir, uiTheme string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	var obj map[string]any
	if uiTheme != "" {
		obj = map[string]any{"ui": map[string]any{"theme": uiTheme}}
	} else {
		obj = map[string]any{}
	}
	data, _ := json.Marshal(obj)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// ── Resolve: claude toolCtx ───────────────────────────────────────────────────

func TestResolve_Claude_DarkEnum(t *testing.T) {
	home := t.TempDir()
	writeClaudeSettings(t, home, "dark")
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark" {
		t.Errorf("claude dark: got %q, want %q", got, "dark")
	}
}

func TestResolve_Claude_LightEnum(t *testing.T) {
	home := t.TempDir()
	writeClaudeSettings(t, home, "light")
	got := theme.Resolve("claude", noEnv(), home)
	if got != "light" {
		t.Errorf("claude light: got %q, want %q", got, "light")
	}
}

func TestResolve_Claude_DarkDaltonismEnum(t *testing.T) {
	home := t.TempDir()
	writeClaudeSettings(t, home, "dark-daltonism")
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark-daltonism" {
		t.Errorf("claude dark-daltonism: got %q, want %q", got, "dark-daltonism")
	}
}

func TestResolve_Claude_SystemEnum_MappsToDark(t *testing.T) {
	home := t.TempDir()
	writeClaudeSettings(t, home, "system")
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark" {
		t.Errorf("claude system: got %q, want %q", got, "dark")
	}
}

func TestResolve_Claude_AbsentTheme_MappsToDark(t *testing.T) {
	home := t.TempDir()
	// Write settings without a theme field.
	writeClaudeSettings(t, home, "")
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark" {
		t.Errorf("claude absent theme: got %q, want %q", got, "dark")
	}
}

func TestResolve_Claude_MissingFile_MappsToDark(t *testing.T) {
	home := t.TempDir()
	// No settings file at all.
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark" {
		t.Errorf("claude missing file: got %q, want %q", got, "dark")
	}
}

// ── Resolve: antigravity toolCtx ─────────────────────────────────────────────

func TestResolve_Antigravity_LightKeyword(t *testing.T) {
	tests := []struct {
		name  string
		theme string
	}{
		{"Ayu Light", "Ayu Light"},
		{"One Light", "One Light"},
		{"light lowercase", "light"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeAntigravitySettings(t, home, tc.theme)
			got := theme.Resolve("antigravity", noEnv(), home)
			if got != "light" {
				t.Errorf("antigravity %q: got %q, want %q", tc.theme, got, "light")
			}
		})
	}
}

func TestResolve_Antigravity_DarkKeyword(t *testing.T) {
	tests := []struct {
		name  string
		theme string
	}{
		{"Tokyo Night", "Tokyo Night"},
		{"One Dark Pro", "One Dark Pro"},
		{"just dark", "dark"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeAntigravitySettings(t, home, tc.theme)
			got := theme.Resolve("antigravity", noEnv(), home)
			if got != "dark" {
				t.Errorf("antigravity %q: got %q, want %q", tc.theme, got, "dark")
			}
		})
	}
}

func TestResolve_Antigravity_DaltonismKeyword(t *testing.T) {
	tests := []struct {
		name  string
		theme string
	}{
		{"Dark Daltonism", "Dark Daltonism"},
		{"colorblind friendly", "colorblind friendly"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeAntigravitySettings(t, home, tc.theme)
			got := theme.Resolve("antigravity", noEnv(), home)
			if got != "dark-daltonism" {
				t.Errorf("antigravity %q: got %q, want %q", tc.theme, got, "dark-daltonism")
			}
		})
	}
}

func TestResolve_Antigravity_UnknownNonEmpty_MapsToDark_NotTerminalFallthrough(t *testing.T) {
	// An unknown non-empty Antigravity theme must map to "dark", NOT fall through
	// to terminal detection (even if COLORTERM would say "light").
	home := t.TempDir()
	writeAntigravitySettings(t, home, "Gruvbox Material")
	// Even with a truecolor terminal env (which would normally yield "dark"),
	// the key invariant is that we return "dark" not "dark8" or anything from
	// terminal detection — it comes from the antigravity keyword bridge, not env.
	got := theme.Resolve("antigravity", envMap(map[string]string{"COLORTERM": "truecolor"}), home)
	if got != "dark" {
		t.Errorf("antigravity unknown: got %q, want %q", got, "dark")
	}
}

func TestResolve_Antigravity_MissingFile_TerminalFallthrough(t *testing.T) {
	// Missing antigravity settings → falls through to terminal.
	home := t.TempDir()
	got := theme.Resolve("antigravity", envMap(map[string]string{"COLORTERM": "truecolor"}), home)
	if got != "dark" {
		t.Errorf("antigravity missing file truecolor: got %q, want %q", got, "dark")
	}
}

func TestResolve_Antigravity_MissingFile_8ColorFallthrough(t *testing.T) {
	home := t.TempDir()
	got := theme.Resolve("antigravity", noEnv(), home)
	if got != "dark8" {
		t.Errorf("antigravity missing file 8-color: got %q, want %q", got, "dark8")
	}
}

func TestResolve_Antigravity_LegacyGeminiSettingsFallback(t *testing.T) {
	// With no antigravity-cli settings file, the legacy Gemini CLI
	// ~/.gemini/settings.json is still honored.
	home := t.TempDir()
	writeLegacyGeminiSettings(t, home, "Ayu Light")
	got := theme.Resolve("antigravity", noEnv(), home)
	if got != "light" {
		t.Errorf("antigravity legacy fallback: got %q, want %q", got, "light")
	}
}

func TestResolve_Antigravity_NewFileWinsOverLegacy(t *testing.T) {
	// The antigravity-cli settings file takes priority over the legacy one.
	home := t.TempDir()
	writeAntigravitySettings(t, home, "One Light")
	writeLegacyGeminiSettings(t, home, "Tokyo Night")
	got := theme.Resolve("antigravity", noEnv(), home)
	if got != "light" {
		t.Errorf("antigravity priority: got %q, want %q", got, "light")
	}
}

func TestResolve_LegacyGeminiToolCtx_Alias(t *testing.T) {
	// The legacy "gemini" toolCtx is still accepted as an alias.
	home := t.TempDir()
	writeAntigravitySettings(t, home, "Ayu Light")
	got := theme.Resolve("gemini", noEnv(), home)
	if got != "light" {
		t.Errorf("legacy gemini alias: got %q, want %q", got, "light")
	}
}

// ── Resolve: empty toolCtx (terminal fallback) ────────────────────────────────

func TestResolve_Empty_Truecolor(t *testing.T) {
	home := t.TempDir()
	got := theme.Resolve("", envMap(map[string]string{"COLORTERM": "truecolor"}), home)
	if got != "dark" {
		t.Errorf("empty truecolor: got %q, want %q", got, "dark")
	}
}

func TestResolve_Empty_24bit(t *testing.T) {
	home := t.TempDir()
	got := theme.Resolve("", envMap(map[string]string{"COLORTERM": "24bit"}), home)
	if got != "dark" {
		t.Errorf("empty 24bit: got %q, want %q", got, "dark")
	}
}

func TestResolve_Empty_256ColorTerm(t *testing.T) {
	home := t.TempDir()
	got := theme.Resolve("", envMap(map[string]string{"TERM": "xterm-256color"}), home)
	if got != "dark" {
		t.Errorf("empty xterm-256color: got %q, want %q", got, "dark")
	}
}

func TestResolve_Empty_8ColorOnly(t *testing.T) {
	home := t.TempDir()
	got := theme.Resolve("", noEnv(), home)
	if got != "dark8" {
		t.Errorf("empty 8-color: got %q, want %q", got, "dark8")
	}
}

// ── Settings anomaly handling (F5 / SEC-5 / SEC-7) ───────────────────────────

func TestResolve_Claude_MalformedJSON_Degrades(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark" {
		t.Errorf("malformed JSON: got %q, want %q (should degrade)", got, "dark")
	}
}

func TestResolve_Claude_OversizeFile_Degrades(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a file larger than 256 KiB.
	big := make([]byte, 257*1024)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark" {
		t.Errorf("oversize file: got %q, want %q (should degrade)", got, "dark")
	}
}

func TestResolve_Claude_FIFO_Degrades(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fifoPath := filepath.Join(dir, "settings.json")
	if err := os.MkdirAll(filepath.Dir(fifoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a named pipe (FIFO) at the settings path.
	if err := createFIFO(fifoPath); err != nil {
		t.Skipf("cannot create FIFO: %v", err)
	}
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark" {
		t.Errorf("FIFO: got %q, want %q (should degrade)", got, "dark")
	}
}

func TestResolve_Claude_OutOfHomeSymlink_Degrades(t *testing.T) {
	home := t.TempDir()
	// Create a real settings file outside home.
	outside := t.TempDir()
	realFile := filepath.Join(outside, "settings.json")
	if err := os.WriteFile(realFile, []byte(`{"theme":"light"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Symlink inside home pointing outside home.
	symPath := filepath.Join(dir, "settings.json")
	if err := os.Symlink(realFile, symPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	got := theme.Resolve("claude", noEnv(), home)
	// Should degrade because symlink points outside home.
	if got != "dark" {
		t.Errorf("out-of-home symlink: got %q, want %q (should degrade)", got, "dark")
	}
}

func TestResolve_Claude_ThemeWrongType_Degrades(t *testing.T) {
	// theme field is an int instead of a string.
	home := t.TempDir()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":42}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := theme.Resolve("claude", noEnv(), home)
	if got != "dark" {
		t.Errorf("wrong type: got %q, want %q (should degrade to dark)", got, "dark")
	}
}
