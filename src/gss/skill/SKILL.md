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

When asked to sync or push changes:

### 1. Identify Target
If not specified, use the current directory or ask the user. Use 'gss scan' to find repositories that need attention.

### 2. Introspect
Run 'gss status' (and optionally 'git diff') to summarize changes.

### 3. Confirm Action
Present the user with a choice:
- **Push to Origin**: (Backup -> Sync -> Push)
- **Create PR**: (Feature Branch -> Push -> GH PR)
- **Show Diffs**: Exact code changes.
- **Cancel**: Abort.

### 4. Execute
Use 'gss push' for the standard workflow. It automatically handles the safety backup branch.

### 5. Summarize & Link
After execution, provide the user with:
- A clear confirmation of success.
- The **GitHub Comparison Link** (from 'gss push') or the **Pull Request URL** (from 'gss pr').
- A high-level recap of the safety steps taken (backup branch created, rebase performed).

## Guidelines
- **Always Backup**: Trust 'gss push' because it creates a safety branch before rebasing.
- **Provide Links**: Never hide the GitHub/PR links; they are the user's primary way to verify the result.
- **Handle Conflicts**: If a rebase conflict occurs, inform the user and show the output.
- **Multi-Repo Awareness**: Use the '--repo' flag to manage repositories from anywhere.

## Help
If the user asks "Which of my projects need a push?", use 'gss scan ~/GitHub/wenlock'.
