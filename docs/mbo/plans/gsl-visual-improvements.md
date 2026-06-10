# gsl visual improvements — implementation plan

- **Slug:** gsl-visual-improvements
- **Date:** 2026-06-08
- **Status:** Draft
- **Relates to:** spec `../specs/gsl-visual-improvements.md` · issue #54 · PR #55

## 1. Summary & verdict

Build three `gsl` visual features — separator bridges, auto theme colors, dynamic width
compaction — on the structural decisions from the architecture review: **detect-once /
format-per-level** (no 4× re-render), **painting owned by the join layer** (segments return raw
text + colorKey), and a **detection-only** theme package (palettes stay in `internal/style`).
The color sink (`colorCode`) and settings reads are hardened before the trust surface widens.

**Review verdict:** 8 confirmed / 14 refuted, with the adversary mis-calibrated for a
design-stage review (it equated "unbuilt" with "fabricated"). All must-fix items are addressed:
detect-once (SYS-1 sysarch), layer-painting (SYS-3/4), preview call site (SYS-1 principal),
testable width seam (SYS-2 principal), theme boundary (SYS-5), sink + settings hardening
(SEC-4/5/7), and `uniseg`-is-transitive (principal SYS-3). See design §7.

**Emoji-coverage review (design §7.1):** 7 confirmed / 4 refuted — emoji is now a first-class
case, not an afterthought. Folded in: every palette variant defines all 5 segment keys with
mid-luminance fg legibility (emoji is fg-tint-only); a **final compaction tier** (glyph-drop →
segment-drop) since emoji's 2-col glyph floor makes text-trim alone unable to fit `COLUMNS≈20`;
emoji byte-equal goldens scoped to the **default palette** with **per-palette emoji goldens**
proving the fg-tint changes; per-icon width fixtures (not all emoji are 2-col); and the `uniseg`
measure pinned for determinism. See the spec "Style coverage" matrix.

## 2. File inventory

| Path (under `sdk/gsl/`) | Purpose | Implements |
| :-- | :-- | :-- |
| `internal/render/segment.go` | `Segment.Render` → `(text, colorKey string, ok bool)`; doc the raw-text/self-omit contract | spec §3, F1 |
| `internal/render/render.go` | Detect-once → `[]segmentData`; `Render` gains `compactLevel`; both call sites | F1, F3 |
| `internal/render/glyphs.go` | Color-aware `join([]segmentBlock)`; bridged-chevron emission; harden `colorCode` | F1, F4 |
| `internal/render/seg_ai.go` | Return raw text + `"ai"`; compaction levels 1–3; final-tier glyph drop | F1, F3 |
| `internal/render/seg_time.go` | Return raw text + `"time"`; compaction levels 1–3; final-tier glyph drop | F1, F3 |
| `internal/render/seg_dirgit.go` | Return raw text + `"dirgit"`; branch abbreviation 1–3; final-tier glyph drop | F1, F3 |
| `internal/render/seg_repo.go` | Return raw text + dynamic `"repo_root"`/`"repo_worktree"`; pass level through | F1 |
| `internal/render/render.go` (fit loop) | Final tier drops lowest-priority segments (`time`→`repo` extras) when glyph-drop still overflows | F3 |
| `internal/render/testdata/golden_powerline_*.txt` | Regenerate (`-update`) for bridged sequences | F1 |
| `internal/render/testdata/golden_emoji_*.txt` | Byte-equal **default palette only**; **add `golden_emoji_<palette>_*.txt`** (light, dark-daltonism) asserting fg-tint `38;5;N` change | F2 |
| `internal/theme/resolve.go` | **New.** `Resolve(toolCtx, env, home) → paletteName`; detection only | F2 |
| `internal/theme/resolve_test.go` | **New.** Priority-chain + gemini-keyword + settings-anomaly tests | F2, F5 |
| `internal/theme/settings.go` | **New.** Bounded/typed/symlink-safe settings read (Lstat, LimitReader 256 KiB, degrade) | F5 |
| `internal/style/builtins.go` | Add `light` / `dark-daltonism` / 8-color palettes — **each defines all 5 segment keys, mid-luminance for emoji fg-only legibility** | F2 |
| `internal/style/resolve.go` | `ResolveConfig` merges auto-theme (user keys win) | F2, F5 |
| `internal/term/width.go` | **New.** `Columns(source)` (injected: `$COLUMNS`→ioctl→80); grapheme `DisplayWidth` (the `uniseg` measure) | F3 |
| `internal/term/width_test.go` | **New.** Injected-source + **per-icon width fixtures** (2-col 📁🏠🌳🤖🔌⏰🌿🧠📦, 1-col ⬆⬇✚✎✦⑂, from `emojiStyle.Icons`) + CJK; assert the `uniseg` value, not terminal cells | F3 |
| `internal/style/builtins_test.go` | **New.** Palette-bounds test: every palette's 5 segment keys are mid-luminance (emoji fg-only legibility) | F2 |
| `cmd/statusline.go` | Derive `toolCtx`; wire `term.Columns`; run fit loop | F2, F3 |
| `internal/preview/model.go` | Apply compaction in `renderLine` (≈:193) for fidelity | F3 |
| `cmd/statusline_test.go` | Narrow→fits / wide→level-0 / **detection-count==1** integration tests | F3 |
| `go.mod` / `go.sum` | Promote `github.com/rivo/uniseg` to a **direct** require | spec §7 |

