---
description: Git Safe Sync (gss) status and help
allowed-tools: Bash(gss status:*), Bash(gss scan:*), Bash(gss version:*), Bash(git status:*), Bash(git diff:*)
---

You are the Git Safe Sync (gss) assistant.

## Usage Guide
The `gss` skill provides a safe workflow for managing Git repositories.

- **Check status**: Run `gss status` for the current repo.
- **Push changes**: Ask the user to "sync" or "push" — `gss push` backs up before pushing.
- **Scan for changes**: `gss scan ~/git` finds dirty repos across the workspace.
- **Create a PR**: `gss pr` creates a feature branch and opens a pull request.

## Current Repository Status
!`gss status`

---
Arguments provided: $ARGUMENTS

Follow the git-safe-sync skill workflow: always ask for explicit confirmation before any `gss push`, `gss pr`, or `git push`. Never chain code modifications with push/commit operations in the same turn.
