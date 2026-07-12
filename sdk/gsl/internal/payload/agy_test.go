package payload_test

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/payload"
)

// readFixture is defined in payload_test.go (same package).

// TestParse_AgyLive (E21) parses the CAPTURED real Antigravity payload.
//
// Provenance of testdata/agy_live.json — this is a real capture, not an
// invention. It was taken on 2026-07-11 from agy v1.1.1 (a Go binary,
// google3/third_party/jetski/cli) by temporarily replacing the statusLine
// command configured in ~/.gemini/antigravity-cli/settings.json
// ("bash ~/.gemini/config/statusline-command.sh") with a shim that tee'd
// stdin to a file, then running `agy -p "..."`. 14 payloads were captured
// across the session; this is the steady-state one. The ONLY edits are
// privacy redactions: the "email" value, the session/conversation UUIDs, and
// the $HOME path components. Every KEY, every value SHAPE, and all quota
// numbers are byte-faithful to what agy actually sent.
//
// The capture disproved two assumptions baked into payload.go by PR #157:
//   - each quota bucket carries reset_time + reset_in_seconds, which the
//     hardcoded struct dropped on the floor;
//   - agy never sends rate_limits at all, so the quota synthesis is the ONLY
//     source of the AI segment's rate data.
func TestParse_AgyLive(t *testing.T) {
	p, err := payload.Parse(readFixture(t, "agy_live.json"))
	if err != nil {
		t.Fatalf("Parse(agy_live.json): unexpected error: %v", err)
	}

	// The four bucket keys agy actually sends.
	wantBuckets := []string{"3p-5h", "3p-weekly", "gemini-5h", "gemini-weekly"}
	for _, k := range wantBuckets {
		if _, ok := p.Quota[k]; !ok {
			t.Errorf("quota bucket %q missing from parsed payload (have %v)", k, p.Quota)
		}
	}
	if got, want := len(p.Quota), len(wantBuckets); got != want {
		t.Errorf("quota bucket count: got %d, want %d", got, want)
	}

	// reset_time is REAL and must survive. The pre-fix struct modelled only
	// remaining_fraction, so this assertion is the red.
	gw, ok := p.Quota["gemini-weekly"]
	if !ok {
		t.Fatal("gemini-weekly bucket missing")
	}
	if gw.ResetTime == nil {
		t.Fatal("gemini-weekly.reset_time was dropped (nil); the capture has it")
	}
	if got, want := gw.ResetTime.String(), "2026-07-14T14:49:20Z"; got != want {
		t.Errorf("gemini-weekly.reset_time: got %q, want %q", got, want)
	}
	if gw.ResetInSeconds == nil || *gw.ResetInSeconds != 227553 {
		t.Errorf("gemini-weekly.reset_in_seconds: got %v, want 227553", gw.ResetInSeconds)
	}

	// agy sends NO rate_limits; the AI segment depends entirely on synthesis.
	if p.RateLimits == nil {
		t.Fatal("RateLimits not synthesized from quota")
	}
	if p.RateLimits.SevenDay == nil || p.RateLimits.SevenDay.UsedPercentage == nil {
		t.Fatal("SevenDay not synthesized")
	}
	// gemini-weekly remaining_fraction 0.86283696 → used ≈ 13.716%
	if got := *p.RateLimits.SevenDay.UsedPercentage; got < 13.7 || got > 13.73 {
		t.Errorf("SevenDay.UsedPercentage: got %f, want ≈13.716", got)
	}
	// The synthesized window must carry the reset timestamp through.
	if p.RateLimits.SevenDay.ResetsAt == nil {
		t.Error("SevenDay.ResetsAt is nil; reset_time from the quota bucket was not threaded through")
	} else if got, want := p.RateLimits.SevenDay.ResetsAt.String(), "2026-07-14T14:49:20Z"; got != want {
		t.Errorf("SevenDay.ResetsAt: got %q, want %q", got, want)
	}

	if p.TerminalWidth == nil || *p.TerminalWidth != 80 {
		t.Errorf("terminal_width: got %v, want 80", p.TerminalWidth)
	}
}

// TestParse_AgyLiveAuthenticating parses the real early-session capture where
// agy sends "model": null, "current_usage": null, no quota, and no plan_tier.
// A null field must not poison the rest of the payload.
func TestParse_AgyLiveAuthenticating(t *testing.T) {
	p, err := payload.Parse(readFixture(t, "agy_live_authenticating.json"))
	if err != nil {
		t.Fatalf("Parse(agy_live_authenticating.json): unexpected error: %v", err)
	}
	if p.Model != nil {
		t.Errorf("model was null in the capture; want nil, got %+v", p.Model)
	}
	if len(p.Quota) != 0 {
		t.Errorf("quota absent in the capture; want empty, got %v", p.Quota)
	}
	// Fields that WERE present must still land.
	if p.Cwd == nil || *p.Cwd != "/home/user/git/dotfiles" {
		t.Errorf("cwd: got %v, want /home/user/git/dotfiles", p.Cwd)
	}
	if p.TerminalWidth == nil || *p.TerminalWidth != 80 {
		t.Errorf("terminal_width: got %v, want 80", p.TerminalWidth)
	}
}

