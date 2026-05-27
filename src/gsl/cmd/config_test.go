package cmd

import (
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/config"
)

// TestConfigEnable_Master tests enabling the master switch.
func TestConfigEnable_Master(t *testing.T) {
	cfg := config.Default()
	cfg.Enabled = false
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configEnableCmd.RunE(configEnableCmd, nil); err != nil {
				t.Fatalf("enable master: %v", err)
			}
		})
		if !strings.Contains(out, "enabled") {
			t.Errorf("expected 'enabled' in output, got: %q", out)
		}
		// Verify it was actually persisted.
		loaded, err := config.Load(config.DefaultPath())
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		if !loaded.Enabled {
			t.Error("config.Enabled should be true after enable")
		}
	})
}

// TestConfigDisable_Master tests disabling the master switch.
func TestConfigDisable_Master(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configDisableCmd.RunE(configDisableCmd, nil); err != nil {
				t.Fatalf("disable master: %v", err)
			}
		})
		if !strings.Contains(out, "disabled") {
			t.Errorf("expected 'disabled' in output, got: %q", out)
		}
		loaded, err := config.Load(config.DefaultPath())
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		if loaded.Enabled {
			t.Error("config.Enabled should be false after disable")
		}
	})
}

// TestConfigEnable_Segment enables a specific segment.
func TestConfigEnable_Segment(t *testing.T) {
	cfg := config.Default()
	// Disable the "ai" segment first.
	for i := range cfg.Segments {
		if cfg.Segments[i].Type == "ai" {
			cfg.Segments[i].Enabled = false
		}
	}
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configEnableCmd.RunE(configEnableCmd, []string{"ai"}); err != nil {
				t.Fatalf("enable segment ai: %v", err)
			}
		})
		if !strings.Contains(out, "ai") {
			t.Errorf("expected 'ai' in output, got: %q", out)
		}
		loaded, err := config.Load(config.DefaultPath())
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		for _, seg := range loaded.Segments {
			if seg.Type == "ai" && !seg.Enabled {
				t.Error("segment 'ai' should be enabled")
			}
		}
	})
}

// TestConfigDisable_Segment disables a specific segment.
func TestConfigDisable_Segment(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configDisableCmd.RunE(configDisableCmd, []string{"time"}); err != nil {
				t.Fatalf("disable segment time: %v", err)
			}
		})
		if !strings.Contains(out, "time") {
			t.Errorf("expected 'time' in output, got: %q", out)
		}
		loaded, err := config.Load(config.DefaultPath())
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		for _, seg := range loaded.Segments {
			if seg.Type == "time" && seg.Enabled {
				t.Error("segment 'time' should be disabled")
			}
		}
	})
}

// TestConfigToggle_Segment toggles a segment and verifies the round-trip.
func TestConfigToggle_Segment(t *testing.T) {
	cfg := config.Default()
	// "repo" starts enabled (Default).
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configToggleCmd.RunE(configToggleCmd, []string{"repo"}); err != nil {
				t.Fatalf("toggle segment repo: %v", err)
			}
		})
		if !strings.Contains(out, "repo") {
			t.Errorf("expected 'repo' in output, got: %q", out)
		}
		loaded, err := config.Load(config.DefaultPath())
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		for _, seg := range loaded.Segments {
			if seg.Type == "repo" && seg.Enabled {
				t.Error("segment 'repo' should be disabled after toggle from enabled")
			}
		}

		// Toggle back.
		captureStdout(t, func() {
			_ = configToggleCmd.RunE(configToggleCmd, []string{"repo"})
		})
		loaded2, _ := config.Load(config.DefaultPath())
		for _, seg := range loaded2.Segments {
			if seg.Type == "repo" && !seg.Enabled {
				t.Error("segment 'repo' should be re-enabled after second toggle")
			}
		}
	})
}

// TestConfigStyleList shows builtins and marks the active one.
func TestConfigStyleList(t *testing.T) {
	cfg := config.Default()
	cfg.Style = "emoji"
	withTempConfig(t, cfg, func() {
		// Set the --list flag on the style subcommand.
		configStyleListFlag = true
		defer func() { configStyleListFlag = false }()

		out := captureStdout(t, func() {
			if err := configStyleCmd.RunE(configStyleCmd, nil); err != nil {
				t.Fatalf("config style --list: %v", err)
			}
		})
		// Should list both builtins.
		if !strings.Contains(out, "powerline") {
			t.Errorf("missing 'powerline' in style list: %q", out)
		}
		if !strings.Contains(out, "emoji") {
			t.Errorf("missing 'emoji' in style list: %q", out)
		}
		// Should mark the active style with *.
		if !strings.Contains(out, "* ") {
			t.Errorf("missing active marker '*' in style list: %q", out)
		}
		// The active style line should start with '* '.
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "* ") && !strings.Contains(line, "emoji") {
				t.Errorf("active marker on wrong style: %q", line)
			}
		}
	})
}

// TestConfigStyleSet changes the style and verifies persistence.
func TestConfigStyleSet(t *testing.T) {
	cfg := config.Default()
	cfg.Style = "powerline"
	withTempConfig(t, cfg, func() {
		configStyleListFlag = false
		out := captureStdout(t, func() {
			if err := configStyleCmd.RunE(configStyleCmd, []string{"emoji"}); err != nil {
				t.Fatalf("config style emoji: %v", err)
			}
		})
		if !strings.Contains(out, "emoji") {
			t.Errorf("expected 'emoji' in output, got: %q", out)
		}
		loaded, err := config.Load(config.DefaultPath())
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		if loaded.Style != "emoji" {
			t.Errorf("Style: got %q, want %q", loaded.Style, "emoji")
		}
	})
}

