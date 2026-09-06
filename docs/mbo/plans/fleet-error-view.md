# fleet TUI panes + stderr error view — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:executing-plans` (recommended
> here — the tasks are strictly stacked) or `superpowers:subagent-driven-development`.
> Steps use checkbox (`- [ ]`) syntax. The execution trio lives in
> [`fleet-error-view/`](./fleet-error-view/): `IMPLEMENTATION.md` (procedure),
> `TRACKING.md` (evidence ledger), `TODO.md` (resumable cursor).

- **Slug:** fleet-error-view
- **Date:** 2026-09-06
- **Status:** Draft
- **Relates to:** spec [`../specs/fleet-error-view.md`](../specs/fleet-error-view.md) · design [`../designs/fleet-error-view.md`](../designs/fleet-error-view.md) · issue [#308](https://github.com/sfc-gh-eraigosa/dotfiles/issues/308) · PR #TBD

**Goal:** Make `fleet tui` a three-pane dashboard (host / log / error) whose panes toggle with
`h` / `l` / `e` and share the viewport correctly, and carry remote **stderr** end to end so a
host that wrote to stderr is flagged as a warning even when its update exited 0.

**Architecture:** Three stacked layers, each independently revertable. (1) `internal/runner`
gains an *optional* `SplitStreamer` capability that keeps stdout and stderr apart; the existing
`RunStreamCtx` is reimplemented on top of it so there is one streaming implementation.
(2) `internal/updexec` gains an *optional* `Console.ErrLine` (nil ⇒ stderr goes to `Line`, i.e.
today's behavior) and tees both streams into the per-host capture, marking stderr `!!`.
(3) `cmd/` gains a pure `layout()` function that owns every pane height, one tagged log buffer
with two projections, the `h`/`e` keys, focus cycling over visible panes, and the warning badge.

**Tech Stack:** Go 1.x (module `sdk/fleet`), bubbletea + lipgloss, table-driven `go test`.

**Spec:** [`docs/mbo/specs/fleet-error-view.md`](../specs/fleet-error-view.md) —
read it alongside this plan; the plan argues from it.

## Global Constraints

- **Purity discipline (existing package rule):** `View()` and every model helper are pure —
  never `time.Now()` in a render path (`nowFn` is the injected clock), never I/O in `Update()`;
  only `tea.Cmd`s touch the network. Do not break this.
- **`keyHelp` is the single source of truth for keys.** A key that is implemented but not
  declared there is a defect (it is exactly how the log pane shipped undiscoverable).
- **No signature change to `runner.Runner`.** Six test doubles across four packages implement
  `RunStreamCtx`; the new capability is a separate, type-asserted interface (precedent:
  `interactiveCtxRunner` in `internal/updexec/exec.go`).
- **`fleet update`'s terminal output must stay byte-identical.** Only the on-disk capture
  changes (stderr lines gain a `!!` prefix).
- **Coverage bars** (baseline measured on this branch 2026-09-06: module total 82.3 %,
  `cmd` 63.5 %, `runner` 59.8 %, `updexec` 92.5 %): module ≥ 82 %, `cmd` ≥ 65 %,
  `runner` ≥ 65 %, `updexec` ≥ 90 %.
- **Gates every task must pass:** `go test ./...` in `sdk/fleet`, `make lint-go`,
  and `go vet ./...`.
- **Markdown:** `make lint-markdown` needs `markdownlint-cli2` on `PATH`
  (`npx --yes markdownlint-cli2 "<file>"` works without installing it). These artifacts are
  clean except `MD010/no-hard-tabs` inside the Go snippets — tabs are what `gofmt` produces, and
  every merged plan in `docs/mbo/plans/` carries the same finding (`gsl-links.md`: 500). Do not
  de-tab Go code to satisfy it. Fix any **other** rule you introduce.
- **Commit discipline:** stage by explicit path (never `git add -A`); one task, one commit;
  new files must be checked with `git status --short -- <path>` first (the repo's `.gitignore`
  is an allowlist — see [`docs/gitignore-allowlist.md`](../../gitignore-allowlist.md)).

---

## 1. Summary & verdict

14 tasks: 2 in `internal/runner`, 3 in `internal/updexec`, 8 in `cmd/`, 1 human-gated live
capture. Build order is strictly stacked (runner → updexec → model → view → docs) because each
layer's tests consume the layer below. **Verdict: build sequentially in ONE PR**, not as a
parallel fan-out — §6.1 explains why the leaves are false splits.

Two findings from grounding the plan in the code, both already folded into the design:

1. `internal/runner/runner.go:237` merges the two streams into one `io.Pipe` *deliberately*
   ("so the log reads in the order the remote produced it"). Splitting them is therefore a
   conscious trade, not an oversight — accepted, documented, and mitigated by keeping stderr
   in the log pane and timestamping every line (design §3.1).
2. **The frame already overflows the terminal** — up to +12 rows at 60×16 (design §1.3), from
   *three* independent causes: the measured chrome (below), an under-count of panel borders,
   and — the one that breaks all height arithmetic — host rows and the column header being
   truncated to `panelWidth()` when the panel's padding leaves only `panelWidth()-2` usable
   columns, so at 80 columns an 8-host panel renders **20 lines**. The
   height invariant test (Task 8) starts RED against today's code. `layout()` therefore takes
   a **measured** `chromeRows`: *both* ends of the chrome vary — the banner is 6 rows at 60
   columns and 5 at ≥80 (the hint strip wraps), and the "status line" is a framed panel of
   8–12 rows in `modeConfirm`/`modeAnswers`. At 60×16 in `modeAnswers` the chrome alone is 19
   rows, so the invariant is scoped to "the panes never add to an over-budget frame" (spec F8b)
   rather than an unachievable "always ≤ `vp.height`".

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/fleet/internal/runner/runner.go` | `Line{Text,Stderr}`, `SplitStreamer`, `Exec.RunSplitStreamCtx`; `Exec.RunStreamCtx` reimplemented on top of it; `Fake.ErrOut` + `Fake.RunSplitStreamCtx` | F9, F9c |
| `sdk/fleet/internal/runner/split_test.go` *(new)* | Real-process `stubSSH` tests: separation, no deadlock under a burst, deadline still honoured, merged-equals-split | F9a, F9c |
| `sdk/fleet/internal/updexec/exec.go` | `Console.ErrLine`; `Batch` prefers `SplitStreamer`; `teeable.withLines`; `RunHost` tees both streams | F10, F11 |
| `sdk/fleet/internal/updexec/stderrnoise.go` *(new)* | `Benign(line string) bool` — the known-benign ssh/git/sudo denylist | F13 |
| `sdk/fleet/internal/updexec/stderrnoise_test.go` *(new)* | The denylist table | F13a |
| `sdk/fleet/internal/updexec/split_test.go` *(new)* | `ErrLine` routing, nil-fallback, capability fallback, capture marking | F9b, F10a, F11a |
| `sdk/fleet/cmd/tui_layout.go` *(new)* | `panes`, `heights`, `layout()` — the ONLY pane-height arithmetic | F4–F7 |
| `sdk/fleet/cmd/tui_layout_test.go` *(new)* | Table-driven layout cases + the height/width invariant matrix | F4a–F8b |
| `sdk/fleet/cmd/tui_model.go` | `hostOpen`/`errOpen`/`focus`; tagged `logEntry`; `appendLogLine`; `warns`; `listHeight`/`logHeight`/`errHeight` delegate to `layout()`; `logLineMsg.stderr` | F1, F2, F12, F16, F17 |
| `sdk/fleet/cmd/tui_keys.go` | `keyHelp` gains `h`/`e`; `routeNormal` toggles + refuses the last pane; focus cycling; per-pane key scoping | F1, F2, F3, F17, F18 |
| `sdk/fleet/cmd/tui_view.go` | `streamPane()` shared renderer, `errView()`, stderr gutter in the log, `ok ⚠N` badge, status summary, measured `chromeRows()`, `panelInnerWidth()` (every truncation moves onto it) | F8, F8c, F12, F14, F15 |
| `sdk/fleet/cmd/tui_cmds.go` | `outLine{text,stderr}` through `lineQueue`/`stream`/`readLine`; `beginStream` wires `Line` + `ErrLine` | F9, F12 |
| `sdk/fleet/cmd/tui_panes_test.go` *(new)* | Toggles, defaults, refuse-last, focus cycling, per-pane search | F1a–F3a, F17a, F17b, F20a |
| `sdk/fleet/cmd/tui_stderr_test.go` *(new)* | End-to-end model test: `Fake.ErrOut` → error pane + log pane + badge | F12a, F14a, F16a |
| `sdk/fleet/cmd/tui_demo_test.go` | New golden frames per pane combination; the height assertion | F8a |
| `sdk/fleet/cmd/tui_panes_test.go` | `TestPaneKeysAreDeclaredInKeyHelp` (the existing guards live in `tui_config_test.go` / `tui_sticky_test.go` and stay untouched) | F18a |
| `sdk/fleet/AGENTS.md` | TUI row gains `h`/`e`; a new invariant line for the split streams | F18 |
| `sdk/fleet/README.md` | Key table + a three-pane screenshot/description | F18 |
| `docs/mbo/designs/fleet-connect.md` | Cross-objective note: `ReservedKeys` must gain `h`/`e`/`l` | design §5 |
| `docs/mbo/designs/sdk-tui.md` | Cross-objective note: fleet rebinds `h` (no lateral axis) | design §5 |
| `docs/mbo/index.md` | The `fleet-error-view` row + state | pipeline step 6 |
| `docs/mbo/plans/fleet-error-view/{IMPLEMENTATION,TRACKING,TODO}.md` | Execution trio | — |
| `docs/mbo/plans/fleet-error-view/evidence/**` | Captured gate output (design §7) | — |

## 3. Interface contracts

**Frozen for the whole build — later tasks compile against these exact names.**

```go
// ---- internal/runner ------------------------------------------------------
// Line is one line of remote output plus which stream produced it.
type Line struct {
    Text   string
    Stderr bool
}

// SplitStreamer is an OPTIONAL capability of a Runner: streaming with the two
// streams kept apart. runner.Runner itself is UNCHANGED.
type SplitStreamer interface {
    RunSplitStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan Line, <-chan error)
}

// ---- internal/updexec -----------------------------------------------------
type Console struct {
    R        runner.Runner
    Line     func(host, line string) // stdout (and stderr when ErrLine is nil)
    ErrLine  func(host, line string) // NEW, optional
    Stdin    func(st updplan.Step) string
    Preamble func(st updplan.Step) string
}

// Benign reports whether a stderr line is known-benign ssh/git/sudo chatter
// that must NOT raise a host's warning count. It never hides the line.
func Benign(line string) bool

// teeable's method becomes:
type teeable interface {
    withLines(out, err func(host, line string)) StepIO
}

// ---- cmd --------------------------------------------------------------
type pane int
const (paneHost pane = iota; paneLog; paneErr)

type panes struct{ host, log, err bool }
type heights struct{ host, log, err int }

// layout is the ONLY place pane heights are computed. chromeRows is MEASURED
// by the caller (the banner wraps; it is not a constant). logActive/errActive
// mean "open AND has something to show".
func layout(vpHeight, chromeRows int, p panes, logActive, errActive bool) heights

type outLine struct {
    text   string
    stderr bool
}
```

Orchestration, end to end:

```text
ssh ──2 pipes──▶ runner.Line{Text,Stderr}
                    │
                    ▼
        updexec.Console.Batch
          ├─ Stderr=false ─▶ Line(host, text)
          └─ Stderr=true  ─▶ ErrLine(host, text)   (nil ⇒ Line)
                    │                       └────────────┐
                    ▼                                    ▼
   cmd: q.push(outLine{text,stderr})        Executor.RunHost tee ▶ capture
                    │                          ("!! " prefix on stderr)
                    ▼
   readLine ▶ logLineMsg{alias,line,stderr} ▶ m.appendLogLine(...)
                    │
        one tagged []logEntry ring (logCap)
          ├─ log pane   : every entry ("!" gutter on stderr)
          ├─ error pane : entries where stderr
          └─ warns[alias]++ when stderr && !updexec.Benign(line)
```

## 4. TDD build order

> Every task: **RED → run it and see it fail → GREEN → run it and see it pass → gates → commit**.
> `cd sdk/fleet` for every `go` command. Capture each done-when command's output with
> `… 2>&1 | tee ../../docs/mbo/plans/fleet-error-view/evidence/<folder>/<task>.txt`.

---

### Task 1: `runner` — split the streams (`SplitStreamer` on `Exec`)

**Files:**

- Modify: `sdk/fleet/internal/runner/runner.go` (`Exec.RunStreamCtx` at ~:229)
- Test: `sdk/fleet/internal/runner/split_test.go` *(new)*

**Interfaces:**

- Consumes: nothing.
- Produces: `runner.Line{Text string; Stderr bool}`, `runner.SplitStreamer`,
  `(Exec).RunSplitStreamCtx(ctx, host, stdin string, argv ...string) (<-chan Line, <-chan error)`.

- [ ] **Step 1: Write the failing tests**

```go
package runner

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// stubSSH already exists in runner_ctx_test.go — reuse it.

func TestRunSplitStreamCtxSeparatesTheStreams(t *testing.T) {
	stubSSH(t, "echo out1; echo err1 >&2; echo out2")

	e := Exec{}
	lines, done := e.RunSplitStreamCtx(context.Background(), "host", "", "whatever")

	var stdout, stderr []string
	for l := range lines {
		if l.Stderr {
			stderr = append(stderr, l.Text)
		} else {
			stdout = append(stdout, l.Text)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("stub should exit 0, got %v", err)
	}
	if len(stdout) != 2 || stdout[0] != "out1" || stdout[1] != "out2" {
		t.Fatalf("stdout order lost: %q", stdout)
	}
	if len(stderr) != 1 || stderr[0] != "err1" {
		t.Fatalf("stderr not separated: %q", stderr)
	}
}

// The two pipes must both drain even when one side is huge: a single reader
// that stops reading one stream must not wedge the other (the deadlock this
// design's split introduces if either pipe is left unread).
func TestSplitStreamDoesNotDeadlockUnderBackpressure(t *testing.T) {
	stubSSH(t, "i=0; while [ $i -lt 2000 ]; do echo out$i; echo err$i >&2; i=$((i+1)); done")

	e := Exec{}
	lines, done := e.RunSplitStreamCtx(context.Background(), "host", "", "whatever")

	var n, nerr int
	for l := range lines {
		n++
		if l.Stderr {
			nerr++
		}
	}
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("RunSplitStreamCtx deadlocked under a 4000-line burst")
	}
	if n != 4000 || nerr != 2000 {
		t.Fatalf("lost lines: total=%d stderr=%d, want 4000/2000", n, nerr)
	}
}

// RunStreamCtx keeps its signature AND its content: it is the split stream
// with the tag dropped, so the two implementations cannot drift.
func TestRunStreamMatchesSplitStreamMerged(t *testing.T) {
	stubSSH(t, "echo a; echo b >&2; echo c")

	e := Exec{}
	merged, mdone := e.RunStreamCtx(context.Background(), "host", "", "x")
	var got []string
	for l := range merged {
		got = append(got, l)
	}
	<-mdone
	want := map[string]bool{"a": true, "b": true, "c": true}
	if len(got) != 3 {
		t.Fatalf("merged stream lost lines: %q", got)
	}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("unexpected line %q in %q", g, got)
		}
	}
	_ = fmt.Sprint(got)
}

// The deadline guarantee must survive the rewrite.
func TestRunSplitStreamCtxKillsTheChildOnDeadline(t *testing.T) {
	stubSSH(t, "sleep 30")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	e := Exec{}
	lines, done := e.RunSplitStreamCtx(ctx, "host", "", "x")
	for range lines {
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from a killed child")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("deadline not honoured within 2s")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./internal/runner/ -run 'Split|Merged' -v`
Expected: FAIL — `e.RunSplitStreamCtx undefined (type Exec has no field or method RunSplitStreamCtx)`.

- [ ] **Step 3: Implement**

In `runner.go`, add the type + capability interface next to the `Runner` interface, then:

```go
// Line is one line of remote output plus which stream produced it.
type Line struct {
	Text   string
	Stderr bool
}

// SplitStreamer is an OPTIONAL capability of a Runner: RunStreamCtx with the
// two streams kept APART. It is deliberately not part of runner.Runner —
// adding a method there would ripple into every other package's Runner test
// double (the same reasoning as updexec's interactiveCtxRunner).
type SplitStreamer interface {
	RunSplitStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan Line, <-chan error)
}

// RunSplitStreamCtx is RunStreamCtx with stdout and stderr on SEPARATE pipes,
// so a consumer can tell a warning from progress. The cost of the split is
// that ordering BETWEEN the two streams is arrival order, not the remote's
// exact interleaving (ordering WITHIN each stream is exact) — accepted in
// docs/mbo/designs/fleet-error-view.md §3.1, which is also why the TUI keeps
// stderr in the merged log pane and timestamps every line.
//
// Both pipes MUST be drained concurrently: a single goroutine scanning one
// then the other would let the unread pipe's buffer fill and wedge the remote
// command. One scanner goroutine per pipe, a WaitGroup, and one closer.
func (e Exec) RunSplitStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan Line, <-chan error) {
	lines := make(chan Line, 256)
	done := make(chan error, 1)

	c := exec.CommandContext(ctx, "ssh", append(e.baseArgs(host), argv...)...)
	c.Stdin = strings.NewReader(stdin)
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	c.Stdout, c.Stderr = outW, errW
	c.WaitDelay = waitDelay

	if err := c.Start(); err != nil {
		close(lines)
		done <- err
		return lines, done
	}

	go func() {
		err := c.Wait()
		_ = outW.Close()
		_ = errW.Close()
		done <- err
	}()

	var wg sync.WaitGroup
	scan := func(r io.Reader, isErr bool) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // install logs have long lines
		for sc.Scan() {
			lines <- Line{Text: sc.Text(), Stderr: isErr}
		}
	}
	wg.Add(2)
	go scan(outR, false)
	go scan(errR, true)
	go func() {
		wg.Wait()
		close(lines)
	}()

	return lines, done
}

// RunStreamCtx is RunSplitStreamCtx with the tag dropped, so there is exactly
// ONE streaming implementation and the merged path can never drift from the
// split one.
func (e Exec) RunStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan string, <-chan error) {
	split, done := e.RunSplitStreamCtx(ctx, host, stdin, argv...)
	lines := make(chan string, 256)
	go func() {
		defer close(lines)
		for l := range split {
			lines <- l.Text
		}
	}()
	return lines, done
}
```

Add `"sync"` to the imports. Update the comment on `RunStream` (it no longer describes a single
shared pipe).

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./internal/runner/ -v 2>&1 | tail -30`
Expected: PASS, including the pre-existing `TestRunStreamCtxKillsTheChildOnDeadline` and
`TestRunStreamDelegatesToCtx`.

**Done-when:** `go test ./internal/runner/` PASS and `go test ./...` PASS (nothing else moved).
**Evidence:** `evidence/stderr/task01-runner-split.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/runner/runner.go sdk/fleet/internal/runner/split_test.go
git commit -m "feat(fleet/runner): stream stdout and stderr on separate pipes

RunSplitStreamCtx keeps the two streams apart so a consumer can tell a
warning from progress; RunStreamCtx is now that stream with the tag
dropped, so there is one implementation instead of two that can drift.
runner.Runner is unchanged — the split is an optional capability."
```

---

### Task 2: `runner.Fake` — scriptable stderr

**Files:**

- Modify: `sdk/fleet/internal/runner/runner.go` (`Fake` struct ~:275, `Fake.RunStreamCtx` ~:337)
- Test: `sdk/fleet/internal/runner/split_test.go`

**Interfaces:**

- Consumes: `runner.Line`, `runner.SplitStreamer` (Task 1).
- Produces: `Fake.ErrOut map[string]string`; `(Fake).RunSplitStreamCtx` with the same signature
  as `Exec`'s — every downstream test scripts stderr with `Fake{Out: …, ErrOut: …}`.

- [ ] **Step 1: Write the failing test**

```go
func TestFakeReplaysErrOutAsStderr(t *testing.T) {
	f := Fake{
		Out:    map[string]string{"h": "one\ntwo"},
		ErrOut: map[string]string{"h": "boom"},
	}
	lines, done := f.RunSplitStreamCtx(context.Background(), "h", "", "x")

	var out, errs []string
	for l := range lines {
		if l.Stderr {
			errs = append(errs, l.Text)
		} else {
			out = append(out, l.Text)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("no Err scripted, want nil, got %v", err)
	}
	if len(out) != 2 || out[0] != "one" || out[1] != "two" {
		t.Fatalf("stdout replay wrong: %q", out)
	}
	if len(errs) != 1 || errs[0] != "boom" {
		t.Fatalf("stderr replay wrong: %q", errs)
	}
}

// The merged path must still see BOTH streams, or every existing test that
// asserts on Fake output would silently start missing lines.
func TestFakeMergedStreamStillCarriesErrOut(t *testing.T) {
	f := Fake{Out: map[string]string{"h": "one"}, ErrOut: map[string]string{"h": "boom"}}
	lines, _ := f.RunStreamCtx(context.Background(), "h", "", "x")
	var got []string
	for l := range lines {
		got = append(got, l)
	}
	if len(got) != 2 {
		t.Fatalf("merged Fake stream lost a line: %q", got)
	}
}

var _ SplitStreamer = Fake{}
var _ SplitStreamer = Exec{}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./internal/runner/ -run 'Fake' -v`
Expected: FAIL — `unknown field ErrOut in struct literal`.

- [ ] **Step 3: Implement**

```go
// Fake gains:
//	// ErrOut is what the host writes to STDERR, replayed line by line by
//	// RunSplitStreamCtx. Out is stdout; a host may script either or both.
//	ErrOut map[string]string

// RunSplitStreamCtx replays Out[host] as stdout and ErrOut[host] as stderr.
// It mirrors RunStreamCtx exactly — including recording stdin (tests assert
// the sudo secret travelled over stdin and NOT argv) and sending Err[host]
// from the same goroutine AFTER the lines — because any divergence between
// the fake's two paths is a bug that only shows up in the tests that use the
// other one.
func (f Fake) RunSplitStreamCtx(ctx context.Context, host, stdin string, _ ...string) (<-chan Line, <-chan error) {
	lines := make(chan Line, 64)
	done := make(chan error, 1)
	if f.Stdin != nil {
		f.Stdin[host] = stdin
	}
	if f.Block[host] {
		go func() {
			defer close(lines)
			<-ctx.Done()
			done <- ctx.Err()
		}()
		return lines, done
	}
	go func() {
		defer close(lines)
		for _, l := range strings.Split(f.Out[host], "\n") {
			if strings.TrimSpace(l) != "" {
				lines <- Line{Text: l}
			}
		}
		for _, l := range strings.Split(f.ErrOut[host], "\n") {
			if strings.TrimSpace(l) != "" {
				lines <- Line{Text: l, Stderr: true}
			}
		}
		done <- f.Err[host]
	}()
	return lines, done
}

// RunStreamCtx is now that stream with the tag dropped.
func (f Fake) RunStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan string, <-chan error) {
	split, done := f.RunSplitStreamCtx(ctx, host, stdin, argv...)
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		for l := range split {
			lines <- l.Text
		}
	}()
	return lines, done
}
```

**Read `Fake.RunStreamCtx` (`runner.go` ~:337) before writing this** and keep every behaviour it
has: the `f.Stdin[host] = stdin` recording, the `Block[host]` ctx path, the blank-line skip
(`strings.TrimSpace(l) != ""`), and `done <- f.Err[host]` sent from inside the same goroutine
after the lines. The two paths must not drift.

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./... 2>&1 | tail -20`
Expected: PASS everywhere — this is the task that proves the Fake change is backwards
compatible for every existing consumer.

**Done-when:** `go test ./...` PASS; `go tool cover` shows `internal/runner` ≥ 65 %.
**Evidence:** `evidence/stderr/task02-fake-errout.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/runner/runner.go sdk/fleet/internal/runner/split_test.go
git commit -m "feat(fleet/runner): Fake.ErrOut scripts a host's stderr"
```

---

### Task 3: `updexec` — `Console.ErrLine` and the capability fallback

**Files:**

- Modify: `sdk/fleet/internal/updexec/exec.go` (`Console` ~:115, `Console.Batch` ~:153)
- Test: `sdk/fleet/internal/updexec/split_test.go` *(new)*

**Interfaces:**

- Consumes: `runner.SplitStreamer`, `runner.Line`, `Fake.ErrOut`.
- Produces: `Console.ErrLine func(host, line string)`.

- [ ] **Step 1: Write the failing tests**

```go
package updexec

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

func TestErrLineReceivesStderrOnly(t *testing.T) {
	f := runner.Fake{
		Out:    map[string]string{"h": "progress"},
		ErrOut: map[string]string{"h": "WARNING: apt-get update failed"},
	}
	var out, errs []string
	c := Console{
		R:       f,
		Line:    func(_, l string) { out = append(out, l) },
		ErrLine: func(_, l string) { errs = append(errs, l) },
	}
	got, err := c.Batch(context.Background(), "h", updplan.Step{Kind: updplan.KindRun}, "script")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0] != "progress" {
		t.Fatalf("Line got %q", out)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "apt-get") {
		t.Fatalf("ErrLine got %q", errs)
	}
	// The returned output keeps BOTH streams: it is a step's captured text and
	// the source of a row's FAIL explanation.
	if !strings.Contains(got, "progress") || !strings.Contains(got, "apt-get") {
		t.Fatalf("Batch output must keep both streams, got %q", got)
	}
}

func TestNilErrLineRoutesStderrToLine(t *testing.T) {
	f := runner.Fake{Out: map[string]string{"h": "a"}, ErrOut: map[string]string{"h": "b"}}
	var seen []string
	c := Console{R: f, Line: func(_, l string) { seen = append(seen, l) }}
	if _, err := c.Batch(context.Background(), "h", updplan.Step{}, "s"); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("with ErrLine nil, stderr must reach Line: %q", seen)
	}
}

// The package's EXISTING recordingRunner (console_test.go) implements
// runner.Runner and NOT runner.SplitStreamer — it is the real-world shape of
// this fallback (every pre-existing test double in the repo is one), so it is
// what the fallback is proven against rather than a purpose-built stub.
func TestBatchFallsBackWhenNotSplitCapable(t *testing.T) {
	var _ runner.Runner = &recordingRunner{}
	if _, ok := interface{}(&recordingRunner{}).(runner.SplitStreamer); ok {
		t.Fatal("this test needs a runner WITHOUT the split capability")
	}
	r := &recordingRunner{streamOut: "x\ny"}
	var out, errs []string
	c := Console{
		R:       r,
		Line:    func(_, l string) { out = append(out, l) },
		ErrLine: func(_, l string) { errs = append(errs, l) },
	}
	if _, err := c.Batch(context.Background(), "h", updplan.Step{}, "s"); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || len(errs) != 0 {
		t.Fatalf("without the capability every line is stdout: out=%q err=%q", out, errs)
	}
	// recordingRunner's field is streamOut (console_test.go); it also has
	// streamErr for the exit error. Construct it the way that file already
	// does — do not add a second way to build it.
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./internal/updexec/ -run 'ErrLine|FallsBack' -v`
Expected: FAIL — `unknown field ErrLine in struct literal of type Console`.

- [ ] **Step 3: Implement**

```go
// Console gains, right after Line:
//	// ErrLine receives the remote's STDERR when the runner can separate the
//	// streams (runner.SplitStreamer). Nil — the CLI's case — routes stderr to
//	// Line instead, which is byte-for-byte today's behavior.
//	ErrLine  func(host, line string)

func (c Console) emit(host string, l runner.Line) {
	if l.Stderr && c.ErrLine != nil {
		c.ErrLine(host, l.Text)
		return
	}
	if c.Line != nil {
		c.Line(host, l.Text)
	}
}

func (c Console) Batch(ctx context.Context, host string, st updplan.Step, script string) (string, error) {
	var out []string
	var err error

	if ss, ok := c.R.(runner.SplitStreamer); ok {
		lines, done := ss.RunSplitStreamCtx(ctx, host, c.runStdin(st), c.runScript(st, script))
		for l := range lines {
			c.emit(host, l)
			out = append(out, l.Text)
		}
		err = <-done
	} else {
		lines, done := c.R.RunStreamCtx(ctx, host, c.runStdin(st), c.runScript(st, script))
		for l := range lines {
			c.emit(host, runner.Line{Text: l})
			out = append(out, l)
		}
		err = <-done
	}

	if errors.Is(err, exec.ErrWaitDelay) && !isExitError(err) {
		err = nil
	}
	if rawExitCode(err) == 255 {
		err = fmt.Errorf("%w: %v", ErrTransport, err)
	}
	return strings.Join(out, "\n"), err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./internal/updexec/ -v 2>&1 | tail -30`
Expected: PASS, including every pre-existing console/exec test.

**Done-when:** `go test ./internal/updexec/` PASS; `updexec` coverage ≥ 90 %.
**Evidence:** `evidence/stderr/task03-errline.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/updexec/exec.go sdk/fleet/internal/updexec/split_test.go
git commit -m "feat(fleet/updexec): optional Console.ErrLine for remote stderr

Nil ErrLine keeps today's behavior exactly (stderr goes to Line), so the
CLI path is unchanged; a runner without the split capability falls back to
the merged stream."
```

---

### Task 4: `updexec` — tee both streams into the capture, marking stderr

**Files:**

- Modify: `sdk/fleet/internal/updexec/exec.go` (`teeable` ~:249, `Console.withLine` ~:255,
  `Background.withLine` ~:268, `Executor.RunHost` tee block ~:364)
- Test: `sdk/fleet/internal/updexec/split_test.go`

**Interfaces:**

- Consumes: `Console.ErrLine` (Task 3).
- Produces: `teeable.withLines(out, err func(host, line string)) StepIO`; the capture's
  `stderrMark = "!! "` prefix.

- [ ] **Step 1: Write the failing test**

```go
// memOutput is a test updexec.Output that keeps every captured line.
//
// NAMES MATTER HERE: exec_test.go:236,243 already declare `recordingOutput`
// and `recordingWriter` in this same package, so reusing either name fails to
// compile ("redeclared in this block"). Check for a collision before adding
// any package-level test type — this is the same trap as recordingRunner.
type memOutput struct{ lines *[]string }

func (m memOutput) Open(string, string) (LineWriter, string) {
	return memWriter{m.lines}, "mem://capture"
}

type memWriter struct{ lines *[]string }

func (w memWriter) Line(s string) { *w.lines = append(*w.lines, s) }
func (w memWriter) Close(string)  {}

func TestCaptureMarksStderr(t *testing.T) {
	var captured []string
	f := runner.Fake{
		Out:    map[string]string{"h": "installing"},
		ErrOut: map[string]string{"h": "WARNING: apt-get update failed"},
	}
	ex := Executor{
		IO:  Console{R: f},
		Out: memOutput{&captured},
	}
	ex.RunHost("h", updplan.Default())

	var sawOut, sawErr bool
	for _, l := range captured {
		if l == "installing" {
			sawOut = true
		}
		if l == stderrMark+"WARNING: apt-get update failed" {
			sawErr = true
		}
	}
	if !sawOut {
		t.Fatalf("stdout must reach the capture unprefixed: %q", captured)
	}
	if !sawErr {
		t.Fatalf("stderr must reach the capture marked %q: %q", stderrMark, captured)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./internal/updexec/ -run 'Capture' -v`
Expected: FAIL — `undefined: stderrMark`.

- [ ] **Step 3: Implement**

```go
// stderrMark prefixes a stderr line in a host's captured log. Without it a
// post-mortem of a headless run cannot tell a warning from progress — the
// distinction exists on the wire now, and throwing it away at the capture is
// where it would be lost for good.
const stderrMark = "!! "

type teeable interface {
	withLines(out, err func(host, line string)) StepIO
}

func (c Console) withLines(out, err func(host, line string)) StepIO {
	origOut, origErr := c.Line, c.ErrLine
	c.Line = func(host, line string) {
		if origOut != nil {
			origOut(host, line)
		}
		out(host, line)
	}
	c.ErrLine = func(host, line string) {
		// Route to the ORIGINAL ErrLine when the caller had one; otherwise the
		// caller wanted stderr on Line (Task 3's nil rule) and must keep it.
		if origErr != nil {
			origErr(host, line)
		} else if origOut != nil {
			origOut(host, line)
		}
		err(host, line)
	}
	return c
}

func (b Background) withLines(out, err func(host, line string)) StepIO {
	b.Console = b.Console.withLines(out, err).(Console)
	return b
}
```

In `Executor.RunHost`, replace the tee block:

```go
	if t, ok := e.IO.(teeable); ok {
		e.IO = t.withLines(
			func(_, line string) { w.Line(line) },
			func(_, line string) { w.Line(stderrMark + line) },
		)
	}
```

The prefix is applied unconditionally — there is no path that tees twice (`RunHost` has a value
receiver, so its rewired `e.IO` never escapes the call), so an idempotence guard would be
handling a scenario that cannot occur.

**Careful:** `withLines` sets `c.ErrLine` non-nil for every teed lane. That changes what
`Console.emit` does for a caller who passed `ErrLine == nil` — hence the `origErr`/`origOut`
fallback above, which is what `TestNilErrLineRoutesStderrToLine` (Task 3) re-proves after this
task. Re-run that test explicitly.

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./internal/updexec/ -v 2>&1 | tail -30`
Expected: PASS, `TestNilErrLineRoutesStderrToLine` included.

**Done-when:** `go test ./...` PASS; `updexec` ≥ 90 %.
**Evidence:** `evidence/stderr/task04-capture-mark.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/updexec/exec.go sdk/fleet/internal/updexec/split_test.go
git commit -m "feat(fleet/updexec): mark stderr in the per-host capture

The run log now says which lines the remote wrote to stderr, which is what
makes a post-mortem of a headless \`fleet update\` possible."
```

---

### Task 5: `updexec.Benign` — the warning denylist

**Files:**

- Create: `sdk/fleet/internal/updexec/stderrnoise.go`
- Test: `sdk/fleet/internal/updexec/stderrnoise_test.go`

**Interfaces:**

- Consumes: nothing.
- Produces: `updexec.Benign(line string) bool`.

- [ ] **Step 1: Write the failing test**

```go
package updexec

import "testing"

func TestBenignStderrTable(t *testing.T) {
	benign := []string{
		`Warning: Permanently added 'host-pi' (ED25519) to the list of known hosts.`,
		`Pseudo-terminal will not be allocated because stdin is not a terminal.`,
		`remote: Enumerating objects: 41, done.`,
		`Receiving objects:  73% (30/41)`,
		`Resolving deltas: 100% (12/12), done.`,
		`Counting objects: 41, done.`,
		`From https://github.com/example/dotfiles`,
		` * branch            main       -> FETCH_HEAD`,
		`   72392c9..9484943  main       -> origin/main`,
		``,
		`   `,
	}
	for _, l := range benign {
		if !Benign(l) {
			t.Errorf("must be benign: %q", l)
		}
	}

	real := []string{
		`WARNING: apt-get update failed; installs may be incomplete.`,
		`WARNING: grouped install failed; retrying packages individually...`,
		`sudo: a password is required`,
		`fatal: could not read Username for 'https://github.com'`,
		`E: Unable to locate package foo`,
		`ssh: connect to host host-pi port 22: No route to host`,
		// A benign PREFIX must not whitelist whatever follows it: git writes
		// its failures through the same "remote: " channel as its progress.
		`remote: fatal: repository not found`,
		`remote: Permission to example/dotfiles.git denied`,
	}
	for _, l := range real {
		if Benign(l) {
			t.Errorf("must NOT be benign: %q", l)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./internal/updexec/ -run Benign -v`
Expected: FAIL — `undefined: Benign`.

- [ ] **Step 3: Implement**

```go
package updexec

import "strings"

// benignPatterns are the stderr lines every healthy run produces: ssh's
// known-hosts and tty notices, and git's progress reporting (git writes ALL
// progress to stderr). Counting these as warnings would put a ⚠ on every host
// on every run, which is the same as having no warning signal at all.
//
// Each pattern is ANCHORED and matches the WHOLE shape of the line, not just a
// prefix. That matters: git reports its failures through the same "remote: "
// channel as its progress, so a prefix test would have whitelisted
// "remote: fatal: repository not found". Listing the exact progress verbs
// keeps the classifier a denylist of known-good shapes rather than a guess
// about what an error looks like — the same reason design §3.1 rejected
// classifying stderr by text in the first place.
var benignPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^Warning: Permanently added .* to the list of known hosts\.$`),
	regexp.MustCompile(`^Pseudo-terminal will not be allocated`),
	regexp.MustCompile(`^remote: (Enumerating|Counting|Compressing|Total|Finding) `),
	regexp.MustCompile(`^(Receiving|Resolving|Counting|Compressing|Unpacking|Enumerating) (objects|deltas):`),
	regexp.MustCompile(`^From (https?://|git@|/)`),
	// NOTE: no leading-space patterns. Benign() trims the line first, so
	// `^ \* branch …` could never match — and every real `git fetch` would put
	// a spurious ⚠ on the row, the exact failure §4.5 exists to prevent.
	regexp.MustCompile(`^\* \[?new (branch|tag)\]?`),
	regexp.MustCompile(`^\* branch\s+\S+\s+-> FETCH_HEAD$`),
	regexp.MustCompile(`^\s*[0-9a-f]{7,40}\.\.[0-9a-f]{7,40}\s+\S+\s+-> \S+$`),
	regexp.MustCompile(`^\[sudo\] password for `),
}

// Benign reports whether a stderr line is routine chatter rather than a
// warning. It NEVER hides the line — the error pane shows every stderr line;
// this only decides whether the host's row gets a ⚠.
//
// Matching is deliberately conservative: an unknown line is a WARNING. A false
// ⚠ costs a glance; a missed one costs a half-installed host reporting success.
func Benign(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return true
	}
	for _, re := range benignPatterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
```

Imports: `regexp` and `strings`. Note the trade: an anchored table needs a new line added when a
tool changes its progress wording, and the failure mode of that is a spurious ⚠ — visible and
cheap. The opposite failure mode (a broad prefix swallowing a real error) is silent, which is
the one this objective exists to prevent.

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./internal/updexec/ -run Benign -v`
Expected: PASS (every table row).

**Done-when:** the table passes with no row commented out.
**Evidence:** `evidence/warn-badge/task05-benign.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/internal/updexec/stderrnoise.go sdk/fleet/internal/updexec/stderrnoise_test.go
git commit -m "feat(fleet/updexec): Benign() classifies routine stderr chatter"
```

---

### Task 6: `cmd` — carry the stderr tag from the stream into the model

**Files:**

- Modify: `sdk/fleet/cmd/tui_cmds.go` (`lineQueue` ~:290, `stream` ~:52, `logLineMsg` ~:42,
  `beginStream` ~:355, `readLine` ~:400)
- Modify: `sdk/fleet/cmd/tui_model.go` (`logEntry` ~:70, `appendLog` ~:530, `Update`'s
  `logLineMsg` case)
- **Modify (existing tests that touch the retyped values — they will NOT compile otherwise):**
  `sdk/fleet/cmd/review_test.go:194,205` (`q.push("line-%d")`, `q.forward(chan string)`) and
  `sdk/fleet/cmd/tui_logpane_test.go:129` (`stream{lines: make(chan string), …}`). Update them
  to `outLine` / `chan outLine`; do not change what they assert.
- Test: `sdk/fleet/cmd/tui_stderr_test.go` *(new)*

**Interfaces:**

- Consumes: `Console.ErrLine` (Task 3), `Fake.ErrOut` (Task 2), `updexec.Benign` (Task 5).
- Produces: `outLine{text string; stderr bool}`; `logEntry.stderr`, `logEntry.warn`;
  `(*tuiModel).appendLogLine(alias, line string, isErr bool)`; `m.warns map[string]int`;
  `logLineMsg{alias, line string, stderr bool}`.

- [ ] **Step 1: Write the failing test**

```go
func TestStderrLineIsTaggedAndCounted(t *testing.T) {
	m := testModel("a")
	m.appendLogLine("a", "installing", false)
	m.appendLogLine("a", "WARNING: apt-get update failed", true)
	m.appendLogLine("a", "Receiving objects:  73% (30/41)", true) // benign

	if len(m.logs) != 3 {
		t.Fatalf("the log buffer keeps every line, got %d", len(m.logs))
	}
	if got := m.errEntries(); len(got) != 2 {
		t.Fatalf("the error projection is the stderr subset, got %d", len(got))
	}
	if m.warns["a"] != 1 {
		t.Fatalf("only non-benign stderr raises the warning count, got %d", m.warns["a"])
	}
}

// appendLog (the 2-arg form every existing test uses) must keep meaning stdout.
func TestAppendLogStaysStdout(t *testing.T) {
	m := testModel("a")
	m.appendLog("a", "hello")
	if m.logs[0].stderr || m.warns["a"] != 0 {
		t.Fatal("appendLog must remain the stdout form")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'Stderr|AppendLog' -v`
Expected: FAIL — `m.appendLogLine undefined`.

- [ ] **Step 3: Implement**

`tui_model.go`:

```go
type logEntry struct {
	alias, line string
	at          time.Time
	stderr      bool // the remote wrote this to stderr
	warn        bool // stderr AND not routine chatter — computed ONCE, here,
	                 // because View() must stay pure and cheap
}

// appendLog is the stdout form, kept so every existing caller and test reads
// the same as before.
func (m *tuiModel) appendLog(alias, line string) { m.appendLogLine(alias, line, false) }

// appendLogLine adds a line, tagged by the stream that produced it, and
// enforces the cap. ONE buffer feeds both panes: two buffers would evict at
// different rates and the panes would disagree about what arrived.
func (m *tuiModel) appendLogLine(alias, line string, isErr bool) {
	if m.logColor == nil {
		m.logColor = map[string]int{}
	}
	if _, ok := m.logColor[alias]; !ok {
		m.logColor[alias] = len(m.logColor)
	}
	warn := isErr && !updexec.Benign(line)
	if warn {
		if m.warns == nil {
			m.warns = map[string]int{}
		}
		m.warns[alias]++
	}
	m.logs = append(m.logs, logEntry{alias: alias, line: line, at: nowFn(), stderr: isErr, warn: warn})
	if len(m.logs) > logCap {
		m.logs = m.logs[len(m.logs)-logCap:]
		if m.logTop > 0 {
			m.logTop--
		}
	}
}

// errEntries is the error pane's projection: the stderr subset, in order.
func (m tuiModel) errEntries() []logEntry {
	out := make([]logEntry, 0, len(m.logs))
	for _, e := range m.logs {
		if e.stderr {
			out = append(out, e)
		}
	}
	return out
}
```

Add `warns map[string]int` to `tuiModel` and initialise it in `newTUIModel`.

`tui_cmds.go`: make the queue and channel carry `outLine`.

```go
type outLine struct {
	text   string
	stderr bool
}

type stream struct {
	lines <-chan outLine
	done  <-chan error
}
```

`lineQueue.buf` becomes `[]outLine`, `push(l outLine)`, `forward(ch chan<- outLine)`.
In `beginStream`, wire both callbacks:

```go
			Line:    func(_, l string) { q.push(outLine{text: l}) },
			ErrLine: func(_, l string) { q.push(outLine{text: l, stderr: true}) },
```

`readLine` returns `logLineMsg{alias: alias, line: l.text, stderr: l.stderr}`, and the model's
`logLineMsg` case calls `m.appendLogLine(msg.alias, msg.line, msg.stderr)`.

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v 2>&1 | tail -30`
Expected: PASS — every existing `tui_logpane_test.go` / `tui_cmds_test.go` / `review_test.go`
case included, with the three retyped call sites above updated and their assertions unchanged.

**Done-when:** `go test ./...` PASS.
**Evidence:** `evidence/stderr/task06-model-tag.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/tui_cmds.go sdk/fleet/cmd/tui_model.go \
        sdk/fleet/cmd/tui_stderr_test.go sdk/fleet/cmd/review_test.go sdk/fleet/cmd/tui_logpane_test.go
git commit -m "feat(fleet/tui): carry the stderr tag into one buffer with two projections"
```

---

### Task 7: `cmd/tui_layout.go` — the pure layout function

**Files:**

- Create: `sdk/fleet/cmd/tui_layout.go`
- Test: `sdk/fleet/cmd/tui_layout_test.go` *(new)*

**Interfaces:**

- Consumes: nothing (pure ints).
- Produces: `panes{host,log,err bool}`, `heights{host,log,err int}`,
  `layout(vpHeight, chromeRows int, p panes, logActive, errActive bool) heights`,
  `minPaneRows`, `panelChrome`, `listHeaderRows`.

- [ ] **Step 1: Write the failing test**

```go
package cmd

import "testing"

func TestLayout(t *testing.T) {
	const chrome = 7 // banner 5 + separator 1 + status 1, as measured at >=80 cols

	cases := []struct {
		name                 string
		vp, chrome           int
		p                    panes
		logActive, errActive bool
		want                 heights
	}{
		{
			name: "host only takes everything",
			vp:   40, chrome: chrome, p: panes{host: true},
			want: heights{host: 40 - chrome - panelFixedRows},
		},
		{
			name: "log only takes everything",
			vp:   40, chrome: chrome, p: panes{log: true}, logActive: true,
			want: heights{log: 40 - chrome - panelFixedRows},
		},
		{
			name: "err only takes everything",
			vp:   40, chrome: chrome, p: panes{err: true}, errActive: true,
			want: heights{err: 40 - chrome - panelFixedRows},
		},
		{
			name: "open but empty stream panes reserve no body rows",
			vp:   40, chrome: chrome, p: panes{host: true, log: true, err: true},
			want: heights{host: 40 - chrome - 3*panelFixedRows},
		},
		{
			name: "host plus log splits, host on top",
			vp:   40, chrome: chrome, p: panes{host: true, log: true}, logActive: true,
			want: heights{host: 8, log: 40 - chrome - 2*panelFixedRows - 8},
		},
		{
			name: "three panes split the bottom, log gets the odd row",
			vp:   40, chrome: chrome, p: panes{host: true, log: true, err: true},
			logActive: true, errActive: true,
			want: heights{host: 8, log: 8, err: 8},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := layout(c.vp, c.chrome, c.p, c.logActive, c.errActive)

			// Invariant 1: nothing is negative.
			if got.host < 0 || got.log < 0 || got.err < 0 {
				t.Fatalf("negative height: %+v", got)
			}
			// Invariant 2: a hidden or inactive pane contributes no body rows.
			if !c.p.host && got.host != 0 {
				t.Fatalf("hidden host pane got %d rows", got.host)
			}
			if (!c.p.log || !c.logActive) && got.log != 0 {
				t.Fatalf("inactive log pane got %d rows", got.log)
			}
			if (!c.p.err || !c.errActive) && got.err != 0 {
				t.Fatalf("inactive error pane got %d rows", got.err)
			}
			// Invariant 3: bodies plus every open panel's fixed cost plus the
			// chrome never exceed the viewport. ONE definition of the total,
			// shared with the frame guard in Task 8.
			total := minFrameRows(c.chrome, c.p) + got.host + got.log + got.err
			if total > c.vp {
				t.Fatalf("layout overflows: %d > %d (%+v)", total, c.vp, got)
			}
			if got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestThreePaneSplitIsEven(t *testing.T) {
	got := layout(40, 7, panes{host: true, log: true, err: true}, true, true)
	if got.log < got.err || got.log-got.err > 1 {
		t.Fatalf("bottom split must be even with the odd row to the log: %+v", got)
	}
	if got.err < minPaneRows {
		t.Fatalf("error pane below its floor at 40 rows: %+v", got)
	}
}

// The host table keeps the top fifth once a stream pane is flowing — the
// proportion the dashboard shipped with. It is deliberately NOT today's exact
// number: today's overflows the terminal (design §1.3).
func TestHostPlusOneBottomPaneKeepsTheTopFifth(t *testing.T) {
	got := layout(40, 7, panes{host: true, log: true}, true, false)
	if got.err != 0 {
		t.Fatalf("a hidden error pane takes no rows: %+v", got)
	}
	if got.host != 40/hostShareDenom {
		t.Fatalf("host should keep the top fifth (%d), got %d", 40/hostShareDenom, got.host)
	}
	if got.log < minPaneRows {
		t.Fatalf("the log keeps its floor: %+v", got)
	}
}

// A viewport too small for the floors must still produce a frame that FITS.
// Floors are a preference: at 60x16 the chrome (8 rows) plus three panel
// frames (3 each) already claim 17 of the 16 rows, so insisting on a 3-row
// floor per pane would be insisting on rows that do not exist.
func TestTinyViewportStillFits(t *testing.T) {
	const chrome = 8 // banner 6 at 60 cols + separator 1 + status 1
	p := panes{host: true, log: true, err: true}
	got := layout(16, chrome, p, true, true)

	if got.host < 0 || got.log < 0 || got.err < 0 {
		t.Fatalf("no height may be negative: %+v", got)
	}
	if got != (heights{}) {
		t.Fatalf("with the fixed rows already over budget the panes take nothing: %+v", got)
	}
	// And the frame is then exactly its fixed cost — the panes added nothing.
	total := minFrameRows(chrome, p) + got.host + got.log + got.err
	if total != minFrameRows(chrome, p) {
		t.Fatalf("panes added rows to an over-budget frame: %d", total)
	}
}

// The same rationing one pane up: at 80x12 with host+log the fixed rows are
// 7+3+3 = 13 for a 12-row terminal, so both panes yield. This is the case the
// pre-existing TestSplitKeepsBothHalvesUsableOnASmallTerminal asserted the
// other way round — see Task 8, which updates it and says why.
func TestSmallTerminalYieldsRatherThanOverflowing(t *testing.T) {
	got := layout(12, 7, panes{host: true, log: true}, true, false)
	if got != (heights{}) {
		t.Fatalf("12 rows cannot hold two panels plus chrome: %+v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run TestLayout -v`
Expected: FAIL — `undefined: layout`.

- [ ] **Step 3: Implement**

```go
package cmd

// The pane model: `fleet tui` is three stacked panels the operator composes.
// The host table on top, the log stream and the error stream sharing the
// bottom. Every height in the frame comes from layout() and nowhere else —
// the arithmetic used to be spread over listHeight/logHeight with inline
// magic numbers, which is how the frame came to overflow the terminal by up
// to 12 rows (docs/mbo/designs/fleet-error-view.md §1.3).

type pane int

const (
	paneHost pane = iota
	paneLog
	paneErr
)

type panes struct{ host, log, err bool }

func (p panes) count() int {
	n := 0
	for _, on := range []bool{p.host, p.log, p.err} {
		if on {
			n++
		}
	}
	return n
}

type heights struct{ host, log, err int }

const (
	// panelBorderRows is a framed panel's top and bottom border.
	panelBorderRows = 2
	// panelLeadRows is the ONE line every open panel writes inside its frame
	// before any body row: the host panel's column header, and a stream
	// pane's title line (logView writes the title into the body before the
	// log lines) — or, when that pane has nothing to show, its collapsed
	// hint, which occupies exactly the same single line.
	panelLeadRows = 1
	// panelFixedRows is therefore what ANY open panel costs before a single
	// body row. Measured against the real render: at 100x40 with host+log,
	// banner 5 + (1+6+2) + (1+24+2) + separator 1 + status 1 = 43.
	panelFixedRows = panelBorderRows + panelLeadRows
	// minPaneRows is the body height below which a pane is useless. It is a
	// PREFERENCE, not a guarantee — see minFrameRows.
	minPaneRows = 3
	// hostShareDenom gives the host table the top fifth once a stream pane is
	// flowing — the ratio the dashboard shipped with.
	hostShareDenom = 5
)

// minFrameRows is what the frame costs with ZERO body rows: the measured
// chrome plus every open panel's fixed cost. When it exceeds the terminal, no
// pane arithmetic can make the frame fit — the panes' job is then to add
// nothing at all. The invariant test uses it as its budget, so the guard and
// the layout cannot disagree about what "fits" means.
func minFrameRows(chromeRows int, p panes) int {
	return chromeRows + panelFixedRows*p.count()
}

// layout splits vpHeight between the visible panes. chromeRows is MEASURED by
// the caller (banner + separator + status), never assumed: the banner's
// key-hint strip wraps at narrow widths, and the "status line" is a framed
// panel of 8-12 rows in modeAnswers/modeConfirm.
//
// A pane that is open but has nothing to show (logActive/errActive false)
// gets zero body rows and renders its one-line hint — an empty box must not
// cost the fleet view a fifth of the screen to say nothing. It costs the same
// panelFixedRows either way, which is why the deduction below does not care.
func layout(vpHeight, chromeRows int, p panes, logActive, errActive bool) heights {
	avail := vpHeight - minFrameRows(chromeRows, p)
	if avail < 0 {
		// Chrome + panel frames already exceed the terminal (a dialog on a
		// short screen, or three panels at 60x16). The panes must never ADD to
		// a frame that already does not fit.
		avail = 0
	}

	bottom := (p.log && logActive) || (p.err && errActive)
	switch {
	case !bottom && !p.host:
		return heights{}
	case !bottom:
		return heights{host: avail}
	case !p.host:
		return splitBottom(avail, p.log && logActive, p.err && errActive)
	}

	host := vpHeight / hostShareDenom
	if host < minPaneRows {
		host = minPaneRows
	}
	if host > avail-minPaneRows {
		host = avail - minPaneRows
	}
	if host < 0 {
		host = 0
	}
	h := splitBottom(avail-host, p.log && logActive, p.err && errActive)
	h.host = host
	return h
}

// splitBottom shares the bottom region. With both stream panes active it is
// halved, the odd row going to the log (the primary). If both floors do not
// fit, the log keeps the whole region and the error pane yields — one usable
// pane beats two unreadable ones.
//
// It deliberately does NOT consider which pane has focus: a focus-dependent
// split would resize the frame every time the operator pressed tab, and a
// layout you cannot predict is worse than one that is merely unequal. An
// error pane left with zero body rows still renders its frame and one line,
// which minFrameRows already budgeted — that is why yielding is safe here.
func splitBottom(avail int, log, err bool) heights {
	if avail < 0 {
		avail = 0
	}
	switch {
	case log && err:
		if avail < 2*minPaneRows {
			return heights{log: avail}
		}
		e := avail / 2
		return heights{log: avail - e, err: e}
	case log:
		return heights{log: avail}
	case err:
		return heights{err: avail}
	}
	return heights{}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -run 'TestLayout|ThreePane|TopFifth|TinyViewport|SmallTerminalYields' -v`
Expected: PASS. Every `want` above is derived from `minFrameRows`, not from what the code
happens to return — if one disagrees, the arithmetic is wrong, not the expectation. Do not
weaken an invariant to make a number fit.

**Done-when:** the layout suite passes; `layout` has no caller yet (that is Task 8).
**Evidence:** `evidence/layout/task07-layout-unit.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/tui_layout.go sdk/fleet/cmd/tui_layout_test.go
git commit -m "feat(fleet/tui): a pure layout() owning every pane height"
```

---

### Task 8: `cmd` — adopt `layout()` and make the frame fit the terminal

**Files:**

- Modify: `sdk/fleet/cmd/tui_model.go` (`logActive` ~:545, `logHeight` ~:552, `listHeight` ~:566,
  `visibleRows` ~:200)
- Modify: `sdk/fleet/cmd/tui_view.go` (`View` ~:95, add `chromeRows()`)
- Test: `sdk/fleet/cmd/tui_layout_test.go`
- **Rewrite one pre-existing test:** `sdk/fleet/cmd/tui_logpane_test.go:97`
  (`TestSplitKeepsBothHalvesUsableOnASmallTerminal` — it asserts today's overflow; see Step 3)

**Interfaces:**

- Consumes: `layout()` (Task 7).
- Produces: `(tuiModel).chromeRows() int`, `(tuiModel).paneState() panes`,
  `(tuiModel).heights() heights`, `(tuiModel).errHeight() int`, `m.errCount int`;
  `listHeight`/`logHeight` keep their names and become `layout()` lookups.

- [ ] **Step 1: Write the failing test**

```go
// TestFrameFitsTheTerminal is the invariant the dashboard never had: whatever
// the size, the mode, or which panes are open, the frame must fit. It FAILS on
// the code that preceded this task — the frame overflowed by up to 12 rows at
// 60x16 (docs/mbo/designs/fleet-error-view.md §1.3).
func TestFrameFitsTheTerminal(t *testing.T) {
	sizes := [][2]int{{60, 16}, {80, 24}, {100, 40}, {200, 60}}
	combos := []panes{
		{host: true}, {log: true}, {err: true},
		{host: true, log: true}, {host: true, err: true},
		{log: true, err: true}, {host: true, log: true, err: true},
	}
	// modeHelp is deliberately absent: its View() replaces the whole frame with
	// the `?` overlay (no panes, no status), so it is not a pane-layout case at
	// all. The overlay's own height is a pre-existing limitation recorded in
	// design §5 with its own follow-up — this objective must not pretend to fix
	// it, and must not let a test claim it did.
	modes := []tuiMode{modeNormal, modeSearch, modeAnswers, modeConfirm}

	for _, size := range sizes {
		for _, p := range combos {
			for _, full := range []bool{false, true} {
				for _, mode := range modes {
					m := settledTestModel(8) // 8 resolved hosts
					m.vp = viewport{width: size[0], height: size[1]}
					m.hostOpen, m.logOpen, m.errOpen = p.host, p.log, p.err
					m.mode = mode
					if full {
						for i := 0; i < 200; i++ {
							m.appendLogLine("h1", fmt.Sprintf("line %d", i), i%3 == 0)
						}
					}
					v := m.View()
					// The achievable invariant: the PANES never add to an
					// over-budget frame. The budget is minFrameRows — the
					// measured chrome plus every open panel's fixed cost —
					// which is the SAME function layout() rations against, so
					// the guard and the layout cannot disagree. At 60x16 in
					// modeAnswers the chrome alone is 19 rows (banner 6 +
					// separator 1 + the answer panel 12), so a bare
					// "<= vp.height" would be a test no implementation can
					// pass without making the dialogs scroll (out of scope).
					budget := size[1]
					if fixed := minFrameRows(m.chromeRows(), m.paneState()); fixed > budget {
						budget = fixed
					}
					if h := lipgloss.Height(v); h > budget {
						t.Errorf("%dx%d panes=%+v full=%v mode=%v: %d rows > %d",
							size[0], size[1], p, full, mode, h, budget)
					}
					if w := lipgloss.Width(v); w > size[0] {
						t.Errorf("%dx%d panes=%+v full=%v mode=%v: %d cols > %d",
							size[0], size[1], p, full, mode, w, size[0])
					}
					if strings.TrimSpace(v) == "" {
						t.Errorf("%dx%d panes=%+v: empty frame", size[0], size[1], p)
					}
				}
			}
		}
	}
}

// When the chrome alone is taller than the terminal — a dialog on a very
// short screen — every pane must yield, rather than the layout piling rows
// onto a frame that already does not fit.
func TestChromeOverBudgetGivesThePanesNothing(t *testing.T) {
	m := settledTestModel(5)
	m.vp = viewport{width: 60, height: 16}
	m.hostOpen, m.logOpen, m.errOpen = true, true, true
	m.appendLogLine("h1", "out", false)
	m.appendLogLine("h1", "boom", true)
	m.mode = modeAnswers // the answer panel alone is 12 rows at 60 columns

	if minFrameRows(m.chromeRows(), m.paneState()) <= m.vp.height {
		t.Skip("the fixed rows fit here; this case only exists when they do not")
	}
	h := m.heights()
	if h.host != 0 || h.log != 0 || h.err != 0 {
		t.Fatalf("over-budget chrome must leave the panes nothing, got %+v", h)
	}
}

// A body row must be exactly ONE rendered line, or every height in layout()
// is meaningless. th.panel has Padding(0,1), so panel.Width(w) leaves w-2
// usable columns (measured: at Width(76) a 74-char line renders 3 rows and a
// 75-char line renders 4) — but View() truncates rows to panelWidth() and the
// column header not at all. At 80 columns an 8-host panel renders 20 lines.
func TestPanelBodyRowIsExactlyOneLine(t *testing.T) {
	for _, w := range []int{60, 80, 100, 200} {
		m := settledTestModel(8)
		m.vp = viewport{width: w, height: 60} // tall enough that nothing is cut
		m.logOpen, m.errOpen = false, false
		// The widest row this table can produce, so the check is not passing
		// only because the fixture is short.
		m.rows[0].Alias = "host-with-a-very-long-name-xyz"
		m.rows[0].Branch = "feature/a-long-branch-name"
		m.rows[0].InstalledBranch = "main"
		m.updating[m.rows[0].Alias] = updState{
			phase: updFail,
			log:   "install.sh: a very long failure explanation that must be truncated, not wrapped",
		}

		want := len(m.rows) + panelFixedRows
		if got := lipgloss.Height(m.View()) - m.chromeRows(); got != want {
			t.Errorf("width %d: host panel rendered %d lines for %d rows, want %d",
				w, got, len(m.rows), want)
		}
	}
}

// An empty fleet and a single-host fleet are the two shapes that used to
// divide by zero or clamp to a negative height.
func TestFrameFitsWithZeroAndOneHost(t *testing.T) {
	for _, n := range []int{0, 1} {
		m := settledTestModel(n)
		m.vp = viewport{width: 60, height: 16}
		m.hostOpen, m.logOpen, m.errOpen = true, true, true
		if h := lipgloss.Height(m.View()); h > 16 {
			t.Fatalf("%d hosts: %d rows > 16", n, h)
		}
	}
}
```

Add the `settledTestModel(n int) tuiModel` helper next to the existing `testModel` helper
(resolved rows `h1..hn`, `pending` cleared, `resort()` called) — read `testModel` first and
follow it exactly.

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run TestFrameFits -v 2>&1 | head -30`
Expected: FAIL with concrete overflow lines, e.g. `60x16 panes={true false false} … 28 rows > 16`.
**Record this output — it is the evidence that the bug existed.**

- [ ] **Step 3: Implement**

**First, make a row one line.** `th.panel` has `Padding(0, 1)`, so `panel.Width(w)` leaves
`w-2` usable columns. Add the accessor and use it for **every** truncation:

```go
// panelInnerWidth is the columns a panel's CONTENT actually gets: panelWidth
// less th.panel's Padding(0, 1). Truncating to panelWidth() instead — which is
// what every call site did — leaves each line two columns too wide, so
// lipgloss wraps it and a one-row host becomes two rendered lines. Measured:
// at panel.Width(76) a 74-column line renders 3 rows, a 75-column line 4.
func (m tuiModel) panelInnerWidth() int {
	w := m.panelWidth() - 2
	if w < 20 {
		w = 20
	}
	return w
}
```

Call sites to move onto it: the host rows (`trunc(m.rowView(i), …)`), the **column header**
(which is not truncated at all today and is ~91 columns wide — this is why the header alone
wraps at 80), `failWidth()`'s budget, and `logWidth()` for the stream panes' line text.

```go
// chromeRows is the frame's fixed cost, MEASURED not assumed. BOTH ends of it
// vary: the banner's key-hint strip wraps at narrow widths, and the status
// area is a full framed PANEL in modeAnswers and modeConfirm rather than one
// line. Assuming three lines of banner and two of status is how the frame came
// to overflow the terminal by up to 12 rows.
func (m tuiModel) chromeRows() int {
	return lipgloss.Height(m.banner()) + statusSeparatorRows + lipgloss.Height(m.statusView())
}

// statusSeparatorRows is the blank line View() emits between the last panel
// and the status area.
const statusSeparatorRows = 1

func (m tuiModel) paneState() panes {
	return panes{host: m.hostOpen, log: m.logOpen, err: m.errOpen}
}

func (m tuiModel) heights() heights {
	return layout(m.vp.height, m.chromeRows(), m.paneState(), m.logActive(), m.errActive())
}

func (m tuiModel) listHeight() int { return m.heights().host }
func (m tuiModel) logHeight() int  { return m.heights().log }
func (m tuiModel) errHeight() int  { return m.heights().err }

// errActive mirrors logActive: open AND with something to show. It reads a
// COUNTER, not the projection: heights() is called from listHeight(),
// logHeight(), errHeight(), visibleRows() and clampViewport() — i.e. on every
// keystroke and on every spinner tick — and rebuilding a filtered copy of a
// 2000-entry ring that often is a render-path cost with no reason to exist.
// appendLogLine maintains it (and decrements it when the cap evicts a tagged
// entry, which is the one place it could silently drift).
func (m tuiModel) errActive() bool { return m.errOpen && m.errCount > 0 }
```

**Call `heights()` once per frame.** `chromeRows()` renders the banner and the status area to
measure them; `View()` must compute `h := m.heights()` at the top and pass the three numbers
down, rather than calling `listHeight()`/`logHeight()`/`errHeight()` per panel. Outside `View()`
the individual accessors stay (motion code calls `visibleRows()`), which is cheap enough — but
pin the counter's correctness:

```go
func TestErrCountTracksTheProjection(t *testing.T) {
	m := testModel("a")
	for i := 0; i < logCap+50; i++ {
		m.appendLogLine("a", fmt.Sprintf("line %d", i), i%2 == 0)
	}
	if m.errCount != len(m.errEntries()) {
		t.Fatalf("errCount drifted from the projection after eviction: %d != %d",
			m.errCount, len(m.errEntries()))
	}
}
```

`View()` renders the host panel only `if m.hostOpen`, the log panel `if m.logOpen`, and the
error panel `if m.errOpen`, and truncates each pane's body to its `heights` value. `visibleRows()`
already returns `listHeight()`, so motion and paging follow automatically.

**Fold the two early returns into the pane loop.** `View()` currently `return`s from inside the
`len(m.rows) == 0` branch (the "no fleet hosts found" panel) — with panes that would swallow the
log and error panes and the status line whenever the fleet is empty. The empty-fleet message
becomes the *host pane's* body; the rest of the frame renders as usual. (`modeHelp` keeps its
own full-frame early return: the overlay deliberately replaces everything.) Pin it:

```go
func TestEmptyFleetStillRendersTheOtherPanes(t *testing.T) {
	m := testModel() // no hosts
	m.vp = viewport{width: 100, height: 40}
	m.hostOpen, m.errOpen = false, true
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	v := m.View()
	if !strings.Contains(v, "apt-get") {
		t.Fatal("an empty fleet must not swallow the error pane")
	}
	if lipgloss.Height(v) > 40 {
		t.Fatalf("empty fleet frame overflows: %d rows", lipgloss.Height(v))
	}
}
```

Where the invariant still fails, the fix is in `layout()`'s arithmetic or in `chromeRows()` —
**never** by loosening the test.

**One pre-existing test must change, and only this one.**
`TestSplitKeepsBothHalvesUsableOnASmallTerminal` (`tui_logpane_test.go:97`) asserts that at
**80×12** both `visibleRows()` and `logHeight()` are ≥ 1. That is precisely the overflow this
task fixes: at 80×12 the chrome (7) plus two panel frames (3 + 3) is **13 rows for a 12-row
terminal**, so "both halves usable" is only achievable by rendering past the bottom of the
screen — which is what it does today. Rewrite it to assert the new, achievable contract, and
say so in the commit:

```go
// On a terminal that CAN fit both halves, the split leaves both usable.
func TestSplitKeepsBothHalvesUsableWhenTheyFit(t *testing.T) {
	m := testModel("a", "b")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	open := mm.(tuiModel)
	open.appendLog("a", "line")
	if open.visibleRows() < 1 || open.logHeight() < 1 {
		t.Fatalf("split collapsed: list=%d log=%d", open.visibleRows(), open.logHeight())
	}
}

// Below that, the frame FITS instead: chrome (7) + two panel frames (6) is 13
// rows, so a 12-row terminal has nothing left to hand out. Rendering both
// halves "usable" there is what put the frame 7 rows past the bottom of the
// screen (design §1.3).
func TestSmallTerminalFitsRatherThanKeepingBothHalves(t *testing.T) {
	m := testModel("a", "b")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	open := mm.(tuiModel)
	open.appendLog("a", "line")
	if lipgloss.Height(open.View()) > minFrameRows(open.chromeRows(), open.paneState()) {
		t.Fatal("the panes must add nothing when the fixed rows are already over budget")
	}
}
```

Every OTHER case in `tui_logpane_test.go` must pass untouched — if a second one fails, stop and
record it in `TRACKING.md` §4 rather than editing it.

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v 2>&1 | tail -40`
Expected: PASS, including every pre-existing `tui_logpane_test.go` case except the one
rewritten above.

**Done-when:** `TestFrameFitsTheTerminal` PASS at all four sizes × 7 combos × 5 modes;
`go test ./...` PASS.
**Evidence:** `evidence/layout/task08-before-overflow.txt` (the RED output) **and**
`evidence/layout/task08-after-fits.txt` (the GREEN run) — both committed.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/tui_model.go sdk/fleet/cmd/tui_view.go \
        sdk/fleet/cmd/tui_layout_test.go sdk/fleet/cmd/tui_logpane_test.go
git commit -m "fix(fleet/tui): the frame no longer overflows the terminal

Every pane height now comes from layout(), and the chrome is measured
rather than assumed — the banner's hint strip wraps, which (with a 3-row
under-count of panel borders) made the frame 12 rows too tall at 60x16."
```

---

### Task 9: `cmd` — the `h` and `e` keys, focus over visible panes

**Files:**

- Modify: `sdk/fleet/cmd/tui_keys.go` (`keyHelp` ~:20, `routeNormal` ~:230, `routeSearch` ~:150)
- Modify: `sdk/fleet/cmd/tui_model.go` (`focus pane` replacing `logFocus bool`)
- **Modify (existing tests that use the removed field):** `sdk/fleet/cmd/tui_demo_test.go:124`
  *assigns* `m.logFocus = true` (a method cannot be assigned, so this is a compile error, not a
  behaviour change) and `sdk/fleet/cmd/tui_reset_lognav_test.go:125,145` read it. Set
  `m.focus = paneLog` and read `logFocused()` respectively; assertions stay as they are.
- Test: `sdk/fleet/cmd/tui_panes_test.go` *(new)*

**Interfaces:**

- Consumes: `panes`, `pane`, `layout()`.
- Produces: `m.hostOpen`, `m.errOpen`, `m.focus pane`, `(*tuiModel).togglePane(pane)`,
  `(*tuiModel).cycleFocus()`, `(tuiModel).logFocused() bool` (compatibility shim for the
  existing log tests).

- [ ] **Step 0: Teach the test harness the `tab` key**

`key()` in `tui_model_test.go:32` maps only `esc`, `enter`, `space`, `backspace`, `ctrl+d`,
`ctrl+u` — everything else becomes `KeyRunes`. `send(m, "tab")` would therefore type the letters
`t`, `a`, `b` and the focus test would pass or fail for the wrong reason. Add one case:

```go
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
```

- [ ] **Step 1: Write the failing tests**

```go
func TestPaneDefaults(t *testing.T) {
	m := testModel("a")
	if !m.hostOpen || !m.logOpen || m.errOpen {
		t.Fatalf("defaults are host+log on, error off: %v %v %v", m.hostOpen, m.logOpen, m.errOpen)
	}
}

func TestHostPaneTogglesAndRestores(t *testing.T) {
	m := testModel("a", "b", "c")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	full := mm.(tuiModel)
	before, cursor := full.visibleRows(), full.cursor

	hidden, _ := send(full, "h")
	if hidden.hostOpen {
		t.Fatal("`h` must hide the host pane")
	}
	if hidden.visibleRows() != 0 {
		t.Fatalf("a hidden host pane reserves no rows, got %d", hidden.visibleRows())
	}

	back, _ := send(hidden, "h")
	if !back.hostOpen || back.visibleRows() != before || back.cursor != cursor {
		t.Fatalf("`h h` must restore height %d and cursor %q, got %d/%q",
			before, cursor, back.visibleRows(), back.cursor)
	}
}

func TestErrorPaneTogglesAndDoesNotStealConfirmEdit(t *testing.T) {
	m := testModel("a")
	on, _ := send(m, "e")
	if !on.errOpen {
		t.Fatal("`e` must open the error pane in normal mode")
	}
	// `e` in the confirm strip still means "edit the answers".
	c := testModel("a")
	c.ans = answers{sudoSecret: "xx"}
	c.mode = modeConfirm
	edited, _ := send(c, "e")
	if edited.mode != modeAnswers {
		t.Fatalf("`e` in modeConfirm must still edit answers, mode=%v", edited.mode)
	}
	if edited.errOpen {
		t.Fatal("`e` in modeConfirm must NOT toggle the error pane")
	}
}

func TestHidingTheLastPaneIsRefused(t *testing.T) {
	m := testModel("a")
	m.logOpen, m.errOpen = false, false // host is the only one left
	got, _ := send(m, "h")
	if !got.hostOpen {
		t.Fatal("the last visible pane must not be hideable")
	}
	if got.status == "" {
		t.Fatal("the refusal must say why")
	}
	// Idempotent: pressing again changes nothing.
	again, _ := send(got, "h")
	if !again.hostOpen {
		t.Fatal("still refused")
	}
}

func TestFocusCyclesVisiblePanesOnly(t *testing.T) {
	m := testModel("a")
	m.appendLogLine("a", "x", false)
	m.hostOpen, m.logOpen, m.errOpen = true, true, false

	one, _ := send(m, "tab")
	if one.focus != paneLog {
		t.Fatalf("tab: host -> log, got %v", one.focus)
	}
	two, _ := send(one, "tab")
	if two.focus != paneHost {
		t.Fatalf("tab must skip the hidden error pane, got %v", two.focus)
	}

	// Hiding the focused pane moves focus somewhere visible.
	three := two
	three.focus = paneLog
	hid, _ := send(three, "l")
	if hid.focus == paneLog {
		t.Fatal("focus must leave a pane that was just hidden")
	}
}

func TestPerPaneSearchIsIndependent(t *testing.T) {
	m := testModel("a")
	m.appendLogLine("a", "installing", false)
	m.appendLogLine("a", "WARNING: apt-get update failed", true)
	m.errOpen, m.focus = true, paneErr

	s, _ := send(m, "/")
	s, _ = send(s, "a")
	s, _ = send(s, "p")
	s, _ = send(s, "t")
	s, _ = send(s, "enter")

	if s.errSearch.input != "apt" {
		t.Fatalf("the error pane owns its pattern, got %q", s.errSearch.input)
	}
	if s.search.input != "" || s.logSearch.input != "" {
		t.Fatal("searching the error pane must not disturb the host filter or the log")
	}
}

func TestPaneStateIsNotPersisted(t *testing.T) {
	a := testModel("x")
	a.hostOpen, a.errOpen = false, true
	b := testModel("x")
	if !b.hostOpen || b.errOpen {
		t.Fatal("a new model always starts at the defaults; pane state is session-only")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'Pane|Focus' -v`
Expected: FAIL — `m.hostOpen undefined`.

- [ ] **Step 3: Implement**

In `tui_model.go`: add `hostOpen`, `errOpen`, `focus pane`, `errFollow`, `errTop`,
`errSearch searchState`; set `hostOpen: true`, `logOpen: true`, `errOpen: false`,
`focus: paneHost` in `newTUIModel`. Replace `logFocus bool` with
`func (m tuiModel) logFocused() bool { return m.focus == paneLog && m.logOpen }` and update its
call sites in `tui_keys.go` / `tui_view.go`.

**Deliberately NOT refactored:** `errFollow`/`errTop`/`errSearch` sit alongside
`logFollow`/`logTop`/`logSearch` as parallel fields rather than being folded into a shared
`streamState` struct. The struct would be tidier, but the existing suite reads `m.logFollow`
and `m.logTop` directly in a dozen places, and rewriting those is a refactor this objective did
not ask for. The duplication is contained to three fields; the *behavior* is shared (one helper
over pointers, below).

Also update `routeSearch`: its `inLog := m.logFocus && m.logOpen` becomes a selection over the
focused pane, returning `&m.search` (host), `&m.logSearch`, or `&m.errSearch`, and committing a
pattern jumps within that pane. A search typed into one pane must not touch another's state —
that is what `TestPerPaneSearchIsIndependent` pins.

```go
// visiblePanes is the cycle order: host, then the streams below it.
func (m tuiModel) visiblePanes() []pane {
	var out []pane
	if m.hostOpen {
		out = append(out, paneHost)
	}
	if m.logOpen {
		out = append(out, paneLog)
	}
	if m.errOpen {
		out = append(out, paneErr)
	}
	return out
}

// togglePane flips one pane, REFUSING to hide the last visible one: a
// dashboard with nothing on it has no key left that obviously brings it back.
func (m *tuiModel) togglePane(p pane) {
	open := map[pane]*bool{paneHost: &m.hostOpen, paneLog: &m.logOpen, paneErr: &m.errOpen}[p]
	if *open && len(m.visiblePanes()) == 1 {
		m.status = "at least one view must stay open"
		return
	}
	*open = !*open
	if !*open && m.focus == p {
		m.cycleFocus()
	}
	m.clampViewport()
}

// cycleFocus moves to the next VISIBLE pane, wrapping.
func (m *tuiModel) cycleFocus() {
	vis := m.visiblePanes()
	if len(vis) == 0 {
		return
	}
	for i, p := range vis {
		if p == m.focus {
			m.focus = vis[(i+1)%len(vis)]
			return
		}
	}
	m.focus = vis[0]
}
```

In `routeNormal`: `case "h": m.togglePane(paneHost)`, `case "e": m.togglePane(paneErr)`,
`case "l": m.togglePane(paneLog); m.logFollow = true`, `case "tab": m.cycleFocus()`.
The error pane gets the same focused-pane motion block the log pane has (`j/k`, `ctrl+d/u`,
`ctrl+f/b`, `gg`, `G` re-follows, `/`, `n`/`N`) driving `errTop`/`errFollow`/`errSearch` —
extract the shared body into a helper over pointers rather than copying it.

**`keyHelp`: add ONE entry, and EDIT the existing `e` — do not add a second.**
`tui_keys.go:46` already declares `{"✏️", "e", "(confirm) edit the remembered answers · enter
runs the update", false}`. A second `e` row fails the pre-existing
`TestKeyHelpHasNoDuplicateBindings` (`tui_config_test.go:29`), which Task 13 forbids modifying —
and rightly: two rows for one letter is exactly the drift `keyHelp` exists to prevent.

```go
	// new, next to the log entry:
	{"🗂️", "h", "show / hide the host list", true},
	// EDIT the existing e entry in place — one key, one row, both meanings:
	{"⚠️", "e", "show / hide the stderr pane · (confirm) edit the remembered answers", true},
```

`s` (ssh) already uses the 🖥️ icon, so `h` takes a different one (🗂️) and the header strip stays
scannable. Promoting `e` to `hdr:true` moves it into the always-visible strip — check the strip
still fits at 60 columns (`headerHints` splits it across two rows; `TestFrameFitsTheTerminal`
covers the width).

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v 2>&1 | tail -40`
Expected: PASS, including `TestFrameFitsTheTerminal` and the existing log-pane suite.

**Done-when:** the pane suite passes; `?` overlay and header strip both list `h` and `e`.
**Evidence:** `evidence/layout/task09-keys.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/tui_keys.go sdk/fleet/cmd/tui_model.go sdk/fleet/cmd/tui_panes_test.go \
        sdk/fleet/cmd/tui_demo_test.go sdk/fleet/cmd/tui_reset_lognav_test.go
git commit -m "feat(fleet/tui): h hides the host list, e opens the error pane"
```

---

### Task 10: `cmd` — render the error pane (shared `streamPane`)

**Files:**

- Modify: `sdk/fleet/cmd/tui_view.go` (`logView` ~:170 → `streamPane`, `View` ~:95)
- Test: `sdk/fleet/cmd/tui_panes_test.go`

**Interfaces:**

- Consumes: `errEntries()`, `errHeight()`, `errSearch`, `errTop`, `errFollow`.
- Produces: `streamPane(cfg streamPaneCfg) string` and `(tuiModel).errView() string`.

- [ ] **Step 1: Write the failing test**

```go
func TestErrorPaneRendersOnlyStderr(t *testing.T) {
	m := settledTestModel(3)
	m.vp = viewport{width: 100, height: 40}
	m.errOpen = true
	m.appendLogLine("h1", "installing packages", false)
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)

	v := m.View()
	if !strings.Contains(v, "WARNING: apt-get update failed") {
		t.Fatal("the error pane must show the stderr line")
	}
	// The log pane keeps EVERYTHING: closing the error pane must not lose it.
	m.errOpen = false
	if !strings.Contains(m.View(), "WARNING: apt-get update failed") {
		t.Fatal("stderr must remain visible in the log pane")
	}
}

func TestErrorPaneOpenButEmptyCollapses(t *testing.T) {
	m := settledTestModel(3)
	m.vp = viewport{width: 100, height: 40}
	m.errOpen = true
	rowsWithout := m.listHeight()
	m.appendLogLine("h1", "boom", true)
	if m.listHeight() >= rowsWithout {
		t.Fatal("once stderr flows the error pane claims height and the list shrinks")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'ErrorPane' -v`
Expected: FAIL — the frame contains no error panel.

- [ ] **Step 3: Implement**

Generalise `logView` into one renderer parameterised by the entries, height, title, empty-hint,
scroll state and search state; `logView()` and `errView()` become thin callers. In the **log**
pane, a `stderr` entry renders a dim red `!` in the gutter before the host tag, so the merged
transcript still says which stream a line came from. The error pane's title carries the warning
count (`errors — 2 warnings on 1 host`), and its empty hint reads
`⚠️ stderr: none captured — this pane fills when a host writes to stderr  (e: hide)`.

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v 2>&1 | tail -20` — PASS.

**Done-when:** both new tests pass and `TestFrameFitsTheTerminal` still passes with the error
pane populated.
**Evidence:** `evidence/layout/task10-errview.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/tui_view.go sdk/fleet/cmd/tui_panes_test.go
git commit -m "feat(fleet/tui): the error pane, sharing the bottom with the log"
```

---

### Task 11: `cmd` — the warning badge, status summary, and per-attempt reset

**Files:**

- Modify: `sdk/fleet/cmd/tui_view.go` (`updateCell` ~:300, `statusView` ~:395)
- Modify: `sdk/fleet/cmd/tui_model.go` (`startUpdate` ~:400)
- Test: `sdk/fleet/cmd/tui_stderr_test.go`

**Interfaces:**

- Consumes: `m.warns`.
- Produces: the `ok ⚠N` cell and the `⚠ N warning(s) on M host(s)` status bit.

- [ ] **Step 1: Write the failing tests**

```go
func TestOkWithWarningsBadge(t *testing.T) {
	m := settledTestModel(2)
	m.vp = viewport{width: 120, height: 40}
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.appendLogLine("h1", "WARNING: grouped install failed", true)
	m.appendLogLine("h2", "Receiving objects:  73% (30/41)", true) // benign only
	m.updating["h1"] = updState{phase: updOK}
	m.updating["h2"] = updState{phase: updOK}

	if got := m.updateCell("h1"); !strings.Contains(got, "ok") || !strings.Contains(got, "2") {
		t.Fatalf("an exit-0 run that wrote stderr must show ok + the count, got %q", got)
	}
	if got := m.updateCell("h2"); strings.Contains(got, "⚠") {
		t.Fatalf("benign-only stderr must not raise a warning, got %q", got)
	}
}

func TestFailRowIsNeverAWarning(t *testing.T) {
	m := settledTestModel(1)
	m.vp = viewport{width: 120, height: 40}
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.updating["h1"] = updState{phase: updFail, log: "install.sh exited 1"}
	got := m.updateCell("h1")
	if !strings.Contains(got, "FAIL") {
		t.Fatalf("a failure still reads FAIL, got %q", got)
	}
	if strings.Contains(got, "ok") {
		t.Fatalf("a failed row must never read ok, got %q", got)
	}
}

func TestStatusBarWarningSummary(t *testing.T) {
	m := settledTestModel(2)
	m.vp = viewport{width: 120, height: 40}
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.appendLogLine("h2", "WARNING: grouped install failed", true)
	if got := m.statusView(); !strings.Contains(got, "⚠") || !strings.Contains(got, "2") {
		t.Fatalf("status bar must summarise warnings, got %q", got)
	}
}

func TestNewAttemptClearsTheWarningBadge(t *testing.T) {
	m := settledTestModel(2)
	m.appendLogLine("h1", "WARNING: apt-get update failed", true)
	m.appendLogLine("h2", "WARNING: grouped install failed", true)
	m.startUpdate([]string{"h1"})
	if m.warns["h1"] != 0 {
		t.Fatalf("a new attempt starts with a clean badge, got %d", m.warns["h1"])
	}
	if m.warns["h2"] != 1 {
		t.Fatalf("another host's badge must be untouched, got %d", m.warns["h2"])
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd sdk/fleet && go test ./cmd/ -run 'Warning|FailRow' -v`
Expected: FAIL — `updateCell` returns a bare `ok`.

- [ ] **Step 3: Implement**

In `updateCell`, the `updOK` branch becomes:

```go
	case updOK:
		// A run can exit 0 having written warnings to stderr — a host that
		// half-installed and reported success is the worst outcome this tool
		// can produce, so the row says so without anyone opening a pane.
		if n := m.warns[alias]; n > 0 {
			return th.ok.Render("ok") + " " + th.warn.Render(fmt.Sprintf("⚠%d", n))
		}
		return th.ok.Render("ok")
```

Add `warn` (yellow, colour `3`) to the `theme`. In `statusView`, append a
`⚠ N warning(s) on M host(s)` bit when any exist. In `startUpdate`, `delete(m.warns, a)` for
each target that is actually started (inside the same loop that sets `updPrecheck`, so a
skipped busy host keeps its badge).

- [ ] **Step 4: Run to verify it passes**

Run: `cd sdk/fleet && go test ./cmd/ -v 2>&1 | tail -20` — PASS.

**Done-when:** all four tests pass and `TestDemoFrames` still renders inside the width.
**Evidence:** `evidence/warn-badge/task11-badge.txt`.

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/tui_view.go sdk/fleet/cmd/tui_model.go sdk/fleet/cmd/tui_stderr_test.go
git commit -m "feat(fleet/tui): flag a host that wrote stderr, even when it exited 0"
```

---

### Task 12: end-to-end model test through the real executor

**Files:**

- Test: `sdk/fleet/cmd/tui_stderr_test.go`

**Interfaces:** consumes every layer built so far; produces nothing new.

- [ ] **Step 1: Write the failing test**

```go
// The whole wire, in one test: a Fake host writes to stderr, the real
// Executor/Console/Background lane carries it, and the model ends with the
// line in BOTH panes and a warning on the row.
func TestStderrReachesBothPanesAndTheBadge(t *testing.T) {
	f := runner.Fake{
		Out:    map[string]string{"h1": "installing"},
		ErrOut: map[string]string{"h1": "WARNING: apt-get update failed"},
	}
	m := settledTestModel(1)
	m.run = f
	m.vp = viewport{width: 120, height: 40}
	m.errOpen = true

	cmd := beginStream("h1", updplan.Default(), answers{}, f, t.TempDir())
	msg := cmd() // streamStartedMsg
	st := msg.(streamStartedMsg).st

	// Drain the stream the way Update() does.
	for {
		lm := readLine("h1", st)()
		if _, done := lm.(logEOFMsg); done {
			break
		}
		l := lm.(logLineMsg)
		m.appendLogLine(l.alias, l.line, l.stderr)
	}

	// updplan.Default() has TWO steps (dotfiles.sync, dotfiles.install) and the
	// Fake replays Out/ErrOut for EACH of them, so assert the relationships,
	// not absolute counts — an absolute count here would be asserting the
	// default plan's step list, which is not what this test is about.
	errs := m.errEntries()
	if len(errs) == 0 {
		t.Fatal("the error pane must have the stderr line")
	}
	if m.warns["h1"] != len(errs) {
		t.Fatalf("every non-benign stderr line is a warning: warns=%d errEntries=%d",
			m.warns["h1"], len(errs))
	}
	for _, e := range errs {
		if !strings.Contains(e.line, "apt-get") {
			t.Fatalf("the error projection must hold only the stderr line, got %q", e.line)
		}
	}
	if len(m.logs) != 2*len(errs) {
		t.Fatalf("the log pane keeps BOTH streams (one stdout per stderr here), got %d for %d",
			len(m.logs), len(errs))
	}
	v := m.View()
	if !strings.Contains(v, "installing") || !strings.Contains(v, "apt-get") {
		t.Fatal("both lines must be on screen")
	}
}
```

- [ ] **Step 2: Run to verify it fails** (if any layer is mis-wired it fails here — that is the
  point of this task). Run: `cd sdk/fleet && go test ./cmd/ -run TestStderrReaches -v`

- [ ] **Step 3: Fix whatever it reveals** — no new production code should be needed; if it is,
  the gap belongs to the task that owned that layer, and the fix goes in with a note in
  `TRACKING.md`.

- [ ] **Step 4: Run to verify it passes.** Also run the whole suite with the race detector:
  `cd sdk/fleet && go test -race ./... 2>&1 | tail -20` — the split streams add two goroutines
  per run, so the race detector is a required gate here.

**Done-when:** `go test -race ./...` PASS.
**Evidence:** `evidence/stderr/task12-e2e-model.txt` (including the `-race` run).

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/tui_stderr_test.go
git commit -m "test(fleet/tui): stderr end to end — stream to both panes and the badge"
```

---

### Task 13: golden frames, usability gate, and docs

**Files:**

- Modify: `sdk/fleet/cmd/tui_demo_test.go`, `sdk/fleet/cmd/tui_panes_test.go`
- Modify: `sdk/fleet/AGENTS.md`, `sdk/fleet/README.md`
- Modify: `docs/mbo/designs/fleet-connect.md`, `docs/mbo/designs/sdk-tui.md`, `docs/mbo/index.md`

- [ ] **Step 1: Extend the demo + usability tests**

Add frames: `host-only`, `log-only`, `error-only`, `host+error`, `three-pane`,
`three-pane-narrow` (60×16), and `warning-badge` (a settled fleet with `ok ⚠2`). Each frame
keeps the existing `must` marker assertion, and the loop gains the width guard's missing twin —
using the SAME budget as `TestFrameFitsTheTerminal`
(`max(m.vp.height, m.chromeRows())`, see Task 8) so the two guards cannot disagree.
The key-declaration guards live in `tui_config_test.go`
(`TestConfigKeysAreDeclaredInKeyHelp`, `TestKeyHelpHasNoDuplicateBindings`) and
`tui_sticky_test.go` (`TestHelpListsTheNewKeys`) — **not** in `usability_test.go`. Leave those
alone and add the pane equivalent next to the pane tests instead (Task 9's file):

```go
func TestPaneKeysAreDeclaredInKeyHelp(t *testing.T) {
	rows := map[string]int{}
	hdr := map[string]bool{}
	for _, k := range keyHelp {
		rows[k.keys]++
		hdr[k.keys] = hdr[k.keys] || k.hdr
	}
	for _, key := range []string{"h", "e", "l"} {
		if rows[key] != 1 {
			// EXACTLY one: `e` already had a row for the confirm-mode meaning,
			// and the pane toggle shares it rather than adding a second.
			t.Errorf("keyHelp must have exactly one %q row, got %d", key, rows[key])
		}
		if !hdr[key] {
			t.Errorf("%q must be hdr:true or it ships undiscoverable", key)
		}
	}
}
```

`TestKeyHelpHasNoDuplicateBindings` (already in the suite) is what catches `h` or `e` colliding
with an existing binding — run it and do not modify it.

- [ ] **Step 2: Run**

Run: `cd sdk/fleet && go test ./cmd/ -run 'TestDemoFrames|KeyHelp|PaneKeys' -v` and
`FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames` (eyeball the colour output).
Expected: PASS; the demo output goes to `evidence/demo/task13-frames.txt`.

- [ ] **Step 3: Update the docs**

- `sdk/fleet/AGENTS.md`: the `fleet tui` command row gains `h` hide hosts / `e` errors; add an
  **invariant** line: *"stdout and stderr are streamed on separate pipes; the log pane keeps
  both, the error pane keeps stderr, and a non-benign stderr line puts a ⚠ on the row even when
  the run exits 0. Pinned by `TestStderrReachesBothPanesAndTheBadge`, `TestOkWithWarningsBadge`,
  `TestBenignStderrTable`."* Add a second: *"the frame never exceeds the terminal — pinned by
  `TestFrameFitsTheTerminal`."*
- `sdk/fleet/README.md`: key table + a short "three panes" section.
- `docs/mbo/designs/fleet-connect.md`: note in §4.2 that `ReservedKeys` must gain `h`, `e`
  (and `l`) — a provider action key must not shadow a pane toggle.
- `docs/mbo/designs/sdk-tui.md`: note that fleet rebinds `h` (it has no lateral axis) so the
  phase-3 port inherits the decision.
- `docs/mbo/index.md`: the `fleet-error-view` row, state `building` → `in-review`.

- [ ] **Step 4: Gates**

Run: `npx --yes markdownlint-cli2 "sdk/fleet/AGENTS.md" "sdk/fleet/README.md" "docs/mbo/**/fleet-error-view*.md"`,
then `make lint-go && (cd sdk/fleet && go test ./...)`.
Expected: Go gates green; markdown clean apart from `MD010` inside Go snippets (see Global
Constraints).

- [ ] **Step 5: Commit**

```bash
git add sdk/fleet/cmd/tui_demo_test.go sdk/fleet/cmd/tui_panes_test.go \
        sdk/fleet/AGENTS.md sdk/fleet/README.md \
        docs/mbo/designs/fleet-connect.md docs/mbo/designs/sdk-tui.md docs/mbo/index.md
git commit -m "docs(fleet): three panes, the stderr invariant, and the key table"
```

---

### Task 14: live gate (human-in-the-loop)

**Files:** `docs/mbo/plans/fleet-error-view/evidence/e2e/` (new capture), `TRACKING.md`.

Splitting the pipes is the one change a stub cannot fully prove: only a real `ssh` under a real
install shows the two pipes draining together over minutes, and only a real host produces real
stderr. **This task cannot be completed by an agent.**

- [ ] **Step 1: Build and install** — `sdk/fleet/build.sh` (installs to `~/opt/bin/fleet`).
- [ ] **Step 2: Run `fleet tui` against a live host** and update one host.
- [ ] **Step 3: Capture** (asciinema or screenshots): the three-pane split; `h` hiding the host
      list; `e` opening the error pane; a real stderr line in both panes; a row reading `ok ⚠N`
      or `FAIL`; the same run's capture file showing `!!` lines
      (`grep -c '^!! ' ~/.local/state/fleet/logs/<host>-*.log`).
- [ ] **Step 4: File anything the live run contradicts** as a TRACKING blocker — do not
      retro-fit the spec to match a surprise.
- [ ] **Step 5: Commit the evidence** to `evidence/e2e/` and tick the stop condition.

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| F1a | `TestHostPaneTogglesAndRestores` (Task 9) |
| F2a | `TestErrorPaneTogglesAndDoesNotStealConfirmEdit` (Task 9) |
| F2b, F20a | `TestPaneDefaults`, `TestPaneStateIsNotPersisted` (Task 9) |
| F3a | `TestHidingTheLastPaneIsRefused` (Task 9) |
| F4a, F7a | `TestLayout` (Task 7); `TestErrorPaneOpenButEmptyCollapses` (Task 10) |
| F5a | `TestHostPlusOneBottomPaneKeepsTheTopFifth` (Task 7) |
| F6a | `TestThreePaneSplitIsEven` (Task 7) |
| F6b | `TestTinyViewportStillFits` (Task 7) |
| F8a | `TestFrameFitsTheTerminal`, `TestFrameFitsWithZeroAndOneHost`, `TestEmptyFleetStillRendersTheOtherPanes` (Task 8); the demo height guard (Task 13) |
| F8b | `TestChromeOverBudgetGivesThePanesNothing` (Task 8) |
| F8c | `TestPanelBodyRowIsExactlyOneLine` (Task 8) |
| F9a | `TestRunSplitStreamCtxSeparatesTheStreams`, `TestSplitStreamDoesNotDeadlockUnderBackpressure`, `TestRunSplitStreamCtxKillsTheChildOnDeadline` (Task 1) |
| F9b | `TestBatchFallsBackWhenNotSplitCapable` (Task 3) |
| F9c | `TestRunStreamMatchesSplitStreamMerged` (Task 1); `TestFakeMergedStreamStillCarriesErrOut` (Task 2) |
| F10a | `TestNilErrLineRoutesStderrToLine` (Task 3, re-run after Task 4) |
| F11a | `TestCaptureMarksStderr` (Task 4) |
| F12a | `TestStderrLineIsTaggedAndCounted`, `TestAppendLogStaysStdout` (Task 6); `TestErrorPaneRendersOnlyStderr` (Task 10) |
| F13a | `TestBenignStderrTable` (Task 5) |
| F14a | `TestOkWithWarningsBadge`, `TestFailRowIsNeverAWarning` (Task 11) |
| F15a | `TestStatusBarWarningSummary` (Task 11) |
| F16a | `TestNewAttemptClearsTheWarningBadge` (Task 11) |
| F17a | `TestFocusCyclesVisiblePanesOnly` (Task 9) |
| F17b | `TestPerPaneSearchIsIndependent` (Task 9) |
| F18a | `TestPaneKeysAreDeclaredInKeyHelp` (Task 13, filed with the pane tests) + the existing `TestKeyHelpHasNoDuplicateBindings` |
| F19a | the existing `tui_logpane_test.go` suite, unmodified (regression gate, Tasks 6–10) |
| UC1–UC6 end to end | `TestStderrReachesBothPanesAndTheBadge` (Task 12) + the live gate (Task 14) |

## 6. Integration & rollout

- **Build/test discovery** is by directory: `scripts/test.sh` already picks up `sdk/fleet`; no
  CI change is needed. `sdk/fleet/build.sh` installs the binary — run it from the **main**
  checkout, never from a worktree, per the root `CLAUDE.md`.
- **No flags, no config, no migration.** Pane state is session-only; the on-disk capture gains
  `!!` marks and nothing else changes shape.
- **Manual acceptance checklist** (Task 14): `h` / `l` / `e` each toggle; one pane alone fills
  the viewport; three panes split with the host on top; refusing to hide the last pane says so;
  a real warning badge appears on a successful run; `grep '^!! '` finds the stderr in the
  capture file.

### 6.1 Build leaves / DAG — **deliberately not broken out**

The candidate leaves (runner / updexec / model / view) are a **false split** by the criteria in
`docs/mbo/AGENTS.md` § Build-breakout policy:

- Tasks 6, 8, 9, 10, 11 all edit `cmd/tui_model.go` and `cmd/tui_view.go` — `gss feature
  conflicts` would report overlap on every pair, which the policy says means "merge them or
  re-cut", not "rebase around it".
- The interfaces are not frozen ahead of the work: `Console.ErrLine`'s exact nil-fallback
  semantics only became clear once the capture tee was written (Task 4's `origErr`/`origOut`
  branch), which is precisely the "interface still in flux" signal.
- The whole change is ~14 small tasks in one Go module with a single test suite; the
  integration cost of four stacked PRs exceeds the serial cost.

**Decision: one `gss feature` worker, one PR, sequential execution.** If the objective later
grows a second consumer (e.g. surfacing warnings in `fleet update --json`), that is a separate
objective with its own row.

## 7. Validation & evidence (show the work)

**Coverage bars**, enforced by re-running `scripts/test.sh` before the final commit (baseline
measured on this branch 2026-09-06): module total ≥ 82 % (from 82.3 %), `cmd` ≥ 65 %
(from 63.5 %), `runner` ≥ 65 % (from 59.8 %), `updexec` ≥ 90 % (from 92.5 %).

**Adversarial scenarios** that must be exercised, not assumed:

| Scenario | Where |
| :-- | :-- |
| 4000-line burst across both streams with a slow reader (the deadlock the split can introduce) | Task 1 |
| A runner without the split capability | Task 3 |
| `ErrLine == nil` after the capture tee has wrapped it | Tasks 3 + 4 |
| A benign prefix with a real error appended (`remote: fatal: …`) | Task 5 |
| 60×16 terminal, all three panes, full buffers, every mode | Task 8 |
| Zero hosts and one host | Task 8 |
| Hiding the focused pane | Task 9 |
| A failed host that also wrote warnings | Task 11 |
| `go test -race ./...` (two new goroutines per run) | Task 12 |

**Evidence protocol** — a tracked tree, committed with the task that produced it:

```text
docs/mbo/plans/fleet-error-view/evidence/
├── layout/      task07-layout-unit.txt · task08-before-overflow.txt · task08-after-fits.txt
│                task09-keys.txt · task10-errview.txt
├── stderr/      task01-runner-split.txt · task02-fake-errout.txt · task03-errline.txt
│                task04-capture-mark.txt · task06-model-tag.txt · task12-e2e-model.txt
├── warn-badge/  task05-benign.txt · task11-badge.txt
├── demo/        task13-frames.txt
└── e2e/         the human-gated live capture (Task 14)
```

Every file starts with a dated header and the exact command; append-only. **A feature without
captured evidence is not done** — and `task08-before-overflow.txt` is the one that proves the
overflow bug was real before it was fixed.
