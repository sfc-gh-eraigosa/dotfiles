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

- **Most recent PR opened**: PR-03 — `internal/gh` Client interface +
  per-verb recording fake (#25 in playground).
- **Playground branches in flight**:
  - `test_gss` — integration trunk (root of the stack).
  - `pr01-internal-errors` — PR #23, awaiting human merge to `test_gss`.
  - `pr02-internal-git-runner` — PR #24, stacked on PR-01, awaiting merge.
  - `pr03-internal-gh-client` — PR #25, stacked on PR-02, awaiting merge.
- **Next PR**: PR-04 — `internal/version/` + `build.sh` ldflags update
  (exports `Version`/`Commit`/`BuildDate`/`Dirty` via `-X`; `gss version`
  reads from this package, not from `main`). Stacks on PR-03.
- **PRs remaining in plan**: 58 (PR-04 through PR-61).
- **This staging snapshot**: reflects playground branch
  `pr03-internal-gh-client` at SHA `4f6bedc`.

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
   git checkout pr03-internal-gh-client
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
- `src/gss/internal/gh/` — `Client` interface (`PRCreate`/`PREdit`/
  `PRReady`/`PRView`/`PRList`/`RepoView`/`AuthStatus`) + `SystemClient`
  real impl over an `Exec` seam + `fake.Client` stateful, per-verb
  scriptable fake + `testdata/gh_responses/*.json` fixtures (PR-03).
- `src/gss/docs/STATE.md` — this file.

The classic gss code at `src/gss/cmd/*.go`, `main.go`, etc. is
**untouched**. The new `internal/` packages are foundation layers that
the classic code will start using at PR-22 (the `internal/classic/push.go`
orchestrator) and beyond; `internal/gh` first gets wired in at PR-24
(the `internal/classic/pr.go` orchestrator).

## Open carry-forward notes

| # | Source | Item | Disposition |
|---|--------|------|-------------|
| 1 | PR-01 Security LOW | `maxValidationField` / `maxValidationReason` constants declared but not enforced. | Defer to PR-07 (identity migration) or fold into a small follow-up. |
| 2 | PR-01 Security LOW | `workerSuffixRe` unused; `<purpose>` and `<suffix>` not structurally split. | Defer to PR-07. |
| 3 | PR-01 Security LOW | `requiredCodeOnEmpty` cosmetic constant. | Defer to PR-07. |
| 4 | PR-02 QA YELLOW | No compile-time `var _ git.Runner = (*fake.Runner)(nil)` in fake package. | ✅ Done in PR-03 — both `gh.SystemClient` and `fake.Client` assert `var _ gh.Client`. (Retro-fit on `git/fake` still open → fold into PR-50.) |
| 5 | PR-02 QA YELLOW | No `name == ""` contract test. | ✅ Done in PR-03 — empty/invalid-input contract pinned on every gh mutating verb. (git/fake retro-fit → PR-50.) |
| 6 | PR-02 QA YELLOW | Unbounded `bytes.Buffer` for huge `git log`. | Track for a future hardening PR. |
| 7 | PR-02 QA YELLOW | Combined-Stdout+Stderr+Err ordering not pinned. | ✅ Done in PR-03 (gh analog) — gh `Exec` returns stdout only, folds stderr into `*ExecError`; contract tested. (git/fake retro-fit → PR-50.) |
| 8 | PR-02 Architect | PR-50 must add CI grep enforcing "no `os/exec` outside `internal/git/` and `internal/gh/`". | Track for PR-50. |
| 9 | PR-02 Architect | PR-03 (`gh/fake`) must use per-verb scripting, not the FIFO shape from `git/fake`. | ✅ Done in PR-03 — per-verb error scripting (`ScriptError(verb, …)`); proven by `TestFakeClient_PerVerbScripting`. |
| 10 | PR-03 | `gh.systemExec.Run` (the real-`gh` subprocess shell) is uncovered by unit tests by design — exercising it needs a live `gh` + repo. | Optional: add an integration test behind a build tag / `GSS_GH_INTEGRATION=1` env gate in a future hardening PR. |

## Sync log

| Date | Synced from | SHA | Notes |
|------|-------------|-----|-------|
| 2026-05-20 | `playground:pr02-internal-git-runner` | `e8c9b8a` | Initial staging snapshot — PR-01 (sentinels + JSON envelope) + PR-02 (`internal/git` Runner + fake). Tests pass in dotfiles context; `go build ./internal/...` clean. |
| 2026-05-21 | `playground:pr03-internal-gh-client` | `4f6bedc` | PR-03 — `internal/gh` `Client` interface + `SystemClient` (over an `Exec` seam) + stateful per-verb `fake.Client` + `testdata/gh_responses/*.json`. 31 tests; coverage gh 77.7%, fake 87.8%; build + full module tests clean in dotfiles context. No new deps. `-race` skipped on the aarch64 dev host (47-bit VMA vs ThreadSanitizer's 48-bit, `unsupported VMA range`); x86 CI covers it. PR #25 stacked on PR-02. |
