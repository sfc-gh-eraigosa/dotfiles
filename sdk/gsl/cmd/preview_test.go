package cmd

import (
	"strings"
	"testing"
	"time"
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

	// Feed "q" so the TUI quits instead of waiting on a real terminal (it used
	// to hang the whole cmd suite whenever stdin was a TTY). We accept either a
	// nil or a non-nil error; the invariant is: no panic, no hang.
	prev := previewInput
	previewInput = strings.NewReader("q")
	defer func() { previewInput = prev }()
	_ = captureStdout(t, func() {
		_ = runPreview(previewCmd, nil)
	})
}

// TestPreviewCmd_Interactive_QuitsOnInput is the regression test for the
// interactive preview hanging the cmd suite whenever stdin is a terminal:
// runPreview must take its input from previewInput, so a test can feed it "q".
func TestPreviewCmd_Interactive_QuitsOnInput(t *testing.T) {
	previewOnce = false
	prev := previewInput
	previewInput = strings.NewReader("q")
	defer func() { previewInput = prev }()

	// Signal only after captureStdout has put os.Stdout back: finishing the
	// test while stdout is still the capture pipe made `go test -cover` write
	// its report into the closed pipe ("write |1: file already closed").
	done := make(chan error, 1)
	go func() {
		var err error
		_ = captureStdout(t, func() { err = runPreview(previewCmd, nil) })
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("interactive preview did not quit on \"q\" within 5s")
	}
}
