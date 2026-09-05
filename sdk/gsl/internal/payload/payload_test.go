package payload_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %q: %v", name, err)
	}
	return data
}

// TestParseFullFixture verifies that a complete JSON payload parses correctly
// with all fields present.
func TestParseFullFixture(t *testing.T) {
	data := readFixture(t, "full.json")
	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse(full.json) returned error: %v", err)
	}

	// cwd
	if p.Cwd == nil || *p.Cwd != "/home/user/project" {
		t.Errorf("Cwd: got %v, want pointer to '/home/user/project'", p.Cwd)
	}

	// model
	if p.Model == nil || p.Model.DisplayName == nil || *p.Model.DisplayName != "claude-sonnet-4-5" {
		t.Errorf("Model.DisplayName: got %v", p.Model)
	}

	// context_window
	if p.ContextWindow == nil {
		t.Fatal("ContextWindow is nil")
	}
	if p.ContextWindow.UsedPercentage == nil || *p.ContextWindow.UsedPercentage != 42.5 {
		t.Errorf("ContextWindow.UsedPercentage: got %v, want 42.5", p.ContextWindow.UsedPercentage)
	}
	if p.ContextWindow.TotalInputTokens == nil || *p.ContextWindow.TotalInputTokens != 8500 {
		t.Errorf("ContextWindow.TotalInputTokens: got %v, want 8500", p.ContextWindow.TotalInputTokens)
	}
	if p.ContextWindow.ContextWindowSize == nil || *p.ContextWindow.ContextWindowSize != 200000 {
		t.Errorf("ContextWindow.ContextWindowSize: got %v, want 200000", p.ContextWindow.ContextWindowSize)
	}

	// rate_limits.five_hour
	if p.RateLimits == nil || p.RateLimits.FiveHour == nil {
		t.Fatal("RateLimits.FiveHour is nil")
	}
	if p.RateLimits.FiveHour.UsedPercentage == nil || *p.RateLimits.FiveHour.UsedPercentage != 25.0 {
		t.Errorf("RateLimits.FiveHour.UsedPercentage: got %v, want 25.0", p.RateLimits.FiveHour.UsedPercentage)
	}
	if p.RateLimits.FiveHour.ResetsAt == nil || p.RateLimits.FiveHour.ResetsAt.String() != "2026-05-25T10:00:00Z" {
		t.Errorf("RateLimits.FiveHour.ResetsAt: got %v", p.RateLimits.FiveHour.ResetsAt)
	}

	// rate_limits.seven_day
	if p.RateLimits.SevenDay == nil {
		t.Fatal("RateLimits.SevenDay is nil")
	}
	if p.RateLimits.SevenDay.UsedPercentage == nil || *p.RateLimits.SevenDay.UsedPercentage != 10.5 {
		t.Errorf("RateLimits.SevenDay.UsedPercentage: got %v, want 10.5", p.RateLimits.SevenDay.UsedPercentage)
	}
}

// TestParseMalformedJSON verifies that malformed JSON returns an error.
func TestParseMalformedJSON(t *testing.T) {
	_, err := payload.Parse([]byte(`{not valid json`))
	if err == nil {
		t.Error("Parse(malformed) should return an error, got nil")
	}
}

// TestParseLiveNumericResetsAt is the regression test for issue #30: the
// live Claude Code statusLine payload ships rate_limits.*.resets_at as a
// Unix-epoch number, which used to nuke the entire AI segment because
// json.Unmarshal failed the whole payload on the type mismatch.
// ResetTime.UnmarshalJSON tolerates both shapes; this fixture proves it.
func TestParseLiveNumericResetsAt(t *testing.T) {
	data := readFixture(t, "live_numeric_resets.json")
	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse(live_numeric_resets.json) returned error: %v (this is the #30 regression)", err)
	}
	if p.RateLimits == nil || p.RateLimits.FiveHour == nil || p.RateLimits.FiveHour.ResetsAt == nil {
		t.Fatalf("RateLimits.FiveHour.ResetsAt is nil; whole payload likely degraded (#30): %+v", p.RateLimits)
	}
	// 1779863400 → 2026-05-27T06:30:00Z (UTC).
	want := "2026-05-27T06:30:00Z"
	if got := p.RateLimits.FiveHour.ResetsAt.String(); got != want {
		t.Errorf("five_hour.ResetsAt: got %q, want %q", got, want)
	}
	if p.RateLimits.SevenDay == nil || p.RateLimits.SevenDay.ResetsAt == nil {
		t.Fatalf("RateLimits.SevenDay.ResetsAt is nil")
	}
	// 1780052400 → 2026-05-29T11:00:00Z (UTC).
	want7 := "2026-05-29T11:00:00Z"
	if got := p.RateLimits.SevenDay.ResetsAt.String(); got != want7 {
		t.Errorf("seven_day.ResetsAt: got %q, want %q", got, want7)
	}
}

