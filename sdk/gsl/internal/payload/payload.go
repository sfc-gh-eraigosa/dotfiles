// Package payload provides defensive parsing of the JSON payload that
// Claude Code pipes to gsl on stdin after every assistant turn.
//
// All struct fields are pointers so absent/null fields are distinguishable
// from zero values. Callers MUST test for nil before dereferencing any field.
//
// Empty or whitespace-only input returns an empty Payload and a nil error —
// this is the "gsl is invoked without a Claude payload" case (e.g. Gemini
// on-demand calls, integration tests, plain shell usage).
package payload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// NewResetTime wraps a time.Time as a ResetTime. Convenience constructor
// for test fixtures and any future caller that needs to synthesize a
// ResetTime from a known time value.
func NewResetTime(t time.Time) ResetTime { return ResetTime{t: t} }

// ResetTime is a rate-limit reset timestamp that tolerates BOTH wire shapes
// Claude Code has shipped to date:
//
//   - An ISO-8601 / RFC3339 string ("2026-05-25T10:00:00Z") — the form the
//     gsl payload schema originally assumed.
//   - A Unix-epoch number — the form the live Claude Code statusLine command
//     actually sends (see issue #30 for the captured payload). Numbers ≥ 1e12
//     are interpreted as milliseconds; smaller magnitudes as seconds.
//
// Before this type existed the JSON decoder failed the whole payload on a
// number-shaped resets_at and the AI segment vanished for the rest of the
// session — the silent regression tracked as #30.
//
// String() returns the canonical RFC3339 representation, or "" for a
// zero-valued ResetTime (absent / null source).
type ResetTime struct {
	t time.Time
}

// Time returns the underlying time.Time (zero value when absent/null).
func (r ResetTime) Time() time.Time { return r.t }

// String returns the canonical RFC3339 (UTC) representation, or "" when
// the source was absent or null. This is what downstream renderers and
// tests should compare against — never reach into the unexported field.
func (r ResetTime) String() string {
	if r.t.IsZero() {
		return ""
	}
	return r.t.UTC().Format(time.RFC3339)
}

// UnmarshalJSON accepts a string (RFC3339) or a JSON number (epoch
// seconds; epoch milliseconds when the magnitude is ≥ 1e12).
func (r *ResetTime) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("resets_at string: %w", err)
		}
		if s == "" {
			return nil
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("resets_at parse %q: %w", s, err)
		}
		r.t = t
		return nil
	}
	var n float64
	if err := json.Unmarshal(trimmed, &n); err != nil {
		return fmt.Errorf("resets_at number: %w", err)
	}
	sec := int64(n)
	nsec := int64((n - float64(sec)) * 1e9)
	if sec >= 1_000_000_000_000 { // ≥ 1e12 → milliseconds
		sec, nsec = sec/1000, (sec%1000)*1_000_000
	}
	r.t = time.Unix(sec, nsec).UTC()
	return nil
}

// RateWindow holds the rate-limit data for one rolling window (five-hour or
// seven-day). All fields are pointers to distinguish absent/null.
type RateWindow struct {
	// UsedPercentage is the fraction of the window's quota consumed (0–100).
	// Pointer so a JSON null value results in nil, not 0.
	UsedPercentage *float64 `json:"used_percentage"`
	// ResetsAt is the timestamp at which the window resets. Tolerates both
	// the original string (RFC3339) form and the live numeric (epoch) form.
	// See ResetTime for details and issue #30 for the motivating payload.
	ResetsAt *ResetTime `json:"resets_at"`
}

// RateLimits groups the rate-limit windows present in the Claude payload.
// Each field is a pointer: absent windows result in nil, not a zero-valued
// RateWindow.
type RateLimits struct {
	FiveHour *RateWindow `json:"five_hour"`
	SevenDay *RateWindow `json:"seven_day"`
}

// ContextWindow holds token-usage data for the current context.
type ContextWindow struct {
	// UsedPercentage is 0–100. Pointer so JSON null → nil (not 0).
	UsedPercentage    *float64 `json:"used_percentage"`
	TotalInputTokens  *float64 `json:"total_input_tokens"`
	ContextWindowSize *float64 `json:"context_window_size"`
}

// Model holds the model metadata from the Claude payload.
type Model struct {
	DisplayName *string `json:"display_name"`
}

// Payload is the top-level structure parsed from Claude's stdin JSON.
// All fields are pointers; an entirely empty/whitespace stdin yields a
// zero-valued Payload (all nil) and no error.
type Payload struct {
	Cwd           *string        `json:"cwd"`
	Model         *Model         `json:"model"`
	ContextWindow *ContextWindow `json:"context_window"`
	RateLimits    *RateLimits    `json:"rate_limits"`
}

// Parse decodes a Claude stdin JSON payload from the given byte slice.
// Empty or whitespace-only input returns an empty Payload{} and a nil error.
// Malformed JSON returns a non-nil error.
func Parse(data []byte) (Payload, error) {
	if strings.TrimSpace(string(data)) == "" {
		return Payload{}, nil
	}
	var p Payload
	if err := json.Unmarshal(data, &p); err != nil {
		return Payload{}, fmt.Errorf("payload: parse: %w", err)
	}
	return p, nil
}

// ParseReader reads all bytes from r then calls Parse. An empty reader
// (e.g. an os.Stdin with nothing piped) returns an empty Payload and no
// error, matching Parse's empty-input contract.
func ParseReader(r io.Reader) (Payload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Payload{}, fmt.Errorf("payload: read: %w", err)
	}
	return Parse(data)
}
