package render

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// stubSegment is a deterministic Segment for orchestration tests.
type stubSegment struct {
	text  string
	ok    bool
	delay time.Duration // simulate slow work; honours ctx cancellation
}

func (s *stubSegment) Render(ctx context.Context, _ style.Style) (string, bool) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return "", false // timed out → omit
		}
	}
	return s.text, s.ok
}

func spaceStyle() style.Style { return style.Style{Separator: "space"} }

func TestRender_OrderPreserved(t *testing.T) {
	cfg := config.Default() // Enabled
	segs := []Segment{
		&stubSegment{text: "A", ok: true},
		&stubSegment{text: "B", ok: true},
		&stubSegment{text: "C", ok: true},
	}
	got := Render(context.Background(), cfg, spaceStyle(), segs)
	if got != "A B C" {
		t.Errorf("render order: want 'A B C', got %q", got)
	}
}

func TestRender_OmittedSegmentSkipped(t *testing.T) {
	cfg := config.Default()
	segs := []Segment{
		&stubSegment{text: "A", ok: true},
		&stubSegment{text: "B", ok: false}, // self-omit
		&stubSegment{text: "C", ok: true},
	}
	got := Render(context.Background(), cfg, spaceStyle(), segs)
	if got != "A C" {
		t.Errorf("render omit: want 'A C', got %q", got)
	}
}

func TestRender_MasterOff_Empty(t *testing.T) {
	cfg := config.Default()
	cfg.Enabled = false
	segs := []Segment{&stubSegment{text: "A", ok: true}}
	if got := Render(context.Background(), cfg, spaceStyle(), segs); got != "" {
		t.Errorf("render master-off: want empty, got %q", got)
	}
}

func TestRender_NoSegments_Empty(t *testing.T) {
	cfg := config.Default()
	if got := Render(context.Background(), cfg, spaceStyle(), nil); got != "" {
		t.Errorf("render no-segs: want empty, got %q", got)
	}
}

func TestRender_SlowSegmentDegrades_NoHang(t *testing.T) {
	cfg := config.Default()
	// One segment sleeps far longer than any deadline; with the parent ctx
	// cancelled quickly it must drop out while the others render.
	segs := []Segment{
		&stubSegment{text: "fast1", ok: true},
		&stubSegment{text: "slow", ok: true, delay: 5 * time.Second},
		&stubSegment{text: "fast2", ok: true},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan string, 1)
	go func() { done <- Render(ctx, cfg, spaceStyle(), segs) }()

	select {
	case got := <-done:
		if got != "fast1 fast2" {
			t.Errorf("render degrade: want 'fast1 fast2', got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("render degrade: Render hung past 2s")
	}
}

func TestRender_PanickingSegmentRecovered(t *testing.T) {
	cfg := config.Default()
	segs := []Segment{
		&stubSegment{text: "ok1", ok: true},
		&panicSegment{},
		&stubSegment{text: "ok2", ok: true},
	}
	got := Render(context.Background(), cfg, spaceStyle(), segs)
	if got != "ok1 ok2" {
		t.Errorf("render panic-recover: want 'ok1 ok2', got %q", got)
	}
}

type panicSegment struct{}

func (panicSegment) Render(context.Context, style.Style) (string, bool) {
	panic("boom")
}

// TestRender_LogsSegmentPanic asserts the segment.panic structured record
// is emitted when a segment panics. The recover() behaviour itself is
// covered by TestRender_PanickingSegmentRecovered.
func TestRender_LogsSegmentPanic(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gsl.log")
	t.Setenv("GSL_LOG_FILE", logPath)
	observe.ResetDefaultForTest()
	t.Cleanup(observe.ResetDefaultForTest)

	cfg := config.Default()
	segs := []Segment{&panicSegment{}}
	_ = Render(context.Background(), cfg, spaceStyle(), segs)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), `"event":"segment.panic"`) {
		t.Fatalf("expected segment.panic in log, got: %s", data)
	}
}

// TestRender_LogsSegmentTimeout asserts the segment.timeout structured
// record is emitted when a segment exceeds the per-segment deadline.
// The drop-and-continue behaviour itself is covered by
// TestRender_SlowSegmentDegrades_NoHang.
func TestRender_LogsSegmentTimeout(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "gsl.log")
	t.Setenv("GSL_LOG_FILE", logPath)
	observe.ResetDefaultForTest()
	t.Cleanup(observe.ResetDefaultForTest)

	cfg := config.Default()
	// stubSegment with delay > segmentDeadline + tight parent ctx so the
	// inner sctx hits DeadlineExceeded quickly.
	segs := []Segment{&stubSegment{text: "x", ok: true, delay: 5 * time.Second}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = Render(ctx, cfg, spaceStyle(), segs)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), `"event":"segment.timeout"`) {
		t.Fatalf("expected segment.timeout in log, got: %s", data)
	}
}

func TestBuildSegments_OrderAndDisable(t *testing.T) {
	cfg := config.Config{
		Enabled: true,
		Segments: []config.Segment{
			{Type: "time", Enabled: true},
			{Type: "ai", Enabled: false}, // disabled → skipped
			{Type: "dirgit", Enabled: true},
			{Type: "bogus", Enabled: true}, // unknown → skipped
		},
		Timezone: "UTC",
	}
	segs := BuildSegments(cfg, Deps{Clock: fixedClock()})
	if len(segs) != 2 {
		t.Fatalf("BuildSegments: want 2 segments (time, dirgit), got %d", len(segs))
	}
	if _, isTime := segs[0].(*TimeSegment); !isTime {
		t.Errorf("BuildSegments: first segment should be TimeSegment, got %T", segs[0])
	}
	if _, isDir := segs[1].(*DirGitSegment); !isDir {
		t.Errorf("BuildSegments: second segment should be DirGitSegment, got %T", segs[1])
	}
}

func TestRender_ConcurrentRaceSafety(t *testing.T) {
	// Run with -race to assert the orchestrator does not race on the results
	// slice when many segments render in parallel.
	cfg := config.Default()
	segs := make([]Segment, 20)
	for i := range segs {
		segs[i] = &stubSegment{text: strings.Repeat("x", i+1), ok: true}
	}
	got := Render(context.Background(), cfg, spaceStyle(), segs)
	if !strings.Contains(got, "x x") { // first two segments "x" and "xx"
		t.Errorf("render concurrent: unexpected output %q", got)
	}
}
