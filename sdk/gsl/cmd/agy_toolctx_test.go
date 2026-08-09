package cmd

// Regression tests for the agy render path.
//
// THE BUG: agy's settings.json points statusLine.command at a shim that `exec`s
// `gsl render` with the payload on stdin — exactly like Claude Code. agy's
// payload carries cwd + model + context_window, so deriveToolCtx's "payload is
// populated ⇒ claude" rule matched EVERY agy render and returned "claude".
// theme.Resolve therefore read ~/.claude/settings.json for Antigravity users and
// the whole `case "antigravity"` branch — including the colorScheme support that
// this leaf added — was DEAD CODE on the real agy path. The unit tests for it
// were green because they called theme.Resolve("antigravity", …) directly, a
// context production never produced.
//
// THE FIX: agy sends "product": "antigravity" in every payload (confirmed in the
// live capture, internal/payload/testdata/agy_live.json). deriveToolCtx now keys
// on that in-band discriminator first.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// agyLivePayload returns the REAL captured agy payload (shared fixture, so this
// test tracks the wire format rather than a hand-written idea of it).
func agyLivePayload(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "internal", "payload", "testdata", "agy_live.json"))
	if err != nil {
		t.Fatalf("reading the captured agy payload: %v", err)
	}
	return data
}

// TestDeriveToolCtx_AgyLivePayload — the real agy payload must resolve to the
// antigravity context, NOT claude. Pre-fix this returned "claude".
func TestDeriveToolCtx_AgyLivePayload(t *testing.T) {
	p, err := payload.Parse(agyLivePayload(t))
	if err != nil {
		t.Fatalf("payload.Parse: %v", err)
	}
	if p.Product == nil {
		t.Fatal("the captured payload has product:\"antigravity\" but Payload.Product is nil " +
			"(field not decoded in Parse)")
	}
	noEnv := func(string) string { return "" }
	if got := deriveToolCtx(p, noEnv); got != "antigravity" {
		t.Errorf("deriveToolCtx(real agy payload) = %q, want \"antigravity\" "+
			"(a populated agy payload was being misread as Claude, so the agy theme was never resolved)", got)
	}
}

// TestDeriveToolCtx_ProductKey pins the discriminator's edges.
func TestDeriveToolCtx_ProductKey(t *testing.T) {
	noEnv := func(string) string { return "" }
	cwd := "/tmp"

	tests := []struct {
		name    string
		product *string
		want    string
	}{
		{"product=antigravity wins over a claude-shaped payload", strptr("antigravity"), "antigravity"},
		{"product is case-insensitive", strptr("Antigravity"), "antigravity"},
		{"no product key -> claude (Claude Code sends none)", nil, "claude"},
		{"unknown product -> claude heuristic", strptr("something-else"), "claude"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := payload.Payload{Cwd: &cwd, Model: &payload.Model{}, Product: tc.product}
			if got := deriveToolCtx(p, noEnv); got != tc.want {
				t.Errorf("deriveToolCtx = %q, want %q", got, tc.want)
			}
		})
	}
}

func strptr(s string) *string { return &s }

// renderWithHomes runs the real render path over the real captured agy payload
// with a synthetic $HOME containing both settings files, and returns the raw
// (ANSI-bearing) output.
func renderWithHomes(t *testing.T, claudeTheme, agyColorScheme string) string {
	t.Helper()

	home := t.TempDir()
	mustWriteJSON(t, filepath.Join(home, ".claude", "settings.json"),
		map[string]any{"theme": claudeTheme})
	mustWriteJSON(t, filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"),
		map[string]any{"colorScheme": agyColorScheme})

	t.Setenv("HOME", home)
	t.Setenv("COLUMNS", "200") // wide: no compaction

	cfg := config.Default()
	cfg.Style = "powerline" // a palette-driven style, so the theme is observable
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	if err := config.Save(config.DefaultPath(), cfg); err != nil {
		t.Fatalf("setup: config.Save: %v", err)
	}

	p, err := payload.Parse(agyLivePayload(t))
	if err != nil {
		t.Fatalf("payload.Parse: %v", err)
	}
	return captureStdout(t, func() { _ = runStatusLine(renderCmd, p, "/tmp") })
}

// ansiCodes extracts just the SGR escape sequences from a rendered line. The
// line also carries a live clock, so the raw strings always differ; the PALETTE
// is what this test is about.
func ansiCodes(s string) string {
	return strings.Join(sgrRe.FindAllString(s, -1), "|")
}

var sgrRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func mustWriteJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestRunStatusLine_AgyThemeComesFromAntigravitySettings is the END-TO-END proof
// that the colorScheme fix reaches a real agy render. Two assertions, one pair:
//
//   - flipping the AGY colorScheme (claude settings held constant) MUST change
//     the rendered ANSI — the agy settings file is being read;
//   - flipping the CLAUDE theme (agy colorScheme held constant) MUST NOT change
//     it — the Claude settings file is NOT being read for an agy payload.
//
// Pre-fix both assertions failed (inverted): the agy payload resolved as
// "claude", so ~/.claude/settings.json drove the palette and the agy colorScheme
// was ignored entirely.
func TestRunStatusLine_AgyThemeComesFromAntigravitySettings(t *testing.T) {
	agyLight := ansiCodes(renderWithHomes(t, "dark", "light"))
	agyDark := ansiCodes(renderWithHomes(t, "dark", "dark"))
	if agyLight == "" || agyDark == "" {
		t.Fatal("no ANSI codes in the render output")
	}
	if agyLight == agyDark {
		t.Errorf("flipping the agy colorScheme did not change the palette (%s)\n"+
			"the Antigravity settings file is not reaching the render path", agyLight)
	}

	claudeDark := ansiCodes(renderWithHomes(t, "dark", "light"))
	claudeLight := ansiCodes(renderWithHomes(t, "light", "light"))
	if claudeDark != claudeLight {
		t.Errorf("flipping the CLAUDE theme changed the palette of an AGY render:\n"+
			" claude=light -> %s\n claude=dark  -> %s\n"+
			"gsl is resolving the Claude theme for an Antigravity payload", claudeLight, claudeDark)
	}
}
