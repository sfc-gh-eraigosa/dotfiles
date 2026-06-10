# gsl — Go Status Line

`gsl` is a Go-based powerline-style status line with two integration points: it renders live context after every Claude Code assistant turn (receiving a JSON payload on stdin) and produces an on-demand status snapshot for Gemini/CLI (via `/gsl-status`). Four independently configurable segments — `dirgit`, `repo`, `ai`, and `time` — run concurrently and self-omit gracefully when their data is absent, so the line is always fast and never blocks.

## Install

### Build

`build.sh` compiles the binary and installs it to `~/opt/bin/gsl`:

```sh
cd sdk/gsl
bash build.sh
```

`build.sh` stamps version, commit, build date, and dirty flag via `-ldflags`, then runs `scripts/check-deps.sh` as a seam/license gate.

### Wire into Claude Code and Gemini

`./install.sh` (from the repo root) does three things for gsl:

1. Builds the `gsl` binary via its own `sdk/gsl/build.sh` block (same pattern as `gss`, `tmux-mgr`, `wol`).
2. Calls `sync-skills.sh` (no flags) to link `sdk/gsl/skill/` into `~/.claude/skills/gsl-status` and `~/.agents/skills/gsl-status`.
3. Calls `install_claude_skills.sh` to symlink `ai/claude/statusline-command.sh` to `~/.claude/statusline-command.sh` — the shim that Claude Code calls after every assistant turn.

You can also build the binary or refresh skill links independently:

```sh
bash sdk/gsl/build.sh        # compile and install gsl binary only
sync-skills.sh --build       # rebuild gsl + refresh skill links
make bin                     # same as bash sdk/gsl/build.sh (if using the Makefile)
```

The Gemini `/gsl-status` command is auto-discovered from `ai/gemini/commands/gsl-status.toml` — no extra step needed.

## Subcommands

### `gsl render`

Reads a Claude Code JSON payload from stdin, loads config, builds all enabled segments, and prints one rendered line to stdout. Empty or invalid stdin is handled gracefully (the line still renders without AI segment data):

```sh
# Claude Code calls this automatically via the shim; you can also test it manually:
echo '{"cwd":"/home/user/myproject","model":{"display_name":"claude-sonnet-4-5"},"context_window":{"used_percentage":42,"total_input_tokens":84000,"context_window_size":200000}}' | gsl render
```

### `gsl status`

Renders the status line without reading stdin. The `ai` segment self-omits (no Claude payload). Useful for Gemini CLI and shell scripts:

```sh
gsl status
```

### `gsl preview`

Interactive bubbletea TUI for exploring the status line without a live Claude session:

- `1` / `2` / `3` / `4` — toggle `dirgit` / `repo` / `ai` / `time`
- `s` — cycle through built-in styles
- `f` — cycle through fixture payloads (clean repo root, dirty worktree)
- `q` / `Ctrl+C` — quit

```sh
gsl preview              # interactive TUI
gsl preview --once       # print one rendered frame and exit (CI / golden-file safe)
```

### `gsl version`

```sh
gsl version              # human-readable: version, commit, dirty flag, build date, description, path
gsl version --json       # same fields as JSON
```

### `gsl config`

Manage `~/.config/gsl/config.json` (respects `$XDG_CONFIG_HOME`):

```sh
gsl config get                        # print the full config as indented JSON
gsl config get <key>                  # print one field
gsl config set <key> <value>          # set a field
gsl config enable [segment]           # enable the master switch (no arg) or a named segment
gsl config disable [segment]          # disable the master switch (no arg) or a named segment
gsl config toggle <segment>           # toggle a named segment on/off
gsl config style [name]               # show the current style, or set it to <name>
gsl config style --list               # list all builtin + user-defined styles (* = active)
```

Keys valid for `config get`: `enabled`, `style`, `timezone`, `time_format`, `date_format`, `segments`, `styles`

Keys valid for `config set`: `style`, `timezone`, `time_format`, `date_format`