// TestParse_QuotaBucketRename (E21 edge: "unknown bucket key → still rendered")
// is the regression guard for the brittleness PR #157 shipped: the bucket keys
// were HARDCODED, so an upstream rename would silently delete the quota display
// rather than degrade. Buckets are now classified by suffix heuristic, so keys
// this code has never seen still resolve to a window.
func TestParse_QuotaBucketRename(t *testing.T) {
	// Plausibly-renamed upstream keys — none of them are the four literals.
	data := []byte(`{
		"quota": {
			"gemini-3-hourly":  {"remaining_fraction": 0.25},
			"gemini-3-weekly":  {"remaining_fraction": 0.50}
		}
	}`)
	p, err := payload.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(p.Quota) != 2 {
		t.Errorf("renamed buckets dropped: got %d buckets, want 2 (%v)", len(p.Quota), p.Quota)
	}

	if p.RateLimits == nil {
		t.Fatal("RateLimits nil: a bucket-key rename silently deleted the quota display")
	}
	if p.RateLimits.FiveHour == nil || p.RateLimits.FiveHour.UsedPercentage == nil {
		t.Fatal("FiveHour not synthesized from the renamed *-hourly bucket")
	}
	if got, want := *p.RateLimits.FiveHour.UsedPercentage, 75.0; got < want-0.01 || got > want+0.01 {
		t.Errorf("FiveHour.UsedPercentage: got %f, want %f", got, want)
	}
	if p.RateLimits.SevenDay == nil || p.RateLimits.SevenDay.UsedPercentage == nil {
		t.Fatal("SevenDay not synthesized from the renamed *-weekly bucket")
	}
	if got, want := *p.RateLimits.SevenDay.UsedPercentage, 50.0; got < want-0.01 || got > want+0.01 {
		t.Errorf("SevenDay.UsedPercentage: got %f, want %f", got, want)
	}
}

// TestClassifyQuotaBucket pins the suffix heuristic (F15).
func TestClassifyQuotaBucket(t *testing.T) {
	cases := []struct {
		key  string
		want payload.QuotaKind
	}{
		// The four keys agy actually sends today.
		{"3p-5h", payload.QuotaFiveHour},
		{"gemini-5h", payload.QuotaFiveHour},
		{"3p-weekly", payload.QuotaSevenDay},
		{"gemini-weekly", payload.QuotaSevenDay},
		// Suffix heuristic — keys we have never seen.
		{"gemini-hourly", payload.QuotaFiveHour},
		{"anything-hour", payload.QuotaFiveHour},
		{"claude-7d", payload.QuotaSevenDay},
		{"opus-week", payload.QuotaSevenDay},
		{"UPPER-WEEKLY", payload.QuotaSevenDay},
		// "week" must win over "hour" when both appear.
		{"168-hour-weekly", payload.QuotaSevenDay},
		// Genuinely unclassifiable.
		{"monthly", payload.QuotaUnknown},
		{"", payload.QuotaUnknown},
	}
	for _, tc := range cases {
		if got := payload.ClassifyQuotaBucket(tc.key); got != tc.want {
			t.Errorf("ClassifyQuotaBucket(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// TestParse_QuotaSynthesisIsDeterministic guards the map iteration order.
// Map ranging is randomized in Go; synthesis must not flap between the
// "gemini" and "3p" buckets from render to render.
func TestParse_QuotaSynthesisIsDeterministic(t *testing.T) {
	data := readFixture(t, "agy_live.json")
	var first float64
	for i := range 50 {
		p, err := payload.Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if p.RateLimits == nil || p.RateLimits.SevenDay == nil || p.RateLimits.SevenDay.UsedPercentage == nil {
			t.Fatal("SevenDay not synthesized")
		}
		got := *p.RateLimits.SevenDay.UsedPercentage
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("synthesis flapped across iterations: got %f, first %f "+
				"(map iteration order leaked into the result)", got, first)
		}
	}
	// gemini-weekly (0.86283696) must win over 3p-weekly (1.0) → ≈13.7, not 0.
	if first < 13.7 || first > 13.73 {
		t.Errorf("expected the gemini-* bucket to win (≈13.716%% used), got %f", first)
	}
}
