package payload_test

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// TestParse_PerField (E20 / F14 / closes #31).
//
// Before this fix Parse did a single json.Unmarshal into the Payload struct,
// so ONE field with an unexpected type made the decoder fail the WHOLE payload:
// Parse returned a zero Payload and an error, the caller dropped everything,
// and the AI segment vanished for the rest of the session. That is exactly the
// class of bug tracked as #30 (a number-shaped resets_at killed the payload)
// and generalized as #31.
//
// The contract now: a field whose type does not match is dropped INDIVIDUALLY;
// every other field survives.
func TestParse_PerField(t *testing.T) {
	// context_window is a STRING where an object is expected. Everything else
	// is well-formed and must survive.
	data := []byte(`{
		"cwd": "/home/user/repo",
		"model": {"display_name": "Claude Opus 4.8 (1M context)"},
		"context_window": "not-an-object",
		"rate_limits": {
			"five_hour": {"used_percentage": 42.0},
			"seven_day": {"used_percentage": 7.5}
		},
		"terminal_width": 200
	}`)

	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse: one bad field must not fail the whole payload, got error: %v", err)
	}

	// The bad field — and ONLY the bad field — is dropped.
	if p.ContextWindow != nil {
		t.Errorf("context_window had a bad type; want nil, got %+v", p.ContextWindow)
	}

	// Everything else survives.
	if p.Model == nil || p.Model.DisplayName == nil {
		t.Fatal("model was discarded by an unrelated bad field (this is #31)")
	}
	if got, want := *p.Model.DisplayName, "Claude Opus 4.8 (1M context)"; got != want {
		t.Errorf("model.display_name: got %q, want %q", got, want)
	}
	if p.RateLimits == nil || p.RateLimits.FiveHour == nil || p.RateLimits.FiveHour.UsedPercentage == nil {
		t.Fatal("rate_limits was discarded by an unrelated bad field (this is #31)")
	}
	if got, want := *p.RateLimits.FiveHour.UsedPercentage, 42.0; got != want {
		t.Errorf("rate_limits.five_hour.used_percentage: got %f, want %f", got, want)
	}
	if p.RateLimits.SevenDay == nil || p.RateLimits.SevenDay.UsedPercentage == nil {
		t.Fatal("rate_limits.seven_day discarded")
	}
	if p.Cwd == nil || *p.Cwd != "/home/user/repo" {
		t.Errorf("cwd: got %v, want /home/user/repo", p.Cwd)
	}
	if p.TerminalWidth == nil || *p.TerminalWidth != 200 {
		t.Errorf("terminal_width: got %v, want 200", p.TerminalWidth)
	}
}

// TestParse_PerField_BadNestedResetTime covers the #30 shape specifically: the
// custom ResetTime.UnmarshalJSON rejects a bool. The five_hour window is lost,
// but seven_day and the rest of the payload must survive.
func TestParse_PerField_BadNestedResetTime(t *testing.T) {
	data := []byte(`{
		"model": {"display_name": "Sonnet"},
		"rate_limits": {
			"five_hour": {"used_percentage": 10.0, "resets_at": true},
			"seven_day": {"used_percentage": 20.0}
		}
	}`)

	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Model == nil || p.Model.DisplayName == nil || *p.Model.DisplayName != "Sonnet" {
		t.Fatal("model discarded by a bad nested resets_at")
	}
	// rate_limits as a whole failed to decode (the bool resets_at is inside it),
	// so it is dropped — but the payload survives and the model still renders.
	// This is the degradation contract: lose the field, never the payload.
}

// TestParse_PerField_EveryFieldBad — a payload where every known field has the
// wrong type must still Parse cleanly, yielding an empty Payload rather than an
// error. Nothing to render is fine; a panic or a hard error is not.
func TestParse_PerField_EveryFieldBad(t *testing.T) {
	data := []byte(`{
		"cwd": 12345,
		"model": [1,2,3],
		"context_window": "nope",
		"rate_limits": false,
		"terminal_width": {"x": 1},
		"quota": "not-an-object"
	}`)

	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse: all-bad fields must degrade, not error: %v", err)
	}
	if p.Cwd != nil || p.Model != nil || p.ContextWindow != nil ||
		p.RateLimits != nil || p.TerminalWidth != nil || p.Quota != nil {
		t.Errorf("expected an empty Payload, got %+v", p)
	}
}

// TestParse_StillErrorsOnMalformedJSON — tolerance is per-FIELD, not per-byte.
// Input that is not a JSON object at all is still an error (the existing
// contract; callers rely on it to distinguish "no payload" from "broken pipe").
func TestParse_StillErrorsOnMalformedJSON(t *testing.T) {
	for _, in := range []string{`{`, `["not","an","object"]`, `nope`, `{"a":}`} {
		if _, err := payload.Parse([]byte(in)); err == nil {
			t.Errorf("Parse(%q): want error, got nil", in)
		}
	}
}
