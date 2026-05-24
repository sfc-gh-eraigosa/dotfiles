---
name: gss_restack_with_git_machete
description: Drive git-machete to restack/rebase a gss feature worker stack when `gss feature conflicts` reports overlap or a checkpoint/rebase/restack aborts on conflict. Local rebase helper only — gss remains the single writer for PRs and the registry.
---

# Restack a gss stack with git-machete

`gss` itself ships **no** stack-rebase engine (design.md → "Companion rebase
tooling", resolution #14). Its built-in baseline is `git config
rebase.updateRefs true` (set in every worktree it creates), which restacks
descendants during an ordinary interactive rebase. When that isn't enough —
a multi-branch stack needs reordering, or a `gss feature` command aborts on a
rebase conflict — this skill drives [`git-machete`](https://github.com/VirtusLab/git-machete)
as a **local** restack helper.

## License (verified before recommending)

- **Tool**: `git-machete` — https://github.com/VirtusLab/git-machete
- **License**: **MIT** — Allowed/permissive per
  [src/CLAUDE.md → Library standards](../../CLAUDE.md). Not GPL/AGPL/LGPL, not
  cloud-gated/SaaS.
- **LICENSE blob (pinned)**:
  https://github.com/VirtusLab/git-machete/blob/master/LICENSE
  git blob SHA `2605254a8861c08d7a77813101e41de912959f2f`
  ("MIT License — Copyright (c) 2017-2024 VirtusLab", verified verbatim from
  the canonical upstream raw `LICENSE`).
- **Version**: require `git-machete >= 3.20` (the verb set below is stable
  across 3.x); validated against **v3.41.0** (May 2026). If `git machete
  --version` reports `< 3.20` or `>= 4.0`, stop and ask the user to install a
  supported version rather than guessing at changed flags.

## Verb allowlist (Security boundary — do not exceed)

git-machete is used **only** as a local branch-tree / rebase tool. gss is the
single writer for GitHub PRs and `registry.json`; this skill must never let
git-machete mutate either.

**Allowed:**

| Verb | Why | Mutates? |
|------|-----|----------|
| `git machete --version` | preflight version gate | no |
| `git machete status` | show the branch tree + sync state | no (read-only) |
| `git machete fork-point <branch>` | inspect where a branch diverged | no (read-only) |
| `git machete update --no-interactive-rebase` | rebase the current branch onto its parent, non-interactively | **yes (local history)** |
| `git machete traverse --fetch --start-from=here` | walk the stack, rebasing each branch onto its parent with conflict prompts | **yes (local history)** |

**Disallowed (never run these from this skill):**

- `git machete github ...` (`create-pr`, `retarget-pr`, `checkout-prs`,
  `anno-prs`, …) — **PRs are gss's job.** Letting git-machete open/retarget
  PRs would desync `registry.json` from GitHub. After any restack, re-run the
  matching `gss feature` command so gss updates PR bases + the registry.
- `git machete delete-unmanaged`, `git machete slide-out` — these remove or
  collapse branches; deletion/teardown is `gss feature done`'s job.
- Any `--push` / force-push flags — gss owns pushes (`--force-with-lease`
  inside its orchestrators).

If a situation seems to require a disallowed verb, stop and hand back to the
user with the diagnosis; do not improvise.

## When to use

- `gss feature conflicts` reports files touched by more than one worker and
  the user wants to linearize/rebase the stack before resolving.
- `gss feature checkpoint` / `rebase` / `restack` aborts with
  `ErrRebaseConflict` and the stack needs a guided, multi-branch restack.

## Prerequisites

- `git-machete` installed (MIT). Linux: `pipx install git-machete` (or
  `sudo apt-get install git-machete` where packaged). macOS: `brew install
  git-machete`. Confirm with `git machete --version`.
- A gss feature stack already exists (run `gss feature list --tree` to see it).
- You are operating inside the repo (not a stripped checkout).

## Steps

### 1. Preflight

Run `git machete --version` and confirm it is `>= 3.20` and `< 4.0`. If not,
stop and ask the user to install a supported version.

### 2. Discover the stack

Run `gss feature list --tree` to see gss's view, then `git machete status`.
If git-machete has no definition file yet (`.git/machete`), build one from the
gss stack: the worker branches and their `base_branch` links map directly onto
machete's parent/child tree. Prefer `git machete discover --list-commits` and
let the user confirm the inferred tree before editing.

### 3. Restack

- Single branch onto its (already-correct) parent:
  `git machete update --no-interactive-rebase`.
- Whole stack: `git machete traverse --fetch --start-from=here` and resolve
  conflicts in the worktree as prompted (git-machete pauses; the user fixes +
  `git rebase --continue`).

### 4. Hand back to gss

git-machete only rewrote **local** history. Now let gss reconcile PRs +
registry:

- Re-run `gss feature checkpoint` (or `gss feature rebase`) in each affected
  worker so gss force-pushes with lease and updates the PR base/body.
- If a parent merged, `gss feature merged <ref>` already handles re-targeting
  — use git-machete only for the local rebase, not the PR retarget.

### 5. Report

Summarize what was restacked, which branches moved, and which `gss feature`
commands the user should run (or that you ran) to bring PRs back in sync.
Never claim a PR was updated unless a gss command did it.
