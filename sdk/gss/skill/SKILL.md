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
- **Repository Summaries**: Generate categorized markdown tables of up to 50 open PRs and Issues, including latest changes and associated issue numbers.

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

- **PR Hygiene — the description and labels must always match the PR's full current scope**: A PR's description and labels are part of its state, not a one-time creation step. Keep them accurate for *everything the PR now contains*, every time you change what's on it:
- **On create** (`gss pr`): `gss pr` does **not** infer a body or labels — pass them explicitly via `gss pr --title "<subject>" --body "<markdown body>"` and use `gh pr edit --add-label "<labels>"` immediately after.
- **`gss pr` never commits** — the Add → Commit steps above are yours, and they come **before** the approval token (the token is HEAD-bound; committing after invalidates it). On the default branch with no commits ahead of `origin/<default>`, `gss pr` fails fast with "nothing to PR" instead of pushing an empty feature branch; commit the pending work, regenerate the token, and re-run.
- **On every later push to a branch that already has an open PR** (`gss push`, or a re-run `gss pr`): `gss push` only updates the branch. After such a push you MUST refresh the description and labels to cover the newly added commits, via `gh pr edit <number> --title "<subject>" --body "<body>" --add-label "<labels>"`. **Never push scope-changing commits to a PR and leave its description or labels behind.**
- **Label Selection**: Use standard prefixes (`feat`, `fix`, `docs`, `ci`, `test`, `style`, `refactor`, `chore`) and area-specific labels (e.g., `gsl`, `wispr`, `remote-claude`). If a PR addresses an issue, ensure it carries the same categorization labels as the issue.
- The body should always include — **What** (summary of functional changes), **Why** (rationale), **Impact** (effect on system/UX), **Testing** (how verified). NEVER use generic or empty descriptions.
- **Note**: `gss pr` has no `--draft` flag — classic PRs are created ready-for-review. (Draft PRs exist only in the `gss feature` stacked-worker workflow, whose PR bodies and labels are owned by `gss feature checkpoint` — do **not** hand-edit those with `gh pr edit`.)

### 3. Execution (Action Phase)
- ONLY proceed if the user explicitly selected a confirmation option in the previous turn.
- **Handshake Generation**: Before calling `gss push`, you MUST generate an approval token — as **two separate commands**. The `safety_guard.sh` hook intentionally blocks chaining the token generation and the push in one command, so the user sees an explicit approve→publish gate.
  * Step 1 (one command): `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`
  * Step 2 (a separate command): `gss push`
- This consumes the token and satisfies the binary's technical safeguard.
- **First push of a brand-new branch (no double prompt)**: `gss push` detects a branch with no `origin/<branch>` counterpart and creates it with `--set-upstream` instead of failing on the rebase step — it prints `New branch — … --set-upstream`. So the single token → `gss push` recipe works for a new branch too; issue it ONCE. Do **not** expect (or work around) the old `sync: rebase ... couldn't find remote ref <branch>` failure, which used to burn the token and force a **second** confirmation prompt. Fallback for an OLD binary that still errors that way: run one plain `git push -u origin HEAD` (a plain push is not token-gated), then create the PR — do not regenerate the token and retry `gss push`.
- **Auto-Recovery**: If `gss push` fails with a "missing or unreadable approval token" error (exit 22), it means you skipped the user confirmation turn. You MUST immediately stop and explicitly ask the user for permission before retrying.
- **Auth/scope failures (403, "refusing to allow … without `workflow` scope", email-privacy rejections)**: check whether a stale `GITHUB_TOKEN` is exported in the environment (`env | grep -c GITHUB_TOKEN`) — gh prefers that env var over its keyring, so a shell-exported token keeps OLD scopes alive even after `gh auth refresh`. Fix: `unset GITHUB_TOKEN`, then retry. Tools that need a raw token fetch the current one on demand with `gh auth token`; never export it at shell startup.
- **Refresh the PR description (mandatory after pushing to an existing PR)**: If the push added commits to a branch that *already* has an open PR (rather than creating a new one), the PR body is now stale — `gss push` never updates it. Immediately reconcile it with `gh pr edit <number> --title "<subject>" --body "<body>"` so the description covers the PR's new full scope (re-derive What/Why/Impact/Testing from *all* commits now on the branch, e.g. `git log <base>..HEAD`). This is part of the push, not an optional follow-up. (Skip only for `gss feature` worktrees — `gss feature checkpoint` owns those bodies.)

### 4. Summarize & Link (Verification Phase)
- After execution, summarize the result using the output from 'gss push'.
- **Detailed Summary**: If fewer than 10 files were changed, provide the list of files and their +/- line counts.
- **Compact Summary**: If 10 or more files were changed, provide the high-level stats (e.g., "15 files changed").
- Provide the **GitHub Comparison Link** or the **Pull Request URL**.
- **Description sync check**: If the push targeted an existing PR, confirm you refreshed its description (Execution phase) so it matches every commit now on the PR. A push that left the description stale is an incomplete sync — go back and run `gh pr edit`.
- **Browser Verification**: Ask the user whether to open the **GitHub Comparison/PR link** (from the push/pr output) in their browser for final verification. To open it, run the repo's `open-url <link>` helper (`opt/scripts/misc/open-url`), which picks the right opener per platform — `open` on macOS, `wslview`/`explorer.exe` under WSL, `xdg-open` on Linux. It exits non-zero and prints the link when no opener exists (e.g. a headless SSH session); in that case just leave the link visible rather than retrying.

