package style_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/style"
)

// ── Builtin / Builtins ────────────────────────────────────────────────────────

func TestBuiltin_Powerline(t *testing.T) {
	s, ok := style.Builtin("powerline")
	if !ok {
		t.Fatal("Builtin(\"powerline\") returned ok=false")
	}
	if s.Separator != "powerline" {
		t.Errorf("Separator: got %q, want %q", s.Separator, "powerline")
	}
	if !s.Fill {
		t.Error("Fill: got false, want true")
	}
	if s.Glyphs != "nerdfont" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "nerdfont")
	}
	// Must include canonical icon keys.
	for _, key := range []string{"dirgit", "repo_root", "repo_worktree", "branch", "ai", "mcp", "time"} {
		if _, found := s.Icons[key]; !found {
			t.Errorf("Icons missing key %q", key)
		}
	}
	// Theme must carry repo colors.
	if got := s.Theme["repo_root"]; got == "" {
		t.Error("Theme[\"repo_root\"] is empty")
	}
	if got := s.Theme["repo_worktree"]; got == "" {
		t.Error("Theme[\"repo_worktree\"] is empty")
	}
}

func TestBuiltin_Emoji(t *testing.T) {
	s, ok := style.Builtin("emoji")
	if !ok {
		t.Fatal("Builtin(\"emoji\") returned ok=false")
	}
	if s.Separator != "thin" {
		t.Errorf("Separator: got %q, want %q", s.Separator, "thin")
	}
	if s.Fill {
		t.Error("Fill: got true, want false")
	}
	if s.Glyphs != "emoji" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "emoji")
	}
	// Key emoji icons.
	wantIcons := map[string]string{
		"repo_root":     "🏠",
		"repo_worktree": "🌳",
		"ai":            "🤖",
		"mcp":           "🔌",
		"time":          "⏰",
		"branch":        "🌿",
	}
	for key, want := range wantIcons {
		if got := s.Icons[key]; got != want {
			t.Errorf("Icons[%q]: got %q, want %q", key, got, want)
		}
	}
}

func TestBuiltin_Unknown(t *testing.T) {
	_, ok := style.Builtin("nonexistent")
	if ok {
		t.Error("Builtin(\"nonexistent\") returned ok=true, want false")
	}
}

func TestBuiltins_ContainsBothStyles(t *testing.T) {
	all := style.Builtins()
	for _, name := range []string{"powerline", "emoji"} {
		if _, ok := all[name]; !ok {
			t.Errorf("Builtins() missing %q", name)
		}
	}
}

func TestBuiltins_ReturnsCopy(t *testing.T) {
	// Mutating the returned map must not affect subsequent calls.
	all1 := style.Builtins()
	all1["powerline"].Icons["dirgit"] = "MUTATED"

	all2 := style.Builtins()
	if v := all2["powerline"].Icons["dirgit"]; v == "MUTATED" {
		t.Error("Builtins() shares icon map references; expected defensive copy")
	}
}

// ── Resolve: unknown style name ───────────────────────────────────────────────

func TestResolve_UnknownName_FallsBackToPowerline(t *testing.T) {
	var buf bytes.Buffer
	s := style.Resolve(&buf, "does-not-exist", nil, false)
	if s.Separator != "powerline" {
		t.Errorf("fallback Separator: got %q, want %q", s.Separator, "powerline")
	}
	if !s.Fill {
		t.Error("fallback Fill: got false, want true")
	}
}

func TestResolve_UnknownName_WritesWarning(t *testing.T) {
	var buf bytes.Buffer
	style.Resolve(&buf, "no-such-style", nil, false)
	warn := buf.String()
	if !strings.Contains(warn, "no-such-style") {
		t.Errorf("warning missing style name; got: %q", warn)
	}
	if !strings.Contains(warn, "powerline") {
		t.Errorf("warning missing fallback name; got: %q", warn)
	}
}

func TestResolve_KnownName_NoWarning(t *testing.T) {
	var buf bytes.Buffer
	style.Resolve(&buf, "powerline", nil, false)
	if buf.Len() > 0 {
		t.Errorf("unexpected warning for known style: %q", buf.String())
	}
}

// ── Resolve: deep-merge ───────────────────────────────────────────────────────

