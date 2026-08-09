# Dotfiles Repository

Welcome to the dotfiles repository. This file serves as the entry point for agent-based discovery of the tools, configurations, and workflows contained within this repo. It is read by both **Antigravity CLI** (`agy`, as `AGENTS.md`) and **Claude Code** (as `CLAUDE.md` — a symlink to this file).

## Repository Structure

- `opt/bin/`: A collection of utility scripts and binaries. [See opt/bin/AGENTS.md](./opt/bin/AGENTS.md) for a categorized registry of these tools.
- `opt/profiles/`: Shell configuration files (.zshrc, .bashrc, .tmux.conf, etc.). [See opt/profiles/AGENTS.md](./opt/profiles/AGENTS.md) for details.
- `opt/docs/`: Legacy and reference documentation for various tools and setups. [See opt/docs/AGENTS.md](./opt/docs/AGENTS.md).
- `src/`: Non-Go custom tools and agent skills (shell/skill tooling). **Go code no longer lives here** — all Go modules live under `sdk/`. [See src/AGENTS.md](./src/AGENTS.md).
- `sdk/`: Go modules (`gss`, `gsl`, `wol`, `tmux-mgr`, `gff`), each independently `go install`-able as `github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>`. **All Go code lives here, not under `src/`.** (Cutover in progress — see [docs/mbo/plans/2026-06-04-sdk-migration-plan.md](./docs/mbo/plans/2026-06-04-sdk-migration-plan.md).)
- `sdk/gff/`: **git fast features** — a git-persisted, layered feature-flag engine (proto schema, 5-layer resolver with provenance, cobra CLI, Go SDK) gating `install.sh` components. [See sdk/gff/AGENTS.md](./sdk/gff/AGENTS.md).
- `opt/Desktop/Apps/scripts/`: Windows-side automation deployed to the Desktop (macOS-style hotkeys + Wispr Flow voice dictation in `macos.ahk`, PowerToys/app/font setup). [See opt/Desktop/Apps/scripts/AGENTS.md](./opt/Desktop/Apps/scripts/AGENTS.md) for the inventory, the WSL→Windows dev loop, and AutoHotkey v2 gotchas.
- `archive/`: Retired-but-kept artifacts (not wired into install). [See archive/AGENTS.md](./archive/AGENTS.md) for the inventory and restore instructions.
- `ai/hooks/`: Unified agent hooks (safety, privacy) shared across CLIs.
- `ai/skills/`: Shared agent skills (each a `SKILL.md` folder) linked into Claude + Antigravity by `sync-skills`; each should ship an `evals/evals.json` validated by `make skill-evals`. [See ai/skills/AGENTS.md](./ai/skills/AGENTS.md).
- [`ai/antigravity/`](./ai/antigravity/AGENTS.md): Antigravity CLI (`agy`) aliases, hook-wiring template, and sanity-check scripts.
- `ai/claude/`: Claude-specific commands, settings, and hook templates.
- `ai/plugins.yaml`: Declarative manifest of the Claude Code plugins this repo installs/enables (ensure-only via `sync-plugins`). See [docs/ai-plugins.md](./docs/ai-plugins.md) for the plugin summary, first-usage examples, and the Antigravity plugin path.
- `ai/teams/`: Specialized agent teams installed as native subagents for Claude, Antigravity, and Ollama. Personas declare an abstract `tier:` resolved via `model-map.yaml`; `install_ai_teams.sh` emits each tool's format and `/team` routes tasks to the right team/member. [See ai/teams/AGENTS.md](./ai/teams/AGENTS.md).
- `docs/`: Repository documentation. **Objective-driven design work — issues, `gss` draft PRs, new features/skills/CLIs/services — starts in [`docs/mbo/`](./docs/mbo/AGENTS.md)**: the Management-By-Objective `design → spec → plan` pipeline, per-task skill routing, and the objective tracker (`docs/mbo/index.md`). [See docs/AGENTS.md](./docs/AGENTS.md).

## Usage Guidelines

