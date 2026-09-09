package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RootPaneID returns the pane ID to use as the target for splits and spawns.
// Priority: $TMUX_PANE (process is inside tmux) → tmux global env
// TMUX_MGR_ROOT_PANE (set via AnchorPane/AdoptPane) → error.
func RootPaneID() (string, error) {
	if id := os.Getenv("TMUX_PANE"); id != "" {
		return id, nil
	}
	out, err := exec.Command("tmux", "show-environment", "-g", "TMUX_MGR_ROOT_PANE").Output()
	if err == nil {
		// Output format: "TMUX_MGR_ROOT_PANE=%42"
		line := strings.TrimSpace(string(out))
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 && parts[1] != "" {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("not running inside a tmux pane and no anchor established — run 'tmux-mgr pane anchor' from your terminal first")
}

// AnchorPane marks the current $TMUX_PANE as the orchestration root by saving
// its ID to the tmux global environment under TMUX_MGR_ROOT_PANE and renaming
// the pane. title defaults to "root". Must be called from inside a tmux pane.
func AnchorPane(title string) (string, error) {
	paneID := os.Getenv("TMUX_PANE")
	if paneID == "" {
		return "", fmt.Errorf("not running inside a tmux pane — $TMUX_PANE is not set. Run 'tmux-mgr pane anchor' from your terminal, not from an AI process")
	}
	if title == "" {
		title = "root"
	}
	if err := exec.Command("tmux", "set-environment", "-g", "TMUX_MGR_ROOT_PANE", paneID).Run(); err != nil {
		return "", fmt.Errorf("failed to save anchor to tmux environment: %w", err)
	}
	// Cosmetic: the anchor above is the operation that had to succeed.
	_ = exec.Command("tmux", "set", "-g", "pane-border-status", "top").Run()
	_ = exec.Command("tmux", "select-pane", "-t", paneID, "-T", title).Run()
	return paneID, nil
}

// AdoptPane creates a new tmux window from outside tmux, registers its pane ID
// as the root anchor under TMUX_MGR_ROOT_PANE, and returns the pane ID.
// Requires a running tmux server. Used by AI processes that need an anchor pane
// without being started from within tmux.
func AdoptPane() (string, error) {
	if err := exec.Command("tmux", "info").Run(); err != nil {
		return "", fmt.Errorf("no tmux server running — start tmux first (e.g. tmux new-session -d -s main)")
	}
	out, err := exec.Command("tmux", "new-window", "-P", "-F", "#{pane_id}").Output()
	if err != nil {
		return "", fmt.Errorf("failed to create tmux window: %w", err)
	}
	paneID := strings.TrimSpace(string(out))
	if paneID == "" {
		return "", fmt.Errorf("tmux returned empty pane ID")
	}
	if err := exec.Command("tmux", "set-environment", "-g", "TMUX_MGR_ROOT_PANE", paneID).Run(); err != nil {
		return "", fmt.Errorf("failed to save anchor to tmux environment: %w", err)
	}
	// Cosmetic: the anchor above is the operation that had to succeed.
	_ = exec.Command("tmux", "set", "-g", "pane-border-status", "top").Run()
	_ = exec.Command("tmux", "select-pane", "-t", paneID, "-T", "root").Run()
	return paneID, nil
}
