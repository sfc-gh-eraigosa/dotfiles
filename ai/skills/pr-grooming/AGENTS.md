# pr-grooming skill

Grooms a repo's open pull requests against `main`: classifies each (mbo-draft, stale-draft,
bugfix, dependabot, other), verifies whether it still applies (paths on `main`, merged PRs and
`docs/mbo/index.md` rows that supersede it, merge state, CI), rebases what is merely behind,
re-proves and promotes old bug fixes, queues green dependency bumps, and closes clearly stale or
duplicate drafts with a reason comment — branches are never deleted.

Agent-facing instructions: [`SKILL.md`](./SKILL.md). Scripts (`bash scripts/<name>.sh`):

| Script | Does |
| :-- | :-- |
| `pr-snapshot.sh` | read-only dump: open PRs (bodies, files, comments, checks), `triage.tsv` with category/age/merge/CI, `mbo-rows.txt` from `index.md` |
| `pr-rebase.sh` | rebases one PR onto its base in a throwaway worktree and pushes `--force-with-lease`; exit 2 on conflicts with the worktree left for manual resolution |
| `pr-apply.sh` | applies a reviewed `verdicts.json`: close-stale (comment + close), comment, ready, ready-for-merge (guarded by draft/CI/merge-state); `--dry-run`, `--only`, `applied.log` |
| `pr-report.sh` | renders `verdicts.json` as the markdown summary (counts + one table per verdict) |

`references/verdict-schema.md` is the JSON contract between triage subagents and the scripts.
`evals/evals.json` holds the trigger/behaviour cases (`make skill-evals`). Sibling skill:
`issue-grooming` (the issue backlog). First run: 2026-10-16, 8 open PRs.
