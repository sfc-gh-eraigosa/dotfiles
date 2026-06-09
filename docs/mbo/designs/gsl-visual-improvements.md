# gsl visual improvements — design

- **Slug:** gsl-visual-improvements
- **Date:** 2026-06-08
- **Status:** Proposed
- **Relates to:** issue #54 · PR #55 (supersedes the legacy `docs/designs/2026-05-29-gsl-visual-improvements-design.md`)
- **Author(s):** Edward Raigosa (re-planned via the MBO pipeline + an architecture-team review Workflow)

> **Re-plan note.** This supersedes the original PR #55 design, which was written
> before the `src/` → `sdk/` migration (so its paths and signatures had drifted) and
> before an architecture-team review surfaced three structural problems. The decisions
> in §4 below incorporate that review; the raw findings are in §7.

## 1. Problem / context

`gsl` renders a powerline status bar for Claude Code and Gemini CLI. Three gaps,
all verified against the current code under `sdk/gsl/`:

1. **Disconnected segments.** `separator()` (`sdk/gsl/internal/render/glyphs.go:144`)
   emits `" " + Icons["sep_right"] + " "` with **no ANSI color context**, and `join()`
   (`glyphs.go:165`) is a plain `strings.Join(parts, separator(st))`. The terminal
   background bleeds through the gap, so powerline segments look like disconnected
   islands instead of one continuous bar.
2. **Hardcoded colors.** Segment colors live in `powerlineStyle.Theme` / `emojiStyle.Theme`
   (`sdk/gsl/internal/style/builtins.go:77`,`:115`) regardless of the user's host-tool
   theme (Claude Code's `theme`, Gemini CLI's `ui.theme`) or terminal capability.
3. **No width awareness.** Nothing reads the terminal width; long segments (model name,
   full date, long branch) overflow narrow terminals with no graceful fallback.

**Style coverage.** `gsl` ships two built-in styles. Bridges (1) are powerline-only by
construction — `emoji` uses `Separator: "thin"` + `Fill: false`, so there is no
background block to bridge. Auto theme (2) and width compaction (3) apply to **both**.

## 2. Goals & non-goals

**Goals**
- Powerline segments connect wall-to-wall with no terminal-background gap; `emoji` unchanged.
- Segment colors derive from host-tool theme → terminal capability → hardcoded default,
  with the user's `~/.config/gsl/config.json` always winning.
- The bar fits any `$COLUMNS ≥ 20` under both styles, showing full detail when it fits.
- No regression to the per-render latency budget (segments must still be detected **once**).
- Stay within the `sdk/` module ≥60% coverage gate.

