package render

// seg_time_data.go — timeData segmentData + TimeSegment.detect().
//
// Compaction levels for the time segment:
//   level 0: <glyph> <date> <time> <tz>
//   level 1: drop date          → <glyph> <time> <tz>
//   level 2: strip seconds      → <glyph> HH:MM <tz>
//   level 3: drop tz            → <glyph> HH:MM

import (
	"context"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// timeData is the detect-once intermediate for the time segment.
type timeData struct {
	// t is the resolved time instant (already in the configured location).
	t time.Time
	// dateLayout is the Go format string for the date portion.
	dateLayout string
	// timeLayout is the Go format string for the time portion (may include seconds).
	timeLayout string
	// tz is the timezone abbreviation string (e.g. "UTC", "PST").
	tz string
}

// detect implements detectable for TimeSegment. Captures the current time once.
func (s *TimeSegment) detect(_ context.Context) (segmentData, bool) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}

	loc := time.UTC
	if s.Timezone != "" {
		if l, err := time.LoadLocation(s.Timezone); err == nil {
			loc = l
		}
	}

	t := now().In(loc)

	dateLayout := s.DateFormat
	if dateLayout == "" {
		dateLayout = defaultDateLayout
	}
	timeLayout := s.TimeFormat
	if timeLayout == "" {
		timeLayout = defaultTimeLayout
	}

	return &timeData{
		t:          t,
		dateLayout: dateLayout,
		timeLayout: timeLayout,
		tz:         t.Format("MST"),
	}, true
}

// format implements segmentData.format for timeData. Pure; no I/O.
func (d *timeData) format(st style.Style, level int) (text, colorKey string) {
	var b strings.Builder

	if g := glyph(st, "time"); g != "" {
		b.WriteString(g)
		b.WriteString(" ")
	}

	// Date: shown at level 0 only.
	if level == 0 {
		b.WriteString(d.t.Format(d.dateLayout))
		b.WriteString(" ")
	}

	// Time: shown at all levels.
	tLayout := d.timeLayout
	if level >= 2 {
		// Strip seconds from the format: replace ":05" / ":SS" → "" if present.
		tLayout = stripSecondsFromLayout(tLayout)
	}
	b.WriteString(d.t.Format(tLayout))

	// Timezone: shown at levels 0–2 only.
	if level <= 2 && d.tz != "" {
		b.WriteString(" ")
		b.WriteString(d.tz)
	}

	return b.String(), "time"
}

// stripSecondsFromLayout removes the seconds component (":05" or ":SS")
// from a Go time layout string so the formatted time shows only HH:MM.
//
// The Go time layout uses "05" for seconds. Common patterns:
//
//	"15:04:05" → "15:04"
//	"3:04:05 PM" → "3:04 PM"
func stripSecondsFromLayout(layout string) string {
	// Go's reference time has seconds at "05". The two-digit 24h form
	// "15:04:05" is the most common; strip ":05" if present.
	if s := strings.Replace(layout, ":05", "", 1); s != layout {
		return s
	}
	return layout
}
