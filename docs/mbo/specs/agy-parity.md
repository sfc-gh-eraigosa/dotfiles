# agy startup parity with Claude Code — spec

- **Slug:** agy-parity
- **Date:** 2026-09-03
- **Status:** Approved
- **Relates to:** issue #268 · design PR #269 · design `../designs/agy-parity.md`

## 1. Goal

A fresh host that runs `install.sh` gets the Antigravity CLI (`agy` 1.1.25) with the same
startup contract Claude Code already has: an opt-in, per-machine launch config driven by a
sentinel file and an `agy-config` tool; a seeded `settings.json` whose keys mirror the Claude
template; the repo's deny/ask permission policy enforced in agy's own settings on every install;
`hooks.json` provisioning that keeps other tools' hooks; and the repo's slash commands plus the
account memories delivered through agy's native plugin mechanism. Existing hosts pick up the
forced subset, the merged hooks, and the plugin on the next install run without losing anything
they set by hand.

## 2. Use cases

- **UC-1 Enable YOLO on one machine** — *actor:* user · *trigger:* `agy-config yolo on` ·
  *flow:* creates `${XDG_CONFIG_HOME:-$HOME/.config}/antigravity/yolo.enabled`; later `agy`
  invocations run `command agy --dangerously-skip-permissions "$@"` · *acceptance:* sentinel
  exists; `agy_launch_flags tty 'fix bug'` yields exactly `--dangerously-skip-permissions`;
  `agy-config yolo off` removes it and the flags array is empty again.
- **UC-2 Inspect launch state and binary** — *actor:* user · *trigger:* `agy-config` /
  `agy-config status` / `agy-config doctor` · *flow:* prints `yolo ON|OFF`; doctor prints the
  resolved binary and every `agy` on PATH · *acceptance:* default is `OFF`; doctor exits 0 with
  one binary and non-zero with a WARNING when two distinct binaries are on PATH.
- **UC-3 Fresh host gets the baseline** — *actor:* `install.sh` on a host with no
  `~/.gemini/antigravity-cli/settings.json` · *flow:* the installer seeds the file from
  `ai/antigravity/settings.json.template`, then applies the forced subset ·
  *acceptance:* `toolPermission == "request-review"`, `editorMode == "vim"`,
  `allowNonWorkspaceAccess == true`, `notifications == true`, `enableTerminalSandbox == false`,
  `permissions.allow` contains `command(gss status)`, `permissions.deny` contains
  `command(rm -rf /)`, `statusLine.enabled == true`.
- **UC-4 Existing host keeps its choices** — *actor:* `install.sh` on a host whose settings
  file already exists with `colorScheme: "light"` and a custom `permissions.allow` entry ·
  *flow:* no template seed; forced merge only · *acceptance:* `colorScheme` still `"light"`,
  the host allow entry survives alongside the forced ones, `permissions.deny`/`ask` equal the
  repo lists exactly, `toolPermission` is absent (templates never overwrite an existing file).
- **UC-5 herdr's hook survives an install** — *actor:* `install.sh` on a host where
  `~/.gemini/config/hooks.json` already holds a `herdr` named hook · *flow:* the installer
  merges its `guards` entry over the file · *acceptance:* both `guards` and `herdr` keys are
  present afterwards; a stale `guards` command is replaced by the freshly rendered one.
- **UC-6 File tool aimed at a credential path is confirmed, not silently allowed** —
  *actor:* agy running a `write_to_file` whose `TargetFile` is under `~/.ssh` · *flow:* the
  adapter runs the guards, then the sensitive-root check · *acceptance:* decision `ask` with a
  reason naming the root; the same tool aimed at a workspace file is `allow`; a `run_command`
  of `rm -rf /` is still `deny` (guard verdicts win).
- **UC-7 Slash commands and memories reach agy** — *actor:* `install.sh` · *flow:* renders
  `~/.gemini/config/plugins/dotfiles/` (`plugin.json`, one `commands/<name>.toml` per
  `ai/claude/commands/*.md`, `rules/AGENTS.md` from the `scope: account` memories) and enables
  the plugin in `~/.gemini/config/config.json` · *acceptance:* TOML count equals command count;
  every TOML has `description` and `prompt`; no literal `$ARGUMENTS` or Claude `!`-backtick
  injection lines remain; `config.json` has `plugins.dotfiles.enabled == true` and any
  pre-existing plugin entry is untouched.

## 3. Architecture

| Component | Boundary | Independently testable by |
| :-- | :-- | :-- |
| `ai/antigravity/aliases.sh` | sourced shell functions: `agy_launch_flags`, `agy`, `agy-config` | `ai/antigravity/aliases_test.sh` (sources the file in subshells with an isolated `XDG_CONFIG_HOME`) |
| `ai/antigravity/settings.json.template` | first-run seed, host-owned afterwards | installer test on a fresh temp `$HOME` |
| `ai/antigravity/settings.forced.json` | immutable subset applied via `opt/scripts/system/apply-forced-settings.sh` (unchanged) | installer test on fresh + pre-existing temp `$HOME`s |
| `opt/scripts/system/install_antigravity_skills.sh` | the only writer of `~/.gemini/config/hooks.json` (merge), `settings.json` (seed + merge), `plugins/dotfiles/`, `config.json` plugins map | `opt/scripts/system/install_antigravity_skills_test.sh` |
| `ai/hooks/antigravity_adapter.sh` | payload translation + guard verdicts + sensitive-root check | new `ai/hooks/antigravity_adapter_test.sh` |
| `opt/scripts/system/render-agy-plugin.sh` | pure function `render-agy-plugin.sh <repo-root> <plugin-dir>` | new `opt/scripts/system/render-agy-plugin_test.sh` |

