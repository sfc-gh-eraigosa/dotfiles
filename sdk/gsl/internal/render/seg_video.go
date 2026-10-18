package render

import (
	"context"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/video"
)

// VideoSegment renders GPU load and memory:
//
//	<glyph> 12%|2.8G/16G
//
// It never probes hardware itself. It reads the cache file (microseconds) and,
// when the data is stale, asks the cache to start one detached refresher; the
// reading shown is the last one stored. No data (first run, or no GPU source
// on this host) hides the segment.
type VideoSegment struct {
	// Cache is the state-dir data file + lock. Nil hides the segment.
	Cache *video.Cache
	// Spawn starts the detached refresher (video.SpawnRefresher in
	// production). Injected so tests count starts without forking.
	Spawn video.SpawnFunc
	// Priority is the DROP priority (config.Segment.EffectivePriority).
	Priority int
}

// NewVideoSegment builds a VideoSegment.
func NewVideoSegment(c *video.Cache, spawn video.SpawnFunc) *VideoSegment {
	return &VideoSegment{Cache: c, Spawn: spawn}
}

// Render implements Segment via the detect/format path.
func (s *VideoSegment) Render(ctx context.Context, st style.Style, level int) (text, colorKey string, ok bool) {
	d, ok := s.detect(ctx)
	if !ok {
		return "", "", false
	}
	text, colorKey = d.format(st, level)
	return text, colorKey, text != ""
}
