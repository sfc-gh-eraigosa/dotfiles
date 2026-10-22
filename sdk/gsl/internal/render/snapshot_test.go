package render

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/term"
)

// One detection serves any later selection, order and width.
func TestSnapshot_RefitsWithoutDetecting(t *testing.T) {
	cfg := config.Default()
	st := asciiStyle()
	deps := Deps{Clock: func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) },
		Hostname: func() (string, error) { return "demohost", nil }}
	snap := DetectAll(context.Background(), cfg, st, deps)
	line := term.StripANSI(snap.Line(cfg, st, 200))
	if !strings.Contains(line, "demohost") {
		t.Fatalf("line = %q", line)
	}
	off := cfg
	off.Segments = append([]config.Segment(nil), cfg.Segments...)
	for i := range off.Segments {
		if off.Segments[i].Type == "host" {
			off.Segments[i].Enabled = false
		}
	}
	if got := term.StripANSI(snap.Line(off, st, 200)); strings.Contains(got, "demohost") {
		t.Errorf("host disabled, still rendered: %q", got)
	}
	if w := term.DisplayWidth(snap.Line(cfg, st, 20)); w > 20 {
		t.Errorf("width 20 → %d columns", w)
	}
	cfg.Enabled = false
	if snap.Line(cfg, st, 200) != "" {
		t.Error("master off must render nothing")
	}
}

// The time segment follows the config handed to Line — timezone and formats
// are edited in the studio after the one detection.
func TestSnapshot_TimeFollowsConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Timezone, cfg.TimeFormat, cfg.DateFormat = "UTC", "15:04", "2006-01-02"
	st := asciiStyle()
	deps := Deps{Clock: func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) },
		Hostname: func() (string, error) { return "demohost", nil }}
	snap := DetectAll(context.Background(), cfg, st, deps)
	if got := term.StripANSI(snap.Line(cfg, st, 200)); !strings.Contains(got, "12:00") {
		t.Fatalf("baseline: %q", got)
	}
	edited := cfg
	edited.Timezone, edited.TimeFormat = "Asia/Tokyo", "3:04 PM"
	if got := term.StripANSI(snap.Line(edited, st, 200)); !strings.Contains(got, "9:00 PM") {
		t.Errorf("edited time not shown: %q", got)
	}
}
