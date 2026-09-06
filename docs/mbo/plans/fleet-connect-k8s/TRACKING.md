# fleet-connect-k8s — live state ledger

- **Slug:** fleet-connect-k8s
- **Started:** 2026-09-05 (planning); build not started — waits for `fleet-connect` PR 3
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../fleet-connect-k8s.md`](../fleet-connect-k8s.md) · spec
  [`../../specs/fleet-connect-k8s.md`](../../specs/fleet-connect-k8s.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`. A row is `done`
> only with a commit SHA **and** observed evidence.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design (these artifacts) | in the `fleet-connect` design worker | `feature/fleet-connect/<user>/design` | (the fleet-connect design worktree) | [#267](https://github.com/sfc-gh-eraigosa/dotfiles/pull/267) | draft |
| kA core | *not created yet* | | | | `gss feature worker add … --purpose core` |
| kB levels | *not created yet* | | | | `--base <core>` |
| kC actions | *not created yet* | | | | `--base <levels>` |
| kD integrate | *not created yet* | | | | `--base <actions>` |

Captured verbatim from `gss feature worker add --json` when execution starts.

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T1 fixtures + parsers | todo | | | PR kA — real captures from k3s + both kind clusters |
| T2 resolution + probe + degraded rows | todo | | | PR kA — **package shape freezes here** |
| T3 contexts + namespaces | todo | | | PR kB |
| T4 kinds + objects | todo | | | PR kB — kind table freezes here |
| T5 containers | todo | | | PR kB |
| T6 streams + exec | todo | | | PR kC — quoting walk |
| T7 service tunnels (keeper) | todo | | | PR kC — needs `fleet-connect` H for the manager test |
| T8 dual path + register + docs + live | todo | | | PR kD |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| K1 resolution + probe | [ ] T2 | [ ] three resolution transcripts | `k3s kubectl`, `~/opt/bin`, PATH |
| K2 degraded rows | [ ] T2 | [ ] stopped-cluster row on `<pi>` | unreachable inside the deadline |
| K3 contexts + namespaces | [ ] T3 | [ ] live frames | 0 / 1 round trips |
| K4 kinds + objects | [ ] T4 | [ ] live kinds + pods tables | every curated kind fixture-backed |
| K5 containers | [ ] T5 | [ ] Grafana pod's containers | three states |
| K6 streams | [ ] T6 | [ ] `l` streaming live | quoted |
| K7 exec handoff | [ ] T6 | [ ] `x` into a shell and back | fixed literal |
| K8 service tunnels | [ ] T7 | [ ] `t` on Grafana + `curl` | keeper = port-forward |
| K9 quoting + secrecy | [ ] T6, T1 | — | walk + leak sweep |
| K10 dual path + CLI | [ ] T8 | [ ] external-plugin tree identical; `ls --json` | keystone |

## 3. Validation done-when — the stop condition

- [ ] Every `TODO.md` box ticked.
- [ ] Every §1 row `done` with a SHA and observed evidence.
- [ ] Every §2 feature has a ticked automated proof (and live proof where listed).
- [ ] `internal/provider/k8s` ≥ 90% coverage; `go test -race ./...` green; `gofmt`/`go vet` clean.
- [ ] `./scripts/test.sh` green; `make lint-shell && make lint-portability` green.
- [ ] No `fleet-connect` package modified (`git diff --stat` on `pkg/provider`, `internal/providers`, `internal/bridge`, `internal/runner` is empty).
- [ ] Live gates 1–6 (plan §7) captured under `evidence/live/`.
- [ ] The 8-step manual checklist (plan §6) signed off.
- [ ] `docs/mbo/index.md` row advanced to `in-review`.

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-05 | planning | Probed the three Kubernetes hosts read-only: k3s v1.36.3 on the Jetson (`k3s kubectl`, root-owned but readable kubeconfig, 6 ns / 11 pods, Grafana ClusterIP :80), kind `bots-e2e` on the WSL host (`/usr/local/bin/kubectl`, 8 ns / 12 pods, `bots-dev` ClusterIP services) and on the Pi (`~/opt/bin/kubectl` + `kind`, off the non-login PATH); port-forward allowed everywhere; every service ClusterIP. Wrote design (built-in provider, k9s-shaped five levels, curated kinds, `l d e x t` actions, `Tunnel.Keeper` = `kubectl port-forward`), spec (UC1–5, K1–K10 with criteria), plan (8 tasks, four stacked PRs kA–kD) and this trio, alongside the `fleet-connect` design review. Operator priority: herdr → ports → k8s. Design issue #297 and sub-issues #298–#301 created 2026-09-05 21:16 PDT; workers not created yet. No code written. |
