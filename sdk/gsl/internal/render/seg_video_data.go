package render

// seg_video_data.go — videoData segmentData + VideoSegment.detect().
//
// Compaction levels for the video segment:
//   level 0: <glyph> 12%|2.8G/16G
//   level 1: <glyph> 12%|2.8G     (drop the max)
//   level 2+: <glyph> 12%         (utilisation only)

import (
	"context"
	"strconv"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/video"
)

// videoData is the detect-once intermediate for the video segment.
type videoData struct {
	stats video.Stats
	prio  int
}

// priority implements prioritized: video drops before everything else.
func (d *videoData) priority() int {
	if d.prio != 0 {
		return d.prio
	}
	return config.PriorityVideo
}

// detect implements detectable. It only touches the cache file; the probe runs
// in the detached refresher, so this never blocks on a subprocess.
func (s *VideoSegment) detect(_ context.Context) (segmentData, bool) {
	if s.Cache == nil {
		return nil, false
	}
	stats, show := s.Cache.Get(s.Spawn)
	if !show {
		return nil, false
	}
	return &videoData{stats: stats, prio: s.Priority}, true
}

// format implements segmentData.format. Pure; no I/O.
func (d *videoData) format(st style.Style, level int) (text, colorKey string) {
	var b []byte
	if g := glyph(st, "video"); g != "" {
		b = append(b, g...)
		b = append(b, ' ')
	}
	b = strconv.AppendInt(b, int64(d.stats.Util), 10)
	b = append(b, '%')
	if level <= 1 && (d.stats.UsedBytes > 0 || d.stats.TotalBytes > 0) {
		b = append(b, '|')
		b = append(b, video.HumanBytes(d.stats.UsedBytes)...)
		if level == 0 && d.stats.TotalBytes > 0 {
			b = append(b, '/')
			b = append(b, video.HumanBytes(d.stats.TotalBytes)...)
		}
	}
	return string(b), "video"
}
