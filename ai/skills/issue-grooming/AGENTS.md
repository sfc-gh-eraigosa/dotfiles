# issue-grooming skill

Grooms a repo's open GitHub issues against what actually shipped: verifies each issue against
`main`, merged PRs and `docs/mbo/index.md`, assigns one `priority:1..5` label (1 = most
important), closes addressed/obsolete issues with evidence comments, and refreshes stale bodies
(original folded under `<details>`).

Agent-facing instructions: [`SKILL.md`](./SKILL.md). Scripts (`bash scripts/<name>.sh`):

| Script | Does |
| :-- | :-- |
| `groom-snapshot.sh` | read-only dump: open issues (+comments), labels, merged PRs with closing refs, first-cut clusters |
| `groom-labels.sh` | idempotently creates `priority:1`…`priority:5` with the rubric in their descriptions |
| `groom-apply.sh` | applies a reviewed `verdicts.json`: labels, closes, body refreshes; `--dry-run`, `--only`, `applied.log` |
| `groom-report.sh` | renders `verdicts.json` as the markdown summary (counts + one table per verdict) |

`references/verdict-schema.md` is the JSON contract between the triage subagents and the
scripts. `evals/evals.json` holds the trigger/behaviour cases (`make skill-evals`). First run:
2026-10-16, 77 open issues (the pass that produced this skill).
