---
description: Stage, commit, and push pending changes via the gss skill in one guided flow
allowed-tools: Bash(gss status:*), Bash(git status:*), Bash(git diff:*), Bash(git log:*), Bash(git rev-parse:*)
---

You are running the **/sync** workflow — a thin wrapper around the `git-safe-sync` skill for the common "commit + push to main" path.

## Current state
!`git status --short --branch`

## Pending diff (stats)
!`git diff --stat`
!`git diff --cached --stat`

## Recent commits (for tone/style)
!`git log --oneline -5`

---
Arguments (optional commit subject hint): $ARGUMENTS

## Required workflow

Follow the **git-safe-sync** skill rules strictly. The sequence below is mandatory — do not collapse turns.

1. **Research** (this turn): summarize what would be committed and propose a commit message. If `$ARGUMENTS` is non-empty, use it as the subject; otherwise infer a Conventional Commits subject from the diff. Group co-changing files; flag any that look unrelated and should be split.
2. **Confirm** (this turn): present the user with explicit options via `AskUserQuestion`: Commit & Push to main, Open a PR instead, Commit only, or Cancel. **Do NOT execute any git/gss command yet.**
3. **Execute** (next turn, only after the user picks an option):
   - Stage files by explicit name (never `git add -A` / `git add .`).
   - Commit using a HEREDOC message that ends with `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
   - For Push: generate the approval token as a **separate Bash call** (`mkdir -p ~/.config/gss && git rev-parse HEAD > ~/.config/gss/approval.token`), then run `gss push` as a **second Bash call**. Chaining them with `&&` is intentionally blocked by `safety_guard.sh`.
   - For PR: use the same two-call pattern with `gss pr` instead of `gss push`.
4. **Surface** (post-execution): show the commit SHA, the safety backup branch, and the GitHub compare/PR URL. Then ask whether to open the URL in the browser (`open` on macOS).

## Hard rules

- **Never** chain code modifications and `gss push` / `gss pr` in the same turn.
- **Never** auto-approve. Even if the user types "sync" or "push", confirmation via `AskUserQuestion` is still required.
- If a file looks like a secret (`.env`, `*.token`, `credentials*`, `*.pem`), flag it and ask before staging.
