// Package payload provides defensive parsing of the JSON payload that
// Claude Code pipes to gsl on stdin after every assistant turn.
//
// All struct fields are pointers so absent/null fields are distinguishable
// from zero values. Callers MUST test for nil before dereferencing any field.
//
// Empty or whitespace-only input returns an empty Payload and a nil error —
// this is the "gsl is invoked without a Claude payload" case (e.g. Antigravity
// on-demand calls, integration tests, plain shell usage).
package payload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/observe"
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
	// ID is the model identifier (Claude Code: e.g. "claude-fable-5-1";
	// Antigravity repeats the display name here). The model-page link derives
	// its family from it, falling back to DisplayName.
	ID          *string `json:"id"`
	DisplayName *string `json:"display_name"`
}

// QuotaWindow holds the quota data for one bucket in the Antigravity payload.
//
// Shape confirmed against a real agy v1.1.1 capture (see
// testdata/agy_live.json and the provenance note on TestParse_AgyLive):
//
//	"gemini-weekly": {
//	  "remaining_fraction": 0.86283696,
//	  "reset_time": "2026-07-14T14:49:20Z",
//	  "reset_in_seconds": 227553
//	}
//
// PR #157 modelled only remaining_fraction and dropped the other two.
type QuotaWindow struct {
	RemainingFraction *float64   `json:"remaining_fraction"`
	ResetTime         *ResetTime `json:"reset_time"`
	ResetInSeconds    *float64   `json:"reset_in_seconds"`
}

// Quotas maps an Antigravity quota-bucket key to its window.
//
// It is deliberately a MAP and not a struct with four hardcoded keys. agy
// v1.1.1 sends "3p-5h", "3p-weekly", "gemini-5h" and "gemini-weekly", but
// those keys are agy's private naming, not a contract: they encode a model
// vendor ("3p" = third-party, "gemini" = first-party) and a window length,
// both of which move as Google ships models. With the previous hardcoded
// struct an upstream rename would have decoded to all-nil and SILENTLY
// deleted the quota display. Buckets are now classified by suffix heuristic
// (ClassifyQuotaBucket), so an unrecognised key still resolves to a window
// and the segment degrades gracefully instead of disappearing.
type Quotas map[string]QuotaWindow

// QuotaKind is the rolling window a quota bucket belongs to.
type QuotaKind int

const (
	// QuotaUnknown means the bucket key matched no window heuristic.
	QuotaUnknown QuotaKind = iota
	// QuotaFiveHour is the short rolling window (agy: "*-5h").
	QuotaFiveHour
	// QuotaSevenDay is the long rolling window (agy: "*-weekly").
	QuotaSevenDay
)

// ClassifyQuotaBucket maps a quota-bucket key to its rolling window using a
// suffix heuristic (F15):
//
//	contains "week" or "7d"          → QuotaSevenDay
//	contains "5h"   or "hour"        → QuotaFiveHour
//	otherwise                        → QuotaUnknown
//
// The seven-day test runs FIRST so a hypothetical "168-hour-weekly" is
// classified as the long window rather than the short one.
func ClassifyQuotaBucket(key string) QuotaKind {
	k := strings.ToLower(key)
	if strings.Contains(k, "week") || strings.Contains(k, "7d") {
		return QuotaSevenDay
	}
	if strings.Contains(k, "5h") || strings.Contains(k, "hour") {
		return QuotaFiveHour
	}
	return QuotaUnknown
}

