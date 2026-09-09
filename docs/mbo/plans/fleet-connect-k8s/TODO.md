# fleet-connect-k8s — execution cursor

- **Slug:** fleet-connect-k8s
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../fleet-connect-k8s.md`](../fleet-connect-k8s.md)

> **How to use:** the **first unchecked box is the next action**. Tick a box only after you ran
> the command and read the output. After finishing a `###` task: capture evidence, update
> `TRACKING.md`, commit by explicit path after the interactive confirm, checkpoint.
>
> **Legend:** `SETUP` · `RED` · `RUN-RED` (expect FAIL) · `GREEN` · `RUN-GREEN` (expect PASS) ·
> `VERIFY` · `EVID` · `ALLOWLIST` · `DOCS` · `COMMIT` · `LEDGER` · `CHECKPOINT`.

## Preflight (once)

- [ ] `gss feature list --feature fleet-connect --tree` → leaves A, B, C merged
- [ ] `cd sdk/fleet && go doc ./pkg/provider Tunnel` → shows `Keeper`; `go doc ./pkg/provider ReservedKeys` exists
- [ ] `cd sdk/fleet && go test -race ./... && gofmt -l . && go vet ./...` → green, empty, clean
- [ ] `./scripts/test.sh` and `make lint-shell && make lint-portability` → green
- [ ] `ssh <nano> 'k3s kubectl get ns'`, `ssh <gigabyte> 'kubectl get ns'`, `ssh <pi> '~/opt/bin/kubectl get ns'` → all three clusters answer
- [ ] `git rev-parse --show-toplevel && git branch --show-current` → the `core` worker worktree
- [ ] `mkdir -p docs/mbo/plans/fleet-connect-k8s/evidence` and `git status --short -- …` → tracked (else a narrow `!`-rule)

---

## PR kA — provider core (blocking)

### Task 1 — fixtures + parsers  (plan T1)

- [ ] SETUP: capture **real** output into `internal/provider/k8s/testdata/{k3s,kind-wsl,kind-pi}/` — `version -o json` (client-only + client+server), `config get-contexts -o name`, `get ns -o json`, `get pods -A -o json`, per curated kind `get <kind> -n <ns> -o json`, one pod JSON with running/waiting/terminated containers; `testdata/errors/` — stopped-cluster and `forbidden` transcripts; provenance header in each (host kind, kubectl/k3s version, date)
- [ ] RED: `TestObjectsRenderTheKindsColumnsFromRealFixtures` (table over every curated kind), `TestContainersLevelRendersRunningWaitingAndTerminated`, `TestSecretValuesNeverLeaveTheHost`, truncated-JSON-is-a-reason
- [ ] RUN-RED: `go test ./internal/provider/k8s/` → expect **FAIL** (package does not exist)
- [ ] GREEN: `parse.go` (narrow structs, `splitSections`), `kinds.go` (curated table with columns + extractors)
- [ ] RUN-GREEN: `go test ./internal/provider/k8s/ -cover` → **PASS**, ≥ 90%
- [ ] EVID + ALLOWLIST (`testdata/**`) + COMMIT + LEDGER

**Done when:** every fixture parses or reports a reason; every curated kind's cells match `kubectl get`.

### Task 2 — resolution + probe + degraded rows  (plan T2)

- [ ] RED: `TestProbeCostsOneRoundTripAndCarriesAbsolutePaths`, `TestKubectlOffTheLoginPathIsResolved`, `TestK3sHostsResolveK3sKubectlWithoutSudo`, `TestKubeconfigEnvWinsOverTheK3sDefault`, `TestAbsentKubectlIsARowNamingThePathsTried`, `TestAMissingOrUnreadableKubeconfigIsARowNotAServerCall`, `TestAnUnreachableServerIsARowNotAHang`, `TestForbiddenNamespacesStillRenderTheCapability`
- [ ] RUN-RED: `go test ./internal/provider/k8s/ -run "Probe|Resolve|K3s|Kubeconfig|Absent|Unreachable|Forbidden"` → expect **FAIL**
- [ ] GREEN: `script.go` `probeScript()` (POSIX sh; candidates in order; `test -r`; `--request-timeout=5s`), `k8s.go` `New(Deps)`, `Probe`, degraded rows, `attrs` (`kubectl`, `k3s`, `kubeconfig`)
- [ ] RUN-GREEN: `go test -race ./internal/provider/k8s/` → **PASS**; round-trip count exactly 1
- [ ] VERIFY: `make lint-portability` green
- [ ] EVID: the three resolution transcripts + the degraded rows
- [ ] COMMIT + LEDGER + CHECKPOINT (PR kA draft)

**Done when:** one dial yields the capability row on k3s, kind and a plain host, and every failure is a row. **PR kA exits.**

---

## PR kB — levels

### Task 3 — contexts + namespaces  (plan T3)

