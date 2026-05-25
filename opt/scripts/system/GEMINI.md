# System & Environment Setup (opt/scripts/system)

This directory contains scripts for configuring the operating system, environment variables, and base development tools.

## Scripts

- `sync-skills.sh`: **Canonical skill linker for both assistants.** Discovers every `SKILL.md` (under `src/*/`, `ai/skills/*`, `.gemini/skills/*`) and links it into **both** `~/.agents/skills` (Gemini CLI) and `~/.claude/skills` (Claude Code). Exposed as the `sync-skills` alias; pass `--build` to also rebuild core binaries (`gss`, `tmux-mgr`, `wol`).
- `gemini_install.sh`: Installs and configures the Gemini CLI and environment.
- `install_gemini_skills.sh`: Gemini-specific config (policies, commands, aliases). Skill links are handled by `sync-skills.sh`.
- `claude_install.sh`: Installs (or updates) the Claude Code CLI binary.
- `install_claude_skills.sh`: Claude-specific config (settings.json, slash commands, hooks, aliases). Skill links are handled by `sync-skills.sh`.
- `install_sops.sh`: Installs the `sops` secrets-management binary into `~/opt/bin` (Linux/WSL fetches the official release; macOS uses the Brewfile).
- `setup_gh_apt_repo.sh`: Registers GitHub CLI's official apt repo + signing key so `gh` installs the latest upstream release instead of Ubuntu's stale `universe` version. Idempotent; called automatically by `pkg-install-apt` before it installs. Force a key/repo refresh with `GH_REPO_FORCE=1`.
- `nvm`: Helper for Node Version Manager setup.
- `google-cli-setup.sh`: Configures the Google Cloud SDK.
- `setup_jtop.sh`: Installs and configures jetson-stats (jtop) for NVIDIA Jetson devices.
- `terminal-theme.sh`: Configures terminal colors and themes.
- `perf-toggle.sh`: Toggles system performance modes (e.g., on Jetson).
- `enable-vmx.sh`: Helper to check/enable virtualization support.
- `coco_install.sh`: Installer for the COCO dataset tools or similar.
- `crouton-alias.sh`: Aliases for Crouton (Chromebook) environments.

## Environment Profile

The `gemini_install.sh` script generates `~/.gemini.profile`, which is sourced by your shell to provide Gemini-specific aliases and functions.