**Outside `sdk/gsl/`:** `scripts/test.sh` — no behavior change (gate already applies); fix stale
`src/`→`sdk/` comments opportunistically (SYS-7). No install/shell surface.

## 3. Interface contracts

```go
// internal/render — painting moves to the layer.
type Segment interface {
    // Render returns RAW (unpainted) text, the theme colorKey it should be
    // painted with, and ok (false ⇒ self-omit). compactLevel 0 = full detail.
    Render(ctx context.Context, st style.Style, compactLevel int) (text, colorKey string, ok bool)
}

type segmentBlock struct { text, colorKey string } // raw text + key; join paints both
func join(st style.Style, blocks []segmentBlock) string // owns fill + bridged chevrons

// internal/theme — DETECTION ONLY (no palettes, no merge).
func Resolve(toolCtx string, env func(string) string, home string) (paletteName string)

// internal/style — palettes + merge stay here.
func ResolveConfig(w io.Writer, name string, raw map[string]map[string]any,
    forceASCII bool, autoPalette string) Style // merges palette for keys user didn't set
func colorCode(value, layer string, trusted bool) string // untrusted ⇒ strict validation

// internal/term — injected width source keeps ioctl testable.
func Columns(source func() (int, bool)) int        // $COLUMNS → ioctl(stdout) → 80
func DisplayWidth(s string) int                    // ANSI-stripped, grapheme-aware (uniseg)
```

Fit loop (in `cmd` and `preview`), detection once:
```
data := render.Detect(ctx, cfg, deps)          // subprocess work happens HERE, once
cols := term.Columns(src)
for level := 0; level <= 3; level++ {
    line := render.Format(data, st, level)     // pure: no I/O
    if term.DisplayWidth(line) <= cols || level == 3 { return line }
}
```

## 4. TDD build order

1. **Interface + layer painting (F1 core).** Tests first: `segment_test` for the new
   `(text,colorKey,ok)` shape; `glyphs_test` for `join([]segmentBlock)` powerline bridge +
   trailing fade and emoji unchanged. Then update the four segments to return raw text + key and
   move `paint` into `join`. Regenerate powerline goldens (`-update`); assert emoji byte-equal
   **(default palette)**. *done-when:* `go test ./internal/render/...` green, emoji default goldens unchanged.
2. **Detect-once / format split (F3 core).** Tests first: a counting fake runner asserting
   detection runs **once** across levels; per-segment level 1–3 tables; the **final tier**
   (glyph-drop then segment-drop) incl. the **emoji `COLUMNS=20`** case; `internal/term` **per-icon
   width fixtures** (asserting the `uniseg` value, not terminal cells). Then split `render` into
   `Detect`→`[]segmentData` and pure `Format(data,st,level)`; add `internal/term`. Promote `uniseg` to direct.
   *done-when:* per-level + final-tier tables green; emoji/`COLUMNS=20` ≤ 20; detection-count test == 1.
3. **Sink hardening (F4).** Tests first: `colorCode` injection/validation vectors. Then add the
   `trusted` path. *done-when:* injection rejected, valid config values still render.
