package config

// priority_test.go — Segment.Priority and Config.FallbackColumns (gsl-ultra WS1).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSegmentPriority_Order(t *testing.T) {
	// The drop order the fit loop relies on: time goes first, repo goes last.
	if DefaultSegmentPriority("time") >= DefaultSegmentPriority("ai") ||
		DefaultSegmentPriority("ai") >= DefaultSegmentPriority("dirgit") ||
		DefaultSegmentPriority("dirgit") >= DefaultSegmentPriority("repo") {
		t.Errorf("want time < ai < dirgit < repo, got time=%d ai=%d dirgit=%d repo=%d",
			DefaultSegmentPriority("time"), DefaultSegmentPriority("ai"),
			DefaultSegmentPriority("dirgit"), DefaultSegmentPriority("repo"))
	}
	if got := DefaultSegmentPriority("nonesuch"); got != 0 {
		t.Errorf("unknown type: want 0, got %d", got)
	}
}

func TestSegment_EffectivePriority(t *testing.T) {
	tests := []struct {
		name string
		seg  Segment
		want int
	}{
		{"unset uses the type default", Segment{Type: "repo"}, PriorityRepo},
		{"unset uses the type default (time)", Segment{Type: "time"}, PriorityTime},
		{"explicit wins", Segment{Type: "time", Priority: 99}, 99},
		{"unknown type unset", Segment{Type: "zzz"}, 0},
		{"unknown type explicit", Segment{Type: "zzz", Priority: 7}, 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.seg.EffectivePriority(); got != tc.want {
				t.Errorf("EffectivePriority() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestConfig_EffectiveFallbackColumns(t *testing.T) {
	if got := Default().EffectiveFallbackColumns(); got != DefaultFallbackColumns {
		t.Errorf("Default(): got %d, want %d", got, DefaultFallbackColumns)
	}
	if got := (Config{FallbackColumns: 200}).EffectiveFallbackColumns(); got != 200 {
		t.Errorf("explicit 200: got %d", got)
	}
	// Unset (0) and nonsensical (negative) both fall back to the default rather
	// than producing a zero-width line.
	if got := (Config{}).EffectiveFallbackColumns(); got != DefaultFallbackColumns {
		t.Errorf("unset: got %d, want %d", got, DefaultFallbackColumns)
	}
	if got := (Config{FallbackColumns: -10}).EffectiveFallbackColumns(); got != DefaultFallbackColumns {
		t.Errorf("negative: got %d, want %d", got, DefaultFallbackColumns)
	}
}

// TestLoad_PriorityAndFallbackRoundTrip proves the new fields survive a
// save/load cycle and that an OLD config.json (with neither field) still loads —
// the schema is a strict superset.
func TestLoad_PriorityAndFallbackRoundTrip(t *testing.T) {
	dir := t.TempDir()

	t.Run("new fields round-trip", func(t *testing.T) {
		path := filepath.Join(dir, "new.json")
		c := Default()
		c.FallbackColumns = 200
		c.Segments[0].Priority = 99
		if err := Save(path, c); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.FallbackColumns != 200 {
			t.Errorf("FallbackColumns = %d, want 200", got.FallbackColumns)
		}
		if got.Segments[0].Priority != 99 {
			t.Errorf("Segments[0].Priority = %d, want 99", got.Segments[0].Priority)
		}
	})

	t.Run("legacy config without the new fields still loads", func(t *testing.T) {
		path := filepath.Join(dir, "legacy.json")
		legacy := `{"enabled":true,"segments":[{"type":"repo","enabled":true}],"style":"powerline"}`
		if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := Load(path)
		if err != nil {
			t.Fatalf("Load legacy: %v", err)
		}
		// Unmarshal-over-Default keeps the default fallback.
		if got.EffectiveFallbackColumns() != DefaultFallbackColumns {
			t.Errorf("legacy fallback = %d, want %d", got.EffectiveFallbackColumns(), DefaultFallbackColumns)
		}
		// An unset Priority resolves to the type default, so an existing config
		// keeps today's drop behaviour without being rewritten.
		if got.Segments[0].EffectivePriority() != PriorityRepo {
			t.Errorf("legacy repo priority = %d, want %d",
				got.Segments[0].EffectivePriority(), PriorityRepo)
		}
	})

	t.Run("unset priority is omitted from the serialized form", func(t *testing.T) {
		data, err := json.Marshal(Segment{Type: "repo", Enabled: true})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(data) != `{"type":"repo","enabled":true}` {
			t.Errorf("zero Priority must be omitempty; got %s", data)
		}
	})
}
