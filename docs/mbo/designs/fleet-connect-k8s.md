# fleet-connect-k8s — Kubernetes resources as a fleet provider — design

- **Slug:** fleet-connect-k8s
- **Date:** 2026-09-05
- **Status:** Proposed
- **Relates to:** the second provider under [`fleet-connect`](./fleet-connect.md) (design
  issue [#266](https://github.com/sfc-gh-eraigosa/dotfiles/issues/266), design PR #267); design
  issue *(pending — see `../index.md`)*; consumes the frozen contract of `fleet-connect` plan
  §3.1 (three action kinds, `Tunnel.Keeper`, no host field) and its bridge manager (leaf H).
- **Author(s):** operator + assistant

## 1. Problem / context

Three of the four fleet hosts run Kubernetes, and none of it is reachable from the dashboard:
the operator drops to ssh, remembers which `kubectl` works on which box, and types
`port-forward` by hand to see a Grafana that only listens on a ClusterIP. `fleet-connect`
builds the framework and proves it with herdr; this objective is the provider that stresses
the framework's depth (five levels), its streams (`logs -f`), its handoffs (`exec -it`) and —
through `Tunnel.Keeper` — its bridges (`port-forward`). The operator's priority is herdr →
ports → **k8s resources**; this design is written alongside `fleet-connect` so the build can
start the day its blocking leaves land.

### 1.1 What the fleet actually runs (read-only probe, 2026-09-05)

| Host | Distribution | `kubectl` | kubeconfig | Contexts | Namespaces · pods | Services of interest |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| `<nano>` (Jetson, aarch64) | **k3s** v1.36.3 server | **none on PATH** — `k3s kubectl` (`/usr/local/bin/k3s`) | `/etc/rancher/k3s/k3s.yaml`, root-owned but mode 0644, readable as the user | `default` | 6 · 11 (`observability`: Grafana 3/3, Prometheus operator; `k3s-smoke`; metrics-server) | `observability-grafana` ClusterIP :80, `loki-headless` :3100, `smoke` :8080 |
| `<gigabyte>` (WSL2, x86_64) | **kind** `bots-e2e` v1.36 | `/usr/local/bin/kubectl` | `~/.kube/config` | `kind-bots-e2e` | 8 · 12 (`bots-dev`: botsapi, botshook, agent-worker) | `bots-botsapi` :50051/9101, `bots-botshook` :8080/9103, `agent-worker-default` :50052/9102 — all ClusterIP |
| `<pi>` (aarch64) | **kind** `bots-e2e` v1.36.1 in docker (`bots-e2e-control-plane`, API on `127.0.0.1:34655`) | `~/opt/bin/kubectl` and `~/opt/bin/kind` — **not on the non-login ssh PATH** | `~/.kube/config` | `kind-bots-e2e` | (kubectl unreachable from a plain ssh shell until the path is resolved) | as `<gigabyte>` |
| `<spark>` (DGX) | — | — | — | — | — | — |

Verified on all three clusters: `kubectl auth can-i create pods/portforward -A` → `yes`; 45
namespaced list-able resource kinds on k3s; every service is ClusterIP, so **nothing is
reachable without a port-forward** — the port a `Tunnel` needs does not exist on the host
until `kubectl port-forward` is running. That is exactly what `Tunnel.Keeper` is for.

### 1.2 What `fleet-connect` gives this provider (frozen at its leaf A/B/H exits)

- `provider.Provider` / `provider.Host` in-process, and the same over the wire (`fleet provider
  serve k8s`), with `Host.Exec(ctx, stdin, argv…) → ExecResult{stdout, stderr, exitCode}`.
- Three declared action kinds: `Handoff` (remote `ssh -t` command or local argv), `Stream`
  (remote command, lines into the log pane, cancellable), `Tunnel` (a host loopback port bridged
  to the workstation, with an optional `Keeper` command fleet runs on the host for as long as
  the bridge lives). No action names a host; fleet stamps the level's alias.
- `runner.Quote` for every provider value that enters a remote command; local handoffs are argv.
- The ports/bridges machinery: one `ssh -N` per host, `t`/`T` keys, `fleet bridge`.
- The keystone test shape: the tree rendered in-process and over the wire must be identical.

## 2. Goals & non-goals

**Goals**

1. **See the cluster from the host row:** `enter` on `<nano>` shows `k8s · k3s 1.36 · context
   default · server ok · 6 ns`; `enter` drills contexts → namespaces → resource kinds → objects
   → containers, each level with its own columns and a bounded number of ssh round trips.
2. **Act at the right level:** logs (`l`, followed stream), describe and events (`d`, `e`,
   one-shot streams), an interactive shell (`x`, handoff, `bash` with `sh` fallback), and a
   **port-forward as a bridge** (`t` on a service) — the ClusterIP Grafana on the Jetson
   becomes `http://127.0.0.1:80` (or an allocated port) in two keystrokes.
3. **Work on every cluster the fleet has:** k3s where `kubectl` is `k3s kubectl` and the
   kubeconfig is root-owned, kind where `kubectl` lives in `~/opt/bin` off the ssh PATH, and a
   plain kubeconfig host — resolved once per probe, carried in `attrs`, never re-guessed.
4. **Degraded states are rows with reasons:** no kubectl, no kubeconfig, unreadable
   kubeconfig, server unreachable (a stopped kind cluster), RBAC forbidden, a context that
   times out — each a row that says so, never an omission or a hang.
5. **Zero framework change.** The provider uses protocol v1 and the frozen contract as-is;
   anything it turns out to need is filed as a `fleet-connect` defect, not patched here.
6. **CLI parity for free:** `fleet ls <host> k8s <ctx> <ns> pods --json` and `fleet connect
   <host> k8s <ctx> <ns> pods <pod> --action x` work because the provider is the same code.

**Non-goals (rejected, not deferred)**

- **Mutating the cluster** (`apply`, `delete`, `scale`, `rollout`): every level is read-only;
  the only writes happen inside the terminal `x` hands the operator. A parameter form would be
  a new TUI mode `fleet-connect` deliberately does not have.
- **Cluster-first views** (one cluster reachable from two hosts shown once): the tree is
  host-rooted while ssh is the bridge; the kind cluster on `<pi>` and the one on `<gigabyte>`
  are two different clusters anyway.
- **Every resource kind.** v1 ships a curated list (§4.4); custom resources and the other ~30
  namespaced kinds are one table row each later, behind the same code path.

## 3. Options considered

**A. A built-in Go provider (`internal/provider/k8s`), also servable over the wire — chosen.**
Same packaging as herdr: in-process for speed, `fleet provider serve k8s` for the dual-path
proof. Costs Go code in fleet's tree; earns the parsers, fixtures and quoting discipline the
herdr provider already models, no binary distribution problem, and the second real consumer
the protocol needs.

**B. A separate `fleet-provider-k8s` binary from day one.** Proves the "no fleet rebuild"
story literally, but doubles the release surface for a provider the operator wants on every
host, and gives the protocol its second consumer *later* than A does (the served built-in is
already that consumer). Rejected for v1; A's provider can be split out with no code change if
it is ever wanted.

**C. A client-go provider talking to the API server through a tunnel.** Richer data, no
kubectl dependency, but it re-implements kubeconfig/context/auth handling client-side, needs a
tunnel *before* the probe, and abandons the `host/exec` rule that a provider reaches a machine
only through fleet's ssh seam. Rejected: `kubectl -o json` over `Host.Exec` is enough, and the
kubeconfig never leaves the host.

**Levels: k9s-shaped, curated.** Contexts → namespaces → **resource kinds** → objects →
containers. The kinds level (a table of `pods 12 · deployments 5 · services 5 …`) is what
makes the tree navigable on a 45-kind cluster; a flat "workloads" level would either hide
kinds or force one column set on all of them.

## 4. Decision

### 4.1 Units

| Unit | Does | Used by | Depends on |
| :-- | :-- | :-- | :-- |
| `internal/provider/k8s` | `New(Deps) provider.Provider`: `Probe`, `Children` per level, `Columns` per kind, action construction, degraded-state rules. | the registry (built-in), `fleet provider serve k8s` | `pkg/provider`, `internal/runner` (`Quote` only) |
| `internal/provider/k8s/script.go` | Pure POSIX-`sh` script builders: `probeScript()`, `nsScript(kc, ctx)`, `kindsScript(kc, ctx, ns, kinds)`, `objectsScript(kc, ctx, ns, kind)`, `podScript(kc, ctx, ns, pod)`; every value `Quote`d; every kubectl call `--request-timeout=5s`. | the provider | `internal/runner.Quote` |
| `internal/provider/k8s/parse.go` | Narrow structs over `kubectl … -o json` for each curated kind; `parseVersion`, `parseContexts`; a parse failure is a reason, never a panic. | the provider | stdlib |
| `internal/provider/k8s/kinds.go` | The curated kind table: kubectl name, display name, columns, cell extractors, actions per kind. Adding a kind is one row. | the provider | — |
| `internal/provider/k8s/testdata/` | **Real captured** output from all three clusters with provenance headers. | tests | — |

No new package outside the provider; no `cmd` change beyond registering the built-in (one line
in `cmd/provider_registry.go`, the site `fleet-connect` leaf G created for exactly this).

### 4.2 The tree

```
host
└─ k8s              CAPABILITY · KUBECTL · CONTEXT · SERVER · NS               (leaf if degraded)
   └─ <context>     CONTEXT · CLUSTER · CURRENT · NAMESPACES                   e: cluster events
      └─ <ns>       NAMESPACE · STATUS · PODS · AGE                            e: namespace events
         └─ <kind>  KIND · COUNT                                               (curated list, §4.4)
            └─ <object>   columns per kind (§4.4)                              l d e x t per kind
               └─ <container>  CONTAINER · IMAGE · READY · RESTARTS · STATE     l x   (Leaf)
```

The path is the contract, as in `fleet-connect`: `fleet ls <host> k8s kind-bots-e2e bots-dev
pods` is the objects level, and the breadcrumb is the same `[]string`.

### 4.3 Resolution and the probe (one round trip)

A POSIX-`sh` probe resolves, in order, `command -v kubectl`, `~/opt/bin/kubectl`,
`~/.local/bin/kubectl`, `/usr/local/bin/kubectl`, then `command -v k3s` (→ `k3s kubectl`, with
`KUBECONFIG=/etc/rancher/k3s/k3s.yaml` when `$KUBECONFIG` and `~/.kube/config` are absent).
It then emits, delimiter-separated exactly like herdr's probe: the resolved invocation, the
kubeconfig path and whether it is readable, `version -o json --request-timeout=5s` (client
always, server when reachable), `config get-contexts -o name`, `config current-context`, and
`get ns -o json` for the current context. `Attrs` carry `kubectl` (absolute path, or the two
words `k3s kubectl` under `k3s=1`) and `kubeconfig`, so no deeper level re-resolves anything.

| State | Detected by | Row |
| :-- | :-- | :-- |
| no kubectl anywhere | probe's absent marker | dimmed `not installed`, paths tried, `Leaf` |
| kubectl, no kubeconfig | no readable config in the three places | `no kubeconfig (tried …)`, `Leaf` |
| kubeconfig unreadable | `test -r` fails (a root-owned k3s.yaml without 0644) | `kubeconfig not readable by <user>: <path>`, `Leaf` |
| server unreachable | `version` has no `serverVersion`, or `get ns` fails | `SERVER=unreachable: <first stderr line>`; contexts still list; deeper levels are `Leaf` with the reason |
| forbidden | `get ns` exits with `forbidden` | `SERVER=ok · ns forbidden (RBAC)`; the operator can still enter a namespace by name via `fleet ls` |
| healthy | serverVersion present, ns listed | `k8s · kubectl 1.36 · <ctx> · server 1.36 · 6 ns` |

### 4.4 Levels, kinds and round-trip budgets

| Level | Round trips | Source | Columns |
| :-- | :-- | :-- | :-- |
| contexts | 0 (from the probe's `attrs`) | `config get-contexts` | CONTEXT · CLUSTER · CURRENT · NAMESPACES (count only for the current context; `-` otherwise) |
| namespaces | 1 | `get ns -o json` + `get pods -A -o json` (counts) | NAMESPACE · STATUS · PODS · AGE |
| kinds | 1 (one script, N `get <kind> -o json` inside it) | the curated list | KIND · COUNT |
| objects | 1 | `get <kind> -n <ns> -o json` | per kind, below |
| containers | 1 | `get pod <name> -n <ns> -o json` | CONTAINER · IMAGE · READY · RESTARTS · STATE |

Curated kinds and their columns (v1): **pods** NAME · READY · STATUS · RESTARTS · AGE · NODE;
**deployments** NAME · READY · UP-TO-DATE · AVAILABLE · AGE; **statefulsets** / **daemonsets**
NAME · READY · AGE; **jobs** NAME · COMPLETIONS · AGE; **cronjobs** NAME · SCHEDULE · LAST · AGE;
**services** NAME · TYPE · CLUSTER-IP · PORTS · AGE; **ingresses** NAME · HOSTS · AGE;
**configmaps** / **secrets** NAME · KEYS · AGE (secrets: key *names* only — values never
leave the host); **persistentvolumeclaims** NAME · STATUS · CAPACITY · AGE; **events** is not a
kind row but the `e` action. A kind whose `get` fails (forbidden, or a CRD not installed) shows
`COUNT=?` with the reason in its detail, never an omission.

### 4.5 Actions (declared data; fleet runs them)

| Row | Key | Kind | Command (every `<>` value `Quote`d; `$K` = the resolved kubectl + `--context <ctx>` + `--request-timeout=5s`) |
| :-- | :-- | :-- | :-- |
| pod | `l` | Stream, follow | `$K -n <ns> logs <pod> --all-containers --tail=100 -f` |
| pod | `d` | Stream, one-shot | `$K -n <ns> describe pod <pod>` |
| pod | `e` | Stream, one-shot | `$K -n <ns> get events --field-selector involvedObject.name=<pod> --sort-by=.lastTimestamp` |
| pod | `x` | Handoff, remote | `$K -n <ns> exec -it <pod> -- sh -c 'command -v bash >/dev/null && exec bash || exec sh'` |
| container | `l` / `x` | as pod, with `-c <container>` | |
| deployment / statefulset / daemonset | `l` | Stream, follow | `$K -n <ns> logs <kind>/<name> --all-containers --tail=100 -f` |
| deployment / … | `d` | Stream, one-shot | `$K -n <ns> describe <kind> <name>` |
| service | `t` | **Tunnel** | `RemotePort: <first port>`, `LocalPort: 0`, `Scheme: "http"` when the port name or number says so (`http`, `80`, `8080`, `3000`, `grafana`), else `""`; `Keeper: $K -n <ns> port-forward --address 127.0.0.1 svc/<name> <port>:<port>` |
| namespace / context | `e` | Stream, one-shot | `$K -n <ns> get events --sort-by=.lastTimestamp` / `$K get events -A --sort-by=.lastTimestamp` |
| kind | — | | a kind row has no actions; `enter` lists its objects |

Key rules from `fleet-connect` apply: `c` is not used by this provider (there is no single
"connect" for a cluster), keys are one printable rune each and never a key fleet reserves
(`enter esc r t T q / n N j k g G` and the unbound dashboard verbs); the TUI runs the cursor
row's action for any other key and lists the row's keys in the header strip.

**Port-forward host port.** The keeper binds the service port on the host's loopback
(`80:80`, `50051:50051`). If that host port is busy the keeper exits with kubectl's `unable to
listen` line and the forward is `failed` with that reason — the operator sees why rather than
fleet guessing a different port on the host. (Allocating on the host is a follow-on: one
round trip to find a free port, carried in `attrs`.)

### 4.6 Quoting and the trust boundary

Every provider value — context, namespace, object, container names, kubeconfig path — passes
`runner.Quote` before it enters a remote command string; the `x` handoff is a remote command
(`ssh -t`), so its `sh -c` payload is a fixed literal and the names around it are quoted.
Nothing from the cluster (an object name is attacker-controlled on a shared cluster) can break
out of a quoted word. Secrets' values are never fetched (`KEYS` counts keys from the JSON's
`data` map without printing values). Pinned by `TestEveryClusterValueIsQuotedInEveryScript`
and `TestSecretValuesNeverLeaveTheHost`.

## 5. Risks & blast radius

| Risk | Severity | Mitigation |
| :-- | :-- | :-- |
| **A cluster-supplied name reaches a shell unquoted** | High | Structural: every script builder is pure and quotes every value; one test walks every builder with a name containing `'`, `$(…)` and a newline. |
| **The kinds level is slow on a large cluster** | Medium | One round trip with N `get`s inside it; `--request-timeout=5s` per call; the curated list is 11 kinds; a kind that times out renders `?` and does not block the others. |
| **A stopped kind cluster hangs the probe** | Medium | `--request-timeout=5s` on every call, the provider's own 10s deadline through `Host.Exec`'s ctx, and the `SERVER=unreachable` row. Pinned by `TestAnUnreachableServerIsARowNotAHang`. |
| **The k3s kubeconfig stops being world-readable** | Low | The `not readable` row names the path; nothing is escalated (no sudo). |
| **`port-forward` fights a busy host port** | Low | The keeper's own error line is the forward's reason (F23g); the operator retries after freeing it. |
| **The protocol lacks something k8s needs** | Low | Designed against the frozen contract with the three action kinds and `Keeper`; nothing here needs a fourth. If one appears it is filed against `fleet-connect`, per its goal 8. |

Blast radius: one new package under `sdk/fleet/internal/provider/`, one registration line, real
fixtures. No host-side writes, no cluster writes, no new credential material; the kubeconfig
never leaves the host.

## 6. Rollback

Reverting the PR(s) removes the provider; `enabled: false` for `k8s` in `providers.yaml`
disables it without a revert. Nothing on any host or cluster carries state from this feature —
a port-forward keeper dies with its bridge, which dies with fleet.

## 7. Evidence expectations

- **Fixtures with provenance:** `version -o json`, `get ns -o json`, `get pods -A -o json`,
  `get svc -o json`, `get deploy -o json`, one pod's JSON, and a `describe`, captured from all
  three clusters (k3s on the Jetson, kind on WSL and on the Pi), plus the unreachable and
  forbidden error outputs — each file headed with host kind, kubectl version, date.
- **Round-trip counts** per level from a recording runner: 1 / 0 / 1 / 1 / 1 / 1.
- **Resolution transcripts:** the probe on all three hosts showing `k3s kubectl`, `~/opt/bin/
  kubectl` and `/usr/local/bin/kubectl` resolved and carried in `attrs`.
- **Quoting:** the built argv for a pod named `it's $(bad)` — the inert quoted word.
- **Dual path:** the five-level tree identical in-process and via `fleet provider serve k8s`.
- **Live:** the drill-down to a Grafana pod on `<nano>` with `l` streaming its log, `x` landing
  in a shell and returning, and `t` on `observability-grafana` yielding a local URL that
  `curl -sI` answers; the same on a `bots-dev` service on `<gigabyte>`; the `<pi>` probe
  resolving `~/opt/bin/kubectl`; `fleet ls <nano> k8s default observability pods --json`.

> Produced via `superpowers:brainstorming` with the operator's priority order (herdr → ports →
> k8s resources) on 2026-09-05, alongside the `fleet-connect` design review. Registered in
> `../index.md`. The matching spec is `../specs/fleet-connect-k8s.md`.
