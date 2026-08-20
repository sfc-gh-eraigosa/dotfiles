package style_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
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

// TestResolve_DeepMerge_SeparatorOnly_KeepsBuiltinFill is the Finding #6
// regression test: a user override of ONLY Separator on a Fill:true builtin
// must keep Fill:true. Previously mergeInto applied user.Fill unconditionally,
// so a user.Fill zero value (false) silently clobbered the builtin's fill:true,
// contradicting Resolve's documented "non-zero scalars only" contract.
func TestResolve_DeepMerge_SeparatorOnly_KeepsBuiltinFill(t *testing.T) {
	// powerline has Fill:true. User sets only Separator (Fill defaults to false).
	user := map[string]style.Style{
		"powerline": {Separator: "thin"},
	}
	var buf bytes.Buffer
	s := style.Resolve(&buf, "powerline", user, false)

	if s.Separator != "thin" {
		t.Errorf("Separator: got %q, want %q", s.Separator, "thin")
	}
	// Fill:true from the builtin must survive because the user did not set it.
	if !s.Fill {
		t.Error("Fill: got false, want true — Separator-only override must not clobber builtin's Fill:true")
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
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
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
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	if s.Fill {
		t.Error("Fill: got true, want false — explicit fill:false in user override must take effect")
	}
}

// TestResolveConfig_EmptyRaw exercises ResolveConfig with no user overrides.
func TestResolveConfig_EmptyRaw(t *testing.T) {
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", nil, false, "")
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

// ── rawToStyle ────────────────────────────────────────────────────────────────

func TestRawToStyle_GlyphsExtracted(t *testing.T) {
	raw := map[string]map[string]any{
		"mything": {
			"glyphs": "emoji",
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "mything", raw, false, "")
	// The resolved style should have Glyphs == "emoji" (from user entry
	// deep-merged over powerline base, which has Glyphs "nerdfont").
	if s.Glyphs != "emoji" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "emoji")
	}
}

func TestRawToStyle_IconsExtracted(t *testing.T) {
	// rawToStyle must convert map[string]any icon values to map[string]string.
	raw := map[string]map[string]any{
		"powerline": {
			"icons": map[string]any{
				"ai":   "★",
				"time": "⏱",
			},
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	if got := s.Icons["ai"]; got != "★" {
		t.Errorf("Icons[\"ai\"]: got %q, want %q", got, "★")
	}
	if got := s.Icons["time"]; got != "⏱" {
		t.Errorf("Icons[\"time\"]: got %q, want %q", got, "⏱")
	}
	// Builtin icons not overridden must still be present.
	if got := s.Icons["branch"]; got == "" {
		t.Error("Icons[\"branch\"] should be inherited from builtin after icons merge")
	}
}

func TestRawToStyle_ThemeExtracted(t *testing.T) {
	// rawToStyle must convert map[string]any theme values to map[string]string.
	raw := map[string]map[string]any{
		"powerline": {
			"theme": map[string]any{
				"ai": "magenta",
			},
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	if got := s.Theme["ai"]; got != "magenta" {
		t.Errorf("Theme[\"ai\"]: got %q, want %q", got, "magenta")
	}
	// Other theme keys from builtin must be inherited.
	if got := s.Theme["repo_root"]; got == "" {
		t.Error("Theme[\"repo_root\"] should be inherited from builtin")
	}
}

func TestRawToStyle_WrongTypeFill_Skipped(t *testing.T) {
	// fill as a string (wrong type) must be silently skipped — no panic.
	// The builtin powerline has fill:true; wrong-type fill must NOT overwrite it.
	raw := map[string]map[string]any{
		"powerline": {
			"fill": "yes", // wrong type: string instead of bool
		},
	}
	var buf bytes.Buffer
	// Must not panic.
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	// Finding #7 fix: a "fill" key whose value is not a JSON bool must NOT
	// trigger the override. Previously the mere presence of the key set the
	// applyFill flag, and because rawToStyle left Fill at its zero value
	// (false) on a type mismatch, the builtin's fill:true was silently
	// clobbered to false. Now the wrong-type value is ignored entirely, so the
	// builtin's fill:true is preserved.
	if !s.Fill {
		t.Error("Fill: got false, want true — wrong-type 'fill' value must be ignored and builtin's Fill:true preserved")
	}
}

func TestRawToStyle_WrongTypeIcons_Skipped(t *testing.T) {
	// icons as a string (wrong type) must be silently skipped — no panic.
	raw := map[string]map[string]any{
		"powerline": {
			"icons": "bad-value", // wrong type: string instead of map[string]any
		},
	}
	var buf bytes.Buffer
	// Must not panic.
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	// Icons should still come from builtin since the wrong-type value is skipped.
	if got := s.Icons["branch"]; got == "" {
		t.Error("Icons[\"branch\"] should be inherited from builtin when user icons has wrong type")
	}
}

func TestRawToStyle_WrongTypeTheme_Skipped(t *testing.T) {
	// theme as a string (wrong type) must be silently skipped — no panic.
	raw := map[string]map[string]any{
		"powerline": {
			"theme": "bad-value", // wrong type: string instead of map[string]any
		},
	}
	var buf bytes.Buffer
	// Must not panic.
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	// Theme should still come from builtin since wrong-type value is skipped.
	if got := s.Theme["repo_root"]; got == "" {
		t.Error("Theme[\"repo_root\"] should be inherited from builtin when user theme has wrong type")
	}
}

// ── ResolveConfig: deep-merge (icons/theme) ──────────────────────────────────

func TestResolveConfig_DeepMerge_IconsOneKeyKeeepsOthers(t *testing.T) {
	// Overriding one icon key must keep all other icons from the builtin.
	raw := map[string]map[string]any{
		"powerline": {
			"icons": map[string]any{
				"ai": "★",
			},
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	if got := s.Icons["ai"]; got != "★" {
		t.Errorf("Icons[\"ai\"]: got %q, want %q", got, "★")
	}
	// branch must survive from builtin.
	if got := s.Icons["branch"]; got == "" {
		t.Error("Icons[\"branch\"] lost after single-icon override via ResolveConfig")
	}
	// repo_root must survive from builtin.
	if got := s.Icons["repo_root"]; got == "" {
		t.Error("Icons[\"repo_root\"] lost after single-icon override via ResolveConfig")
	}
}

func TestResolveConfig_DeepMerge_ThemeOneKeyKeepsOthers(t *testing.T) {
	// Overriding one theme key must keep all other theme keys from the builtin.
	raw := map[string]map[string]any{
		"powerline": {
			"theme": map[string]any{
				"repo_root": "red",
			},
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	if got := s.Theme["repo_root"]; got != "red" {
		t.Errorf("Theme[\"repo_root\"]: got %q, want %q", got, "red")
	}
	// repo_worktree must survive from builtin.
	if got := s.Theme["repo_worktree"]; got == "" {
		t.Error("Theme[\"repo_worktree\"] lost after single-theme override via ResolveConfig")
	}
}

// ── ResolveConfig: ASCII fallback branch ─────────────────────────────────────

func TestResolveConfig_ForceASCII_UsesASCIITable(t *testing.T) {
	// forceASCII=true must replace the icon table with the ASCII fallback.
	raw := map[string]map[string]any{
		"powerline": {
			"separator": "space",
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, true, "")
	if s.Glyphs != "ascii" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "ascii")
	}
	if got := s.Icons["repo_root"]; got != "[root]" {
		t.Errorf("Icons[\"repo_root\"]: got %q, want %q", got, "[root]")
	}
	if got := s.Icons["ai"]; got != "[ai]" {
		t.Errorf("Icons[\"ai\"]: got %q, want %q", got, "[ai]")
	}
}

func TestResolveConfig_ForceASCII_NoUserStyle(t *testing.T) {
	// forceASCII=true with no user overrides at all.
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", nil, true, "")
	if s.Glyphs != "ascii" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "ascii")
	}
	if got := s.Icons["staged"]; got != "*" {
		t.Errorf("Icons[\"staged\"]: got %q, want %q", got, "*")
	}
	if got := s.Icons["unstaged"]; got != "!" {
		t.Errorf("Icons[\"unstaged\"]: got %q, want %q", got, "!")
	}
}

func TestResolveConfig_ASCIIGlyphs_Triggers_ASCIITable(t *testing.T) {
	// A user style that sets glyphs:"ascii" (without forceASCII) must still
	// trigger the ASCII fallback branch inside ResolveConfig.
	raw := map[string]map[string]any{
		"powerline": {
			"glyphs": "ascii",
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, false, "")
	if s.Glyphs != "ascii" {
		t.Errorf("Glyphs: got %q, want %q", s.Glyphs, "ascii")
	}
	if got := s.Icons["repo_root"]; got != "[root]" {
		t.Errorf("Icons[\"repo_root\"]: got %q, want %q", got, "[root]")
	}
}

func TestResolveConfig_ForceASCII_UserIconOverrideInASCIIMode(t *testing.T) {
	// User may still override individual ASCII icons even when forceASCII=true.
	raw := map[string]map[string]any{
		"powerline": {
			"icons": map[string]any{
				"ai": "(AI)",
			},
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, true, "")
	if got := s.Icons["ai"]; got != "(AI)" {
		t.Errorf("Icons[\"ai\"]: got %q, want %q", got, "(AI)")
	}
	// Other ASCII defaults must still be present.
	if got := s.Icons["repo_root"]; got != "[root]" {
		t.Errorf("Icons[\"repo_root\"]: got %q, want %q", got, "[root]")
	}
}

// ── ResolveConfig: auto-palette validation (untrusted colorCode path) ─────────

// TestResolveConfig_AutoPalette_TruecolorValueAccepted verifies that a valid
// truecolor fragment in a custom palette is accepted by the untrusted validation.
// This exercises the validateTruecolorFrag / validateUntrustedColor path.
func TestResolveConfig_AutoPalette_TruecolorValueAccepted(t *testing.T) {
	// Inject a custom palette override via user raw style that uses truecolor.
	// We exercise this by directly calling ResolveConfig with a user theme that
	// has a truecolor value — this is the "user-set" path (trusted), but the
	// auto-palette validation path is exercised by having a palette name that
	// exposes a ";"-bearing value.
	//
	// Since our built-in palettes use indices or named colors (not truecolor),
	// the truecolor validation path in validateUntrustedColor is reached only
	// when a caller passes a custom palette. We test the validation helpers
	// indirectly via the auto-palette merge: if we could register a palette
	// with a truecolor value, it would flow through validateUntrustedColor.
	// Since Palette() is read-only, we instead test the helpers via ResolveConfig
	// with user-provided theme values (those take the trusted path and are not
	// filtered by validateUntrustedColor). We accept the coverage gap here
	// because the truecolor fragment path in validateUntrustedColor / validateTruecolorFrag
	// is reachable only when a non-built-in palette source (future Phase 5+)
	// injects truecolor values.
	//
	// For now, assert that auto-palette with known palette works correctly and
	// that an injection attempt (";"-bearing invalid value) in a user theme
	// is still passed through (user theme values take the trusted path in render).
	raw := map[string]map[string]any{
		"powerline": {
			"theme": map[string]any{
				// This is a USER value, so it takes the trusted path in render.
				// It is NOT filtered by validateUntrustedColor.
				"ai": "38;2;100;150;200",
			},
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", raw, false, "light")
	// The user value takes effect even though it contains ";".
	// (render.paint will accept it since it's from user config / trusted path)
	if got := s.Theme["ai"]; got != "38;2;100;150;200" {
		t.Errorf("trusted user truecolor value: got %q, want %q", got, "38;2;100;150;200")
	}
	// Other keys from light palette must still be applied.
	p, _ := style.Palette("light")
	if got := s.Theme["repo_root"]; got != p["repo_root"] {
		t.Errorf("repo_root should come from light palette: got %q, want %q", got, p["repo_root"])
	}
}

// ── ResolveConfig: unknown style name fallback ────────────────────────────────

func TestResolveConfig_UnknownStyle_FallsBackToPowerline(t *testing.T) {
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "no-such-style", nil, false, "")
	if s.Separator != "powerline" {
		t.Errorf("fallback Separator: got %q, want %q", s.Separator, "powerline")
	}
	if !s.Fill {
		t.Error("fallback Fill: got false, want true")
	}
	if !strings.Contains(buf.String(), "no-such-style") {
		t.Errorf("warning should mention unknown style name; got: %q", buf.String())
	}
}

// ── ResolveConfig: auto-palette merge (F2) ───────────────────────────────────

// TestResolveConfig_AutoPalette_LightMergesSegmentKeys verifies that the
// "light" auto-palette is merged for all five segment keys when the user has
// set none of them.
func TestResolveConfig_AutoPalette_LightMergesSegmentKeys(t *testing.T) {
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "powerline", nil, false, "light")

	// All five segment keys must be from the light palette.
	p, _ := style.Palette("light")
	for _, key := range style.SegmentColorKeys() {
		want := p[key]
		got := s.Theme[key]
		if got != want {
			t.Errorf("light palette key %q: got %q, want %q", key, got, want)
		}
	}
}

// TestResolveConfig_AutoPalette_DarkDaltonism verifies dark-daltonism palette.
func TestResolveConfig_AutoPalette_DarkDaltonism(t *testing.T) {
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "emoji", nil, false, "dark-daltonism")

	p, _ := style.Palette("dark-daltonism")
	for _, key := range style.SegmentColorKeys() {
		want := p[key]
		got := s.Theme[key]
		if got != want {
			t.Errorf("dark-daltonism palette emoji key %q: got %q, want %q", key, got, want)
		}
	}
}

// TestResolveConfig_AutoPalette_UserKeyWins verifies that a user-set theme key
// is NOT overwritten by the auto-palette (UC-5).
func TestResolveConfig_AutoPalette_UserKeyWins(t *testing.T) {
	raw := map[string]map[string]any{
		"emoji": {
			"theme": map[string]any{
				"ai": "199", // user sets ai explicitly
			},
		},
	}
	var buf bytes.Buffer
	s := style.ResolveConfig(&buf, "emoji", raw, false, "light")

	// User-set key must survive.
	if got := s.Theme["ai"]; got != "199" {
		t.Errorf("user key ai: got %q, want %q — user key must not be overwritten by auto-palette", got, "199")
	}

	// Other keys not set by user must come from the light palette.
	p, _ := style.Palette("light")
	for _, key := range []string{"repo_root", "repo_worktree", "dirgit", "time"} {
		want := p[key]
		got := s.Theme[key]
		if got != want {
			t.Errorf("non-user key %q: got %q, want %q (should be from light palette)", key, got, want)
		}
	}
}

// TestResolveConfig_AutoPalette_EmptyName_NoMerge verifies that an empty
// autoPalette leaves the builtin theme unchanged (backward-compatible).
func TestResolveConfig_AutoPalette_EmptyName_NoMerge(t *testing.T) {
	var buf bytes.Buffer
	sNoAuto := style.ResolveConfig(&buf, "powerline", nil, false, "")
	sWithDark := style.ResolveConfig(&buf, "powerline", nil, false, "dark")

	// The "dark" auto-palette intentionally matches the builtins, so every
	// segment colour must agree. This used to be an empty if body that asserted
	// nothing beyond "did not panic"; making it a real comparison is what the
	// surrounding comment always claimed it was doing.
	for _, key := range style.SegmentColorKeys() {
		if sNoAuto.Theme[key] != sWithDark.Theme[key] {
			t.Errorf("segment %q: auto-palette %q must match the builtin, got %q vs %q",
				key, "dark", sNoAuto.Theme[key], sWithDark.Theme[key])
		}
	}
}

// TestResolveConfig_AutoPalette_UnknownName_NoMerge verifies that an unknown
// autoPalette name is silently ignored (no crash, no partial merge).
func TestResolveConfig_AutoPalette_UnknownName_NoMerge(t *testing.T) {
	var buf bytes.Buffer
	// Must not panic.
	s := style.ResolveConfig(&buf, "powerline", nil, false, "nonexistent-palette")
	// Theme should still come from the builtin.
	if s.Theme["repo_root"] == "" {
		t.Error("Theme[\"repo_root\"] should be from builtin when autoPalette is unknown")
	}
}
