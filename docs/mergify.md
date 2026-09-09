# Merge model: solo maintainer, AI-accelerated, Mergify-queued

This doc is the canonical explanation of how changes land on `main` in this
repo: the rules, why each exists, and how
the model keeps AI-accelerated PR velocity **traceable and safe**. The
machine-readable versions live in [`.github/mergify.yml`](../.github/mergify.yml)
(merge gates as code) and [`.github/rulesets/main.json`](../.github/rulesets/main.json)
(reviewable snapshot of the GitHub ruleset).

## The landing flow

1. Work happens on a feature branch; **gss** is the canonical commit/push path
   (its approval-token handshake keeps a human in the loop for every publish).
2. A PR opens (usually via `gss pr`). CI runs — **every required check runs on
   every PR**; none are path-filtered (see "lessons" below).
3. The maintainer applies the **`ready-for-merge`** label (or comments
   `@mergifyio queue`). On a draft, the label pre-runs the heavy CI job; the
   PR queues once marked ready.
4. **Mergify's merge queue** takes over: updates the PR in place against the
   latest `main` (merge-update, no force-push), waits for the required checks
   on the updated code, then **squash-merges**. Entries are serialized —
   stale-green merges are impossible.
5. After a Mergify merge, the label swaps automatically:
   `ready-for-merge` → **`merged-by-mergify`** (the audit marker).

## Why Mergify (and not GitHub's native queue)

GitHub's merge queue requires organization-owned repositories; these repos are
owned by a personal account, where the feature is unavailable at any plan.
Mergify provides the same serialized-validation semantics as a GitHub App
(free tier: OSS + private repos ≤5 contributors). The `merge_group` CI
triggers remain in the workflows — inert under Mergify, instantly active if
the repos ever move to an org.

## The rules and why we picked them

| Rule | Where | Why |
|---|---|---|
| Squash-only merges | ruleset `pull_request.allowed_merge_methods` | History is squash-linear; Mergify squashes; removes the accidental-merge-commit hazard. |
| Strict up-to-date checks | ruleset `strict_required_status_checks_policy: true` | A PR can only merge with CI validated against the exact `main` it lands on. The queue satisfies this automatically; manual merges get the same guarantee. |
| In-place queue, no speculative trains | `merge_queue.max_parallel_checks: 1`, `batch_size: 1` | Compatible with strict up-to-date (speculative draft-PR checks are not); serial landing is the right shape for solo velocity. |
| Required checks always run on PRs | workflow `pull_request` triggers, unfiltered | A path-skipped required check never reports, deadlocking the queue ("waiting for queue conditions" forever). Fast checks always run; slow suites are demoted instead (below). |
| Slow suites demoted, not required | e.g. a kind-cluster e2e suite: weekly schedule + `urgent` issue on failure | Keeps per-PR latency low without losing the signal — a scheduled failure opens/bumps an `urgent`-labeled issue automatically. |
| Merge gates as code | `merge_protections` in `.mergify.yml` | Gate changes are reviewable PRs, not silent settings edits. Reports as the single "Mergify Merge Protections" check. |
| Ruleset snapshots | `.github/rulesets/*.json` + `make ruleset-snapshot` | Rulesets are settings with no file form; the snapshot is the PR-reviewable audit trail. Refresh after any settings change. |

## External contributions: the review gate

Threat model: as velocity (and visibility) grows, outside contributors — human
or bot — may open PRs. A label slip must never land an unreviewed external
change.

Defense in depth:

1. **Labels are maintainer-only.** GitHub only lets users with triage+
   permission apply labels, so an outside contributor cannot self-apply
   `ready-for-merge`.
2. **Conditional review requirement (the failsafe).** A Mergify protection
   requires `approved-reviews-by = sfc-gh-eraigosa` for any PR whose author is
   not the maintainer or `dependabot[bot]` (a GitHub-operated, trusted
   source). Even if the label lands by mistake, the queue refuses external
   PRs until the maintainer has submitted a real review. The same author
   guard is on the queue-entry rule itself.
3. **Why not GitHub's native "require 1 approval"?** It cannot discriminate
   by author, and PR authors cannot approve their own PRs — so it would
   deadlock every solo PR. Mergify's author-conditional rule requires review
   exactly where review is possible and meaningful, and stays out of the way
   of the maintainer's own work.

AI-assisted review of external PRs can layer on later; the human approval
remains the gate either way.

## Traceability of AI-performed work

Velocity with AI is only acceptable because every change stays attributable
and auditable:

- **Session links**: commits carry a `Claude-Session:` trailer and PR bodies
  a session URL — every AI-authored change traces to its full conversation.
