---
name: herdr
description: Use when the user mentions herdr or is working inside a herdr pane and wants to save/restore a pane layout, change herdr's theme, prefix or other preferences on this host only, start or check on an agent in another herdr pane, or asks why the herdr sidebar colors are unreadable. Requires HERDR_ENV=1.
---

# herdr

herdr is the terminal workspace manager the dotfiles install for coding agents
(workspaces > tabs > panes, with agent state in the sidebar). This skill is the
herdr counterpart of the `tmux` skill: the same verbs, backed by herdr's own
CLI plus two bundled tools for what the CLI lacks.

**Gate first.** Every command below talks to the server socket of the pane you
are in. Run `test "${HERDR_ENV:-}" = 1`; if it fails, say you are not inside
herdr and stop. Never control a herdr session from outside it.

**Host-local by design.** Everything this skill writes lives under
`~/.config/herdr/` (`layouts/`, `config.toml`), never in the dotfiles repo.
The fleet baseline for `config.toml` is `ai/herdr/config.toml`, rendered by
`install_herdr.sh config`; per-host changes go through `herdr-prefs`, which
takes ownership of the file away from `install.sh`.

## Verb map (tmux skill -> herdr)

| tmux-mgr | herdr | Notes |
| --- | --- | --- |
| `session list/new/attach/kill` | `herdr session list/attach <name>/stop <name>` | Named sessions are separate servers; prefer workspaces |
| `window split`, `pane split` | `herdr pane split --current --direction right\|down --cwd "$PWD" --no-focus` | Read the new id from `.result.pane.pane_id` |
| `window move/resize` | `herdr pane focus/resize --direction ...` | |
| `capture` | `herdr pane read <id>` / `herdr agent read <name>` | |
| `agent start` | `herdr agent start <name> --kind claude --pane <id>` | Pane must be an idle shell; then `herdr agent prompt <name> "..." --wait` |
| `agent list/complete` | `herdr agent list` / `herdr agent wait <name>` + `agent read` | States: working, blocked, done, idle, unknown |
| `save` / `restore` | `scripts/herdr-layout save\|restore <name>` | No CLI equivalent in herdr; see below |
| (none) | `scripts/herdr-prefs status\|get\|set\|reset` | Host-local preferences |

The installed binary is the syntax authority: `herdr <group>` with no
subcommand prints that group's usage. Do not run bare `herdr` (it attaches the
TUI). Never answer a `blocked` approval dialog on the user's behalf.

## Layouts (`scripts/herdr-layout`)

herdr auto-restores the last session shape on restart (`session.json`) but has
no named layouts and no `herdr layout` CLI; only the socket methods
`layout.export` / `layout.apply` exist. The bundled tool wraps them:

```bash
S=~/.claude/skills/herdr/scripts        # skill folder, symlinked by sync-skills
$S/herdr-layout save review --current    # the caller's tab -> ~/.config/herdr/layouts/review.json
$S/herdr-layout save review --tab w1:t2  # a specific tab; bare `save` = the UI-focused tab
$S/herdr-layout list | show review | delete review
$S/herdr-layout restore review [--workspace w2] [--label dev] [--replace-tab w1:t1] [--focus]
```

Facts that matter:

- From an agent pane use `--current`: the UI-focused tab may belong to the
  user or another client.
- One tab per saved layout (export is per tab). Save each tab under its own name.
- Saved JSON is portable: ephemeral `pane_id`s are stripped; tree, labels,
  cwd, env and argv commands are kept.
- Restore creates a NEW tab from the tree, labelled after the layout and left
  in the background unless `--focus`. It does not preserve live PTYs,
  scrollback or running processes. `--replace-tab` closes the old tab after
  the new one exists. The printed tab id is the new tab (ids are monotonic
  per session, so expect `w1:t5`, not `w1:t2`).
- `HERDR_LAYOUT_DIR` and `HERDR_CONFIG_DIR` redirect where the tools read and
  write (dry runs, tests). herdr itself ignores them: to validate a redirected
  config use `XDG_CONFIG_HOME=<dir-containing-herdr/> herdr config check`.

## Preferences (`scripts/herdr-prefs`)

```bash
$S/herdr-prefs status                    # ownership, theme/prefix keys, and the host's light/dark report
$S/herdr-prefs set theme.dark_name nord  # edit one key, drop the managed marker, reload the server
$S/herdr-prefs reset                     # back to the fleet baseline (keeps config.toml.bak)
```

`set` removes the `managed by dotfiles` marker, so from then on `install.sh`
warns and leaves the file alone. Keys are `section.key`; true/false/numbers
stay unquoted. herdr reloads live, no restart.

**Theme trap.** The baseline has `auto_switch = true`, so herdr follows the
host terminal's light/dark report and `theme.name` is only the fallback.
Setting `theme.name = nord` alone changes nothing visible on a host that
reports its appearance. Read `host appearance:` from `herdr-prefs status`
(never assume dark) and change the sibling that applies, `theme.dark_name` or
`theme.light_name`, or pin one theme for every terminal profile:

```bash
$S/herdr-prefs set theme.auto_switch false && $S/herdr-prefs set theme.name nord
```

Built-in themes (0.8.2): catppuccin / catppuccin-latte, tokyo-night /
tokyo-night-day, gruvbox / gruvbox-light, one-dark / one-light, solarized /
solarized-light, kanagawa / kanagawa-lotus, rose-pine / rose-pine-dawn,
`terminal` (follows the host ANSI palette), and dark-only dracula, nord,
vesper. A dark-only theme pinned with `auto_switch = false` is unreadable on
a light profile; as `dark_name` it is not.

## Unreadable sidebar

Dark text on grey means a dark herdr theme over a light terminal (or vice
versa). Run `herdr-prefs status`: a host-owned file with `auto_switch =
false` and a dark `name` on a light terminal is the usual cause. Fix with
`herdr-prefs reset` (managed, auto-switching) or set the matching sibling.

## Common mistakes

- Building a socket client or grepping the schema: the tools above already do it.
- Editing `~/.config/herdr/config.toml` by hand and leaving the marker: the
  next `install.sh` run overwrites the edit. Use `herdr-prefs set`.
- Committing a rendered `config.toml` or a layout JSON to the dotfiles repo.
- Sending keys into an agent pane to "check on it": use `agent list`/`read`.
