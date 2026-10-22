package cmd

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/spf13/cobra"
)

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check config.json and report every problem (exit 1 if any)",
	Long: `validate lints ~/.config/gsl/config.json: the style name, links, timezone,
segments, fallback_columns, and every user style override (separator, glyphs,
and each theme colour — a name, 0-255, #rrggbb or 38;5;N / 38;2;r;g;b).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(config.DefaultPath())
		if err != nil {
			return err
		}
		problems := validateConfig(cfg)
		if len(problems) == 0 {
			fmt.Printf("OK: %s\n", config.DefaultPath())
			return nil
		}
		for _, p := range problems {
			fmt.Println(p)
		}
		return fmt.Errorf("gsl config validate: %d problem(s) in %s", len(problems), config.DefaultPath())
	},
}

// validateConfig returns one line per problem, "<key>: <value> — <why>".
func validateConfig(cfg config.Config) []string {
	var out []string
	bad := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }

	if err := knownStyle(cfg, cfg.Style); err != nil {
		bad("style: %q — %v", cfg.Style, err)
	}
	if cfg.Palette != "" {
		if _, ok := style.Palette(cfg.Palette); !ok {
			bad("palette: %q — want dark, light, dark8 or dark-daltonism (or unset for auto)", cfg.Palette)
		}
	}
	switch cfg.Links {
	case "", config.LinksUnderline, config.LinksPlain, config.LinksOff:
	default:
		bad("links: %q — want underline, plain or off", cfg.Links)
	}
	if cfg.Timezone != "" {
		if _, err := time.LoadLocation(cfg.Timezone); err != nil {
			bad("timezone: %q — not an IANA zone", cfg.Timezone)
		}
	}
	if cfg.FallbackColumns < 0 {
		bad("fallback_columns: %d — must be 0 (default) or more", cfg.FallbackColumns)
	}
	seen := map[string]bool{}
	for _, s := range cfg.Segments {
		if !slices.Contains(config.KnownSegmentTypes, s.Type) {
			bad("segments: unknown type %q — known: %v", s.Type, config.KnownSegmentTypes)
		}
		if seen[s.Type] {
			bad("segments: %q listed twice", s.Type)
		}
		seen[s.Type] = true
	}

	names := make([]string, 0, len(cfg.Styles))
	for n := range cfg.Styles {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		m, ok := cfg.Styles[n].(map[string]any)
		if !ok {
			bad("styles.%s: not an object", n)
			continue
		}
		if v, ok := m["separator"].(string); ok && !slices.Contains([]string{"powerline", "thin", "space"}, v) {
			bad("styles.%s.separator: %q — want powerline, thin or space", n, v)
		}
		if v, ok := m["glyphs"].(string); ok && !slices.Contains([]string{"nerdfont", "emoji", "ascii"}, v) {
			bad("styles.%s.glyphs: %q — want nerdfont, emoji or ascii", n, v)
		}
		theme, _ := m["theme"].(map[string]any)
		keys := make([]string, 0, len(theme))
		for k := range theme {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, isString := theme[k].(string)
			if _, ok := style.ParseColor(v); !isString || !ok {
				bad("styles.%s.theme.%s: %q — not a colour (a name, 0-255, #rrggbb, 38;5;N or 38;2;r;g;b)", n, k, theme[k])
			}
		}
	}
	return out
}

// knownStyle is nil when name is a builtin style or one the config defines.
func knownStyle(cfg config.Config, name string) error {
	if _, ok := style.Builtin(name); ok {
		return nil
	}
	if _, ok := cfg.Styles[name]; ok {
		return nil
	}
	builtins := make([]string, 0)
	for n := range style.Builtins() {
		builtins = append(builtins, n)
	}
	sort.Strings(builtins)
	return errors.New("not a builtin style (" + fmt.Sprint(builtins) + ") or one defined under styles")
}

func init() { configCmd.AddCommand(configValidateCmd) }
