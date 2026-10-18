package render

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/video"
)

// seededVideoCache returns a cache in a temp dir with rec already written.
func seededVideoCache(t *testing.T, rec video.Record) *video.Cache {
	t.Helper()
	c := &video.Cache{Dir: t.TempDir()}
	if err := c.Write(rec); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	return c
}

// freshOKRecord is a reading stamped now, so the cache is not stale and the
// segment never asks for a refresher.
func freshOKRecord() video.Record {
	return video.Record{
		OK: true,
		Stats: video.Stats{
			Util:       6,
			UsedBytes:  2069835776, // 1.9G
			TotalBytes: 8319870976, // 7.7G
			Source:     "nvidia",
		},
		Updated: time.Now().UnixMilli(),
	}
}

// spawnCounter counts refresher starts. It never execs anything.
type spawnCounter struct{ n int }

func (s *spawnCounter) spawn(*os.File) error { s.n++; return nil }

func TestVideoSegment_RendersUtilAndMemory(t *testing.T) {
	cache := seededVideoCache(t, freshOKRecord())
	var spy spawnCounter
	seg := NewVideoSegment(cache, spy.spawn)

	st := powerlineStyleFixture()
	text, colorKey, ok := seg.Render(context.Background(), st, 0)
	if !ok {
		t.Fatal("seeded video segment hidden, want shown")
	}
	want := glyph(st, "video") + " 6%|1.9G/7.7G"
	if text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
	if colorKey != "video" {
		t.Fatalf("colorKey = %q, want %q", colorKey, "video")
	}
	if spy.n != 0 {
		t.Fatalf("fresh seeded cache spawned %d refreshers, want 0", spy.n)
	}
}

func TestVideoSegment_CompactionLevels(t *testing.T) {
	cache := seededVideoCache(t, freshOKRecord())
	st := powerlineStyleFixture()
	g := glyph(st, "video") + " "
	cases := []struct {
		level int
		want  string
	}{
		{0, g + "6%|1.9G/7.7G"},
		{1, g + "6%|1.9G"},
		{2, g + "6%"},
	}
	for _, c := range cases {
		seg := NewVideoSegment(cache, nil)
		got, _, ok := seg.Render(context.Background(), st, c.level)
		if !ok || got != c.want {
			t.Errorf("level %d: got (%q, %v), want %q", c.level, got, ok, c.want)
		}
	}
}

func TestVideoSegment_ASCIIShowsLabel(t *testing.T) {
	cache := seededVideoCache(t, freshOKRecord())
	st := asciiStyle()
	seg := NewVideoSegment(cache, nil)
	text, _, ok := seg.Render(context.Background(), st, 0)
	if !ok {
		t.Fatal("ascii video segment hidden, want shown")
	}
	if want := "[video] 6%|1.9G/7.7G"; text != want {
		t.Fatalf("ascii text = %q, want %q", text, want)
	}
}

func TestVideoSegment_HiddenWithoutData(t *testing.T) {
	cases := []struct {
		name  string
		cache func(t *testing.T) *video.Cache
	}{
		{
			name:  "no record yet (first run)",
			cache: func(t *testing.T) *video.Cache { return &video.Cache{Dir: t.TempDir()} },
		},
		{
			name: "no-source record",
			cache: func(t *testing.T) *video.Cache {
				return seededVideoCache(t, video.Record{OK: false, Updated: time.Now().UnixMilli()})
			},
		},
		{
			name:  "nil cache",
			cache: func(t *testing.T) *video.Cache { return nil },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var spy spawnCounter
			seg := NewVideoSegment(c.cache(t), spy.spawn)
			st := powerlineStyleFixture()
			if text, _, ok := seg.Render(context.Background(), st, 0); ok {
				t.Fatalf("hidden case rendered %q", text)
			}
			// Absent data may ask for a refresh; the segment must still hide.
			if spy.n > 1 {
				t.Fatalf("spawned %d refreshers, want at most 1", spy.n)
			}
		})
	}
}

func TestVideoSegment_NoSourceSuppressesSpawnWithinBackoff(t *testing.T) {
	cache := seededVideoCache(t, video.Record{OK: false, Updated: time.Now().UnixMilli()})
	var spy spawnCounter
	seg := NewVideoSegment(cache, spy.spawn)
	for i := 0; i < 5; i++ {
		seg.Render(context.Background(), powerlineStyleFixture(), 0)
	}
	if spy.n != 0 {
		t.Fatalf("no-source backoff window spawned %d refreshers, want 0", spy.n)
	}
}