// pick returns the quota bucket to use for the given window.
//
// Map iteration order is randomised in Go, so selection must be deterministic
// or the rendered percentage would flap between the "gemini" and "3p" buckets
// from one render to the next. Candidate keys are sorted, and a first-party
// ("gemini") bucket wins over any other — preserving the precedence PR #157
// established with its explicit if/else chain.
func (q Quotas) pick(kind QuotaKind) (QuotaWindow, bool) {
	var keys []string
	for k := range q {
		if ClassifyQuotaBucket(k) == kind {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return QuotaWindow{}, false
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.Contains(strings.ToLower(k), "gemini") {
			return q[k], true
		}
	}
	return q[keys[0]], true
}

// toRateWindow converts an agy quota bucket into the RateWindow shape the
// renderer consumes. remaining_fraction (1.0 = untouched) inverts into a
// used percentage; reset_time carries through to ResetsAt.
func (w QuotaWindow) toRateWindow() *RateWindow {
	if w.RemainingFraction == nil {
		return nil
	}
	used := (1.0 - *w.RemainingFraction) * 100.0
	rw := &RateWindow{UsedPercentage: &used}
	if w.ResetTime != nil && !w.ResetTime.Time().IsZero() {
		rt := *w.ResetTime
		rw.ResetsAt = &rt
	}
	return rw
}

// Payload is the top-level structure parsed from Claude's stdin JSON.
// All fields are pointers; an entirely empty/whitespace stdin yields a
// zero-valued Payload (all nil) and no error.
//
// IMPORTANT: every field here must have a matching decodeField call in Parse.
// A field added to the struct but not to Parse decodes to nil FOREVER, silently.
// TestParse_EveryFieldIsDecoded (reflection over the json tags) is the guard.
type Payload struct {
	Cwd           *string        `json:"cwd"`
	Model         *Model         `json:"model"`
	ContextWindow *ContextWindow `json:"context_window"`
	RateLimits    *RateLimits    `json:"rate_limits"`
	TerminalWidth *int           `json:"terminal_width"`
	Quota         Quotas         `json:"quota"`

	// Product is the host tool's self-identification. agy v1.1.1 sends
	// "antigravity" in EVERY payload (confirmed in the live capture — see
	// testdata/agy_live.json); Claude Code sends no such key.
	//
	// This is the only reliable in-band discriminator between the two hosts.
	// Both CLIs invoke the SAME shim (`gsl render`, payload on stdin), and the
	// payload shapes overlap almost completely, so without this key an agy
	// render is indistinguishable from a Claude render and gets the Claude
	// theme resolved against ~/.claude/settings.json. See cmd.deriveToolCtx.
	Product *string `json:"product"`
}

// IsAntigravity reports whether the payload came from the Antigravity CLI,
// using the in-band `product` key agy sends on every render.
func (p Payload) IsAntigravity() bool {
	return p.Product != nil && strings.EqualFold(strings.TrimSpace(*p.Product), "antigravity")
}

// decodeField decodes one top-level field into dst, IN ISOLATION.
//
// This is the heart of the per-field tolerance contract (F14 / #31). A field
// whose JSON type does not match its Go type is DROPPED — dst is left at its
// zero value and the failure is logged at Debug — while every other field in
// the payload still decodes. Absent and null fields are no-ops.
//
// Before this existed, Parse ran a single json.Unmarshal over the whole
// Payload struct, so the first type mismatch aborted the entire decode: the
// caller got a zero Payload and the AI segment vanished for the rest of the
// session. That was #30 (a number-shaped resets_at) and, generalised, #31.
func decodeField[T any](raw map[string]json.RawMessage, key string, dst *T) {
	r, ok := raw[key]
	if !ok {
		return
	}
	trimmed := bytes.TrimSpace(r)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return
	}
	var v T
	if err := json.Unmarshal(trimmed, &v); err != nil {
		// Drop ONLY this field. The payload survives.
		observe.Default().WithField("field", key).
			WithError(err).
			Debug("payload: dropping field with unexpected type")
		return
	}
	*dst = v
}

// Parse decodes a Claude Code / Antigravity stdin JSON payload.
//
// Empty or whitespace-only input returns an empty Payload{} and a nil error.
// Input that is not a JSON object returns a non-nil error.
//
// Tolerance is per-FIELD, not per-byte: once the input is a valid JSON object,
// each known field is decoded independently and a field with an unexpected
// type is dropped on its own rather than discarding the whole payload (F14 /
// #31). See decodeField.
func Parse(data []byte) (Payload, error) {
	if strings.TrimSpace(string(data)) == "" {
		return Payload{}, nil
	}

	// One shallow decode establishes that this IS a JSON object; field values
	// stay raw so each can be decoded (and fail) independently.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Payload{}, fmt.Errorf("payload: parse: %w", err)
	}

	var p Payload
	decodeField(raw, "cwd", &p.Cwd)
	decodeField(raw, "model", &p.Model)
	decodeField(raw, "context_window", &p.ContextWindow)
	decodeField(raw, "rate_limits", &p.RateLimits)
	decodeField(raw, "terminal_width", &p.TerminalWidth)
	decodeField(raw, "quota", &p.Quota)
	decodeField(raw, "product", &p.Product)

	// Antigravity sends `quota` and NO `rate_limits` at all (confirmed across
	// 14 captured agy v1.1.1 payloads), so synthesis is the ONLY source of the
	// AI segment's rate data under agy. Buckets are matched by heuristic, not
	// by hardcoded key.
	if p.RateLimits == nil && len(p.Quota) > 0 {
		rl := &RateLimits{}
		if w, ok := p.Quota.pick(QuotaFiveHour); ok {
			rl.FiveHour = w.toRateWindow()
		}
		if w, ok := p.Quota.pick(QuotaSevenDay); ok {
			rl.SevenDay = w.toRateWindow()
		}
		if rl.FiveHour != nil || rl.SevenDay != nil {
			p.RateLimits = rl
		}
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
