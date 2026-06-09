# gsl visual improvements — spec

- **Slug:** gsl-visual-improvements
- **Date:** 2026-06-08
- **Status:** Draft
- **Relates to:** issue #54 · PR #55 · design `../designs/gsl-visual-improvements.md`

## 1. Goal

A `gsl` powerline bar that reads as one continuous, theme-aware ribbon: colored segments
connect wall-to-wall via color-transition chevrons; segment colors follow the user's host-tool
theme (Claude Code / Gemini CLI) with terminal and hardcoded fallbacks; and the bar compacts
gracefully to fit any terminal width instead of overflowing — all without re-running per-render
detection and without changing the `emoji` style's output.

## 2. Use cases

**UC-1 — Connected powerline.** *Actor:* any user on `style: powerline`. *Trigger:* status
renders. *Flow:* segments are detected, painted by the join layer, and joined with color-bridged
chevrons. *Acceptance:* between any two adjacent powerline segments there is no
terminal-background gap; the trailing chevron fades the last segment color to the background;
`emoji` output is byte-for-byte unchanged.

**UC-2 — Theme follows Claude Code.** *Actor:* Claude Code user. *Trigger:* render via the
`render` subcommand (Claude payload on stdin). *Flow:* `~/.claude/settings.json` `theme` →
palette → merged into `Style.Theme` for keys the user did not override. *Acceptance:* changing
`theme` between `dark`/`light`/`dark-daltonism` changes segment colors accordingly; `system`/
absent → dark palette.

**UC-3 — Theme follows Gemini CLI.** *Actor:* Gemini user. *Trigger:* render with Gemini context.
*Flow:* `~/.gemini/settings.json` `ui.theme` → keyword bridge → palette. *Acceptance:*
`"Ayu Light"`→light, `"Tokyo Night"`→dark, `"Dark Daltonism"`→daltonism, unknown→dark.

**UC-4 — Terminal fallback.** *Actor:* user with no host-tool theme set. *Trigger:* render.
*Flow:* no settings → `$COLORTERM`/`$TERM` → truecolor/256 palette vs 8-color named palette.
*Acceptance:* a 256-color terminal gets the dark ANSI-256 palette; an 8-color terminal gets
named colors so the terminal's own palette applies.

**UC-5 — User override wins.** *Actor:* user with a custom `~/.config/gsl/config.json` style.
*Trigger:* render. *Flow:* auto-theme merges only keys the user did not set. *Acceptance:* a
user-set `theme.ai` value survives regardless of host-tool/terminal signals.

**UC-6 — Narrow terminal.** *Actor:* any user. *Trigger:* render where the level-0 bar exceeds
`$COLUMNS`. *Flow:* detect once; format at levels 0→3; return the first that fits (or level 3).
*Acceptance:* for any `$COLUMNS ≥ 20`, displayed width ≤ `$COLUMNS` under both `powerline` and
`emoji`; when level 0 fits, level-0 output is returned (no compaction).

**UC-7 — Hostile/odd settings file.** *Actor:* attacker or accident writing `~/.claude/settings.json`.
*Trigger:* render. *Flow:* the read is bounded and validated; any anomaly degrades to the next
priority level. *Acceptance:* a non-regular file (fifo/socket/device), an out-of-`$HOME` symlink,
a >256 KiB file, or malformed JSON never breaks, hangs, or injects escapes into the bar — the
status line still renders with the fallback palette.

## 3. Architecture

Components (each independently testable; all under `sdk/gsl/`):

- **`internal/render` (modified)** — the integration layer. `Segment.Render` returns
  **raw text + colorKey + ok** (no embedded ANSI). A new re-formattable intermediate carries
  per-segment detected data so the fit loop can format at multiple levels without re-detecting.
  A color-aware `join` owns all painting: segment fill (via `style`/`paint`) **and** the
  color-bridged powerline chevrons.
- **`internal/theme` (new — detection only)** — `Resolve(toolCtx, env, home) → paletteName`
  (or sparse override). Reads host-tool settings + terminal env via injected accessors;
  contains **no** palettes and **no** merge logic.
