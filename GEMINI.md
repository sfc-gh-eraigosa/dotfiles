# Dotfiles Repository

Welcome to the dotfiles repository. This file serves as the entry point for agent-based discovery of the tools, configurations, and workflows contained within this repo. It is read by both **Gemini CLI** (as `GEMINI.md`) and **Claude Code** (as `CLAUDE.md` — a symlink to this file).

## Repository Structure

- `opt/bin/`: A collection of utility scripts and binaries. [See opt/bin/GEMINI.md](./opt/bin/GEMINI.md) for a categorized registry of these tools.
- `opt/profiles/`: Shell configuration files (.zshrc, .bashrc, .tmux.conf, etc.). [See opt/profiles/GEMINI.md](./opt/profiles/GEMINI.md) for details.
- `opt/docs/`: Legacy and reference documentation for various tools and setups. [See opt/docs/GEMINI.md](./opt/docs/GEMINI.md).
- `src/`: Non-Go custom tools and agent skills (shell/skill tooling). **Go code no longer lives here** — all Go modules live under `sdk/`. [See src/GEMINI.md](./src/GEMINI.md).
- `sdk/`: Go modules (`gss`, `gsl`, `wol`, `tmux-mgr`), each independently `go install`-able as `github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>`. **All Go code lives here, not under `src/`.** (Cutover in progress — see [docs/mbo/plans/2026-06-04-sdk-migration-plan.md](./docs/mbo/plans/2026-06-04-sdk-migration-plan.md).)
- `opt/Desktop/Apps/scripts/`: Windows-side automation deployed to the Desktop (macOS-style hotkeys + Wispr Flow voice dictation in `macos.ahk`, PowerToys/app/font setup). [See opt/Desktop/Apps/scripts/GEMINI.md](./opt/Desktop/Apps/scripts/GEMINI.md) for the inventory, the WSL→Windows dev loop, and AutoHotkey v2 gotchas.
- `archive/`: Retired-but-kept artifacts (not wired into install). [See archive/GEMINI.md](./archive/GEMINI.md) for the inventory and restore instructions.
- `ai/hooks/`: Unified agent hooks (safety, privacy) shared across CLIs.
- `ai/skills/`: Shared agent skills (each a `SKILL.md` folder) linked into Claude + Gemini by `sync-skills`; each should ship an `evals/evals.json` validated by `make skill-evals`. [See ai/skills/GEMINI.md](./ai/skills/GEMINI.md).
- `ai/gemini/`: Gemini-specific commands, TOML policies, and settings.
- `ai/claude/`: Claude-specific commands, settings, and hook templates.
- `ai/plugins.yaml`: Declarative manifest of the Claude Code plugins this repo installs/enables (ensure-only via `sync-plugins`). See [docs/ai-plugins.md](./docs/ai-plugins.md) for the plugin summary, first-usage examples, and the Gemini-extension path.
- `ai/mcp.yaml`: Declarative manifest of standalone **MCP servers** (e.g. NotebookLM) this repo registers for Claude + Gemini (ensure-only via `sync-mcp.sh`; a separate concern from plugins — different CLI verbs, config files, idempotency). stdio-only, pinned versions, registration-only (never runs browser auth). See [docs/ai-mcp.md](./docs/ai-mcp.md).
- `ai/teams/`: Specialized agent teams installed as native subagents for Claude, Gemini, Antigravity, and Ollama. Personas declare an abstract `tier:` resolved via `model-map.yaml`; `install_ai_teams.sh` emits each tool's format and `/team` routes tasks to the right team/member. [See ai/teams/GEMINI.md](./ai/teams/GEMINI.md).
- `docs/`: Repository documentation. **Objective-driven design work — issues, `gss` draft PRs, new features/skills/CLIs/services — starts in [`docs/mbo/`](./docs/mbo/GEMINI.md)**: the Management-By-Objective `design → spec → plan` pipeline, per-task skill routing, and the objective tracker (`docs/mbo/index.md`). [See docs/GEMINI.md](./docs/GEMINI.md).

## Usage Guidelines

