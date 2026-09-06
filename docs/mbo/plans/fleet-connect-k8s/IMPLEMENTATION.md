# fleet-connect-k8s — implementation playbook

- **Slug:** fleet-connect-k8s
- **Date:** 2026-09-05
- **Status:** Ready to execute once `fleet-connect` PR 3 (registry) has merged
- **Plan (source of truth):** [`../fleet-connect-k8s.md`](../fleet-connect-k8s.md) · spec
  [`../../specs/fleet-connect-k8s.md`](../../specs/fleet-connect-k8s.md) · design
  [`../../designs/fleet-connect-k8s.md`](../../designs/fleet-connect-k8s.md)
- **Objective anchors:** design issue *(pending — `gh issue create`, see `docs/mbo/index.md`)* ·
  parent framework issue [#266](https://github.com/sfc-gh-eraigosa/dotfiles/issues/266) ·
  `docs/mbo/index.md` row `fleet-connect-k8s`

> This file is the **procedure**. It does not restate the plan. The plan wins any conflict.

## 0. The three files

| File | Role | Written by |
| :-- | :-- | :-- |
| `IMPLEMENTATION.md` (this) | Procedure: preconditions, worker map, loop, hard rules, kickoff prompt | Read-only during the run (except §7 corrections) |
| [`TRACKING.md`](./TRACKING.md) | Live state ledger: task rows, proof matrix, blockers, session log | Updated after **every** task |
| [`TODO.md`](./TODO.md) | The cursor: the plan's 8 tasks as ordered micro-steps | Checkboxes ticked as you go |

**Resumption rule:** the first unchecked box in `TODO.md` is the next action; `TRACKING.md`
says what has been proven. Re-run the last verification command before continuing.

## 1. Preconditions

| Precondition | Verify with |
| :-- | :-- |
| `fleet-connect` leaves A, B, C merged (contract, protocol, registry + `provider serve`) | `gss feature list --feature fleet-connect --tree`; `cd sdk/fleet && go doc ./pkg/provider Tunnel` shows `Keeper`; `go doc ./pkg/provider ReservedKeys` |
| For T7's manager test: `fleet-connect` leaf H merged | `go doc ./internal/bridge Manager` |
| For T8's live gates: leaves E, F, G merged | `fleet ls --help`, `fleet bridge --help` |
| Toolchain and gates green before starting | `cd sdk/fleet && go test -race ./... && gofmt -l . && go vet ./...`; `./scripts/test.sh`; `make lint-shell && make lint-portability` |
| The three clusters reachable for fixture capture and live gates | `ssh <nano> 'k3s kubectl get ns'`; `ssh <gigabyte> 'kubectl get ns'`; `ssh <pi> '~/opt/bin/kubectl get ns'` |
| Working tree is the objective's worker worktree | `git rev-parse --show-toplevel && git branch --show-current` |
| `.gitignore` opts in `testdata/**` and `evidence/**` | `git status --short -- <path>`; else `git check-ignore -v` and add a narrow `!`-rule |

**Never run `install.sh` from a worker worktree.** Build to a scratch path (§6).

## 2. Worker map (plan §6.1 — four PRs, linear stack, blocking-first)

| PR | Worker ref | Base | Tasks | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| kA core | `fleet-connect-k8s/<user>/core` | `main` after `fleet-connect` PR 3, or `feature/fleet-connect/<user>/registry` to start early | T1–T2 | **yes** |
| kB levels | `fleet-connect-k8s/<user>/levels` | kA's branch | T3–T5 | yes |
| kC actions | `fleet-connect-k8s/<user>/actions` | kB's branch | T6–T7 | no |
| kD integrate | `fleet-connect-k8s/<user>/integrate` | kC's branch | T8 | no |

```bash
gss feature start fleet-connect-k8s --goal "Kubernetes resources as a fleet provider (design issue #<n>)"
gss feature worker add --feature fleet-connect-k8s --purpose core      --description "k8s provider core: fixtures, parsers, probe, degraded rows (T1–T2)" --json
gss feature worker add --feature fleet-connect-k8s --purpose levels    --description "k8s levels: contexts, namespaces, kinds, objects, containers (T3–T5)" --base <core branch> --json
gss feature worker add --feature fleet-connect-k8s --purpose actions   --description "k8s actions: logs/describe/events streams, exec handoff, service tunnels (T6–T7)" --base <levels branch> --json
gss feature worker add --feature fleet-connect-k8s --purpose integrate --description "k8s: dual-path proof, registration, docs, live gates (T8)" --base <actions branch> --json
```

Capture each `--json` output verbatim into `TRACKING.md` §0. One sub-issue per PR under the
design issue (`mbo-plan` §7); each draft PR body carries `Closes #<sub-issue>`; every PR and
issue carries the program label `fleet-connect` (`gh pr edit <n> --add-label fleet-connect`).

## 3. The execution loop (every task)

1. **Locate:** first unchecked `TODO.md` box → its plan task; read it and the spec rules it maps to.
2. **RED:** write the failing test; run it; **verify it fails**; record the failure line.
3. **GREEN:** implement the minimum; run to pass.
4. **Gates:** `go test -race ./...`, `gofmt -l .`, `go vet ./...`; `make lint-portability` for any
   task that touches `script.go`; the task's own gate from the plan.
5. **Evidence:** `tee` the gate output into `evidence/taskNN/` with a dated header; sanitise hosts.
6. **Ledgers:** tick `TODO.md`; update the `TRACKING.md` row (status, SHA, observed evidence).
7. **Commit** by explicit path; confirm via the interactive prompt; `gss feature checkpoint`.

## 4. Done-when gates

Per PR (plan §6.1): kA — T1–T2 green, fixtures carry provenance, probe round trip = 1,
lint-portability green, ≥ 90%. kB — T3–T5 green, round trips 0/1/1/1, every curated kind
fixture-backed. kC — T6–T7 green, the quoting walk passes, no reserved key used, no real port in
tests. kD — `./scripts/test.sh` green, dual-path identical, the 8-step checklist captured.

**The objective is done when** every `TODO.md` box is ticked, every `TRACKING.md` §1 row has a
SHA and evidence, §2 shows a proof for all 10 features, and the live gates are in `evidence/live/`.

## 5. Hard rules

- **The provider never opens a connection, spawns a process, or imports `cmd`.** Its reach is
  `Host.Exec`; its actions are data.
- **Every cluster value is `runner.Quote`d** in every script and action — names, namespaces,
  contexts, paths. The `x` handoff's `sh -c` payload is a fixed literal; nothing is interpolated
  into it.
- **No cluster write, ever.** No `apply`, `delete`, `scale`, `rollout`, `edit`, `patch`.
- **No secret value leaves the host.** Secrets show key names only; the kubeconfig travels as a
  path in `attrs`, never as content.
- **Every kubectl call carries `--request-timeout=5s`.** A stopped cluster is a row, not a hang.
- **Do not edit the `fleet-connect` contract or its packages.** A gap goes to `TRACKING.md` §4
  and is filed against `fleet-connect`.
- **Fixtures are real captures with provenance** (host kind, kubectl/k3s version, date).
- **Every remote script is POSIX `sh`**; `make lint-portability` is enforcing.
- **Evidence before assertions.** A row is `done` only with a SHA and observed output.

## 6. Command cheat-sheet

```bash
cd sdk/fleet
go test ./internal/provider/k8s/ -run TestName -v
go test -race ./... && gofmt -l . && go vet ./...
go test ./internal/provider/k8s/ -cover
make -C ../.. lint-shell lint-portability
go build -o /tmp/fleet-dev ./ && /tmp/fleet-dev ls <nano> k8s --json
gss feature checkpoint                      # from the worker worktree, after the interactive confirm
```

## 7. Resuming, recovery, and corrections

Fix a wrong command here freely and note it in `TRACKING.md` §5. Never edit the plan or spec
as part of the build. Registry drift: `gss feature audit --feature fleet-connect-k8s --json`,
then `--repair`, then `list --tree`.

## 8. Kickoff prompt (always CURRENT — history lives in git)

> **Maintenance rule:** exactly ONE prompt here — the one that starts the NEXT session.

> Continue the `fleet-connect-k8s` objective in the `core` worker worktree. Read
> `docs/mbo/plans/fleet-connect-k8s/TODO.md` and start at the **first unchecked box**; the
> task detail is in `docs/mbo/plans/fleet-connect-k8s.md`, the requirements in
> `docs/mbo/specs/fleet-connect-k8s.md`, the rationale in
> `docs/mbo/designs/fleet-connect-k8s.md`, and the frozen contract you build against in
> `docs/mbo/plans/fleet-connect.md` §3. First verify the preconditions in `IMPLEMENTATION.md`
> §1 — `fleet-connect` leaves A, B and C must be merged, `go doc ./pkg/provider Tunnel` must
> show `Keeper`. Work strictly TDD: failing test, observed failure pasted, minimal
> implementation, `go test -race ./...` plus the task's gate, `tee` into `evidence/taskNN/`,
> update `TRACKING.md` with the SHA and observed output, tick the box. Honour §5 above all:
> every cluster value quoted, no cluster write, no secret value off the host, `--request-timeout`
> on every kubectl call, and never touch the `fleet-connect` packages. Stage by explicit path
> and **confirm with the interactive prompt before any `git add`, `commit`, or checkpoint**.
> Stop and ask the operator for: capturing fixtures from the real clusters (T1), the live gates
> (T8), and anything that would need a `fleet-connect` change. Done when every `TODO.md` box is
> ticked and `TRACKING.md` §2 shows a proof for all 10 features.