- **`internal/style` (modified)** — keeps palettes (incl. new `light`/`dark-daltonism`/8-color
  sets) and the merge. `ResolveConfig` gains the auto-theme merge (user keys win). `colorCode`
  hardened against untrusted `;`-bearing input. **Every palette variant must define all five
  segment colorKeys (`repo_root`,`repo_worktree`,`ai`,`dirgit`,`time`)**: for `emoji` (`Fill:false`)
  `paint()` reads *only* the segment key — `accent`/`fg`/`bg` are inert — so a variant that omits a
  segment key (or only redefines `accent`/`fg`) leaves emoji un-themed. Making `accent` tint emoji
  would require a `paint()` change and is out of scope.
- **`internal/term` (new)** — `Columns(source) int` with an **injected** width source
  (`$COLUMNS` → ioctl(stdout) → 80) and grapheme-aware display-width measurement over
  ANSI-stripped text (via the now-direct `github.com/rivo/uniseg`).
- **`cmd` (modified)** — `statusline.go` derives `toolCtx`, wires `term.Columns`, runs the fit
  loop; `preview` (`internal/preview/model.go:193`) receives the same compaction for fidelity.

Data flow: `cmd` → detect-once `render` → `[]segmentData` → fit loop (`term` width + `style`
palette from `theme`) → color-aware `join` → string.

## 4. Behavior / features

- **F1 Bridges** — color-transition chevrons on powerline; trailing fade; emoji untouched.
- **F2 Auto-theme** — host-tool → terminal → default palette; user config wins.
- **F3 Compaction** — detect-once, format levels 0→3, first that fits; per-segment tables.
- **F4 Sink hardening** — `colorCode` validates non-config color values.
- **F5 Settings-read hardening** — bounded, symlink/type-checked, degrade-on-error.

**Style coverage (explicit — emoji is a first-class case, not an afterthought).**

| Feature | `powerline` (`Fill:true`, 1-col Nerd glyphs) | `emoji` (`Fill:false`, mixed 1/2-col glyphs) |
| :-- | :-- | :-- |
| F1 bridges | full color-bridged chevrons | **none — unchanged** (no bg block to bridge; thin `·` separator stays) |
| F2 theme | palette as bg blocks + fg | palette as **fg tint only**; needs all 5 segment keys + fg-legibility on light *and* dark |
| F3 compaction | applies | **binding case** — 2-col glyph floor means text-trim alone can't fit COLUMNS≈20; needs the glyph/segment-drop tier |
| F4 / F5 | applies | applies (style-independent) |

## 5. Evaluation criteria (per feature — every rule is a test)

**F1 Bridges** — *fires:* `Separator=="powerline"` & `Fill` → interior chevron carries
`bg=next,fg=prev`; trailing chevron `fg=last` + reset. *must-not-fire:* `emoji`/`thin`/`space`
→ output identical to today (golden byte-equal). *edge:* single surviving segment → trailing
fade only, no interior chevron; zero segments → empty string. *pass:* powerline golden updated &
deterministic; emoji golden unchanged.

