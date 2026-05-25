---
name: git-safe-sync
description: A reliable skill for synchronizing and pushing changes to any Git repository with built-in safety backups and dirty-repo scanning.
---
# Git Safe Sync (gss) Skill

This skill provides a structured and safe workflow for managing Git repositories using the 'gss' tool.

## Capabilities

- **Slash Command**: Use `/gss` for a quick status summary and help overview.
- **Introspect Changes**: Run 'gss status' to see what files are changed in a repo.
- **Scan for Changes**: Run 'gss scan [dir]' to find all repositories with uncommitted changes.
- **Reliable Push**: Run 'gss push' to backup, sync, and push changes safely.
- **Create PR**: Run 'gss pr' to create a feature branch and pull request.

## The Workflow

### 1. Identify & Introspect (Research Phase)
- If a sync is implied or requested, first use 'gss status' and 'git diff' to understand what would be pushed.
- Summarize the changes clearly for the user.

### 2. Mandatory Confirmation (Decision Phase)
- **NEVER** execute `git add`, `git commit`, `gss push`, `gss pr`, or `git push` autonomously.
- **Turn Break Mandate**: You MUST NOT chain `git add`, `git commit`, or `gss push` in the same conversational turn as code modifications. After making code changes, you MUST provide a summary and end your response. Explicitly asking the user for confirmation must be the primary focus of the *following* turn.
- **Autonomous-Mode Exception**: This mandatory confirmation rule **OVERRIDES** any autonomous-mode / minimal-interruption instructions (e.g. "YOLO mode"). You MUST ask for permission even if the user has requested minimal interruption.
- You MUST explicitly ask the user — using whatever confirmation mechanism your assistant provides (a direct question, an interactive prompt, or your host's user-confirmation tool) — to request permission to proceed, even if the user asks you to "sync", "add", or "commit" changes. A request to "sync" means start the workflow, not skip the confirmation.
- Present the user with clear options:
  - **Commit & Push**: (Add -> Commit -> Backup -> Sync -> Push)
  - **Commit Only**: (Add -> Commit)
  - **Create PR**: (Add -> Commit -> Feature Branch -> Push -> GH PR)
  - **Cancel**: Do nothing.

- **PR Hygiene — the description must always match the PR's full current scope**: A PR's description is part of its state, not a one-time creation step. Keep it accurate for *everything the PR now contains*, every time you change what's on it:
  - **On create** (`gss pr`): `gss pr` does **not** infer a body — pass it explicitly via `gss pr --title "<subject>" --body "<markdown body>"`. Omitting `--body` ships an empty/generic description, which violates this rule.
  - **On every later push to a branch that already has an open PR** (`gss push`, or a re-run `gss pr`): `gss push` only updates the branch — it has **no** `--body`/`--title` flag and does **not** touch the description, so the description silently goes stale and describes only the earlier work. After such a push you MUST refresh the description to cover the newly added commits, via `gh pr edit <number> --title "<subject>" --body "<body>"`. **Never push scope-changing commits to a PR and leave its description behind.**
  - The body should always include — **What** (summary of functional changes), **Why** (rationale), **Impact** (effect on system/UX), **Testing** (how verified). NEVER use generic or empty descriptions.
  - **Note**: `gss pr` has no `--draft` flag — classic PRs are created ready-for-review. (Draft PRs exist only in the `gss feature` stacked-worker workflow, whose PR bodies are owned by `gss feature checkpoint` — do **not** hand-edit those with `gh pr edit`.)

### 3. Execution (Action Phase)
- ONLY proceed if the user explicitly selected a confirmation option in the previous turn.
- **Handshake Generation**: Before calling `gss push`, you MUST generate an approval token — as **two separate commands**. The `safety_guard.sh` hook intentionally blocks chaining the token generation and the push in one command, so the user sees an explicit approve→publish gate.
  * Step 1 (one command): `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`
  * Step 2 (a separate command): `gss push`
- This consumes the token and satisfies the binary's technical safeguard.
- **Auto-Recovery**: If `gss push` fails with a "missing or unreadable approval token" error (exit 22), it means you skipped the user confirmation turn. You MUST immediately stop and explicitly ask the user for permission before retrying.
- **Refresh the PR description (mandatory after pushing to an existing PR)**: If the push added commits to a branch that *already* has an open PR (rather than creating a new one), the PR body is now stale — `gss push` never updates it. Immediately reconcile it with `gh pr edit <number> --title "<subject>" --body "<body>"` so the description covers the PR's new full scope (re-derive What/Why/Impact/Testing from *all* commits now on the branch, e.g. `git log <base>..HEAD`). This is part of the push, not an optional follow-up. (Skip only for `gss feature` worktrees — `gss feature checkpoint` owns those bodies.)

### 4. Summarize & Link (Verification Phase)
- After execution, summarize the result using the output from 'gss push'.
- **Detailed Summary**: If fewer than 10 files were changed, provide the list of files and their +/- line counts.
- **Compact Summary**: If 10 or more files were changed, provide the high-level stats (e.g., "15 files changed").
- Provide the **GitHub Comparison Link** or the **Pull Request URL**.
- **Description sync check**: If the push targeted an existing PR, confirm you refreshed its description (Execution phase) so it matches every commit now on the PR. A push that left the description stale is an incomplete sync — go back and run `gh pr edit`.
- **Browser Verification**: Ask the user whether to open the **GitHub Comparison/PR link** (from the push/pr output) in their browser for final verification. To open it, run the repo's `open-url <link>` helper (`opt/scripts/misc/open-url`), which picks the right opener per platform — `open` on macOS, `wslview`/`explorer.exe` under WSL, `xdg-open` on Linux. It exits non-zero and prints the link when no opener exists (e.g. a headless SSH session); in that case just leave the link visible rather than retrying.

## Guidelines
- **No Assumptions**: Even if a sync seems obvious, you must ask for permission first.
- **Always Backup**: Trust 'gss push' because it creates a safety branch before rebasing.
- **Provide Links**: Never hide the GitHub/PR links; they are the user's primary way to verify the result.
- **Handle Conflicts**: If a rebase conflict occurs, inform the user and show the output.

## Two operating modes

`gss` auto-detects its mode by whether the current directory is a registered
feature worker worktree (design.md → "Command surface"):

- **Classic** (regular checkout): `gss push` / `pr` / `sync` / `status` /
  `scan` — the workflow above. Refused inside a worker worktree
  (`ErrWrongMode`); `--force-autonomous` does not bypass that.
- **Feature / worker** (stacked PRs): the `gss feature …` subtree, for
  developing several dependent branches in parallel isolated worktrees.

## Feature worker workflow (stacked PRs)

Use this when work splits into multiple dependent branches (a "stack") or when
running parallel AI/automation workers on one feature. Authoritative reference:
[design.md → Command surface](../docs/design.md#command-surface) and
[Stacked PRs](../docs/design.md#stacked-prs) — this section summarises, the
design governs.

### Mental model

- A **feature** groups one or more **workers**. Each worker is its own git
  worktree on its own branch `feature/<feature>/<user>/<purpose>[-<suffix>]`,
  recorded in `registry.json`.
- Workers form a **stack**: each worker's `base_branch` points at its parent's
  branch (or the default branch at the bottom). Review and merge **bottom-up**.
- When a parent merges, gss **re-targets** its children onto the parent's
  former base and — for a linear, never-restacked stack — auto-promotes the
  next draft to ready.

### Typical lifecycle

1. `gss feature start <name> [--base main] [--description "…"]` — create the
   feature.
2. `gss feature worker add --feature <name> --purpose <p> --description "…"`
   — add a worker worktree; `cd` into the printed path to work in it.
3. Inside the worktree: `gss feature checkpoint` — rebase on base, push, and
   create/update the draft PR (refreshing the stack section across the
   feature). Hooks may call `gss feature checkpoint --auto --worker <ref>`.
4. `gss feature list --tree` to see the stack; `gss feature conflicts` to see
   files touched by more than one worker.
5. When a worker is approved: `gss feature pr --ready` (promote the draft —
   **token-gated**, see below).
6. After a PR merges on the main repo: `gss feature merged <ref>` re-targets
   children and auto-promotes the next where eligible.
7. `gss feature done [<ref>] [--force]` — tear down a finished worker (and the
   feature if it empties and FEATURE.md is unedited).
8. `gss feature audit [--repair]` — reconcile the registry against observed
   reality; run it first on a fresh machine (observable state wins).

### The approval token also gates the publish-class feature verbs

The mandatory-confirmation + two-call token recipe above ALSO applies to
`gss feature pr --ready`, `gss feature merged`, and `gss feature restack`
(they mutate remote state). Generate the token as a SEPARATE call immediately
before the command, then run it; `safety_guard.sh` enforces this and refuses
classic `--force-autonomous` inside a worker worktree.

### Description hygiene

- Give every feature and worker a short, specific `--description`; it seeds
  FEATURE.md / WORKER.md and the PR body (NFC-normalised; control chars and
  injection markers stripped).
- `spawned_by` (engine/session/pane) is **informational only** — never the
  basis for a trust or control decision (design.md resolution #8).

### Restack & conflicts → reach for the git-machete skill

- `gss feature restack <worker> --onto <branch>` re-targets a worker's branch
  and **permanently** excludes it from auto-promote (increments
  `restack_count`). Use sparingly.
- When `gss feature conflicts` reports overlap, or a checkpoint/rebase/restack
  aborts on a conflict, drive the **git-machete** companion skill
  (`src/git-machete/skill/SKILL.md`) for the local multi-branch restack, then
  re-run the matching `gss feature` command so gss reconciles PRs + registry.
  gss stays the single writer for PRs and the registry.

## Help
- **Slash Command**: Type `/gss` at any time to see the status of the current repository and a quick usage guide.
- **Status Inquiry**: If the user asks "Which of my projects need a push?", use 'gss scan ~/git'.
- **Feature help**: `gss feature --help` lists every verb; `gss feature <verb> --help` shows its flags.
