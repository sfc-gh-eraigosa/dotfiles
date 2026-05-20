# gss v1.0 — Execution State (live cursor)

This file is the **handoff snapshot** for the gss v1.0 stacked PR series.
It lives on the long-running draft PR branch `wip/gss-v1-staging` and is
refreshed after every playground PR opens or merges.

The branch this file lives on is **never merged** — it exists only so a
new machine can pick up the work without cloning the playground.

The real per-PR review surface is the stack of small PRs in the
**playground** repo (see "Cursor" below). When v1.0 is fully validated,
PR-61 (the final integration) opens *its own* non-draft PR against
`dotfiles:main` and lands the validated work.

## Cursor

- **Most recent PR opened**: PR-02 — `internal/git` Runner interface +
  recording fake (#24 in playground).
- **Playground branches in flight**:
  - `test_gss` — integration trunk (root of the stack).
  - `pr01-internal-errors` — PR #23, awaiting human merge to `test_gss`.
  - `pr02-internal-git-runner` — PR #24, stacked on PR-01, awaiting merge.
- **Next PR**: PR-03 — `internal/gh` Client interface + per-verb fake
  (Architect heads-up: `gh/fake` needs per-verb scripting, not the FIFO
  shape used in `git/fake`).
- **PRs remaining in plan**: 59 (PR-03 through PR-61).
- **This staging snapshot**: reflects playground branch
  `pr02-internal-git-runner` at SHA `e8c9b8a`.

## Handoff procedure (new machine)

1. **Clone dotfiles**:
   ```
   git clone <dotfiles-remote> ~/git/dotfiles
   cd ~/git/dotfiles
   git checkout wip/gss-v1-staging
   ```
2. **Optional — clone the playground** for the full per-PR commit history
   and PR review surface:
   ```
   git clone <playground-remote> ~/GitHub/playground
   cd ~/GitHub/playground
   git checkout pr02-internal-git-runner
   ```
3. **Tooling install** (Phase 3 prerequisites):
   ```
   go install github.com/google/go-licenses@latest
   brew install git-machete           # or: pip install git-machete
   ```
4. **Auth check**:
   ```
   gh auth status
   ```
   PRs in this series have been opened under `sfc-gh-eraigosa`. If this
   machine uses a different `gh` identity, switch before continuing.
5. **Resume**: invoke the agent and say "continue" — it will read
   `src/gss/docs/plan.md`, find the next un-merged deliverable per the
   cursor above, and proceed.

## What's in the staging branch right now

Additive to the existing `src/gss/` tree (the classic `cmd/`, `main.go`,
`go.mod` with cobra all unchanged):

- `src/gss/LICENSE` — Apache 2.0, mirrored from upstream.
- `src/gss/internal/errors/` — 14 sentinels, exit-code map (10–29), JSON
  envelope with control-char + ANSI + worker_ref defence (PR-01).
- `src/gss/internal/git/` — `Runner` interface + `SystemRunner` real
  impl + `fake.Runner` recording fake (PR-02).
- `src/gss/docs/STATE.md` — this file.

The classic gss code at `src/gss/cmd/*.go`, `main.go`, etc. is
**untouched**. The new `internal/` packages are foundation layers that
the classic code will start using at PR-22 (the `internal/classic/push.go`
orchestrator) and beyond.

## Open carry-forward notes

| # | Source | Item | Disposition |
|---|--------|------|-------------|
| 1 | PR-01 Security LOW | `maxValidationField` / `maxValidationReason` constants declared but not enforced. | Defer to PR-07 (identity migration) or fold into a small follow-up. |
| 2 | PR-01 Security LOW | `workerSuffixRe` unused; `<purpose>` and `<suffix>` not structurally split. | Defer to PR-07. |
| 3 | PR-01 Security LOW | `requiredCodeOnEmpty` cosmetic constant. | Defer to PR-07. |
| 4 | PR-02 QA YELLOW | No compile-time `var _ git.Runner = (*fake.Runner)(nil)` in fake package. | Fold into PR-03 or PR-50. |
| 5 | PR-02 QA YELLOW | No `name == ""` contract test. | Fold into PR-03 or PR-50. |
| 6 | PR-02 QA YELLOW | Unbounded `bytes.Buffer` for huge `git log`. | Track for a future hardening PR. |
| 7 | PR-02 QA YELLOW | Combined-Stdout+Stderr+Err ordering not pinned. | Fold into PR-03 or PR-50. |
| 8 | PR-02 Architect | PR-50 must add CI grep enforcing "no `os/exec` outside `internal/git/` and `internal/gh/`". | Track for PR-50. |
| 9 | PR-02 Architect | PR-03 (`gh/fake`) must use per-verb scripting, not the FIFO shape from `git/fake`. | Apply at PR-03 dispatch time. |

## Sync log

| Date | Synced from | SHA | Notes |
|------|-------------|-----|-------|
| 2026-05-20 | `playground:pr02-internal-git-runner` | `e8c9b8a` | Initial staging snapshot — PR-01 (sentinels + JSON envelope) + PR-02 (`internal/git` Runner + fake). Tests pass in dotfiles context; `go build ./internal/...` clean. |
