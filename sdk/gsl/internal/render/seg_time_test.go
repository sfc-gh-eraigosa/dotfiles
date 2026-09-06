package render

import (
	"context"
	"strings"
	"testing"
)

func TestTime_ValidTZ(t *testing.T) {
	st := asciiStyle()
	seg := NewTimeSegment(fixedClock(), "America/Los_Angeles", "15:04", "Mon 01-02")

	got, colorKey, ok := seg.Render(context.Background(), st, 0)
	if !ok {
		t.Fatal("time: want ok=true")
	}
	if colorKey != "time" {
		t.Errorf("time: want colorKey=time, got %q", colorKey)
	}
	// 2026-05-25 14:30 UTC → 07:30 PDT, Monday May 25.
	if !strings.Contains(got, "07:30") {
		t.Errorf("time LA: want 07:30, got %q", got)
	}
	if !strings.Contains(got, "PDT") {
		t.Errorf("time LA: want PDT abbreviation, got %q", got)
	}
	if !strings.Contains(got, "Mon") {
		t.Errorf("time LA: want Mon, got %q", got)
	}
}

func TestTime_BadTZ_FallsBackToUTC(t *testing.T) {
	st := asciiStyle()
	seg := NewTimeSegment(fixedClock(), "Not/AReal_Zone", "15:04", "Mon 01-02")

	// Must not panic.
	got, _, ok := seg.Render(context.Background(), st, 0)
	if !ok {
		t.Fatal("time: want ok=true even with a bad tz")
	}
	if !strings.Contains(got, "14:30") {
		t.Errorf("time bad-tz: want UTC 14:30, got %q", got)
	}
	if !strings.Contains(got, "UTC") {
		t.Errorf("time bad-tz: want UTC abbreviation, got %q", got)
	}
}

func TestTime_EmptyTZ_UsesUTC(t *testing.T) {
	st := asciiStyle()
	seg := NewTimeSegment(fixedClock(), "", "", "")
	got, _, ok := seg.Render(context.Background(), st, 0)
	if !ok {
		t.Fatal("time: want ok=true")
	}
	if !strings.Contains(got, "UTC") {
		t.Errorf("time empty-tz: want UTC, got %q", got)
	}
	// Default time layout is 15:04.
	if !strings.Contains(got, "14:30") {
		t.Errorf("time default layout: want 14:30, got %q", got)
	}
}

func TestTime_GlyphModes(t *testing.T) {
	// nerdfont
	pl := powerlineStyleFixture()
	seg := NewTimeSegment(fixedClock(), "UTC", "15:04", "Mon 01-02")
	got, _, _ := seg.Render(context.Background(), pl, 0)
	if !strings.Contains(got, pl.Icons["time"]) {
		t.Errorf("time nerdfont: want time glyph %q in %q", pl.Icons["time"], got)
	}

	// emoji
	em := emojiStyleFixture()
	got, _, _ = seg.Render(context.Background(), em, 0)
	if !strings.Contains(got, "⏰") {
		t.Errorf("time emoji: want ⏰ in %q", got)
	}

	// ascii
	as := asciiStyle()
	got, _, _ = seg.Render(context.Background(), as, 0)
	if !strings.Contains(got, "[time]") {
		t.Errorf("time ascii: want [time] in %q", got)
	}
}

func TestTime_Span_WholeTextAfterGlyph(t *testing.T) {
	seg := NewTimeSegment(fixedClock(), "America/Los_Angeles", "15:04:05", "2006-01-02")
	seg.Links = Links{Time: true, TimeURL: DefaultTimeURL}
	text, _, spans, _ := seg.RenderLinked(context.Background(), asciiStyle(), 0)
	if len(spans) != 1 || spans[0].URL != "https://time.is/Los_Angeles" || spans[0].End != len(text) {
		t.Errorf("spans = %+v text=%q", spans, text)
	}
}