- **Tool Discovery**: Check `opt/scripts/GEMINI.md` for available shell scripts.
- **Configuration**: Shell profiles and aliases are maintained in `opt/profiles/`.
- **Progressive Loading**: Only read subdirectory `GEMINI.md` (or `CLAUDE.md` — same file) when specifically needing information about that section, to conserve context.
- **GEMINI.md + CLAUDE.md in every documented directory**: Whenever a new directory is added that contains tools, scripts, or documentation worth describing to AI agents, create a `GEMINI.md` in that directory and a `CLAUDE.md -> GEMINI.md` symlink alongside it (`ln -s GEMINI.md CLAUDE.md`). This ensures both Gemini CLI and Claude Code can navigate the repo from any subdirectory. The symlink keeps both agents in sync from a single source file. Add a link to the new `GEMINI.md` in this root file's Repository Structure section.
- **Skills are shared**: any `SKILL.md` under `src/` (e.g. `src/ssh-host-finder/SKILL.md`, `src/wispr-flow-debug/SKILL.md`, or a tool's `src/<tool>/skill/`) drives both assistants — `sync-skills` discovers every `SKILL.md` and links it into `~/.claude/skills` and `~/.agents/skills`. Edit once, benefit twice.
- **Skills should ship evals**: a skill folder should carry an `evals/evals.json` (the `skill-creator` format — `{ "skill_name", "evals": [ { "id", "prompt", "expected_output" } ] }`) capturing its trigger/behavior cases. `make skill-evals` (→ `opt/scripts/system/skill-eval.sh --check`) deterministically validates every skill's corpus and reports a **SKIP** for any skill folder that has none — no model calls, CI-safe. Behavioral grading (with-skill vs baseline accuracy) is the on-demand `skill-creator` loop, not this gate. See [`ai/skills/GEMINI.md`](./ai/skills/GEMINI.md).

## Asking the User (blocking questions must be colorized)

- **Surface every must-answer question through the interactive prompt, not plain prose.** Whenever you need a decision from the user *before you can proceed* — a genuine fork you can't resolve from the request, the code, or a sensible default — present it via the assistant's interactive question tool so it renders as a **distinct, colorized prompt** that's impossible to miss in a wall of output. In Claude Code that is the **`AskUserQuestion`** tool; in Gemini CLI / other harnesses use the equivalent interactive confirmation/elicitation tool.
- **Give real options.** List 2–4 concrete, mutually-exclusive choices; put the recommended one first and label it `(Recommended)`. The user can always pick "Other".
- **Graceful fallback when no interactive tool exists** (e.g. a headless or pipe-driven run): format the question as a visually distinct block so it still stands out — a blockquote led by a bold marker, e.g. `> ⚠️ **NEEDS YOUR INPUT:** …` — never a sentence buried mid-paragraph.
- **Don't overuse it.** Skip the prompt for trivial choices with an obvious default or facts you can verify yourself — pick the sensible option, state it in passing, and proceed. Reserve the colorized prompt for decisions whose answer actually changes what you do next.
- This generalizes the mandatory-confirmation rule under **Git Workflow** below (which already routes `git add` / `commit` / `gss push` / `gss pr` through `AskUserQuestion`): that gate is one instance of this broader convention.

## Portability & Best Practices

- **Shell Portability Standard (read before writing any `.sh`)**: All shell scripts and sourced
  profile fragments MUST follow [docs/mbo/specs/shell-portability.md](./docs/mbo/specs/shell-portability.md) —
  the normative contract for working identically across **WSL2-Ubuntu, macOS (zsh + BSD coreutils + bash 3.2),
  and Linux (Raspberry Pi / Jetson Nano)**. It covers shebang policy, banned zsh-isms (`read -A`), BSD-vs-GNU
  coreutil traps (`sed -i`, `stat`, `date`, …), the mandatory `eval "$(tool init)"` PATH-clobber guard (the
  bug that broke macOS `install.sh`), and a per-script checklist. CI enforces the mechanical parts via
  `make lint-shell` and the `shell-lint` workflow; review enforces the rest.
- **Cross-shell portability gate is ENFORCING — run `make lint-portability` before pushing any shell change.**
  `opt/scripts/system/shell-portability-scan.sh --strict` runs in the `shell-lint` workflow and **fails CI**
  on any Tier 1 (dash `/bin/sh` parse breakage — the class that caused the Raspberry Pi GUI **login loop**)
  or Tier 2 (macOS BSD-coreutil / bash-3.2 hazard) finding. It catches what `shellcheck` and `bash -n` miss
  (both pass bashisms). When writing or editing a script, conform to these rules so you don't trip the gate:
  - **Shebang:** executable scripts use `#!/usr/bin/env bash` (never `#!/bin/bash` — that's the macOS 3.2
    relic; never `#!/bin/sh` unless the script is genuinely POSIX). Sourced fragments use `# shellcheck shell=bash`.
  - **Anything dash sources at login is POSIX-only:** `~/.profile`, `~/.xsessionrc`, and every file they
    unconditionally dot-source (e.g. `~/.nano_profile`). Use `name() { … }` (never `function name { … }`),
    no `[[ … ]]`, no arrays — a parse error here aborts the LightDM Xsession and bounces you to the greeter.
  - **Use portable tools:** `command -v` not `which`; `grep -E` not `grep -P`/`\K`; `cd -- "$(dirname "$0")" && pwd -P`
    not `readlink -f`; `sed -i.bak … && rm -f ….bak` not bare `sed -i`; probe `stat -f` then `stat -c`.
  - **Bash-4 features** (`declare -A`, `mapfile`, `${v,,}`) break macOS system bash 3.2 — rewrite them, or
    require bash 4+ via `#!/usr/bin/env bash` (picks up Homebrew bash 5 on macOS) and document it.
  - **Opt-out for a reviewed exception:** add a trailing `# portability-ok: <reason>` comment on the line and
    the scanner skips it. Use sparingly and always with a reason.
- **Use $HOME**: Always use `${HOME}` or `~` instead of absolute home paths (e.g., `/home/wenlock` or `/Users/eraigosa`) in scripts, aliases, and configuration files to ensure they are portable across different systems and users.
- **Avoid Hardcoded Usernames**: Never hardcode usernames in paths or instructions; use environment variables like `$USER` if needed.
- **Avoid Hardcoded Paths**: Use relative paths or environment variables (like `BASE_DIR` in `install.sh`) whenever possible.

## Shell & Dotfiles Conventions

- **Minimal alias surface**: prefer ONE canonical alias per workflow. Don't propose variants (`foo-c`, `foo-r`, `foo-status`) up front — add them only when asked. The shorter the alias surface, the easier the dotfiles are to memorize and audit.
- **Self-contained shell config**: `.bashrc` / `.zshrc` / sourced fragments must resolve paths from their own location (or `$HOME` / `$DOTFILES_DIR` env vars), never from hardcoded `$HOME/git/dotfiles`. The repo can be cloned anywhere and the config must still work.
- **One install path**: when adding shell config, wire it through `install.sh` so a fresh clone bootstraps cleanly. Don't rely on the user manually symlinking anything you create.
- **Worktree safety & `install.sh` (critical)**: NEVER run `install.sh` from a `gss feature` worktree. The script creates absolute symlinks in your `$HOME` (e.g., `~/.zshrc -> .../dotfiles/opt/profiles/zshrc`). Running it from a worktree will link your global configuration to a transient, task-specific path. Always switch to the main repository (`~/git/dotfiles`), checkout the desired branch, and run `install.sh` from there to ensure your system remains in a predictable, stable state.
- **`install.sh` is interactive — never run it non-interactively or backgrounded**: on Windows/WSL it deploys `opt/Desktop/*` and then **prompts** ("Windows Desktop Customization detected … [y/n/s]") before running the PowerShell setup (Terminal themes, winget apps, the elevated AHK autostart task). With no TTY (a backgrounded run, or piping through `tail`/redirection) that prompt blocks forever or is silently skipped, so the customization step never runs. Run it in a real terminal and answer the prompt. The earlier file deploy (e.g. the fixed `macos.ahk`) happens *before* the prompt, so a skipped prompt still deploys files — it only skips the PowerShell setup/AHK reload.
- **AI-tool config provisioning — copy into well-known `$HOME` paths; no new symlinks**: There are two ways the repo populates `$HOME` to configure the AI tools we support (`~/.claude`, `~/.gemini`, etc.): (1) **copy** a file into a well-known path under `$HOME`, or (2) a **symlink** into `$HOME` that references `git/dotfiles`. **Copy is the forward mechanism.** Symlinks are **legacy** — still present and allowed, but **do not introduce new ones**; they are to be retired over time. Settings/config files must reference well-known `$HOME` paths (e.g. `~/.claude/hooks/safety_guard.sh`) and **must not** reference repo-internal paths like `$HOME/git/dotfiles/ai/hooks/...` (which couples global config to the checkout location and breaks on worktrees/CI). Host-local settings *values* are preserved by a forced-field merge on install (immutable fields like hook wiring are re-applied from the repo; undeclared host fields are left intact) — see [docs/mbo/designs/2026-06-02-ai-config-home-provisioning.md](./docs/mbo/designs/2026-06-02-ai-config-home-provisioning.md).

## Git Workflow

- The **git-safe-sync (gss)** skill is the canonical commit + push path. Use the `/sync` slash command for the common case (commit + push to main) — it scaffolds the introspect → propose → confirm → execute flow.
- **Confirmation is mandatory** regardless of the user phrasing ("sync", "push", "commit it"): always present options via `AskUserQuestion` before any `git add` / `git commit` / `gss push` / `gss pr`. The gss skill's "Mandatory Confirmation" rule overrides any autonomous-mode preference.
- **Two-call recipe for gss push**: the `safety_guard.sh` hook intentionally blocks chained `mkdir + token + gss push` in a single Bash call. Always issue the token-generation line as one Bash call and `gss push` (or `gss pr` / `gss sync`) as a separate second call.
- **Push the branch before creating a PR (no double prompt)**: a PR can only be opened once its branch exists on origin. `gss push` now handles the **first push of a brand-new branch** — it detects a missing `origin/<branch>` and creates it with `--set-upstream` instead of failing on the rebase step (the old `sync: … couldn't find remote ref <branch>` error burned the approval token and forced a *second* confirmation). So the single token → `gss push` recipe works for a new branch; do not pre-push or regenerate the token. If you are on an **older gss binary** that still errors that way, fall back to one plain `git push -u origin HEAD` (a plain push is not token-gated), verify with `git ls-remote --heads origin <branch>`, then open the PR.
- **Group related changes**: prefer one cohesive commit per logical unit. Stage files by explicit name (never `git add -A` / `git add .`) to keep blast radius tight and avoid sweeping in unrelated dirty state.
- **`.gitignore` allowlist pattern (critical — read first)**: this repo's `.gitignore` starts with `*`, which blocks **every path** by default. Files become visible to git only when an explicit `!`-rule opts them back in. The consequence is non-obvious and bites new contributors (human and agent): if you create `sdk/gss/docs/plan.md` and don't see it in `git status`, the file is **not lost** — it's been silently ignored by the default `*` because no `!`-rule covers its path yet. This is also why a freshly-created top-level folder (say `~/git/dotfiles/notes/`) won't show up in `git status` at all — git treats it as ignored, not as untracked, and skips it entirely.
  - **Whenever you add a new file or directory**: first check whether the path is covered by an existing `!`-rule. Top-level paths already opted in include `!src/**`, `!opt/**`, `!ai/**`, `!system/**`, `!docs/**`, `!.config/**`, `!.github/**`, `!.devcontainer/**`, `!scripts/**`, plus per-file allowlists for `README.md`, `CONTRIBUTING.md`, `Makefile`, `install.sh`, `LICENSE`, `Dockerfile`, `azure-pipelines.yml`, `**/GEMINI.md`, `**/CLAUDE.md`. Anything outside those trees is invisible by default.
  - **Verify, don't assume.** After creating a new file, run `git status --short -- <path>` and `git check-ignore -v <path>`. If the file does not appear in `git status`, add an `!`-rule to `.gitignore` **before** attempting to stage. Never paper over this with `git add -f` — forced adds bypass the policy and leave the next contributor confused about whether the path is supposed to be tracked.
  - **When in doubt, opt in explicitly.** Prefer narrow rules (`!sdk/gss/**`) over broad ones (`!**/*.md`). Document any path that is *intentionally* local-only with an inline `.gitignore` comment explaining why (examples: `opt/.DS_Store`, `ai/claude/settings.json`, `sdk/tmux-mgr/tmux-mgr` — each has a comment line describing the reason).
  - **Worked example for `sdk/gss/docs/`**: the design document is stage-able only because `!src/**` (lines 25–26 of `.gitignore`) opts in the entire `src/` tree. If we ever moved gss docs out of `src/` (e.g. to a top-level `design/` folder), the new path would be invisible until we added `!design/` and `!design/**` rules. Conversely, anything we add *under* `src/` is auto-tracked — no per-file rule needed. The same logic explains why `sdk/gss/docs/design.md` shows in `git status` today: it's covered by `!src/**`, not by a per-doc rule.
  - **Never rely on "it's already covered by `*`"** as a substitute for a documented decision. A path is either explicitly tracked or explicitly local-only; ambiguity is a bug.
  - **Note on `docs/` at the repo root**: `!docs/` and `!docs/**` are in `.gitignore` so the top-level `docs/` tree, if/when it appears, is tracked. This is distinct from `sdk/gss/docs/` (covered by `!src/**`) and from `opt/docs/` (covered by `!opt/**`).

## Hook & Regex Safety

- `ai/hooks/safety_guard.sh` is a PreToolUse hook with regex-based deny rules. Its companion test driver is `ai/hooks/safety_guard_test.sh`.
- **When editing the hook**: extend `safety_guard_test.sh` first. Add at least one new `assert_exit 0` case proving a legitimate command of the same shape still passes, and one new `assert_exit 2` case proving the malicious shape is still blocked. Run the test driver and require all cases to pass before committing.
- **Beware bash regex line-spanning**: bash regex `.*` matches newlines and command separators (`;`, `|`, `&`). Use `${SAFE_CHARS}` (defined at the top of the hook as `[^[:cntrl:];|&]`) to scope a pattern to one shell-command segment.
- **Strip heredoc bodies before matching**: multi-line content (commit messages, README text) gets passed via heredocs and routinely contains literal dangerous patterns. Use `strip_heredocs.awk` to drop those bodies before regex evaluation.
