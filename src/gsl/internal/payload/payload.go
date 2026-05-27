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
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RateWindow holds the rate-limit data for one rolling window (five-hour or
// seven-day). All fields are pointers to distinguish absent/null.
type RateWindow struct {
	// UsedPercentage is the fraction of the window's quota consumed (0–100).
	// Pointer so a JSON null value results in nil, not 0.
	UsedPercentage *float64 `json:"used_percentage"`
	// ResetsAt is the RFC3339 timestamp at which the window resets.
	ResetsAt *string `json:"resets_at"`
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

// FieldError records a single top-level field that failed to decode during
// a per-field Parse. It is non-fatal: the field is left nil and the rest of
// the payload still parses. Collected for future structured logging (#32).
type FieldError struct {
	// Field is the JSON key that failed (e.g. "rate_limits").
	Field string
	// Err is the underlying decode error.
	Err error
}

func (e FieldError) Error() string {
	return fmt.Sprintf("payload: field %q: %v", e.Field, e.Err)
}

// Payload is the top-level structure parsed from Claude's stdin JSON.
// All fields are pointers; an entirely empty/whitespace stdin yields a
// zero-valued Payload (all nil) and no error.
type Payload struct {
	Cwd           *string        `json:"cwd"`
	Model         *Model         `json:"model"`
	ContextWindow *ContextWindow `json:"context_window"`
	RateLimits    *RateLimits    `json:"rate_limits"`

	// FieldErrors holds non-fatal per-field decode failures. Populated by
	// Parse when a top-level field is present but malformed; the field is
	// left nil and parsing continues (issue #31, logging per #32).
	FieldErrors []FieldError `json:"-"`
}

// Parse decodes a Claude stdin JSON payload from the given byte slice.
//
// Empty or whitespace-only input returns an empty Payload{} and a nil error.
// Top-level malformed JSON (not a JSON object) returns an empty Payload{}
// and a non-nil error.
//
// Each known top-level field (cwd, model, context_window, rate_limits) is
// decoded INDEPENDENTLY: a single malformed sub-object is skipped (left nil)
// and recorded in Payload.FieldErrors, never discarding the fields that did
// parse. This shrinks the blast radius of one bad sub-field (issue #31).
func Parse(data []byte) (Payload, error) {
	if strings.TrimSpace(string(data)) == "" {
		return Payload{}, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Payload{}, fmt.Errorf("payload: parse: %w", err)
	}

	var p Payload
	decodeField(raw, "cwd", &p.Cwd, &p.FieldErrors)
	decodeField(raw, "model", &p.Model, &p.FieldErrors)
	decodeField(raw, "context_window", &p.ContextWindow, &p.FieldErrors)
	decodeField(raw, "rate_limits", &p.RateLimits, &p.FieldErrors)
	return p, nil
}

// decodeField unmarshals the raw[key] message into dst. An absent key or an
// explicit JSON null is a no-op (dst stays nil). A present-but-malformed
// value records a FieldError in *errs and leaves dst nil — it never aborts
// the surrounding Parse.
func decodeField[T any](raw map[string]json.RawMessage, key string, dst **T, errs *[]FieldError) {
	msg, ok := raw[key]
	if !ok || string(msg) == "null" {
		return
	}
	var v T
	if err := json.Unmarshal(msg, &v); err != nil {
		*errs = append(*errs, FieldError{Field: key, Err: err})
		return
	}
	*dst = &v
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
