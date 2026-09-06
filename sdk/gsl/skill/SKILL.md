---
name: gsl-status
description: Query and configure the gsl (Go Status Line) that renders a powerline-style status bar in Claude Code and Antigravity CLI.
---
# gsl — Go Status Line

`gsl` is a Go-based powerline-style status line. **Both** host CLIs drive it the same
way: they pipe a JSON payload on stdin after every assistant turn and print whatever
the shim writes to stdout.

- **Claude Code**: pipes its payload to `~/.claude/statusline-command.sh`; the shim
  `exec`s `gsl render`, so stdin flows straight through to the binary.
- **Antigravity CLI** (`agy`): pipes its payload to `~/.gemini/config/statusline-command.sh`
  — the *same* shim, deployed to agy's config root — which also `exec`s `gsl render`.

> **Note (changed in #157).** agy is a first-class payload host. Older docs said to point
> agy's `/statusline` at `gsl status` and claimed "the `ai` segment self-omits under
> Antigravity because no payload is supplied". **Both statements are false now.** agy sends
> a full JSON payload (model, context window, quota, terminal width), and the `ai` segment
> renders under agy exactly as it does under Claude Code.

## Segments

| Segment  | What it shows |
|----------|---------------|
| `dirgit` | Current directory name (basename, `~` for `$HOME`) + branch + staged/unstaged/untracked/stash/ahead/behind badges |
| `repo`   | Root-or-worktree indicator glyph + optional feature/worker/branch name + optional PR badge (tinted by state) + optional worktree count badge; self-omits outside a git repo. Links: glyph → repo home, label → branch on GitHub, badge → the PR |
| `ai`     | Model display name + context-window usage + MCP active/configured count + 5h/7d rate-limit percentages. Renders under **both** Claude Code and agy. Self-omits only when there is genuinely no payload on stdin (e.g. bare `gsl status` in a plain shell) |
| `time`   | Date + time formatted by config Go layouts + timezone abbreviation; always renders |

Each segment is independently enabled/disabled via `gsl config enable/disable/toggle <segment>`.

## Host payload differences

The two hosts send different shapes for the same information. `gsl` normalizes them.

| | Claude Code | Antigravity (`agy` v1.1.1) |
|---|---|---|
| Rate limits | `rate_limits.five_hour` / `.seven_day`, each `{used_percentage, resets_at}` | **No `rate_limits` at all.** Sends `quota` instead |
| Quota | — | `quota` object of buckets: `3p-5h`, `3p-weekly`, `gemini-5h`, `gemini-weekly`, each `{remaining_fraction, reset_time, reset_in_seconds}` |
| Width | `terminal_width` | `terminal_width` |
| Host self-ID | *(no `product` key)* | **`"product": "antigravity"`** in every payload |
| Theme source | `~/.claude/settings.json` → `theme` | `~/.gemini/antigravity-cli/settings.json` → top-level **`colorScheme`** |

**How gsl tells the hosts apart.** Both hosts run the *same* shim (`gsl render`, payload on
stdin) and both send `cwd` + `model` + `context_window`, so the payload shape alone cannot
identify the caller. gsl keys on agy's in-band **`product`** field, which it sends on every
render. That is what selects the theme source above — without it an agy render is read as a
Claude render and the Antigravity `colorScheme` is ignored entirely.

**Quota → rate-limit synthesis.** agy reports quota *remaining*; the `ai` segment shows
*used*. gsl inverts it (`used = (1 - remaining_fraction) * 100`) and maps buckets onto the
5h/7d windows by **suffix heuristic** — a key containing `week`/`7d` is the seven-day
window, one containing `5h`/`hour` is the five-hour window. Bucket keys are *not* hardcoded,
so if Google renames them the display degrades gracefully instead of vanishing. A first-party
(`gemini-*`) bucket wins over a third-party (`3p-*`) one.

**Tolerant decoding.** The payload is decoded field-by-field: a field whose JSON type is
unexpected is dropped on its own and the rest of the payload still renders. One bad field
can no longer blank the whole segment for the rest of the session (issues #30, #31).

## Subcommands

```
gsl render                          # Read the host's JSON payload from stdin, print the status line
gsl status                          # Print the status line now, without stdin (ai segment self-omits)
gsl preview                         # Interactive TUI: toggle segments, cycle styles, live time
gsl preview --once                  # Print one rendered frame and exit (CI / golden-file safe)
gsl version                         # Show version, commit, dirty flag, build date, description, binary path
gsl version --json                  # Same, as JSON
```

`gsl render` is what both hosts call. `gsl status` is for humans at a shell prompt.

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

### Segment options

Set under `segments[].options` in `~/.config/gsl/config.json`.

| Segment | Option | Default | Effect |
|---------|--------|---------|--------|
| `repo` | `show_pr` | `true` | Show the PR badge |
| `repo` | `show_count` | `true` | Show the worktree-count badge |
| `repo` | `name` | `"feature"` | Label source: `feature` / `worker` / `branch` / `off` |
| `repo` | `link_pr` | `true` | Make the PR badge a hyperlink to the PR (URL from the gss registry `pr_url`, or `gh pr view --json url`; unlinked when neither supplies one) |
| `dirgit` | `dir_link` | `"vscode"` | Directory link: `vscode` = vscode.dev GitHub view (PR changes if the branch has a PR, else the branch; `file://` off GitHub), `file` = always `file://` |
| `ai` | `usage_url` | Claude: `https://claude.ai/settings/usage`; agy: none | Target of the context / 5h / 7d links. The only way to give Antigravity a usage target. |
| `ai` | `model_url` | built-in family map | Model-name link. Empty = Claude families → `https://www.anthropic.com/claude/{family}`, gemini → Google's Gemini models page; template placeholders `{family}` `{model_id}` `{display_name}`; unknown family → no link |
| `time` | `link_url` | `https://time.is/{tz_city}` | Template for the date/time link. Placeholders `{tz}` `{tz_city}` `{iso_utc}` `{unix}`; empty disables it |

### Links (OSC 8 hyperlinks)

Every field with a web home is an OSC 8 hyperlink, underlined by default. Open
with the terminal's link gesture — **Ctrl+click** in gnome-terminal / VTE
(URL on hover), Cmd+click in iTerm2. Targets: directory → the vscode.dev GitHub
view (`/pull/<n>/changes` when the branch has a PR, else `/tree/<branch>`;
`file://<cwd>` off GitHub); dirgit branch and repo label →
`https://github.com/<owner>/<repo>/tree/<branch>` (GitHub remotes only, from
`git remote get-url origin`); repo glyph → repo home; PR badge → the PR; ai model
name → its model page (family from `model.id`, e.g. anthropic.com/claude/fable);
ai context/5h/7d → the usage page; time → the `link_url` template. The PR is
looked up once in cmd (gss registry, then gh) and shared by both segments.

Links are **spans**: each segment records the byte ranges of its raw text that
address something, and the join layer wraps exactly those ranges (OSC 8 +
underline SGR). A URL carries zero display width — `term.StripANSI` consumes
OSC as well as CSI — so the fit loop measures, truncates, and sheds the same
with or without links, and a truncated field keeps the link on what survives.

**Two off-switches, both fail-open (an error only ever leaves links on):**

- `gsl config set links off` — no links; `plain` keeps OSC 8 without the underline; `underline` is the default.
- gff flags (all `true` by default): `gsl.links.enabled` (master), `gsl.links.repo`
  (PR badge + label + glyph + dirgit branch), `gsl.links.dirgit` (directory),
  `gsl.links.ai`, `gsl.links.time`. `gsl render` resolves them through the gff SDK
  from the dotfiles namespace (registered by `install.sh` via `gff install`),
  falling back to the checkout in `$DOTFILES_DIR`; unresolvable ⇒ `true`.
  Lookups run concurrently with the origin lookup under a 100 ms budget.

**Keys valid for `config get`:** `enabled`, `style`, `timezone`, `time_format`, `date_format`, `links`, `segments`, `styles`

**Keys valid for `config set`:** `style`, `timezone`, `time_format`, `date_format`, `links` (`underline` | `plain` | `off`)

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
| **Hard off** | Remove `statusLine.command` from `~/.claude/settings.json` (Claude), or set `statusLine.enabled: false` in `~/.gemini/antigravity-cli/settings.json` (agy) | The host never calls the shim; no bar |
| **Soft off** | `gsl config disable` | The shim is called but prints nothing; the bar goes blank |

Use **soft off** to toggle quickly without editing settings. Use **hard off** for zero
overhead (no subprocess fork per turn).

## Install wiring

The shim (`ai/claude/statusline-command.sh`) is **copied** — not symlinked — into each
host's well-known `$HOME` path. Copy is the forward provisioning mechanism (see the root
`CLAUDE.md`): a repo-pointing symlink couples global config to the checkout location, and
would aim `$HOME` config at a transient path if the installer ever ran from a `gss` worktree.

| Installer | Destination |
|-----------|-------------|
| `opt/scripts/system/install_claude_skills.sh` | `~/.claude/statusline-command.sh` (mode 0755) |
| `opt/scripts/system/install_antigravity_skills.sh` | `~/.gemini/config/statusline-command.sh` (mode 0755) |

agy's `statusLine` block is applied from `ai/antigravity/settings.forced.json` into
`~/.gemini/antigravity-cli/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "bash ~/.gemini/config/statusline-command.sh",
    "enabled": true
  }
}
```

The skill directory is linked by `opt/scripts/system/sync-skills.sh` into
`~/.gemini/config/skills/gsl-status` (Antigravity) and `~/.claude/skills/gsl-status` (Claude).

Run `sync-skills --build` to rebuild the binary and refresh all skill links.

**Font setup (Nerd Font for powerline style)**

`install.sh` runs the gsl-packaged font installers AFTER the gsl build so the
`powerline` style renders glyphs correctly. Pinned release: `NERD_FONTS_VERSION=v3.4.0`
(ryanoasis/nerd-fonts, `Meslo.zip` — family: `MesloLGS Nerd Font`).

| OS | Installer | Notes |
|----|-----------|-------|
| macOS | `sdk/gsl/scripts/install_nerd_font_macos.sh` | Writes an iTerm2 Dynamic Profile (`gsl-nerd-font`) |
| Linux/WSL | `sdk/gsl/scripts/install_nerd_font_linux.sh` | Points gnome-terminal's default profile at the font (only when it is still on the system font); also invokes the Windows installer from WSL for Windows Terminal |
| Windows | `sdk/gsl/scripts/install_nerd_font_windows.ps1` | Called by `setup-apps.ps1 → Install-NerdFont`; touchless (`-NonInteractive -ExecutionPolicy Bypass`) |

After install, `sdk/gsl/scripts/check-font-glyphs.sh` proves the installed font
covers all 17 PUA codepoints gsl emits. Note it validates the font *file*, not
the terminal's font *selection* — a terminal still pointed at a non-Nerd font
renders tofu even though this check passes. Run it manually to verify:

```bash
bash sdk/gsl/scripts/check-font-glyphs.sh
# expect: OK: all 17 gsl codepoints present in .../MesloLGSNerdFont-Regular.ttf
```

## Fallback behaviour

If `~/opt/bin/gsl` is missing, `statusline-command.sh` falls back to a
dependency-light bash snippet that prints: `<basename $PWD>  <git branch>  <HH:MM>`.
The pipe never breaks and no error is returned, so the host's status bar degrades
gracefully instead of breaking. This applies to both Claude Code and agy.
