# gsl resets_at Type Mismatch Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `gsl` tolerate Claude Code's numeric `rate_limits.*.resets_at` so the entire payload no longer fails to parse and the AI segment keeps rendering on every refresh.

**Architecture:** `internal/payload/payload.go` decodes Claude's stdin JSON into a `Payload` tree of pointer fields; a single type mismatch on `RateWindow.ResetsAt` (declared `*string`) makes `json.Unmarshal` reject the whole document, after which `cmd/render.go` degrades to an empty `Payload{}` and `internal/render/seg_ai.go` self-omits the AI segment. Since `ResetsAt` is never read in rendering (`ratePart` only reads `used_percentage`), we change its type to `json.RawMessage`, which accepts any JSON shape (number, string, or null) without ever failing to unmarshal — making parsing future-proof against further upstream type changes.

**Tech Stack:** Go, encoding/json

---

Closes #30.

## Background facts (verified against the real files)

- Module path: `github.com/wenlock/dotfiles/gsl`
- Field to change: `internal/payload/payload.go`, struct `RateWindow`, field `ResetsAt *string` → `ResetsAt json.RawMessage` (tag `json:"resets_at"`).
- `json` is already imported in `payload.go`.
- `ResetsAt` is read **nowhere** outside `payload.go`/tests. Confirmed via grep: only `internal/payload/payload.go` declares it; `internal/render/seg_ai.go` `ratePart` reads only `UsedPercentage`.
- The existing test `payload_test.go::TestParseFullFixture` (lines 61–63) asserts on `*p.RateLimits.FiveHour.ResetsAt == "2026-05-25T10:00:00Z"`. Changing the type to `json.RawMessage` requires updating that assertion (RawMessage of a JSON string includes the surrounding quotes).
- Real captured payload from issue #30 (numeric `resets_at`):

  ```json
  "rate_limits": {
    "five_hour": {"used_percentage": 28.99, "resets_at": 1779863400},
    "seven_day": {"used_percentage": 32,    "resets_at": 1780052400}
  }
  ```

- Test command (run from `src/gsl`): `go test ./internal/payload/` for the focused package, `go test ./...` for the full suite.

---

## Task 1 — Add the regression fixture (numeric resets_at)

- [ ] Create the file `internal/payload/testdata/numeric_resets_at.json` with the real captured payload from issue #30. Numeric `resets_at` in BOTH windows. COMPLETE contents:

  ```json
  {
    "cwd": "/home/user/project",
    "model": {
      "display_name": "claude-sonnet-4-5"
    },
    "context_window": {
      "used_percentage": 42.5,
      "total_input_tokens": 8500,
      "context_window_size": 200000
    },
    "rate_limits": {
      "five_hour": {
        "used_percentage": 28.99,
        "resets_at": 1779863400
      },
      "seven_day": {
        "used_percentage": 32,
        "resets_at": 1780052400
      }
    }
  }
  ```

- [ ] Commit: `test(gsl): add numeric resets_at regression fixture for #30`

## Task 2 — Write the failing regression test

- [ ] Append a new test to `internal/payload/payload_test.go`. COMPLETE code (paste at end of file):

  ```go
  // TestParseNumericResetsAt is the regression test for issue #30: Claude Code
  // sends rate_limits.*.resets_at as a Unix-epoch NUMBER, not an RFC3339 string.
  // The whole payload must still parse (no error) and used_percentage must be
  // available for both windows.
  func TestParseNumericResetsAt(t *testing.T) {
  	data := readFixture(t, "numeric_resets_at.json")
  	p, err := payload.Parse(data)
  	if err != nil {
  		t.Fatalf("Parse(numeric_resets_at.json) returned error: %v", err)
  	}

  	if p.RateLimits == nil {
  		t.Fatal("RateLimits is nil — entire payload was rejected (the #30 bug)")
  	}
  	if p.RateLimits.FiveHour == nil || p.RateLimits.FiveHour.UsedPercentage == nil {
  		t.Fatal("RateLimits.FiveHour.UsedPercentage is nil")
  	}
  	if *p.RateLimits.FiveHour.UsedPercentage != 28.99 {
  		t.Errorf("FiveHour.UsedPercentage: got %v, want 28.99", *p.RateLimits.FiveHour.UsedPercentage)
  	}
  	if p.RateLimits.SevenDay == nil || p.RateLimits.SevenDay.UsedPercentage == nil {
  		t.Fatal("RateLimits.SevenDay.UsedPercentage is nil")
  	}
  	if *p.RateLimits.SevenDay.UsedPercentage != 32 {
  		t.Errorf("SevenDay.UsedPercentage: got %v, want 32", *p.RateLimits.SevenDay.UsedPercentage)
  	}
  }
  ```