Data flow at install time: `install.sh` → `install_antigravity_skills.sh` → copies hooks, renders
+ merges `hooks.json`, seeds/merges `settings.json`, copies aliases, calls
`render-agy-plugin.sh`, enables the plugin in `config.json`. At launch: `.zshrc`/`.bashrc`
source `~/.config/antigravity/aliases.sh` → `agy()` → `agy_launch_flags` → `command agy`.

## 4. Behavior / features

- **F1 launch config** — `agy_launch_flags <tty|other> [args…]` fills `AGY_LAUNCH_FLAGS`;
  `agy` anchors tmux then execs `command agy "${AGY_LAUNCH_FLAGS[@]}" "$@"`; `agy-config`
  has `status` (default), `yolo on|off`, `doctor`; unknown verbs exit 2. `agy-yolo` is
  removed.
- **F2 settings seed** — template copied only when the settings file is absent.
- **F3 forced policy** — `permissions.deny`/`ask` replaced from the repo; `permissions.allow`
  unioned; `statusLine` replaced; every other host key preserved.
- **F4 hooks merge** — rendered `guards` entry merged over the existing file; other named
  hooks preserved; output stays valid JSON with no `__HOME__` left.
- **F5 sensitive-root ask** — file tools whose target is under `~/.ssh`, `~/.aws`, `~/.gnupg`,
  `~/.config/gss`, `~/.kube`, `~/.docker` get `ask`; guard `deny` still wins; `run_command`
  is unaffected.
- **F6 plugin render** — deterministic output from the repo sources; idempotent; removed
  sources drop their TOML on the next render.
- **F7 plugin enable** — `config.json` created if absent, `plugins.dotfiles.enabled = true`
  set, other keys untouched.
- **F8 docs + sanity** — `ai/antigravity/AGENTS.md`, `docs/machine-local-overrides.md`,
  `ai/AGENTS.md` describe the new surface; `ai/antigravity/scripts/sanity_check.sh` asserts
  the seeded keys and the plugin dir.

## 5. Evaluation criteria (per feature)

| Feature | Trigger predicate | Fires | Must not fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1 | `agy` invoked from an interactive shell | yolo sentinel present → `--dangerously-skip-permissions` injected | no sentinel → zero flags | prompt text contains `-p` is not print mode; `-p`/`--print`/`--prompt` as an argv token is | `aliases_test.sh` all PASS, no `_`-prefixed helper, `agy-yolo` absent |
| F2 | settings file absent at install | seeded with the template keys | file present → template not applied | template `_comment` key stripped by the merge | UC-3 and UC-4 assertions |
| F3 | every install run | deny/ask equal repo lists; allow ⊇ repo list | host allow entries never dropped | host had deny entries → replaced (policy is immutable) | installer test |
| F4 | every install run | `guards` rendered; other keys kept | never drops a foreign named hook | absent file → created; invalid JSON existing file → renamed `.invalid` and recreated | installer test + `jq -e .` |
| F5 | file tool with a `TargetFile` under a sensitive root | `ask` with reason | `run_command`, or a workspace path → unaffected | `~`-prefixed target expanded; deny from a guard precedes ask | adapter test |
| F6 | `render-agy-plugin.sh` run | one TOML per command, rules file from account memories | no `$ARGUMENTS`, no `` !` `` lines in output | zero commands → empty `commands/`, still valid `plugin.json` | renderer test |
| F7 | every install run | `plugins.dotfiles.enabled == true` | never edits other plugin entries | `config.json` absent → created as `{"plugins":{…}}` | installer test |
| F8 | docs build / sanity run | sanity asserts keys + plugin dir | — | sanity skips cleanly when agy settings absent | `make lint-shell`, `make lint-portability`, sanity transcript |

## 6. Verification harness

- **Automated:** `make shell-test` discovers `ai/antigravity/aliases_test.sh`,
  `ai/hooks/antigravity_adapter_test.sh`, `opt/scripts/system/install_antigravity_skills_test.sh`,
  `opt/scripts/system/render-agy-plugin_test.sh` (all on `ai/_test_helpers.sh`). Gates:
  `make lint-shell`, `make lint-portability` (strict). Every task's gate output is `tee`'d to
  `docs/mbo/plans/agy-parity/evidence/<unit>/`.
- **Live host (human-visible evidence):** run `install_antigravity_skills.sh` from the checkout
  (copy-forward only, no symlinks, so safe from a worktree), then capture `agy-config status`,
  `jq` of the live settings, `jq keys` of `hooks.json`, and `ls` of the plugin dir. A bounded
  `agy -p` probe records how `permissions.deny` behaves for a `command(...)` prefix.

## 7. Prerequisites / dependencies

`jq`, `bash` ≥ 3.2, `python3` (renderer test validates TOML with `tomllib` when available,
falls back to grep), `agy` 1.1.25 on the live host for the transcript only. Design #269
approved.

## 8. Out of scope (and why)

Remote control for agy (no CLI flag exists); Claude-side fixes noted in the design; agy
version pinning; agy's `knowledge/`/`brain/` stores; verifying `agy agents` team discovery
(it belongs to the teams objective and printed nothing under a pseudo-TTY during analysis).

## 9. Rollback

Revert the build commits and re-run `install.sh`: aliases and `hooks.json` are re-rendered,
the forced subset reverts to statusLine-only, the plugin dir is removed by the installer when
its renderer is absent. Template-seeded keys remain in the host-owned settings file (harmless;
`settings.json.bak` holds the pre-migration copy).

> Produced from the approved design via the `mbo-plan` skill. The matching plan is
> `../plans/agy-parity.md`.