**Non-goals**
- No new style beyond `powerline` / `emoji`.
- No change to which segments exist or their data sources.
- No interactive/animated rendering (the bubbletea preview TUI is reused, not extended).
- Nerd Font installation — already shipped (PR #49, merged).

## 3. Options considered

### Compaction control flow (the pivotal choice)
- **A — Re-render per level (original PR #55).** `for level := 0..3 { render.Render(…, level) }`.
  *Rejected.* `render.Render` runs each segment's **uncached subprocess detection**
  (`git.Status`, `repo.Locate`, `repo.PR` via `gh`, `mcp.ActiveCount`); looping re-runs all
  that I/O up to 4× and multiplies the 1s context budget by the pass count. (Architecture
  review headline — see §7 SYS-1.)
- **B — Detect once, format per level (chosen).** Detection happens once (concurrently, as
  today); the result is a re-formattable intermediate; the fit loop only re-formats cached
  data at escalating compaction levels. Wall-clock and budget are unchanged from today.

### Where ANSI painting lives
- **A — Segments self-paint (today).** Each `seg_*.go` calls `paint()` internally and returns
  finished ANSI text with a trailing `ansiReset` (`glyphs.go:126`,`:133`). *Rejected for the
  bridge:* a color-aware chevron between two segments would have to splice color sequences
  around resets already baked into each segment's text — fragile. (§7 SYS-3/SYS-4.)
- **B — Layer owns painting (chosen).** Segments return **raw** text + a `colorKey`; the join
  layer owns *all* ANSI emission (segment fill + bridged chevrons). One source of color truth,
  bridge correct by construction, and it dovetails with detect-once/format-per-level (the
  layer formats the cached raw data at each level).

### Theme package boundary
- **A — New `internal/theme` produces a full palette map merged into `Style.Theme`.** *Rejected:*
  `internal/style` already owns palettes, the color-role key vocabulary, value formats, and the
  merge/override semantics (`builtins.go`, `resolve.go`); a second palette source splits one
  cohesive responsibility. (§7 SYS-5.)
- **B — New package does DETECTION only (chosen).** It resolves host-tool/terminal signals into
  a chosen **palette name** (or sparse override); `internal/style` keeps the palettes and the
  merge. Detection I/O is isolated and testable; color truth stays in one place.

## 4. Decision

Three features, layered on these structural decisions:

**4.1 Separator bridges (powerline only).**
Segments return raw text + `colorKey`; a color-aware join builds, for each interior boundary,
a chevron painted `bg = next segment's color`, `fg = prev segment's color`, then the glyph,
then `ansiReset`; a trailing chevron after the last segment fades `fg = last color` to the
terminal background. Fires only when `st.Separator == "powerline"` (and `st.Fill`); `thin`/
`space`/`emoji` fall through to today's space-padded glyph path unchanged.

**4.2 Auto theme colors.**
A new detection unit resolves, in priority order: (1) **host-tool settings** —
`~/.claude/settings.json` `theme` (enum) for Claude, `~/.gemini/settings.json` `ui.theme`
(free-form → keyword bridge: `light`→light, `daltonism`/`colorblind`→daltonism, else dark)
for Gemini; (2) **terminal** `$COLORTERM`/`$TERM` (truecolor/256 vs 8-color); (3) hardcoded
default (today's values). It returns a chosen palette; `style.ResolveConfig`
(`sdk/gsl/internal/style/resolve.go:16` — the real entrypoint, **not** `Resolve`) merges it into
`Style.Theme` for keys the user did not set. **User config always wins.** Both styles share the
same `Theme` keys; in `emoji` (`Fill:false`) the palette applies as foreground tint only.

**4.3 Dynamic width compaction.**
Detection runs once. The fit loop formats the cached data at levels 0→3 and returns the first
whose display width ≤ terminal columns (or level 3). Width is measured with grapheme-aware
counting (East-Asian-wide & ZWJ emoji = correct columns) after stripping ANSI. Per-segment
compaction tables for `ai` / `time` / `dirgit` (branch abbreviation). Terminal width comes from
an **injected** source (`$COLUMNS` → ioctl on stdout → 80) so the ioctl path stays testable.

**Security: harden the color sink first.** `colorCode()` (`glyphs.go:89`) currently passes any
value containing `;` verbatim into an ANSI SGR escape. Today its only source is the user's own
`config.json` (low risk), but 4.2 widens the sources (settings.json, env-derived palettes).
Before 4.2 lands, `colorCode` must validate input from non-config sources (numeric 256-index or
well-formed `38;2;r;g;b` truecolor only) and reject arbitrary `;`-bearing strings. (§7 SEC-4.)

**Boundaries touched (authoritative list — the original design under-counted these):**
`render.Render` has **two** callers — `cmd/statusline.go` *and* `internal/preview/model.go:193`
(the bubbletea preview). The `Segment` interface + join change ripple through all four
`seg_*.go`, `glyphs.go`, `render.go`, and the `golden_test.go`/`render_test.go` harnesses.
Preview should receive compaction too, for fidelity.

## 5. Risks & blast radius

| Risk | Mitigation |
| :-- | :-- |
| Interface change breaks a missed call site / implementer | §4 enumerates all of them; build + the full `go test ./...` gate it. |
| Re-rendering re-runs detection (latency/budget regression) | Detect-once/format-per-level (decision 3-B) — detection count unchanged. |
| Color-bridge corrupts output via stray resets | Painting moves to the layer (3-B); segments return raw text, no embedded resets. |
| Untrusted/huge/symlinked settings.json read every render | Lstat + reject non-regular, resolve-then-reject out-of-`$HOME` symlinks, `io.LimitReader` 256 KiB, degrade to defaults on **any** error. (§7 SEC-5.) |
| Escape-injection via widened color sources | Harden `colorCode` before 4.2 (decision above). |
| Golden-file churn hides a real diff | Powerline goldens regenerate behind `-update`; emoji goldens must stay byte-identical (asserted); review diffs with `cat -v`. |
| Coverage dips below the gsl=60 floor | New detection/format/width units are pure functions with injected I/O — table-driven tests; `scripts/test.sh` gate enforces it. |

**Blast radius:** confined to `sdk/gsl` (render, style, the new detection/width units, cmd,
preview). No other `sdk/` module, no shell/install surface. Default behavior is unchanged when
no host-tool settings exist and the terminal is wide.

## 6. Rollback

Each feature is an independent, separately-revertable commit (interface-shared parts of 4.1+4.3
land together — see plan §4). Reverting any one restores prior behavior; the auto-theme merge
and compaction are additive (full detail at a wide terminal == today's output). The legacy
design doc remains in git history via PR #55.

## 7. Architecture-team review (2026-06-08)

A `Workflow` ran sysarch / principal / secarch lenses over this design + the real `sdk/gsl`
code, each finding adversarially verified. **22 findings: 8 confirmed, 14 refuted.**

**Calibration caveat (important).** The adversary treated *unbuilt design behavior* as
"fabricated" — it refuted several findings solely because the proposed code "does not exist
yet." For a **design-stage** review that is a category error: the proposal is judged against
its own logic with current code as the baseline-to-change. The decisions above therefore
**re-include** valid concerns the adversary wrongly refuted (notably SYS-1 sysarch, the
compaction-loop cost — which the sysarch lens summary itself calls "an architectural mistake").

**Acted on:**
- *SYS-1 (sysarch)* compaction re-renders detection 4× → decision 3-B (detect once). **[reshaped]**
- *SYS-3 + SYS-4 (confirmed)* `join()`/`separator()` split + `paint()` resets → decision 3-B (layer owns paint). **[reshaped]**
- *SEC-4 (secarch)* `colorCode` `;` passthrough → harden before 4.2. **[reshaped]**
- *SYS-1 (principal, confirmed)* missed `preview` call site + ripple → §4 boundary list.
- *SYS-2 (principal)* ioctl untestable → inject the width source.
- *SYS-5 (sysarch, confirmed)* theme/style overlap → decision 2-B (detection-only package).
- *SEC-5 / SEC-7 (confirmed)* settings read hardening + `$HOME` path construction → §5 + spec §7.
- *SYS-3 (principal)* `uniseg` is **already transitive** (v0.4.7 via bubbletea→lipgloss→runewidth)
  → promote to a **direct** require; not a new dependency, zero supply-chain cost.
- *SYS-2 (sysarch)* entrypoint is `ResolveConfig`, not `Resolve` → corrected throughout.
- *SYS-7 (principal, nit)* `scripts/test.sh` comments stale (`src/`); gate still applies → plan §6.
- *SEC-6 (nit)* unbounded stdin `io.ReadAll` → optional 1 MiB `LimitReader`, low priority.

**Open question carried to the spec:** the canonical environment variable Gemini CLI sets when
launching a status-line command (for `toolCtx == "gemini"`) is unconfirmed; fallback is `""`.

> Produced via an architecture-team `Workflow`. Registered in `../index.md`.
> Matching spec: `../specs/gsl-visual-improvements.md`.
