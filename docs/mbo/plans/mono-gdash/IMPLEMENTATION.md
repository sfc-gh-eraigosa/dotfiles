# mono-gdash — implementation playbook

- **Slug:** mono-gdash
- **Date:** 2026-10-18
- **Status:** Ready to execute
- **Plan (source of truth):** [`../mono-gdash.md`](../mono-gdash.md) · spec [`../../specs/mono-gdash.md`](../../specs/mono-gdash.md)
- **Objective anchors:** issue [#410](https://github.com/sfc-gh-eraigosa/dotfiles/issues/410) · design PR [#411](https://github.com/sfc-gh-eraigosa/dotfiles/pull/411) · `docs/mbo/index.md` row `mono-gdash`

> This file is the **procedure**. It does not restate the plan: it tells a fresh session how to
> execute it, task by task, resumably. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | procedure, preconditions, worker map, loop, rules, kickoff prompt | read-only during the run (except §7 corrections and §8) |
| [`TRACKING.md`](./TRACKING.md) | evidence ledger: task rows, proof matrix, blockers, session log | after every task |
| [`TODO.md`](./TODO.md) | the cursor: first unchecked box = next action | as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action. Re-run the last
task's verification command before continuing: the ledger is a claim, the command is the proof.

## 1. Preconditions

| Check | Verify |
| :-- | :-- |
| #411 merged (design + spec + plan + trio on main) | `gh pr view 411 --json state -q .state` → `MERGED` |
| Grafana reachable, no tunnel | `curl -s -m 5 http://192.168.0.128:30300/api/health` → `"database": "ok"`, version 13.1.3 |
| Viewer token present, 0600, never printed | `stat -c %a ~/.config/gdash/lab.cred` → `600`; `curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $(cat ~/.config/gdash/lab.cred)" http://192.168.0.128:30300/api/search?limit=1` → `200` |
| Go toolchain per `sdk/mono/go.mod` | `go version` |
| Lint/security tools | `make lint-go` and `make security` run (green on main) |

## 2. Worker map

Created after #411 merges, with `gss feature worker add --feature mono-gdash --purpose <leaf>
--description "…" --json`. Paste each JSON result **verbatim** here and in TRACKING §0.

| Leaf | Purpose | Base | Created |
| :-- | :-- | :-- | :-- |
| `gdash` | Task 0, A1–A8 | `main` | — |
| `deck` | B1–B6, then C1–C2 | `main` | — |

The two leaves are independent (plan §6.1) and may run in parallel sessions.

## 3. The execution loop (every task)

1. **Locate:** the first unchecked `TODO.md` box gives the task; read its plan row and the spec rules it names.
2. **RED:** write the failing test, run it, and **see it fail**. Record the failure line in TRACKING.
3. **GREEN:** implement the minimum, then run until it passes.
4. **Gates:** the task's done-when, `go test ./...` in the module, `make lint-go` and `make security` for code tasks.
5. **Evidence:** `… | tee docs/mbo/plans/mono-gdash/evidence/<task>/<name>.txt` with a dated header line.
6. **Ledgers:** tick TODO, then update the TRACKING row (status, commit, evidence).
7. **Commit and push:** commit staging files by name; issue the approval token as a **separate**
   command (`git rev-parse HEAD > ~/.config/gss/approval.token`); then `gss feature checkpoint`.

## 4. Done-when gates

- **Each task:** its plan §4 done-when.
- **Each leaf:** all its tasks done; module coverage ≥ 60%; `auth`, `vars` and `render` ≥ 80%; lint and security green; PR labelled `ready-for-merge`.
- **The objective:** C2's evidence captured, TRACKING §3 all ticked, and `index.md` → `done`.

## 5. Hard rules

- **Secrets:** the token never appears in argv, output, logs, fixtures, commits or chat. Read
  it from the file in a subshell (`$(cat …)` inside the `curl` header) only for Task 0 capture,
  and scrub the fixtures. E12 is the guard.
- **mono only runs tools:** no import of `sdk/gdash` from `sdk/mono` (E24).
- **One keymap:** new keys go through `sdk/libs/tui`; mono's help, `--help` and README render from it (E23).
- **#407 configs keep working byte-for-byte** (E16).
- **Real demos only**, and never write a result you didn't observe.
- **Privacy guard:** no login names or home paths in committed files; use `$HOME`/`~`.
- **Never edit the spec or plan during the build.** A contract defect goes to TRACKING §4 and is escalated.

## 6. Command cheat-sheet

```bash
(cd sdk/gdash && go test ./... -cover)          # module tests
(cd sdk/mono  && go test ./... -cover)
(cd sdk/gdash && go test ./internal/render -run Golden -update)   # refresh goldens, then review the diff
bash sdk/gdash/build.sh && ~/opt/bin/gdash version
make lint-go && make security && make lint-markdown
gss feature conflicts                            # before each checkpoint once both leaves exist
git rev-parse HEAD > ~/.config/gss/approval.token   # separate command, then:
gss feature checkpoint
```

## 7. Resuming, recovery, and corrections

Fix wrong commands here and note it in the TRACKING session log.

- **2026-10-19:** on wslgiga1, `make lint-go` fails for **every** module because goenv can't find the repo's `.go-version` (1.27.2). Lint the module directly with `GOENV_VERSION=1.26.3 golangci-lint run ./...` (golangci-lint lives under that goenv version). Use the override **only** for golangci-lint: set for `go` itself, it makes `go test` and `build.sh` fail. CI is unaffected. If the Grafana token stops
working (401), stop: the owner re-issues it. Never create Grafana credentials from an agent
session without the owner's go-ahead.

## 8. Kickoff prompt (always CURRENT)

> **Maintenance rule:** exactly ONE prompt here, the one that starts the NEXT session.

```
Execute docs/mbo/plans/mono-gdash/ (dotfiles). Read, in order: IMPLEMENTATION.md, TRACKING.md,
TODO.md, then the plan ../mono-gdash.md and spec ../../specs/mono-gdash.md.
Scope now: the `gdash` leaf, starting at Task 0 (lab facts + fixtures) — preconditions in
IMPLEMENTATION §1 first; the Viewer token is at ~/.config/gdash/lab.cred (0600) and must never
be printed. Then A1 onward, following the §3 loop with evidence per task.
Stop and ask the owner if: the token is missing or returns 401; a dashboard the plan names
does not exist; a spec rule looks wrong (record it in TRACKING §4 first).
Done when: the gdash leaf's PR is green and labelled ready-for-merge, TRACKING updated, and
this prompt replaced with the next session's (the `deck` leaf, if not already running).
```
