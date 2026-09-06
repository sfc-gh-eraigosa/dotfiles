# fleet-connect-k8s — implementation plan

- **Slug:** fleet-connect-k8s
- **Date:** 2026-09-05
- **Status:** Draft
- **Relates to:** spec [`../specs/fleet-connect-k8s.md`](../specs/fleet-connect-k8s.md) · design
  [`../designs/fleet-connect-k8s.md`](../designs/fleet-connect-k8s.md) · parent
  [`fleet-connect`](./fleet-connect.md) (issue #266, PR #267) · design issue *(pending)* · PRs
  recorded in [`../index.md`](../index.md)

## 1. Summary & verdict

Build `sdk/fleet/internal/provider/k8s`: the Kubernetes resources provider — resolution and a
one-round-trip probe that works on k3s, kind and a plain kubeconfig host; five levels (contexts
→ namespaces → kinds → objects → containers) with bounded round trips; `l d e` streams, the `x`
exec handoff, and `t` service tunnels kept alive by `kubectl port-forward`; registered as a
built-in and proven over the wire. 8 tasks, strict TDD, one commit each, **four PRs stacked in
order** (§6.1), starting the day `fleet-connect`'s registry leaf lands.

**Verdict:** proceed. The provider needs nothing the frozen `fleet-connect` contract does not
already carry — three action kinds, `Tunnel.Keeper`, alias stamping, `Host.Exec` with a ctx —
and every host-specific gotcha found in the 2026-09-05 probe (`k3s kubectl`, `~/opt/bin` off the
PATH, root-owned but readable kubeconfig, ClusterIP-only services) has a row or a rule.

**Must-hold constraints (inherited):** the provider never opens a connection or spawns a
process; every cluster value is `runner.Quote`d; no cluster write; no secret value leaves the
host; no `cmd` import; the contract is not edited here — a gap is a `fleet-connect` defect.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/fleet/internal/provider/k8s/k8s.go` | `New(Deps) provider.Provider`; `Probe`; `Children` dispatch by depth; `Columns`; degraded-state rows | K1, K2, K3–K5 |
| `sdk/fleet/internal/provider/k8s/script.go` | pure builders: `probeScript()`, `nsScript`, `kindsScript`, `objectsScript`, `podScript`; `kubectlInvocation(attrs)` | K1, K3–K5, K9 |
| `sdk/fleet/internal/provider/k8s/parse.go` | `parseProbe`, `parseVersion`, `parseContexts`, `parseNamespaces`, `parsePods`, per-kind parsers, `splitSections` | K1–K5 |
| `sdk/fleet/internal/provider/k8s/kinds.go` | the curated kind table: kubectl name · display · columns · extractor · actions | K4 |
| `sdk/fleet/internal/provider/k8s/actions.go` | `logsStream`, `describeStream`, `eventsStream`, `execHandoff`, `serviceTunnel` — pure, quoted | K6–K8 |
| `sdk/fleet/internal/provider/k8s/testdata/{k3s,kind-wsl,kind-pi}/*.json` + `errors/*.txt` | **real captured** `-o json` output and error transcripts, provenance headers | K1–K5, K2c |
| `sdk/fleet/internal/provider/k8s/*_test.go` | resolution table, round-trip counts, degraded rows, per-kind tables, quoting walk, secret leak sweep, action argv | K1–K9 |
| `sdk/fleet/cmd/provider_registry.go` *(edit, one line)* | register `k8s` after `herdr` and `ports` | K10 |
| `sdk/fleet/cmd/providers_k8s_test.go` | dual-path equality for the k8s tree; `fleet ls … pods --json` golden | K10a, K10b |
| `sdk/fleet/AGENTS.md`, `sdk/fleet/README.md` *(edit)* | the k8s rows in the providers table; a drill-down + `t` demo with **real** output | §6 |
| `docs/mbo/plans/fleet-connect-k8s/evidence/**` | per-task captures | §7 |

Not touched: `pkg/provider`, `internal/providers`, `internal/bridge`, `internal/runner`, the
TUI, `fleet ls`/`connect`/`bridge`. If any of them needs a change, stop and file it against
`fleet-connect` (TRACKING §4).

## 3. Interface contracts

### 3.1 Consumed (frozen by `fleet-connect` leaf A/B/H)

`provider.Provider`, `provider.Host` (`Exec(ctx, stdin, argv…) → ExecResult`), `provider.Node`,
`Action{Key, Label, Unavailable, Handoff|Stream|Tunnel}`, `Tunnel{RemotePort, LocalPort,
Scheme, Keeper}`, `provider.ReservedKeys`, `runner.Quote`. Nothing else.

### 3.2 Produced: `attrs` (round-tripped from the probe to every level)

| Key | Value | Example |
| :-- | :-- | :-- |
| `kubectl` | absolute path, or `k3s kubectl` | `/usr/local/bin/kubectl` · `~/opt/bin/kubectl` expanded · `k3s kubectl` |
| `k3s` | `1` when the invocation is `k3s kubectl` | |
| `kubeconfig` | absolute path passed as `--kubeconfig`, or empty when kubectl's default applies | `/etc/rancher/k3s/k3s.yaml` |
| `context` | set at the contexts level; every deeper script adds `--context <quoted>` | `kind-bots-e2e` |

### 3.3 Produced: the kind table (frozen at the end of Task 4 — adding a kind is additive)

```go
type kind struct {
    Name     string            // kubectl resource name: "pods"
    Display  string            // "pods"
    Columns  []string          // {"NAME","READY","STATUS","RESTARTS","AGE","NODE"}
    Cells    func(obj) []string
    Actions  func(ctx, ns string, obj) []provider.Action   // l d e x t per §4.5 of the design
    Leaf     bool              // pods: false (containers below); everything else: true
}
```

### 3.4 Produced: the `x` handoff literal (frozen at Task 6)

`<K> -n <ns> exec -it <pod> [-c <container>] -- sh -c 'command -v bash >/dev/null && exec bash || exec sh'`
— the `sh -c` payload is a fixed literal; every `<>` is `Quote`d; `<K>` is the resolved
invocation plus `--context <quoted> --request-timeout=5s` (and `--kubeconfig <quoted>`).

## 4. TDD build order

Each task: failing test first, observe it fail, minimal implementation, gates (`go test -race
./...`, `gofmt -l .`, `go vet ./...`, `make lint-shell && make lint-portability` for scripts),
evidence into `plans/fleet-connect-k8s/evidence/taskNN/`, commit by explicit path.

### PR kA — provider core (blocking)

**T1 · fixtures + parsers.** Capture **real** output from all three clusters: `version -o json`
(client-only and client+server), `config get-contexts -o name`, `get ns -o json`, `get pods -A
-o json`, per curated kind `get <kind> -n <ns> -o json` (at least pods, deployments, services,
configmaps, secrets, pvcs from k3s; pods, deployments, services from both kind clusters), one
pod's JSON with `containerStatuses` in `running`, `waiting` and `terminated`, plus the error
transcripts for a stopped kind cluster and a `forbidden`. Tests: every fixture parses into the
narrow structs; cells for READY/STATUS/RESTARTS/PORTS/KEYS equal `kubectl get`'s rendering
(K4b, K5a); a secret's `data` values never appear in any cell (K9b); a truncated JSON is a
reason, not a panic. Implement `parse.go` and `kinds.go`'s extractors. **Done when** the table
passes ≥ 90%. *Evidence:* provenance headers + test output.

**T2 · resolution + probe + degraded rows.** Tests: kubectl on PATH → one round trip, absolute
path and kubeconfig in `attrs` (K1a); only `~/opt/bin/kubectl` → resolved (the `<pi>` case);
k3s only → `k3s kubectl` + `/etc/rancher/k3s/k3s.yaml`, `$KUBECONFIG` respected (K1b); absent →
paths tried, `Leaf`, no actions (K2a); missing/unreadable kubeconfig → the row, no server call
(K2b); unreachable → `SERVER=unreachable: <line>` inside the deadline with contexts listed,
`forbidden` → `ns forbidden` (K2c, `Fake.Block` + captured errors). Implement `probeScript()`
(POSIX sh), `Probe`, the row builder. **Done when** all pass, round-trip count 1, lint-portability
green. *Evidence:* the three resolution transcripts and the degraded rows. **PR kA exits —
the package, its scripts' shape and `attrs` are frozen.**

### PR kB — levels

**T3 · contexts + namespaces.** Tests: contexts from `attrs` with zero round trips, CURRENT
marked, NAMESPACES for the current context only (K3a); namespaces in one round trip with POD
counts from one `get pods -A`, N = 0 renders an empty level with its header (K3b). Implement
`nsScript` and the two `Children` branches. **Done when** counts hold. *Evidence:* the recorded
argv and rows.

**T4 · kinds + objects.** Tests: kinds level in one round trip with COUNT per curated kind, a
failing kind → `?` + reason without dropping the others, a kind timing out (K4a); objects level
in one round trip with the kind's columns, cells matching the fixtures, a hostile object name
rendered verbatim as a cell (K4b). Implement `kindsScript` (one script, N `get`s), `objectsScript`,
the kind table's columns. **Done when** every curated kind has a fixture-backed row test.
*Evidence:* the kinds frame for k3s and the pods table for kind.

**T5 · containers.** Tests: one round trip from the pod JSON; rows `Leaf`; STATE for running /
waiting (with reason) / terminated; init containers listed and marked (K5a). Implement
`podScript` and the branch. **Done when** the three states render. *Evidence:* rows for the
Grafana pod (3 containers). **PR kB exits.**

### PR kC — actions

**T6 · streams + exec.** Tests: `l` yields a followed `Stream` with `logs --all-containers
--tail=100 -f` and the quoted pod, `-c <quoted>` on a container (K6a); `d`/`e` are one-shot,
`e` on a context uses `-A` (K6b); `x` is a remote `Handoff` with the fixed `sh -c` literal and
quoted names (K7a); the quoting walk — every builder with `'`, `$(…)` and a newline in every
value (K9a); no action uses a key in `provider.ReservedKeys`. Implement `actions.go` and attach
actions in the kind table. **Done when** all pass. *Evidence:* the argv for a hostile pod name.

**T7 · service tunnels.** Tests: `t` on a ClusterIP service → `Tunnel{first port, 0, scheme
guess, Keeper: … port-forward --address 127.0.0.1 svc/<quoted> p:p}`, no address but
`127.0.0.1` anywhere; a headless service → listed with `Unavailable` (K8a); the action fed to a
`bridge.Manager` with a fake keeper: keeper starts under the bridge context, a keeper that
exits fails only its forward with kubectl's line (K8b, via `fleet-connect` F23g). Implement
`serviceTunnel`. **Done when** both pass with no real port. *Evidence:* the tunnel JSON and the
manager transcript. **PR kC exits.**

### PR kD — integrate

**T8 · dual path + register + docs + live.** Tests: the six-level tree byte-identical in-process
and via `fleet provider serve k8s`, including unreachable and absent (K10a); `fleet ls <host>
k8s <ctx> <ns> pods --json` golden, unknown kind names the curated list (K10b). Register the
built-in; write the AGENTS.md rows and the README demo with **real** output; run the live gates
(§7). **Done when** `./scripts/test.sh` is green and every live capture is committed.
*Evidence:* §7's live captures. **PR kD exits — objective done.**

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| K1a | `TestProbeCostsOneRoundTripAndCarriesAbsolutePaths` · `TestKubectlOffTheLoginPathIsResolved` |
| K1b | `TestK3sHostsResolveK3sKubectlWithoutSudo` · `TestKubeconfigEnvWinsOverTheK3sDefault` |
| K2a | `TestAbsentKubectlIsARowNamingThePathsTried` |
| K2b | `TestAMissingOrUnreadableKubeconfigIsARowNotAServerCall` |
| K2c | `TestAnUnreachableServerIsARowNotAHang` · `TestForbiddenNamespacesStillRenderTheCapability` |
| K3a | `TestContextsLevelCostsZeroRoundTrips` |
| K3b | `TestNamespacesLevelCostsOneRoundTripWithPodCounts` |
| K4a | `TestKindsLevelCountsEveryCuratedKindInOneRoundTrip` · `TestAFailingKindRendersAReasonNotAnOmission` |
| K4b | `TestObjectsRenderTheKindsColumnsFromRealFixtures` (table over every curated kind) |
| K5a | `TestContainersLevelRendersRunningWaitingAndTerminated` |
| K6a | `TestLogsStreamFollowsAllContainersWithQuotedNames` |
| K6b | `TestDescribeAndEventsAreOneShotStreams` |
| K7a | `TestExecHandoffUsesTheFixedShellFallbackLiteral` |
| K8a | `TestServiceTunnelKeeperIsAPortForwardOnLoopback` · `TestHeadlessServicesAreListedAndRefused` |
| K8b | `TestAKeeperThatExitsFailsOnlyItsForward` |
| K9a | `TestEveryClusterValueIsQuotedInEveryScript` |
| K9b | `TestSecretValuesNeverLeaveTheHost` |
| K10a | `TestTheK8sTreeIsIdenticalInProcessAndOverTheWire` |
| K10b | `TestLsRendersAK8sLevelAndNamesTheCuratedKinds` |

## 6. Integration & rollout

- **Discovery:** `scripts/test.sh` and the Makefile loops pick up the package by directory; no
  wiring. Coverage floor 60 unchanged; the package targets ≥ 90.
- **Registration:** one line in `cmd/provider_registry.go` after `ports`.
- **Docs:** `sdk/fleet/AGENTS.md` gains the k8s row in the providers table and two invariants:
  every cluster value is quoted, and no secret value leaves the host. `README.md` gains the
  five-level drill-down and the `t`-on-a-service demo, pasted from a real run.
- **Manual acceptance checklist** (real hardware):
  1. `enter` on `<nano>` → `k8s · k3s 1.36 · default · server ok · 6 ns`.
  2. Drill to `observability` → `pods` → `l` on Grafana streams; `esc` stops it.
  3. `x` on the Grafana pod → a shell; exit → dashboard back, row re-polled.
  4. `services` → `t` on `observability-grafana` → a local URL; `curl -sI` answers; `T` stops
     it and `ss -ltn` on `<nano>` shows loopback `:80` gone.
  5. `enter` on `<pi>` → the probe resolves `~/opt/bin/kubectl`; the kind tree renders.
  6. Stop the kind container on `<pi>` → `SERVER=unreachable` row within the deadline; start it.
  7. `fleet ls <nano> k8s default observability pods --json | jq` → the wire shape.
  8. Configure `k8s` as an external plugin (`command: fleet, args: [provider, serve, k8s]`) →
     identical tree.

### 6.1 Build leaves / DAG → PR stack (authoritative; blocking-first)

```
fleet-connect PR 3 (registry) merged ─▶ kA(core) ─▶ kB(levels) ─▶ kC(actions) ─▶ kD(integrate)
fleet-connect PR 7 (bridges) merged ───────────────────────────────▲ (kC's T7 live proof; kD)
```

| PR | Worker (`fleet-connect-k8s/<user>/<purpose>`) | Tasks | Owns (paths) | Consumes | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| **kA** | `core` | T1–T2 | `internal/provider/k8s/{parse,kinds,script,k8s}.go`, `testdata/**` | `fleet-connect` A, B, C merged (`pkg/provider`, `provider serve`) | T1–T2 green; fixtures with provenance; round trip 1; lint-portability; ≥ 90% | **yes (base)** |
| **kB** | `levels` | T3–T5 | `k8s.go` level branches, `script.go` level builders | kA (§3.2 attrs, §3.3 table) | T3–T5 green; round trips 0/1/1/1; every curated kind fixture-backed | yes (kC attaches to its rows) |
| **kC** | `actions` | T6–T7 | `actions.go`, the `Actions` column of `kinds.go` | kB; `fleet-connect` H merged for T7's manager test | T6–T7 green; quoting walk; reserved-key check; no real port | no |
| **kD** | `integrate` | T8 | `cmd/provider_registry.go` (one line), `cmd/providers_k8s_test.go`, docs | kC; `fleet-connect` G merged (E/F for live) | `./scripts/test.sh` green; dual-path identical; 8-step checklist captured | no |

Every PR and issue of this objective carries the program label **`fleet-connect`** (parent plan
§6.1): after each `gss feature checkpoint`, `gh pr edit <n> --add-label fleet-connect`.

The stack is linear: `gss feature start fleet-connect-k8s`, then `worker add --purpose core`
(base `main` once `fleet-connect` PR 3 has merged; `--base feature/fleet-connect/<user>/registry`
to start earlier), `levels --base <core branch>`, `actions --base <levels branch>`, `integrate
--base <actions branch>`. Each worker's draft PR body carries `Closes #<its sub-issue>`.

## 7. Validation & evidence (show the work)

Evidence tree `docs/mbo/plans/fleet-connect-k8s/evidence/task01..08/` + `live/`, append-only,
dated, hostnames sanitised. **Coverage:** `internal/provider/k8s` ≥ 90%; `go test -race`
green; module floor 60 untouched.

**Adversarial scenarios by test:** a pod named `it's $(bad)` in every builder (K9a); a secret
fixture with base64 values (K9b); a stopped cluster with `Fake.Block` (K2c); a root-owned
unreadable kubeconfig (K2b); a headless service (K8a); a keeper that exits at once (K8b); a
kind that is forbidden (K4a); an action key colliding with a reserved key (T6).

**Live gates** (`evidence/live/`): (1) the three resolution transcripts — `k3s kubectl`,
`/usr/local/bin/kubectl`, `~/opt/bin/kubectl`; (2) the five-level drill-down on `<nano>` with
`l`, `x`, `t` and the `curl`; (3) `t` on a `bots-dev` service on `<gigabyte>`; (4) the
stopped-cluster row on `<pi>`; (5) `fleet ls … pods --json`; (6) the external-plugin tree
identical.

> Produced via `superpowers:writing-plans` on 2026-09-05. Execute with
> `superpowers:executing-plans`, TDD throughout, using the trio in
> [`./fleet-connect-k8s/`](./fleet-connect-k8s/). Update [`../index.md`](../index.md) as it moves.