Segments valid for enable / disable / toggle: `dirgit`, `repo`, `ai`, `time`

## Segments

All four segments run concurrently under a shared 1-second parent deadline. Each segment also has its own 1-second hard cap (`segmentDeadline = 1000 ms`); the underlying git / mcp / gh detection packages apply tighter internal budgets (~800 ms for git, ~500 ms for MCP). A segment that times out is dropped while the rest still render.

### `dirgit`

Always renders (directory is always available). Shows:

- Directory basename (`~` for `$HOME`, bare basename for paths under home or other locations)
- Git branch (when inside a git repo)
- Staged / unstaged / untracked counts as icon+number badges
- Stash count
- Ahead / behind remote counts

When the working directory is not inside a git repo, only the directory name is shown — the segment does not self-omit.

### `repo`

Renders when inside a git repo; self-omits outside one. Shows:

- A root or worktree indicator glyph (blue tint for main worktree, magenta tint for a linked worktree)
- An optional name label: the gss feature name (from the registry), the trailing branch segment (`worker` mode), or the raw branch name. Controlled by the `name` option in the segment's `options` block.
- An optional PR badge (`PR#<n>`) tinted by state: green for OPEN, magenta for MERGED, red for CLOSED. Controlled by `show_pr` option.
- An optional worktree count badge when 2 or more worktrees are present. Controlled by `show_count` option.

Segment options (set in the `options` map of the `repo` segment in `config.json`):

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `show_pr` | bool | `true` | Show the PR number badge |
| `show_count` | bool | `true` | Show the worktree count badge |
| `name` | string | `"feature"` | Name label mode: `feature` (gss registry name), `worker` (trailing branch segment), `branch` (raw branch), `off` (no label) |

### `ai`

**Payload-dependent — self-omits when no Claude payload is present** (i.e., in `gsl status` / Gemini mode). When a payload is present, shows:

- Model display name (e.g. `claude-sonnet-4-5`)
- Context-window usage: `<pct>% <used>k/<total>k` (token counts abbreviated with k/m suffixes)
- MCP server count: `<active>/<configured>` when servers are configured; active count uses a short-lived subprocess query with a cache
- 5-hour and 7-day rate-limit usage percentages (`5h 42%`, `7d 10%`)

### `time`

Always renders. Shows:

- Date formatted by `date_format` (default `2006-01-02` — Go reference layout)
- Time formatted by `time_format` (default `15:04:05` — 24-hour HH:MM:SS)
- Timezone abbreviation (e.g. `PST`, `UTC`)

Falls back to UTC silently on unknown or missing timezone — never panics.

## Configuration

Config file: `${XDG_CONFIG_HOME:-~/.config}/gsl/config.json`

A missing config file is not an error — `Default()` is used automatically. On first write, the directory is created.

### Schema and defaults

```json
{
  "enabled": true,
  "segments": [
    {"type": "dirgit", "enabled": true},
    {"type": "repo",   "enabled": true, "options": {"show_pr": true, "show_count": true, "name": "feature"}},
    {"type": "ai",     "enabled": true},
    {"type": "time",   "enabled": true}
  ],
  "timezone":    "America/Los_Angeles",
  "time_format": "15:04:05",
  "date_format": "2006-01-02",
  "style":       "powerline",
  "styles":      {}
}
```

Field reference:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Master on/off switch. When false, nothing is printed. |
| `segments` | array | (see above) | Ordered list of segments; config order is render order. |
| `segments[].type` | string | — | Segment kind: `dirgit`, `repo`, `ai`, `time`. |
| `segments[].enabled` | bool | `true` | Whether this segment is included in the render. |
| `segments[].options` | object | `{}` | Segment-specific overrides (see `repo` options above). |
| `timezone` | string | `"America/Los_Angeles"` | IANA timezone for the time segment. |
| `time_format` | string | `"15:04:05"` | Go layout string for the clock. |
| `date_format` | string | `"2006-01-02"` | Go layout string for the date. |
| `style` | string | `"powerline"` | Active style name. |
| `styles` | object | `{}` | User-defined style overrides (see Styles section). |

