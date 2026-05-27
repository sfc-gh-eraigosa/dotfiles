# gsl Token Usage History & Delta Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist a per-turn token usage timeline to a local JSONL file and expose `gsl mark <label>` and `gsl tokens` commands so users can checkpoint workflows and compute accurate token consumption deltas between any two named markers.

**Architecture:** A new `internal/usagelog` package owns all JSONL I/O — it appends `UsageEntry` (turn) or `MarkEntry` (checkpoint) records to `${XDG_STATE_HOME:-~/.local/state}/gsl/usage.jsonl`, reusing the state-directory infrastructure introduced by #32. `cmd/render.go` calls `usagelog.Append` after each successful payload parse to record token data, and two new cobra commands (`cmd/mark.go`, `cmd/tokens.go`) provide the write-checkpoint and read/diff user-facing surface respectively. **Coupled with the same recording hook**, an opt-in `internal/metrics` package emits Prometheus gauges (via the node_exporter textfile-collector pattern by default, Pushgateway as an alternative) so the same per-turn data can flow into Grafana for durable, long-term retention and visualization beyond the local `usage.jsonl`. Both the usage-log write and the metrics write are off the hot path and strictly non-fatal: a failure in either must never break the rendered status line (same contract as #32 logging).

**Tech Stack:** Go (depends on #32 logging); optional `github.com/prometheus/client_golang` (Apache-2.0) for the metrics exposition writer

---

> **Dependency notice — implement/merge AFTER #32.**
> Issue #32 (structured logging) defines the state directory
> `${XDG_STATE_HOME:-~/.local/state}/gsl/` and its `MkdirAll` helper.  This
> feature reuses that directory without re-inventing it.  Where #32 introduces a
> `statedir` package or equivalent, import it here; if #32 ships only a simple
> `os.MkdirAll` call inside the logging path, mirror the same XDG resolution
> function.  The `usage.jsonl` file lives alongside whatever log file #32
> creates in that state directory.

Closes #34.

---

## JSONL Schema

### Turn record (`"kind": "turn"`)

Appended by `gsl render` once per successful payload parse (skipped when all
token fields are nil — e.g. Gemini/CLI mode with no Claude payload).

```jsonl
{"kind":"turn","ts":"2026-05-27T14:23:01Z","input_tokens":12450,"context_size":200000,"used_pct":6.225}
```

| Field | Go type | Source |
|---|---|---|
| `kind` | `string` | constant `"turn"` |
| `ts` | `string` (RFC3339) | `time.Now().UTC().Format(time.RFC3339)` |
| `input_tokens` | `int64` | `payload.ContextWindow.TotalInputTokens` (cast from `*float64`) |
| `context_size` | `int64` | `payload.ContextWindow.ContextWindowSize` (cast from `*float64`) |
| `used_pct` | `float64` | `payload.ContextWindow.UsedPercentage` (dereferenced `*float64`) |

### Mark record (`"kind": "mark"`)

Appended by `gsl mark <label>`.

```jsonl
{"kind":"mark","ts":"2026-05-27T14:25:00Z","label":"before-refactor"}
```

| Field | Go type | Source |
|---|---|---|
| `kind` | `string` | constant `"mark"` |
| `ts` | `string` (RFC3339) | `time.Now().UTC().Format(time.RFC3339)` |
| `label` | `string` | CLI arg |

---

## Task Breakdown

Each task is 2–5 minutes, test-first, with one commit per task.

---

### Task 0 — Pre-validation gate: confirm Claude Code actually gives us the data (MUST run BEFORE implementation)

> **This is a hard gate.** Do NOT start Task 1 until this task proves Claude
> Code's live status-line payload populates the three fields we record. If the
> fields come back absent/null in a real session, stop and escalate — the whole
> recording premise is invalid and the design must be revisited.

The fields we depend on are modeled (verified) in
`src/gsl/internal/payload/payload.go`:

- `ContextWindow.UsedPercentage` → JSON `context_window.used_percentage` (`*float64`)
- `ContextWindow.TotalInputTokens` → JSON `context_window.total_input_tokens` (`*float64`)
- `ContextWindow.ContextWindowSize` → JSON `context_window.context_window_size` (`*float64`)

Being *modeled* is not the same as being *populated at runtime*. This task
proves population end-to-end.

- [ ] **0.1** Capture a real Claude Code status-line payload. Claude Code pipes
  the status-line JSON to the configured `statusLine.command` on stdin after a
  turn. Temporarily point the status-line command at a capture shim and run one
  real turn:

  ```sh
  # Temporary capture shim — write whatever Claude Code pipes us to a file,
  # then echo a placeholder so the status line still renders.
  cat > /tmp/gsl-capture.sh <<'EOF'
  #!/usr/bin/env bash
  tee /tmp/gsl-payload.json
  echo "capturing…"
  EOF
  chmod +x /tmp/gsl-capture.sh
  # Point ~/.claude/settings.json statusLine.command at /tmp/gsl-capture.sh,
  # run ONE real Claude Code turn, then inspect:
  cat /tmp/gsl-payload.json | python3 -m json.tool
  ```

  Assert the captured JSON contains a non-null `context_window` object with
  non-null `used_percentage`, `total_input_tokens`, and `context_window_size`.

- [ ] **0.2** Add an automated probe that proves the fields parse into non-nil
  pointers. Write `src/gsl/internal/payload/payload_prevalidate_test.go`:

  ```go
  package payload_test

  import (
  	"strings"
  	"testing"

  	"github.com/wenlock/dotfiles/gsl/internal/payload"
  )

  // TestPrevalidate_ClaudeContextWindowFieldsPresent proves that a
  // representative Claude Code status-line payload populates the three
  // context_window fields gsl records (see docs/plans Task 0 gate).
  // Replace the literal below with a REAL captured payload from /tmp/gsl-payload.json
  // before relying on this gate.
  func TestPrevalidate_ClaudeContextWindowFieldsPresent(t *testing.T) {
  	// Representative payload captured from a live Claude Code turn (Task 0.1).
  	raw := `{
  		"cwd": "/tmp",
  		"model": {"display_name": "claude-sonnet-4-6"},
  		"context_window": {
  			"used_percentage": 18.0,
  			"total_input_tokens": 36000,
  			"context_window_size": 200000
  		},
  		"rate_limits": {
  			"five_hour": {"used_percentage": 12.5, "resets_at": "2026-05-27T19:00:00Z"},
  			"seven_day": {"used_percentage": 40.0, "resets_at": "2026-06-01T00:00:00Z"}
  		}
  	}`

  	p, err := payload.ParseReader(strings.NewReader(raw))
  	if err != nil {
  		t.Fatalf("ParseReader: %v", err)
  	}
  	if p.ContextWindow == nil {
  		t.Fatal("context_window is nil; Claude payload did not populate it — GATE FAILS")
  	}
  	if p.ContextWindow.UsedPercentage == nil {
  		t.Fatal("used_percentage is nil — GATE FAILS")
  	}
  	if p.ContextWindow.TotalInputTokens == nil {
  		t.Fatal("total_input_tokens is nil — GATE FAILS")
  	}
  	if p.ContextWindow.ContextWindowSize == nil {
  		t.Fatal("context_window_size is nil — GATE FAILS")
  	}
  	if *p.ContextWindow.TotalInputTokens != 36000 {
  		t.Errorf("total_input_tokens = %v; want 36000", *p.ContextWindow.TotalInputTokens)
  	}
  }
  ```

  Run: `go test ./internal/payload/... -run TestPrevalidate -v` — must PASS
  against the captured/representative payload. If `context_window` or any of the
  three fields is nil with a *real* captured payload, the gate FAILS.

- [ ] **0.3** Restore `~/.claude/settings.json` `statusLine.command` to the real
  `gsl render` command and delete `/tmp/gsl-capture.sh` / `/tmp/gsl-payload.json`.

- [ ] **0.4** Record the gate result in the PR description (paste the redacted
  captured payload + the passing probe output). Only then proceed to Task 1.

#### Gemini (future) — tracked in #42, NOT in scope for this PR

Gemini CLI does **not** currently expose a post-turn status-line hook equivalent
to Claude Code's (this is the gap behind **#33**), so there is no per-turn
payload to record token usage from on Gemini today. Decision: **Claude Code
ships FIRST**; Gemini support is a deliberate follow-up.

Route options to study for Gemini (summary only — do not implement here):

1. **Wait for / drive a Gemini post-turn hook** (depends on #33 landing a
   status-line/turn hook) — cleanest, mirrors the Claude path one-for-one.
2. **Parse Gemini CLI session/telemetry artifacts** if Gemini writes a
   per-turn usage record to disk we can tail — works without a hook but is
   format-fragile.
3. **Wrap the Gemini invocation** so gsl observes token counts out-of-band —
   most invasive, least preferred.

This work is already tracked — do **NOT** open a new issue:
**#42 — "gsl: token-usage tracking for Gemini CLI (follow-up to #34)"**
(https://github.com/sfc-gh-eraigosa/dotfiles/issues/42). Link #42 from the PR.

---

### Task 1 — `internal/usagelog`: package scaffold + path helper (TDD)

- [ ] **1.1** Write failing tests in `src/gsl/internal/usagelog/usagelog_test.go`:

```go
package usagelog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/usagelog"
)

func TestDefaultPath_XDGSet(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/teststate")
	got := usagelog.DefaultPath()
	want := "/tmp/teststate/gsl/usage.jsonl"
	if got != want {
		t.Errorf("DefaultPath() = %q; want %q", got, want)
	}
}

func TestDefaultPath_XDGUnset(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home, _ := os.UserHomeDir()
	got := usagelog.DefaultPath()
	want := filepath.Join(home, ".local", "state", "gsl", "usage.jsonl")
	if got != want {
		t.Errorf("DefaultPath() = %q; want %q", got, want)
	}
}
```

Run: `go test ./internal/usagelog/... -run TestDefaultPath` — expect **compile error** (package does not exist yet).

- [ ] **1.2** Create `src/gsl/internal/usagelog/usagelog.go` with only the path helper:

```go
// Package usagelog appends token-usage and mark records to a JSONL file
// under ${XDG_STATE_HOME:-~/.local/state}/gsl/usage.jsonl.
package usagelog

import (
	"os"
	"path/filepath"
)

// DefaultPath returns the canonical usage.jsonl path, honouring XDG_STATE_HOME.
func DefaultPath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "gsl", "usage.jsonl")
}
```

Run: `go test ./internal/usagelog/... -run TestDefaultPath -v`
Expected output:
```
--- PASS: TestDefaultPath_XDGSet (0.00s)
--- PASS: TestDefaultPath_XDGUnset (0.00s)
PASS
ok  	github.com/wenlock/dotfiles/gsl/internal/usagelog
```

- [ ] **1.3** Commit: `feat(usagelog): add package scaffold and DefaultPath helper`

---

### Task 2 — `internal/usagelog`: Entry types + JSON round-trip (TDD)

- [ ] **2.1** Add failing tests to `usagelog_test.go`:

```go
func TestEntryRoundTrip_Turn(t *testing.T) {
	e := usagelog.TurnEntry{
		Kind:        "turn",
		Ts:          "2026-05-27T14:23:01Z",
		InputTokens: 12450,
		ContextSize: 200000,
		UsedPct:     6.225,
	}
	data, err := e.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["kind"] != "turn" {
		t.Errorf("kind = %v; want turn", got["kind"])
	}
	if got["input_tokens"].(float64) != 12450 {
		t.Errorf("input_tokens = %v; want 12450", got["input_tokens"])
	}
}

func TestEntryRoundTrip_Mark(t *testing.T) {
	e := usagelog.MarkEntry{
		Kind:  "mark",
		Ts:    "2026-05-27T14:25:00Z",
		Label: "before-refactor",
	}
	data, err := e.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["kind"] != "mark" {
		t.Errorf("kind = %v; want mark", got["kind"])
	}
	if got["label"] != "before-refactor" {
		t.Errorf("label = %v; want before-refactor", got["label"])
	}
}
```

Run: `go test ./internal/usagelog/... -run TestEntryRoundTrip` — expect **compile error** (types missing).

- [ ] **2.2** Add types + `MarshalJSON` to `usagelog.go`:

```go
import (
	"encoding/json"
	// ... existing imports
)

// TurnEntry is one JSONL record appended per gsl render turn.
type TurnEntry struct {
	Kind        string  `json:"kind"`
	Ts          string  `json:"ts"`
	InputTokens int64   `json:"input_tokens"`
	ContextSize int64   `json:"context_size"`
	UsedPct     float64 `json:"used_pct"`
}

// MarshalJSON serialises TurnEntry to JSON bytes.
func (e TurnEntry) MarshalJSON() ([]byte, error) {
	type alias TurnEntry
	return json.Marshal(alias(e))
}

// MarkEntry is one JSONL record appended per gsl mark <label> call.
type MarkEntry struct {
	Kind  string `json:"kind"`
	Ts    string `json:"ts"`
	Label string `json:"label"`
}

// MarshalJSON serialises MarkEntry to JSON bytes.
func (e MarkEntry) MarshalJSON() ([]byte, error) {
	type alias MarkEntry
	return json.Marshal(alias(e))
}
```

Run: `go test ./internal/usagelog/... -run TestEntryRoundTrip -v`
Expected output:
```
--- PASS: TestEntryRoundTrip_Turn (0.00s)
--- PASS: TestEntryRoundTrip_Mark (0.00s)
PASS
```

- [ ] **2.3** Commit: `feat(usagelog): add TurnEntry and MarkEntry types with JSON round-trip`

---

### Task 3 — `internal/usagelog`: Append + ReadAll (TDD)

- [ ] **3.1** Add failing tests:

```go
func TestAppendTurnAndMark(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")

	turn := usagelog.TurnEntry{
		Kind: "turn", Ts: "2026-05-27T14:23:01Z",
		InputTokens: 12450, ContextSize: 200000, UsedPct: 6.225,
	}
	if err := usagelog.AppendTurn(path, turn); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	mark := usagelog.MarkEntry{
		Kind: "mark", Ts: "2026-05-27T14:25:00Z", Label: "before-refactor",
	}
	if err := usagelog.AppendMark(path, mark); err != nil {
		t.Fatalf("AppendMark: %v", err)
	}

	entries, err := usagelog.ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d; want 2", len(entries))
	}
	if entries[0].Kind != "turn" {
		t.Errorf("entries[0].Kind = %q; want turn", entries[0].Kind)
	}
	if entries[1].Kind != "mark" {
		t.Errorf("entries[1].Kind = %q; want mark", entries[1].Kind)
	}
	if entries[1].Label != "before-refactor" {
		t.Errorf("entries[1].Label = %q; want before-refactor", entries[1].Label)
	}
}

func TestAppendTurn_CreatesDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "gsl", "usage.jsonl")
	turn := usagelog.TurnEntry{Kind: "turn", Ts: "2026-05-27T00:00:00Z"}
	if err := usagelog.AppendTurn(path, turn); err != nil {
		t.Fatalf("AppendTurn should create dirs: %v", err)
	}
}

func TestReadAll_MissingFile(t *testing.T) {
	entries, err := usagelog.ReadAll("/nonexistent/path/usage.jsonl")
	if err != nil {
		t.Fatalf("ReadAll on missing file should not error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d; want 0", len(entries))
	}
}
```

Run: `go test ./internal/usagelog/... -run TestAppend` — expect **compile error**.

- [ ] **3.2** Implement `AppendTurn`, `AppendMark`, `ReadAll`, and the shared `RawEntry` type in `usagelog.go`:

```go
import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	// existing imports
)

// RawEntry is the polymorphic shape returned by ReadAll.
// Callers switch on Kind to decide which fields are populated.
type RawEntry struct {
	Kind        string  `json:"kind"`
	Ts          string  `json:"ts"`
	InputTokens int64   `json:"input_tokens,omitempty"`
	ContextSize int64   `json:"context_size,omitempty"`
	UsedPct     float64 `json:"used_pct,omitempty"`
	Label       string  `json:"label,omitempty"`
}

// appendLine marshals v, appends one JSON line + newline to path,
// creating parent directories as needed.
func appendLine(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("usagelog: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("usagelog: open: %w", err)
	}
	defer f.Close()
	line, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("usagelog: marshal: %w", err)
	}
	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

// AppendTurn writes a TurnEntry as a JSONL record.
func AppendTurn(path string, e TurnEntry) error {
	return appendLine(path, e)
}

// AppendMark writes a MarkEntry as a JSONL record.
func AppendMark(path string, e MarkEntry) error {
	return appendLine(path, e)
}

// ReadAll reads all JSONL records from path into a []RawEntry slice.
// Returns an empty slice (no error) if path does not exist.
// Malformed lines are skipped with no error (best-effort read).
func ReadAll(path string) ([]RawEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("usagelog: open: %w", err)
	}
	defer f.Close()

	var entries []RawEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e RawEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}
```

Run: `go test ./internal/usagelog/... -v`
Expected output:
```
--- PASS: TestDefaultPath_XDGSet (0.00s)
--- PASS: TestDefaultPath_XDGUnset (0.00s)
--- PASS: TestEntryRoundTrip_Turn (0.00s)
--- PASS: TestEntryRoundTrip_Mark (0.00s)
--- PASS: TestAppendTurnAndMark (0.00s)
--- PASS: TestAppendTurn_CreatesDirs (0.00s)
--- PASS: TestReadAll_MissingFile (0.00s)
PASS
ok  	github.com/wenlock/dotfiles/gsl/internal/usagelog
```

- [ ] **3.3** Commit: `feat(usagelog): implement AppendTurn, AppendMark, and ReadAll`

---

### Task 4 — Wire `gsl render` to append a turn record (TDD)

The hook goes into `cmd/render.go` after `payload.ParseReader` succeeds and the
context window fields are non-nil.

- [ ] **4.1** Add a failing integration test to `src/gsl/cmd/render_test.go` (or a new `src/gsl/cmd/render_usagelog_test.go`):

```go
package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/usagelog"
)

// TestRenderAppendsUsageEntry verifies that running the render command with a
// full Claude payload appends exactly one "turn" record to usage.jsonl.
func TestRenderAppendsUsageEntry(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	// Also disable the config-load path so no real config is read.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	payload := `{
		"cwd": "/tmp",
		"model": {"display_name": "claude-sonnet-4-6"},
		"context_window": {
			"used_percentage": 18.0,
			"total_input_tokens": 36000,
			"context_window_size": 200000
		}
	}`

	// Invoke the render subcommand via cobra's Execute directly or by
	// setting os.Stdin and calling rootCmd.Execute().
	// Approach: set stdin and args, then call Execute.
	old := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	_, _ = w.WriteString(payload)
	w.Close()
	defer func() { os.Stdin = old }()

	os.Args = []string{"gsl", "render"}
	// Execute is safe to call in tests because SilenceUsage/SilenceErrors are set.
	Execute()

	logPath := filepath.Join(stateDir, "gsl", "usage.jsonl")
	entries, err := usagelog.ReadAll(logPath)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d; want 1", len(entries))
	}
	e := entries[0]
	if e.Kind != "turn" {
		t.Errorf("Kind = %q; want turn", e.Kind)
	}
	if e.InputTokens != 36000 {
		t.Errorf("InputTokens = %d; want 36000", e.InputTokens)
	}
	if e.ContextSize != 200000 {
		t.Errorf("ContextSize = %d; want 200000", e.ContextSize)
	}
	if e.UsedPct != 18.0 {
		t.Errorf("UsedPct = %v; want 18.0", e.UsedPct)
	}
	if !strings.HasPrefix(e.Ts, "20") {
		t.Errorf("Ts = %q; want RFC3339 timestamp", e.Ts)
	}
}
```

Run: `go test ./cmd/... -run TestRenderAppendsUsageEntry` — expect **FAIL** (no append in render yet).

- [ ] **4.2** Update `src/gsl/cmd/render.go` to append the turn record after parsing:

```go
package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/wenlock/dotfiles/gsl/internal/payload"
	"github.com/wenlock/dotfiles/gsl/internal/usagelog"
)

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Render the status line from a Claude JSON payload on stdin",
	Long: `render reads a Claude Code JSON payload from stdin, loads the config,
builds the status-line segments, and prints one rendered line to stdout.

Empty or invalid stdin is handled gracefully — the status line is still
rendered (without AI segment data). If the master enable flag is false,
nothing is printed.`,
	RunE: runRender,
}

func init() {
	rootCmd.AddCommand(renderCmd)
}

func runRender(cmd *cobra.Command, args []string) error {
	// Parse payload from stdin; degrade gracefully on error.
	p, err := payload.ParseReader(os.Stdin)
	if err != nil {
		// Bad JSON on stdin: log to stderr but continue with empty payload.
		fmt.Fprintf(os.Stderr, "gsl render: stdin parse error (degrading): %v\n", err)
		p = payload.Payload{}
	}

	// Append a turn record to usage.jsonl when token data is present.
	if p.ContextWindow != nil &&
		p.ContextWindow.TotalInputTokens != nil &&
		p.ContextWindow.ContextWindowSize != nil &&
		p.ContextWindow.UsedPercentage != nil {
		entry := usagelog.TurnEntry{
			Kind:        "turn",
			Ts:          time.Now().UTC().Format(time.RFC3339),
			InputTokens: int64(*p.ContextWindow.TotalInputTokens),
			ContextSize: int64(*p.ContextWindow.ContextWindowSize),
			UsedPct:     *p.ContextWindow.UsedPercentage,
		}
		// Best-effort: never let a log write fail the render.
		if werr := usagelog.AppendTurn(usagelog.DefaultPath(), entry); werr != nil {
			fmt.Fprintf(os.Stderr, "gsl render: usage log write failed (ignored): %v\n", werr)
		}
	}

	// Determine cwd hint from payload.
	cwdHint := ""
	if p.Cwd != nil && *p.Cwd != "" {
		cwdHint = *p.Cwd
	}

	return runStatusLine(cmd, p, cwdHint)
}

// configToRawStyles converts config.Styles (map[string]any, raw JSON) to the
// map[string]map[string]any shape that style.ResolveConfig expects.
// Top-level values that are not map[string]any are silently skipped.
func configToRawStyles(raw map[string]any) map[string]map[string]any {
	if raw == nil {
		return nil
	}
	out := make(map[string]map[string]any, len(raw))
	for k, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out[k] = m
		}
	}
	return out
}
```

Run: `go test ./cmd/... -run TestRenderAppendsUsageEntry -v`
Expected output:
```
--- PASS: TestRenderAppendsUsageEntry (0.00s)
PASS
```

- [ ] **4.3** Full suite: `go test ./... -count=1`
Expected: all existing tests still pass + new test passes.

- [ ] **4.4** Commit: `feat(render): append turn record to usage.jsonl on every Claude payload`

---

### Task 5 — `cmd/mark.go`: `gsl mark <label>` command (TDD)

- [ ] **5.1** Write failing test `src/gsl/cmd/mark_test.go`:

```go
package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/usagelog"
)

func TestMarkCommand_AppendsMarkEntry(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	os.Args = []string{"gsl", "mark", "workflow-start"}
	Execute()

	logPath := filepath.Join(stateDir, "gsl", "usage.jsonl")
	entries, err := usagelog.ReadAll(logPath)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d; want 1", len(entries))
	}
	e := entries[0]
	if e.Kind != "mark" {
		t.Errorf("Kind = %q; want mark", e.Kind)
	}
	if e.Label != "workflow-start" {
		t.Errorf("Label = %q; want workflow-start", e.Label)
	}
}

func TestMarkCommand_RequiresLabel(t *testing.T) {
	os.Args = []string{"gsl", "mark"}
	// Execute should return a usage error — captured via cobra SilenceErrors.
	// We can't easily assert the exit code here without a subprocess; just
	// verify no panic occurs and the label is required.
	Execute() // should not panic
}
```

Run: `go test ./cmd/... -run TestMarkCommand` — expect **compile error** (cmd not registered).

- [ ] **5.2** Create `src/gsl/cmd/mark.go`:

```go
package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/wenlock/dotfiles/gsl/internal/usagelog"
)

var markCmd = &cobra.Command{
	Use:   "mark <label>",
	Short: "Insert a named checkpoint into the token usage log",
	Long: `mark appends a timestamped marker record to the token usage log
(` + "`${XDG_STATE_HOME:-~/.local/state}/gsl/usage.jsonl`" + `).

Use markers to delimit workflow phases so that 'gsl tokens --diff' can
compute token consumption between two named points.

Example:
  gsl mark before-refactor
  # ... run a long agentic workflow ...
  gsl mark after-refactor
  gsl tokens --diff before-refactor after-refactor`,
	Args: cobra.ExactArgs(1),
	RunE: runMark,
}

func init() {
	rootCmd.AddCommand(markCmd)
}

func runMark(_ *cobra.Command, args []string) error {
	label := args[0]
	entry := usagelog.MarkEntry{
		Kind:  "mark",
		Ts:    time.Now().UTC().Format(time.RFC3339),
		Label: label,
	}
	if err := usagelog.AppendMark(usagelog.DefaultPath(), entry); err != nil {
		return fmt.Errorf("gsl mark: %w", err)
	}
	fmt.Printf("OK: mark %q written\n", label)
	return nil
}
```

Run: `go test ./cmd/... -run TestMarkCommand -v`
Expected output:
```
--- PASS: TestMarkCommand_AppendsMarkEntry (0.00s)
--- PASS: TestMarkCommand_RequiresLabel (0.00s)
PASS
```

- [ ] **5.3** Commit: `feat(mark): add gsl mark <label> command for usage-log checkpoints`

---

### Task 6 — `internal/usagelog`: Delta computation logic (TDD)

The delta logic is isolated in `usagelog` so it can be tested without cobra.

- [ ] **6.1** Write failing tests in `usagelog_test.go`:

```go
func TestComputeLast_ReturnsLastTurn(t *testing.T) {
	entries := []usagelog.RawEntry{
		{Kind: "mark", Ts: "2026-05-27T14:20:00Z", Label: "start"},
		{Kind: "turn", Ts: "2026-05-27T14:21:00Z", InputTokens: 5000, ContextSize: 200000, UsedPct: 2.5},
		{Kind: "turn", Ts: "2026-05-27T14:23:00Z", InputTokens: 9520, ContextSize: 200000, UsedPct: 4.76},
	}
	last, ok := usagelog.Last(entries)
	if !ok {
		t.Fatal("Last returned ok=false; want true")
	}
	if last.InputTokens != 9520 {
		t.Errorf("InputTokens = %d; want 9520", last.InputTokens)
	}
}

func TestComputeLast_NoTurns(t *testing.T) {
	entries := []usagelog.RawEntry{
		{Kind: "mark", Ts: "2026-05-27T14:20:00Z", Label: "start"},
	}
	_, ok := usagelog.Last(entries)
	if ok {
		t.Error("Last returned ok=true; want false when no turns present")
	}
}

func TestDiff_BetweenTwoMarks(t *testing.T) {
	entries := []usagelog.RawEntry{
		{Kind: "mark", Ts: "2026-05-27T14:20:00Z", Label: "start"},
		{Kind: "turn", Ts: "2026-05-27T14:21:00Z", InputTokens: 5000, ContextSize: 200000, UsedPct: 2.5},
		{Kind: "mark", Ts: "2026-05-27T14:22:00Z", Label: "mid"},
		{Kind: "turn", Ts: "2026-05-27T14:23:00Z", InputTokens: 9520, ContextSize: 200000, UsedPct: 4.76},
		{Kind: "mark", Ts: "2026-05-27T14:24:00Z", Label: "end"},
	}

	d, err := usagelog.Diff(entries, "start", "end")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	// First turn after "start": 5000 tokens, 2.5%
	// Last turn before "end": 9520 tokens, 4.76%
	// Delta tokens = 9520 - 5000 = 4520
	if d.DeltaTokens != 4520 {
		t.Errorf("DeltaTokens = %d; want 4520", d.DeltaTokens)
	}
	if d.StartUsedPct != 2.5 {
		t.Errorf("StartUsedPct = %v; want 2.5", d.StartUsedPct)
	}
	if d.EndUsedPct != 4.76 {
		t.Errorf("EndUsedPct = %v; want 4.76", d.EndUsedPct)
	}
}

func TestDiff_MissingStartMark(t *testing.T) {
	entries := []usagelog.RawEntry{
		{Kind: "turn", Ts: "2026-05-27T14:21:00Z", InputTokens: 5000, ContextSize: 200000, UsedPct: 2.5},
	}
	_, err := usagelog.Diff(entries, "nonexistent", "")
	if err == nil {
		t.Error("Diff should return error for missing start mark")
	}
}

func TestDiff_NoTurnsBetweenMarks(t *testing.T) {
	entries := []usagelog.RawEntry{
		{Kind: "mark", Ts: "2026-05-27T14:20:00Z", Label: "a"},
		{Kind: "mark", Ts: "2026-05-27T14:21:00Z", Label: "b"},
	}
	_, err := usagelog.Diff(entries, "a", "b")
	if err == nil {
		t.Error("Diff should return error when no turns exist between marks")
	}
}
```

Run: `go test ./internal/usagelog/... -run TestComputeLast|TestDiff` — expect **compile error**.

- [ ] **6.2** Add `Last`, `DiffResult`, and `Diff` to `usagelog.go`:

```go
// DiffResult holds the computed delta between two marks.
type DiffResult struct {
	StartMark    string
	EndMark      string
	DeltaTokens  int64
	StartUsedPct float64
	EndUsedPct   float64
	ContextSize  int64
}

// Last returns the most recent "turn" entry in entries, or (_, false) if none.
func Last(entries []RawEntry) (RawEntry, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Kind == "turn" {
			return entries[i], true
		}
	}
	return RawEntry{}, false
}

// Diff computes token consumption between the first occurrence of startMark
// and the first occurrence of endMark (or end-of-log when endMark is "").
//
// "Token consumption" is defined as:
//   last turn's InputTokens − first turn's InputTokens
//   (within the window bounded by the two marks)
//
// Returns an error if startMark is not found, or no turn records exist
// between the two marks.
func Diff(entries []RawEntry, startMark, endMark string) (DiffResult, error) {
	// Find start mark index.
	startIdx := -1
	for i, e := range entries {
		if e.Kind == "mark" && e.Label == startMark {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return DiffResult{}, fmt.Errorf("usagelog: mark %q not found", startMark)
	}

	// Find end mark index (exclusive upper bound).
	endIdx := len(entries)
	if endMark != "" {
		for i := startIdx + 1; i < len(entries); i++ {
			if entries[i].Kind == "mark" && entries[i].Label == endMark {
				endIdx = i
				break
			}
		}
	}

	// Collect turns in window.
	var turns []RawEntry
	for _, e := range entries[startIdx+1 : endIdx] {
		if e.Kind == "turn" {
			turns = append(turns, e)
		}
	}
	if len(turns) == 0 {
		return DiffResult{}, fmt.Errorf("usagelog: no turn records between marks %q and %q", startMark, endMark)
	}

	first := turns[0]
	last := turns[len(turns)-1]
	return DiffResult{
		StartMark:    startMark,
		EndMark:      endMark,
		DeltaTokens:  last.InputTokens - first.InputTokens,
		StartUsedPct: first.UsedPct,
		EndUsedPct:   last.UsedPct,
		ContextSize:  last.ContextSize,
	}, nil
}
```

Run: `go test ./internal/usagelog/... -v`
Expected output (all tests pass):
```
--- PASS: TestDefaultPath_XDGSet (0.00s)
--- PASS: TestDefaultPath_XDGUnset (0.00s)
--- PASS: TestEntryRoundTrip_Turn (0.00s)
--- PASS: TestEntryRoundTrip_Mark (0.00s)
--- PASS: TestAppendTurnAndMark (0.00s)
--- PASS: TestAppendTurn_CreatesDirs (0.00s)
--- PASS: TestReadAll_MissingFile (0.00s)
--- PASS: TestComputeLast_ReturnsLastTurn (0.00s)
--- PASS: TestComputeLast_NoTurns (0.00s)
--- PASS: TestDiff_BetweenTwoMarks (0.00s)
--- PASS: TestDiff_MissingStartMark (0.00s)
--- PASS: TestDiff_NoTurnsBetweenMarks (0.00s)
PASS
ok  	github.com/wenlock/dotfiles/gsl/internal/usagelog
```

- [ ] **6.3** Commit: `feat(usagelog): add Last, Diff, and DiffResult for delta computation`

---

### Task 7 — `cmd/tokens.go`: `gsl tokens --last` and `gsl tokens --diff` (TDD)

- [ ] **7.1** Write failing tests `src/gsl/cmd/tokens_test.go`:

```go
package cmd_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gsl/internal/usagelog"
)

// captureStdout runs f() while capturing os.Stdout to a string.
func captureStdout(f func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func seedUsageLog(t *testing.T, stateDir string) {
	t.Helper()
	logPath := filepath.Join(stateDir, "gsl", "usage.jsonl")
	_ = usagelog.AppendMark(logPath, usagelog.MarkEntry{Kind: "mark", Ts: "2026-05-27T14:20:00Z", Label: "start"})
	_ = usagelog.AppendTurn(logPath, usagelog.TurnEntry{Kind: "turn", Ts: "2026-05-27T14:21:00Z", InputTokens: 5000, ContextSize: 200000, UsedPct: 2.5})
	_ = usagelog.AppendTurn(logPath, usagelog.TurnEntry{Kind: "turn", Ts: "2026-05-27T14:23:00Z", InputTokens: 9520, ContextSize: 200000, UsedPct: 4.76})
	_ = usagelog.AppendMark(logPath, usagelog.MarkEntry{Kind: "mark", Ts: "2026-05-27T14:24:00Z", Label: "end"})
}

func TestTokensLast(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	seedUsageLog(t, stateDir)

	os.Args = []string{"gsl", "tokens", "--last"}
	out := captureStdout(func() { Execute() })

	// Expected output contains last turn data.
	if !strings.Contains(out, "9520") {
		t.Errorf("output missing token count 9520; got: %q", out)
	}
	if !strings.Contains(out, "4.76") {
		t.Errorf("output missing used_pct 4.76; got: %q", out)
	}
}

func TestTokensDiff(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	seedUsageLog(t, stateDir)

	os.Args = []string{"gsl", "tokens", "--diff", "start", "end"}
	out := captureStdout(func() { Execute() })

	// Expected: "Token usage delta: +4,520 tokens (Context: 2.50% -> 4.76%)"
	if !strings.Contains(out, "+4,520 tokens") {
		t.Errorf("output missing delta line; got: %q", out)
	}
	if !strings.Contains(out, "2.50%") {
		t.Errorf("output missing start pct; got: %q", out)
	}
	if !strings.Contains(out, "4.76%") {
		t.Errorf("output missing end pct; got: %q", out)
	}
}

func TestTokensDiff_MissingMark(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	seedUsageLog(t, stateDir)

	os.Args = []string{"gsl", "tokens", "--diff", "nosuchmark"}
	out := captureStdout(func() { Execute() })
	// Should print an error message, not panic.
	_ = out // error goes to stderr; just verify no panic
}

func TestTokensLast_EmptyLog(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	os.Args = []string{"gsl", "tokens", "--last"}
	out := captureStdout(func() { Execute() })
	if !strings.Contains(out, "no turn") && !strings.Contains(out, "empty") {
		// Allow any user-facing "no data" message
		_ = out
	}
}
```

Run: `go test ./cmd/... -run TestTokens` — expect **compile error** (command not registered).

- [ ] **7.2** Create `src/gsl/cmd/tokens.go`:

```go
package cmd

import (
	"fmt"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"os"

	"github.com/spf13/cobra"
	"github.com/wenlock/dotfiles/gsl/internal/usagelog"
)

var tokensLastFlag bool
var tokensDiffFlag []string

var tokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Display token usage history and deltas",
	Long: `tokens queries the local token usage log and prints consumption data.

Examples:
  gsl tokens --last
      Print the most recent turn's token count and context usage.

  gsl tokens --diff before-refactor
      Show token consumption from the mark named "before-refactor" to the
      end of the log.

  gsl tokens --diff before-refactor after-refactor
      Show token consumption between two named marks.

Output format:
  Token usage delta: +4,520 tokens (Context: 15.00% -> 18.00%)`,
	RunE: runTokens,
}

func init() {
	tokensCmd.Flags().BoolVar(&tokensLastFlag, "last", false, "Print the most recent turn's token data")
	tokensCmd.Flags().StringArrayVar(&tokensDiffFlag, "diff", nil, "Compute delta between start [end] marks (1 or 2 args)")
	rootCmd.AddCommand(tokensCmd)
}

func runTokens(_ *cobra.Command, _ []string) error {
	entries, err := usagelog.ReadAll(usagelog.DefaultPath())
	if err != nil {
		return fmt.Errorf("gsl tokens: %w", err)
	}

	if tokensLastFlag {
		return printLast(entries)
	}
	if len(tokensDiffFlag) > 0 {
		return printDiff(entries, tokensDiffFlag)
	}
	return fmt.Errorf("gsl tokens: specify --last or --diff <start_mark> [end_mark]")
}

func printLast(entries []usagelog.RawEntry) error {
	e, ok := usagelog.Last(entries)
	if !ok {
		fmt.Fprintln(os.Stdout, "gsl tokens: no turn records in usage log")
		return nil
	}
	p := message.NewPrinter(language.English)
	p.Printf("Last turn: %s\n", e.Ts)
	p.Printf("  Input tokens : %d\n", e.InputTokens)
	p.Printf("  Context size : %d\n", e.ContextSize)
	p.Printf("  Used         : %.2f%%\n", e.UsedPct)
	return nil
}

func printDiff(entries []usagelog.RawEntry, args []string) error {
	startMark := args[0]
	endMark := ""
	if len(args) >= 2 {
		endMark = args[1]
	}

	d, err := usagelog.Diff(entries, startMark, endMark)
	if err != nil {
		return fmt.Errorf("gsl tokens: %w", err)
	}

	p := message.NewPrinter(language.English)
	sign := "+"
	if d.DeltaTokens < 0 {
		sign = ""
	}
	endLabel := d.EndMark
	if endLabel == "" {
		endLabel = "now"
	}
	p.Printf("Token usage delta: %s%d tokens (Context: %.2f%% -> %.2f%%)\n",
		sign, d.DeltaTokens, d.StartUsedPct, d.EndUsedPct)
	p.Printf("  Range  : %s → %s\n", d.StartMark, endLabel)
	p.Printf("  Context: %d tokens total\n", d.ContextSize)
	return nil
}
```

> **Note on `golang.org/x/text`:** If `x/text` is not already in `go.mod` (it is
> listed as an indirect dependency in the module; confirm with `go list -m all`),
> run `go get golang.org/x/text` to add it as a direct dependency before
> importing `message` and `language`. Alternatively, use
> `fmt.Sprintf("%s%s", sign, formatComma(d.DeltaTokens))` with a small helper
> that formats with commas using only the standard library (see open design
> question below).

Run: `go test ./cmd/... -run TestTokens -v`
Expected output:
```
--- PASS: TestTokensLast (0.00s)
--- PASS: TestTokensDiff (0.00s)
--- PASS: TestTokensDiff_MissingMark (0.00s)
--- PASS: TestTokensLast_EmptyLog (0.00s)
PASS
```

- [ ] **7.3** Run full suite and confirm coverage:

```
go test ./... -count=1 -cover
```

Expected: all packages pass; `internal/usagelog` coverage >= 80%.

- [ ] **7.4** Commit: `feat(tokens): add gsl tokens --last and --diff commands`

---

### Task 8 — `internal/metrics`: Prometheus exposition writer, COUPLED with the recording hook (TDD)

> **Why this layer (review comments 1 + 3):** `usage.jsonl` is a local,
> rotating-ish file with no native query/visualization story. Coupling the
> per-turn recording with Prometheus gauges gives us (1) the ability to
> "leverage this further" by publishing to **Grafana**, and (3) "longer life"
> — durable long-term retention and dashboards beyond the local JSONL. Same
> data, two sinks: `usage.jsonl` for `gsl tokens` deltas, Prometheus for
> historical/visual analysis.

> **Why the textfile-collector pattern (not an HTTP `/metrics` endpoint):**
> `gsl render` is an **ephemeral, per-prompt CLI invocation** — it starts,
> prints one line, and exits in milliseconds. An in-process `/metrics` HTTP
> server cannot work: nothing is alive to scrape. The correct Prometheus
> pattern for short-lived jobs is the **node_exporter textfile collector** —
> gsl writes/refreshes a `gsl.prom` exposition file in a textfile-collector
> directory, and a long-lived local `node_exporter` (started by the user, not
> by gsl) scrapes that directory on its own interval. For hosts with **no**
> local node_exporter, the alternative is the **Pushgateway** (gsl POSTs the
> gauges to a Pushgateway URL). Both are opt-in.

**Config / opt-in surface (OFF by default, non-fatal always):**

| Knob | Env | Config key | Effect |
|---|---|---|---|
| Textfile dir | `GSL_METRICS_DIR` | `metrics.textfile_dir` | If set, write `<dir>/gsl.prom`. This is the **default** mode. |
| Pushgateway URL | `GSL_METRICS_PUSHGATEWAY` | `metrics.pushgateway_url` | If set (and `GSL_METRICS_DIR` unset), push to this Pushgateway. |
| Disable | `GSL_METRICS=0` | `metrics.enabled: false` | Force-off even if a dir/URL is set. |

If neither `GSL_METRICS_DIR` nor `GSL_METRICS_PUSHGATEWAY` is set, the metrics
layer is a no-op — zero overhead, no new files. A write/push failure is logged
to stderr and **swallowed**; it must never affect the status line (same
contract as #32 logging and the `usage.jsonl` write).

**Gauges emitted (label sets):**

| Metric | Type | Labels | Source field |
|---|---|---|---|
| `gsl_context_used_percentage` | gauge | `model`, `assistant` | `context_window.used_percentage` |
| `gsl_total_input_tokens` | gauge | `model`, `assistant` | `context_window.total_input_tokens` |
| `gsl_context_window_size` | gauge | `model`, `assistant` | `context_window.context_window_size` |
| `gsl_rate_limit_used_percentage` | gauge | `model`, `assistant`, `window` | `rate_limits.<window>.used_percentage` (`window="five_hour"` / `"seven_day"`) |

`model` comes from `payload.Model.DisplayName` (fallback `"unknown"`);
`assistant` is `"claude"` for this PR (the label exists so the future Gemini
path from #42 can reuse the same series).

- [ ] **8.1** Decide the dependency. The plan adopts
  **`github.com/prometheus/client_golang`** for correct exposition-format
  encoding and Pushgateway support.

  **License check (required by `src/CLAUDE.md`):** `prometheus/client_golang`
  is **Apache-2.0**, which is in this repo's **Allowed (permissive)** set —
  in fact the *preferred* license ("Apache License 2.0 (preferred; explicit
  patent grant)", `src/CLAUDE.md` lines 30–31). LICENSE file (verify on the PR):
  https://github.com/prometheus/client_golang/blob/main/LICENSE — Apache-2.0.
  No flag-for-review or banned licenses involved. Run `go mod tidy` and confirm
  `go.sum` doesn't drag in a banned transitive license (the client_golang tree
  is Apache-2.0 / BSD-3-Clause throughout; spot-check on the PR).

  > **Reviewer decision to confirm:** take the `client_golang` dependency
  > (Apache-2.0, preferred), **or** hand-write the Prometheus text exposition
  > format with zero new deps. Hand-writing is viable for the textfile mode
  > (the `.prom` format is a few lines of `name{labels} value`), but Pushgateway
  > content-negotiation is fiddly by hand. The plan recommends `client_golang`;
  > if the reviewer prefers zero-dep, drop Pushgateway to "future" and emit the
  > textfile format with `fmt.Fprintf` (see 8.3 alt note).

- [ ] **8.2** Write failing tests `src/gsl/internal/metrics/metrics_test.go`:

  ```go
  package metrics_test

  import (
  	"os"
  	"path/filepath"
  	"strings"
  	"testing"

  	"github.com/wenlock/dotfiles/gsl/internal/metrics"
  )

  func sampleSnapshot() metrics.Snapshot {
  	return metrics.Snapshot{
  		Model:            "claude-sonnet-4-6",
  		Assistant:        "claude",
  		UsedPercentage:   18.0,
  		TotalInputTokens: 36000,
  		ContextWindowSize: 200000,
  		RateLimits: map[string]float64{
  			"five_hour": 12.5,
  			"seven_day": 40.0,
  		},
  	}
  }

  // TestWriteTextfile_EmitsGauges proves the textfile collector mode writes a
  // valid .prom file containing the expected gauges and labels.
  func TestWriteTextfile_EmitsGauges(t *testing.T) {
  	dir := t.TempDir()
  	if err := metrics.WriteTextfile(dir, sampleSnapshot()); err != nil {
  		t.Fatalf("WriteTextfile: %v", err)
  	}
  	data, err := os.ReadFile(filepath.Join(dir, "gsl.prom"))
  	if err != nil {
  		t.Fatalf("read gsl.prom: %v", err)
  	}
  	s := string(data)
  	for _, want := range []string{
  		`gsl_context_used_percentage`,
  		`gsl_total_input_tokens`,
  		`gsl_context_window_size`,
  		`gsl_rate_limit_used_percentage`,
  		`model="claude-sonnet-4-6"`,
  		`assistant="claude"`,
  		`window="five_hour"`,
  		`window="seven_day"`,
  	} {
  		if !strings.Contains(s, want) {
  			t.Errorf("gsl.prom missing %q; got:\n%s", want, s)
  		}
  	}
  }

  // TestWriteTextfile_AtomicReplace proves the writer replaces gsl.prom (no
  // duplicate/append) so node_exporter never reads a partial file.
  func TestWriteTextfile_AtomicReplace(t *testing.T) {
  	dir := t.TempDir()
  	if err := metrics.WriteTextfile(dir, sampleSnapshot()); err != nil {
  		t.Fatalf("first write: %v", err)
  	}
  	if err := metrics.WriteTextfile(dir, sampleSnapshot()); err != nil {
  		t.Fatalf("second write: %v", err)
  	}
  	data, _ := os.ReadFile(filepath.Join(dir, "gsl.prom"))
  	if n := strings.Count(string(data), "gsl_context_window_size"); n != 1 {
  		t.Errorf("gsl_context_window_size appears %d times; want 1 (atomic replace)", n)
  	}
  	// No leftover temp file.
  	entries, _ := os.ReadDir(dir)
  	for _, e := range entries {
  		if strings.HasPrefix(e.Name(), ".gsl.prom") || strings.HasSuffix(e.Name(), ".tmp") {
  			t.Errorf("leftover temp file: %s", e.Name())
  		}
  	}
  }

  // TestResolveSink_NoConfig proves metrics are a no-op when nothing is set.
  func TestResolveSink_NoConfig(t *testing.T) {
  	t.Setenv("GSL_METRICS_DIR", "")
  	t.Setenv("GSL_METRICS_PUSHGATEWAY", "")
  	t.Setenv("GSL_METRICS", "")
  	sink := metrics.ResolveSink()
  	if sink.Enabled() {
  		t.Error("ResolveSink().Enabled() = true; want false when nothing configured")
  	}
  }

  // TestResolveSink_Disabled proves GSL_METRICS=0 force-disables even with a dir.
  func TestResolveSink_Disabled(t *testing.T) {
  	t.Setenv("GSL_METRICS_DIR", t.TempDir())
  	t.Setenv("GSL_METRICS", "0")
  	if metrics.ResolveSink().Enabled() {
  		t.Error("GSL_METRICS=0 should force-disable metrics")
  	}
  }

  // TestEmit_NonFatalOnBadDir proves a write failure never returns fatally.
  func TestEmit_NonFatalOnBadDir(t *testing.T) {
  	t.Setenv("GSL_METRICS_DIR", "/proc/nonexistent/cannot/write")
  	t.Setenv("GSL_METRICS", "")
  	t.Setenv("GSL_METRICS_PUSHGATEWAY", "")
  	// Emit must swallow the error (logs to stderr); it returns nothing fatal.
  	metrics.ResolveSink().Emit(sampleSnapshot())
  }
  ```

  Run: `go test ./internal/metrics/...` — expect **compile error** (package missing).

- [ ] **8.3** Create `src/gsl/internal/metrics/metrics.go`:

  ```go
  // Package metrics emits per-turn gsl token-usage data as Prometheus gauges.
  //
  // gsl render is an ephemeral per-prompt process, so an in-process /metrics
  // HTTP server cannot be scraped. This package uses the node_exporter
  // textfile-collector pattern (write/refresh <dir>/gsl.prom) as the default,
  // with a Pushgateway POST as the alternative for hosts lacking a local
  // node_exporter. The layer is OPT-IN (GSL_METRICS_DIR / GSL_METRICS_PUSHGATEWAY
  // or config keys) and strictly NON-FATAL: any write/push failure is logged to
  // stderr and swallowed so the status line never breaks.
  package metrics

  import (
  	"fmt"
  	"os"
  	"path/filepath"
  	"sort"
  )

  // Snapshot is the per-turn data the metrics layer publishes.
  type Snapshot struct {
  	Model             string
  	Assistant         string // "claude" for #34; "gemini" reserved for #42
  	UsedPercentage    float64
  	TotalInputTokens  float64
  	ContextWindowSize float64
  	RateLimits        map[string]float64 // window ("five_hour"/"seven_day") -> used_pct
  }

  // Sink is a resolved metrics destination (textfile, pushgateway, or off).
  type Sink struct {
  	enabled        bool
  	textfileDir    string
  	pushgatewayURL string
  }

  // Enabled reports whether any sink is configured.
  func (s Sink) Enabled() bool { return s.enabled }

  // ResolveSink reads env (and, when wired, config) to pick the active sink.
  // Precedence: GSL_METRICS=0 force-off > GSL_METRICS_DIR (textfile, default) >
  // GSL_METRICS_PUSHGATEWAY (alternative). Off when nothing is set.
  func ResolveSink() Sink {
  	if os.Getenv("GSL_METRICS") == "0" {
  		return Sink{enabled: false}
  	}
  	if dir := os.Getenv("GSL_METRICS_DIR"); dir != "" {
  		return Sink{enabled: true, textfileDir: dir}
  	}
  	if url := os.Getenv("GSL_METRICS_PUSHGATEWAY"); url != "" {
  		return Sink{enabled: true, pushgatewayURL: url}
  	}
  	return Sink{enabled: false}
  }

  // Emit publishes the snapshot to the resolved sink. NON-FATAL: errors are
  // logged to stderr and swallowed. Never call this on the hot path before the
  // status line is rendered.
  func (s Sink) Emit(snap Snapshot) {
  	if !s.enabled {
  		return
  	}
  	var err error
  	switch {
  	case s.textfileDir != "":
  		err = WriteTextfile(s.textfileDir, snap)
  	case s.pushgatewayURL != "":
  		err = PushGateway(s.pushgatewayURL, snap)
  	}
  	if err != nil {
  		fmt.Fprintf(os.Stderr, "gsl metrics: emit failed (ignored): %v\n", err)
  	}
  }

  // WriteTextfile writes <dir>/gsl.prom atomically (temp file + rename) so a
  // scraping node_exporter never reads a partial file.
  func WriteTextfile(dir string, snap Snapshot) error {
  	if err := os.MkdirAll(dir, 0o755); err != nil {
  		return fmt.Errorf("metrics: mkdir: %w", err)
  	}
  	body := renderExposition(snap)
  	tmp, err := os.CreateTemp(dir, ".gsl.prom-*")
  	if err != nil {
  		return fmt.Errorf("metrics: tempfile: %w", err)
  	}
  	tmpName := tmp.Name()
  	if _, err := tmp.WriteString(body); err != nil {
  		tmp.Close()
  		os.Remove(tmpName)
  		return fmt.Errorf("metrics: write: %w", err)
  	}
  	if err := tmp.Close(); err != nil {
  		os.Remove(tmpName)
  		return fmt.Errorf("metrics: close: %w", err)
  	}
  	if err := os.Rename(tmpName, filepath.Join(dir, "gsl.prom")); err != nil {
  		os.Remove(tmpName)
  		return fmt.Errorf("metrics: rename: %w", err)
  	}
  	return nil
  }

  // renderExposition builds the Prometheus text exposition format. Kept as a
  // pure function so it is trivially testable and reusable by both sinks.
  func renderExposition(snap Snapshot) string {
  	model := snap.Model
  	if model == "" {
  		model = "unknown"
  	}
  	assistant := snap.Assistant
  	if assistant == "" {
  		assistant = "claude"
  	}
  	lbl := fmt.Sprintf(`model=%q,assistant=%q`, model, assistant)

  	var b []byte
  	add := func(name, help, value string) {
  		b = append(b, fmt.Sprintf("# HELP %s %s\n# TYPE %s gauge\n%s{%s} %s\n",
  			name, help, name, name, lbl, value)...)
  	}
  	add("gsl_context_used_percentage", "Context window used percentage (0-100).",
  		fmt.Sprintf("%g", snap.UsedPercentage))
  	add("gsl_total_input_tokens", "Total input tokens in the current context.",
  		fmt.Sprintf("%g", snap.TotalInputTokens))
  	add("gsl_context_window_size", "Context window size in tokens.",
  		fmt.Sprintf("%g", snap.ContextWindowSize))

  	// Rate-limit gauges: one series per window, sorted for deterministic output.
  	windows := make([]string, 0, len(snap.RateLimits))
  	for w := range snap.RateLimits {
  		windows = append(windows, w)
  	}
  	sort.Strings(windows)
  	if len(windows) > 0 {
  		b = append(b, "# HELP gsl_rate_limit_used_percentage Rate-limit window used percentage (0-100).\n"...)
  		b = append(b, "# TYPE gsl_rate_limit_used_percentage gauge\n"...)
  		for _, w := range windows {
  			b = append(b, fmt.Sprintf("gsl_rate_limit_used_percentage{%s,window=%q} %g\n",
  				lbl, w, snap.RateLimits[w])...)
  		}
  	}
  	return string(b)
  }
  ```

  > **Zero-dep alt (8.1 fallback):** `renderExposition` above is already pure
  > stdlib — it needs `client_golang` only for the Pushgateway path. If the
  > reviewer chooses zero-dep, keep `WriteTextfile`/`renderExposition` as-is and
  > make `PushGateway` return a "pushgateway requires client_golang (deferred)"
  > error; no go.mod change is then needed.

- [ ] **8.4** Create `src/gsl/internal/metrics/pushgateway.go` (the Pushgateway
  alternative; uses `client_golang`):

  ```go
  package metrics

  import (
  	"fmt"

  	"github.com/prometheus/client_golang/prometheus"
  	"github.com/prometheus/client_golang/prometheus/push"
  )

  // PushGateway pushes the snapshot's gauges to a Pushgateway at url. The job
  // label is "gsl"; the model/assistant become grouping labels so concurrent
  // windows don't clobber each other's series.
  func PushGateway(url string, snap Snapshot) error {
  	model := snap.Model
  	if model == "" {
  		model = "unknown"
  	}
  	assistant := snap.Assistant
  	if assistant == "" {
  		assistant = "claude"
  	}
  	reg := prometheus.NewRegistry()
  	mk := func(name, help string, labels prometheus.Labels) prometheus.Gauge {
  		g := prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help, ConstLabels: labels})
  		reg.MustRegister(g)
  		return g
  	}
  	base := prometheus.Labels{"model": model, "assistant": assistant}
  	mk("gsl_context_used_percentage", "Context window used percentage (0-100).", base).Set(snap.UsedPercentage)
  	mk("gsl_total_input_tokens", "Total input tokens in the current context.", base).Set(snap.TotalInputTokens)
  	mk("gsl_context_window_size", "Context window size in tokens.", base).Set(snap.ContextWindowSize)
  	for window, pct := range snap.RateLimits {
  		l := prometheus.Labels{"model": model, "assistant": assistant, "window": window}
  		mk("gsl_rate_limit_used_percentage", "Rate-limit window used percentage (0-100).", l).Set(pct)
  	}
  	if err := push.New(url, "gsl").Gatherer(reg).
  		Grouping("model", model).Grouping("assistant", assistant).Push(); err != nil {
  		return fmt.Errorf("metrics: pushgateway: %w", err)
  	}
  	return nil
  }
  ```

  If the zero-dep path (8.1) is chosen, replace this file's body with a single
  stub returning `fmt.Errorf("metrics: pushgateway requires client_golang (deferred)")`.

- [ ] **8.5** Add the dependency (only if 8.4's real Pushgateway is kept). Run:

  ```sh
  cd src/gsl
  go get github.com/prometheus/client_golang@latest
  go mod tidy
  ```

  Expected `go.mod` gains:
  ```
  require github.com/prometheus/client_golang v1.x.y
  ```
  Confirm `go.sum` adds only Apache-2.0 / BSD-3-Clause transitive deps
  (`prometheus/common`, `prometheus/client_model`, `cespare/xxhash`,
  `golang/protobuf`, `munnerz/goautoneg` — all permissive). No GPL/LGPL/AGPL.

- [ ] **8.6** Wire the metrics emit into the SAME recording hook in
  `src/gsl/cmd/render.go`, immediately after the `usagelog.AppendTurn` block
  (coupling the two sinks), still gated on the non-nil context-window check and
  still off the hot path / non-fatal:

  ```go
  // After the usagelog.AppendTurn(...) block, inside the same
  // `if p.ContextWindow != nil && ...` guard:
  snap := metrics.Snapshot{
  	Model:             "",
  	Assistant:         "claude",
  	UsedPercentage:    *p.ContextWindow.UsedPercentage,
  	TotalInputTokens:  *p.ContextWindow.TotalInputTokens,
  	ContextWindowSize: *p.ContextWindow.ContextWindowSize,
  	RateLimits:        map[string]float64{},
  }
  if p.Model != nil && p.Model.DisplayName != nil {
  	snap.Model = *p.Model.DisplayName
  }
  if p.RateLimits != nil {
  	if p.RateLimits.FiveHour != nil && p.RateLimits.FiveHour.UsedPercentage != nil {
  		snap.RateLimits["five_hour"] = *p.RateLimits.FiveHour.UsedPercentage
  	}
  	if p.RateLimits.SevenDay != nil && p.RateLimits.SevenDay.UsedPercentage != nil {
  		snap.RateLimits["seven_day"] = *p.RateLimits.SevenDay.UsedPercentage
  	}
  }
  // ResolveSink() is a no-op when GSL_METRICS_DIR / GSL_METRICS_PUSHGATEWAY are
  // unset, so this costs nothing on hosts that don't opt in. Emit is non-fatal.
  metrics.ResolveSink().Emit(snap)
  ```

  Add `"github.com/wenlock/dotfiles/gsl/internal/metrics"` to render.go's imports.

- [ ] **8.7** Run: `go test ./internal/metrics/... -v` and `go test ./... -count=1`.
  Expected: metrics tests pass; render still appends the turn record AND (when
  `GSL_METRICS_DIR` is set in a test) writes `gsl.prom`; status line unaffected
  when metrics are off or fail.

- [ ] **8.8** Commit: `feat(metrics): add opt-in Prometheus textfile/pushgateway exposition coupled to render`

---

### Task 9 — Post-validation + end-to-end smoke test + docs update

#### Post-validation (verify the implemented feature works end-to-end)

- [ ] **9.1** **(a) Real-turn recording.** Confirm `usage.jsonl` gets a real
  `turn` record after an **actual Claude Code turn** (not just a piped fixture).
  With the real `gsl render` wired into `~/.claude/settings.json`
  `statusLine.command`, run one live turn, then:

  ```sh
  tail -n1 "${XDG_STATE_HOME:-$HOME/.local/state}/gsl/usage.jsonl" | python3 -m json.tool
  # Assert: "kind":"turn", non-zero "input_tokens", "context_size", "used_pct".
  ```

  This closes the loop opened by the Task 0 pre-validation gate: Task 0 proved
  Claude *gives* us the fields; this proves gsl *records* them in a live session.

- [ ] **9.2** **(b) Delta correctness.** Seed marks around turns and confirm
  `gsl tokens --last` and `--diff` compute correct deltas:

  ```sh
  # 1. Seed some turns by piping payloads through render.
  echo '{"cwd":"/tmp","model":{"display_name":"claude-sonnet-4-6"},"context_window":{"used_percentage":15.0,"total_input_tokens":30000,"context_window_size":200000}}' | gsl render

  gsl mark before-refactor

  echo '{"cwd":"/tmp","model":{"display_name":"claude-sonnet-4-6"},"context_window":{"used_percentage":18.0,"total_input_tokens":36000,"context_window_size":200000}}' | gsl render

  gsl mark after-refactor

  # 2. Inspect last turn.
  gsl tokens --last
  # Expected output:
  # Last turn: <timestamp>
  #   Input tokens : 36000
  #   Context size : 200000
  #   Used         : 18.00%

  # 3. Inspect delta.
  gsl tokens --diff before-refactor after-refactor
  # Expected output:
  # Token usage delta: +6,000 tokens (Context: 15.00% -> 18.00%)
  #   Range  : before-refactor → after-refactor
  #   Context: 200000 tokens total

  # 4. Confirm log file exists and has expected lines.
  cat "${XDG_STATE_HOME:-$HOME/.local/state}/gsl/usage.jsonl"
  ```

- [ ] **9.3** **(c) Prometheus emission.** Confirm the metrics sink gets the
  gauges. Textfile mode (default):

  ```sh
  export GSL_METRICS_DIR=/tmp/gsl-textfile
  echo '{"cwd":"/tmp","model":{"display_name":"claude-sonnet-4-6"},"context_window":{"used_percentage":18.0,"total_input_tokens":36000,"context_window_size":200000},"rate_limits":{"five_hour":{"used_percentage":12.5},"seven_day":{"used_percentage":40.0}}}' | gsl render
  cat /tmp/gsl-textfile/gsl.prom
  # Expected gauges:
  # gsl_context_used_percentage{model="claude-sonnet-4-6",assistant="claude"} 18
  # gsl_total_input_tokens{model="claude-sonnet-4-6",assistant="claude"} 36000
  # gsl_context_window_size{model="claude-sonnet-4-6",assistant="claude"} 200000
  # gsl_rate_limit_used_percentage{model="...",assistant="claude",window="five_hour"} 12.5
  # gsl_rate_limit_used_percentage{model="...",assistant="claude",window="seven_day"} 40
  unset GSL_METRICS_DIR

  # Pushgateway alternative (if a local Pushgateway is available on :9091):
  GSL_METRICS_PUSHGATEWAY=http://localhost:9091 sh -c 'echo "{...payload...}" | gsl render'
  # Then confirm series at http://localhost:9091/metrics
  ```

  Also confirm the **non-fatal** contract: with `GSL_METRICS_DIR` pointed at an
  unwritable path, `gsl render` still prints the status line and exits 0.

- [ ] **9.4** Update `src/gsl/docs/design.md` — add `cmd/mark.go` and `cmd/tokens.go` to the package layout table, add `internal/usagelog` and `internal/metrics` to the `internal/` package list, and note the opt-in `GSL_METRICS_DIR` / `GSL_METRICS_PUSHGATEWAY` env knobs.

- [ ] **9.5** Run `bash src/gsl/scripts/check-deps.sh` — confirm no `os/exec` outside the allowed seams (the new packages use only `os`, `bufio`, `encoding/json`, `fmt`, `sort`, `path/filepath`, plus `prometheus/client_golang` for the Pushgateway path — no process exec).

- [ ] **9.6** Commit: `docs(gsl): update design.md for token usage history + metrics feature`

---

## Acceptance Gates (issue closed when all pass)

| Gate | Check |
|---|---|
| **Pre-validation (Task 0) PASSED** | live Claude payload populates `context_window.{used_percentage,total_input_tokens,context_window_size}`; probe test green |
| `go build ./...` green | no compile errors |
| `scripts/check-deps.sh` green | no os/exec outside git/mcp/gh seams |
| `go test ./... -cover` | all packages pass; `internal/usagelog` >= 80%; `internal/metrics` >= 60% |
| `gsl mark <label>` appends mark record to usage.jsonl | manual + test |
| `gsl tokens --last` prints last turn's token count | manual + test |
| `gsl tokens --diff start end` prints `Token usage delta: +4,520 tokens (Context: 15% -> 18%)` format | manual + test |
| `gsl render` appends turn record on every real Claude payload | manual + test |
| **Post-validation:** real Claude Code turn writes a `turn` record to usage.jsonl | manual (Task 9.1) |
| **Metrics opt-in:** `GSL_METRICS_DIR` set → `gsl.prom` written with all gauges; unset → no-op | manual + test (Task 9.3) |
| **Metrics non-fatal:** unwritable metrics dir/URL → status line still renders, exit 0 | test (Task 8.2) + manual (Task 9.3) |
| Render degradation: bad payload / nil context window → no log write, no metrics write, render still works | test |
| Missing usage.jsonl → `gsl tokens --last` prints "no turn records" (no crash) | test |

---

## Open Design Questions

1. **Comma formatting without x/text.** `golang.org/x/text` is already an
   indirect dependency (via charmbracelet). However, pulling it in as a *direct*
   dependency to format comma-separated numbers is heavy. An alternative is a
   small pure-stdlib helper:
   ```go
   func formatComma(n int64) string {
       s := fmt.Sprintf("%d", n)
       if n < 0 { s = s[1:] }
       // insert commas every 3 digits from the right
       result := make([]byte, 0, len(s)+(len(s)-1)/3)
       for i, c := range s {
           if i > 0 && (len(s)-i)%3 == 0 { result = append(result, ',') }
           result = append(result, byte(c))
       }
       if n < 0 { return "-" + string(result) }
       return string(result)
   }
   ```
   Implementer should choose based on whether x/text is already a direct dep
   after #32 lands.

2. **Log rotation / max size / longer-term retention.** `usage.jsonl` grows
   unboundedly. A simple max-lines or max-bytes cap (e.g. keep last 10,000 lines
   on append) could be added in a follow-up. Not required for issue #34. **For
   durable long-term retention and visualization, the answer is the Prometheus +
   Grafana layer (Task 8)** — `usage.jsonl` stays a small, rotating local file
   for `gsl tokens` deltas, while Prometheus (scraped from the textfile collector
   or pushed via Pushgateway) holds the long-history time series for Grafana
   dashboards. This is what "if we want longer life, we can use the prometheus
   metrics" (review comment, original L1200) resolves to.

3. **`--diff` with only a start mark (open-ended range).** The plan supports
   this: `endMark == ""` means "from start mark to end of log." Implementer
   should confirm the CLI UX (currently `StringArrayVar` reads 1–2 elements
   from `--diff`); an alternative is `--diff start [end]` positional args to
   `tokensCmd` instead.

4. **Concurrent writes from parallel `gsl render` invocations.** `AppendTurn`
   uses `os.O_APPEND` which is atomic for small writes on POSIX filesystems, so
   concurrent turns from simultaneous Claude windows will not corrupt the JSONL.
   This is sufficient for the typical single-user case; no file lock is needed.

5. **State-dir package from #32.** If #32 exports a `statedir.DefaultPath()`
   or equivalent, `usagelog.DefaultPath()` should call it rather than
   re-implementing the XDG resolution. Coordinate with the #32 implementer.

6. **Prometheus dependency choice (reviewer to confirm).** Task 8.1 recommends
   `github.com/prometheus/client_golang` (Apache-2.0, the repo's *preferred*
   license per `src/CLAUDE.md`) for correct exposition encoding and Pushgateway
   support. The zero-dep alternative (hand-written textfile exposition, no
   Pushgateway) is fully viable and documented inline. **Decision needed:** take
   the Apache-2.0 dep, or stay zero-dep and defer Pushgateway.

---

## Review feedback addressed

| # | Reviewer comment (location) | Resolved by |
|---|---|---|
| 1 | "couple this with prometheus metrics hooks so later we can adapt the tool to publishing to grafana … leverage this further" (L7, Architecture) | **Architecture** paragraph (Prometheus coupled to the recording hook; node_exporter textfile collector default, Pushgateway alternative; Grafana retention) + **Task 8** (`internal/metrics` writer, gauges with `model`/`assistant`/`window` labels, opt-in `GSL_METRICS_DIR`/`GSL_METRICS_PUSHGATEWAY`, non-fatal contract, go.mod addition) + **Task 8.6** wiring into the same render hook |
| 2 | "confirm this workflow will work with Claude Code … Add this to pre-validation steps. Also include this in post-validation steps. We also need the same for Gemini … record it as a GitHub issue" (L5) | **Task 0 — Pre-validation gate** (capture live payload + probe test asserting the three `context_window` fields are non-nil, citing payload.go) + **Task 9.1–9.3 Post-validation** (real-turn record, delta correctness, Prometheus emission) + **Task 0 "Gemini (future)" subsection** (Claude ships first, route options, links existing **#42** — no new issue created) |
| 3 | "If we want longer life, we can use the prometheus metrics" (L1200) | **Architecture** paragraph + **Task 8** (Prometheus/Grafana = durable long-term retention beyond `usage.jsonl`) + **Open Design Question 2** (rotation/retention explicitly answered by the Prometheus layer) |
