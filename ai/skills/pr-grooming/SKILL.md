---
name: pr-grooming
description: >-
  Use when the user wants to groom, triage, or clean up the open pull requests of a repo:
  "groom the PRs", "review the open PRs", "which drafts are stale", "close old draft PRs",
  "rebase the dependabot PRs", "are the open PRs still mergeable", "refresh that old bugfix PR",
  or a periodic PR review — even when they do not say "grooming". Also use when a single old
  PR looks abandoned and the user asks whether it still applies. Not for listing PRs in chat
  (pr-list), not for the issue backlog (issue-grooming), and not for reviewing one PR's code.
---

# pr-grooming — decide the fate of every open PR against `main`

A PR grooming pass answers, for every open PR, **does this still apply**, **what does it need
to land** (a rebase, a refresh, a decision), and **who acts next**. The output is a shorter PR
list where each survivor is rebased, CI-verified and either queued or clearly waiting on a
human, plus a report the owner can scan in a minute.

Why the rigor: in this repo drafts are cheap (`gss feature worker add` opens one per leaf), so
they pile up — design docs written for a CLI that was retired, plans that later merged under a
different objective, WIP parked "to decide later". A PR's own description says what it *was*;
`main`, `docs/mbo/index.md` and the merged-PR list say what is *still true*. Only the evidence
decides, and every close names its successor so nothing is lost (branches are never deleted).

## 0 — Setup (one command)

```bash
bash ai/skills/pr-grooming/scripts/pr-snapshot.sh -o <scratch>/pr-groom
```

`triage.tsv` gives each PR a first-cut **category**, age, draft state, merge state and CI
state; `prs.txt` has bodies, files, comments and check names; `mbo-rows.txt` lists the
`docs/mbo/index.md` rows that mention each PR or its branch slug. Read all three before
judging anything. Correct the category by hand where the heuristic is wrong (a `fix:` title on a
docs change is not a bugfix).

Hooks note (this repo): the privacy hook blocks command text containing the login name or an
absolute home path — refer to scratch and worktree dirs by a glob or `~`. The Bash tool runs
zsh; build flag lists as arrays.

## 1 — Categories and what each one needs

| Category | Question to answer | Typical outcome |
| :-- | :-- | :-- |
| `mbo-draft` (draft touching `docs/mbo/`, or an objective's design/plan) | Is the objective still live? Is there a **merged** row in `index.md` (or a merged PR) covering the same slug — a duplicate or a conflicting plan? | prefer the merged MBO: close the draft as `duplicate`; a live objective that is merely behind → `rebase-only`; a design written for something the repo no longer has → `stale` |
| `stale-draft` (any other draft ≥ 30 days) | Does the work still apply to `main`, and is anyone going to finish it? | clearly abandoned → `stale` with the reason; still plausible → `rebase-only` and a `comment` asking for the keep/drop decision |
| `bugfix` | Does the bug still reproduce on `main`? | still reproduces → `refresh`: rebase, resolve conflicts, prove the PR fixes it, promote to ready with a comment asking for a review refresh; no longer reproduces → `stale` (name the commit that fixed it) |
| `dependabot` | Is it a patch/minor bump with green CI after a rebase? | rebase; green patch/minor → `ready` (label `ready-for-merge`); major or red → `leave` with a comment on what failed |
| `other` | Behind `main`? CI green? | behind → `rebase-only`; green and current → `leave` |

Age alone is not evidence: a 9-day-old draft of a live objective stays; a 140-day-old design
for a retired CLI goes. "Clearly stale" means at least two of: the thing it changes no longer
exists on `main` (path, tool, CLI), a merged PR or index row supersedes it, its own review
comments say it needs rework that never came, or it has conflicts nobody has touched for a
month.

## 2 — Verdicts

| Verdict | Means | Action |
| :-- | :-- | :-- |
| `stale` | no longer applies, or abandoned beyond doubt | `close-stale` — comment says why and where the tracker lives (issue number), branch kept |
| `duplicate` | a merged PR / index row already covers it | `close-stale` — comment names the merged PR or row |
| `refresh` | still wanted and still a bug; rebased and re-proven in this pass | `ready` — promote + comment "please re-review" with the proof |
| `ready` | rebased, CI green, safe to land without review (patch/minor bump, docs-only fix) | `ready-for-merge` — Mergify lands it |
| `rebase-only` | still valid; just behind `main` | rebase with `pr-rebase.sh`, verify CI, no state change |
| `leave` | current and green, or waiting on a human decision | `none` or `comment` |

A PR that is the **current session's own** work, or already carries `ready-for-merge`, is
`leave` — the queue owns it.

## 3 — Verify before you judge

For each PR, establish the facts the verdict rests on:

- **Supersession**: `grep -n <slug-or-number> docs/mbo/index.md` (the snapshot's
  `mbo-rows.txt`), and `gh pr list --state merged --search "<leaf or slug>"` — a plan or
  leaf often merged under a PR that never cites the draft.
- **Still applies**: open the paths the PR touches on `main` (`git show origin/main:<path>`);
  a design that cites a retired tool, a `docs/plans/` path that moved to `docs/mbo/plans/`,
  or a script that was rewritten, is evidence of staleness.
- **Rebase state**: `mergeStateStatus` from the snapshot — `BEHIND` rebases cleanly, `DIRTY`
  has conflicts (see what they are before deciding whether a refresh is worth it).
- **Bug still there** (bugfix only): reproduce on `origin/main` with the PR's own test or the
  issue's steps *before* rebasing; after the rebase run the same check on the PR head.
- **CI**: after any push, wait for the rollup (`gh pr checks N --watch`) and read failing
  job names — a failure that predates the rebase (an old gosec rule, a since-fixed flake)
  usually clears; one that persists is the PR's own.

Fan out when the list is long: one subagent per category, each given the PR numbers, the
snapshot dir, the tables above and the exact JSON shape in `references/verdict-schema.md`,
writing `verdicts-<category>.json` into the scratch dir; merge with
`jq -s 'add | sort_by(.number)'`. A pass of a handful of PRs is triaged inline.

## 4 — Rebase and refresh mechanics

```bash
bash ai/skills/pr-grooming/scripts/pr-rebase.sh <N> --work-dir <scratch>/pr-groom/wt   # BEHIND → push
```

The helper rebases in a throwaway worktree and pushes with `--force-with-lease`; on a
conflict (exit 2) it leaves the worktree in place and names the files. Resolve there, keep
the PR's intent (for docs ledgers prefer the newer `main` text and re-apply the PR's additions),
`git rebase --continue`, push, remove the worktree. A gss worker branch can instead be
rebased with `gss feature checkpoint --worker <ref>` from its own worktree.