// TestResetTime_NullAndAbsent verifies the corner cases that surrounding
// pointers handle today: null → zero ResetTime, absent → nil pointer.
func TestResetTime_NullAndAbsent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // String() output; "" means zero/absent
	}{
		{"absent", `{"used_percentage":50}`, ""},
		{"null", `{"used_percentage":50,"resets_at":null}`, ""},
		{"string", `{"used_percentage":50,"resets_at":"2026-05-25T10:00:00Z"}`, "2026-05-25T10:00:00Z"},
		{"epoch_seconds", `{"used_percentage":50,"resets_at":1779863400}`, "2026-05-27T06:30:00Z"},
		{"epoch_millis", `{"used_percentage":50,"resets_at":1779863400000}`, "2026-05-27T06:30:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w payload.RateWindow
			if err := json.Unmarshal([]byte(tc.in), &w); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			var got string
			if w.ResetsAt != nil {
				got = w.ResetsAt.String()
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseEmptyStdin verifies that empty (or whitespace-only) input returns
// an empty struct with no error — matching the "no Claude payload" case.
func TestParseEmptyStdin(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"nil", nil},
		{"empty", []byte("")},
		{"whitespace", []byte("   \n\t  ")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := payload.Parse(tc.input)
			if err != nil {
				t.Errorf("Parse(%q) returned error: %v; want nil", tc.input, err)
			}
			if p.Cwd != nil {
				t.Errorf("Cwd should be nil for empty input, got %v", p.Cwd)
			}
			if p.Model != nil {
				t.Errorf("Model should be nil for empty input, got %v", p.Model)
			}
			if p.ContextWindow != nil {
				t.Errorf("ContextWindow should be nil for empty input, got %v", p.ContextWindow)
			}
			if p.RateLimits != nil {
				t.Errorf("RateLimits should be nil for empty input, got %v", p.RateLimits)
			}
		})
	}
}

// TestParseNullUsedPercentage verifies that a null used_percentage field
// results in a nil pointer, not a zero float64.
func TestParseNullUsedPercentage(t *testing.T) {
	data := readFixture(t, "null_used_pct.json")
	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse(null_used_pct.json) returned error: %v", err)
	}
	if p.ContextWindow == nil {
		t.Fatal("ContextWindow should not be nil")
	}
	if p.ContextWindow.UsedPercentage != nil {
		t.Errorf("UsedPercentage should be nil (was JSON null), got %v", p.ContextWindow.UsedPercentage)
	}
}

// TestParseFiveHourOnlySevenDayAbsent verifies that when seven_day is absent
// from the JSON, its pointer is nil, but five_hour is still populated.
func TestParseFiveHourOnlySevenDayAbsent(t *testing.T) {
	data := readFixture(t, "five_hour_only.json")
	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse(five_hour_only.json) returned error: %v", err)
	}
	if p.RateLimits == nil {
		t.Fatal("RateLimits should not be nil")
	}
	if p.RateLimits.FiveHour == nil {
		t.Error("RateLimits.FiveHour should not be nil")
	}
	if p.RateLimits.SevenDay != nil {
		t.Errorf("RateLimits.SevenDay should be nil (absent from JSON), got %v", p.RateLimits.SevenDay)
	}
}

// TestParseFromReader verifies ParseReader with an io.Reader (e.g. os.Stdin).
func TestParseFromReader(t *testing.T) {
	data := readFixture(t, "full.json")
	r := strings.NewReader(string(data))
	p, err := payload.ParseReader(r)
	if err != nil {
		t.Fatalf("ParseReader returned error: %v", err)
	}
	if p.Cwd == nil || *p.Cwd != "/home/user/project" {
		t.Errorf("Cwd: got %v, want '/home/user/project'", p.Cwd)
	}
}

// TestParseFromReaderEmpty verifies that an empty reader returns an empty
// struct and no error.
func TestParseFromReaderEmpty(t *testing.T) {
	r := strings.NewReader("")
	p, err := payload.ParseReader(r)
	if err != nil {
		t.Errorf("ParseReader(empty) returned error: %v", err)
	}
	if p.Cwd != nil || p.Model != nil || p.ContextWindow != nil || p.RateLimits != nil {
		t.Errorf("expected empty Payload from empty reader, got %+v", p)
	}
}

// TestParseQuotaSynthesis verifies that when quota is present in the payload,
// it is correctly synthesized into RateLimits for the AI segment.
func TestParseQuotaSynthesis(t *testing.T) {
	data := []byte(`{
		"cwd": "/tmp",
		"quota": {
			"gemini-5h": {
				"remaining_fraction": 0.83
			},
			"gemini-weekly": {
				"remaining_fraction": 0.93
			}
		}
	}`)
	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.RateLimits == nil {
		t.Fatal("expected RateLimits to be synthesized, got nil")
	}
	if p.RateLimits.FiveHour == nil || p.RateLimits.FiveHour.UsedPercentage == nil {
		t.Fatal("expected synthesized FiveHour UsedPercentage")
	}
	// (1.0 - 0.83) * 100 = 17.0 (with float precision)
	if got, want := *p.RateLimits.FiveHour.UsedPercentage, 17.0; got < want-0.01 || got > want+0.01 {
		t.Errorf("FiveHour.UsedPercentage: got %f, want %f", got, want)
	}

	if p.RateLimits.SevenDay == nil || p.RateLimits.SevenDay.UsedPercentage == nil {
		t.Fatal("expected synthesized SevenDay UsedPercentage")
	}
	// (1.0 - 0.93) * 100 = 7.0 (with float precision)
	if got, want := *p.RateLimits.SevenDay.UsedPercentage, 7.0; got < want-0.01 || got > want+0.01 {
		t.Errorf("SevenDay.UsedPercentage: got %f, want %f", got, want)
	}
}

// TestParse_ModelID: Claude Code sends model.id alongside display_name; the
// model-page link is derived from it (the display name is the fallback).
func TestParse_ModelID(t *testing.T) {
	p, err := payload.Parse([]byte(`{"model":{"id":"claude-fable-5-1","display_name":"Fable"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Model == nil || p.Model.ID == nil || *p.Model.ID != "claude-fable-5-1" {
		t.Fatalf("Model = %+v, want ID claude-fable-5-1", p.Model)
	}
}
