# gsl Payload Per-Field Decode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `payload.Parse` decode `model`, `context_window`, and `rate_limits` independently so a single malformed sub-object is skipped rather than discarding the entire payload.

**Architecture:** `Parse` first unmarshals the top-level JSON object into a `map[string]json.RawMessage`. It then unmarshals each known field (`cwd`, `model`, `context_window`, `rate_limits`) from its own `RawMessage` into the existing pointer-typed struct fields. A unit (field) that fails to decode is left nil and its error is collected into a `[]FieldError` for future logging (per #32); failure of one field never aborts the others. The "never break the line" contract is preserved: top-level non-object JSON (or an empty input) still returns an empty `Payload`, and partial data still renders downstream in `cmd/render.go`.

**Tech Stack:** Go, encoding/json

---

Closes #31.

## Context (read these first — paths are real)

- `src/gsl/internal/payload/payload.go` — current `Parse`/`ParseReader` + struct defs (`Payload`, `Model`, `ContextWindow`, `RateLimits`, `RateWindow`). All fields are pointers.
- `src/gsl/internal/payload/payload_test.go` — existing tests (full fixture, malformed JSON, empty stdin, null used_percentage, five-hour-only, reader variants).
- `src/gsl/internal/payload/testdata/{full,five_hour_only,null_used_pct}.json` — fixtures.
- `src/gsl/cmd/render.go` — `runRender` calls `payload.ParseReader(os.Stdin)`; on error it degrades to `payload.Payload{}`. This is the all-or-nothing blast radius we are shrinking.
- Module path: `github.com/wenlock/dotfiles/gsl`.

### Current Parse entry point (the thing we are changing)

```go
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
```

The problem: `json.Unmarshal(data, &p)` is all-or-nothing. A wrong-typed leaf (e.g. `rate_limits.five_hour.used_percentage: "oops"`) fails the whole unmarshal, so `model` and `context_window` are lost too.

### Behavior contract we MUST preserve

1. Empty / whitespace-only input → empty `Payload{}`, nil error.
2. Top-level malformed JSON (not even a JSON object, e.g. `{not valid json`) → empty `Payload{}`, non-nil error (render.go degrades gracefully).
3. A valid top-level object with one bad sub-object → the bad field is nil, the good fields are populated, **and** `Parse` returns a nil error (partial success is not a failure; the bad field is recorded in `Payload.FieldErrors` for #32).

---

## Task 1 — Add a failing test: bad `rate_limits` must not lose `model`/`context_window`

- [ ] Create fixture `src/gsl/internal/payload/testdata/bad_rate_limits.json` with valid `model` + `context_window` but a wrong-typed `rate_limits` leaf:

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
      "used_percentage": "not-a-number",
      "resets_at": "2026-05-25T10:00:00Z"
    }
  }
}
```

- [ ] Append this test to `src/gsl/internal/payload/payload_test.go`:

```go
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
```

- [ ] Run and confirm RED (this references `p.FieldErrors`, which does not exist yet, so it is a COMPILE failure first — that is expected and acceptable as the failing-first signal):

```
cd src/gsl && go test ./internal/payload/
```

Expected (compile error, then once FieldErrors exists but Parse is still all-or-nothing, an assertion failure):

```
# github.com/wenlock/dotfiles/gsl/internal/payload [github.com/wenlock/dotfiles/gsl/internal/payload.test]
./payload_test.go:NNN: p.FieldErrors undefined (type payload.Payload has no field or method FieldErrors)
FAIL	github.com/wenlock/dotfiles/gsl/internal/payload [build failed]
```

- [ ] Commit: `test(gsl): add failing per-field decode test for bad rate_limits (#31)`

## Task 2 — Add the `FieldError` type and `FieldErrors` field

- [ ] In `src/gsl/internal/payload/payload.go`, add a `FieldError` type and a `FieldErrors` slice to `Payload`. Insert the type just above the `Payload` struct:

```go
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
```

- [ ] Add `FieldErrors` to the `Payload` struct. It is NOT a JSON field — tag it `json:"-"` so it is ignored on the (now removed) whole-struct unmarshal and on any future re-marshal:

```go
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
```

- [ ] Run and confirm the test now compiles but still FAILS on behavior (Parse is still all-or-nothing, so the bad rate_limits still aborts the whole unmarshal → model/context_window come back nil and no FieldErrors are recorded):

```
cd src/gsl && go test ./internal/payload/
```

Expected (assertion failure, not a compile error):

```
--- FAIL: TestParseBadRateLimitsPreservesOtherFields (0.00s)
    payload_test.go:NNN: Parse(bad_rate_limits.json) returned error: payload: parse: ...
FAIL	github.com/wenlock/dotfiles/gsl/internal/payload	0.00s
FAIL
```

- [ ] Commit: `feat(gsl): add FieldError type for non-fatal per-field decode (#31)`

## Task 3 — Refactor `Parse` to per-field `json.RawMessage` decoding

- [ ] Replace the body of `Parse` in `src/gsl/internal/payload/payload.go` with a per-field decode. The new `Parse`:
  1. Returns empty `Payload{}` + nil for empty/whitespace input (unchanged).
  2. Unmarshals the top level into `map[string]json.RawMessage`. If THAT fails (top-level not a JSON object), returns empty `Payload{}` + the error (preserves contract #2).
  3. Decodes each known field from its own `RawMessage` via the `decodeField` helper; a per-field failure records a `FieldError` and leaves that field nil — it does NOT abort.
  4. Returns the (possibly partial) `Payload` with a nil error.

```go
// Parse decodes a Claude stdin JSON payload from the given byte slice.
//
// Empty or whitespace-only input returns an empty Payload{} and a nil error.
// Top-level malformed JSON (not a JSON object) returns an empty Payload{}
// and a non-nil error.
//
// Each known top-level field (model, context_window, rate_limits, cwd) is
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

// decodeField unmarshals the raw[key] message into dst. An absent key is a
// no-op (dst stays nil). A present-but-malformed value records a FieldError
// in *errs and leaves dst nil — it never aborts the surrounding Parse.
func decodeField[T any](raw map[string]json.RawMessage, key string, dst **T, errs *[]FieldError) {
	msg, ok := raw[key]
	if !ok {
		return
	}
	var v T
	if err := json.Unmarshal(msg, &v); err != nil {
		*errs = append(*errs, FieldError{Field: key, Err: err})
		return
	}
	*dst = &v
}
```

Note: `decodeField` is generic over the pointed-to type. For `cwd` the dst is `**string`; for `model` it is `**Model`; etc. A JSON `null` for a field unmarshals into the zero value `v` and sets `*dst = &v` (a non-nil pointer to a zero struct/string) — that matches the prior whole-struct behavior where `"model": null` left `p.Model == nil`... so DOUBLE-CHECK this in Task 4. If a JSON `null` must yield a nil pointer (it must, to match `TestParseNullUsedPercentage`'s sibling expectations and the package doc), guard against the literal `null` token. Update `decodeField`:

```go
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
```

- [ ] Run the package tests and confirm GREEN:

```
cd src/gsl && go test ./internal/payload/
```

Expected:

```
ok  	github.com/wenlock/dotfiles/gsl/internal/payload	0.00s
```

- [ ] Commit: `fix(gsl): decode payload fields independently so one bad sub-object is non-fatal (#31)`

## Task 4 — Full suite + regression guard

- [ ] Run the entire module test suite and confirm nothing regressed (especially `TestParseNullUsedPercentage`, `TestParseEmptyStdin`, `TestParseMalformedJSON`, `TestParseFullFixture`, and the `cmd` render tests that depend on `ParseReader`):

```
cd src/gsl && go test ./...
```

Expected (all `ok`, no `FAIL`):

```
?   	github.com/wenlock/dotfiles/gsl	[no test files]
ok  	github.com/wenlock/dotfiles/gsl/cmd	...
ok  	github.com/wenlock/dotfiles/gsl/internal/payload	...
... (all packages ok) ...
```

- [ ] `go vet` the payload package to catch generic/pointer mistakes:

```
cd src/gsl && go vet ./internal/payload/
```

Expected: no output (clean).

- [ ] Commit: `test(gsl): confirm per-field decode keeps full suite green (#31)`
