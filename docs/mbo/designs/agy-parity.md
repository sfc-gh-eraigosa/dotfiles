# agy startup parity with Claude Code — design

- **Slug:** agy-parity
- **Date:** 2026-09-03
- **Status:** Approved (2026-09-03, PR #269)
- **Relates to:** design issue #268 · design PR #269 · builds on `claude-config` (PR #126), `ai-config-home-provisioning` (PR #113), `memory-provisioning` (#134)
- **Author(s):** Edward Raigosa + Claude (analysis session 2026-09-02)

## 1. Problem / context

The repo installs and configures both assistants, but only Claude Code gets a real
**startup contract**. Every fact below was verified against the checkout and the live
host (agy 1.1.25; the repo docs still say "verified against agy 1.0.16").

| Startup concern | Claude (`ai/claude/`, `install_claude_skills.sh`) | agy (`ai/antigravity/`, `install_antigravity_skills.sh`) |
| :-- | :-- | :-- |
| Shell wrapper | `claude()` reads opt-in sentinels under `~/.config/claude/` and injects `--dangerously-skip-permissions` / `--remote-control`; `claude-config yolo\|remote\|doctor\|status`; 36 test cases | `agy()` only anchors tmux. YOLO is a second alias `agy-yolo`, per invocation. No persistent config, no doctor, 8 test cases |
| Settings seed | `settings.json.template` seeds first run: `permissions.defaultMode: auto`, allow list (gss, tmux-mgr, read-only git, make targets), `theme: auto`, `editorMode: vim`, sandbox block off, notification flags | Seeds `{}`. The live host file had to be hand-edited to add `allowNonWorkspaceAccess` and `trustedWorkspaces` |
| Forced subset (every run) | hooks + statusLine + `permissions.deny`/`ask` replaced, `permissions.allow` unioned (`apply-forced-settings.sh`) | `statusLine` only |
| Permission policy | deny list (`rm -rf /`, `mkfs`, `dd`, …), ask list (force push, reboot, `sudo`) | None in settings. The safety hook is the only gate, and the adapter maps guard exit 0 to `allow`, so agy auto-approves anything the guard does not flag |
| Hooks | PreToolUse safety + privacy, `DirectoryAdded` guard, herdr `SessionStart` (survives the merge) | PreToolUse via `antigravity_adapter.sh` only. `hooks.json` is overwritten whole on each run, dropping herdr's entry; `install.sh` carries an ordering hack to re-add it |
| Slash commands | 8 repo commands linked into `~/.claude/commands` (`/sync`, `/gss`, `/gss-pr`, `/gss-scan`, `/team`, `/tmux-agent`, `/ssh-find`, `/ssh-keys`) | None. agy accepts commands only through plugins (`plugins/<name>/commands/*.toml`) |
| Memories | `provision-claude-memory.sh` seeds `ai/claude/memory/` into the live project slug | Nothing |
| Skills, plugins, teams, status line | shared via `sync-skills`, `sync-plugins`, `install_ai_teams.sh`, the copied statusline shim | parity already |
| Binary policy | one canonical binary; `claude-config doctor` flags duplicates; version pinned by npm/brew | bootstrapper plus an unconditional `agy update` on every install run |

**What agy 1.1.25 supports** (from its shipped `antigravity_guide` + `agy-customizations`
skills, the public CLI reference, and `strings` on the binary):

- `~/.gemini/antigravity-cli/settings.json` keys: `toolPermission`
  (`request-review` | `always-proceed` | `proceed-in-sandbox` | `strict`),
  `permissions.{allow,deny,ask}` in `action(target)` form (e.g. `command(git push --force)`,
  precedence deny > ask > allow), `editorMode: "vim"`, `vimInsertFirst`, `colorScheme`,
  `notifications`, `enableTerminalSandbox`, `allowNonWorkspaceAccess` (boolean in the CLI),
  `trustedWorkspaces`, `statusLine`, `showTips`, `showFeedbackSurvey`, `enableTelemetry`.
- CLI flags: `--dangerously-skip-permissions`, `--mode accept-edits|plan`, `--sandbox`,
  `--print`/`-p`, `--effort`, `--model`, `--agent`, `--add-dir`. **No `--remote-control`
  equivalent** (remote presence is `remoteControlHostname` in `~/.gemini/config/config.json`
  plus `mic-serve`).
- Customization root `~/.gemini/config/`: `hooks.json` (named hooks, merged by name),
  `plugins/<name>/{plugin.json,commands/*.toml,rules/AGENTS.md,skills/,hooks.json}`,
  `skills/`, `config.json` (`plugins` enable map).
- Hook events: `PreToolUse`, `PostToolUse`, `PreInvocation`, `PostInvocation`, `Stop`.
  No `DirectoryAdded`; the `PreToolUse` payload carries `workspacePaths`.

## 2. Goals & non-goals

**Goals**

- One launch contract for both assistants: same opt-in sentinel model, same
  `<tool>-config` verbs, same default-OFF posture on a fresh host.
- A seeded agy settings baseline that mirrors Claude's template key-for-key where agy has
  a key, applied with the **same** forced-field merge semantics (`apply-forced-settings.sh`
  unchanged).
- The repo's deny/ask permission policy enforced in agy's own settings, not only in the hook.
- The repo's slash commands and account memories reach agy through its native plugin
  mechanism.
- `hooks.json` provisioning that preserves foreign named hooks (herdr), removing the
  `install.sh` ordering hack.
- Tests and docs at the same bar as the Claude side (`aliases_test.sh` parity cases,
  `install_antigravity_skills_test.sh` coverage for seed/merge/preserve).

**Non-goals**

- A remote-control equivalent for agy (the CLI has no such flag).
- Changing Claude's side of the contract. Two Claude-side inconsistencies surfaced during
  analysis (`~/.config/claude/aliases.sh` is still a symlink; the Claude template does not
  ship the `inputNeededNotifEnabled`/`agentPushNotifEnabled` keys the live host has) and are
  noted for a separate follow-up.
- Pinning the agy version. Whether `agy update` on every install run should follow the
  Claude fleet policy in `docs/claude-code-support.md` is raised as an open question, not
  decided here.
- Converting agy's `knowledge/` or `brain/` dirs. They are agy-owned and undocumented.

## 3. Options considered

**Option A — Mirror the Claude provisioning shape file-for-file (RECOMMENDED).**
Add `ai/antigravity/settings.json.template`, extend `settings.forced.json`, rewrite
`aliases.sh` in the `claude_launch_flags` shape, emit a local `dotfiles` plugin for
commands + memory rules, and jq-merge `hooks.json`. Reuses `apply-forced-settings.sh`,
the test helpers, and the copy-forward provisioning rule verbatim.
*Trade-off:* two parallel installers to keep in sync (already the case today).
*Why it wins:* every mechanism it needs is already proven on the Claude side and every
agy-side hook point exists in 1.1.25; the diff is additive and each piece is independently
testable.

**Option B — One shared installer with a per-tool adapter table (REJECTED for now).**
Collapse `install_claude_skills.sh` and `install_antigravity_skills.sh` into one script
driven by a tool → paths/keys map. Cleaner long term, but it rewrites the tested Claude path
to fix an agy gap, and the two tools' key names and file layouts differ enough that the
adapter table would carry most of the complexity anyway. Revisit once agy parity exists
and both sides are stable.

**Option C — Rely on the safety hook alone for agy permissions (REJECTED).**
Today's state. The adapter's exit-0 → `allow` mapping means agy silently runs in an
auto-approve posture that Claude reaches only through `defaultMode: auto` plus its
deny/ask lists. Leaving the deny/ask policy out of agy's own settings makes the hook a
single point of failure (a missing `hooks.json` degrades to agy's default, not to ours).

## 4. Decision

Option A, delivered as seven units. Units 1 through 4 are self-contained and land first;
5 and 6 depend on two verifications listed in §7; 7 is housekeeping.

1. **`ai/antigravity/aliases.sh` in the Claude shape.** Top-level `agy_launch_flags`
   (no underscore prefix, TTY state passed in), `agy()` injecting from the sentinel,
   `agy-config yolo on|off`, `agy-config status`, `agy-config doctor`. Sentinel
   `${XDG_CONFIG_HOME:-$HOME/.config}/antigravity/yolo.enabled`, present = ON, default OFF.
   Print mode (`-p`/`--print`/`--prompt`) is scanned positionally, as Claude does, so a
   prompt containing `-p` is not mistaken for print mode. `agy-yolo` is removed (one
   canonical alias per workflow); its former behaviour is `agy-config yolo on`. No
   cross-repo contract variable exists on the agy side, so no `AGY_YOLO_FILE` guarantee
   is made. **Doctor** reports the resolved binary and warns on duplicates on PATH, same as
   Claude's.
2. **`ai/antigravity/settings.json.template`** seeded on first run instead of `{}`:

   | Claude template key | agy key | Value |
   | :-- | :-- | :-- |
   | `permissions.defaultMode: auto` | `toolPermission` | `request-review` (the hook's `allow` verdict is what auto-approves; agy's `always-proceed` is the YOLO tier and stays a per-launch flag) |
   | `editorMode: vim` | `editorMode` | `vim` |
   | `theme: auto` | `colorScheme` | `terminal` |
   | `inputNeededNotifEnabled` / `agentPushNotifEnabled` | `notifications` | `true` |
   | `sandbox.enabled: false` | `enableTerminalSandbox` | `false` |
   | (no Claude analogue; Claude prompts per directory) | `allowNonWorkspaceAccess` | `true` (gss worktrees under `~/.config/gss` and `~/.herdr`, and `~/opt/scripts`, sit outside any workspace root) |
   | `permissions.allow` | `permissions.allow` | the same list translated to `command(<prefix>)` |

   `trustedWorkspaces` stays host-owned (the analogue of Claude's `enabledPlugins`), as does
   anything else the host has set. `showTips`/`showFeedbackSurvey` are left at agy's
   defaults; they have no Claude analogue.
3. **`ai/antigravity/settings.forced.json`** gains `permissions.deny` and `permissions.ask`
   translated from Claude's forced subset (`command(rm -rf /)`, `command(mkfs)`,
   `command(git push --force)`, `command(sudo)`, …) plus the gss `push`/`pr`/`sync` allow
   entries. `apply-forced-settings.sh` already replaces `deny`/`ask` and unions `allow`, so
   the merge script does not change.
4. **`hooks.json` merge, not overwrite.** Render the repo's `guards` entry, then jq-merge it
   over the existing file so any other named hook (herdr's) survives. Delete the ordering
   comment/hack in `install.sh` and `install_herdr.sh` once this lands. Add a second
   `PreToolUse` rule through the adapter that applies `dir_added_guard.sh`'s check to
   file tools using the payload's `workspacePaths`, since agy has no `DirectoryAdded` event.
5. **Slash commands via a local plugin.** `install_antigravity_skills.sh` writes
   `~/.gemini/config/plugins/dotfiles/` with `plugin.json` and one `commands/<name>.toml`
   (`description` + `prompt`) per `ai/claude/commands/*.md`, converted at install time so the
   Markdown stays the single source. Enable it in `config.json`'s `plugins` map.
6. **Memories as plugin rules.** The same plugin ships `rules/AGENTS.md` generated from
   `ai/claude/memory/*.md` (the scrubbed canonical store). Rules are always-on for an enabled
   plugin, the nearest agy analogue to a seeded memory index.
7. **Housekeeping.** `ai/antigravity/AGENTS.md` (version 1.1.25, new files, `agy-config`),
   `docs/machine-local-overrides.md` (agy launch config), `ai/antigravity/scripts/sanity_check.sh`
   (assert seeded keys), `install_antigravity_skills_test.sh` (seed, forced merge, hooks
   preserve), `ai/antigravity/aliases_test.sh` (port the yolo/doctor/print-mode cases).

**Provisioning rules that carry over unchanged:** copy-forward into well-known `$HOME`
paths, no new repo-pointing symlinks, host-owned settings file with a forced subset, tests
next to the script, `make lint-portability` clean.

## 5. Risks & blast radius

- **Permission-target semantics.** agy documents `command(...)` targets as
  "prefix / regex / `*`". If prefix matching differs from Claude's `Bash(x:*)`, a deny rule
  could over- or under-match. Mitigation: §7 verification before unit 3 ships; deny rules
  are additive to the hook, never a replacement.
- **Command placeholder syntax.** The TOML conversion in unit 5 must map Claude's
  `$ARGUMENTS`; agy's substitution syntax is unverified. Mitigation: §7 verification; ship
  unit 5 behind the check.
- **Behaviour change for existing hosts.** Seeding only happens when the settings file is
  absent, so existing hosts get the forced subset (deny/ask) but not the template values.
  That matches how Claude hosts were migrated. Hosts that relied on `agy-yolo` lose the
  alias; the `agy-config` status line tells them what replaced it.
- **`allowNonWorkspaceAccess: true`** widens agy's file reach on a fresh host. The privacy
  guard still gates writes; this mirrors what the live host already set by hand.
- **Blast radius** is confined to `ai/antigravity/`, `ai/hooks/antigravity_adapter.sh`,
  `opt/scripts/system/install_antigravity_skills.sh` (+ test), two comment blocks in
  `install.sh`/`install_herdr.sh`, and docs. Claude provisioning is untouched.

## 6. Rollback

Every unit is a file the installer copies or renders. Reverting the commit and re-running
`install.sh` restores the previous `aliases.sh`, `hooks.json`, and the forced subset; the
seeded template keys stay in the host-owned settings file (harmless, and removable by hand
or via the once-only `settings.json.bak`). The generated `dotfiles` plugin directory is
removed by the installer when its source is absent.

## 7. Evidence expectations

- **Test-run captures** for `ai/antigravity/aliases_test.sh` (parity cases with
  `ai/claude/aliases_test.sh`: defaults OFF, yolo on/off, print-mode exclusion, doctor
  single/duplicate binary) and `install_antigravity_skills_test.sh` (first-run seed, forced
  merge preserves host keys, hooks.json merge preserves a foreign named hook).
- **Live-host transcripts**: `agy-config status` on a fresh sentinel dir; `jq` dump of the
  seeded `settings.json`; `hooks.json` after an `install_herdr.sh integrations` run showing
  both `guards` and `herdr` entries.
- **Two verifications that gate units 3, 5 and 6, recorded in the plan's tracking ledger:**
  1. agy `command(...)` matching: seed `permissions.deny: ["command(rm -rf /)"]` and confirm
     an interactive `rm -rf /tmp/x` is not denied while `rm -rf /` is.
  2. Command TOML argument substitution and whether `agy agents` lists the team agents
     written to `~/.config/antigravity/agents/` (a non-TTY `agy agents` printed nothing
     during analysis).
- **Portability gate:** `make lint-portability` and `make lint-shell` output on the new
  scripts.

> Produced via the `mbo-plan` skill from the 2026-09-02 analysis session. Register the
> objective in `../index.md`. The matching spec goes in `../specs/agy-parity.md`.