func TestResolve_DeepMerge_OverrideOneField(t *testing.T) {
	// Override only the Separator of powerline; all other fields must be
	// retained from the built-in.
	user := map[string]style.Style{
		"powerline": {Separator: "space", Fill: true, Glyphs: "nerdfont"},
	}
	var buf bytes.Buffer
	s := style.Resolve(&buf, "powerline", user, false)

	if s.Separator != "space" {
		t.Errorf("Separator: got %q, want %q", s.Separator, "space")
	}
	// Fill and Glyphs should match user (passed through mergeInto).
	if s.Glyphs != "nerdfont" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "nerdfont")
	}
	// Icons from built-in must still be present.
	if _, found := s.Icons["branch"]; !found {
		t.Error("Icons[\"branch\"] lost after single-field override")
	}
	// Theme from built-in must still be present.
	if _, found := s.Theme["repo_root"]; !found {
		t.Error("Theme[\"repo_root\"] lost after single-field override")
	}
}

func TestResolve_DeepMerge_OverrideSingleIcon(t *testing.T) {
	// Override one icon; other icons must be inherited from the built-in.
	user := map[string]style.Style{
		"powerline": {Icons: map[string]string{"ai": "★"}},
	}
	var buf bytes.Buffer
	s := style.Resolve(&buf, "powerline", user, false)

	if got := s.Icons["ai"]; got != "★" {
		t.Errorf("Icons[\"ai\"]: got %q, want %q", got, "★")
	}
	// Branch icon from built-in must be retained.
	if got := s.Icons["branch"]; got == "" {
		t.Error("Icons[\"branch\"] emptied after single-icon override")
	}
}

func TestResolve_DeepMerge_OverrideSingleTheme(t *testing.T) {
	user := map[string]style.Style{
		"powerline": {Theme: map[string]string{"repo_root": "red"}},
	}
	var buf bytes.Buffer
	s := style.Resolve(&buf, "powerline", user, false)

	if got := s.Theme["repo_root"]; got != "red" {
		t.Errorf("Theme[\"repo_root\"]: got %q, want %q", got, "red")
	}
	// repo_worktree from built-in must be retained.
	if got := s.Theme["repo_worktree"]; got == "" {
		t.Error("Theme[\"repo_worktree\"] emptied after single-theme override")
	}
}

func TestResolve_BrandNewStyleName(t *testing.T) {
	// A style name that is NOT a built-in but IS in userStyles.
	// It should be merged over the powerline base AND a warning emitted.
	user := map[string]style.Style{
		"mything": {
			Separator: "thin",
			Fill:      false,
			Glyphs:    "emoji",
			Icons:     map[string]string{"ai": "🎯"},
		},
	}
	var buf bytes.Buffer
	s := style.Resolve(&buf, "mything", user, false)

	// Warning must mention the unknown name.
	if !strings.Contains(buf.String(), "mything") {
		t.Errorf("warning missing brand-new style name; got: %q", buf.String())
	}
	// User fields must win.
	if s.Separator != "thin" {
		t.Errorf("Separator: got %q, want %q", s.Separator, "thin")
	}
	if s.Glyphs != "emoji" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "emoji")
	}
	if got := s.Icons["ai"]; got != "🎯" {
		t.Errorf("Icons[\"ai\"]: got %q, want %q", got, "🎯")
	}
	// Built-in icons not overridden should be inherited from powerline base.
	if got := s.Icons["branch"]; got == "" {
		t.Error("Icons[\"branch\"] missing; expected inheritance from powerline base")
	}
}

// ── Resolve: ASCII fallback ───────────────────────────────────────────────────

func TestResolve_ASCIIGlyphs_UsesASCIITable(t *testing.T) {
	user := map[string]style.Style{
		"powerline": {Glyphs: "ascii", Separator: "powerline", Fill: true},
	}
	var buf bytes.Buffer
	s := style.Resolve(&buf, "powerline", user, false)

	if s.Glyphs != "ascii" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "ascii")
	}
	// ASCII table key checks.
	checks := map[string]string{
		"repo_root":     "[root]",
		"repo_worktree": "[wt]",
		"staged":        "*",
		"unstaged":      "!",
		"untracked":     "?",
	}
	for key, want := range checks {
		if got := s.Icons[key]; got != want {
			t.Errorf("ascii Icons[%q]: got %q, want %q", key, got, want)
		}
	}
}

