---
name: issue-grooming
description: >-
  Use when the user wants to groom, triage, prioritize, or clean up the GitHub issue backlog of
  a repo: "groom the issues", "triage the backlog", "which issues are already done", "prioritize
  the open issues", "close stale issues", "label issues 1-5", "refactor old issues to match the
  repo", or a periodic backlog review — even when they do not say "grooming". Also use when a
  single old issue looks stale and the user asks whether it is still valid. Not for reviewing or
  merging PRs (pr-list / gss) and not for planning one objective (mbo-plan).
---

# issue-grooming — groom the backlog against what actually shipped

Grooming answers three questions for every open issue, with evidence from the repo rather than
from the issue's own text: **is it already addressed**, **how important is it now**, and **does its
body still describe work that fits the repo**. The output is a labelled, trimmed backlog and a
report the owner can scan in a minute.

Why the rigor: issues are written at a moment in time. In this repo code moves (`src/` → `sdk/`),
leaves of a plan merge under PR titles that never mention the issue, follow-ups get absorbed by
later objectives, and a bot keeps filing reports. An issue's text is the *claim*; `main`, merged
PRs, and `docs/mbo/index.md` are the *evidence*. Only the evidence decides.

## 0 — Setup (one command)

```bash
bash ai/skills/issue-grooming/scripts/groom-snapshot.sh -o <scratch>/groom
bash ai/skills/issue-grooming/scripts/groom-labels.sh      # ensures priority:1..5 exist (idempotent)
```

The snapshot writes `issues.json`/`issues.txt` (bodies + comments), `labels.json`,
`merged-prs.json` (with the issues each PR closed), and `clusters.txt` (a first-cut grouping).
Read `clusters.txt` and regroup by hand: a cluster is a set of issues that share a program,
tool, or plan (e.g. all `fleet-connect` sub-issues, all `gss` tech-debt, one bot's reports).
Clusters are the unit of triage because verifying them needs the same files.

Hooks note (this repo): the privacy hook blocks command text containing the login name or an
absolute home path, so refer to scratch dirs by a glob or `~`. The Bash tool runs zsh; build
flag lists as arrays.

## 1 — Verify, one cluster at a time

For each cluster, establish what shipped **before** judging any issue:

- `docs/mbo/index.md` row(s) and the plan's `TRACKING.md` ledger — the state column and the
  "pending" lists are the fastest map of what landed and what is still owed.
- `merged-prs.json` — grep titles for the slug / leaf names and the issue number; many fixes
  cite the issue only in the PR title (`… (#331)`), not in a closing reference.
- The code or docs the issue names — open the path on `main` (`git show main:<path>` when the
  working tree is on a branch) and confirm the verb/flag/test/doc exists.

Clusters are independent, so fan them out: one subagent per cluster, each given the issue
numbers, the verification pointers above, the rubric below, and the exact JSON return shape in
`references/verdict-schema.md`. Insist on *evidence you actually checked* (paths, PR numbers,
SHAs) — a verdict without a citation is a guess. Merge the returned arrays into one
`verdicts.json`.

## 2 — Verdicts

| Verdict | Means | Action |
| :-- | :-- | :-- |
| `addressed` | the whole ask is on `main` (code, docs, or a merged PR) | close as **completed** with an evidence comment |
| `obsolete` | superseded by a later objective, or the repo no longer has the thing it describes | close as **not planned** with a comment naming the successor |
| `partially` | some of it shipped; the rest is still wanted | keep open; refresh the body so it lists only what remains |
| `open` | nothing shipped; still valid | keep open; relabel |

A tracker whose remaining work is *only* a human/live gate (run it on a real host, sign off) is
`partially`, not `addressed` — closing it would lose the gate.

## 3 — Priority 1–5 (one label per issue, `priority:N`)

| P | Rule |
| :-: | :-- |
| 1 | Blocks daily work, or a correctness/safety bug in a shipped daily-use tool (install, git flow, CI gate, fleet). |
| 2 | In-flight objective with design/spec on `main` and a clear next build step. |
| 3 | Valuable follow-up to a shipped feature; nothing is blocked without it. |
| 4 | Nice-to-have, speculative, or a large program that has not started. |
| 5 | Parked, likely obsolete or superseded; kept open only as a reference. |

Re-evaluate from scratch; ignore any priority label the issue already carries — it reflects the
moment it was filed. Within a program, sub-issues inherit the parent's priority unless they are
the current blocking leaf (then one step higher).

## 4 — Refactor bodies that no longer fit

Refresh a body when it carries stale paths (`src/<tool>` for Go that now lives in `sdk/`),
leaves or PRs that already merged, sequencing that changed, or a checklist that is mostly done.
Write the **remaining** work as the new body — a short "Groomed <date>" header, what shipped
(with PR links), what remains, and the next step. The apply script folds the original body under
a `<details>` block automatically, so never paste the old text back in; rewrite.

Do not refactor a bot-generated report (drift reports, eval batches): close it if a newer run
exists, otherwise leave it and lower its priority.

## 5 — Apply, then report

```bash
bash ai/skills/issue-grooming/scripts/groom-apply.sh <scratch>/groom/verdicts.json --dry-run
bash ai/skills/issue-grooming/scripts/groom-apply.sh <scratch>/groom/verdicts.json
bash ai/skills/issue-grooming/scripts/groom-report.sh <scratch>/groom/verdicts.json
```

Read the dry-run before sending: it is the only review step. `groom-apply.sh` sets exactly one
`priority:N` label, refuses to close anything that is not `addressed`/`obsolete`, requires a
`close_comment` for every close, and logs each action to `applied.log`. Closing is reversible
(`gh issue reopen`), so a grooming pass may close without asking; it must never delete content.

Finish with the report from `groom-report.sh` (counts, then one table per verdict) in the
final message, and — in this repo — one new row in the `docs/mbo/index.md` audit log naming the
issues closed and any objective rows whose state changed. Commit that row through the normal
gss flow.

## Common mistakes

- **Trusting the title.** A sub-issue titled "PR 3 · leaf C" may have merged under a PR named
  after the branch. Grep `merged-prs.json` for the leaf name and the slug, not the issue number.
- **Closing a human gate.** TRACKING rows that read "pending on the operator" keep the issue
  open as `partially`.
- **Counting absence as proof.** "grep found nothing" only proves something when the grep
  demonstrably ran against `main` and the term is the one the code would use.
- **Re-labelling without the rubric.** Two graders with different rubrics produce a backlog
  nobody trusts; every subagent gets the same table from §3, verbatim.
