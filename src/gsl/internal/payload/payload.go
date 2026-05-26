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
