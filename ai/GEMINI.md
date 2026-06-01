# AI-First Infrastructure

This directory serves as the control plane for the repository's **AI-assisted engineering environment**. It contains the configuration, safety policies, and declarative manifests that empower both **Claude Code** and **Gemini CLI** to work effectively and safely in this workspace.

## 🛠️ Unified Capabilities

This repository bridges the gap between different AI assistants by providing a shared set of tools and rules:

### 🔄 Declarative Plugin Management (`ai/plugins.yaml`)
We manage AI assistant extensions (Claude Plugins and Gemini Extensions) as code. 
- **Source of Truth**: [`ai/plugins.yaml`](./plugins.yaml) defines which plugins are installed and enabled.
- **Auto-Provisioning**: `install.sh` automatically synchronizes these plugins using the `sync-plugins` engine.
- **Cross-Platform**: Every entry in the manifest can define both a Claude-native plugin and a Gemini-native extension source (git URL).
- **See also**: [docs/ai-plugins.md](../docs/ai-plugins.md) for the full catalog and mapping.

### 🧠 Shared Skills (`ai/skills/` & `src/`)
Agents in this repo are trained with specialized "skills" — instructions for complex workflows like Git management (`gss`), Tmux orchestration (`tmux-mgr`), and SSH discovery.
- **Architecture**: Skills are authored as `SKILL.md` files once.
- **Linking**: The `sync-skills` command symlinks these files into the native skill directories for both Claude (`~/.claude/skills`) and Gemini (`~/.agents/skills`).
- **Discovery**: Run `gemini skills list` or check the `skills/` subdirectory to see what's available.

### 🛡️ Safety & Policies
We enforce identical security boundaries across all assistants:
- **Claude**: Controlled via PreToolUse hooks in `ai/claude/hooks/` — `safety_guard.sh` (destructive-command guard) and `privacy_guard.sh` (blocks leaking home paths / usernames / hostnames / secrets into tracked files, PR/issue bodies, and commit messages).
- **Gemini**: Controlled via TOML policies in `ai/gemini/policies/`.
- **Rules**: Both block dangerous destructive commands (e.g., `rm -rf /`) and mandate user confirmation for sensitive Git actions (`gss push`). The privacy guard additionally requires a variable/placeholder (`$HOME`, `~`, `${USER}`, `<user>`, `<REDACTED>`) instead of literal identity or secrets in shared content.

## 📂 Directory Structure

| Path | Purpose |
| :--- | :--- |
| [`claude/`](./claude/) | Claude Code settings, slash commands, and hooks. |
| [`gemini/`](./gemini/) | Gemini CLI settings, custom commands, and policies. |
| [`skills/`](./skills/) | Repository-wide agent skills (linked via `sync-skills`). |
| [`plugins.yaml`](./plugins.yaml) | Declarative manifest for assistant extensions. |

## 🚀 Key Commands

- **`sync-plugins`**: Reads `plugins.yaml` and installs/enables missing extensions for the active assistant.
- **`sync-skills`**: Re-links all `SKILL.md` files from `src/` and `ai/skills/` to the assistant's runtime directories.
- **`install.sh`**: The root installer — calls both of the above as part of a fresh system bootstrap.

---
*This configuration ensures that no matter which assistant you choose, it has the same knowledge, follows the same rules, and uses the same toolset.*