- **Human-in-the-loop publishes**: the gss approval token is generated one
  Bash call before any push/PR, and repo policy requires an explicit
  confirmation prompt before commits — AI cannot silently publish.
- **`merged-by-mergify` label**: marks exactly which PRs landed via the
  queue's automated path.
- **Settings changes leave diffs**: ruleset snapshots + the GitHub audit log
  cover the one surface PRs can't.

## Troubleshooting: a green PR that keeps getting dequeued

Symptom: every required check is green, the branch is up to date and
conflict-free, yet Mergify dequeues with

> Mergify failed to merge the pull request. GitHub can't merge the pull
> request after 10 minutes of retrying. Repository rule violations found —
> N of 8 required status checks have not succeeded: M expected.

That is a **queue livelock**, and it is a CI-trigger bug, not a code problem.
Mergify writes `queued` / `dequeued` labels as it moves a PR through the
queue. If a workflow that owns required checks is dispatched by
`labeled` / `unlabeled` pull_request events, Mergify's own bookkeeping
restarts that workflow at the exact moment it holds the merge lock — the
required checks flip back to `expected`, GitHub refuses the merge, Mergify
exhausts its 10-minute retry budget and dequeues, and the `dequeued` label
write restarts the whole cycle.

How to tell it apart from a real failure: list the check runs for the head
SHA and look for many runs of the same job on **one unchanged commit**.

```sh
sha=$(gh pr view <N> --json headRefOid -q .headRefOid)
gh api "repos/{owner}/{repo}/commits/$sha/check-runs?per_page=100" \
  -q '.check_runs[] | "\(.started_at)\t\(.name)\t\(.conclusion)"' | sort -k2
gh api "repos/{owner}/{repo}/issues/<N>/timeline?per_page=100" \
  -q '.[] | select(.event | test("labeled")) | "\(.created_at)\t\(.event)\t\(.label.name)\t\(.actor.login)"'
```

If the label timeline and the extra runs line up second-for-second, it is
this bug. Two rules keep it away, both learned the hard way:

1. **Never let a label event restart a required check on a non-draft PR.**
   `.github/workflows/docker-image.yml` gates its three root jobs on the
   label name and draft status (see "MERGIFY QUEUE LIVELOCK" in that file's
   `on:` block, found via #264). A label event carries no new code, so there
   is nothing to re-validate.
2. **Never fold label events into a shared, cancelling concurrency group.**
   Entering the queue can add and remove labels in the same instant; two runs
   start together and one cancels the other, and a required check whose newest
   run is `cancelled` is not `success` (found via #243).

A job skipped by `if:` reports its required check as successful, so gating
jobs this way does not strand the merge.

## Break-glass (admin emergencies)

The ruleset grants `Repository admin` unconditional bypass (`bypass_actors`,
`bypass_mode: always`). In an emergency — Mergify outage, CI outage, anything
unplannable — the maintainer can bypass-merge (GitHub requires an explicit,
logged confirmation) or edit the ruleset directly. Obligations after the
fire: re-run `make ruleset-snapshot` and commit the diff if settings changed,
and note the bypass in the PR. Bypass is deliberate friction — never part of
the routine flow.

## Pending: promote `Go Lint (golangci-lint)` to a ruleset-required check

The `go-lint` job (`.github/workflows/docker-image.yml`) is STRICT — it runs
`make lint-go` with no `continue-on-error` — and is listed in
`.mergify.yml`'s `merge_protections`. It is **not yet in the GitHub ruleset's
native required-check list**, which is still the real enforcer (see Rollout
state below), so a Go regression currently fails that job without blocking a
merge.

Order matters, and it is the reverse of the obvious one:

1. **Merge the PR that adds the job first.** Adding the context to the ruleset
   beforehand would make every open PR wait on a check that cannot report —
   their branches predate the job — which is the "waiting for queue
   conditions" deadlock described above.
2. Then add `Go Lint (golangci-lint)` to the ruleset's required checks.
3. Then `make ruleset-snapshot` and commit the resulting
   `.github/rulesets/main.json` diff.

Why this check exists separately from the `lint` job: `make lint` is
warn-only across every linter, so a 212-finding Go backlog accumulated on
`main` under a permanently green "Lint" check. The Go category is now at zero
across all six `sdk/` modules, which is the condition the `lint` job names for
going strict "per linter as each category reaches zero". Shell (~90) and
markdown (~979) are not there yet, so `make lint` itself stays warn-only.

## Rollout state

`merge_protections` currently runs alongside the ruleset's native required
check list (dry run). Once the "Mergify Merge Protections" check is verified
reporting, the native list slims to just that check and the gate list becomes
single-sourced in `.mergify.yml`. This doc should be updated when that flips.
