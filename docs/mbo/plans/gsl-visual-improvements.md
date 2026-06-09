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

## 2. File inventory

| Path (under `sdk/gsl/`) | Purpose | Implements |
| :-- | :-- | :-- |
| `internal/render/segment.go` | `Segment.Render` → `(text, colorKey string, ok bool)`; doc the raw-text/self-omit contract | spec §3, F1 |
| `internal/render/render.go` | Detect-once → `[]segmentData`; `Render` gains `compactLevel`; both call sites | F1, F3 |
| `internal/render/glyphs.go` | Color-aware `join([]segmentBlock)`; bridged-chevron emission; harden `colorCode` | F1, F4 |
| `internal/render/seg_ai.go` | Return raw text + `"ai"`; compaction levels 1–3 | F1, F3 |
| `internal/render/seg_time.go` | Return raw text + `"time"`; compaction levels 1–3 | F1, F3 |
| `internal/render/seg_dirgit.go` | Return raw text + `"dirgit"`; branch abbreviation 1–3 | F1, F3 |
| `internal/render/seg_repo.go` | Return raw text + dynamic `"repo_root"`/`"repo_worktree"`; pass level through | F1 |
| `internal/render/testdata/golden_powerline_*.txt` | Regenerate (`-update`) for bridged sequences | F1 |
| `internal/theme/resolve.go` | **New.** `Resolve(toolCtx, env, home) → paletteName`; detection only | F2 |
| `internal/theme/resolve_test.go` | **New.** Priority-chain + gemini-keyword + settings-anomaly tests | F2, F5 |
| `internal/theme/settings.go` | **New.** Bounded/typed/symlink-safe settings read (Lstat, LimitReader 256 KiB, degrade) | F5 |
| `internal/style/builtins.go` | Add `light` / `dark-daltonism` / 8-color palettes | F2 |
| `internal/style/resolve.go` | `ResolveConfig` merges auto-theme (user keys win) | F2, F5 |
| `internal/term/width.go` | **New.** `Columns(source)` (injected: `$COLUMNS`→ioctl→80); grapheme `DisplayWidth` | F3 |
| `internal/term/width_test.go` | **New.** Injected-source + emoji/CJK width tests | F3 |
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
   move `paint` into `join`. Regenerate powerline goldens (`-update`); assert emoji byte-equal.
   *done-when:* `go test ./internal/render/...` green, emoji goldens unchanged.
2. **Detect-once / format split (F3 core).** Tests first: a counting fake runner asserting
   detection runs **once** across levels; per-segment level 1–3 tables. Then split `render` into
   `Detect`→`[]segmentData` and pure `Format(data,st,level)`; add `internal/term` (injected
   source + `DisplayWidth`) with its tests. Promote `uniseg` to direct.
   *done-when:* per-level tables green; detection-count test == 1.
3. **Sink hardening (F4).** Tests first: `colorCode` injection/validation vectors. Then add the
   `trusted` path. *done-when:* injection rejected, valid config values still render.
4. **Settings read + theme detection (F2/F5).** Tests first: `theme.Resolve` priority chain,
   gemini keyword bridge, and `settings.go` anomaly fixtures (fifo/socket/symlink/oversize/
   malformed → degrade). Then implement detection + palettes in `style` + the `ResolveConfig`
   merge. *done-when:* priority/keyword/anomaly tables green; user-override-wins test passes.
5. **Wire-up (F2/F3 integration).** Tests first: `cmd/statusline_test` narrow→fits, wide→level-0,
   detection-count==1; add `toolCtx` derivation. Then wire `cmd` + `preview`.
   *done-when:* integration tests green under both styles; `go test ./...` green; coverage ≥ 60%.

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| F1 interior chevron `bg=next,fg=prev`; trailing `fg=last` | `glyphs_test` bridge cases + powerline golden |
| F1 emoji unchanged | `golden_emoji_*` byte-equal assertion (both styles) |
| F1 single/zero segment edges | `glyphs_test` edge cases |
| F2 claude enum / gemini keyword / terminal fallback | `theme/resolve_test` priority tables |
| F2 user override wins; unknown gemini → dark | `style/resolve_test`, `theme/resolve_test` |
| F3 escalate-until-fit; level-0 when fits | `cmd/statusline_test` narrow/wide |
| F3 detection runs once | counting-fake test (render + cmd) |
| F3 emoji 2-col width | `term/width_test` emoji/CJK |
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
  colors (Claude + Gemini); `COLUMNS=60 gsl status` fits under `powerline` and `emoji`.

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
