package payload_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// FuzzParse (E20) fuzzes the payload decoder.
//
// Parse is the prime fuzz target in gsl: it is the only place that consumes
// untrusted bytes from another process's stdout, and ResetTime has a CUSTOM
// UnmarshalJSON that hand-rolls number/string discrimination and does epoch
// arithmetic (int64 conversion, a millisecond branch, time.Unix) — exactly the
// shape that panics on a hostile input.
//
// The contract under fuzz:
//   - Parse never panics, for any input.
//   - Parse never returns both a non-nil error AND a populated Payload.
//   - A Payload that comes back without an error is internally consistent
//     (its ResetTime values can be stringified without blowing up).
func FuzzParse(f *testing.F) {
	// Seed with the real captures + every hand-written fixture.
	for _, name := range []string{
		"agy_live.json",
		"agy_live_authenticating.json",
		"full.json",
		"five_hour_only.json",
		"live_numeric_resets.json",
		"null_used_pct.json",
	} {
		if data, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(data)
		}
	}

	// Seeds that target the custom ResetTime decoder and the per-field seam.
	f.Add([]byte(``))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"quota":{"x-5h":{"remaining_fraction":1e308}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":9223372036854775807}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":-9223372036854775808}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":1e308}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":-1e308}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":1783813000000}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":"2026-07-14T14:49:20Z"}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":""}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":null}}}`))
	f.Add([]byte(`{"rate_limits":{"five_hour":{"resets_at":NaN}}}`))
	f.Add([]byte(`{"context_window":"not-an-object","model":{"display_name":"x"}}`))
	f.Add([]byte(`{"terminal_width":-2147483648}`))
	f.Add([]byte(`{"quota":{"":{"remaining_fraction":null}}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := payload.Parse(data) // must not panic
		if err != nil {
			// On error the Payload must be the zero value — callers rely on
			// not having to distinguish "partially populated" from "empty".
			if p.Cwd != nil || p.Model != nil || p.ContextWindow != nil ||
				p.RateLimits != nil || p.TerminalWidth != nil || p.Quota != nil {
				t.Fatalf("Parse returned an error AND a populated payload: %+v", p)
			}
			return
		}

		// Touch every decoded value: stringifying a ResetTime must not panic
		// regardless of what epoch arithmetic produced.
		if p.RateLimits != nil {
			for _, w := range []*payload.RateWindow{p.RateLimits.FiveHour, p.RateLimits.SevenDay} {
				if w != nil && w.ResetsAt != nil {
					_ = w.ResetsAt.String()
					_ = w.ResetsAt.Time()
				}
			}
		}
		for k, w := range p.Quota {
			_ = payload.ClassifyQuotaBucket(k)
			if w.ResetTime != nil {
				_ = w.ResetTime.String()
			}
		}
	})
}
