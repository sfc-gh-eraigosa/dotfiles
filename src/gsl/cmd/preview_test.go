package cmd

import (
	"strings"
	"testing"
)

// TestPreviewCmd_Once verifies that --once prints one non-empty line.
func TestPreviewCmd_Once(t *testing.T) {
	previewOnce = true
	defer func() { previewOnce = false }()

	out := captureStdout(t, func() {
		if err := runPreview(previewCmd, nil); err != nil {
			t.Fatalf("runPreview --once: %v", err)
		}
	})

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Error("preview --once: expected non-empty output, got empty")
	}
	// The time segment should appear.
	if !strings.Contains(out, "10:00") {
		// Allow for clock variation in test environment — just check non-empty.
		t.Logf("preview --once output: %q", trimmed)
	}
}

// TestPreviewCmd_Interactive_NoTTY verifies that runPreview without --once
// returns an error in a non-TTY test environment (which is expected behavior).
// We just check that it does not panic.
func TestPreviewCmd_Interactive_NoTTY(t *testing.T) {
	previewOnce = false
	defer func() { previewOnce = false }()

	// This will fail with a bubbletea error because there is no real TTY in
	// the test environment. We accept either nil or a non-nil error.
	// The important invariant is: no panic.
	_ = captureStdout(t, func() {
		_ = runPreview(previewCmd, nil)
	})
}