**F2 Auto-theme** — *fires:* claude `theme` enum and gemini `ui.theme` keyword map to the
documented palette; terminal fallback when no settings; **for `emoji`, the resolved segment-key
colors apply as fg tint** (the rendered `38;5;N` codes change vs the default palette).
*must-not-fire:* a user-set `Theme` key is never overwritten — **verified for emoji via the
rendered fg code (the tint is emoji's only visible theme signal), not just the palette name**;
unknown gemini theme → dark (not terminal fallthrough). *edge:* missing/unreadable settings →
terminal → default; **emoji legibility — each palette's five segment-key indices are mid-luminance
and readable as bare fg on both a light and a dark terminal (emoji has no bg block for contrast,
and detection yields a palette *name*, not background luminance).** *pass:* table-driven tests per
priority level with temp settings files + injected env; render golden injects a fixed palette via
`Deps`; a palette-bounds unit test asserts no segment-key index sits within N steps of either
luminance extreme.

**F3 Compaction** — *fires:* level-0 width > cols → escalate until ≤ cols or the deepest level.
The deepest tier may **drop the per-segment leading glyph, then drop lowest-priority segments**
(e.g. `time`, then `repo` extras) — text-trimming alone cannot fit `emoji` at COLUMNS≈20 because
each emoji glyph is an irreducible 2 cols + space (a 4-segment bar's glyph+separator floor is ~19
cols before any text). *must-not-fire:* level-0 width ≤ cols → return level 0 (no compaction);
**detection runs exactly once regardless of level count** (assert via a counting fake runner).
*edge:* **`emoji` at `COLUMNS == 20` is the binding case** — `displayWidth ≤ 20` must hold;
mixed-width icons counted correctly (2-col: 📁🏠🌳🤖🔌⏰🌿🧠📦; **1-col: ⬆⬇✚✎✦⑂** — not all emoji
icons are 2 cols). *pass:* per-level table tests + an integration test asserting
`displayWidth(output) ≤ cols` (incl. the `emoji`/`COLUMNS=20` row) and a detection-call-count of 1.

**F4 Sink hardening** — *fires:* a non-config color value that is not a bare 256-index or a
well-formed `38;2;r;g;b` is rejected (no escape emitted). *must-not-fire:* valid user-config
values still render (back-compat). *edge:* `"0m\x1b[2J"`-style injection → rejected. *pass:*
`colorCode` unit tests incl. injection vectors.

**F5 Settings-read hardening** — *fires:* fifo/socket/device/out-of-`$HOME`-symlink/>256 KiB/
malformed → degrade to next priority. *must-not-fire:* a normal small settings file is read.
*edge:* file absent → no error. *pass:* unit tests with `t.TempDir` fixtures for each anomaly.

## 6. Verification harness

- **Unit (table-driven):** `theme` priority chain; gemini keyword bridge; palette-bounds
  (emoji fg-legibility); `colorCode` validation/injection; `term` width — **each real emoji icon
  pinned to its expected column count** (2-col 📁🏠🌳🤖🔌⏰🌿🧠📦, 1-col ⬆⬇✚✎✦⑂; source of truth
  = `emojiStyle.Icons`) plus CJK; per-segment compaction levels incl. the glyph/segment-drop tier;
  settings anomaly handling. **Width is the `uniseg.StringWidth` value (font/terminal-independent,
  deterministic); tests assert that value and that the fit loop consumes it — never the physical
  cells a specific terminal paints** (so a flaky test is never "fixed" by hardcoding a terminal width).
- **Golden:** `internal/render/testdata/golden_powerline_*.txt` regenerated behind `-update`
  for bridged sequences (reviewed with `cat -v`); `golden_emoji_*.txt` **byte-identical only under
  the DEFAULT palette** (F1 does not touch emoji). **F2 adds `golden_emoji_<palette>_*.txt` per
  non-default palette (light, dark-daltonism) asserting the segment fg-tint `38;5;N` codes change**
  — without this the default-only byte-equal assertion lets an emoji fg-tint regression ship silently.
- **Integration (`cmd`):** narrow `cols` → `displayWidth ≤ cols` under both styles **incl. the
  binding `emoji` / `COLUMNS=20` case**; wide `cols` → level-0 returned; **detection-call-count == 1**
  across the fit loop (counting fake).
- **Gate:** `go test ./...` green; module-wide coverage ≥ 60% (`scripts/test.sh` `COVERAGE_MIN[gsl]`).
- **Human-evidenced:** `gsl preview --once` shows wall-to-wall blocks; toggling Claude/Gemini
  theme changes colors; `COLUMNS=60 gsl status` fits under both styles. Capture as PR evidence.

## 7. Prerequisites / dependencies

- Nerd Font auto-install (PR #49) — **merged**.
- `github.com/rivo/uniseg` — **already a transitive dependency** (v0.4.7 via
  bubbletea→lipgloss→go-runewidth); promote to a **direct** `require` (MIT, on the allowed list).
- Path construction for settings reads: `filepath.Join(os.Getenv("HOME"), ".claude"/".gemini",
  "settings.json")` — worktree/CI-safe, no hardcoded home.
- **Open item:** confirm the env var Gemini CLI sets for a status-line invocation (drives
  `toolCtx=="gemini"`); fallback `""` (terminal/default path) until confirmed.

## 8. Out of scope (and why)

- New styles / new segments — orthogonal; keeps the change reviewable.
- Animated/interactive rendering — preview TUI reused as-is.
- Truecolor (24-bit) palette authoring beyond detection plumbing — can follow once detection lands.
- Retrofitting the `colorCode`/stdin hardening as a standalone security PR — folded in here
  because F2 is what widens the trust surface (SEC-6 stdin bound is optional, low priority).

## 9. Rollback

Per-feature independent commits (interface-shared 4.1+4.3 land together). Reverting any restores
prior behavior; additive features (auto-theme merge, compaction) reduce to today's output at a
wide terminal with no host-tool theme. Legacy design preserved in PR #55 history.

> Produced via the MBO pipeline + architecture review. Matching plan: `../plans/gsl-visual-improvements.md`.
> Register / update `../index.md`.
