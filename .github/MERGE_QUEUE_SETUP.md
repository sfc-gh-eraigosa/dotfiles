# Merge Queue — Admin Setup Runbook

> Reference: <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue>

This file documents the EXACT branch-protection-rule settings an admin must
apply in GitHub's UI to enable the merge queue for `main`. The workflow side
(the `merge_group:` trigger on `.github/workflows/docker-image.yml`) and the
contributor-facing docs (`CONTRIBUTING.md` → "Merge queue") have already
landed; the only remaining step is flipping the UI toggle.

## Where to apply

`Settings` → `Branches` → `Add branch protection rule` (or edit the existing
rule for `main`).

## Required settings

| Setting | Value | Why |
| :--- | :--- | :--- |
| Branch name pattern | `main` | The branch the queue gates. |
| Require a pull request before merging | ON | No direct pushes to `main`. |
| Require status checks to pass before merging | ON | Gates the queue on CI. |
| Require branches to be up to date before merging | ON | Lets the queue rebase cleanly. |
| Require merge queue | **ON** | The feature itself. |
| Merge method | **REBASE** | Linear history; matches `gss feature merged` retargeting. |
| Build concurrency | `2` | Max number of queue entries being built in parallel. |
| Minimum pull requests to merge | `1` | Don't wait for batching — merge as soon as one passes. |
| Maximum pull requests to merge | `3` | Cap per merge group (batch size ceiling). |
| Maximum pull requests to build | `3` | Cap on parallel synthetic-commit builds. |
| Wait time to meet minimum group size | `0` minutes | No artificial delay; we prefer latency over batching. |
| Status check timeout | `60` minutes (default) | Generous enough for Docker integration test. |

## Required status checks

Add each of these as a required check (they correspond to the four jobs in
`.github/workflows/docker-image.yml`):

- `lint`
- `unit-tests`
- `shell-tests`
- `build-and-validate`

> The check name GitHub sees is the **job ID** (the YAML key), not the
> `name:` field. Don't add `Lint (go + shell + markdown + actions)` —
> add `lint`.

> **`lint` is currently warn-only.** Its linters run and report findings,
> but the steps are `continue-on-error: true` so the job reports green
> (likewise `unit-tests` runs the coverage gate with `COVERAGE_ENFORCE=0`).
> This is intentional: a required check that is permanently red against the
> phase-1 baseline backlog (see `.ci-baseline-issues.md`) would deadlock the
> queue. It is still safe to require `lint` — a *new* hard failure (e.g. a
> missing tool, or actionlint catching broken YAML once those flags are
> dropped) will still go red. As follow-up phases burn the backlog down,
> drop the `continue-on-error` flags / set `COVERAGE_ENFORCE=1` to make the
> gates strict.

## Verification

1. Open any non-trivial PR against `main`.
2. Click **Merge when ready** instead of **Merge pull request**.
3. Confirm the PR shows "Queued" status and a new check run appears with
   ref `refs/heads/gh-readonly-queue/main/...`.
4. After CI passes, confirm `main` advances by a rebase (linear, no merge
   commit) and the PR is marked merged.

## Tuning notes

These values are conservative for a low-volume repo. Once CI is reliably
fast (say, < 5 minutes end-to-end), raise `Build concurrency` and
`Maximum pull requests to build` to e.g. `5` to let the queue absorb
burst contribution traffic without serializing. Leave `Minimum pull
requests to merge` at `1` unless lengthy CI starts making per-PR latency
worse than wait-then-batch.

## Rollback

If the queue causes operational pain, switching back is a single UI toggle:
turn **Require merge queue** OFF. The `merge_group:` trigger in the
workflow is harmless when the feature is disabled (GitHub simply never
dispatches the event), so the workflow file does not need to be reverted.
