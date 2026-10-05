package usage_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/usage"
)

var now = time.Date(2026, 10, 5, 5, 0, 0, 0, time.UTC)

func parse(t *testing.T, js string) payload.Payload {
	t.Helper()
	p, err := payload.Parse([]byte(js))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return p
}

func TestFromPayload_Claude(t *testing.T) {
	p := parse(t, `{"model":{"id":"claude-opus-5-5","display_name":"Opus 5.5"},
		"rate_limits":{"five_hour":{"used_percentage":12.5,"resets_at":"2026-10-05T07:00:00Z"},
		"seven_day":{"used_percentage":34,"resets_at":1791622800}}}`)
	s, ok := usage.FromPayload(p, now)
	if !ok {
		t.Fatal("ok = false; want a snapshot")
	}
	if s.Host != usage.HostClaude || s.Model != "claude-opus-5-5" || !s.CapturedAt.Equal(now) {
		t.Errorf("header = %+v", s)
	}
	if s.FiveHour == nil || s.FiveHour.UsedPercentage != 12.5 || s.FiveHour.ResetsAt != "2026-10-05T07:00:00Z" {
		t.Errorf("five_hour = %+v", s.FiveHour)
	}
	if s.SevenDay == nil || s.SevenDay.UsedPercentage != 34 || s.SevenDay.ResetsAt == "" {
		t.Errorf("seven_day = %+v (epoch resets_at must become RFC3339)", s.SevenDay)
	}
}

func TestFromPayload_AntigravityQuota(t *testing.T) {
	p := parse(t, `{"product":"antigravity","model":{"display_name":"Gemini"},
		"quota":{"gemini-weekly":{"remaining_fraction":0.75,"reset_time":"2026-10-09T00:00:00Z"}}}`)
	s, ok := usage.FromPayload(p, now)
	if !ok || s.Host != usage.HostAntigravity || s.Model != "Gemini" {
		t.Fatalf("snapshot = %+v ok=%v", s, ok)
	}
	if s.SevenDay == nil || s.SevenDay.UsedPercentage < 24.99 || s.SevenDay.UsedPercentage > 25.01 {
		t.Errorf("seven_day = %+v; want used 25%% from remaining 0.75", s.SevenDay)
	}
	if s.FiveHour != nil {
		t.Errorf("five_hour = %+v; agy sent no 5h bucket", s.FiveHour)
	}
}

func TestFromPayload_NothingToRecord(t *testing.T) {
	for _, js := range []string{`{}`, `{"rate_limits":{}}`, `{"rate_limits":{"seven_day":{"resets_at":"2026-10-09T00:00:00Z"}}}`} {
		if _, ok := usage.FromPayload(parse(t, js), now); ok {
			t.Errorf("%s: ok = true; want nothing recorded", js)
		}
	}
}

func TestWriteRead_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "usage")
	if _, err := usage.Read(dir, usage.HostClaude); !errors.Is(err, usage.ErrNone) {
		t.Errorf("Read before any write = %v; want ErrNone", err)
	}
	if all, err := usage.ReadAll(dir); err != nil || len(all) != 0 {
		t.Errorf("ReadAll on a missing dir = %v, %v; want empty, nil", all, err)
	}
	c := usage.Snapshot{Host: usage.HostClaude, CapturedAt: now, SevenDay: &usage.Window{UsedPercentage: 34}}
	a := usage.Snapshot{Host: usage.HostAntigravity, CapturedAt: now, SevenDay: &usage.Window{UsedPercentage: 25}}
	for _, s := range []usage.Snapshot{c, a} {
		if err := usage.Write(dir, s); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	c.SevenDay.UsedPercentage = 40 // a newer render replaces the old one
	if err := usage.Write(dir, c); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := usage.Read(dir, usage.HostClaude)
	if err != nil || got.SevenDay.UsedPercentage != 40 || !got.CapturedAt.Equal(now) {
		t.Errorf("Read = %+v, %v; want the newest snapshot", got, err)
	}
	all, err := usage.ReadAll(dir)
	if err != nil || len(all) != 2 || all[0].Host != usage.HostAntigravity || all[1].Host != usage.HostClaude {
		t.Errorf("ReadAll = %+v, %v; want both hosts sorted", all, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("dir holds %d entries; want 2 (no temp files left behind)", len(entries))
	}
}

func TestDir_XDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/x/state")
	if d, err := usage.Dir(); err != nil || d != "/x/state/gsl/usage" {
		t.Errorf("Dir = %q, %v", d, err)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/h")
	if d, err := usage.Dir(); err != nil || d != "/h/.local/state/gsl/usage" {
		t.Errorf("Dir = %q, %v", d, err)
	}
}