## Auto theme colors

Segment colors are resolved automatically from the host tool's settings, then the terminal, then a hardcoded default. The user's `~/.config/gsl/config.json` `theme` map always wins over auto-resolved colors.

### Resolution priority

1. **Host-tool settings file** (read once per render, degrading gracefully on any error):
   - **Claude Code** — `~/.claude/settings.json` field `"theme"` (enum). Values: `"dark"` → dark palette; `"light"` → light palette; `"dark-daltonism"` → daltonism palette; `"system"` or absent → dark palette.
   - **Gemini CLI** — `~/.gemini/settings.json` field `"ui.theme"` (free-form string, keyword bridge): contains `"light"` → light palette; contains `"daltonism"` or `"colorblind"` → daltonism palette; any other non-empty value → dark palette. If the file is missing or unreadable, falls through to terminal detection.

2. **Terminal environment** (only when no host-tool context is detected, or the settings file is absent/unreadable for Gemini):
   - `$COLORTERM == "truecolor"` or `"24bit"` → `dark` palette
   - `$TERM` contains `"256color"` → `dark` palette
   - Otherwise → `dark8` palette (8-color named-color palette; no 256-color escapes emitted)

3. **Hardcoded default**: the `dark` palette (same as the pre-existing built-in style theme values).

### Palette names

| Palette | When used | Notes |
|---------|-----------|-------|
| `dark` | Dark terminals, truecolor / 256-color | Named ANSI colors (system 8): green, blue, magenta, cyan, yellow |
| `light` | Light-background terminals | ANSI-256 indices, darker shades for fg readability |
| `dark-daltonism` | Red-green colorblind users | Blue/orange/teal ANSI-256; avoids red/green for segment identity |
| `dark8` | 8-color terminals | Named ANSI colors (same as `dark`); no 256-color escapes |

### What changes per palette

The five **segment color keys** change: `repo_root`, `repo_worktree`, `ai`, `dirgit`, `time`. All five are defined in every palette.

For the `emoji` style (`Fill: false`), the palette is applied as a **foreground tint only** — there is no background block, so `accent`, `fg`, and `bg` theme keys are inert for that style.

### Settings-file security hardening

The settings reader applies these checks before reading any file:
- Lstat + reject FIFOs, sockets, and device files before opening.
- Symlinks are resolved and the target must remain under `$HOME`; out-of-home symlinks are rejected.
- The file body is bounded to 256 KiB via `io.LimitReader`.
- Any error at any stage degrades to `""` so `Resolve` falls through to the next priority level. The status line always renders.

## Separator bridges (powerline style)

When `style == "powerline"` (the default), adjacent segments connect wall-to-wall via color-transition chevrons — no terminal background bleeds through the gap:

- Each interior boundary emits a chevron painted with `bg = next segment's color` and `fg = previous segment's color`.
- A trailing chevron after the last segment fades from the last segment's color to the terminal background.
- Painting is owned by the join layer, not by individual segments. Segments return raw text plus a `colorKey`; the layer emits all ANSI color sequences in one pass.

The `emoji` and `thin` separator styles are unchanged — they use a plain `|` bar with no fill block, so no bridge chevron is needed or emitted.

## Dynamic width compaction

The status bar fits the available terminal width automatically.

### Width detection

Width is resolved once per render in this order:

