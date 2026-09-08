# gcfg — implementation playbook

- **Slug:** gcfg
- **Date:** 2026-09-05
- **Status:** Ready to execute (after design PR approval)
- **Plan (source of truth):** [`../gcfg.md`](../gcfg.md) · spec [`../../specs/gcfg.md`](../../specs/gcfg.md) · design [`../../designs/gcfg.md`](../../designs/gcfg.md)
- **Objective anchors:** issue #284 · design PR #285 · `gss feature gcfg`

> This file is the **procedure**. It does not restate the plan — it tells a fresh agent
> session how to execute the plan, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, per-task loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live **state ledger**: per-task status/commit/evidence, proof matrix, blockers, append-only session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The **cursor**: plan tasks expanded into ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md`
says what has been proven. Re-run the last verification command before continuing —
the ledger is a claim, the command is the proof.

## 1. Preconditions

| Check | Verify command | Expect |
| :-- | :-- | :-- |
| Go toolchain | `go version` matches `cat .go-version` | same minor |
| gh login (repo scope) | `gh auth status` | logged in; scopes include `repo` |
| design PR merged (plan approved) | `gh pr view <design PR> --json state` | `MERGED` |
| feature registered | `gss feature list --feature gcfg --json` | feature row present |
| clean base | `git -C <worktree> status --short` | empty |
| actionlint available (for P3/P5) | `actionlint --version` | prints |

## 2. Worker map

Captured VERBATIM from `gss feature worker add --json` when each leaf worker is created
(CAP-C). Blocking-first order per plan §6.1: `ghapp` ∥ `core` → `fam-repo-a`,
`fam-repo-b`, `fam-org`, `auth` (also ← ghapp), `actions`, `tui` → `adoption`.

| Leaf | worker_ref | branch | worktree_path | base | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design | `gcfg/edward-raigosa/design` | `feature/gcfg/edward-raigosa/design` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gcfg/edward-raigosa/design` | main | **superseded** — its two commits ship in the build worker; PR #285 closed 2026-09-07 |
| ghapp + core | `gcfg/edward-raigosa/build` | `feature/gcfg/edward-raigosa/build` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gcfg/edward-raigosa/build` | main (re-targeted from the design branch 2026-09-07) | **P0 + P1 done**, [#287](https://github.com/sfc-gh-eraigosa/dotfiles/pull/287) |
| fam-repo-a / fam-repo-b / fam-org | (create after #287 lands) | | | main | next |
| auth / actions / tui | (create after #287 lands) | | | main | next |
| adoption | (create last) | | | main | last |

The build was done sequentially in one PR (the `mbo-plan` default), so a single worker
`gcfg/edward-raigosa/build` carried P0 and P1 in plan order. The remaining leaves are
independent and can run in parallel off `main` once #287 is in.

## 3. The execution loop (every task)

1. Locate: first unchecked `TODO.md` box → its plan task; read the plan task fully.
2. RED: write the failing test first; run it; **verify it fails**; record the failure line.
3. GREEN: implement the minimum; run to pass.
4. Gates: `go vet ./...`, coverage bar for the package, `make lint-shell lint-portability`
   for any shell edit, actionlint for any workflow, token-leak grep for any cmd output test.
5. Evidence: `tee` the gate command into `plans/gcfg/evidence/<task>/<date>.txt`.
6. Ledgers: tick `TODO.md`; update the `TRACKING.md` task row (status, commit, evidence).
7. Confirm via the interactive prompt, then commit with the plan's exact message, staging
   by explicit name. `gss feature checkpoint` (push + draft PR refresh).

## 4. Done-when gates

- Per leaf: plan §6.1 table.
- Objective stop condition: `make gcfg-verify` green in CI on this repo with its exported
  `.github/gcfg.yaml`; both workflows installed and evidenced red→green; `gcfg auth doctor`
  evidence for the `gh` token and an App token; the two one-off scripts removed; every
  TRACKING row `done` with SHA + evidence.

## 5. Hard rules

- Frozen contracts in plan §3 (schema shape, `Family`/`Client`/engine signatures, CLI,
  exit codes, workflow templates). A contract defect is a TRACKING blocker, never a
  silent patch.
- No network in tests. Every GitHub call goes through `gh.Client`.
- Never print/store/export a token or secret value; every cmd test greps for the fixture
  token and fails on a hit.
- Deletes only under `ownership: full`; apply always re-reads.
- Coverage 80/90/90; `go vet` clean; shell gates fail open; portability scan clean.
- Confirmation before every commit/checkpoint; never run `install.sh` from a worktree.
- Evidence before assertions: a task is done when its gate output is in `evidence/`.

## 6. Command cheat-sheet

```
cd sdk/gcfg && go test ./... -cover                 # unit + coverage
cd sdk/gcfg && go vet ./...
make gcfg-test gcfg-e2e                             # bars + binary e2e (once wired)
cd sdk/ghapp && go test ./... -cover
make lint-shell lint-portability                    # after shell edits
actionlint .github/workflows/gcfg-*.yml             # after workflow edits
go run ./sdk/gcfg verify --markdown                 # live, this repo (gh token)
gss feature checkpoint                              # push + draft PR refresh (confirm first)
```

## 7. Resuming, recovery, and corrections

Fix wrong commands here freely and note it in the TRACKING session log. Never edit the
plan/spec as part of the build — escalate contract defects via TRACKING blockers. If the
gss registry drifts from reality: `gss feature audit --feature gcfg --json`, then
`--repair`, then `gss feature list --feature gcfg --tree`.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.

Mission: execute `docs/mbo/plans/gcfg.md` phase **P2 (families)**, TDD, in a new
`gss feature gcfg` worker off `main`. P0 (ghapp) and P1 (gcfg core) landed in
[#287](https://github.com/sfc-gh-eraigosa/dotfiles/pull/287); the engine, the `Family`
contract, the recording fake and the two reference families are already there.

Read first: `docs/mbo/plans/gcfg.md` (§3.1 for each family's YAML shape, §4 P2), this
file §3–§5, `docs/mbo/plans/gcfg/TODO.md` (first unchecked box), and — as the shape to
copy — `sdk/gcfg/internal/family/general/` (a fixed set of scalars, one key table shared
by Export and Diff) and `.../security/` (three endpoint shapes behind one family, plus
`DiffAfterApply` for a write GitHub accepts and ignores).

Scope in order: **P2-T1 rulesets** (also imports `.github/rulesets/*.json` on export,
updates by name, deletes only under `full`) → P2-T2 actions → P2-T3 labels (pagination
past 100) → P2-T4 autolinks → P2-T5 environments → P2-T6 secrets (names only) +
webhooks (no secret) → P2-T7 collaborators + pages (report-only) → P2-T8/T9/T10 the org
families. Each task: RED → GREEN → gate → evidence → ledger → confirmed commit →
checkpoint. A list family is where `ownership: full` finally bites — extras become drift
and `apply` deletes them, so every one needs the create/update/delete matrix and a
pre-image on the delete.

Stop and ask (interactive prompt) before every commit and before any live `apply` on
this repo. Blocked → TRACKING §4 with the real command output. Done when every P2 family
has fixtures, an export golden, a diff matrix and a recorded apply body, `gcfg-ci` is
green at 80/90/90, and `gcfg export` on this repo covers every family that is live.

**Carried over from P0, unfinished:** one real `ghapp create` + `ghapp token --repo
sfc-gh-eraigosa/dotfiles` + `ghapp doctor` → `evidence/ghapp/` (redacted). Two callback
windows expired without the browser step; it needs a browser that can reach
`127.0.0.1:8479` on the host running it.
