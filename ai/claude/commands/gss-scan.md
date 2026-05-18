---
description: Scan ~/git (or a given directory) for repos with uncommitted changes
allowed-tools: Bash(gss scan:*), Bash(gss status:*)
---

Scan for repositories that need attention.

## Scan results
!`gss scan ${ARGUMENTS:-$HOME/git}`

---
Arguments: $ARGUMENTS

For each dirty repo above, summarize what kind of change is pending (uncommitted, unpushed, ahead/behind). Don't take action — just report. If the user wants to sync any of them, hand off to the git-safe-sync skill which requires explicit confirmation per repo.
