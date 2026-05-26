# Dotfiles Repository

Welcome to the dotfiles repository. This file serves as the entry point for agent-based discovery of the tools, configurations, and workflows contained within this repo. It is read by both **Gemini CLI** (as `GEMINI.md`) and **Claude Code** (as `CLAUDE.md` — a symlink to this file).

## Repository Structure

- `opt/bin/`: A collection of utility scripts and binaries. [See opt/bin/GEMINI.md](./opt/bin/GEMINI.md) for a categorized registry of these tools.
- `opt/profiles/`: Shell configuration files (.zshrc, .bashrc, .tmux.conf, etc.). [See opt/profiles/GEMINI.md](./opt/profiles/GEMINI.md) for details.
- `opt/docs/`: Legacy and reference documentation for various tools and setups. [See opt/docs/GEMINI.md](./opt/docs/GEMINI.md).
- `src/`: Source code for custom tools and agent skills. [See src/GEMINI.md](./src/GEMINI.md).
- `ai/gemini/`: Gemini-specific commands, TOML policies, and settings.
- `ai/claude/`: Claude-specific commands, settings, and the `safety_guard.sh` PreToolUse hook.
- `ai/plugins.yaml`: Declarative manifest of the Claude Code plugins this repo installs/enables (ensure-only via `sync-plugins`). See [docs/ai-plugins.md](./docs/ai-plugins.md) for the plugin summary, first-usage examples, and the Gemini-extension path.

## Usage Guidelines

- **Tool Discovery**: Check `opt/scripts/GEMINI.md` for available shell scripts.
- **Configuration**: Shell profiles and aliases are maintained in `opt/profiles/`.
- **Progressive Loading**: Only read subdirectory `GEMINI.md` (or `CLAUDE.md` — same file) when specifically needing information about that section, to conserve context.
- **Skills are shared**: `SKILL.md` files under `src/*/skill/` and `src/ssh-*/` drive both assistants. Edit once, benefit twice.

## Portability & Best Practices

- **Use $HOME**: Always use `${HOME}` or `~` instead of absolute home paths (e.g., `/home/wenlock` or `/Users/eraigosa`) in scripts, aliases, and configuration files to ensure they are portable across different systems and users.
- **Avoid Hardcoded Usernames**: Never hardcode usernames in paths or instructions; use environment variables like `$USER` if needed.
- **Avoid Hardcoded Paths**: Use relative paths or environment variables (like `BASE_DIR` in `install.sh`) whenever possible.

## Shell & Dotfiles Conventions

- **Minimal alias surface**: prefer ONE canonical alias per workflow. Don't propose variants (`foo-c`, `foo-r`, `foo-status`) up front — add them only when asked. The shorter the alias surface, the easier the dotfiles are to memorize and audit.
- **Self-contained shell config**: `.bashrc` / `.zshrc` / sourced fragments must resolve paths from their own location (or `$HOME` / `$DOTFILES_DIR` env vars), never from hardcoded `$HOME/git/dotfiles`. The repo can be cloned anywhere and the config must still work.
- **One install path**: when adding shell config, wire it through `install.sh` so a fresh clone bootstraps cleanly. Don't rely on the user manually symlinking anything you create.

## Git Workflow