### 5. Repository Summaries (PRs & Issues)
- When asked to summarize open PRs or issues, always fetch detailed data up to a limit of 50 items (e.g., `gh pr list --state open --limit 50 --json number,title,author,labels,state,updatedAt,body` and similarly for `gh issue list`).
- Present the results in **categorized markdown tables**, grouped by focus area or topic (e.g., "Infrastructure", "UI Improvements").
- Include columns for PR/Issue Number, Title, Status, and a brief Summary of what the item addresses.
- For PRs, explicitly extract and include **associated issue numbers** and highlight any **latest changes** or recent commits based on the retrieved data.
- **Label Markers**: Include visible markers for labels (e.g., `[feat]`, `[fix]`) in the Title or Status columns to help categorize items within the summary. If labels are missing but the intent is clear (e.g., "Fixes X"), suggest the appropriate label to the user.

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

### Which lane to pick (read this before publishing anything)

The two modes are not equally reachable to an autonomous assistant, and picking
the wrong one wastes a round trip per attempt:

| You are… | Use | Because |
| :--- | :--- | :--- |
| An assistant publishing your own work | **feature worker → `gss feature checkpoint`** | `checkpoint` is deliberately **not** token-gated: it publishes a *draft* PR, which is safe to create autonomously. |
| A human doing a one-off push | `gss push` / `gss pr` | Simpler, but **token-gated** — needs a human-minted approval token. |

**`gss push` / `gss pr` are a dead end for an assistant working alone.** They
require `~/.config/gss/approval.token`, and minting it *is* the human gate — an
assistant that mints its own token has defeated the control, not satisfied it.
When there is no human at the keyboard to mint one, do not queue up a `gss pr`
and wait: **start a feature, add a worker, and `checkpoint`** — that is the lane
built for you, and it lands a reviewable draft PR without touching the gate.

Promotion stays human: `gss feature pr --ready` *is* token-gated.

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
   feature. **Run it from inside the target repo** (see
   [Traps](#traps-that-cost-real-time) — `-r` does not scope this), and add a
   worker in the same sitting: a feature with zero workers has no supported
   teardown.
2. `gss feature worker add --feature <name> --purpose <p> --description "…"`
   — add a worker worktree. `--json` prints `worker_ref` / `branch` /
   `worktree_path`; **keep the `worker_ref`** — it is how you address the
   worker from anywhere.
3. `gss feature checkpoint --worker <ref>` — rebase on base, push, and
   create/update the draft PR (refreshing the stack section across the
   feature). **Prefer `--worker <ref>` over `cd`-ing into the worktree**; see
   [Traps](#traps-that-cost-real-time). Add `--auto` for hook/pane-close use.
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

### Traps that cost real time

Each of these fails **silently or misleadingly** — they are not guessable from
`--help`, which is why they are written down.

- **`-r/--repo` does not scope `gss feature` commands.** It is a global flag and
  it is accepted without complaint, but the feature verbs resolve owner/repo
  from **cwd**. Running `gss -r ~/git/dotfiles feature start x` from a different
  repo silently registers the feature against *that* repo, with *its* base
  commit — no error, no warning. **Always `cd` into the target repo** for
  `feature` verbs; treat `-r` as classic-mode only. Verify afterwards: the
  printed worktree path must read `…/worktrees/<owner>/<expected-repo>/<name>`.
- **Do not `cd` into a worktree to run gated verbs — pass `--worker <ref>`.**
  Two hooks conflict here and make the "run it from inside the worktree" advice
  impossible to follow from an assistant: `safety_guard.sh` matches the command
  text **without expanding variables**, so `cd "$HOME/…/worker" && gss feature
  checkpoint` is refused as "not a feature worker worktree" — while
  `privacy_guard.sh` blocks writing the expanded `/home/<user>/…` path that
  would satisfy it. `--worker <feature>/<user>/<purpose>` sidesteps both and
  works from anywhere in the repo.
- **`--help` is blocked for gated verbs from the wrong cwd.** `safety_guard.sh`
  matches on the verb, not the flags, so `gss feature checkpoint --help` is
  refused outside a worktree. To discover flags, read the cobra source
  (`sdk/gss/cmd/feature_<verb>.go`) or pass `--worker` alongside `--help`.
- **A feature with zero workers cannot be torn down.** `gss feature done`
  removes a *worker*, deleting the feature only when its last worker goes and
  `FEATURE.md` is unedited. `gss feature start` alone therefore leaves a
  registry row + `FEATURE.md` with no supported removal path — cleanup means
  hand-editing `registry.json`. **Add the worker immediately after `start`**, or
  don't `start` yet.

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
- **`WORKER.md` placement (issue #132)**: `WORKER.md` is seeded **outside** the
  worker's git worktree, at
  `<worktrees-root>/<owner>/<repo>/<feature>/<user>/.gss-meta/<leaf>/WORKER.md`,
  so it never appears in the consumer repo's `git status` and **cannot** be
  accidentally committed. There is **no manual cleanup step** — `gss feature
  done` removes the worker's `.gss-meta/<leaf>/` automatically. (An older gss
  may have left a root-level `WORKER.md` inside the worktree; the next
  `checkpoint --auto` relocates it to the meta path.)
- **`FEATURE.md` cleanup**: `FEATURE.md` is transient feature-level scaffolding;
  `gss feature done` removes it when the feature empties and it carries no human
  edits (an edited FEATURE.md is retained with a notice). No manual step needed.
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
