# fleet TUI panes + stderr error view — design

- **Slug:** fleet-error-view
- **Date:** 2026-09-06
- **Status:** Draft
- **Relates to:** issue [#308](https://github.com/sfc-gh-eraigosa/dotfiles/issues/308) · design PR #TBD · spec [`../specs/fleet-error-view.md`](../specs/fleet-error-view.md) · plan [`../plans/fleet-error-view.md`](../plans/fleet-error-view.md)
- **Author(s):** Edward Raigosa (with Claude)

## 1. Problem / context

`fleet tui` renders two stacked panels: the host list (always on) and the streaming log
pane (`l` toggles it). Two gaps, both found by using the thing:

1. **The host view cannot be hidden.** `l` frees the whole viewport for the host list, but
   there is no inverse: when an operator is reading a long install log, the host table still
   eats the top fifth of the screen. `tui_model.go:listHeight()` hard-codes the split
   (`vp.height/5 - 2` once logs flow) with no "host pane off" state at all.

2. **stderr is invisible as stderr.** `internal/runner/runner.go:237` sets
   `c.Stdout, c.Stderr = pw, pw` — one `io.Pipe` for both, *deliberately*, so the log reads
   in the order the remote produced it. Consequences today:
   - A host can emit `WARNING: apt-get update failed; installs may be incomplete.` on stderr,
     exit **0**, and the row reads a clean `ok`. That is the same class of defect the
     `sudoGate` exists to prevent (a half-installed host reporting success), one layer up.
   - When a run *does* fail, the cause is somewhere in a 2000-line interleaved buffer; the
     row's FAIL text is only `tailFor(alias, 3)` — the last three lines, which for install.sh
     are usually epilogue, not the error.

   Verified: nothing in `sdk/fleet` distinguishes the two streams. `Console.Line` is
   `func(host, line string)`; `runner.RunStreamCtx` returns `<-chan string`. The information
   is destroyed at the pipe, before any consumer sees it.

3. **The frame already overflows the terminal.** Measured on this branch (2026-09-06, real
   `View()` through the real model, `lipgloss.Height`):

   | viewport | logs flowing | rendered | overflow |
   | :-- | :-- | :-- | :-- |
   | 100×40 | yes | 43 rows | **+3** |
   | 80×24 | no | 30 rows | **+6** |
   | 80×24 | yes | 31 rows | **+7** |
   | 60×16 | no | 28 rows | **+12** |

   Two independent causes, both measured directly (`lipgloss.Height` of the real renders):

   | Chrome piece | 60 cols | ≥80 cols |
   | :-- | :-- | :-- |
   | banner | **6** rows (the key-hint strip wraps) | 5 rows |
   | status, normal / search mode | 1 | 1 |
   | status, `modeConfirm` | **10** | 8 |
   | status, `modeAnswers` | **12** | 10 |

   So neither end of the chrome is a constant: the banner's height depends on the *width*, and
   the "status line" is a framed **panel** in the two dialog modes. `listHeight`/`logHeight`
   additionally under-count panel borders by 3 rows.

   And a third cause, which no height arithmetic can survive: **a body "row" is not always one
   line.** `th.panel` has `Padding(0, 1)`, so `panel.Width(w)` leaves `w-2` usable columns —
   measured: at `Width(76)` a 74-character line renders 3 rows, a 75-character line renders 4.
   But `View()` truncates each host row to `panelWidth()` (= `w`), two columns too many, and
   truncates the **column header not at all** (it is ~91 columns wide). At 80 columns every host
   row therefore wraps to two lines: the host panel renders **20 rows for 8 hosts**, which is
   the whole of the 80×24 overflow. `TestDemoFrames` never caught any of it because it asserts
   **width only**. Adding a third pane on top of arithmetic that is already
   wrong would compound this, so the layout is rewritten as a function that *measures* its chrome.

   **One consequence is a limitation, not a bug this objective fixes:** at 60×16 in
   `modeAnswers` the chrome alone is 6 + 1 + 12 = **19 rows** — taller than the terminal, before
   a single pane. No pane arithmetic can fix that; only making the dialogs themselves scroll
   could, which is out of scope (§2). The invariant is therefore "**the panes never add to an
   over-budget frame**" (§4.4 rule 7), and a dialog taller than a very short terminal is
   recorded as a known limitation with its own follow-up.

The ask: a third pane for stderr, below the log, sharing the bottom of the screen with it;
off by default; a per-host **warning** signal on the host row even when the command
succeeded; and a hide key for the host pane symmetric with `l`.

## 2. Goals & non-goals

### Goals

- G1 — `h` toggles the host pane; `e` toggles the error pane. Host on by default, log on by
  default (unchanged), error **off** by default.
- G2 — One pane visible ⇒ it owns the whole viewport. Host + one bottom pane ⇒ today's split.
  Host + log + error ⇒ host on top, log and error sharing the bottom region.
- G3 — stderr is captured **as stderr**, end to end: runner → executor → TUI model → view,
  and into the on-disk run capture.
- G4 — A host that wrote (non-benign) stderr is flagged on its row as a **warning** even when
  its update exits 0; a hard failure keeps reading FAIL, not warning.
- G5 — The log pane stays the complete transcript: stderr appears there too, marked.
- G6 — The layout is a pure function with a height invariant test — the frame must never
  exceed the terminal height for any pane combination.

### Non-goals

- Persisting pane visibility across sessions (session-only; the defaults are the contract).
- A configurable noise filter (the denylist is in-tree and small; see §4.5).
- Changing headless `fleet update`'s stdout/stderr behavior or its exit codes.
- Porting fleet onto `sdk/libs/tui` — that is `sdk-tui` phase 3 (§5).
- Per-pane resizing, mouse, or pane reordering.

## 3. Options considered

### 3.1 How to tell stderr from stdout on the wire

| Option | Trade-off |
| :-- | :-- |
| **A. Split the pipes locally (chosen)** | `RunStreamCtx` gains a sibling that gives `c.Stdout` and `c.Stderr` their own pipes and emits tagged lines. No remote-side change, no shell dependency, works on every POSIX host in the fleet. **Cost:** relative ordering *between* the two streams becomes arrival-ordered rather than exact (ordering *within* each stream is still exact). |
| B. Tag remotely, keep one pipe | Wrap each step so stderr is prefixed with a sentinel (`exec 2> >(sed …)`). Exact interleaving. **Rejected:** process substitution is bash-only and every host must have it — a direct violation of [`docs/mbo/specs/shell-portability.md`](./../specs/shell-portability.md); it also mangles a step's own redirections, and the sentinel can occur in real output. |
| C. Classify by text (match `ERROR` / `WARN`) | No interface change at all. **Rejected:** it is a guess, not stderr. It would miss a silent stderr line and false-positive on any stdout line containing the word "error" — the opposite of a trustworthy warning badge. |

Chosen: **A**, accepting arrival-order interleaving. This is exactly why G5 keeps stderr in
the log pane too: the merged transcript remains the place to read *sequence*, and each entry
is timestamped, so an operator can always reconstruct what happened around a failure.

### 3.2 How to thread the tag through `updexec`

| Option | Trade-off |
| :-- | :-- |
| **A. Optional capability + optional `ErrLine` callback (chosen)** | `runner.Runner` is untouched; `Console.Batch` type-asserts for a `SplitStreamer` and falls back to the merged path. `Console` gains `ErrLine func(host, line string)`; when it is nil, stderr routes to `Line` — i.e. **every existing caller and test behaves exactly as today**. Precedent: `interactiveCtxRunner` in `updexec/exec.go`, added for the same reason ("adding a method there would ripple into every other package's Runner test double"). |
| B. Change `RunStreamCtx` to return `<-chan runner.Line` | One path, no drift. **Rejected for v1:** six test doubles across four packages implement `RunStreamCtx` today; the churn buys nothing a fallback branch does not, and the fallback is 3 lines pinned by a test. |
| C. Two `Line` callbacks with a `stream` enum argument | Same shape as A with a wider blast radius on every existing `Line` closure. |

### 3.3 Where the error pane's lines live

| Option | Trade-off |
| :-- | :-- |
| **A. One tagged buffer, two projections (chosen)** | `logEntry` gains `stderr bool`; the log pane renders all of it, the error pane renders the `stderr` subset. One cap (`logCap`), one ring, no duplicate memory, and the two panes can never disagree about what arrived. |
| B. Two independent buffers | Simpler render, but two caps that evict at different rates: the error pane could show a line the log had already dropped (or vice versa), and "the same event, twice" is a bug factory. |

## 4. Decision

### 4.1 Layer 1 — `internal/runner`: split streams as an optional capability

```go
// Line is one line of remote output plus which stream produced it.
type Line struct {
    Text   string
    Stderr bool
}

// SplitStreamer is an OPTIONAL capability of a Runner: streaming with the two
// streams kept apart. Runner itself is unchanged.
type SplitStreamer interface {
    RunSplitStreamCtx(ctx context.Context, host, stdin string, argv ...string) (<-chan Line, <-chan error)
}
```

`Exec` implements it with two `io.Pipe`s and two scanners feeding one channel; `Fake` gains
`ErrOut map[string]string` so a test can script stderr. `Exec.RunStreamCtx` is **reimplemented
on top of** `RunSplitStreamCtx` (dropping the tag) so there is exactly one streaming
implementation and the merged path cannot drift from the split one.

*What it does:* streams a remote command's output line by line, tagged by stream.
*How it's used:* `updexec.Console.Batch` type-asserts for it. *Depends on:* `os/exec` only.

### 4.2 Layer 2 — `internal/updexec`: an optional `ErrLine`, and a marked capture

```go
type Console struct {
    R        runner.Runner
    Line     func(host, line string)   // unchanged
    ErrLine  func(host, line string)   // NEW; nil ⇒ stderr goes to Line (today's behavior)
    Stdin    func(st updplan.Step) string
    Preamble func(st updplan.Step) string
}
```

- `Console.Batch` prefers `SplitStreamer`; without it, every line is stdout — identical to today.
- The returned `out` string (a step's captured output, used for FAIL text) keeps **both**
  streams in arrival order, as today.
- `teeable.withLine` becomes `withLines(out, err func(host, line string))` so
  `Executor.RunHost` tees **both** streams into the per-host capture. Captured stderr lines
  are written with a stable `!!` prefix — the run log gains the distinction it never had,
  which is what makes a post-mortem of a headless `fleet update` possible.
- `Background` inherits all of it unchanged.

*Blast radius:* the CLI (`cmd/update.go`) sets only `Line`, so its terminal output stays
byte-identical. Only the capture file's stderr lines change (they gain the prefix).

### 4.3 Layer 3 — the TUI model: panes, one buffer, warning counts

```go
type pane int
const (paneHost pane = iota; paneLog; paneErr)

type logEntry struct {
    alias, line string
    at          time.Time
    stderr      bool   // NEW
    warn        bool   // NEW: stderr AND not benign (computed once, at append)
}
```

Model fields: `hostOpen bool` (default **true**), `errOpen bool` (default **false**),
`logOpen` unchanged (default true); per-pane `follow`/`top`/`search` state for the error pane
mirroring the log's; `focus pane` replacing `logFocus bool`; `warns map[string]int` per alias.

- `tab` cycles focus over the **visible** panes only.
- `h` / `e` toggle their pane. **Hiding the last visible pane is refused** with a status
  message ("at least one view must stay open") — there is no zero-pane state to get stuck in.
- Starting a new update for a host clears that host's `warns` entry, so the badge always
  describes the current attempt.

### 4.4 Layer 3 — the layout: one pure function

A new `cmd/tui_layout.go` replaces the magic numbers in `listHeight`/`logHeight`:

```go
type panes struct{ host, log, err bool }        // visibility
type heights struct{ host, log, err int }       // rows for each pane's BODY

// chromeRows is MEASURED by the caller (lipgloss.Height of the rendered banner
// plus the status rows) — never a constant: the banner's key-hint strip wraps at
// narrow widths, which is one of the two causes of today's overflow (§1.3).
func layout(vpHeight, chromeRows int, p panes, logActive, errActive bool) heights
```

Rules, in order:

0. Every panel truncates its content to the panel's *inner* width (`panelWidth() - 2`, the
   padding) — rows, the column header, and each stream line. Without this a "row" is not one
   line and no height arithmetic is meaningful (§1.3).
1. Chrome (banner + status area) is measured, and each open panel's fixed cost — border plus
   the one lead line every panel writes (the host's column header, a stream pane's title, or
   its collapsed hint) — is **3 rows**, from named constants rather than inline arithmetic.
2. A pane that is open but has nothing to show (`logActive`/`errActive` false) collapses to
   its one-line hint, exactly as the log pane does today, and reserves no body rows.
3. Exactly one active pane ⇒ it gets every remaining row.
4. Host + bottom panes ⇒ the host body is `vp/5` (the top fifth the dashboard shipped with),
   floored at 3 rows and clamped so the bottom region keeps its own floor; borders are already
   out of the pool by rule 1, so they are not subtracted a second time here. This deliberately
   does **not** reproduce today's numbers — today's overflow the terminal (§1.3).
5. Bottom region with both log and error active ⇒ split in half, the error pane taking the
   floor of the division (the log is the primary and keeps the odd row), each *aiming* for
   3 rows. If both floors do not fit, the **log keeps the region** and the error pane yields —
   one readable pane beats two unreadable ones, and the rule is deterministic (it does not
   depend on which pane has focus, so the frame does not resize as you press `tab`).
   **The floors are a preference, not a guarantee:** at 60×16 the chrome (8) plus three panel
   borders (6) plus the host's column header (1) already claim 15 of 16 rows, so panes yield
   rows until the frame fits rather than insisting on a floor that does not exist.
6. Host hidden ⇒ the bottom region is the whole remaining viewport, split by rule 5.
7. **Chrome over budget** (measured chrome ≥ the viewport — a dialog taller than the terminal)
   ⇒ every pane gets 0 rows. The panes must never *add* to a frame that already does not fit;
   what remains on screen is the banner and the dialog, which is the right thing to show when
   the operator is answering one.

`visibleRows()` returns `heights.host`, so every existing motion/paging calculation follows
the layout automatically — unchanged from today's design.

### 4.5 What counts as a warning

Every stderr line reaches the error pane. The **row badge** counts only lines that survive a
small in-tree denylist of known-benign stderr (`internal/updexec/stderrnoise.go`, pure,
table-driven):

- ssh: the `Warning: Permanently added …` known-hosts notice and `Pseudo-terminal will not be
  allocated …`
- git progress (git writes **all** of it to stderr): the `remote:` enumerate/count/compress
  counters, `Receiving objects:` / `Resolving deltas:` / `Unpacking objects:`, the `From <url>`
  header and the ref-update summary lines
- the `[sudo] password for …` prompt echo, and blank lines

Without this the badge is meaningless: a `sync` step alone makes every host write to stderr.

Each entry is an **anchored pattern matched against the whole line**, not a bare prefix. That
distinction is load-bearing: git reports its *failures* through the same `remote:` channel as
its progress, so a prefix test would have silently whitelisted
`remote: fatal: repository not found`. Listing the exact progress verbs keeps this a denylist of
known-good *shapes* rather than a guess about what an error looks like — the same reason §3.1
rejected classifying stderr by text. An unknown line is a warning; a false ⚠ costs a glance,
a missed one costs a half-installed host reporting success.

The filter is deliberately **not** configurable in v1 (§2 non-goals); it is one file with one
test table, and widening it is a one-line PR.

### 4.6 Layer 4 — the view

- `logView` and the new `errView` are one parameterized renderer (`streamPane`) over the
  filtered entry slice, differing in title, colour, and empty-state hint. stderr lines inside
  the **log** pane are marked with a dim red `!` gutter so the merged transcript still says
  which stream a line came from.
- `updateCell`: a host that finished OK with `warns[alias] > 0` renders `ok ⚠N` (yellow ⚠,
  green ok — the outcome colour never changes meaning). A FAIL row keeps reading FAIL.
- `statusView` gains `⚠ N` (total warning lines, hosts affected) when any exist.
- `keyHelp` gains `h`, `e` (both `hdr: true`) — it is the single source of truth, so the
  header strip, the `?` overlay, and the README table all follow from that one edit.

## 5. Risks & blast radius

| Risk | Severity | Mitigation |
| :-- | :-- | :-- |
| Splitting the pipes loses exact stdout/stderr interleaving | Medium | Accepted and documented (§3.1). Each entry is timestamped; stderr stays in the log pane; ordering within each stream is exact. Pinned by a test that scripts both streams and asserts per-stream order. |
| The new height invariant test exposes **existing** overflow | Medium | **Confirmed, not hypothetical** — §1.3 measures it (up to +12 rows at 60×16). Fixing it is in scope for the layout task; `layout()` therefore takes a *measured* `chromeRows` (the banner is rendered and its `lipgloss.Height` taken, because it wraps) rather than a compile-time constant. The invariant test is the gate. |
| `h` collides with the shared keymap | Medium | `sdk/libs/tui/GUIDE.md` §3 makes `h`/`l` lateral motions but explicitly allows a tool with no lateral axis to rebind them "and say so in its help". fleet has no lateral axis today and already rebinds `l`. Recorded here so the `sdk-tui` phase-3 port inherits the decision instead of rediscovering it. |
| `h`/`e` collide with `fleet-connect` drill-down provider actions | Medium | `fleet-connect` design §4.2 `ReservedKeys` must gain `h` and `e` (and `l`, already fleet's) or a plugin could claim a pane toggle. Cross-objective follow-up recorded in that design's plan; no code dependency either way. |
| Three panes make a small terminal unusable | Low | Rules 2 and 5 (collapse-when-empty, floors, focused pane wins) plus the height invariant across a width×height matrix including 80×24 and 60×16. |
| Capture format change (`!!` prefix) breaks a consumer | Low | The capture is read by humans; no parser in-tree. Pinned by an updexec capture test. |
| A noisy host permanently shows ⚠ | Low | §4.5 denylist; the count resets per update attempt. |

## 6. Rollback

Three independent revert points, in reverse dependency order: the view/model panes
(Layer 3–4) revert without touching `updexec`; `ErrLine` reverts to a merged `Line` without
touching `runner`; `SplitStreamer` reverts by deleting the optional interface — `RunStreamCtx`
keeps its signature throughout, so nothing outside these files ever sees the change. No
config, no on-disk state, no migration.

## 7. Evidence expectations

The plan must capture, under `plans/fleet-error-view/evidence/`:

- `layout/` — the height/width invariant matrix output, and rendered golden frames for each
  pane combination (host-only, log-only, err-only, host+log, host+err, all three, and each
  with the "open but empty" collapse).
- `stderr/` — a test transcript proving a scripted stderr line reaches the error pane, the
  log pane, and the capture file with its `!!` prefix, and never reaches the badge when it
  is on the denylist.
- `warn-badge/` — the `ok ⚠N` row for an exit-0 run that wrote stderr (the defect this
  objective exists to fix), and a FAIL row proving it still reads FAIL.
- `e2e/` — **human-gated:** one real `fleet tui` run against a live host, with a screenshot
  or asciinema capture showing `h`, `e`, the split, and a real warning badge. Splitting the
  pipes is the one change a fake runner cannot fully prove: only a real `ssh` proves the two
  pipes drain without deadlock under a long install.
- `demo/` — `FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames` output including the new frames.
