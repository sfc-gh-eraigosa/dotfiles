package payload_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/payload"
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
	if p.RateLimits.FiveHour.ResetsAt == nil || *p.RateLimits.FiveHour.ResetsAt != "2026-05-25T10:00:00Z" {
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

// TestParseBadRateLimitsPreservesOtherFields verifies that a wrong-typed
// rate_limits sub-field does NOT discard the model/context_window that
// parsed cleanly. Per-field decode means a bad sub-object is skipped, not
// fatal, and Parse returns no error for partial success (issue #31).
func TestParseBadRateLimitsPreservesOtherFields(t *testing.T) {
	data := readFixture(t, "bad_rate_limits.json")
	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse(bad_rate_limits.json) returned error: %v; want nil (partial success)", err)
	}

	// model must survive the bad rate_limits.
	if p.Model == nil || p.Model.DisplayName == nil || *p.Model.DisplayName != "claude-sonnet-4-5" {
		t.Errorf("Model.DisplayName: got %v, want pointer to 'claude-sonnet-4-5'", p.Model)
	}

	// context_window must survive the bad rate_limits.
	if p.ContextWindow == nil {
		t.Fatal("ContextWindow is nil; should have survived bad rate_limits")
	}
	if p.ContextWindow.UsedPercentage == nil || *p.ContextWindow.UsedPercentage != 42.5 {
		t.Errorf("ContextWindow.UsedPercentage: got %v, want 42.5", p.ContextWindow.UsedPercentage)
	}

	// rate_limits failed to decode → nil, and a FieldError recorded.
	if p.RateLimits != nil {
		t.Errorf("RateLimits should be nil (sub-object was malformed), got %v", p.RateLimits)
	}
	if len(p.FieldErrors) == 0 {
		t.Error("expected a FieldError recorded for the bad rate_limits, got none")
	}
	foundRate := false
	for _, fe := range p.FieldErrors {
		if fe.Field == "rate_limits" {
			foundRate = true
		}
	}
	if !foundRate {
		t.Errorf("expected a FieldError for 'rate_limits', got %+v", p.FieldErrors)
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
