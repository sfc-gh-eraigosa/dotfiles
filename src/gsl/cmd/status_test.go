package cmd

import (
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/config"
)

// TestStatusCmd_NoStdin runs runStatus without touching stdin.
// The function must not panic and should produce output when the config is
// enabled. The AI segment should be absent (no payload).
func TestStatusCmd_NoStdin(t *testing.T) {
	cfg := config.Default()
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := runStatus(statusCmd, nil); err != nil {
				t.Errorf("runStatus: unexpected error: %v", err)
			}
		})
		// We can't assert hard content since the test may run outside a git repo,
		// but the function must succeed without panicking.
		_ = out
	})
}

// TestStatusCmd_MasterDisabled produces no output when master is off.
func TestStatusCmd_MasterDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Enabled = false
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := runStatus(statusCmd, nil); err != nil {
				t.Errorf("runStatus: unexpected error: %v", err)
			}
		})
		if strings.TrimSpace(out) != "" {
			t.Errorf("expected empty output when master disabled, got: %q", out)
		}
	})
}

// TestStatusCmd_NoAISegment verifies that the AI segment is NOT rendered by
// status because the payload is empty. We enable only the "ai" segment and
// verify that disabling it doesn't break anything, and also that rendering with
// an empty payload produces no AI-segment markers.
func TestStatusCmd_NoAISegment(t *testing.T) {
	// Config with ONLY the AI segment enabled; with empty payload it should
	// produce empty output (the segment self-omits).
	cfg := config.Config{
		Enabled: true,
		Segments: []config.Segment{
			{Type: "ai", Enabled: true},
		},
		Timezone:   "UTC",
		TimeFormat: "15:04:05",
		DateFormat: "2006-01-02",
		Style:      "powerline",
	}
	withTempConfig(t, cfg, func() {
		out := captureStdout(t, func() {
			if err := runStatus(statusCmd, nil); err != nil {
				t.Errorf("runStatus: unexpected error: %v", err)
			}
		})
		// The AI segment self-omits with empty payload → output should be empty.
		if strings.TrimSpace(out) != "" {
			t.Errorf("expected empty output (AI segment self-omits), got: %q", out)
		}
	})
}
