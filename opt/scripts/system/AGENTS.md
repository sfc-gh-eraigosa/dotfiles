# System & Environment Setup (opt/scripts/system)

This directory contains scripts for configuring the operating system, environment variables, and base development tools.

## Scripts

- `sync-skills.sh`: **Canonical skill linker for both assistants.** Discovers every `SKILL.md` (under `src/*/`, `sdk/*/`, `ai/skills/*`, `.agents/skills/*`) and links it into **both** `~/.gemini/config/skills` (Antigravity CLI) and `~/.claude/skills` (Claude Code). Exposed as the `sync-skills` alias; pass `--build` to also rebuild core binaries (`gss`, `tmux-mgr`, `wol`).
- `sync-plugins.sh`: **Ensure-only sync of AI-assistant plugins** from the `ai/plugins.yaml` manifest. Installs + enables the listed Claude plugins (via `claude plugin install`/`enable`) and installs the listed Antigravity plugins (via `agy plugin install`); never removes anything. Exposed as the `sync-plugins` alias; pass `--dry-run` to preview. Parsed with mikefarah `yq`. Companion test: `sync-plugins_test.sh`.
- `bump-sdk-version.sh`: **Conventional-commit SDK version bumper (issue #139).** For each `sdk/<tool>/` module (a dir with both `go.mod` and `VERSION`), computes the next semver from the conventional-commit subjects of commits touching the module's *source* since its last `sdk/<tool>/v<X.Y.Z>` tag (`feat:`→minor, `!`/`BREAKING CHANGE`→major, else patch). `--check` (default; `make sdk-bump`) reports needed bumps and exits 1 if any; `--write` applies them. Idempotent and excludes the `VERSION` file from the source-delta check so the CI auto-bump loop terminates; respects a manually pre-bumped VERSION; leaves untagged modules to the tagger. Drives `.github/workflows/sdk-auto-bump.yml`. Companion test: `bump-sdk-version_test.sh`.
- `antigravity_install.sh`: Installs the Antigravity CLI (`agy`, successor to the retired Gemini CLI) via Google's checksummed bootstrapper, removes the retired `@google/gemini-cli` npm package, and generates `~/.antigravity.profile`.
- `install_antigravity_skills.sh`: Antigravity-specific config (guard hooks into `~/.gemini/config/hooks/`, `hooks.json` rendering, aliases, legacy Gemini cleanup). Skill links are handled by `sync-skills.sh`. Companion test: `install_antigravity_skills_test.sh`.
- `claude_install.sh`: Installs (or updates) the Claude Code CLI binary.
- `install_claude_skills.sh`: Claude-specific config (settings.json, slash commands, hooks, aliases). Skill links are handled by `sync-skills.sh`. Also invokes `provision-claude-memory.sh`.
- `provision-claude-memory.sh`: **Account-scoped memory provisioner (issue #134).** Seeds the repo's `scope: account` Claude memories (`ai/claude/memory/*.md`) into this machine's live project-memory store (`~/.claude/projects/<computed-slug>/memory/` — the slug is derived per-machine via `pwd -P`). Seed-and-preserve: copies account files, **never** clobbers host-local memories, regenerates `MEMORY.md` from the union. Mirrors `apply-forced-settings.sh` (standalone + tested). Companion test: `provision-claude-memory_test.sh`. Design: `docs/mbo/designs/memory-provisioning.md`.
- `install_ai_teams.sh`: **Transforms `ai/teams/` personas into native agents for all three tools** (Claude `~/.claude/agents/teams/`, Antigravity `~/.config/antigravity/agents/`, Ollama Modelfiles). Resolves each persona's `tier:` via `ai/teams/model-map.yaml`, composes the system prompt from `_partials/`, and compiles a routing `description`. Idempotent; graceful-skip per tool. Exposed as the `sync-teams` alias (`--dry-run`/`--tool`). Validates first via `ai/teams/validate.sh`. Companion test: `install_ai_teams_test.sh`.
- `install_sops.sh`: Installs the `sops` secrets-management binary into `~/opt/bin` (Linux/WSL fetches the official release; macOS uses the Brewfile).
- `install_yq.sh`: Installs the mikefarah `yq` (YAML processor) binary into `~/opt/bin` (Linux/WSL fetches the official release; macOS uses the Brewfile/`packages.tsv`). The apt `yq` is the incompatible kislyuk Python variant, so it is intentionally not used. Needed by `sync-plugins.sh`.
- `setup_gh_apt_repo.sh`: Registers GitHub CLI's official apt repo + signing key so `gh` installs the latest upstream release instead of Ubuntu's stale `universe` version. Idempotent; called automatically by `pkg-install-apt` before it installs. Force a key/repo refresh with `GH_REPO_FORCE=1`.
- `nvm`: Helper for Node Version Manager setup.
- `google-cli-setup.sh`: Installs/updates gcloud, the Antigravity CLI (only when absent — `antigravity_install.sh` owns the install), and the `gws` Workspace CLI; `status` subcommand reports their health.
- `setup_jtop.sh`: Installs and configures jetson-stats (jtop) for NVIDIA Jetson devices.
- `terminal-theme.sh`: Configures terminal colors and themes.
- `perf-toggle.sh`: Toggles system performance modes (e.g., on Jetson).
- `enable-vmx.sh`: Helper to check/enable virtualization support.
- `coco_install.sh`: Installer for the COCO dataset tools or similar.
- `crouton-alias.sh`: Aliases for Crouton (Chromebook) environments.
- `retire-ahk-voice-macro.sh`: **WSL migration helper.** Removes the old AutoHotkey Copilot-key voice macro from a machine's deployed `macos.ahk` (backs it up locally, re-deploys the cleaned copy, restarts AutoHotkey). One-off cleanup for hosts provisioned before voice dictation moved to Wispr Flow. Idempotent; `--dry-run` to preview. See `opt/Desktop/Apps/scripts/WISPR-FLOW.md`.

## Environment Profile

The `antigravity_install.sh` script generates `~/.antigravity.profile`, which is sourced by your shell to provide the Antigravity environment (agy PATH, opt/scripts discovery, tmux helpers). The legacy `~/.gemini.profile` is removed by the same script.
