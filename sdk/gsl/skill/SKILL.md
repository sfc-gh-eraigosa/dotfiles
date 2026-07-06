---
name: gsl-status
description: Query and configure the gsl (Go Status Line) that renders a powerline-style status bar in Claude Code and Antigravity CLI.
---
# gsl — Go Status Line

`gsl` is a Go-based powerline-style status line with two integration points:

- **Claude Code**: Claude pipes a JSON payload (cwd, model, context-window usage) to
  `~/.claude/statusline-command.sh` after every assistant turn; the shim `exec`s
  `gsl render` so the payload flows straight to the binary and the rendered line
  appears in the Claude Code status bar.
- **Antigravity CLI** (`agy`): point its built-in `/statusline` custom command at
  `gsl status` to render the status line on demand without a JSON payload. The
  `ai` segment self-omits because no Claude payload is supplied; `dirgit`,
  `repo`, and `time` still render.

## Segments

| Segment  | What it shows |
|----------|---------------|
| `dirgit` | Current directory name (basename, `~` for `$HOME`) + branch + staged/unstaged/untracked/stash/ahead/behind badges |
| `repo`   | Root-or-worktree indicator glyph + optional feature/worker/branch name + optional PR badge (tinted by state) + optional worktree count badge; self-omits outside a git repo |
| `ai`     | Model display name + context-window usage + MCP active/configured count + 5h/7d rate-limit percentages; **self-omits when no Claude payload is present** (Antigravity/CLI mode) |
| `time`   | Date + time formatted by config Go layouts + timezone abbreviation; always renders |

Each segment is independently enabled/disabled via `gsl config enable/disable/toggle <segment>`.

## Subcommands

```
gsl render                          # Read Claude JSON payload from stdin, print status line
gsl status                          # Print status line now (no stdin; ai segment self-omits)
gsl preview                         # Interactive TUI: toggle segments, cycle styles, live time
gsl preview --once                  # Print one rendered frame and exit (CI / golden-file safe)
gsl version                         # Show version, commit, dirty flag, build date, description, binary path
gsl version --json                  # Same, as JSON
```

## Configuration

Config file: **`~/.config/gsl/config.json`**

```
gsl config get                      # Print the full config as JSON
gsl config get <key>                # Print one field: enabled, style, timezone, time_format, date_format, segments, styles
gsl config set <key> <value>        # Set a field: style, timezone, time_format, date_format
gsl config enable [segment]         # Enable the master switch (no arg) or a named segment
gsl config disable [segment]        # Disable the master switch (no arg) or a named segment
gsl config toggle <segment>         # Toggle a named segment on/off
gsl config style [name]             # Show the current style, or set it to <name>
gsl config style --list             # List all builtin + user-defined styles (* = active)
```

**Segments** valid for enable/disable/toggle: `dirgit`, `repo`, `ai`, `time`

**Keys valid for `config get`:** `enabled`, `style`, `timezone`, `time_format`, `date_format`, `segments`, `styles`

**Keys valid for `config set`:** `style`, `timezone`, `time_format`, `date_format`

## Styles

Two built-in styles:

| Name | Separator | Fill | Glyphs | Notes |
|------|-----------|------|--------|-------|
| `powerline` | filled chevron | yes | nerdfont | Default; requires a Nerd Font-patched terminal font |
| `emoji` | thin bar (`\|`) | no | emoji | No font dependency; works in any terminal |

Add user overrides in the `styles` object of `config.json` under the same key as the built-in name. Only the fields you specify are changed; omitting `fill` preserves the built-in's fill value (fill-presence merging).

## Two on/off layers

| Layer | How | Effect |
|-------|-----|--------|
| **Hard off** | Remove (or comment out) `statusLine.command` from `~/.claude/settings.json` | Claude Code never calls the shim; no bar |
| **Soft off** | `gsl config disable` | Shim is called but prints nothing; bar goes blank |

Use **soft off** when you want to toggle quickly without editing settings. Use
**hard off** when you want zero overhead (no subprocess fork per turn).

## Install wiring

The shim is installed by `opt/scripts/system/install_claude_skills.sh`:

```
~/.claude/statusline-command.sh  →  $DOTFILES/ai/claude/statusline-command.sh
```

The skill directory is linked by `opt/scripts/system/sync-skills.sh` into
`~/.claude/skills/gsl-status` and `~/.agents/skills/gsl-status`.

Run `sync-skills --build` to rebuild the binary and refresh all skill links.

**Font setup (Nerd Font for powerline style)**

`install.sh` runs the gsl-packaged font installers AFTER the gsl build so the
`powerline` style renders glyphs correctly. Pinned release: `NERD_FONTS_VERSION=v3.4.0`
(ryanoasis/nerd-fonts, `Meslo.zip` — family: `MesloLGS Nerd Font`).

| OS | Installer | Notes |
|----|-----------|-------|
| macOS | `sdk/gsl/scripts/install_nerd_font_macos.sh` | Writes an iTerm2 Dynamic Profile (`gsl-nerd-font`) |
| Linux/WSL | `sdk/gsl/scripts/install_nerd_font_linux.sh` | Also invokes Windows installer from WSL for Windows Terminal |
| Windows | `sdk/gsl/scripts/install_nerd_font_windows.ps1` | Called by `setup-apps.ps1 → Install-NerdFont`; touchless (`-NonInteractive -ExecutionPolicy Bypass`) |

After install, `sdk/gsl/scripts/check-font-glyphs.sh` proves the installed font
covers all 17 PUA codepoints gsl emits. Run it manually to verify:

```bash
bash sdk/gsl/scripts/check-font-glyphs.sh
# expect: OK: all 17 gsl codepoints present in .../MesloLGSNerdFont-Regular.ttf
```

## Antigravity command

Antigravity CLI ships a built-in `/statusline` slash command that supports a
custom command — set it to `gsl status` to render the current status line on
demand. (The Gemini-era `/gsl-status` TOML custom command retired with
Gemini CLI.)

## Fallback behaviour

If `~/opt/bin/gsl` is missing, `statusline-command.sh` falls back to a
dependency-light bash snippet that prints: `<basename $PWD>  <git branch>  <HH:MM>`.
The pipe never breaks and no error is returned, so Claude Code's status bar
degrades gracefully instead of breaking.
