---
name: gss-land-flow-no-interrupt
description: "On gss stacked-PR work, after the user picks a land option, run commit→push→PR→snapshot end-to-end without pausing for per-step confirmation."
metadata: 
  scope: account
  node_type: memory
  type: feedback
  originSessionId: 8dd7e936-9dd0-4e5c-a770-ebd121bb7b84
---

During the gss v1.0 stacked-PR series, once the user has chosen *how* to land a PR (e.g. "commit + PR + snapshot"), execute the entire flow — `git add`/`commit`, `git push`, `gh pr create`, mirror to `wip/gss-v1-staging`, update STATE.md, commit + push the snapshot — without stopping to re-confirm each git step.

**Why:** On 2026-05-21, after I presented landing options for PR-03 and the user selected the full option, they said: "you don't need to ask me for permissions anymore to continue on pushing … just complete everything." They want momentum on a 61-PR effort, not a prompt per git command.

**How to apply:** This does NOT remove the *initial* land-decision question — CLAUDE.md still requires presenting options via `AskUserQuestion` before git-mutating work, and that single confirmation is what authorizes the run. What it removes is intra-flow re-prompting between steps. Still stage files by explicit name, never `git add -A`, and still surface anything surprising (unexpected dirty state, a failing build) rather than plowing through. See [[gss-stacked-pr-workflow]].