- The **git-safe-sync (gss)** skill is the canonical commit + push path. Use the `/sync` slash command for the common case (commit + push to main) — it scaffolds the introspect → propose → confirm → execute flow.
- **Confirmation is mandatory** regardless of the user phrasing ("sync", "push", "commit it"): always present options via `AskUserQuestion` before any `git add` / `git commit` / `gss push` / `gss pr`. The gss skill's "Mandatory Confirmation" rule overrides any autonomous-mode preference.
- **Two-call recipe for gss push**: the `safety_guard.sh` hook intentionally blocks chained `mkdir + token + gss push` in a single Bash call. Always issue the token-generation line as one Bash call and `gss push` (or `gss pr` / `gss sync`) as a separate second call.
- **Group related changes**: prefer one cohesive commit per logical unit. Stage files by explicit name (never `git add -A` / `git add .`) to keep blast radius tight and avoid sweeping in unrelated dirty state.
- **`.gitignore` allowlist pattern (critical — read first)**: this repo's `.gitignore` starts with `*`, which blocks **every path** by default. Files become visible to git only when an explicit `!`-rule opts them back in. The consequence is non-obvious and bites new contributors (human and agent): if you create `src/gss/docs/plan.md` and don't see it in `git status`, the file is **not lost** — it's been silently ignored by the default `*` because no `!`-rule covers its path yet. This is also why a freshly-created top-level folder (say `~/git/dotfiles/notes/`) won't show up in `git status` at all — git treats it as ignored, not as untracked, and skips it entirely.
  - **Whenever you add a new file or directory**: first check whether the path is covered by an existing `!`-rule. Top-level paths already opted in include `!src/**`, `!opt/**`, `!ai/**`, `!system/**`, `!docs/**`, `!.config/**`, `!.github/**`, `!.devcontainer/**`, `!scripts/**`, plus per-file allowlists for `README.md`, `CONTRIBUTING.md`, `Makefile`, `install.sh`, `LICENSE`, `Dockerfile`, `azure-pipelines.yml`, `**/GEMINI.md`, `**/CLAUDE.md`. Anything outside those trees is invisible by default.
  - **Verify, don't assume.** After creating a new file, run `git status --short -- <path>` and `git check-ignore -v <path>`. If the file does not appear in `git status`, add an `!`-rule to `.gitignore` **before** attempting to stage. Never paper over this with `git add -f` — forced adds bypass the policy and leave the next contributor confused about whether the path is supposed to be tracked.
  - **When in doubt, opt in explicitly.** Prefer narrow rules (`!src/gss/**`) over broad ones (`!**/*.md`). Document any path that is *intentionally* local-only with an inline `.gitignore` comment explaining why (examples: `opt/.DS_Store`, `ai/claude/settings.json`, `src/tmux-mgr/tmux-mgr` — each has a comment line describing the reason).
  - **Worked example for `src/gss/docs/`**: the design document is stage-able only because `!src/**` (lines 25–26 of `.gitignore`) opts in the entire `src/` tree. If we ever moved gss docs out of `src/` (e.g. to a top-level `design/` folder), the new path would be invisible until we added `!design/` and `!design/**` rules. Conversely, anything we add *under* `src/` is auto-tracked — no per-file rule needed. The same logic explains why `src/gss/docs/design.md` shows in `git status` today: it's covered by `!src/**`, not by a per-doc rule.
  - **Never rely on "it's already covered by `*`"** as a substitute for a documented decision. A path is either explicitly tracked or explicitly local-only; ambiguity is a bug.
  - **Note on `docs/` at the repo root**: `!docs/` and `!docs/**` are in `.gitignore` so the top-level `docs/` tree, if/when it appears, is tracked. This is distinct from `src/gss/docs/` (covered by `!src/**`) and from `opt/docs/` (covered by `!opt/**`).

## Hook & Regex Safety

- `ai/claude/hooks/safety_guard.sh` is a PreToolUse hook with regex-based deny rules. Its companion test driver is `ai/claude/hooks/safety_guard_test.sh`.
- **When editing the hook**: extend `safety_guard_test.sh` first. Add at least one new `assert_exit 0` case proving a legitimate command of the same shape still passes, and one new `assert_exit 2` case proving the malicious shape is still blocked. Run the test driver and require all cases to pass before committing.
- **Beware bash regex line-spanning**: bash regex `.*` matches newlines and command separators (`;`, `|`, `&`). Use `${SAFE_CHARS}` (defined at the top of the hook as `[^[:cntrl:];|&]`) to scope a pattern to one shell-command segment.
- **Strip heredoc bodies before matching**: multi-line content (commit messages, README text) gets passed via heredocs and routinely contains literal dangerous patterns. Use `strip_heredocs.awk` to drop those bodies before regex evaluation.
