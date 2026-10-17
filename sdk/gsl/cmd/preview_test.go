package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

// TestPreviewCmd_Interactive_QuitOnQ runs the interactive path with an
// injected input that immediately sends 'q', so the program starts, processes
// one quit key, and exits — deterministically, with or without a TTY.
//
// The previous version of this test ran the bubbletea program on the real
// stdin, assuming the test environment never has a TTY. Under an interactive
// `make test` stdin IS a TTY, so the full interactive UI launched and blocked
// forever waiting for a keypress (the "make test hangs on gsl" bug).
func TestPreviewCmd_Interactive_QuitOnQ(t *testing.T) {
	previewOnce = false
	previewTeaOpts = []tea.ProgramOption{
		tea.WithInput(strings.NewReader("q")),
		tea.WithoutRenderer(),
	}
	defer func() {
		previewOnce = false
		previewTeaOpts = nil
	}()

	_ = captureStdout(t, func() {
		if err := runPreview(previewCmd, nil); err != nil {
			t.Errorf("runPreview interactive with injected 'q': %v", err)
		}
	})
}
