# AI-First Infrastructure

This directory serves as the control plane for the repository's **AI-assisted engineering environment**. It contains the configuration, safety policies, and declarative manifests that empower both **Claude Code** and **Antigravity CLI** (`agy`) to work effectively and safely in this workspace.

## 🛠️ Unified Capabilities

This repository bridges the gap between different AI assistants by providing a shared set of tools and rules:

### 🔄 Declarative Plugin Management (`ai/plugins.yaml`)
We manage AI assistant extensions (Claude Plugins and Antigravity plugins) as code. 
- **Source of Truth**: [`ai/plugins.yaml`](./plugins.yaml) defines which plugins are installed and enabled.
- **Auto-Provisioning**: `install.sh` automatically synchronizes these plugins using the `sync-plugins` engine.
- **Cross-Platform**: Every entry in the manifest can define both a Claude-native plugin and an Antigravity plugin source (`agy plugin install <source>`).
- **See also**: [docs/ai-plugins.md](../docs/ai-plugins.md) for the full catalog and mapping.

### 🧠 Shared Skills (`ai/skills/` & `src/`)
Agents in this repo are trained with specialized "skills" — instructions for complex workflows like Git management (`gss`), Tmux orchestration (`tmux-mgr`), and SSH discovery.
- **Architecture**: Skills are authored as `SKILL.md` files once.
- **Linking**: The `sync-skills` command symlinks these files into the native skill directories for both Claude (`~/.claude/skills`) and Antigravity (`~/.gemini/config/skills`).
- **Discovery**: Check the `skills/` subdirectory (or `~/.gemini/config/skills`) to see what's available.

### 🛡️ Safety & Policies
We enforce identical security boundaries across all assistants:
- **Shared Hooks**: Controlled via PreToolUse/BeforeTool hooks in `ai/hooks/`.
  - `safety_guard.sh`: Blocks dangerous destructive commands (e.g., `rm -rf /`) and mandates user confirmation for sensitive Git actions (`gss push`).
  - `privacy_guard.sh`: Blocks leaking home paths, usernames, hostnames, email addresses, and secrets into content that will be shared. Judges Write/Edit into tracked-able files, Bash that writes a tracked file by any means (heredoc, `>`, `tee`, `sed -i`, interpreter one-liners), and publishing verbs by **what they publish** — `git commit` (the staged additions + `-F` file), `git push` / `gss push|pr|sync` / `gss feature checkpoint` (the outgoing commits), `gh pr|issue|release` (inline body + `--body-file`/`--notes-file`), `gh gist create` (the files). Fails closed without `jq`; logs every deny to `~/.local/state/privacy_guard/blocks.log`. Mandates placeholders (`$HOME`, `~`, `${USER}`, `<user>`, `<host>`, `<email>`, `<REDACTED>`).
  - `privacy_rules.sh`: the ONE rule set (identity tokens incl. git `user.email` and username-as-prefix, email shape, secret shapes, URL credentials) shared by `privacy_guard.sh` and the git hooks below, so the two layers cannot drift. Per-machine tuning in `~/.config/privacy_guard/`: `identity` (extra tokens to refuse), `allow` (tokens to never refuse).
- **Git hooks** (`ai/githooks/`, installed globally via `core.hooksPath` by `opt/scripts/git/install_git_hooks.sh`): `pre-commit` judges the staged additions, `commit-msg` the message (`-m`, `-F`, or editor), `pre-push` the commits the remote lacks. This is the layer that catches what no agent hook can see — content that reached the tree without a tool call, and every `gss`/`gh` path, since they all end in `git push`. They chain to any repo-local `.git/hooks/<name>`. `PRIVACY_GUARD_SKIP=1` is the loud, explicit bypass for a reviewed exception. Suites: `make hook-test` (CI-strict).
- **Antigravity Wiring**: The same guards run under `agy` via `ai/hooks/antigravity_adapter.sh`, wired through `~/.gemini/config/hooks.json` (the repo's `guards` entry, rendered from `ai/antigravity/hooks.json.template` and merged so other tools' hooks survive). The deny/ask lists are also enforced in agy's own `settings.json` via `ai/antigravity/settings.forced.json`.
- **Startup parity**: both assistants get the same launch contract — `claude-config` / `agy-config` sentinel launch flags, a seeded settings template, a forced policy subset, and the repo slash commands + account memories (Claude: `~/.claude/commands` + memory provisioning; agy: the rendered local `dotfiles` plugin). See `docs/mbo/designs/agy-parity.md`.

## 📂 Directory Structure

| Path | Purpose |
| :--- | :--- |
| [`hooks/`](./hooks/) | Unified agent hooks (safety, privacy) shared across CLIs. |
| [`claude/`](./claude/) | Claude Code settings, slash commands, and hook templates. |
| [`antigravity/`](./antigravity/) | Antigravity CLI aliases, hooks template, and sanity checks. |
| [`skills/`](./skills/) | Repository-wide agent skills (linked via `sync-skills`). |
| [`plugins.yaml`](./plugins.yaml) | Declarative manifest for assistant extensions. |

## 🚀 Key Commands

- **`sync-plugins`**: Reads `plugins.yaml` and installs/enables missing extensions for the active assistant.
- **`sync-skills`**: Re-links all `SKILL.md` files from `src/` and `ai/skills/` to the assistant's runtime directories.
- **`install.sh`**: The root installer — calls both of the above as part of a fresh system bootstrap.

---
*This configuration ensures that no matter which assistant you choose, it has the same knowledge, follows the same rules, and uses the same toolset.*
