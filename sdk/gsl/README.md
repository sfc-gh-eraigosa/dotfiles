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
