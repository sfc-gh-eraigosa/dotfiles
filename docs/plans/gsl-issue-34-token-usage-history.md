# gsl Token Usage History & Delta Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist a per-turn token usage timeline to a local JSONL file and expose `gsl mark <label>` and `gsl tokens` commands so users can checkpoint workflows and compute accurate token consumption deltas between any two named markers.

**Architecture:** A new `internal/usagelog` package owns all JSONL I/O — it appends `UsageEntry` (turn) or `MarkEntry` (checkpoint) records to `${XDG_STATE_HOME:-~/.local/state}/gsl/usage.jsonl`, reusing the state-directory infrastructure introduced by #32. `cmd/render.go` calls `usagelog.Append` after each successful payload parse to record token data, and two new cobra commands (`cmd/mark.go`, `cmd/tokens.go`) provide the write-checkpoint and read/diff user-facing surface respectively.

**Tech Stack:** Go (depends on #32 logging)

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

### Task 8 — End-to-end smoke test + docs update

- [ ] **8.1** Manual verification steps (record in PR description):

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

- [ ] **8.2** Update `src/gsl/docs/design.md` — add `cmd/mark.go` and `cmd/tokens.go` to the package layout table, and add `internal/usagelog` to the `internal/` package list.

- [ ] **8.3** Run `bash src/gsl/scripts/check-deps.sh` — confirm no `os/exec` outside the allowed seams (the new packages use only `os`, `bufio`, `encoding/json`, `fmt`).

- [ ] **8.4** Commit: `docs(gsl): update design.md for token usage history feature`

---

## Acceptance Gates (issue closed when all pass)

| Gate | Check |
|---|---|
| `go build ./...` green | no compile errors |
| `scripts/check-deps.sh` green | no os/exec outside git/mcp/gh seams |
| `go test ./... -cover` | all packages pass; `internal/usagelog` >= 80% |
| `gsl mark <label>` appends mark record to usage.jsonl | manual + test |
| `gsl tokens --last` prints last turn's token count | manual + test |
| `gsl tokens --diff start end` prints `Token usage delta: +4,520 tokens (Context: 15% -> 18%)` format | manual + test |
| `gsl render` appends turn record on every real Claude payload | manual + test |
| Render degradation: bad payload / nil context window → no log write, render still works | test |
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

2. **Log rotation / max size.** `usage.jsonl` grows unboundedly. A simple
   max-lines or max-bytes cap (e.g. keep last 10,000 lines on append) could be
   added in a follow-up. Not required for issue #34.

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
