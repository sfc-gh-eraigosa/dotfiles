# fleet-connect-k8s — Kubernetes resources as a fleet provider — spec

- **Slug:** fleet-connect-k8s
- **Date:** 2026-09-05
- **Status:** Draft
- **Relates to:** design [`../designs/fleet-connect-k8s.md`](../designs/fleet-connect-k8s.md) ·
  parent framework [`fleet-connect`](../designs/fleet-connect.md) (issue #266, PR #267) · design
  issue *(pending)* · PR recorded in `../index.md`

## 1. Goal

From a host row the operator presses `enter`, sees `k8s · kubectl 1.36 · kind-bots-e2e ·
server ok · 8 ns`, and drills contexts → namespaces → kinds → objects → containers with the
columns `kubectl get` would show. At a pod they press `l` for a followed log or `x` for a
shell; at a service they press `t` and a ClusterIP port that exists nowhere outside the
cluster becomes a local URL, kept alive by a `kubectl port-forward` fleet runs on the host for
as long as the bridge lives. Every level is read-only, rides fleet's multiplexed ssh through
`Host.Exec`, costs a bounded number of round trips, and works on k3s, on kind, and on a host
whose kubectl is off the ssh PATH.

## 2. Use cases

### UC1 — Watch a workload on the Jetson's k3s
**Actor:** the operator. **Trigger:** Grafana on `<nano>` is restarting. **Flow:** `fleet` →
`enter` on `<nano>` → `enter` on `k8s` (contexts: `default`) → `enter` → namespaces
(`observability · Active · 5 pods`) → `enter` → kinds (`pods 5 · deployments 2 · services 4
…`) → `enter` on `pods` → the pod table with READY/STATUS/RESTARTS → `l` on
`observability-grafana-…` → the log pane streams its lines with the host's colour → `esc` back.
**Acceptance:** five levels rendered with exactly 1 / 0 / 1 / 1 / 1 ssh round trips; the log
stream stops when the operator pops or presses the log-pane stop key; `RESTARTS` matches
`kubectl get pods`.

### UC2 — Reach a ClusterIP service from the workstation
**Actor:** the operator. **Trigger:** they want Grafana in a browser. **Flow:** … → kinds →
`services` → `t` on `observability-grafana` (ClusterIP :80). **Acceptance:** fleet starts
`kubectl port-forward --address 127.0.0.1 svc/observability-grafana 80:80` on `<nano>` as the
forward's keeper, then the host's `ssh -N`; the level line reads `⇄ 80→http://127.0.0.1:80 up`
(or an allocated local port with the note); `curl -sI http://127.0.0.1:<l>` answers `302`;
`T` or `q` stops both the bridge and the keeper, and `ss -ltn` on `<nano>` no longer shows
`:80` on loopback.

### UC3 — A shell in a container on the kind cluster
**Actor:** the operator. **Trigger:** `bots-botsapi` on `<gigabyte>` misbehaves. **Flow:** … →
`bots-dev` → `pods` → `bots-botsapi-…` → `enter` (containers: `botsapi · image · ready ·
restarts · running`) → `x`. **Acceptance:** an `ssh -t` handoff runs `kubectl exec -it … -c
botsapi -- sh -c 'command -v bash >/dev/null && exec bash || exec sh'`; the shell appears;
on exit the dashboard returns and the host row is re-polled; `--dry-run` from `fleet connect`
shows every cluster value quoted.

### UC4 — A host where kubectl is off the PATH, and one where the cluster is down
**Actor:** the operator. **Trigger:** `enter` on `<pi>`, then on a host whose kind container
is stopped. **Acceptance:** `<pi>`'s probe resolves `~/opt/bin/kubectl` in one round trip
and the tree works; the stopped cluster renders `SERVER=unreachable: <kubectl's first stderr
line>` within the 10s deadline, still lists its contexts, and offers no deeper level that
would hang.

### UC5 — Script it
**Actor:** a script. **Flow:** `fleet ls <nano> k8s default observability pods --json | jq
'.nodes[] | select(.cells[2] != "Running")'`. **Acceptance:** the node shape is the
`fleet-connect` wire shape; `cells` are positional against the level's `columns`; nothing in
the JSON is a secret value.

## 3. Architecture

| Component | Path | Boundary |
| :-- | :-- | :-- |
| Provider | `internal/provider/k8s/k8s.go` | `New(Deps)`; `Probe`, `Children` by path depth, `Columns` by kind; action construction; degraded-state rules. Depends on `pkg/provider` and `runner.Quote` only. |
| Scripts | `internal/provider/k8s/script.go` | Pure POSIX-`sh` builders, every value quoted, every kubectl call `--request-timeout=5s`, delimiter-separated output. |
| Parsers | `internal/provider/k8s/parse.go` | Narrow structs per curated kind over `-o json`; a parse failure is a reason. |
| Kind table | `internal/provider/k8s/kinds.go` | One row per curated kind: kubectl name, columns, cell extractors, actions. |
| Fixtures | `internal/provider/k8s/testdata/` | Real captures from k3s and both kind clusters, with provenance. |

**Data flow (one level):** the registry calls `Children(ctx, host, path, attrs)` → the
provider picks the level from `len(path)` → builds one script from `attrs["kubectl"]`,
`attrs["kubeconfig"]` and the quoted path segments → `host.Exec(ctx, "", "sh", "-c", script)`
→ parses the sections → `[]Node` with positional cells and declared actions. In-process and
over the wire are the same code; the keystone equality test covers this provider too.

**Layering rule (inherited):** the provider never imports `cmd`, never opens a connection, and
never spawns anything — its actions are data.

## 4. Behavior / features

- **K1 Resolution + probe.** One round trip resolves kubectl (`command -v`, `~/opt/bin`,
  `~/.local/bin`, `/usr/local/bin`, then `k3s kubectl` with the k3s kubeconfig), checks the
  kubeconfig is readable, and emits client/server version, contexts, current context and the
  current context's namespaces. `Attrs["kubectl"]`, `Attrs["kubeconfig"]` (absolute paths),
  `Attrs["k3s"]`.
- **K2 Degraded states as rows.** Not installed (paths tried) · no kubeconfig · unreadable
  kubeconfig · server unreachable (contexts still listed; deeper levels `Leaf` with the reason)
  · forbidden · timed out — each a row and a reason, never an omission, never a hang.
- **K3 Contexts and namespaces levels.** Contexts from the probe (0 round trips); namespaces in
  1 round trip with pod counts from one `get pods -A`.
- **K4 Kinds and objects levels.** The curated kind table (pods, deployments, statefulsets,
  daemonsets, jobs, cronjobs, services, ingresses, configmaps, secrets, pvcs); kinds level in 1
  round trip with per-kind counts (`?` + reason for a failing kind); objects level in 1 round
  trip with per-kind columns; secrets show key names only.
- **K5 Containers level.** 1 round trip from the pod's JSON; rows are `Leaf`; READY/RESTARTS/
  STATE from `containerStatuses`.
- **K6 Streams.** `l` followed logs (pod, container, deployment/statefulset/daemonset), `d`
  describe, `e` events (pod, namespace, context) — all `Stream` actions, all quoted.
- **K7 Exec handoff.** `x` on a pod or container: a remote `Handoff` running `kubectl exec -it
  … -- sh -c '<fixed literal>'` with bash-then-sh fallback.
- **K8 Service tunnels.** `t` on a service: a `Tunnel` with `RemotePort` = the first port,
  `LocalPort` 0, a scheme guess, and `Keeper` = `kubectl port-forward --address 127.0.0.1
  svc/<name> <p>:<p>`; a busy host port is the keeper's own error, surfaced as the forward's
  reason.
- **K9 Quoting and secrecy.** Every cluster value in every script is `Quote`d; secret values
  are never fetched; the kubeconfig never crosses the wire (only its path, in `attrs`).
- **K10 Dual path + registration.** `fleet provider serve k8s` renders the identical tree; the
  built-in is registered in `cmd/provider_registry.go`; `fleet ls`/`connect`/`bridge` work with
  no CLI change.

## 5. Evaluation criteria (per feature)

- **K1a** a host with kubectl on PATH · one round trip; `attrs` carry the absolute path and the
  kubeconfig · must not dial twice for version and contexts · kubectl only in `~/opt/bin` (the
  `<pi>` case) → still resolved · recording-runner count + attrs assertion.
- **K1b** a k3s host with no kubectl · resolves `k3s kubectl` and `/etc/rancher/k3s/k3s.yaml` ·
  must not require sudo · `$KUBECONFIG` set → respected over the k3s default · resolution
  table test.
- **K2a** no kubectl anywhere · dimmed row naming the paths tried, `Leaf`, no actions · must not
  be omitted · kubectl present but `version --client` fails → row with its stderr · absent test.
- **K2b** kubeconfig missing or unreadable · a row naming the path and the reason · must not
  attempt a server call · a readable root-owned file (mode 0644) → healthy · kubeconfig test.
- **K2c** server unreachable (a stopped kind cluster) · `SERVER=unreachable: <line>` within the
  deadline, contexts listed, deeper levels `Leaf` with the reason · must not hang past the
  provider deadline or the 5s request timeout · `forbidden` on `get ns` → `ns forbidden` with
  the row still enterable by name from the CLI · timed test with `Fake.Block` and a captured
  error fixture.
- **K3a** the contexts level · zero round trips; CURRENT marked; NAMESPACES counted for the
  current context only · must not dial · a kubeconfig with three contexts · attrs-only test.
- **K3b** the namespaces level with N namespaces · exactly one round trip; POD counts from one
  `get pods -A` · must not dial per namespace · N = 0 (empty level with header) · count test.
- **K4a** the kinds level · one round trip; a row per curated kind with COUNT · a failing kind
  renders `?` with its reason and must not remove the others · a kind timing out (5s) ·
  fixture-driven test.
- **K4b** the objects level for each curated kind · one round trip; the kind's columns; cells
  match the real fixture (`kubectl get`'s values for READY, RESTARTS, PORTS…) · must not show a
  secret's value · an object name with `'` and `$(…)` rendered verbatim as a cell · per-kind
  table test over real fixtures.
- **K5a** the containers level · one round trip; rows `Leaf`; STATE from `containerStatuses`
  (`running` / `waiting: CrashLoopBackOff` / `terminated: Completed`) · `enter` on a container
  is a no-op · a pod with an init container (listed, marked) · fixture test.
- **K6a** `l` on a pod · a followed `Stream` whose command contains `logs`, `--all-containers`,
  `--tail=100`, `-f` and the quoted pod · must not contain an unquoted value · on a container
  it carries `-c <quoted>` · argv assertion.
- **K6b** `d` and `e` · one-shot `Stream`s (`Follow: false`) · must not carry `-f` · `e` on a
  context uses `-A` · assertion.
- **K7a** `x` on a pod · a remote `Handoff` with `exec -it`, the quoted pod, and the fixed
  `sh -c` fallback literal · must not be a local handoff and must not interpolate into the
  literal · on a container `-c <quoted>` · argv assertion; `fleet connect … --dry-run` shows it.
- **K8a** `t` on a service · a `Tunnel` with the first port, `LocalPort` 0, the scheme guess,
  and a `Keeper` containing `port-forward`, `--address 127.0.0.1`, `svc/<quoted>` and `p:p` ·
  must not name any address but `127.0.0.1` · a headless service (`ClusterIP: None`) → `t`
  listed with `Unavailable: "headless service — forward a pod instead"` · tunnel assertion.
- **K8b** a keeper that exits (busy host port) · that forward `failed` with kubectl's line;
  siblings up · must not retry in a loop · a keeper that starts and the local dial succeeds →
  `up` · bridge-manager test with a fake keeper (`fleet-connect` F23g, driven from this
  provider's action).
- **K9a** every script builder with a value containing `'`, `$(…)` and a newline · the value
  appears exactly once, quoted, in the built script · an unquoted occurrence fails the test ·
  the kubeconfig path with a space · quoting walk over every builder.
- **K9b** the secrets kind · KEYS counts `data` keys; no value string from the fixture appears
  anywhere in any cell or detail · must not fetch `-o yaml`/values · a secret with zero keys ·
  leak sweep.
- **K10a** the tree in-process vs via `fleet provider serve k8s` · byte-identical at all six
  levels, including the unreachable and absent cases · any divergence fails · the `t` tunnel
  action's keeper string identical both ways · dual-path equality test.
- **K10b** `fleet ls <host> k8s <ctx> <ns> pods --json` · the wire shape, cells positional ·
  must not need the TUI · an unknown kind segment errors naming it and listing the curated
  kinds · CLI test.

## 6. Verification harness

- **Unit (pure).** Script builders and parsers over **real captured** fixtures from all three
  clusters; the kind table drives per-kind tests so adding a kind adds a fixture and a row.
- **Provider.** `runner.Fake`/a recording runner behind a `providertest` `Host`: round-trip
  counts per level, degraded rows, `Fake.Block` for the unreachable timeout.
- **Dual path.** The `fleet-connect` keystone test generalised: `internal/provider/k8s` served
  over the wire equals in-process.
- **Bridge integration.** The `t` action fed to `bridge.Manager` with a fake keeper process
  (F23g) — no real port, no real cluster.
- **Shell.** `make lint-shell && make lint-portability` on every embedded script; `bash -n`.
- **Live gates** (captured under `plans/fleet-connect-k8s/evidence/live/`): the three probes;
  a five-level drill-down on `<nano>` with `l`, `x` and `t`; `t` on a `bots-dev` service on
  `<gigabyte>`; the stopped-cluster row; `fleet ls … --json`.
- **Coverage.** `internal/provider/k8s` ≥ 90%; module floor 60 unchanged; `go test -race`.

## 7. Prerequisites / dependencies

- `fleet-connect` leaves **A** (contract with `Tunnel.Keeper` and reserved keys), **B**
  (protocol), **C** (registry + `provider serve`) merged — K1–K7 and K9–K10 build on those;
  **H** (bridges) merged for K8's live proof; **E**/**F** for the TUI/CLI live gates.
- The TUI runs a cursor row's action for any declared key (a `fleet-connect` F18 rule), so
  `l d e x t` need no keymap change.
- kubectl ≥ 1.26 on a host (for `-o json` shapes used); k3s ≥ 1.30 (`k3s kubectl`). No
  install.sh change, no host writes, no cluster writes.

## 8. Out of scope (and why)

| Item | Why |
| :-- | :-- |
| Cluster mutation (`apply`, `scale`, `delete`, `rollout`) and a parameter form | Read-only by construction; writes happen in the `x` terminal. A form is a TUI mode `fleet-connect` chose not to build. |
| Every resource kind, CRDs, cluster-scoped kinds beyond namespaces/events | v1 is the curated table; each further kind is one row + one fixture, later. |
| Pod port-forwards, host-side port allocation for keepers | Services cover the fleet's need today; pods and allocation are additive under the same `Tunnel`. |
| Cluster-first views; a separate `fleet-provider-k8s` binary | Host-rooted tree while ssh is the bridge; the served built-in is already the wire consumer. |

## 9. Rollback

Revert the PR(s), or set `enabled: false` for `k8s` in `~/.config/fleet/providers.yaml`.
Nothing on a host or cluster carries state; a keeper dies with its bridge.

> Produced via `superpowers:brainstorming` on 2026-09-05. The matching plan is
> `../plans/fleet-connect-k8s.md`. Register / update `../index.md`.
