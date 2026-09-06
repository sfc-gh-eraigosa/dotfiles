# fleet-connect — connections beyond ssh: a local-RPC provider plugin framework + the herdr provider — design

- **Slug:** fleet-connect
- **Date:** 2026-09-02
- **Status:** Proposed
- **Relates to:** builds on `fleet` (`./fleet.md`, PR #224) and `fleet-tui` (`./fleet-tui.md`,
  PR #227); consumes the `herdr` dossier (`./herdr.md`, PR #261). Design issue
  [#266](https://github.com/sfc-gh-eraigosa/dotfiles/issues/266); design PR recorded in
  `../index.md` row `fleet-connect`.
- **Author(s):** operator + assistant
- **Amended:** 2026-09-05 — port bridges (goal 7, §3.4, §4.7–§4.8): a third declared action
  kind, `Tunnel`, and the `ports` provider, decided with the operator before the contract freezes.

## 1. Problem / context

`fleet` knows exactly one thing about a host: dotfiles install drift, read in one ssh round
trip (`sdk/fleet/cmd/status.go` `probeHost`). Its dashboard's only "connect" is `s`, a bare
ssh shell (`cmd/tui_cmds.go` `sshShell`). Everything an operator actually wants to reach on a
host — a herdr workspace with agents in it, a Kubernetes pod's logs, a container's shell, an
`htop`/`nvtop` view — sits one or more levels *below* the host row, and today fleet cannot
see any of it. The operator drops to ssh and navigates by hand, on every host, every time.

The request has two halves:

1. From a host's dashboard row, **drill down** into what lives on it and **connect from
   whichever level fits** — with ssh as the bridge, so no new transport or credential appears.
2. **Onboarding a new tool should not require changing fleet.** The operator asked for tools to
   be **plugins**, addressed over a **local RPC protocol, MCP-shaped**, so the same mechanism
   later serves a remote RPC setup. Building it that way "from the get-go" is the ask, because
   a protocol retrofitted after three providers exist is a protocol shaped by three accidents.

### 1.1 What the fleet actually runs (read-only probe, 2026-09-02)

| Host | Orchestration | Containers | Sessions | Stats tools | Listening ports of interest |
| :-- | :-- | :-- | :-- | :-- | :-- |
| `<nano>` (Jetson) | **k3s** server (6443) | docker | tmux `nano-ollama` | jtop, tegrastats, nvidia-smi | ollama 11434, 9100, 50051/2 |
| `<spark>` (DGX) | — | docker (`dsh-searxng`) | **herdr** in `~/.local/bin` (not on the non-login ssh PATH) | nvtop, nvitop, btop | ollama 11434–6, 3080, 8888, 11000 |
| `<gigabyte>` (WSL) | **kind** (`bots-e2e`) + kubectl | docker | — | htop | — |
| `<pi>` | **kind** (`bots-e2e`) | docker | tmux `claude-bots` `claude-oleddash` `claude-recover` | htop | VNC 5900, ollama 11434 |

The targets are real and heterogeneous: two Kubernetes flavours, docker everywhere, three live
Claude remote-control tmux sessions, a herdr server, and several web UIs reachable only through
a tunnel. That heterogeneity is the argument for a plugin boundary rather than a growing
`switch` inside fleet.

### 1.2 herdr's remote surface (verified against herdr 0.8.2)

- `herdr status --json` → `client{version,protocol}` + `server{running,version,protocol,
  compatible,capabilities{…},restart_needed}`; the `server` block is absent when no daemon runs.
- `herdr session list --json` → `sessions[{name,default,running,session_dir,socket_path}]`.
- `herdr [--session <name>] api snapshot` → `snapshot.agents[{agent,agent_status
  (idle|working|blocked|done|unknown),pane_id,terminal_title_stripped,cwd,…}]`. `--session` is a
  global flag, so a named session's snapshot is one command.
- **Attach:** `herdr --remote <ssh-alias> [--session <name>]` — a **local** binary that runs its
  own ssh against the operator's `~/.ssh/config`. It needs a real tty: it may prompt to install
  or restart the remote binary.
- The client/server **protocol is versioned** (20 today); mismatched releases may refuse to
  attach. This is also the precedent for versioning fleet's own plugin protocol.
- **PATH gotcha:** a non-login ssh shell's PATH lacks `~/.local/bin` and `~/opt/bin`. On the
  Spark `command -v herdr` fails while `~/.local/bin/herdr status` works.

### 1.3 What fleet's code offers as seams (verified)

- `internal/runner` is the **only** package that touches a remote host (`Runner` interface;
  `Exec` real, `Fake` for tests). Every ssh call carries `MuxArgs()` (ControlMaster reuse).
- The `Runner` interface cannot produce an `*exec.Cmd`, yet `tea.ExecProcess` needs one, so the
  TUI's four interactive actions each spell `ssh` out themselves in `cmd/tui_cmds.go`.
- `runner.RunStreamCtx` (landed with fleet-update #270, tested in `runner_ctx_test.go`) already
  gives a cancellable, killable stream lane, so a `logs -f` needs nothing new. What the seam
  lacks is a context on the **batch** lane: `Run(host, argv…)` returns stdout only and cannot
  be cancelled, so a provider's exec would have no deadline and no stderr or exit code —
  `RunCtx` is the one batch-lane addition this objective makes (§4.2).
- `sshconf.Host` has no kind field; `Row` (`cmd/status.go`) is install-drift-specific; there is
  no registry or plugin mechanism anywhere. The closest existing pattern is the wake ladder: a
  slice of uniform strategies with every impure edge injected through `reach.Deps`.
- The TUI is one column (banner · host table · log pane · status bar); cursor and selection are
  alias-keyed; `keyHelp` is the single source of truth for keys; `TestDemoFrames` renders golden
  frames and doubles as the width guard.
- `sdk/gff` and `sdk/tmux-mgr` already ship a public `pkg/` alongside `internal/`, so a
  plugin-author-facing package has precedent. `gopkg.in/yaml.v3` is already a direct
  requirement of fleet's own `go.mod` (used by `internal/updplan`), so the providers loader
  adds no dependency. Eleven non-test `runner.Exec{}` construction sites exist today; none move.

## 2. Goals & non-goals

**Goals**

1. A **provider plugin framework**: a versioned, MCP-shaped JSON-RPC protocol over a local
   transport, by which any executable can tell fleet what lives on a host, level by level, with
   its own columns, and what an operator can do at each row. Designed so the transport can
   later be a socket or a remote endpoint without touching the methods.
2. **A plugin never touches a host itself.** Its only route to a remote machine is a callback
   into fleet's `runner` seam, so one credential path, one multiplexed connection and one
   quoting discipline serve every provider. This is what makes plugins safe to add.
3. **Config-only onboarding of a plugin:** a stanza in `~/.config/fleet/providers.yaml` naming
   the command. No fleet rebuild, no code change, order and enablement from the file.
4. **Drill-down navigation** in `fleet tui`: `enter` on a host pushes that host's capability
   table; each deeper level is its own table with kind-specific columns; `esc` pops; a
   breadcrumb shows where you are; the dashboard (level 0) is unchanged.
5. **The herdr provider** as the framework's first real consumer: host → herdr → sessions →
   agents, attach at the session level, with every degraded state a row and a reason.
6. **CLI parity:** `fleet ls <host> [path…] [--json]` and `fleet connect <host> <path…>` run the
   same providers, and their JSON node shape **is** the plugin wire shape — one schema, four
   consumers (TUI, CLI, plugin protocol, scripts).
7. **Port bridges:** from a host's `ports` level, bridge any number of that host's listening
   ports to `127.0.0.1` on the workstation over the ssh connection fleet already multiplexed,
   so a dsh or web UI that only listens on the remote becomes a local URL. Bridges on several
   hosts are started, stopped and shown from one dashboard or one `fleet bridge` command, and
   **a bridge lives exactly as long as the fleet process that opened it.**
8. **The provider roadmap is the program's goal, in this order:** herdr (this objective),
   ports and bridges (this objective, leaf H), then **Kubernetes resources**
   (`fleet-connect-k8s` — designed, specified and planned alongside this objective so it
   starts the day the framework lands), then containers, tmux/Claude sessions, system stats, a
   declarative-manifest plugin and remote transports (§3.1; each registered in `../index.md`).
   Every one of them is a provider speaking protocol v1 with **zero framework change** — a
   framework change one of them needs is a defect in this design, not a feature of theirs.

**Non-goals (rejected, not deferred)**

- **A bridge that outlives fleet, or binds anything but loopback.** Weighed in §3.4 and rejected
  by the operator: when fleet dies every bridge dies with it, and re-exposing a port to the LAN
  is `expose`'s job.
- **Sandboxing or policing plugins.** Installing a plugin is trusting an executable, exactly
  like any CLI on PATH; §5 states the boundary instead of pretending to enforce it.

## 3. Options considered

### 3.1 The plugin roadmap (what "more than ssh" actually means)

The catalogue this objective was asked to produce. Every kind rides ssh as the bridge, through
the same `runner` seam and ControlMaster socket, so no new credential path exists.

| Kind | Probe (read-only) | Tree | Connect / stream actions | When |
| :-- | :-- | :-- | :-- | :-- |
| **herdr** | `status --json` via PATH candidates | herdr → sessions → agents | attach: `herdr --remote <alias> --session <name>` (local binary, tty) | **this objective** |
| **kubernetes** (k3s, kind, kubeadm) | `kubectl` / `k3s kubectl` + kubeconfig presence | contexts → namespaces → resource kinds → objects (pods → containers) | `kubectl logs -f` (stream), `kubectl exec -it` (handoff), describe/events (stream), port-forward via a `Tunnel` whose `Keeper` is `kubectl port-forward` | **next — `fleet-connect-k8s`**, planned in this PR (`../designs/fleet-connect-k8s.md`) |
| **containers** (docker, podman) | `docker ps --format json` | containers (+ compose projects) | `docker logs -f` (stream), `docker exec -it` (handoff), `docker stats --no-stream` | sequenced: `fleet-connect-containers` (index row) |
| **sessions** (tmux, Claude remote-control) | `tmux ls -F`; `capture-pane` for the `Remote Control active` marker | sessions → windows | `ssh -t <alias> tmux attach -t <s>` — absorbs the attach step of the `remote-claude-session` skill and the unbuilt `tmux-mgr remote` design (#63) | sequenced: `fleet-connect-sessions` (index row) |
| **system** | `uptime`, `free`, `nvidia-smi --query-gpu`, `tegrastats` one-shot, `command -v` for htop/btop/nvtop/nvitop/jtop | stats row · tool list · systemd units | handoff `ssh -t <alias> htop`; `journalctl -fu <unit>` stream | sequenced: `fleet-connect-system` (index row) |
| **ports** | `ss -ltnp` | listening ports, labelled (ollama, dsh, VNC, web UIs) | `t`: bridge the port to `127.0.0.1` over the mux — N ports per host ride one `ssh -N`; the local URL is printed | **this objective** (§3.4, §4.7–§4.8) |
| not planned | WoL (rejected in `fleet-tui`), mosh, serial console, RDP/VNC launch, sftp/sshfs | | | — |

### 3.2 How a tool becomes a provider

**A. Local RPC plugins, MCP-shaped, with a host-exec callback — chosen.** Providers are
processes speaking versioned JSON-RPC 2.0 over stdio; fleet is the host and exposes exactly one
capability back to them, `host/exec`, routed through `runner`. Built-in providers implement the
same Go interface in-process and can also be *served* over the protocol
(`fleet provider serve herdr`), so the wire is dogfooded by a real provider from day one.
Onboarding a plugin is a config stanza. Costs a protocol package and a loader; earns
language-independence, a stable boundary, and a transport that generalises to remote.

**B. Go interfaces only, compiled in.** Simplest and fastest. Every new tool is a fleet code
change and a release; a plugin can only be Go; nothing generalises to remote. Rejected against
the operator's explicit ask — but retained *inside* A as the in-process fast path, because a
protocol that cannot also be spoken in-process would tax the built-ins for nothing.

**C. Declarative YAML manifests interpreted by one engine.** Genuinely config-only for simple
tools, and attractive. But an engine grows a template language, a JSON-path dialect and a
conditional syntax — a programming language in YAML — the moment a tool needs anything herdr's
protocol-mismatch rule needs. Rejected *as the boundary*, kept as a **future plugin**: under A
it is one binary implementing the protocol, so it can arrive later with zero framework change
and give the config-only path for the easy 80%.

**D. Exec plugins with an ad-hoc CLI contract** (`fleet-provider-<name> probe <alias>` printing
JSON). Nearly free, and it was the tempting middle ground. But each plugin would then do its own
ssh, so quoting, multiplexing and BatchMode discipline leave fleet and multiply across plugins —
losing precisely the invariant that makes fleet's remote surface reviewable. Rejected. (A's
`host/exec` callback is this idea with the transport inverted so fleet keeps the socket.)

### 3.3 Navigation shape (decided with mockups)

**Push views + breadcrumb (k9s idiom) — chosen.** Each level is its own table with its own
columns; `esc` pops. A tree expanded in place would force every kind into one shared column set
(pod restarts and agent states squeezed into a NOTE column). A two-column split (hosts left,
detail right) would rework the derived-width rows and the bottom log pane. Push views reuse the
existing panel, log pane and status bar unchanged.

### 3.4 How N ports become N local bridges (decided 2026-09-05)

The action contract had exactly two kinds — a `Handoff` takes the terminal, a `Stream` emits
lines — and a port bridge is neither: a long-lived background process that produces no output
and has to be started, listed and stopped. The bridge is a third **declared** kind, `Tunnel`
(§4.2), and four shapes were weighed for the process behind it:

**A. One `ssh -N` per host carrying every `-L` — chosen.** Fleet keeps one background ssh per
host that has a bridge, built by `runner` as
`ssh -N -o ExitOnForwardFailure=yes <BatchMode + mux options> -L 127.0.0.1:<l>:127.0.0.1:<r> … <alias>`,
and owns its context: cancelling it tears every one of that host's bridges down at once. Adding
or removing a port restarts that one process — a sub-second blip on the host's other bridges,
accepted because it happens only on an operator keystroke. Costs one process per bridged host,
never per port; earns one lifecycle, one pure argv builder, and a `runner.Fake` test path
(`Block`) that already simulates a process that never ends on its own.

**B. `ssh -O forward` / `-O cancel` against the ControlMaster.** No extra process and no blip,
but a forward on the master dies when `ControlPersist` (10m) expires after the last client —
the wrong lifetime for a dsh session the operator is working in, and invisible when it happens.
Rejected. It remains the natural upgrade *inside* A (a keeper process plus `-O forward`) if the
restart blip ever matters.

**C. One `ssh -N -L` per port.** Simplest to reason about, but N processes per host and no
single place that knows "this host's bridges". Rejected.

**D. A detached forwarder that outlives fleet** (a daemon, `ssh -f`, or `expose`'s pidfile
model). Rejected by the operator: a bridge lives exactly as long as the fleet process that
opened it. Quitting the TUI, or Ctrl-C on `fleet bridge`, *is* the teardown.

Under A a `Tunnel` follows the same rule as the other two kinds: the provider declares it as
data — two port numbers and a URL scheme — fleet stamps the alias and turns it into a process.
There is nothing to quote: the argv is built from integers.

## 4. Decision

**The bet:** the TUI knows nothing about any provider's shape, and fleet knows nothing about any
provider's *implementation*. A level is `(columns []string, rows []Node)`; cells are positional;
a location is a `[]string` path; and every one of those types is JSON on a wire. Making the
contract serialisable from the start is what keeps behaviour out of it.

### 4.1 Units

| Unit | Does | Used by | Depends on |
| :-- | :-- | :-- | :-- |
| `pkg/provider` (**public**) | The contract and the wire: `Node`, `Action`, `Handoff`, `Stream`, `Provider`, `ErrAbsent`; the JSON-RPC method/param types; `Serve(Provider)` for plugin authors; `Client` for fleet. Pure data + transport, no ssh, no exec of remote things. | fleet, every plugin author | stdlib only |
| `internal/providers` | The registry: built-in entries, the `providers.yaml` loader, lazy plugin dial, per-call timeouts, health/status, and the `host/exec` bridge onto `runner.Runner`. | TUI nav, CLI verbs | `pkg/provider`, `internal/runner` |
| `internal/provider/herdr` | The herdr provider: `New(Deps)`; pure parsers over real captured JSON; pure, quoted remote scripts; the degraded-state rules. | the registry (built-in) and `fleet provider serve herdr` | `pkg/provider`, `internal/runner` (types) |
| `internal/runner` (extended) | Turns a declared `provider.Handoff` into an `*exec.Cmd` (`ssh -t` + mux options for remote, bare argv with no shell for local) and a set of `provider.Tunnel`s into one `ssh -N` (`BridgeArgv`, pure; `RunBridgeCtx`); `RunCtx` (a context, stdin, stderr and an exit code on the batch lane, for `Host.Exec`); `Quote` (moved here from `updexec.ShQuote`, which re-points to it, so one quoting implementation exists). The alias is a parameter fleet supplies, never a field the provider fills. | the TUI, `fleet connect`, `fleet bridge`, the `host/exec` bridge | `pkg/provider` (data only) |
| `internal/bridge` | One `Set` per alias — its forwards, the `ssh -N` context, state (`starting · up · failed · stopped`) and reason — under a `Manager` keyed by alias: `Add`/`Remove` restart the host's set, `Status()` is the table every consumer renders, `Close()` tears every host down. Readiness is a loopback TCP dial, injected so tests open no socket. | the TUI, `fleet bridge`, `fleet connect` | `pkg/provider` (data), `internal/runner` |
| `internal/provider/ports` | The ports provider: one `ss -ltnp` round trip; a small label table (11434 ollama, 3080 dsh, 5900 VNC, 6443 k8s API, …); every loopback-reachable port a row with a `t` `Tunnel` action; a port bound to a non-loopback address only is listed and refused with the reason. | the registry (built-in) | `pkg/provider` |
| `cmd/tui_nav.go`, `cmd/tui_nav_view.go`, `cmd/tui_bridge.go` | The view stack, push/pop/reload, async level loads with a generation counter, breadcrumb, the generic table renderer, `runProviderAction`; the bridge keys, the gutter marker, the level bridge line and teardown on quit. | `fleet tui` | `pkg/provider`, `internal/providers`, `internal/bridge` |
| `cmd/ls.go`, `cmd/connect.go`, `cmd/bridge.go`, `cmd/providers.go` | `fleet ls` / `fleet connect` / `fleet bridge`; `fleet providers list|check`; the hidden `fleet provider serve <name>`. | operators, scripts, protocol tests | `internal/providers`, `internal/bridge` |

### 4.2 The contract (frozen in the blocking leaf; every field is JSON)

```go
type Node struct {
    ID      string            `json:"id"`      // one path segment; the row's identity across a refresh
    Kind    string            `json:"kind"`    // the provider's namespace ("herdr-session"); fleet never switches on it
    Cells   []string          `json:"cells"`   // positional against Columns(Kind); short slice renders blanks
    Detail  string            `json:"detail"`  // one-line qualifier for the level status bar
    Leaf    bool              `json:"leaf"`    // enter is a no-op; actions still apply
    Attrs   map[string]string `json:"attrs"`   // opaque provider state carried back on the next call
    Actions []Action          `json:"actions"`
}
type Action struct {
    Key         string   `json:"key"`         // exactly one printable rune, carried as a string ("c")
    Label       string   `json:"label"`
    Unavailable string   `json:"unavailable"` // non-empty: LISTED but refused, with the reason
    Handoff     *Handoff `json:"handoff"`     // takes the terminal              } exactly
    Stream      *Stream  `json:"stream"`      // lines into the log pane         } one
    Tunnel      *Tunnel  `json:"tunnel"`      // a port bridged to 127.0.0.1     } is set
}
type Handoff struct {                          // DATA. Only internal/runner turns it into a process.
    Kind    HandoffKind `json:"kind"`          // "remote" (ssh -t + mux) | "local" (argv, no shell)
    Command string      `json:"command"`       // remote: a shell command the provider has already quoted
    Argv    []string    `json:"argv"`          // local: argv, so a hostile value is inert
}                                              // no Host field: fleet stamps the level's alias (below)
type Stream struct { Command string `json:"command"`; Follow bool `json:"follow"` }
// ReservedKeys are the printable runes the HOST TOOL owns, so a provider that declared one would
// be silently shadowed. Three groups: the navigation/search keys common to every sdk TUI
// (j k h l g G / n N ? : q space), fleet's own dashboard verbs (r s u w v a p P A F e J K), and
// this objective's bridge keys (t T). enter and esc are not printable and cannot be declared.
//
// It is a MIRROR of fleet's keymap, not the source — pkg/provider is stdlib-only and cannot
// import it — kept honest by fleet's TestEveryFleetKeyIsReservedAgainstProviders. See §4.10.
var ReservedKeys = map[rune]bool{ /* j k h l g G / n N ? : q ' ' r s u w v a p P A F e J K t T */ }
const TunnelKey = "t"   // the one reserved key a provider DOES declare: every Tunnel action carries it (found by T1's tests)
type Tunnel struct {                           // DATA. Two integers, a scheme, and at most one quoted command.
    RemotePort int    `json:"remotePort"`      // 1–65535, always on the HOST's loopback
    LocalPort  int    `json:"localPort"`       // 0: prefer RemotePort locally, else allocate
    Scheme     string `json:"scheme"`          // "http" | "https" | "" — printed before 127.0.0.1:<l>
    Keeper     string `json:"keeper"`          // optional: a provider-quoted shell command that must be
}                                              // RUNNING on the host for RemotePort to listen (kubectl
                                               // port-forward, docker run -p); fleet runs it under the
                                               // bridge's context and stops it with the bridge. "" = the
                                               // port already listens (the ports provider).

type Provider interface {
    Name() string
    Probe(ctx context.Context, h Host) (Node, error)               // bounded round trips; ErrAbsent still yields a Node
    Children(ctx context.Context, h Host, path []string, attrs map[string]string) ([]Node, error)
    Columns(kind string) []string                                   // unknown kind → nil → IDs only
}

// Host is what a provider is allowed to do to a machine: run one command on it.
// In-process it wraps runner.Runner.RunCtx; over the wire it is the host/exec callback.
// Both paths see the SAME data — stdin in; stdout, stderr and exit code out — so the
// dual-path test compares like with like. A non-zero exit is a result, not an error;
// err means the command could not be run at all (or ctx expired).
type Host interface {
    Alias() string
    Exec(ctx context.Context, stdin string, argv ...string) (ExecResult, error)
}
type ExecResult struct {
    Stdout   string `json:"stdout"`
    Stderr   string `json:"stderr"`
    ExitCode int    `json:"exitCode"`
}
```

Why positional cells and not a map: a map invites a lookup by column name, which is exactly the
coupling the bet forbids. Positional cells make "fleet cannot know a kind" structural — the same
technique as `sshconf.Host` making an exec directive *unrepresentable* rather than filtered.

Why `Host` rather than passing a `runner.Runner`: it is the single narrow capability a provider
needs, it serialises to one RPC method, and it is the reason a plugin cannot open its own
connection to a fleet machine.

Why no host field on `Handoff`, `Stream` or `Tunnel`: the machine an action runs against is the
one the operator drilled into, and fleet already knows it — the level's alias is stamped onto
the action by `runProviderAction` / `fleet connect` / `fleet bridge` at the moment it becomes a
process, exactly as `host/exec` resolves its `callId`. A provider that could name a host in a
handoff could open an interactive ssh to any fleet machine, which is the enumeration escape
the `callId` rule closes for exec; leaving the field out makes it unrepresentable on every
path. The same rule is why `Tunnel` has no remote *address*: a forward always targets the
dispatched host's own `127.0.0.1`, so a plugin cannot turn a fleet machine into a jump host.

### 4.3 The plugin protocol (v1)

JSON-RPC 2.0, one JSON object per line, over the plugin's stdin/stdout. The plugin's stderr is
captured into fleet's log (`libs/log`) and, while drilled in, the log pane. The transport is
addressed by URL in config, so `stdio` today and `unix://` / `tcp://` / an ssh-tunnelled endpoint
later change the dialer only, never the methods.

```
fleet (host)                                    plugin (provider)
  ── initialize {fleetVersion, protocol:1} ───────────▶
  ◀── {name, version, protocol:1, capabilities{levels,streams,actions}}
  ── provider/probe {callId, alias} ──────────────────▶
  ◀── {node} | {absent:{reason, node?}}
  ── provider/children {callId, alias, path[], attrs{}} ▶
  ◀── {kind, columns[], nodes[]}
  ── provider/columns {kind} ─────────────────────────▶   (for an empty level's header)
  ◀── {columns[]}
  ── shutdown {} ─────────────────────────────────────▶
  ◀── {}                                                  (so fleet can wait for a clean exit)

  ◀── host/exec {callId, argv[], stdin?} ─────────────   (plugin-initiated: the ONLY way out)
  ── {stdout, stderr, exitCode} ──────────────────────▶
  ◀── log {level, message} (notification)
```

Decisions inside the protocol, each with its reason:

- **Version negotiation is a handshake, not a hope.** `protocol: 1`; a major mismatch disables
  the plugin and renders its capability row as `plugin protocol 2, fleet speaks 1`. herdr's own
  versioned protocol is the cautionary precedent — fleet should fail the same way, legibly.
- **`host/exec` is the only outward capability**, and it lands on `runner.Runner.RunCtx` under
  the provider call's context, so BatchMode, `ConnectTimeout` and the ControlMaster socket apply
  unchanged, and `runner.Fake` drives plugin tests with no socket. Its reply `{stdout, stderr,
  exitCode}` is exactly the `ExecResult` an in-process `Host.Exec` returns, stdin included on
  both paths, so a built-in and its served twin see the same failing command the same way. It
  carries **no alias**: `callId` names the `provider/*` request the
  plugin is answering, and fleet resolves that to the machine it already chose. A plugin that
  could name a host could enumerate the fleet through exec, and concurrent calls could not be
  told apart — so the escape is unrepresentable rather than filtered. The alias still travels
  *to* the plugin so it can label rows and build handoffs; it is a name, never a route, and no
  hostname, port, user, key path or credential ever crosses the wire.
- **Streams, handoffs and tunnels are declared, never spawned by the plugin.** A plugin cannot
  take the operator's terminal, hold an open pipe or bind a port; it returns a
  `Stream`/`Handoff`/`Tunnel` and fleet runs it on the alias it dispatched. That keeps
  `tea.ExecProcess` — which suspends the entire dashboard — and every background `ssh -N` under
  fleet's control, and keeps a plugin's failure modes bounded to "answered badly" or "did not
  answer".
- **`attrs` round-trips.** A probe puts opaque state there (herdr's resolved binary path) and
  fleet hands it back on `children`, so a plugin needs no session state and can be restarted or
  reconnected between calls — the property a remote transport will need.
- **Every call has a deadline** (per-provider `timeout`, default 10s), and the clock measures
  the plugin's *own* time: it pauses while a `host/exec` the plugin issued is outstanding,
  because host time is already bounded by the runner's `ConnectTimeout` and by the context the
  bridge hands `RunCtx`. A breach fails that **call** (the row reads `timed out after 10s`) and
  kills the process; the next call to that provider re-dials it — a slow *host* must never cost
  the operator a plugin for the rest of the session. Built-ins get the same rule through the
  context on `Host.Exec`: a hung in-process probe is cancelled, not waited for. A hung plugin
  must never hang the dashboard.
- **In-process is the same interface.** Built-ins are `Provider` values; `fleet provider serve
  <name>` wraps one in `Serve()` so the *same* herdr code can be configured as an external
  plugin. That is how the protocol gets a real consumer before any third party writes one.

### 4.4 Configuration — onboarding without touching fleet

`~/.config/fleet/providers.yaml`; **absent means the built-ins, enabled, in declaration order** —
the same rule as "a missing `~/.ssh/config` is an empty fleet, not a failure".

```yaml
providers:
  - name: herdr            # built-in; listed only to set order or disable it
    enabled: true
  - name: k8s              # a plugin: no fleet rebuild, no code change
    command: ~/opt/bin/fleet-provider-k8s
    args: []
    timeout: 10s
  - name: herdr-next       # a plugin may shadow a built-in by name, to test a newer one
    provides: herdr
    command: fleet
    args: [provider, serve, herdr]
```

File order is render order. `fleet providers list` shows name · source (`builtin`/`plugin`) ·
state (`ok`, `disabled`, `handshake failed: …`) · protocol · the command. `fleet providers check
<name> [--host <alias>]` performs the handshake and one probe and prints the raw exchange — the
first thing to run when a plugin misbehaves.

### 4.5 TUI data flow

- **State:** `nav []navFrame` (empty = level 0), `navHost`, `navGen`, `reg *providers.Registry`
  (injected in `cmd/tui.go` beside `ansPath`; `nil` disables drill-down, which is what existing
  tests and demo frames get), `provStreams`, and `bridges *bridge.Manager` (one for the whole
  process, every host's sets inside it). A `navFrame` holds `path`, `kind`, `columns`, `rows`,
  an ID-keyed `cursor`, `top`, `loading`, `err`, and its own `search` state.
- **Loading:** `loadLevel(gen, depth, alias, path, reg, r)` runs off the UI thread. An empty path
  probes every enabled provider in registry order; an absent capability becomes a dimmed row
  carrying its reason; a failed plugin becomes a row saying so. A non-empty path resolves
  `path[0]` and calls `Children`. The reply is dropped when its generation no longer matches:
  bubbletea commands are asynchronous, and an operator who enters host A, escapes, then enters
  host B must not see A's rows land in B's frame.
- **Ownership:** `enter` is gated by the existing `canStartConfigAction()` — the predicate that
  already guards `s`, `p`, `P` — so a host owned by the update engine or the wake ladder cannot
  be probed mid-flight. No fifth in-flight state. While drilled in, the dashboard's engines keep
  running on other hosts; popping reveals the settled result.
- **Actions:** `runProviderAction` is the one place an `Action` becomes a process. An
  `Unavailable` action produces a status line with its reason. A `Stream`'s lines pass through
  `appendLog` (host colour, legend and `logCap` reused) but carry their own message types —
  never `bgUpdateDoneMsg`, whose handler decrements the update engine's slot count and re-polls.
  A `Handoff` becomes `runner.Command(alias, h)` under `tea.ExecProcess`, completing through
  the existing `execDoneMsg{ssh:true}` path so the row is re-polled on return — `alias` is
  `navHost`, never a value from the provider, and the handler re-polls only an alias present
  in `m.hosts`. No `handoffWrapper` banner: `herdr --remote` repaints the screen within a
  frame, and so will `kubectl exec`. A `Tunnel` is a **toggle** on the bridge manager keyed by
  `(navHost, RemotePort)`: absent → `Add` (the host's `ssh -N` restarts with one more `-L`),
  present → `Remove`. The manager reports through its own message types (`bridgeUpMsg`,
  `bridgeDoneMsg`) and its transitions are lines in the log pane via `appendLog`, so it shares
  nothing with the update engine.
- **Keys:** `keyHelp` gains a level marker and the always-visible header strip filters on it, so
  a drill-down key is never implemented-but-invisible (the defect the log pane shipped with).
  Inside a level: `enter` pushes (no-op on a `Leaf`), `esc` clears the level's filter first and
  pops otherwise, `r` reloads only this level, `t` toggles the cursor row's `Tunnel` action
  (any kind may carry one; the ports provider is merely the first), `T` stops every bridge on
  the level's host, vim motions and `/ n N` are scoped to the level, log-pane keys are
  unchanged, and `u w v space a p P A F` are **unbound** — a fleet-wide update is a dashboard
  verb, and a stray keystroke three levels down must not reinstall a fleet. **Any other
  printable key runs the cursor row's action with that key** (`c` for herdr's attach, `l d e
  x` for k8s), and the header strip lists the cursor row's action keys with their labels, so
  a provider's keys are visible without a keymap change in fleet; `ReservedKeys` (§4.2) is
  what keeps them from colliding. Bridges survive `esc`: they belong to the process, not the
  level.
- **Bridge rendering is kind-agnostic.** Fleet never adds a column to a provider's table.
  A row whose `Tunnel` is active gets a `⇄` marker in the cursor gutter (keyed on
  `(alias, RemotePort)`, so it survives a reload), the level status line lists the host's
  bridges (`⇄ 3080→http://127.0.0.1:3080 up · 11434→127.0.0.1:41234 starting`), and at level 0
  a host row with bridges carries `⇄N` in its existing NOTE column. `q` runs
  `bridges.Close()` before `tea.Quit`, so the last frame the operator sees is one with no
  bridges; the force-quit path does the same.
- **Breadcrumb:** derived from `navHost` plus the top frame's `path` — the same `[]string` that
  is the `fleet ls` argument list and the RPC `path` parameter. One list, no second source.

### 4.6 The herdr provider

```
host
└─ herdr        CAPABILITY · VERSION · PROTO · SERVER · SESSIONS      c: attach the default session
   └─ <session> SESSION · STATE · AGENTS · DIR                        c: herdr --remote <alias> --session <name>
      └─ <agent> AGENT · STATE · PANE · TITLE · CWD  (Leaf)           c: attach the session holding it
```

- **Probe is one round trip.** A POSIX-`sh` script (the shell-portability spec applies to remote
  scripts too) resolves the binary — `command -v herdr`, then `~/opt/bin/herdr`, then
  `~/.local/bin/herdr` — then emits the resolved path, `status --json` and `session list --json`,
  delimiter-separated exactly like the install-stamp probe. The resolved path travels in
  `Attrs["binary"]` as an **absolute** path — the script expands `$HOME` before emitting, so
  the value is never a `~` that a later quoted command would fail to expand — and deeper levels
  invoke herdr by it, never re-resolving PATH.
- **The sessions level costs two round trips regardless of session count** (`session list`, then
  one script looping `"$b" --session '<name>' api snapshot` over quoted names); **the agents
  level costs one**. Bounded up front, per the wake ladder's own lesson about budgets.
- **`Deps{LocalBinary, LocalStatus}`** injects the two local facts an attach decision needs — is
  a local herdr installed, and which protocol does it speak — modelled on `reach.Deps`. Cached
  once per process, never per keystroke.
- **Attach is a local handoff:** `Argv: [localHerdr, "--remote", alias, "--session", name]`.
  There is no shell, so a hostile session name is an inert argv element.
- **Degraded states are rows and reasons:**

| State | Detected by | Behaviour |
| :-- | :-- | :-- |
| absent | the probe's absent marker | dimmed row `not installed`, listing the paths tried; `Leaf`, no actions. Omitting it would render the same screen as "we did not look". |
| present, server stopped | no `server` block, or `running:false` | `SERVER=stopped`; sessions still list (the session dir is on disk); attach stays available — `herdr --remote` starting the server is the point, not a failure |
| protocol mismatch | local `client.protocol` ≠ remote `server.protocol`, or `compatible:false` | attach is listed with `Unavailable: "local client speaks 20, <host> serves 19 — update one side"` |
| no local herdr | `Deps.LocalBinary` returns nothing | attach unavailable with the reason; `fleet connect` runs the **local** client |

### 4.7 The ports provider

```
host
└─ ports        PORT · BIND · PROCESS · LABEL                          t: bridge to 127.0.0.1
```

- **Probe is one round trip:** `ss -H -ltnp` (POSIX-`sh` wrapper; `ss` absent → the row says so
  and names the fallback the operator can install). Rows are `Leaf`; there is no deeper level.
- **PROCESS** comes from `ss -p`, which names only the caller's own processes without root;
  blank is an honest cell, not a failure. **LABEL** is a small in-tree table keyed by port —
  `11434 ollama`, `3080 dsh`, `5900 VNC`, `6443 kubernetes`, `8888 jupyter`, `9100
  node-exporter` — falling back to the process name, then blank.
- **Bind rules.** A port bound to `0.0.0.0`, `::`, `127.0.0.1` or `::1` is reachable via the
  host's loopback and gets `t: Tunnel{RemotePort: p, LocalPort: 0, Scheme: guess}` where the
  scheme guess is `http` for the label table's web UIs and `""` otherwise. A port bound only to
  a non-loopback address (`192.168.0.5:5900`) is **listed with `Unavailable: "bound to
  192.168.0.5 only — not reachable via the host's loopback"`**, because a tunnel never targets
  anything but `127.0.0.1` on the host (§4.2).
- **Bridge state is not the provider's.** `Attrs` carry nothing; the `⇄` marker and the bridge
  line are fleet's, keyed on `(alias, RemotePort)`, so `r` reloads the port list without
  touching a bridge.

### 4.8 Bridges: lifecycle, ports and lifetime

- **One `ssh -N` per host.** `bridge.Set{alias, forwards []Forward{Remote, Local, Scheme}}`;
  `runner.BridgeArgv(alias, forwards)` is pure and asserted by test:
  `ssh -N -o ExitOnForwardFailure=yes` + `baseArgs` (BatchMode, ConnectTimeout, mux) +
  one `-L 127.0.0.1:<l>:127.0.0.1:<r>` per forward + `<alias>`. `runner.RunBridgeCtx` starts it
  under `exec.CommandContext` with the same `WaitDelay` discipline as `RunStreamCtx`, and on
  Linux sets `Pdeathsig=SIGTERM` so a fleet that is killed outright takes its bridges with it.
- **Keepers.** A forward whose `Tunnel.Keeper` is set gets its own host-side process first:
  fleet runs the keeper through `RunStreamCtx` (its lines go to the log pane, the same lane a
  `Stream` uses, and `Quote` discipline F2c applies to it exactly as to `Stream.Command`), then
  starts or restarts the host's `ssh -N`. Readiness is unchanged — the local dial succeeds
  only once the keeper is listening. A keeper that exits fails **its** forward with its last
  line (`kubectl port-forward` prints why); the host's other forwards stay up. `Remove` stops
  the keeper before restarting the set. This is how a ClusterIP service on a kind cluster
  becomes `http://127.0.0.1:8080` in two keystrokes and no framework change (`fleet-connect-k8s`).
- **Change = restart.** `Add`/`Remove` cancel the host's `ssh -N` context, wait for `done`, and
  start a new process with the new forward list; keepers are per-forward and are not restarted
  by a sibling's change. Other bridges on that host blip for the restart;
  bridges on other hosts are untouched. `Remove` of the last forward stops the process and
  deletes the set.
- **Local port policy.** `LocalPort: 0` prefers the remote port number (`3080 → 127.0.0.1:3080`,
  the case the operator actually types into a browser); if that local port is busy, fleet
  allocates a free one by binding `127.0.0.1:0`, releases it, and says so in the bridge line.
  An explicit `LocalPort` that is busy fails the bridge with `ExitOnForwardFailure`'s reason
  rather than silently moving. (The allocate-then-release race is accepted and named.)
- **Readiness.** `ssh -N` prints nothing on success, so the set is `starting` until an injected
  loopback dial to `<l>` succeeds (polled, bounded by `ConnectTimeout`), then `up`; if the
  process exits first the set is `failed` with its last stderr line (`bind: Address already in
  use`, `Permission denied (publickey)`, …) — the same explain-the-exit discipline the update
  engine uses.
- **Lifetime.** A set lives while its context lives; `Manager.Close()` cancels every set and
  waits. The TUI calls it on `q`, `fleet bridge` on Ctrl-C/SIGTERM, and a crash is covered by
  `Pdeathsig` on Linux. **Residual, named:** on macOS a SIGKILLed fleet can orphan an `ssh -N`;
  `fleet bridge` prints the pid of each process it starts so the operator can find it.
- **Multiple targets.** The manager is keyed by alias, so the dashboard shows `⇄N` on every
  bridged host at once, and `fleet bridge <spark>:3080 <spark>:11434 <nano>:11434` opens two
  sets in one process and prints one table:

```
HOST     REMOTE  LOCAL                    STATE     NOTE
<spark>  3080    http://127.0.0.1:3080    up
<spark>  11434   127.0.0.1:11434          up
<nano>   11434   127.0.0.1:41234          up        11434 busy locally → allocated
```

### 4.9 What this makes cheap next

- **Kubernetes** is a provider with five levels and two action types; it needs no framework
  change, and it may ship in-process or as `fleet-provider-k8s` — the decision is now a
  packaging one, not an architectural one.
- **A declarative plugin** (`fleet-provider-decl`, YAML manifest per tool) becomes the
  config-only path for simple tools without putting a template language inside fleet. It is one
  more binary speaking v1.
- **Remote providers** need a dialer and a config URL, because `attrs` round-trip, every call has
  a deadline, and no method assumes a shared filesystem or a live pipe.
- **A per-plugin `exec.allow` list** (argv[0] names a plugin may run through `host/exec`) is
  one field in `providers.yaml` and one check in the bridge, for operators who want the
  read-only property enforced on third-party plugins too.

### 4.10 The provider key space (amended 2026-09-06, after the cross-cutting review)

A provider declares its actions by key, and the host tool binds keys of its own, so the two
share one namespace and something has to arbitrate. The first version of `ReservedKeys` was
written from THIS DOCUMENT rather than from fleet's running keymap, and it drifted: six keys
fleet already bound were left free for providers to take, `l` (the streaming log pane) and `s`
(ssh to the cursor host) among them. Both were live header keys. The `fleet-connect-k8s` design
had already spent `l` on logs, so the collision was one build away from shipping.

The rule, and why it is shaped this way:

- **The host tool's keymap is the source of truth.** `provider.ReservedKeys` is a mirror.
  `pkg/provider` is stdlib-only by contract (a gate proves it) and therefore cannot import
  fleet's keymap — or, later, `sdk/libs/tui/keymap` — so the mirror cannot be derived at compile
  time.
- **The agreement is mechanical, not clerical.** `TestEveryFleetKeyIsReservedAgainstProviders`
  in `cmd` reads `keyHelp` and fails if fleet binds a rune the contract leaves free. A second
  test refuses a reserved rune that nothing binds and no comment justifies, so the list stays
  reviewable instead of accumulating. Editing the list by eye is what produced the drift; keep
  the tests passing instead.
- **Reserving a key is not spending it.** `h` and `:` are held ahead of use — `h` is navigation
  real estate (never help, which is `?`), `:` is the command line. Holding them now costs a
  provider nothing it was going to use and keeps the option open.
- **A provider's key space is what is left**: `b c d f i m o x y z`, the digits, and the
  uppercase letters fleet does not bind. herdr attaches on `c`; k8s uses `o d E x` and the
  shared `t`.

**On `sdk/libs/tui` (landed 2026-09-05) — planned for, not adopted here.** Its `keymap.Vim`
claims `h`/`l` as page-left/page-right, which would break fleet's shipped `l`. Its own guide
says a tool with no lateral axis removes those two with `Without`, and fleet's drill-down has no
lateral axis, so that is the likely resolution — but it belongs to the `fleet-tui` adoption, not
to this objective. Reserving `h` and `l` now is correct under either outcome, and the drift test
keeps working when fleet swaps `keyHelp` for a `keymap.Map`, because it derives from whatever
fleet actually uses. **Follow-on:** the mirror currently mixes shared navigation keys with
fleet's own verbs inside a package third-party plugin authors import. Splitting them belongs
with the registry (leaf C), which is where a non-fleet host would supply its own set.

## 5. Risks & blast radius

| Risk | Severity | Mitigation |
| :-- | :-- | :-- |
| **A plugin is arbitrary code** | High | Stated, not pretended away: installing a plugin is trusting an executable, exactly like installing any CLI on your PATH. Mitigations that *are* real: fleet never passes credentials, hostnames or key paths to a plugin; a plugin's only remote reach is `host/exec` on the call it is answering, so it cannot even name another machine; it cannot spawn an interactive process, hold the terminal or bind a port; it runs under a deadline. Third-party plugins are opt-in via a config file the operator writes by hand. **Not** mitigated, and said so: `host/exec` runs whatever argv the plugin sends, on the host the operator drilled into, as the ssh user — a bad plugin can write there, and reverting this PR does not undo it. The read-only guarantee (§6) is for the built-ins, whose scripts are in-tree and reviewed; a per-plugin argv allowlist is a cheap follow-on (§4.9), not a v1 promise. |
| **A provider-supplied value reaches a shell unquoted** | High | Structural: local handoffs are argv with no shell; every remote command value passes `runner.Quote`; `connect --dry-run` prints the resolved argv. Pinned by `TestLocalHandoffNeverInvokesAShell` and `TestRemoteHandoffQuotesEveryProviderSuppliedValue`. |
| **A hung or crashing plugin freezes the dashboard** | Medium | Per-call deadline, process kill, capability row rendered as failed; loads run off the UI thread; the registry never blocks `fleet status`, which consults no provider at all. |
| **A drill-down probe races an in-flight update or wake** | Medium | `enter` reuses `canStartConfigAction()`; no new ownership state to keep in sync. |
| **A late level load lands in the wrong frame** | Medium | Generation counter; stale replies dropped. Pinned by `TestALateLevelLoadForAPoppedViewIsDiscarded`. |
| **A provider stream leaks into the update engine** | Medium | Own message types and own map; only `appendLog` is shared. Pinned by `TestAProviderStreamNeverTouchesTheUpdateEngine`. |
| **The protocol proves wrong once k8s lands** | Medium | Bought down deliberately: v1 is dogfooded by a real provider over the wire (`fleet provider serve herdr`) before it is published, `RunStreamCtx` lands now so a followed log is cancellable, and the version handshake gives a clean break if v2 is needed. **Two residuals, accepted and named:** (a) an action needing operator *input* (`kubectl scale`, choosing `sh` vs `bash`) is not modelled — an interactive handoff asks for itself, and a real parameter form would be one new mode beside `modeAnswers`, additive; (b) the tree is rooted at a host, so a cluster reachable from two hosts appears twice — correct while ssh is the bridge. |
| **A bridge outlives fleet** | Medium | By construction a set dies with its context (`q`, Ctrl-C, `Close()`); `Pdeathsig` on Linux covers a SIGKILLed fleet. Residual on macOS, named in §4.8: the pid is printed. Pinned by `TestClosingTheManagerStopsEveryBridge` and `TestQuitTearsDownEveryBridgeBeforeExit`. |
| **A plugin declares a tunnel to pivot through a host** | Medium | Structural: `Tunnel` has no address field, so a forward can only target the dispatched host's `127.0.0.1`; ports are validated to 1–65535; `BridgeArgv` is built from integers, so nothing is interpolated. Pinned by `TestBridgeArgvTargetsOnlyTheHostsLoopback`. |
| **A keeper is a provider-supplied command on the host** | Medium | Same boundary and same mitigation as `Stream.Command`: declared data, run by fleet under a context it owns, every interpolated value `Quote`d (F2c), stopped with the bridge; it is the one place a tunnel touches the host beyond ssh's own forward. Pinned by `TestAKeeperRunsUnderTheBridgeContextAndStopsWithIt`. |
| **A local port collision** | Low | `ExitOnForwardFailure=yes` makes ssh exit with the reason instead of running a half-working set; `LocalPort: 0` allocates around a busy port and says so. Pinned by `TestABusyLocalPortIsAllocatedAroundAndReported`. |
| **The restart blip drops a connection on a sibling bridge** | Low | Accepted (§3.4 A); happens only on an operator keystroke; `-O forward` on a keeper process is the upgrade path if it ever matters. |
| **Protocol overhead on every drill-down** | Low | Built-ins stay in-process; only configured plugins spawn, lazily on first use, reused for the session. `fleet status` and the dashboard consult no provider. |
| **The providers config format** | Low | `gopkg.in/yaml.v3` is already a direct requirement of fleet (`internal/updplan`), so nothing is added; chosen over JSON so a hand-edited plugin registry can carry comments, as `.github/gff/features.yaml` does. |
| **Behaviour regression on the dashboard** | Low | Level 0 untouched; `reg == nil` in every existing test; no existing golden frame changes. |

Blast radius: additive code in `sdk/fleet` plus one optional user config file. No `install.sh`
change, no host-side writes, no new credential material, no daemon — a bridge is a child
process of the fleet that opened it, on the workstation only.

## 6. Rollback

Every **built-in** probe and listing is read-only on the host; the only mutations are what the
operator does inside a handed-off terminal (exactly what pressing `s` does today) — or what a
third-party plugin the operator installed chose to run through `host/exec`, which no revert
undoes. Rolling back is reverting the PR: no host, no config file and no socket carries state from this feature, and a
bridge is in-memory state of one fleet process — quitting it removes every bridge.
Two graduated kill switches exist below that: deleting or emptying `providers.yaml` returns to
built-ins only, and `reg == nil` (the injected registry) disables drill-down entirely without a
revert.

## 7. Evidence expectations

The plan captures into `plans/fleet-connect/evidence/`, not asserts:

- **Protocol:** the full JSON-RPC transcript of `initialize` → `provider/probe` →
  `host/exec` → response → `provider/children`, captured from `fleet providers check`.
- **Dogfooding:** the same herdr tree rendered twice — once in-process, once with herdr
  configured as an external plugin (`command: fleet, args: [provider, serve, herdr]`) — shown to
  be byte-identical. This is the proof that the wire is real and not a paper design.
- **Protocol failure modes:** a plugin that reports protocol 2, one that exceeds its deadline,
  and one that exits at once — each rendered as a row with its reason.
- **Contract:** the argv produced for a remote and a local handoff, showing `-t`, the mux
  options, and — for a hostile session name — the inert argv element.
- **Bounded cost:** recording-runner transcripts showing one round trip for the herdr probe, two
  for the sessions level regardless of session count, one for the agents level.
- **Degraded states as rows:** frames for absent, server-stopped and protocol-mismatched hosts,
  each with its reason on screen.
- **Nav correctness:** golden frames per level, and a transcript of the stale-reply drop.
- **Bridges:** the argv produced by `BridgeArgv` for two forwards (showing `-N`,
  `ExitOnForwardFailure`, the mux options and both `-L`s targeting `127.0.0.1`); a manager
  transcript of add → add → remove showing exactly one process per host and a restart per
  change; the busy-local-port allocation line; the `fleet bridge` table for two hosts; and a
  frame of the dashboard after `q` showing no bridge survived.
- **Live:** `fleet ls <spark> herdr` and `fleet ls <spark> herdr default` real output;
  `fleet connect <spark> herdr default --dry-run` argv; a real attach from the TUI (`enter` →
  `enter` → `c`, then return to the dashboard with the row re-polled); `fleet ls <spark>
  ports`; two bridges toggled from the ports level and `curl -sI http://127.0.0.1:3080`
  succeeding, then `q` and `ss -ltn` on the workstation showing both local ports gone;
  `fleet bridge <spark>:3080 <nano>:11434` with its table. Hostnames sanitised.
- **Regression:** `go test ./... -cover` and `./scripts/test.sh` showing the floor met with no
  existing test modified.

> Produced via `superpowers:brainstorming` (2026-09-02: shape, scope cut, navigation mockups,
> framework choice, and the operator's plugin/local-RPC direction) with the go-team architect for
> the contracts; amended 2026-09-05 after the design review (port bridges as a third action kind,
> option A of §3.4, chosen by the operator; bridges never outlive fleet). Registered in
> `../index.md`. The matching spec is `../specs/fleet-connect.md`.
