---
name: pr-list
description: List GitHub Pull Requests in a Discord-friendly format with status icons, blockquotes, and direct links. Use when the user asks for "pr list", "list PRs", "open pull requests", or a chat-ready summary of PRs for a repository.
---

# PR List (Discord-friendly)

List pull requests for a repository and present them in a compact, chat-ready
format. Works in Discord, Slack, and terminal markdown alike.

## Fetching the data

Use the GitHub CLI (`gh`). Default to the repository of the current working
directory; if the user names a repo (`owner/name`), pass it with `-R`.

```bash
gh pr list --json number,title,url,author,isDraft,reviewDecision,statusCheckRollup,updatedAt --limit 20
```

For "my PRs" add `--author @me`; for review requests add `--search "review-requested:@me"`.

## Output format

One entry per PR, newest first. Each entry is a blockquote with a status icon,
a **direct link**, the author, and age:

```
📋 **Pull Requests — <owner/repo>** (<N> open)

> 🟢 [#88 fix(clawdad): self-sufficient install](https://github.com/owner/repo/pull/88)
> ✍️ wenlock · updated 2h ago · ✅ checks passing

> 📝 [#87 draft: new dashboard layout](https://github.com/owner/repo/pull/87)
> ✍️ alice · updated 3d ago · ⏳ checks running
```

### Status icons

| Icon | Meaning |
|------|---------|
| 🟢 | ready for review / approved (`reviewDecision: APPROVED`) |
| 🔴 | changes requested (`reviewDecision: CHANGES_REQUESTED`) |
| 📝 | draft (`isDraft: true`) |
| ✅ | checks passing (statusCheckRollup all SUCCESS) |
| ❌ | checks failing |
| ⏳ | checks pending/running |

### Rules

- Always link the PR number directly to its URL — never a bare `#123`.
- Keep each entry to two lines (title line + meta line) so mobile Discord stays readable.
- If there are more than ~10 PRs, summarize the overflow: `…and 12 more: <link to PR list>`.
- No PRs open → say so in one line with the repo link, don't render an empty list.

## Requirements

- `gh` CLI authenticated (`gh auth status`).
