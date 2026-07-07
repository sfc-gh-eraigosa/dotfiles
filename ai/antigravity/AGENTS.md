# Antigravity CLI (agy) Configuration

Assistant-specific config for the Antigravity CLI — Google's successor to the
retired Gemini CLI (EOL 2026-06-18). Everything here is provisioned into the
home directory by `opt/scripts/system/install_antigravity_skills.sh`; shared
skills are linked separately by `sync-skills.sh`.

## Layout

| File | Purpose |
| ---- | ------- |
| `aliases.sh` | `agy()` / `agy-yolo()` shell wrappers with tmux auto-anchor. **Copied** to `~/.config/antigravity/aliases.sh` (copy-forward provisioning; no repo-pointing symlinks). Test: `aliases_test.sh`. |
| `hooks.json.template` | Hook wiring rendered to `~/.gemini/config/hooks.json` (`__HOME__` substituted). One entry runs both shared guards through `ai/hooks/antigravity_adapter.sh`, which translates agy's `{toolCall}`/`{decision}` hook dialect. |
| `scripts/sanity_check.sh` | Container/CI sanity check: PATH discovery, binaries, profile files, and live hook wiring (via `ai/claude/scripts/validate_hooks.sh`). |

## Where agy reads config (verified against agy 1.0.16)

- CLI settings (agy-owned, not managed here): `~/.gemini/antigravity-cli/settings.json`
- Global customization root: `~/.gemini/config/` — `skills/` (from sync-skills),
  `hooks/` + `hooks.json` (from the installer), `mcp_config.json`
- Workspace customization root: `.agents/` (`skills/`, `plugins/`, `rules/*.md`)
- Context files: `AGENTS.md` (this repo's convention) and legacy `GEMINI.md`

## Safety model

The shared guards in [`../hooks/`](../hooks/) are the single rule set for both
assistants. For agy, `antigravity_adapter.sh` maps guard verdicts to hook
decisions: exit 0 → `allow` (auto-approve — mirrors the retired
trusted-tools.toml tier), exit 2 → `deny`, exit 3 → `ask` (confirmation tier:
power-state commands, force pushes). Misconfiguration degrades to `ask`,
never to silent allow.
