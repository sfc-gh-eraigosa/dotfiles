# gsl — In-Tree Design Summary

This document describes the package layout, the key architectural seams, the concurrent render model, and the style system as they actually shipped in CP1–CP3. The full design rationale and the multi-agent review that shaped them live in `docs/mbo/plans/gsl-status-line.md` and `docs/mbo/plans/gsl-status-line-execution.md` (PR #21) — those files are in the repo's top-level `docs/mbo/plans/` directory, present on `main` and after this feature merges; they are not part of this impl worktree tree.

## Package layout

```
sdk/gsl/
├── main.go                   Entry point — calls cmd.Execute()
├── cmd/
│   ├── root.go               Root cobra command (gsl)
│   ├── render.go             gsl render — reads Claude JSON from stdin
│   ├── status.go             gsl status — no-payload render (Gemini/CLI)
│   ├── statusline.go         Shared wiring: load config, resolve style, build deps, render
│   ├── config.go             gsl config get|set|enable|disable|toggle|style
│   ├── preview.go            gsl preview [--once] — bubbletea TUI
│   └── version.go            gsl version [--json]
└── internal/
    ├── payload/              Defensive JSON parsing of the Claude stdin payload
    ├── config/               Config file schema, Load/Save, Default(); path via XDG_CONFIG_HOME
    ├── git/                  git.Runner interface + SystemRunner (os/exec seam) + git.Status/Worktree
    ├── gh/                   gh.Runner interface + SystemRunner (os/exec seam) + PR query
    ├── mcp/                  mcp.Runner interface + SystemRunner (os/exec seam) + configured/active counts + cache
    ├── repo/                 repo.Locate (git worktree detection), repo.PR (registry + gh fallback), registry parsing
    ├── style/                Style struct, two built-ins (powerline/emoji), Resolve/ResolveConfig, ASCII fallback
    ├── render/               Segment interface, BuildSegments, concurrent Render, four seg_*.go files, glyphs/ANSI
    ├── preview/              bubbletea TUI model, fixture payloads, RenderOnce (--once path)
    └── version/              Version/Commit/BuildDate/Dirty vars stamped by build.sh ldflags
```

## The os/exec seam

All subprocess calls are **confined** to three packages: `internal/git`, `internal/mcp`, and `internal/gh`. Each exposes a `Runner` interface whose `SystemRunner` implementation shells out via `os/exec`. No other package under `internal/` (and certainly not `render`) may import `"os/exec"` directly.

This is enforced at build time by `scripts/check-deps.sh`:

```sh
grep -rln '"os/exec"' --include='*.go' internal \
  | grep -v '_test\.go$' \
  | grep -v '^internal/git/' \
  | grep -v '^internal/mcp/' \
  | grep -v '^internal/gh/'
```

Any violation causes `build.sh` to exit non-zero. The `cmd/` package is the composition root and is intentionally exempt — it wires `SystemRunner` instances into `render.Deps`.

Tests replace runners with fakes in `internal/git/fake`, `internal/gh/fake`, and `internal/mcp/fake`. The fakes are script-based (`[]Response` slices) so tests make no subprocess calls and are fully deterministic.

## Concurrent render model and timeout budget

```
runStatusLine (cmd/)
└── render.Render(ctx, cfg, st, segs)
    ├── parent context: 1-second total deadline (set in cmd/statusline.go)
    └── per segment goroutine:
        ├── child context: segmentDeadline = 1000 ms hard cap
        ├── segment.Render(sctx, st) → (text, ok)
        │   ok == false → segment self-omits (no contribution to line)
        └── panic recovery: panicking segment drops silently
```

Segments are launched concurrently (`sync.WaitGroup`), each in its own goroutine under a child context with a 1-second deadline. Results are collected into a pre-allocated `[]result` by index to preserve config order. Detection packages apply tighter internal budgets (~800 ms for git, ~500 ms for MCP) before the outer hard cap fires.

A segment that times out, returns `ok=false`, or panics is **dropped** — the surviving segments still render and the line still prints. This guarantees the status bar never hangs Claude Code even if git or the MCP server is slow.

### Self-omit rules per segment

| Segment | Self-omits when |
|---------|----------------|
| `dirgit` | Working directory unavailable (extremely rare) |
| `repo` | Not inside a git repo, or git unavailable |
| `ai` | No Claude payload (all pointer fields nil — Gemini/CLI mode) |
| `time` | Never (time is always available) |

## Style system

A `Style` struct (`internal/style`) bundles four fields:

- `Separator` — `"powerline"` (filled chevron), `"thin"` (bar), `"space"`.
- `Fill` — whether colored background blocks are drawn.
- `Glyphs` — `"nerdfont"`, `"emoji"`, `"ascii"`.
- `Icons` / `Theme` — maps from logical key names to glyph strings and color values.

### Two built-ins

| Name | Separator | Fill | Glyphs |
|------|-----------|------|--------|
| `powerline` | powerline chevron | `true` | `nerdfont` |
| `emoji` | thin bar | `false` | `emoji` |

An `asciiIcons` fallback table is substituted into `Icons` whenever `Glyphs == "ascii"` or `forceASCII` is true.

### Font dependency (powerline = Nerd Font)

The `powerline` built-in emits Nerd Font private-use-area codepoints (`U+E0B0`, `U+F07B`, …); they render only on a terminal whose font is a patched **Nerd Font**. gsl emits the correct bytes regardless — a blank glyph is a *font-coverage* gap in the rendering terminal, not a gsl bug. The canonical font is **MesloLGS Nerd Font**, installed and wired into Windows Terminal by `sdk/gsl/scripts/install_nerd_font_windows.ps1` (kept in-tree so the font set stays in sync with the codepoints `powerline` uses; `cmd/glyphcheck` is the verifier — it asserts a font covers every emitted rune). Because rendering happens in the terminal emulator the user sits in front of, an SSH/WSL session needs the font on the **client**, not the remote host. The `emoji` built-in is the no-font fallback. See the README "Fonts and remote terminals" section for the user-facing version.

### Resolution (`ResolveConfig`)

`cmd/statusline.go` calls `style.ResolveConfig(os.Stderr, cfg.Style, rawUserStyles, false)`:

1. Look up the built-in named by `cfg.Style`. Unknown name → warn to stderr, fall back to `powerline`.
2. Deep-merge the same-named entry from `cfg.Styles` (if present) over the built-in:
   - Scalar fields (`Separator`, `Glyphs`) overwrite only when non-empty.
   - `Fill` is only applied when the raw JSON entry actually contained a `"fill"` key (fill-presence tracking). This prevents a partial override like `{"separator":"thin"}` from silently zeroing out `fill`.
   - `Icons` and `Theme` maps are merged key-by-key; user keys win, unspecified keys inherit from the built-in.
3. If `Glyphs == "ascii"` after merging, replace `Icons` with the ASCII fallback table (user icon overrides are still applied on top).

The resolved `Style` is passed into `render.BuildSegments` and then into every `Segment.Render` call. Glyphs are looked up by logical key at render time (`glyph(st, "branch")`); missing keys yield `""` — no crash.

## Auto theme colors (shipped in the visual-improvements phase)

Theme color resolution is handled by a detection-only package (`internal/theme`) that returns a **palette name** string; `internal/style` owns the actual palette definitions and the merge logic.

Resolution priority (see `internal/theme/resolve.go`):
1. Host-tool settings file — Claude `~/.claude/settings.json` `"theme"` enum, or Gemini `~/.gemini/settings.json` `"ui"."theme"` free-form string via keyword bridge (`"light"`, `"daltonism"`/`"colorblind"`, else dark).
2. Terminal env — `$COLORTERM`/`$TERM` selects `dark` (truecolor/256color) or `dark8` (8-color).
3. Hardcoded default: `dark`.

Palette names: `dark`, `light`, `dark-daltonism`, `dark8`. All five segment color keys (`repo_root`, `repo_worktree`, `ai`, `dirgit`, `time`) are defined in every palette so the `emoji` style (which applies colors as fg-tint only, `Fill: false`) can always tint every segment.

`style.ResolveConfig` merges the resolved palette into `Style.Theme` for keys the user did not explicitly set. User config always wins. See `internal/style/resolve.go` (`ResolveConfig`) and `internal/style/builtins.go` (`palettes`, `Palette`, `SegmentColorKeys`).

The settings reader (`internal/theme/settings.go`) applies hardened I/O: Lstat + file-type check, symlink resolve + out-of-home rejection, 256 KiB `io.LimitReader`. Any error degrades to `""` so Resolve falls through to the next level.

For the full design rationale, architecture-review findings, and emoji-coverage decisions, see `docs/mbo/designs/gsl-visual-improvements.md`.

## Dynamic width compaction (shipped in the visual-improvements phase)

Width detection runs once per render (`internal/term/width.go`):
- `$COLUMNS` → ioctl on stdout (`StdoutWidthSource`, returns `(0, false)` when not a TTY) → 80.

The Detect/Format/Fit split (`internal/render/detect.go`) keeps I/O to exactly one pass:
- `Detect` runs all subprocess work concurrently and returns a `[]segmentData` slice.
- `Format` is pure: it formats cached data at a given compaction level and emits the joined output.
- `Fit` calls `Format` at levels 0 through `finalCompactLevel` (currently 4), returning the first output whose `term.DisplayWidth` ≤ terminal columns, then progressively drops segments from the right if still too wide.

`DisplayWidth` uses `uniseg.StringWidth` after stripping ANSI SGR sequences — deterministic regardless of font or terminal.

## Separator bridges (powerline only)

The join layer (not individual segments) owns all ANSI painting. Segments return raw text plus a `colorKey`; the layer emits the background fill, bridges between adjacent segments (chevron with `fg = prev color`, `bg = next color`), and a trailing fade to the terminal background. The `emoji`/`thin` path is unchanged (no fill block, no bridge needed).

## Full design rationale

Full design rationale and the multi-agent review that shaped this implementation live in `docs/mbo/plans/gsl-status-line.md` and `docs/mbo/plans/gsl-status-line-execution.md` (PR #21) — both in the repo's top-level `docs/mbo/plans/` directory (present on `main` / after this feature merges).