- **Tool Discovery**: Check `opt/scripts/AGENTS.md` for available shell scripts.
- **Configuration**: Shell profiles and aliases are maintained in `opt/profiles/`.
- **Progressive Loading**: Only read subdirectory `AGENTS.md` (or `CLAUDE.md` — same file) when specifically needing information about that section, to conserve context.
- **AGENTS.md + CLAUDE.md in every documented directory**: Whenever a new directory is added that contains tools, scripts, or documentation worth describing to AI agents, create a `AGENTS.md` in that directory and a `CLAUDE.md -> AGENTS.md` symlink alongside it (`ln -s AGENTS.md CLAUDE.md`). This ensures both Antigravity CLI and Claude Code can navigate the repo from any subdirectory. The symlink keeps both agents in sync from a single source file. Add a link to the new `AGENTS.md` in this root file's Repository Structure section.
- **Skills are shared**: any `SKILL.md` under `src/` (e.g. `src/ssh-host-finder/SKILL.md`, `src/wispr-flow-debug/SKILL.md`, or a tool's `src/<tool>/skill/`) drives both assistants — `sync-skills` discovers every `SKILL.md` and links it into `~/.claude/skills` and `~/.gemini/config/skills`. Edit once, benefit twice.
- **Skills should ship evals**: a skill folder should carry an `evals/evals.json` (the `skill-creator` format — `{ "skill_name", "evals": [ { "id", "prompt", "expected_output" } ] }`) capturing its trigger/behavior cases. `make skill-evals` (→ `opt/scripts/system/skill-eval.sh --check`) deterministically validates every skill's corpus and reports a **SKIP** for any skill folder that has none — no model calls, CI-safe. Behavioral grading (with-skill vs baseline accuracy) is the on-demand `skill-creator` loop, not this gate. See [`ai/skills/AGENTS.md`](./ai/skills/AGENTS.md).

## Asking the User (blocking questions must be colorized)

- **Surface every must-answer question through the interactive prompt, not plain prose.** Whenever you need a decision from the user *before you can proceed* — a genuine fork you can't resolve from the request, the code, or a sensible default — present it via the assistant's interactive question tool so it renders as a **distinct, colorized prompt** that's impossible to miss in a wall of output. In Claude Code that is the **`AskUserQuestion`** tool; in Antigravity CLI / other harnesses use the equivalent interactive confirmation/elicitation tool.
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
  (both pass bashisms). The per-rule detail (shebang policy, POSIX-only login files, portable-tool
  substitutions, bash-4 hazards, and the `# portability-ok: <reason>` opt-out) lives in the spec's
  [§2 rules and §5 checklist](./docs/mbo/specs/shell-portability.md) — conform to those so you don't trip the gate.
- **Use $HOME**: Always use `${HOME}` or `~` instead of absolute home paths (e.g., `/home/wenlock` or `/Users/eraigosa`) in scripts, aliases, and configuration files to ensure they are portable across different systems and users.
- **Avoid Hardcoded Usernames**: Never hardcode usernames in paths or instructions; use environment variables like `$USER` if needed.
- **Avoid Hardcoded Paths**: Use relative paths or environment variables (like `BASE_DIR` in `install.sh`) whenever possible.

## Shell & Dotfiles Conventions

- **Minimal alias surface**: prefer ONE canonical alias per workflow. Don't propose variants (`foo-c`, `foo-r`, `foo-status`) up front — add them only when asked. The shorter the alias surface, the easier the dotfiles are to memorize and audit.
- **Self-contained shell config**: `.bashrc` / `.zshrc` / sourced fragments must resolve paths from their own location (or `$HOME` / `$DOTFILES_DIR` env vars), never from hardcoded `$HOME/git/dotfiles`. The repo can be cloned anywhere and the config must still work.
- **One install path**: when adding shell config, wire it through `install.sh` so a fresh clone bootstraps cleanly. Don't rely on the user manually symlinking anything you create.
- **Worktree safety & `install.sh` (critical)**: NEVER run `install.sh` from a `gss feature` worktree. The script creates absolute symlinks in your `$HOME` (e.g., `~/.zshrc -> .../dotfiles/opt/profiles/zshrc`). Running it from a worktree will link your global configuration to a transient, task-specific path. Always switch to the main repository (`~/git/dotfiles`), checkout the desired branch, and run `install.sh` from there to ensure your system remains in a predictable, stable state.
- **`install.sh` is interactive — never run it non-interactively or backgrounded**: prompts are front-loaded, but on Windows/WSL the Desktop deploy + UAC elevation runs at the **END** of the script — stay nearby after answering `[y]`. Prompt timing, `[y]/[s]`/no-TTY semantics, and the gff `install.windows.*` overrides: [docs/install-windows.md](./docs/install-windows.md).
- **AI-tool config provisioning — copy into well-known `$HOME` paths; no new symlinks**: the repo configures AI tools by **copying** files into well-known paths (`~/.claude`, `~/.gemini`, …); symlinks into the checkout are legacy — do not introduce new ones. Settings must reference well-known `$HOME` paths (e.g. `~/.claude/hooks/safety_guard.sh`), never repo-internal paths like `$HOME/git/dotfiles/ai/hooks/...` (breaks on worktrees/CI). Forced-field merge semantics: [docs/mbo/designs/2026-06-02-ai-config-home-provisioning.md](./docs/mbo/designs/2026-06-02-ai-config-home-provisioning.md).

## Docker build layering — where a new install step belongs (CI performance)

The CI **Build and Integration Test** job (the most expensive job) is kept fast
by a **three-tier layering**. When you add a new dependency, tool, package, or
config step, put it in the RIGHT tier or you silently regress build time (or,
worse, correctness). Decision guide:

1. **OS-level foundation → `Dockerfile.base`** (published to GHCR from **main
   only**, rebuilt weekly). For truly foundational, rarely-changing things:
   base apt packages, docker/dind, the shell itself. Changing the base is
   costly to validate — see the base-change note below.
2. **App-level external install dependency → `install.sh` `--phase deps`**
   (the **cached** deps layer of the app `Dockerfile`). This is where the *rule
   of thumb* lands: **if it's an external install — an apt/brew package, a
   downloaded tool/CLI, or a language runtime/toolchain (goenv/pyenv/rbenv/nvm)
   — it's a `deps` step.** Gate it with a `gff` flag and add that flag's key to
   **`_IP_DEPS_FLAGS`** in `install.sh`. Kept in the app (not the base) so a PR
   that changes a deps installer still busts that layer and is **validated per
   PR**; unchanged, it's a cache hit and skipped.
3. **Config / skill / symlink / repo-content step → `install.sh` `--phase
   config`** (the per-commit layer). **If it consumes repo content — links a
   dotfile, syncs a skill/plugin, builds an sdk binary, runs a project script —
   it does NOT belong in deps.** Add its flag key to **`_IP_CONFIG_FLAGS`**.

**Keep the two lists in `install.sh` in sync with new sections.** Omitting a
deps key makes the step re-run every commit (slow); omitting a config key bakes
it into the cached deps layer so later edits stop taking effect per commit (a
correctness bug). `--phase all` (a normal `./install.sh`) applies no overrides,
so real machines are unaffected.

**Changing `Dockerfile.base` in a PR? CI detects it automatically.** Because the
base publishes from main only, the Build job checks the PR's changed files and
**builds the base locally** (instead of pulling the stale published one) when a
PR touches `Dockerfile.base` or `opt/scripts/docker/dockerd-entrypoint.sh`, so
your base change is actually validated in that PR. Deps/config changes need no
special handling — their app layers re-validate on their own.

## Git Workflow

- The **git-safe-sync (gss)** skill is the canonical commit + push path. Use the `/sync` slash command for the common case (commit + push to main) — it scaffolds the introspect → propose → confirm → execute flow.
- **Confirmation is mandatory** regardless of the user phrasing ("sync", "push", "commit it"): always present options via `AskUserQuestion` before any `git add` / `git commit` / `gss push` / `gss pr`. The gss skill's "Mandatory Confirmation" rule overrides any autonomous-mode preference.
- **Push mechanics live in the gss skill** ([sdk/gss/skill/SKILL.md](./sdk/gss/skill/SKILL.md), loaded when you push): the two-call approval-token recipe (token generation and `gss push`/`gss pr`/`gss sync` must be **separate** Bash calls — the safety hook blocks chaining) and first-push handling for brand-new branches (`--set-upstream` auto-detect, single token, no double prompt). Follow the skill when executing.
- **Merging goes through the Mergify queue**: label a PR `ready-for-merge` and it lands itself (in-place update, squash, serialized). Before changing CI workflows, required checks, or `.github/mergify.yml`, read [docs/mergify.md](./docs/mergify.md) — it explains every merge rule (squash-only, strict up-to-date, always-run required checks, the external-contributor review gate, break-glass) and why breaking them deadlocks or bypasses the queue.
- **Group related changes**: prefer one cohesive commit per logical unit. Stage files by explicit name (never `git add -A` / `git add .`) to keep blast radius tight and avoid sweeping in unrelated dirty state.
- **`.gitignore` allowlist pattern (critical — read first)**: `.gitignore` starts with `*` — **every** path is ignored until an explicit `!`-rule opts it in, so a new file missing from `git status` is not lost, it's ignored. After creating any file: `git status --short -- <path>`; if absent, `git check-ignore -v <path>` and add a narrow `!`-rule **before** staging — never `git add -f`. Opted-in path list, worked examples, and ground rules: [docs/gitignore-allowlist.md](./docs/gitignore-allowlist.md).

## Hook & Regex Safety

- `ai/hooks/safety_guard.sh` is a PreToolUse hook with regex-based deny rules. Its companion test driver is `ai/hooks/safety_guard_test.sh`.
- **When editing the hook**: extend `safety_guard_test.sh` first. Add at least one new `assert_exit 0` case proving a legitimate command of the same shape still passes, and one new `assert_exit 2` case proving the malicious shape is still blocked. Run the test driver and require all cases to pass before committing.
- **Beware bash regex line-spanning**: bash regex `.*` matches newlines and command separators (`;`, `|`, `&`). Use `${SAFE_CHARS}` (defined at the top of the hook as `[^[:cntrl:];|&]`) to scope a pattern to one shell-command segment.
- **Strip heredoc bodies before matching**: multi-line content (commit messages, README text) gets passed via heredocs and routinely contains literal dangerous patterns. Use `strip_heredocs.awk` to drop those bodies before regex evaluation.
