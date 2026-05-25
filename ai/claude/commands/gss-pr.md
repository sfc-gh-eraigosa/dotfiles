---
description: Create a feature branch and open a pull request via gss pr
allowed-tools: Bash(gss status:*), Bash(git status:*), Bash(git diff:*), Bash(git log:*)
---

You are about to run the `gss pr` workflow (creates a feature branch, pushes, opens a PR on GitHub).

## Current state
!`gss status`

## Pending diff
!`git diff --stat`

---
Arguments (PR title hint, optional): $ARGUMENTS

Follow the **git-safe-sync** skill rules strictly:
1. Summarize what would be pushed and what the PR will contain.
2. STOP. Do NOT execute `gss pr` until the user explicitly confirms in a follow-up turn.
3. On confirmation, generate the approval token and run `gss pr` as **two separate Bash calls** (chaining them with `&&` is intentionally blocked by `safety_guard.sh`):
   - Call 1: `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`
   - Call 2: `gss pr --title "<subject>" --body "<markdown body>"`
   Per the skill's **PR Hygiene** rule you MUST pass a real `--body` (What/Why/Impact/Testing); `gss pr` does not infer one, so omitting it ships an empty description. (`gss pr` has no `--draft` flag; classic PRs are created ready-for-review.)
   - **Updating an existing PR**: if the branch already has an open PR and you are pushing more commits, `gss pr`/`gss push` will NOT refresh the body — you MUST update it yourself with `gh pr edit <number> --title/--body` so the description reflects every commit now on the PR. Never leave a stale description behind a push.
4. After execution, surface the PR URL prominently. If you updated an existing PR, confirm its description was refreshed to match. Then ask whether to open it via the `open-url <url>` helper (`opt/scripts/misc/open-url`) — it picks the right opener per platform (macOS `open`, WSL `wslview`/`explorer.exe`, Linux `xdg-open`).

**Never** chain code modifications and `gss pr` in the same turn — that violates the Turn Break Mandate.
