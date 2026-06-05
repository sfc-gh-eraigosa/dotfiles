package render

import (
	"context"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/style"
)

// TimeSegment renders the current time in a configurable timezone:
//
//	<time-glyph> Mon 01-02 15:04 PST
//
// The clock is INJECTED (Now) so golden tests are deterministic. A bad or
// unknown timezone falls back to UTC and never panics. DateFormat / TimeFormat
// come from config (Go layout strings); empty values use sensible defaults.
type TimeSegment struct {
	// Now returns the current instant. Injected for deterministic tests; when
	// nil, time.Now is used.
	Now func() time.Time
	// Timezone is an IANA tz name (e.g. "America/Los_Angeles"). Empty or
	// unknown ⇒ UTC.
	Timezone string
	// TimeFormat is the Go layout for the clock (default "15:04").
	TimeFormat string
	// DateFormat is the Go layout for the date (default "Mon 01-02").
	DateFormat string
}

// NewTimeSegment builds a TimeSegment from config values. now may be nil to use
// time.Now.
func NewTimeSegment(now func() time.Time, tz, timeFormat, dateFormat string) *TimeSegment {
	return &TimeSegment{
		Now:        now,
		Timezone:   tz,
		TimeFormat: timeFormat,
		DateFormat: dateFormat,
	}
}

const (
	defaultTimeLayout = "15:04"
	defaultDateLayout = "Mon 01-02"
)

// Render implements Segment. It never returns ok=false (time is always
// available) and never panics on a bad tz.
func (s *TimeSegment) Render(_ context.Context, st style.Style) (string, bool) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}

	loc := time.UTC
	if s.Timezone != "" {
		if l, err := time.LoadLocation(s.Timezone); err == nil {
			loc = l
		}
		// On error: keep UTC fallback. Never crash on an unknown tz.
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

	var b strings.Builder
	if g := glyph(st, "time"); g != "" {
		b.WriteString(g)
		b.WriteString(" ")
	}
	b.WriteString(t.Format(dateLayout))
	b.WriteString(" ")
	b.WriteString(t.Format(timeLayout))

	// Timezone abbreviation (e.g. "PST", "UTC"). Format("MST") yields the
	// abbreviation; LoadLocation failures already collapsed to UTC.
	if abbr := t.Format("MST"); abbr != "" {
		b.WriteString(" ")
		b.WriteString(abbr)
	}

	return paint(st, "time", b.String()), true
}