- [ ] RED: `TestContextsLevelCostsZeroRoundTrips`, `TestNamespacesLevelCostsOneRoundTripWithPodCounts` (N = 0 keeps its header)
- [ ] RUN-RED: `go test ./internal/provider/k8s/ -run "Contexts|Namespaces"` → expect **FAIL**
- [ ] GREEN: `nsScript`, the two `Children` branches, `Columns` for both kinds
- [ ] RUN-GREEN → **PASS**, counts 0 / 1
- [ ] EVID + COMMIT + LEDGER

**Done when:** contexts cost nothing and namespaces cost one dial for any N.

### Task 4 — kinds + objects  (plan T4)

- [ ] RED: `TestKindsLevelCountsEveryCuratedKindInOneRoundTrip`, `TestAFailingKindRendersAReasonNotAnOmission`, the per-kind objects table (hostile object name verbatim as a cell)
- [ ] RUN-RED: `go test ./internal/provider/k8s/ -run "Kinds|Objects|FailingKind"` → expect **FAIL**
- [ ] GREEN: `kindsScript` (one script, N `get`s, each with `--request-timeout=5s`), `objectsScript`, the branches
- [ ] RUN-GREEN → **PASS**, round trips 1 / 1
- [ ] VERIFY: `make lint-portability` green
- [ ] EVID: the kinds frame (k3s) + the pods table (kind)
- [ ] COMMIT + LEDGER

**Done when:** every curated kind has a fixture-backed row and a failing kind is `?` with a reason.

### Task 5 — containers  (plan T5)

- [ ] RED: `TestContainersLevelRendersRunningWaitingAndTerminated` extended to `Leaf` + init containers marked
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `podScript`, the branch
- [ ] RUN-GREEN → **PASS**, one round trip
- [ ] EVID + COMMIT + LEDGER + CHECKPOINT (PR kB draft)

**Done when:** containers render with live state and are leaves. **PR kB exits.**

---

## PR kC — actions

### Task 6 — streams + exec  (plan T6)

- [ ] RED: `TestLogsStreamFollowsAllContainersWithQuotedNames`, `TestDescribeAndEventsAreOneShotStreams`, `TestExecHandoffUsesTheFixedShellFallbackLiteral`, `TestEveryClusterValueIsQuotedInEveryScript`, no key in `provider.ReservedKeys`
- [ ] RUN-RED: `go test ./internal/provider/k8s/ -run "Stream|Exec|Quoted|Reserved"` → expect **FAIL**
- [ ] GREEN: `actions.go`; attach actions in `kinds.go`
- [ ] RUN-GREEN → **PASS**
- [ ] EVID: the argv for `it's $(bad)`
- [ ] COMMIT + LEDGER

**Done when:** every action is quoted data and no key collides with fleet's.

### Task 7 — service tunnels  (plan T7)

- [ ] RED: `TestServiceTunnelKeeperIsAPortForwardOnLoopback`, `TestHeadlessServicesAreListedAndRefused`, `TestAKeeperThatExitsFailsOnlyItsForward` (via `bridge.Manager` + a fake keeper)
- [ ] RUN-RED: `go test ./internal/provider/k8s/ -run Tunnel` → expect **FAIL**
- [ ] GREEN: `serviceTunnel` (first port, scheme guess, `Keeper`)
- [ ] RUN-GREEN → **PASS**; no real port bound
- [ ] EVID: the tunnel JSON + the manager transcript
- [ ] COMMIT + LEDGER + CHECKPOINT (PR kC draft)

**Done when:** a ClusterIP service is a declared tunnel with a keeper. **PR kC exits.**

---

## PR kD — integrate

### Task 8 — dual path + register + docs + live  (plan T8)

- [ ] RED: `TestTheK8sTreeIsIdenticalInProcessAndOverTheWire`, `TestLsRendersAK8sLevelAndNamesTheCuratedKinds`
- [ ] RUN-RED: `go test ./cmd/ -run K8s` → expect **FAIL**
- [ ] GREEN: register `k8s` in `cmd/provider_registry.go`; the golden
- [ ] RUN-GREEN: `go test -race ./... && go test ./... -cover` → **PASS**
- [ ] VERIFY: `./scripts/test.sh`, `make lint-shell && make lint-portability` green; `git diff --stat` shows no `fleet-connect` package touched
- [ ] DOCS: `sdk/fleet/AGENTS.md` k8s row + two invariants; `README.md` demo with **real** output
- [ ] LIVE 1: the three resolution transcripts
- [ ] LIVE 2: five-level drill-down on `<nano>` with `l`, `x`, `t` + `curl`
- [ ] LIVE 3: `t` on a `bots-dev` service on `<gigabyte>`
- [ ] LIVE 4: the stopped-cluster row on `<pi>`
- [ ] LIVE 5: `fleet ls <nano> k8s default observability pods --json`
- [ ] LIVE 6: external-plugin tree identical
- [ ] CHECKLIST: the 8 manual steps in plan §6 signed off
- [ ] EVID (`evidence/live/`) + COMMIT + LEDGER + CHECKPOINT (PR kD draft → ready); `docs/mbo/index.md` → `in-review`

**Done when:** the stop condition in `TRACKING.md` §3 is fully ticked.
