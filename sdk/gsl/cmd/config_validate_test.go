package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
)

func TestValidateConfig_DefaultIsClean(t *testing.T) {
	if p := validateConfig(config.Default()); len(p) != 0 {
		t.Fatalf("default config has problems: %v", p)
	}
}

// Each kind of mistake is reported with the key it is under.
func TestValidateConfig_FindsProblems(t *testing.T) {
	cfg := config.Default()
	cfg.Style = "neon"
	cfg.Links = "sometimes"
	cfg.Timezone = "Mars/Olympus_Mons"
	cfg.FallbackColumns = -3
	cfg.Segments = append(cfg.Segments, config.Segment{Type: "weather", Enabled: true}, config.Segment{Type: "mcp", Enabled: true}, config.Segment{Type: "ai", Enabled: true})
	cfg.Styles = map[string]any{
		"powerline": map[string]any{
			"separator": "zigzag",
			"glyphs":    "hieroglyphs",
			"theme":     map[string]any{"repo_root": "nope", "dirgit": "#00ff00", "time": "\x1b[31m"},
		},
	}
	got := strings.Join(validateConfig(cfg), "\n")
	for _, want := range []string{
		`style: "neon"`, `links: "sometimes"`, `timezone: "Mars/Olympus_Mons"`, "fallback_columns: -3",
		`segments: unknown type "weather"`, `segments: "mcp" listed twice`, `segments: "ai" is now [model context mcp limits]`,
		`styles.powerline.separator: "zigzag"`, `styles.powerline.glyphs: "hieroglyphs"`,
		`styles.powerline.theme.repo_root: "nope"`, `styles.powerline.theme.time`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "theme.dirgit") {
		t.Errorf("a hex colour is valid:\n%s", got)
	}
}

// A user-defined style name is a valid `style`.
func TestValidateConfig_UserStyleName(t *testing.T) {
	cfg := config.Default()
	cfg.Style = "mine"
	cfg.Styles = map[string]any{"mine": map[string]any{"separator": "thin"}}
	if p := validateConfig(cfg); len(p) != 0 {
		t.Fatalf("user style rejected: %v", p)
	}
}

// The verb: non-zero (an error) on problems, OK otherwise; and `config set
// style` refuses a name that is neither builtin nor user-defined.
func TestConfigValidateCmd(t *testing.T) {
	bad := config.Default()
	bad.Links = "sometimes"
	withTempConfig(t, bad, func() {
		if err := configValidateCmd.RunE(configValidateCmd, nil); err == nil {
			t.Error("validate passed a bad config")
		}
		if err := configSetCmd.RunE(configSetCmd, []string{"style", "neon"}); err == nil || !strings.Contains(err.Error(), "neon") {
			t.Errorf("set style neon: %v", err)
		}
	})
	withTempConfig(t, config.Default(), func() {
		out := captureStdout(t, func() {
			if err := configValidateCmd.RunE(configValidateCmd, nil); err != nil {
				t.Errorf("validate failed a default config: %v", err)
			}
		})
		if !strings.Contains(out, "OK") {
			t.Errorf("out = %q", out)
		}
	})
}

func TestPaletteFor(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cfg := config.Default()
	cfg.Palette = "light"
	home := t.TempDir()
	if got := paletteFor(cfg, "", env(map[string]string{"TERM": "xterm-256color"}), home); got != "light" {
		t.Errorf("config palette: %s", got)
	}
	if got := paletteFor(cfg, "", env(map[string]string{"SDK_THEME": "dark8"}), home); got != "dark8" {
		t.Errorf("SDK_THEME must win for the session: %s", got)
	}
	cfg.Palette = "neon"
	if got := paletteFor(cfg, "", env(map[string]string{"TERM": "xterm-256color"}), home); got != "dark" {
		t.Errorf("an unknown config palette falls through: %s", got)
	}
	if p := strings.Join(validateConfig(cfg), "\n"); !strings.Contains(p, `palette: "neon"`) {
		t.Errorf("validate missed the palette: %s", p)
	}
}

// A file that still lists "ai" validates OK (it migrates on load) but gets a
// note saying what it became and how to write the new form.
func TestConfigValidateCmd_LegacyAINote(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		raw := `{"enabled":true,"segments":[{"type":"dirgit","enabled":true},{"type":"ai","enabled":true}]}`
		if err := os.WriteFile(config.DefaultPath(), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		out := captureStdout(t, func() {
			if err := configValidateCmd.RunE(configValidateCmd, nil); err != nil {
				t.Errorf("a legacy ai entry is not an error: %v", err)
			}
		})
		if !strings.Contains(out, `note: segments lists "ai"`) || !strings.Contains(out, "model, context, mcp, limits") || !strings.Contains(out, "OK") {
			t.Errorf("out = %q", out)
		}
	})
}
