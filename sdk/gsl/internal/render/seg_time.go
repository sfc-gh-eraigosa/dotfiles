package render

import (
	"context"
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
	// Priority is the DROP priority used by the fit loop (config.Segment.Priority,
	// or the built-in default for this type when unset). It is independent of the
	// segment's position in the line.
	Priority int

	// Links is the link policy (Deps.Links): Time gates the date/time text →
	// the Links.TimeURL template. Zero value or empty template ⇒ no link.
	Links Links
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

// Render implements Segment. It delegates to RenderLinked and discards the
// spans, so the legacy path can never drift from the detect/format path. It
// never returns ok=false (time is always available) and never panics on a bad tz.
func (s *TimeSegment) Render(ctx context.Context, st style.Style, level int) (text, colorKey string, ok bool) {
	text, colorKey, _, ok = s.RenderLinked(ctx, st, level)
	return text, colorKey, ok
}

// RenderLinked implements LinkedSegment: detect once, then format with spans.
func (s *TimeSegment) RenderLinked(ctx context.Context, st style.Style, level int) (text, colorKey string, spans []LinkSpan, ok bool) {
	d, ok := s.detect(ctx)
	if !ok {
		return "", "", nil, false
	}
	text, colorKey, spans = formatLinkedOf(d, st, level)
	return text, colorKey, spans, text != ""
}
