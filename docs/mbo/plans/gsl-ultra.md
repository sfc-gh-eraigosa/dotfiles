# gsl ultra — implementation plan

- **Slug:** gsl-ultra
- **Date:** 2026-07-11
- **Status:** In progress — 4 of 7 leaves built (see §6.2)
- **Relates to:** spec `../specs/gsl-ultra.md` · design `../designs/gsl-ultra.md` · issue [#158](https://github.com/sfc-gh-eraigosa/dotfiles/issues/158) · absorbs #31

## 1. Summary & verdict

Six workstreams that make `gsl` correct, fast, legible, and host-agnostic, then give it a bubbletea
config studio. Grounded in an 8-dimension audit (**70 findings → 39 confirmed** after adversarial
verification), with the four headline defects re-verified by hand on the live binary:

| Verified on 2026-07-11 | Evidence |
| :-- | :-- |
| MCP active count is **always 0** | gsl greps U+2713 `✓`; the CLI emits U+2714 `✔` (census: 6× U+2714, 5× U+2718, **0**× U+2713) |
| MCP probe is **always SIGKILLed** | `runnerTimeout` 500 ms vs measured **3.45 / 3.73 / 4.10 s** |
| Width is **always 80** | `$COLUMNS` is not exported (`env \| grep -c '^COLUMNS='` → `0`) → dead branch; stdout is a pipe → ioctl fails |
| Model name renders as **`context)`** | live: `🤖 context) 🧠 42%` |

**Must-fixes, all addressed:** the MCP fixtures that encoded the bug are **deleted** and replaced
with a verbatim byte capture (§4 P2.1); the width invariant becomes a **property test**, not an
example (§4 P1.1); `-race` and the `check-deps.sh` seam gate enter CI (§6).

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| **WS1 — width & fit** | | |
| `sdk/gsl/cmd/statusline.go` | replace the ad-hoc cols block with `resolveColumns()`; call `StdoutWidthSource()` once | F1 |
| `sdk/gsl/cmd/columns.go` *(new)* | `resolveColumns(p, env, ttyStdout, ttyStderr) (int, string)` — pure, injected | F1 / E1,E2 |
| `sdk/gsl/cmd/columns_test.go` *(new)* | the four host-scenario table | E1, E2 |
| `sdk/gsl/internal/render/detect.go` | `Fit`: explicit **priority** drop order; final **truncate** pass | F2 / E3,E4,E6 |
| `sdk/gsl/internal/render/truncate.go` *(new)* | `truncateToWidth(blocks, st, cols)` — grapheme-safe, pure | F2 / E3 |
| `sdk/gsl/internal/render/seg_repo_data.go` | honour `level` (drop worktree count → `#157` → ellipsize) | F3 / E5 |
| `sdk/gsl/internal/render/seg_ai_data.go` | `shortenModelName`: alias table → first token → **rune-safe** | F4 / E7 |
| `sdk/gsl/internal/config/config.go` | `+ FallbackColumns int`, `+ Segment.Priority int` | F1, F2 |
| `sdk/gsl/internal/render/fit_property_test.go` *(new)* | width invariant + level monotonicity | **E3, E4, E5, E6** |
| **WS3 — latency & timeouts** | | |
| `sdk/gsl/internal/gh/pr.go` | `writeCache` on the **error** and **empty** paths; nil-guard the Runner | F13 / E19 |
| `sdk/gsl/internal/{git,gh,mcp}/exec.go` | `cmd.WaitDelay` + `SysProcAttr{Setpgid}` + process-group kill | F12 / E18 |
| `sdk/gsl/internal/exec/procgroup.go` *(new)* | shared `Setpgid`/kill helper (+ `_windows.go` no-op) | F12 |
| `sdk/gsl/cmd/statusline.go` | **delete** the serial `git.Status`; thread `*git.Info` into `Deps` | F12 |
| `sdk/gsl/internal/render/segment.go` | `Deps { + GitInfo *git.Info }` *(frozen iface — see §3)* | F12 |
| `sdk/gsl/internal/observe/logger.go` | demote expected non-zero exits WARN→Debug; `segment.panic`→Error | — |
| `sdk/gsl/internal/{git,gh,mcp}/exec_test.go` | grandchild-holds-pipe deadline test | **E18** |
| **WS2 — MCP** | | |
| `sdk/gsl/internal/mcp/status.go` *(new)* | `Status` struct; `ParseStatus` (keyword, sigil-agnostic) | F5 / **E8, E9** |
| `sdk/gsl/internal/mcp/cache.go` | cwd-keyed cache; `Status()` is **cache-only**; negative caching + backoff | F7 / E11, E12 |
| `sdk/gsl/internal/mcp/refresh.go` *(new)* | `Refresh()` — detached, single-flight (lockfile) | F7 |
| `sdk/gsl/internal/mcp/detect.go` | dedup by name; `local>project>user`; plugin caches; `disabledMcpjsonServers` | F6 / E10 |
| `sdk/gsl/cmd/mcp.go` *(new)* | `gsl mcp status` / `gsl mcp refresh` (the out-of-band verb) | F7 |
| `sdk/gsl/internal/render/seg_ai_data.go` | adaptive breakdown L0..L4; kill the `N/0` path | F8 / E13 |
| `sdk/gsl/internal/mcp/testdata/mcp-list-real.txt` *(new)* | **verbatim byte capture** — never retyped | **E8** |
| `sdk/gsl/internal/mcp/cache_test.go` | **DELETE** the hand-typed `✓`/`✗` fixtures (lines 51-55, 69-81) | **E8** |
| **WS6 — agy parity + hygiene** | | |
| `sdk/gsl/internal/payload/payload.go` | per-field tolerant decode (**#31**); quota as `map[string]QuotaWindow` | F14, F15 / E20, E21 |
| `sdk/gsl/internal/payload/testdata/agy_live.json` *(new)* | **captured** real agy payload | **E21** |
| `sdk/gsl/internal/payload/fuzz_test.go` *(new)* | `FuzzParse` | E20 |
| `sdk/gsl/internal/theme/settings.go` | read agy's top-level `colorScheme` (`ui.theme` = legacy fallback) | — |
| `sdk/gsl/internal/theme/resolve.go` | `auto` / `light-ansi` / `*-daltonized` via substring match | — |
| `opt/scripts/system/install_claude_skills.sh` | `ln -sf` → `install -m 0755` (repo copy-not-symlink rule) | — |
| `install.sh` | `\|\| exit 1` on the `build.sh` call so the seam gate can actually fail | — |
| `sdk/gsl/skill/SKILL.md` | rewrite for the post-#157 agy wiring | — |
| **WS4 — style/theme** | | |
| `sdk/gsl/internal/style/color.go` *(new)* | `resolveBlockColors` → **always** a concrete `(bg,fg)`; hex; WCAG | F9 / **E14, E15** |
| `sdk/gsl/internal/style/capability.go` *(new)* | `NO_COLOR` / `TERM=dumb` / `COLORTERM` ladder | F10 / E16 |
| `sdk/gsl/internal/style/palettes.go` *(new)* | catppuccin · tokyonight · nord · gruvbox · dracula | F9 |
| `sdk/gsl/internal/style/style.go` | new schema (`extends`, `separator{}`, `glyphs`, `palette`, roles) | F9 |
| `sdk/gsl/internal/render/glyphs.go` | `paint()` + `joinPowerline()` both consume `resolveBlockColors` | F9 / E14 |
| `sdk/gsl/internal/render/seg_repo.go` | `prBadge`: move the tint into the join layer (no embedded reset) | F11 / E17 |
| `sdk/gsl/cmd/config.go` | `gsl config validate`; validate `set style` against builtins ∪ user | — |
| `sdk/gsl/internal/style/contrast_test.go` *(new)* | WCAG ≥ 4.5:1 for every palette × role | **E15** |
| **WS5 — the TUI** | | |
| `sdk/gsl/internal/tui/**` *(new pkg)* | model · panels · keymap · width ruler · MCP panel · colour editor | F16 / E22 |
| `sdk/gsl/cmd/config.go` | **bare** `gsl config` → TUI; subcommands unchanged | F16 / **E23** |
| `sdk/gsl/internal/preview/model.go` | delete the aliased `segmentEnabled` map; `cfg.Segments` is the truth | E22 |
| `sdk/gsl/go.mod` | promote `lipgloss` + `bubbles` indirect → direct (**no new modules**) | — |
| **cross-cutting** | | |
| `.github/workflows/docker-image.yml` | add `-race`, standalone `go vet`, `GSL_STRICT_CHECK=1 check-deps.sh` | §6 |
| `sdk/gsl/cmd/*_test.go` | inject `Deps` — stop forking real `git`/`gh`/`claude` (15 s → < 2 s) | §6 |
| `sdk/gsl/README.md`, `sdk/gsl/AGENTS.md` | document `gsl config` (TUI), `gsl mcp`, the new style schema | §6 |

## 3. Interface contracts

**Frozen at the leaf boundaries** (everything downstream compiles against these; they land in the
`iface` leaf **first**):

```go
// internal/mcp — consumed by render + tui + cmd
type Status struct {
    Connected  int
    Failed     int
    NeedsAuth  int
    Configured int          // deduped denominator (F6)
    Servers    []ServerStatus
    FetchedAt  time.Time
    Stale      bool         // true when served from an expired entry
}
type ServerStatus struct {
    Name  string
    State State  // StateConnected | StateFailed | StateNeedsAuth | StateConnecting
    Scope Scope  // ScopeLocal | ScopeProject | ScopeUser | ScopePlugin
}

func ParseStatus(out []byte) Status                                    // PURE. keyword-based.
func Get(ctx context.Context, cwd string, o Options) (Status, error)   // CACHE-ONLY. never spawns. (E12)
func Refresh(ctx context.Context, r Runner, cwd string) error          // the out-of-band probe. single-flight.

// internal/style — consumed by render + tui
type Role string // "ok" | "warn" | "err" | "muted" | segment keys
func resolveBlockColors(st Style, role Role) (bg, fg RGB)              // ALWAYS concrete. contrast-safe. (E14,E15)

// internal/render — consumed by cmd + preview + tui
type Deps struct { /* …existing… */ GitInfo *git.Info }                // pre-threaded (WS3)
func truncateToWidth(blocks []segmentBlock, st style.Style, cols int) []segmentBlock  // grapheme-safe

// cmd — pure, injected
func resolveColumns(p payload.Payload, env func(string) string,
                    ttyStdout, ttyStderr func() (int, bool)) (cols int, source string)
```

**CLI contract**

```
gsl config                 → bubbletea studio (NEW; bare invocation only)
gsl config <sub> …         → unchanged CLI behaviour
gsl config validate        → NEW: lint config.json, non-zero on error
gsl mcp status [--json]    → NEW: print the cached Status
gsl mcp refresh [--detach] → NEW: the out-of-band probe (SWR writer)
gsl preview [--once]       → UNCHANGED, hermetic (fixtures + fixed clock; CI golden path)
gsl render / status        → unchanged contract; correct output
```

**Width precedence (F1):** `payload.terminal_width` → `ioctl(stdout)` → `ioctl(stderr)` →
`$COLUMNS` → `cfg.FallbackColumns` (120). Returns `source` for the log.

**MCP render ladder (F8):** L0 `6✓ 5✗ 1!` · L1 `6✓ 5✗` · L2 `6/12` · L3 `6` · L4 dropped.

## 4. TDD build order

Tests first, everywhere. Each phase's **done-when** is its merge gate.

**P0 — iface leaf (BLOCKING; everything stacks on this).**
Land the §3 type/signature stubs (compiling, `panic("todo")` bodies are *not* allowed — return
zero values) + `Deps.GitInfo`. No behaviour.
*done-when:* `go build ./...` green; downstream leaves can branch from it.

**P1 — WS1 width & fit.**
1. **Write `fit_property_test.go` FIRST** — E3 (width invariant, ~1150 cases), E5 (monotonicity),
   E6 (drop order), E4 (non-empty). **It must FAIL** on `repoData` (flat across levels) and on
   `Fit(20)` (returns 27–33 cols). *That red is the proof the test is real.*
2. `columns_test.go` (E1, E2) → then `columns.go`.
3. `truncate.go` + priority drop order → E3/E4/E6 green.
4. `seg_repo_data.go` real levels → E5 green. `shortenModelName` (E7).
*done-when:* the property suite is green; `gsl render` in a 200-col terminal emits a level-0 line;
`Fit(20)` ≤ 20; coverage ≥ 60%.

**P2 — WS2 MCP.**
1. **Capture** `claude mcp list` → `testdata/mcp-list-real.txt` (byte-for-byte; `hexdump` the
   codepoints into a comment). **Delete** `cache_test.go:51-55,69-81`.
2. `TestParseStatus_Golden` (E8) asserting `(connected=6, failed=5)` — **fails** against today's
   parser. Then `status.go`.
3. E12 (`Get` never spawns) + E11 (cwd-keyed) → `cache.go` + `refresh.go`.
4. E10 (dedup/precedence/disabled) → `detect.go`.
5. E13 (the L0..L4 ladder, never `N/0`) → `seg_ai_data.go`. `cmd/mcp.go`.
6. `//go:build integration` E9 (drift alarm).
*done-when:* `gsl mcp status` prints `6✓ 5✗` on this machine; a render spawns **0** MCP
subprocesses (verified with `strace`/`ps`); E8–E13 green.

**P3 — WS3 latency.**
1. E18 (grandchild holds the pipe) — **fails at 5121 ms** against an 800 ms deadline. Then
   `procgroup.go` + `WaitDelay` on all three seams.
2. E19 (`gh.PR` negative cache) — assert the 2nd call spawns **0**.
3. Delete the serial `git.Status`; thread `GitInfo`.
*done-when:* E18/E19 green; `BenchmarkRender` on `main` (no PR) shows the ~760 ms `gh` call gone;
git subprocess count 4 → 2.

**P4 — WS6 agy + hygiene.**
1. **Capture a real agy payload** → `testdata/agy_live.json` (**blocks E21** — do not invent it).
2. E20 per-field decode (#31) + `FuzzParse`. E21 dynamic quota map.
3. `colorScheme`; the two shell fixes; `SKILL.md`.
*done-when:* E20/E21 green; fuzz 60 s clean; a real agy session renders a non-empty line.

**P5 — WS4 style/theme.**
1. E14 (no unpainted block / no stray wedge) + E15 (WCAG ≥ 4.5:1) + E17 (no embedded reset) +
   E16 (`NO_COLOR`) — **all fail** today. Then `color.go`, `capability.go`, `palettes.go`.
2. Rewire `paint()`/`joinPowerline()`; fix `prBadge`.
3. Regenerate goldens behind `-update`; assert the default emoji golden is **byte-identical**.
*done-when:* E14–E17 green; `hexdump` shows no unpainted cells in the ribbon.

**P6 — WS5 the TUI.**
1. E22 (`Update` is value-semantic) — **fails** today on the aliased map. Fix by deleting the map.
2. E23 (bare `gsl config` → TUI; subcommands unchanged).
3. Panels; `[w]` save; golden `View()` at 40/80/200 cols (no line exceeds width).
*done-when:* E22/E23 green; `gsl config` opens, edits, saves, and the next render reflects it.

**P7 — prove it (the user's explicit ask).**
Real `gsl render` under **Claude Code** and **agy**, ≥2 widths, captured bytes before/after;
`-race` clean; full suite green; `cmd` suite < 2 s.

## 5. Verification mapping

| Spec rule | Test | Phase |
| :-- | :-- | :-- |
| E1, E2 | `cmd.TestResolveColumns` | P1 |
| E3, E4 | `render.TestFit_WidthInvariant`, `TestFit_NonEmpty` | P1 |
| E5 | `render.TestSegmentWidth_MonotonicInLevel` | P1 |
| E6 | `render.TestFit_DropOrderIndependentOfConfigOrder` | P1 |
| E7 | `render.TestShortenModelName` | P1 |
| E8 | `mcp.TestParseStatus_Golden` (verbatim capture) | P2 |
| E9 | `mcp.TestParseStatus_RealCLI` (`//go:build integration`) | P2 |
| E10 | `mcp.TestConfiguredCount_Dedup` | P2 |
| E11 | `mcp.TestStatus_CacheKeyedByCwd` | P2 |
| E12 | `mcp.TestStatus_NeverSpawns` | P2 |
| E13 | `render.TestMCPPart_Levels` | P2 |
| E18 | `{git,gh,mcp}.TestRun_ReturnsWithinDeadline_WhenGrandchildHoldsPipe` | P3 |
| E19 | `gh.TestPR_NegativeCacheOnError` | P3 |
| E20 | `payload.TestParse_PerField`, `payload.FuzzParse` | P4 |
| E21 | `payload.TestParse_AgyLive` | P4 |
| E14 | `render.TestJoinPowerline_NoUnpaintedBlock` | P5 |
| E15 | `style.TestContrast_AllBuiltinPalettes` | P5 |
| E16 | `style.TestColorMode_Ladder` | P5 |
| E17 | `render.TestSegments_NoEmbeddedReset` | P5 |
| E22 | `tui.TestModel_Update_IsValueSemantic` | P6 |
| E23 | `cmd.TestConfigCmd_Routing` | P6 |
| E24 | `cmd.TestDegradedPaths` | P3 |

## 6. Integration & rollout

- **CI** (`.github/workflows/docker-image.yml`): add `-race`; a standalone `go vet`; and
  `GSL_STRICT_CHECK=1 bash sdk/gsl/scripts/check-deps.sh` as its **own step** — today `install.sh`
  has no `set -e` and swallows the seam gate's failure.
- **Hermetic `cmd` suite:** inject `Deps` (today `statusline.go:78-80` hardcodes
  `NewSystemRunner()`), so tests stop forking real `git`/`gh`/`claude`. 15 s → < 2 s.
- **Docs:** `sdk/gsl/README.md` + `AGENTS.md` gain `gsl config` (TUI), `gsl mcp`, the style schema.
  `skill/SKILL.md` rewritten (it documents pre-#157 agy wiring that is now false).
- **Index:** move `gsl-ultra` through `planning → building → in-review → merged`; **fix the stale
  `gsl-visual-improvements` row** (says `in-review`; PR #55 is **merged**).
- **Manual acceptance:** §4 P7.

### 6.1 Build leaves / DAG (authoritative graph)

Edge `A → B` = "B consumes A's frozen §3 interface". Blocking leaves are built first
(Interface-First) and are the `--base` for their dependents.

| Leaf | Owns (paths) | Consumes (in-edges) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| `iface` | `internal/mcp/status.go` (types), `internal/style/color.go` (sig), `internal/render/segment.go` (`Deps.GitInfo`) | — | `go build ./...` green; §3 signatures frozen | **yes (base)** |
| `width` | `cmd/columns*.go`, `internal/render/{detect,truncate,seg_repo_data,seg_ai_data}.go`, `internal/render/fit_property_test.go`, `internal/config/config.go` | `iface` | E3,E4,E5,E6,E7,E1,E2 green; cov ≥ 60% | no |
| `latency` | `internal/{git,gh,mcp}/exec.go`, `internal/exec/procgroup*.go`, `internal/gh/pr.go`, `internal/observe/logger.go`, `cmd/statusline.go` (git.Status removal) | `iface` | E18,E19,E24 green; `gh` gone from the hot path | no |
| `mcp` | `internal/mcp/{status,cache,refresh,detect}.go`, `internal/mcp/testdata/**`, `cmd/mcp.go`, `internal/render/seg_ai_data.go` (mcpPart) | `iface`, `latency` (exec seam) | E8,E10,E11,E12,E13 green; render spawns 0 MCP procs | no |
| `agy` | `internal/payload/**`, `internal/theme/{settings,resolve}.go`, `opt/scripts/system/install_claude_skills.sh`, `install.sh`, `sdk/gsl/skill/SKILL.md` | `iface` | E20,E21 green; fuzz 60 s clean; real agy payload captured | no |
| `style` | `internal/style/{color,capability,palettes,style}.go`, `internal/render/{glyphs,seg_repo}.go`, `cmd/config.go` (validate) | `iface`, `width` (truncation is raw-text-aware) | E14,E15,E16,E17 green; emoji default golden byte-identical | no |
| `tui` | `internal/tui/**` *(new pkg)*, `cmd/config.go` (bare routing), `internal/preview/model.go` (map removal), `go.mod` | `iface`, `width`, `mcp`, `style` | E22,E23 green; save→render round-trip works | no |

```mermaid
graph LR
  iface --> width
  iface --> latency
  iface --> agy
  latency --> mcp
  width --> style
  width --> tui
  mcp --> tui
  style --> tui
```

**Path-disjointness check.** Three leaves touch `cmd/config.go` (`style` adds `validate`, `tui` adds
bare routing) and two touch `seg_ai_data.go` (`width` fixes `shortenModelName`, `mcp` rewrites
`mcpPart`) — **these are the only overlaps, and they are function-disjoint**. `tui` and `style` are
sequenced (`style` → `tui` via the `width` chain), and `mcp` bases on `latency`. Run
`gss feature conflicts --json` as a dry-run gate before fan-out; if it reports a *file* overlap that
is not function-disjoint, **merge the leaves** rather than rebasing around it.

**Landing order:** `iface` → (`width` ∥ `latency` ∥ `agy`) → `mcp` → `style` → `tui`.

### 6.2 Execution ledger (updated 2026-07-26; consolidated into [#196](https://github.com/sfc-gh-eraigosa/dotfiles/pull/196))

| Leaf | PR | State | Gate evidence |
| :-- | :-- | :-- | :-- |
| `iface` | [#196](https://github.com/sfc-gh-eraigosa/dotfiles/pull/196) (was #161) | in-review (consolidated) | `go build ./...` + `go vet ./...` green — P0 gate met |
| `width` | [#196](https://github.com/sfc-gh-eraigosa/dotfiles/pull/196) (was #165) | in-review (consolidated) | full `go test ./...` green incl. `fit_property_test.go` (E1–E7) |
| `latency` | [#196](https://github.com/sfc-gh-eraigosa/dotfiles/pull/196) (was #166) | in-review (consolidated) | full suite green incl. procgroup/gh-cache tests (E18, E19, E24) |
| `agy` | [#196](https://github.com/sfc-gh-eraigosa/dotfiles/pull/196) (was #167) | in-review (consolidated) | full suite green; `FuzzParse` 20 s smoke clean (E20, E21); reconciled with main's tmux-palette rule (#190) |
| `mcp` | — | **not started** | blocked on `latency` (exec seam) |
| `style` | — | **not started** | blocked on `width` (raw-text-aware truncation) |
| `tui` | — | **not started** | blocked on `width` + `mcp` + `style` |

Deviation log: main's [#190](https://github.com/sfc-gh-eraigosa/dotfiles/pull/190) added a
tmux/screen → `dark8` rule to `theme/resolve.go` after this plan was written; the `agy` leaf's
`keywordToPalette` bridge composes with it (host-tool settings still take precedence; the tmux rule
only affects the terminal-env fallback). No interface change required.

> Produced via `superpowers:writing-plans` from the audit synthesis. Execute with TDD throughout.
> Update `../index.md` state as it moves.
