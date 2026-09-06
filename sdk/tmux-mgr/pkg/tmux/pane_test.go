package tmux

import (
	"os"
	"strings"
	"testing"
)

func TestRootPaneID_fromEnv(t *testing.T) {
	t.Setenv("TMUX_PANE", "%99")
	id, err := RootPaneID()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if id != "%99" {
		t.Errorf("expected %%99, got %s", id)
	}
}

func TestRootPaneID_noEnv_noAnchor(t *testing.T) {
	if err := os.Unsetenv("TMUX_PANE"); err != nil {
		t.Fatalf("unset TMUX_PANE: %v", err)
	}
	// When there is no tmux server (CI), show-environment will also fail.
	_, err := RootPaneID()
	if err == nil {
		t.Skip("tmux server is running with TMUX_MGR_ROOT_PANE set; skipping no-anchor test")
	}
	if !strings.Contains(err.Error(), "pane anchor") {
		t.Errorf("expected error to mention 'pane anchor', got: %v", err)
	}
}

func TestAnchorPane_noEnv(t *testing.T) {
	if err := os.Unsetenv("TMUX_PANE"); err != nil {
		t.Fatalf("unset TMUX_PANE: %v", err)
	}
	_, err := AnchorPane("")
	if err == nil {
		t.Fatal("expected error when TMUX_PANE is unset")
	}
	if !strings.Contains(err.Error(), "$TMUX_PANE is not set") {
		t.Errorf("expected error about $TMUX_PANE, got: %v", err)
	}
}

func TestAnchorPane_withEnv(t *testing.T) {
	t.Setenv("TMUX_PANE", "%55")
	// AnchorPane will try to exec tmux — in CI without tmux this errors on the
	// set-environment call. We only care that it does NOT fail the TMUX_PANE guard.
	_, err := AnchorPane("test-root")
	if err != nil && strings.Contains(err.Error(), "$TMUX_PANE is not set") {
		t.Errorf("should not fail the TMUX_PANE guard when env is set, got: %v", err)
	}
}

func TestAdoptPane_errorShape(t *testing.T) {
	// AdoptPane must return a recognizable error when tmux is absent or fails.
	_, err := AdoptPane()
	if err != nil {
		for _, needle := range []string{"tmux server", "pane ID", "anchor", "window"} {
			if strings.Contains(err.Error(), needle) {
				return
			}
		}
		t.Errorf("unexpected error message shape: %v", err)
	}
}