func TestResolve_ForceASCII_OverridesGlyphs(t *testing.T) {
	// Even with the powerline style (nerdfont glyphs), forceASCII=true
	// must replace the icon table with the ASCII fallback.
	var buf bytes.Buffer
	s := style.Resolve(&buf, "powerline", nil, true)

	if s.Glyphs != "ascii" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "ascii")
	}
	if got := s.Icons["repo_root"]; got != "[root]" {
		t.Errorf("Icons[\"repo_root\"]: got %q, want %q", got, "[root]")
	}
}

func TestResolve_ForceASCII_EmojiStyle(t *testing.T) {
	// emoji style with forceASCII=true must also use ASCII table.
	var buf bytes.Buffer
	s := style.Resolve(&buf, "emoji", nil, true)

	if s.Glyphs != "ascii" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "ascii")
	}
	if got := s.Icons["ai"]; got != "[ai]" {
		t.Errorf("Icons[\"ai\"]: got %q, want %q", got, "[ai]")
	}
}

// ── Resolve: nil userStyles is safe ──────────────────────────────────────────

func TestResolve_NilUserStyles_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	// Must not panic with nil map.
	s := style.Resolve(&buf, "emoji", nil, false)
	if s.Separator == "" {
		t.Error("resolved style has empty Separator; expected non-empty")
	}
}

// ── Resolve: empty userStyles map ────────────────────────────────────────────

func TestResolve_EmptyUserStyles(t *testing.T) {
	var buf bytes.Buffer
	s := style.Resolve(&buf, "powerline", map[string]style.Style{}, false)
	if s.Separator != "powerline" {
		t.Errorf("Separator: got %q, want %q", s.Separator, "powerline")
	}
}

// ── ResolveConfig: fill-presence-aware merge ─────────────────────────────────

// TestResolveConfig_FillPresence_NofillKeyKeepsBuiltinFill proves that a user
// style override that does NOT contain the "fill" key does NOT overwrite the
// builtin's fill=true. This is the CP2 Minor #3 fix.
func TestResolveConfig_FillPresence_NofillKeyKeepsBuiltinFill(t *testing.T) {
	// powerline has fill:true.
	// User override changes only separator — no "fill" key at all.
	raw := map[string]map[string]any{
		"powerline": {
			"separator": "space",
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, false)
	if s.Separator != "space" {
		t.Errorf("Separator: got %q, want %q", s.Separator, "space")
	}
	// fill:true from builtin must be preserved because raw had no "fill" key.
	if !s.Fill {
		t.Error("Fill: got false, want true — user override without 'fill' key must not clobber builtin's fill:true")
	}
	// Icons from builtin must still be present.
	if _, found := s.Icons["branch"]; !found {
		t.Error("Icons[\"branch\"] lost after fill-presence-aware override")
	}
}

// TestResolveConfig_FillPresence_ExplicitFalseOverrides proves that when the
// user explicitly sets fill:false, it DOES override the builtin's fill:true.
func TestResolveConfig_FillPresence_ExplicitFalseOverrides(t *testing.T) {
	raw := map[string]map[string]any{
		"powerline": {
			"fill": false,
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, false)
	if s.Fill {
		t.Error("Fill: got true, want false — explicit fill:false in user override must take effect")
	}
}

// TestResolveConfig_EmptyRaw exercises ResolveConfig with no user overrides.
func TestResolveConfig_EmptyRaw(t *testing.T) {
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", nil, false)
	if s.Separator != "powerline" {
		t.Errorf("Separator: got %q, want %q", s.Separator, "powerline")
	}
	if !s.Fill {
		t.Error("Fill: got false, want true")
	}
}

// ── ASCII user icon override in ascii mode ────────────────────────────────────

func TestResolve_ASCIIMode_UserCanOverrideASCIIIcon(t *testing.T) {
	// A user may want custom ASCII labels even in ASCII mode.
	user := map[string]style.Style{
		"powerline": {
			Glyphs: "ascii",
			Icons:  map[string]string{"ai": "(AI)"},
		},
	}
	var buf bytes.Buffer
	s := style.Resolve(&buf, "powerline", user, false)

	if got := s.Icons["ai"]; got != "(AI)" {
		t.Errorf("Icons[\"ai\"]: got %q, want %q", got, "(AI)")
	}
	// Other ASCII defaults still present.
	if got := s.Icons["repo_root"]; got != "[root]" {
		t.Errorf("Icons[\"repo_root\"]: got %q, want %q", got, "[root]")
	}
}