For a `refresh`, add the proof to the comment: the failing command on `main`, the passing one
on the PR head, the CI link. For `ready` on a dependency bump, the proof is the green rollup
after the rebase; set `ci_verified: true` in the verdict only after reading it.

Never rebase a PR whose head lives in a fork, and never delete a branch — closing is reversible,
deleting is not.

## 5 — Apply, then report

```bash
bash ai/skills/pr-grooming/scripts/pr-apply.sh <scratch>/pr-groom/verdicts.json --dry-run
bash ai/skills/pr-grooming/scripts/pr-apply.sh <scratch>/pr-groom/verdicts.json
bash ai/skills/pr-grooming/scripts/pr-report.sh <scratch>/pr-groom/verdicts.json
```

Read the dry run — it is the review step. `pr-apply.sh` closes only `stale`/`duplicate` and
only with a `close_comment`, promotes only `refresh`, labels `ready-for-merge` only for
`ready` with a non-draft, passing (or `ci_verified`) head, retries transient `gh` failures
three times and logs to `applied.log`; compare its row count with the verdicts before calling
the pass done. Rebases happen before apply (§4), not inside it.

Finish with the `pr-report.sh` tables in the final message and — when a close or a label
changed an objective's state — one row in the `docs/mbo/index.md` audit log, plus the
objective rows it touched (a closed design draft leaves its issue as the tracker; say so in
the row). Commit through the normal gss flow. Issues that a closed PR was going to close stay
open; if their body still points at the PR, run `issue-grooming` on them.

**Targeted re-check** ("is #N still worth keeping?"): snapshot, read that PR block, verify
inline, apply with `--only N`.

## Common mistakes

- **Closing by age.** 30 days is the threshold for *looking*, not for closing. Cite the
  superseding row, the missing path or the unanswered review comment.
- **Promoting without proof.** `refresh` requires the bug to reproduce on `main` and not on
  the PR head, in this pass; `ready` requires the post-rebase rollup, not the pre-rebase one.
- **Treating a stale check as the PR's fault.** Re-run CI after the rebase before reading a
  failure; many are the base's old gates.
- **Losing the tracker.** A closed design/plan draft must leave its issue open and mention,
  in the close comment, where the artifact lives (the kept branch) and what replaces it.