// TestConfigGet_AllKeys prints the full config without error.
func TestConfigGet_AllKeys(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configGetCmd.RunE(configGetCmd, nil); err != nil {
				t.Fatalf("config get: %v", err)
			}
		})
		if !strings.Contains(out, "enabled") {
			t.Errorf("config get output missing 'enabled': %q", out)
		}
	})
}

// TestConfigGet_SpecificKey prints a specific field.
func TestConfigGet_SpecificKey(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configGetCmd.RunE(configGetCmd, []string{"style"}); err != nil {
				t.Fatalf("config get style: %v", err)
			}
		})
		if !strings.Contains(out, "powerline") {
			t.Errorf("expected 'powerline' in output, got: %q", out)
		}
	})
}

// TestConfigGet_UnknownKey returns an error for unknown keys.
func TestConfigGet_UnknownKey(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		err := configGetCmd.RunE(configGetCmd, []string{"nosuchkey"})
		if err == nil {
			t.Error("expected error for unknown key, got nil")
		}
	})
}

// TestConfigGet_EachField checks each known key is printable.
func TestConfigGet_EachField(t *testing.T) {
	cfg := config.Default()
	keys := []string{"enabled", "timezone", "time_format", "date_format", "segments"}
	withTempConfig(t, cfg, func() {
		for _, k := range keys {
			out := captureStdout(t, func() {
				if err := configGetCmd.RunE(configGetCmd, []string{k}); err != nil {
					t.Errorf("config get %q: %v", k, err)
				}
			})
			if strings.TrimSpace(out) == "" {
				t.Errorf("config get %q: empty output", k)
			}
		}
	})
}

// TestConfigSet_Timezone sets the timezone field.
func TestConfigSet_Timezone(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configSetCmd.RunE(configSetCmd, []string{"timezone", "UTC"}); err != nil {
				t.Fatalf("config set timezone UTC: %v", err)
			}
		})
		if !strings.Contains(out, "timezone") {
			t.Errorf("expected 'timezone' in output, got: %q", out)
		}
	})
}

// TestConfigSet_TimeFormat sets the time_format field.
func TestConfigSet_TimeFormat(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		captureStdout(t, func() {
			if err := configSetCmd.RunE(configSetCmd, []string{"time_format", "15:04"}); err != nil {
				t.Fatalf("config set time_format: %v", err)
			}
		})
		loaded, _ := config.Load(config.DefaultPath())
		if loaded.TimeFormat != "15:04" {
			t.Errorf("TimeFormat: got %q, want %q", loaded.TimeFormat, "15:04")
		}
	})
}

// TestConfigSet_DateFormat sets the date_format field.
func TestConfigSet_DateFormat(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		captureStdout(t, func() {
			if err := configSetCmd.RunE(configSetCmd, []string{"date_format", "01/02/2006"}); err != nil {
				t.Fatalf("config set date_format: %v", err)
			}
		})
		loaded, _ := config.Load(config.DefaultPath())
		if loaded.DateFormat != "01/02/2006" {
			t.Errorf("DateFormat: got %q, want %q", loaded.DateFormat, "01/02/2006")
		}
	})
}

// TestConfigSet_UnknownKey returns an error.
func TestConfigSet_UnknownKey(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		err := configSetCmd.RunE(configSetCmd, []string{"badkey", "value"})
		if err == nil {
			t.Error("expected error for unknown key, got nil")
		}
	})
}

// TestConfigEnable_UnknownSegment returns an error.
func TestConfigEnable_UnknownSegment(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		err := configEnableCmd.RunE(configEnableCmd, []string{"nosuchsegment"})
		if err == nil {
			t.Error("expected error for unknown segment, got nil")
		}
	})
}

// TestConfigDisable_UnknownSegment returns an error.
func TestConfigDisable_UnknownSegment(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		err := configDisableCmd.RunE(configDisableCmd, []string{"nosuchsegment"})
		if err == nil {
			t.Error("expected error for unknown segment, got nil")
		}
	})
}

// TestConfigToggle_UnknownSegment returns an error.
func TestConfigToggle_UnknownSegment(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		err := configToggleCmd.RunE(configToggleCmd, []string{"nosuchsegment"})
		if err == nil {
			t.Error("expected error for unknown segment, got nil")
		}
	})
}

// TestConfigStyleList_WithUserStyle adds a user style to config.Styles and
// verifies it appears in the list output.
func TestConfigStyleList_WithUserStyle(t *testing.T) {
	cfg := config.Default()
	cfg.Style = "powerline"
	cfg.Styles = map[string]any{
		"mytheme": map[string]any{"separator": "space"},
	}
	withTempConfig(t, cfg, func() {
		configStyleListFlag = true
		defer func() { configStyleListFlag = false }()

		out := captureStdout(t, func() {
			if err := configStyleCmd.RunE(configStyleCmd, nil); err != nil {
				t.Fatalf("config style --list with user style: %v", err)
			}
		})
		if !strings.Contains(out, "mytheme") {
			t.Errorf("expected user style 'mytheme' in output, got: %q", out)
		}
		if !strings.Contains(out, "user") {
			t.Errorf("expected 'user' label for user style, got: %q", out)
		}
	})
}

// TestConfigSet_Style changes a setting via config set.
func TestConfigSet_Style(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := configSetCmd.RunE(configSetCmd, []string{"style", "emoji"}); err != nil {
				t.Fatalf("config set style emoji: %v", err)
			}
		})
		if !strings.Contains(out, "style") {
			t.Errorf("expected 'style' in output, got: %q", out)
		}
		loaded, _ := config.Load(config.DefaultPath())
		if loaded.Style != "emoji" {
			t.Errorf("Style: got %q, want %q", loaded.Style, "emoji")
		}
	})
}