4. **Settings read + theme detection (F2/F5).** Tests first: `theme.Resolve` priority chain,
   gemini keyword bridge, `settings.go` anomaly fixtures (fifo/socket/symlink/oversize/malformed →
   degrade), the **palette-bounds** test (every palette's 5 segment keys mid-luminance for emoji),
   and **per-palette emoji goldens** (light, dark-daltonism) asserting the fg-tint `38;5;N` changes
   + the **emoji user-override-wins** rendered-fg case. Then implement detection + palettes in
   `style` (all 5 segment keys per variant) + the `ResolveConfig` merge.
   *done-when:* priority/keyword/anomaly/palette-bounds tables green; emoji per-palette + override
   goldens assert the expected fg codes.
5. **Wire-up (F2/F3 integration).** Tests first: `cmd/statusline_test` narrow→fits, wide→level-0,
   detection-count==1; add `toolCtx` derivation. Then wire `cmd` + `preview`.
   *done-when:* integration tests green under both styles; `go test ./...` green; coverage ≥ 60%.

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| F1 interior chevron `bg=next,fg=prev`; trailing `fg=last` | `glyphs_test` bridge cases + powerline golden |
| F1 emoji unchanged (**default palette only**) | `golden_emoji_*` byte-equal assertion |
| F1 single/zero segment edges | `glyphs_test` edge cases |
| F2 claude enum / gemini keyword / terminal fallback | `theme/resolve_test` priority tables |
| F2 user override wins; unknown gemini → dark | `style/resolve_test`, `theme/resolve_test` |
| F2 **emoji fg-tint changes per palette** | `golden_emoji_<palette>_*` (light, dark-daltonism) — `38;5;N` differs from default |
| F2 **emoji user-override wins (rendered fg)** | emoji golden/table asserts the user fg code, not the palette code |
| F2 **emoji palette legibility (mid-luminance)** | `style/builtins_test` palette-bounds |
| F3 escalate-until-fit; level-0 when fits | `cmd/statusline_test` narrow/wide |
| F3 detection runs once | counting-fake test (render + cmd) |
| F3 **emoji binding case at COLUMNS=20** | `cmd/statusline_test` emoji/`COLUMNS=20` → `displayWidth ≤ 20` |
| F3 **final tier drops glyph then segments** | `glyphs_test`/`render_test` deepest-level cases |
| F3 per-icon width (not all emoji 2-col) | `term/width_test` per-icon fixtures (assert `uniseg` value) |
| F4 injection rejected; valid config renders | `glyphs_test` `colorCode` vectors |
| F5 settings anomalies degrade | `theme/resolve_test` + `settings` fixtures |

## 6. Integration & rollout

- **Build/test discovery:** automatic — `scripts/test.sh` scans `sdk/`; the three new packages
  count toward `COVERAGE_MIN[gsl]=60`. Fix stale `src/` comments in `test.sh` (SYS-7, optional).
- **Dependency:** `go.mod` gains a direct `require github.com/rivo/uniseg`; `check-deps.sh`
  (allowed-license MIT) stays green.
- **Docs:** update `sdk/gsl/README.md` + `sdk/gsl/docs/` for the theme/width behavior; refresh
  `index.md` state.
- **Manual acceptance (PR evidence):** `gsl preview --once` wall-to-wall; theme toggle changes
  colors **under both styles — emoji visibly re-tints (fg)**; `COLUMNS=60` *and* the binding
  `COLUMNS=20` `gsl status` fit under `powerline` and `emoji`.

### 6.1 Build leaves / DAG (only if the build is parallelized — `mbo-plan` CAP-B)

Recommended **sequential, single-PR** build: the leaves share the `Segment` interface and the
render layer, so parallel workers would collide on the same files (a false split — design §4
boundary list). If parallelized despite that, the only clean cut is interface-first:

| Leaf | Owns (paths) | Consumes (in-edges) | done-when gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| iface+paint | `internal/render/segment.go`, `glyphs.go`, `render.go`, `seg_*.go`, goldens | — | `go test ./internal/render/...` green; emoji goldens byte-equal; ≥60% | yes (base) |
| theme | `internal/theme/**`, `internal/style/{builtins,resolve}.go` | iface (§3 `Segment`, `ResolveConfig`) | priority/keyword/anomaly tables green | no |
| width+wire | `internal/term/**`, `cmd/statusline*.go`, `internal/preview/model.go` | iface (§3 `Format`/`Columns`) | narrow/wide + detection-count==1 green | no |

Edge `iface → {theme, width+wire}`. `theme` and `width+wire` are parallel **only** after
`iface+paint` merges. Given the shared-file overlap, prefer one PR (default).

> Produced via `superpowers:writing-plans` + the architecture review. Execute TDD throughout;
> update `../index.md` state as it moves.