- [ ] Run: `cd src/gsl && go test ./internal/payload/`
- [ ] EXPECTED (RED): test fails with a parse error similar to:

  ```
  --- FAIL: TestParseNumericResetsAt (0.00s)
      payload_test.go:NNN: Parse(numeric_resets_at.json) returned error: payload: parse: json: cannot unmarshal number into Go struct field RateWindow.rate_limits.five_hour.resets_at of type string
  FAIL
  ```

- [ ] Commit: `test(gsl): failing regression for numeric resets_at (#30)`

## Task 3 — Change ResetsAt to json.RawMessage (make it GREEN)

- [ ] Edit `internal/payload/payload.go`. Replace the `ResetsAt` field declaration in `RateWindow`:

  FROM:
  ```go
  	// ResetsAt is the RFC3339 timestamp at which the window resets.
  	ResetsAt *string `json:"resets_at"`
  ```

  TO:
  ```go
  	// ResetsAt is the time at which the window resets. Claude Code sends this
  	// as a Unix-epoch NUMBER (e.g. 1779863400), while older/other producers may
  	// send an RFC3339 string. It is not used in rendering (ratePart reads only
  	// UsedPercentage), so we accept any JSON shape via json.RawMessage to keep
  	// parsing future-proof and prevent a single field from rejecting the whole
  	// payload. See issue #30.
  	ResetsAt json.RawMessage `json:"resets_at"`
  ```

- [ ] Run: `cd src/gsl && go test ./internal/payload/`
- [ ] EXPECTED (still RED): `TestParseFullFixture` now fails on the ResetsAt string-equality assertion (lines 61–63), because `json.RawMessage` of a JSON string carries the surrounding quotes. This is the next task. `TestParseNumericResetsAt` should now PASS.

- [ ] Commit: `fix(gsl): accept numeric resets_at via json.RawMessage (#30)`

## Task 4 — Update the existing full-fixture assertion for the new type

- [ ] Edit `internal/payload/payload_test.go`. Replace the FiveHour `ResetsAt` assertion block.

  FROM:
  ```go
  	if p.RateLimits.FiveHour.ResetsAt == nil || *p.RateLimits.FiveHour.ResetsAt != "2026-05-25T10:00:00Z" {
  		t.Errorf("RateLimits.FiveHour.ResetsAt: got %v", p.RateLimits.FiveHour.ResetsAt)
  	}
  ```

  TO:
  ```go
  	// ResetsAt is now json.RawMessage; a JSON string fixture preserves the
  	// surrounding quotes verbatim.
  	if string(p.RateLimits.FiveHour.ResetsAt) != `"2026-05-25T10:00:00Z"` {
  		t.Errorf("RateLimits.FiveHour.ResetsAt: got %s", p.RateLimits.FiveHour.ResetsAt)
  	}
  ```

- [ ] Run: `cd src/gsl && go test ./internal/payload/`
- [ ] EXPECTED (GREEN):

  ```
  ok  	github.com/wenlock/dotfiles/gsl/internal/payload	0.00Xs
  ```

- [ ] Commit: `test(gsl): adapt full-fixture ResetsAt assertion to RawMessage (#30)`

## Task 5 — Full suite + vet

- [ ] Run: `cd src/gsl && go vet ./... && go test ./...`
- [ ] EXPECTED (GREEN): every package reports `ok` (or `[no test files]`), no `FAIL`.
- [ ] Commit (if vet/build produced any incidental changes; otherwise skip): `chore(gsl): go vet clean after #30 fix`

---

## Notes / out of scope

- Issue #31 ("one bad field shouldn't nuke the whole payload") is closely related but a separate, broader hardening task. This plan only fixes the `resets_at` type mismatch; it does not introduce a per-field tolerant decoder.
- `json.RawMessage` is preferred over `*int64` because it accepts numbers, strings, and null without code changes, future-proofing against further upstream shape changes. The value is never read, so storing it raw costs nothing.
