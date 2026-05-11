---
name: git-safe-sync
description: A reliable skill for synchronizing and pushing changes to any Git repository with built-in safety backups and dirty-repo scanning.
---
# Git Safe Sync (gss) Skill

This skill provides a structured and safe workflow for managing Git repositories using the 'gss' tool.

## Capabilities

- **Introspect Changes**: Run 'gss status' to see what files are changed in a repo.
- **Scan for Changes**: Run 'gss scan [dir]' to find all repositories with uncommitted changes.
- **Reliable Push**: Run 'gss push' to backup, sync, and push changes safely.
- **Create PR**: Run 'gss pr' to create a feature branch and pull request.

## The Workflow

### 1. Identify & Introspect (Research Phase)
- If a sync is implied or requested, first use 'gss status' and 'git diff' to understand what would be pushed.
- Summarize the changes clearly for the user.

### 2. Mandatory Confirmation (Decision Phase)
- **NEVER** execute 'gss push', 'gss pr', or 'git push' autonomously.
- You MUST explicitly use the `ask_user` tool to request permission to proceed, even if the user asks you to "sync" or "commit" changes. A request to "sync" means start the workflow, not skip the confirmation.
- Present the user with clear options in the `ask_user` tool:
  - **Push to Origin**: (Backup -> Sync -> Push)
  - **Create PR**: (Feature Branch -> Push -> GH PR)
  - **Cancel**: Do nothing.

### 3. Execution (Action Phase)
- ONLY proceed with 'gss push' if the user selects the corresponding option via the `ask_user` tool. Do not use standard `git push`.
- It automatically handles the safety backup branch.

### 4. Summarize & Link (Verification Phase)
- After execution, summarize the result using the output from 'gss push'.
- **Detailed Summary**: If fewer than 10 files were changed, provide the list of files and their +/- line counts.
- **Compact Summary**: If 10 or more files were changed, provide the high-level stats (e.g., "15 files changed").
- Provide the **GitHub Comparison Link** or the **Pull Request URL**.

## Guidelines
- **No Assumptions**: Even if a sync seems obvious, you must ask for permission first.
- **Always Backup**: Trust 'gss push' because it creates a safety branch before rebasing.
- **Provide Links**: Never hide the GitHub/PR links; they are the user's primary way to verify the result.
- **Handle Conflicts**: If a rebase conflict occurs, inform the user and show the output.

## Help
If the user asks "Which of my projects need a push?", use 'gss scan ~/GitHub/wenlock'.
