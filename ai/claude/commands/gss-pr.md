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
3. On confirmation, generate the approval token first, then run `gss pr`:
   `mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token && gss pr`
4. After execution, surface the PR URL prominently.

**Never** chain code modifications and `gss pr` in the same turn — that violates the Turn Break Mandate.
