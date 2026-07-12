package render

// panic_level_test.go — WS3 logging hygiene.
//
// A panicking segment is a BUG in gsl; it must be logged at Error. It was
// logged at Warn, in the same band as the ~1.6k routine "expected non-zero
// exit" records the seams emitted per session — which is exactly how a real
// segment.panic goes unnoticed for months.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
)

// readPanicLog runs f with a temp log file and returns its contents.
func readPanicLog(t *testing.T, f func()) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "gsl.log")
	t.Setenv("GSL_LOG_FILE", logPath)
	observe.ResetDefaultForTest()
	t.Cleanup(observe.ResetDefaultForTest)

	f()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(data)
}

// TestRender_SegmentPanic_LogsAtError covers the Render path.
func TestRender_SegmentPanic_LogsAtError(t *testing.T) {
	got := readPanicLog(t, func() {
		_ = Render(context.Background(), config.Default(), spaceStyle(), []Segment{&panicSegment{}})
	})

	if !strings.Contains(got, `"event":"segment.panic"`) {
		t.Fatalf("expected segment.panic in log, got: %s", got)
	}
	if !strings.Contains(got, `"level":"error"`) {
		t.Errorf("segment.panic must be logged at level=error (it is a gsl bug, not a routine "+
			"degradation); got: %s", got)
	}
}

// TestDetect_SegmentPanic_LogsAtError covers the Detect path — the one that
// actually runs in production, and whose recover() silently swallowed the nil
// Runner panic in gh.PR.
func TestDetect_SegmentPanic_LogsAtError(t *testing.T) {
	cfg := config.Default()
	got := readPanicLog(t, func() {
		_ = Detect(context.Background(), cfg, spaceStyle(), []Segment{&panicSegment{}})
	})

	if !strings.Contains(got, `"event":"segment.panic"`) {
		t.Fatalf("expected segment.panic in log, got: %s", got)
	}
	if !strings.Contains(got, `"level":"error"`) {
		t.Errorf("segment.panic during Detect must be logged at level=error; got: %s", got)
	}
}
