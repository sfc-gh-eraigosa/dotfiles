# gsl ultra — spec

- **Slug:** gsl-ultra
- **Date:** 2026-07-11
- **Status:** Draft
- **Relates to:** design `../designs/gsl-ultra.md` · issue [#158](https://github.com/sfc-gh-eraigosa/dotfiles/issues/158) · absorbs #31

## 1. Goal

`gsl` tells the truth about the session, at the right width, legibly, without ever stalling a turn
— under **both** Claude Code and Antigravity (`agy`). Concretely: the MCP segment reports real
connected/failed/needs-auth counts (today it is unconditionally `0`); the line is exactly as wide
as the real terminal and never wraps (today it is always 80 columns); the model name is the model
name (today it renders as the literal word `context)`); no render makes a network call; and
`gsl config` opens a bubbletea studio where you can see, edit, and save all of it.

## 2. Use cases

**UC-1 — I want to know my MCP servers are healthy.**
*Actor:* a developer in Claude Code with 12 MCP servers configured.
*Trigger:* any assistant turn (Claude invokes `gsl render`).
*Flow:* gsl serves the cached MCP status instantly; if stale, it forks a detached refresh that
runs `claude mcp list` out of band and rewrites the cwd-keyed cache.
*Acceptance:* the segment shows `6✓ 5✗ 1!` at full width. The render itself blocks for **0 ms** on
MCP. Within ~60 s of a server going down, the `✗` count increments.

**UC-2 — I use a 200-column terminal.**
*Actor:* a developer on a wide monitor.
*Trigger:* any render.
*Flow:* `resolveColumns()` finds the real width — from `payload.terminal_width`, else `ioctl` on
stdout, else `ioctl` on **stderr** (which Claude typically leaves on the tty), else `$COLUMNS`,
else a configurable fallback.
*Acceptance:* the line uses the available width at compaction level 0 and shows full detail. It is
**never** wider than the terminal. The chosen width **and its source** are recoverable from the log.

**UC-3 — I use a 40-column split pane.**
*Actor:* the same developer, tmux-split.
*Trigger:* any render.
*Acceptance:* `DisplayWidth(line) ≤ 40`. The line does **not** wrap. Segments degrade through the
compaction ladder and — as a last resort — the final surviving segment is **truncated with an
ellipsis** rather than being emitted over-wide.

**UC-4 — I want to see my model.**
*Acceptance:* `"Claude Opus 4.8 (1M context)"` renders as `Opus 4.8` (not `context)`), and
shortens rune-safely under compaction.

**UC-5 — I run `agy`.**
*Actor:* a developer in Antigravity CLI.
*Acceptance:* quota buckets render whatever keys agy actually sends (validated against a **captured
real payload**, not invented keys); agy's `colorScheme` drives the palette; the line is not empty.

**UC-6 — I want to restyle my status line without editing JSON.**
*Trigger:* the user runs bare `gsl config`.
*Flow:* a bubbletea studio opens: live preview against the **real** repo/config, a width ruler, an
MCP health panel, a palette picker, segment reorder, per-role colour editor.
*Acceptance:* `[w]` writes `~/.config/gsl/config.json` and the next `gsl render` reflects it.
`gsl config <subcommand>` still behaves exactly as it does today.

**UC-7 — my `~/.claude.json` is corrupt / git is missing / the payload is malformed.**
*Acceptance:* gsl prints a **degraded but valid** line (or nothing) on stdout, never a panic or a
stack trace, and exits 0.

## 3. Architecture

Unchanged pipeline; the fixes land inside its existing seams.

```
stdin JSON ─► payload.Parse (per-field, tolerant)
                     │
cmd/statusline.go ───┼─► resolveColumns(p, env, ttyStdout, ttyStderr) ─► (cols, source)   [WS1]
                     │
                     ├─► render.Deps{ git.Info (pre-threaded, WS3), Git, GH, MCP runners }
                     │
              render.Detect(ctx)          ── ALL subprocess I/O, once, concurrent, deadlined
                     │                        · gh.PR      → negative-cached          [WS3]
                     │                        · mcp.Status → cache-only, never probes [WS2]
                     ▼
              render.Format(level)        ── PURE
                     │
              render.Fit(cols)            ── escalate levels, then drop, then TRUNCATE [WS1]
                     │
              join / joinPowerline        ── resolveBlockColors() owns ALL colour      [WS4]
                     ▼
                  stdout (one line)
```

**New / changed units** (each independently testable, I/O injected):

| Unit | Package | Contract |
| :-- | :-- | :-- |
| `resolveColumns` | `cmd` | `(payload, env, ttyStdout, ttyStderr) → (cols int, source string)` — pure, injected |
| `truncateToWidth` | `internal/render` | `(blocks, st, cols) → blocks` — grapheme-safe, pure |
| `mcp.Status` | `internal/mcp` | `(ctx, cwd, opts) → (Status{Connected,Failed,NeedsAuth,Configured}, error)` — **cache-only**, never spawns |
| `mcp.Refresh` | `internal/mcp` | `(ctx, runner, cwd) → error` — the out-of-band probe; single-flight |
| `mcp.ParseStatus` | `internal/mcp` | `([]byte) → Status` — **pure**, keyword-based, sigil-agnostic |
| `resolveBlockColors` | `internal/style` | `(st, roleKey) → (bg, fg RGB)` — **always concrete**, contrast-safe |
| `tui.Model` | `internal/tui` | the config studio (new package; `internal/preview` stays hermetic) |

**Frozen interfaces** (these are the leaf boundaries — see plan §6.1):
- `mcp.Status` struct + `mcp.ParseStatus` signature — consumed by the render layer and the TUI.
- `style.Role` (`ok|warn|err|muted` + the segment keys) and `resolveBlockColors` — consumed by the
  render layer, WS2's MCP tint, and the TUI's colour editor.
- `render.Deps` gains `GitInfo *git.Info` — consumed by `cmd` and `internal/preview`.

## 4. Behavior / features

**F1 — Width resolution.** Precedence `payload.terminal_width` → `ioctl(stdout)` → `ioctl(stderr)`
→ `$COLUMNS` → `cfg.FallbackColumns` (default **120**). Returns the source string; logged at Debug.

**F2 — Width invariant.** `term.DisplayWidth(Fit(datas, st, cols)) ≤ cols` for **all** inputs. Fit
escalates compaction levels 0..4, then drops segments right-to-left by **explicit priority** (not
slice position), then **truncates** the survivor.

**F3 — Real compaction levels.** Every `segmentData.format(st, level)` is **monotonically
non-increasing in width** as level rises. (Today `repoData` ignores `level` entirely.)

**F4 — Model name.** Alias table (Opus/Sonnet/Haiku/Gemini/GPT) → first meaningful token →
rune-safe prefix. Never the trailing parenthetical.

**F5 — MCP status.** `ParseStatus` splits each line on the **last** `" - "` and classifies the
remainder case-insensitively by keyword: `connected` / `failed` / `needs authentication` /
`connecting`. Sigils (U+2713/U+2714/U+2705/U+2718) are a **secondary** signal only.

**F6 — MCP configured count.** Dedup by server **name** across scopes with `local > project > user`
precedence; include plugin `.mcp.json` under `~/.claude/plugins/cache/**` gated by `enabledPlugins`;
**subtract** `disabledMcpjsonServers`; honour `enableAllProjectMcpServers`.

**F7 — MCP freshness (stale-while-revalidate).** A render **never** spawns a probe. It reads a
**cwd-keyed** cache (`sha256(cwd)[:16]`) and returns immediately, even if stale. On staleness it
forks a **detached, single-flight** refresh. Failures are **negatively cached** with exponential
backoff.

**F8 — MCP render shape.** Adaptive breakdown: L0 `6✓ 5✗ 1!` · L1 `6✓ 5✗` · L2 `6/12` · L3 `6` ·
L4 dropped. Roles: connected→`ok`, failed→`err`, needs-auth→`warn`. Never renders `N/0`.

**F9 — Colour engine.** `resolveBlockColors` parses `name | 0-255 | #rrggbb`, downsamples by
capability (`truecolor|256|16|never`), and when only a bg is given computes a **WCAG-contrast-safe**
fg. `paint()` and `joinPowerline()` both consume it — a block and its chevron can never disagree.

**F10 — Capability & accessibility.** `NO_COLOR` and `TERM=dumb` ⇒ no escapes at all. `COLORTERM`
selects the ladder rung. `--color` / `--glyphs` flags override.

**F11 — Segments emit no escapes.** No `segmentData.format()` output contains an ANSI sequence (the
`segment.go:12` invariant). The join layer owns all painting, including badges.

**F12 — Subprocess containment.** All three seams set `cmd.WaitDelay` and `Setpgid`, and kill the
**process group** on cancel. A grandchild holding the pipe cannot defeat a deadline.

**F13 — Negative caching.** `gh.PR` writes a cache entry on the **error** and **empty** paths, so a
branch with no PR does not re-invoke `gh` (a network call) every turn.

**F14 — Tolerant payload (#31).** A payload field with an unexpected type is dropped **individually**;
the rest of the payload survives. (Today one bad field discards everything.)

**F15 — agy quota.** `map[string]QuotaWindow` with suffix heuristics; renders whatever keys arrive.

**F16 — `gsl config` studio.** Bare invocation opens the TUI; subcommands unchanged; `[w]` saves.

## 5. Evaluation criteria (per feature)

Every rule is a test.

| # | Feature | Trigger predicate | Fires | Must **not** fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| E1 | F1 | payload has `terminal_width: 200` | `cols=200, source="payload"` | never returns 80 | payload width `0`/negative → fall through | `TestResolveColumns` |
| E2 | F1 | stdout piped, no payload width, stderr is a tty (80) | `cols=80, source="ioctl:stderr"` | must not skip stderr | both piped → `$COLUMNS` → fallback 120 | `TestResolveColumns` |
| E3 | F2 | any {fixture × style × cols 1..200} | `DisplayWidth(Fit()) ≤ cols` | never `> cols` | cols=1; CJK path; ZWJ emoji branch | `TestFit_WidthInvariant` |
| E4 | F2 | `Fit` must emit something | non-empty whenever any `segmentData ≠ nil` | must not return `""` with live data | all-but-one dropped | `TestFit_NonEmpty` |
| E5 | F3 | every `segmentData` type, levels 0..4 | width non-increasing | must not be flat across all levels | repo w/ worktree + PR | `TestSegmentWidth_MonotonicInLevel` |
| E6 | F2 | reverse `cfg.Segments` order | the same segment survives | drop order must not follow slice position | 2 segments | `TestFit_DropOrderIndependentOfConfigOrder` |
| E7 | F4 | `"Claude Opus 4.8 (1M context)"` | → `Opus 4.8` | never `context)`; never a byte-slice panic | multi-byte model name | `TestShortenModelName` |
| E8 | F5 | **verbatim byte capture** of real `claude mcp list` | `(connected=6, failed=5)` | must not depend on U+2713 | U+2714/U+2718; `Needs authentication` | `TestParseStatus_Golden` |
| E9 | F5 | the **real** CLI, `//go:build integration` | ≥1 status line parses | must not silently return all-zero | format drift on a new release | `TestParseStatus_RealCLI` |
| E10 | F6 | same server name in global **and** `.mcp.json` | configured = **1** | must not be 2 | name in `disabledMcpjsonServers` → not counted | `TestConfiguredCount_Dedup` |
| E11 | F7 | repoA(5) cached, then repoB(1) within TTL | repoB reads **1** | must not read 5 | missing cache dir | `TestStatus_CacheKeyedByCwd` |
| E12 | F7 | `Status()` called during a render | **0** subprocesses spawned | must never exec | cold cache → returns zero-value + `stale=true` | `TestStatus_NeverSpawns` |
| E13 | F8 | `(configured=12, connected=6, failed=5, auth=1)` | L0 `6✓ 5✗ 1!`; L2 `6/12` | never `6/0`; never `N/0` | all-zero → segment self-omits | `TestMCPPart_Levels` |
| E14 | F9 | theme value ∈ `{"default","#00ff00","nope",""}` | every block emits **both** bg and fg | no unpainted block; no stray trailing wedge | unknown role key | `TestJoinPowerline_NoUnpaintedBlock` |
| E15 | F9 | every builtin palette × every role | WCAG contrast(fg,bg) ≥ **4.5:1** | white-on-yellow (1.35:1) must fail | user-set fg always wins | `TestContrast_AllBuiltinPalettes` |
| E16 | F10 | `NO_COLOR=1` / `TERM=dumb` | output contains **no** `\x1b` | must not emit escapes | `COLORTERM=truecolor` → 24-bit | `TestColorMode_Ladder` |
| E17 | F11 | every segment, every level | `format()` output has no `\x1b` | `prBadge` must not embed a reset | non-fill path | `TestSegments_NoEmbeddedReset` |
| E18 | F12 | child exits, grandchild holds stdout 5 s | `Run` returns within `timeout+WaitDelay+slack` | must not take 5121 ms on an 800 ms deadline | all 3 seams | `TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe` |
| E19 | F13 | `gh` exits 1 (no PR) | a cache entry **is** written; 2nd call spawns **0** | must not re-exec | timeout → shorter TTL | `TestPR_NegativeCacheOnError` |
| E20 | F14 | payload with one bad field type | the other fields survive | must not discard the whole payload | fuzz corpus | `TestParse_PerField` + `FuzzParse` |
| E21 | F15 | **captured real agy payload** | quota buckets render | must not depend on invented keys | unknown bucket key → still rendered | `TestParse_AgyLive` |
| E22 | F16 | `Update(msg)` on the TUI model | the **caller's** model is unchanged | map/slice aliasing must not leak | every msg type | `TestModel_Update_IsValueSemantic` |
| E23 | F16 | `gsl config` (bare) vs `gsl config set …` | bare → TUI; sub → CLI | subcommands must not regress | `--help` | `TestConfigCmd_Routing` |
| E24 | UC-7 | corrupt `~/.claude.json`, no git, bad payload, `$HOME` unset | exit **0**, no panic, no stack trace on stdout | must not print a Go panic | each independently | `TestDegradedPaths` |

## 6. Verification harness

**Automated layers**

1. **Property** — `internal/render/fit_property_test.go`. The width invariant (E3) swept over
   `{short, long-basename, long-branch, CJK path, emoji-in-branch} × {powerline, emoji, ascii} ×
   {worktree y/n} × cols 1..200` (~1150 cases, ~0.34 s). Plus monotonicity (E5) for every
   `segmentData` type — *this is the test that catches `repoData` ignoring the level*.
2. **Captured goldens, never retyped** — `internal/mcp/testdata/mcp-list-real.txt` is a **verbatim
   byte capture** of `claude mcp list`, checked in with a comment pinning the codepoints. The
   hand-typed fixtures at `cache_test.go:51-55,69-81` are **deleted**. This is the root-cause fix
   for the class of bug that shipped: *the suite was green against a format the CLI never produced*.
3. **Integration (drift alarm)** — `//go:build integration`, shells the **real** CLI (E9). Not in
   the default gate; run in CI nightly and pre-release.
4. **Contrast + ANSI** — `TestJoinPowerline_NoUnpaintedBlock`, `TestContrast_AllBuiltinPalettes`,
   `TestSegments_NoEmbeddedReset`.
5. **Deadline** — `TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe` across all three seams.
6. **Fuzz** — `FuzzParse` on the payload (custom `UnmarshalJSON` for `ResetTime` is the risk).
7. **Bench** — `BenchmarkRender` end-to-end; a regression gate on the hot path.
8. **Race** — `go test -race ./...` added to CI (absent today).
9. **Hermetic `cmd`** — inject `Deps` so the suite stops forking real `git`/`gh`/`claude`
   (currently **10–15 s**). Target < 2 s.

**Human-evidenced gates** (the user's "prove it"):
- A real `gsl render` under **Claude Code** and under **agy**, at ≥2 terminal widths, with
  before/after screenshots or captured bytes.
- `hexdump` evidence that the powerline ribbon has no unpainted cells.

**Coverage:** the `sdk/` ≥ 60% floor holds per package; `internal/mcp`, `internal/render`,
`internal/style` target ≥ 80% (they hold the defect density).

## 7. Prerequisites / dependencies

- Go 1.26.3 (current). New direct deps: `lipgloss`, `bubbles` (both already **indirect** via
  bubbletea — promotion only, **zero** new supply-chain surface). `uniseg` already direct.
- `claude` CLI on `$PATH` for the integration test only (skipped when absent).
- A real `agy` session to capture `agy_live.json` (**blocks WS6's E21** — must be captured, not
  invented).

## 8. Out of scope (and why)

- **#34 token-usage history / #42 agy token tracking** — new *data*, not a fix to existing data;
  separate objective with its own storage design.
- **#33 tmux status bar** — a new *output target*; orthogonal.
- **#56 standalone plugin distribution** — a packaging concern; orthogonal.
- **New segments** — this objective makes the four existing segments correct. Adding a fifth while
  the four are broken is how we got here.
- **Native Go MCP health-dialling** — rejected in design §3.1-C; we would duplicate Claude's
  transport/auth and drift from its notion of "connected".

## 9. Rollback

Per-workstream, as in design §6: WS1/WS3/WS6 are bug fixes (revert → prior broken behaviour, no
migration); WS2's cache is versioned and degrades to a cold miss; WS4's schema is a strict superset
so existing `config.json` files keep working; WS5 adds a new entrypoint only. No forward-only data
migration exists anywhere in this objective.

> Produced from the audit `Workflow` synthesis. Matching plan: `../plans/gsl-ultra.md`.
> Registered in `../index.md`.