1. `$COLUMNS` environment variable (if set and a positive integer).
2. `ioctl TIOCGWINSZ` on stdout (via `charmbracelet/x/term`). Returns no result when stdout is not a TTY (e.g. piped output under Claude Code's status-line command), so this step is effectively a no-op in normal Claude Code usage.
3. Hard fallback: **80** columns.

### Fit loop

Detection (`Detect`) runs all subprocess I/O once, concurrently. The fit loop (`Fit`) is pure — it calls `Format` at escalating compaction levels using the cached detection data, with no further I/O:

| Level | What changes |
|-------|-------------|
| 0 | Full detail — all text, all glyphs |
| 1 | Per-segment text compaction (abbreviated model name, shorter branch, condensed time) |
| 2 | More aggressive text abbreviation |
| 3 | Deepest text abbreviation |
| 4 (final tier) | Leading glyph dropped from every segment, then lowest-priority segments dropped from the right (`time` → `ai` → `dirgit` → `repo`) until the output fits or one segment remains |

`Fit` returns the first level whose `DisplayWidth` (grapheme-cluster-aware, ANSI-stripped, `uniseg.StringWidth`) is ≤ terminal columns, or the most compact form if nothing fits.

The `emoji` style is the binding case: each emoji icon is an irreducible ~2 columns plus a space, so a four-segment bar has a large floor before any text. Text-compaction alone cannot reach `COLUMNS ≈ 20`; the final glyph-drop tier exists for this reason.

## Styles

### Built-in styles

| Name | Separator | Fill | Glyphs | Notes |
|------|-----------|------|--------|-------|
| `powerline` | powerline chevron | yes | nerdfont | Default. Requires a Nerd Font-patched terminal font. |
| `emoji` | thin bar (`\|`) | no | emoji | No font dependency; works in any terminal. |

Switch styles:

```sh
gsl config style emoji       # switch to emoji style
gsl config style powerline   # switch back
gsl config style --list      # show all styles with active marker (*)
```

### Glyph modes

The `glyphs` field of a style controls the icon repertoire:

- `nerdfont` — Nerd Font private-use-area codepoints; requires a patched font.
- `emoji` — Unicode emoji; no font dependency.
- `ascii` — plain printable ASCII fallback; also forced when the terminal reports no color support.

### Fonts and remote terminals

The `powerline` style (the default) draws its icons and separators with **Nerd Font private-use-area glyphs** (`U+E0B0` chevron, `U+F07B` folder, …). They render **only** if the terminal's font is a patched **Nerd Font**. If yours is not, those glyphs appear as blank gaps or missing-glyph boxes — the bytes are correct, the font just has no glyph for them.

- **Canonical font:** **MesloLGS Nerd Font** (v3.4.0). On Windows it is installed and wired into the Windows Terminal profiles automatically by `sdk/gsl/scripts/install_nerd_font_windows.ps1` (invoked by the repo's `install.sh` → `setup-apps.ps1`). On macOS/Linux, install any Nerd Font and select it in your terminal.
- **Installing is not enough — you must _select_ it.** Set the font in your terminal's profile (e.g. Windows Terminal → Settings → *profile* → Appearance → Font face → `MesloLGS Nerd Font`). The default Windows Terminal font (Cascadia Mono) lacks these glyphs.
- **SSH / WSL — the font must live on the _client_, not the remote host.** Glyph rendering is done by the terminal emulator you are sitting in front of. Installing a Nerd Font on a headless box you `ssh` into does nothing; the font must be installed and selected in the **local** terminal that draws the screen.
- **No-font fallback:** the `emoji` style needs no special font and works in any terminal — `gsl config style emoji` (or press `s` in `gsl preview`).
- **Verify a font** actually covers every glyph gsl emits: `go run ./cmd/glyphcheck /path/to/Font.ttf` (exit 0 = all 17 codepoints present).

### User style overrides

Add an entry to `styles` in `config.json` whose key matches a built-in name to deep-merge overrides onto that built-in. Only the fields you specify are changed; the rest inherit from the built-in.

**Fill-presence behavior**: if you omit `fill` from your override object, the built-in's `fill` value is preserved. This prevents a partial override like `{"separator":"thin"}` from accidentally disabling fill on the powerline style.

Example — override powerline to use thin separators without changing fill or glyphs:

```json
{
  "style": "powerline",
  "styles": {
    "powerline": {
      "separator": "thin",
      "icons": {
        "ai": "★"
      },
      "theme": {
        "dirgit": "cyan"
      }
    }
  }
}
```

A `styles` key that does not match any built-in name creates a brand-new user style, resolved by merging the user entry over the `powerline` base. Activate it with `gsl config style <name>`.

## Known limitations

- **Nerd Font must be on the rendering terminal**: the `powerline` style requires a Nerd Font installed *and selected* in the terminal that draws the screen — for SSH/WSL sessions that is the **local client**, not the remote host. See [Fonts and remote terminals](#fonts-and-remote-terminals). Use the `emoji` style for a no-font alternative.
- **Gemini status-line environment variable**: The canonical environment variable that Gemini CLI sets when invoking a status-line command has not been confirmed. `gsl` checks `GEMINI_CLI`, `GEMINI_API_KEY`, and `GEMINI_CLI_CONTEXT` as a best-effort heuristic. If none is set, `toolCtx` is `""` and theme resolution falls through to terminal detection. This has no effect on rendering correctness — it only means auto-theme may not pick the Gemini-settings palette on some Gemini CLI versions.

## Two on/off layers

| Layer | How | Effect |
|-------|-----|--------|
| **Hard off** | Remove (or comment out) `statusLine.command` from `~/.claude/settings.json` | Claude Code never calls the shim; no subprocess per turn; no status bar |
| **Soft off** | `gsl config disable` (sets `enabled: false`) | Shim is still called but prints nothing; bar goes blank |

Use **soft off** to toggle quickly without editing Claude settings. Use **hard off** for zero per-turn overhead.

## Fallback behavior

If `~/opt/bin/gsl` is missing or fails to exec, `statusline-command.sh` falls back to a pure-bash snippet that prints `<basename $PWD>  <git branch>  <HH:MM>`. The pipe never breaks and Claude Code's status bar degrades gracefully rather than erroring.

## Logging

gsl writes a structured JSON log so silent regressions get caught on the first failing refresh (Claude Code discards stderr for status-line commands, so any stderr-only message — like the one that hid issue #30 for months — is invisible).

| Setting | Default | Override |
|---------|---------|----------|
| Log file location | `$XDG_STATE_HOME/gsl/gsl.log`, or `~/.local/state/gsl/gsl.log`, or `~/.cache/gsl/gsl.log` (first writable wins) | `GSL_LOG_FILE=/abs/path/file.log` |
| Log level | `info` | `GSL_LOG_LEVEL=debug\|info\|warn\|error` |
| Rotation | 5 MB per file · 3 backups · 7-day max age · gzip-compressed | edit `Options.MaxSizeMB` / `MaxBackups` / `MaxAgeDays` in `internal/observe` |

Events currently recorded:

- `payload.parse_error` — Claude stdin JSON failed to parse; full error message (which names the offending field for type mismatches) is captured.
- `git.subprocess_error` / `gh.subprocess_error` / `mcp.subprocess_error` — a subprocess at one of the three runner seams exited non-zero; binary + argv + error are captured.
- `segment.panic` — a status-line segment panicked and was dropped.
- `segment.timeout` — a segment exceeded its per-segment deadline and was dropped.

Logger initialization is **always non-fatal**: if the log path cannot be opened, the logger degrades to `io.Discard` and the status line keeps rendering. Observability never breaks gsl.

## Development

```sh
cd sdk/gsl
go build ./...
go test ./... -cover
bash scripts/check-deps.sh
```

`scripts/check-deps.sh` enforces two gates:

1. **os/exec seam** — only `internal/git`, `internal/mcp`, and `internal/gh` may import `"os/exec"`. All subprocess work must go through those three runner interfaces so the logic stays testable with fakes.
2. **License gate** — `go-licenses check ./...` must find no GPL / AGPL / LGPL / forbidden licenses in the dependency tree (skipped with a warning when `go-licenses` is absent; enforced when `GSL_STRICT_CHECK=1`).
