# fleet-connect — connections beyond ssh: a local-RPC provider plugin framework + the herdr provider — design

- **Slug:** fleet-connect
- **Date:** 2026-09-02
- **Status:** Proposed
- **Relates to:** builds on `fleet` (`./fleet.md`, PR #224) and `fleet-tui` (`./fleet-tui.md`,
  PR #227); consumes the `herdr` dossier (`./herdr.md`, PR #261). Design issue
  [#266](https://github.com/sfc-gh-eraigosa/dotfiles/issues/266); design PR recorded in
  `../index.md` row `fleet-connect`.
- **Author(s):** operator + assistant

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
- `runner.RunStream` cannot be cancelled — fine for an install that ends, wrong for a `logs -f`
  that does not.
- `sshconf.Host` has no kind field; `Row` (`cmd/status.go`) is install-drift-specific; there is
  no registry or plugin mechanism anywhere. The closest existing pattern is the wake ladder: a
  slice of uniform strategies with every impure edge injected through `reach.Deps`.
- The TUI is one column (banner · host table · log pane · status bar); cursor and selection are
  alias-keyed; `keyHelp` is the single source of truth for keys; `TestDemoFrames` renders golden
  frames and doubles as the width guard.
- `sdk/gff` and `sdk/tmux-mgr` already ship a public `pkg/` alongside `internal/`, so a
  plugin-author-facing package has precedent. `yaml.v3` is already in the repo's dependency
  graph (`sdk/gss`, `sdk/gff`), though not yet in fleet's.

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

**Non-goals (YAGNI — each is a later objective or a rejected idea)**

- Kubernetes, docker, tmux/Claude sessions, system stats with htop/nvtop/jtop handoffs, and port
  tunnels: catalogued in §3.1 as the plugin roadmap, not built here. "Basic stats" in this
  objective means herdr's own (server state, agent states).
- A **declarative (YAML-manifest) plugin** that would make simple tools truly zero-code. It is
  itself just a plugin, so the protocol makes it purely additive; §4.7 sizes it as the natural
  follow-on.
- **Remote** transports. The protocol is designed for them (URL-addressed, no stdio assumptions
  in the method layer); only local stdio ships.
- Auto-refresh of a level, a parameter form for actions needing operator input, and anything
  that writes to a target: every probe and listing is read-only on the host.
- Sandboxing plugins. Installing a plugin is trusting an executable, exactly like installing any
  CLI on your PATH; §5 states that boundary plainly rather than pretending to enforce it.

## 3. Options considered

### 3.1 The plugin roadmap (what "more than ssh" actually means)

The catalogue this objective was asked to produce. Every kind rides ssh as the bridge, through
the same `runner` seam and ControlMaster socket, so no new credential path exists.

| Kind | Probe (read-only) | Tree | Connect / stream actions | When |
| :-- | :-- | :-- | :-- | :-- |
| **herdr** | `status --json` via PATH candidates | herdr → sessions → agents | attach: `herdr --remote <alias> --session <name>` (local binary, tty) | **this objective** |
| **kubernetes** (k3s, kind, kubeadm) | kubectl / `k3s kubectl` + kubeconfig presence | contexts → namespaces → workloads → pods → containers | `kubectl logs -f` (stream), `kubectl exec -it` (handoff), describe, events, port-forward | next (`fleet-connect-k8s`) |
| **containers** (docker, podman) | `docker ps --format json` | containers (+ compose projects) | `docker logs -f` (stream), `docker exec -it` (handoff), `docker stats --no-stream` | later |
| **sessions** (tmux, Claude remote-control) | `tmux ls -F`; `capture-pane` for the `Remote Control active` marker | sessions → windows | `ssh -t <alias> tmux attach -t <s>` — absorbs the attach step of the `remote-claude-session` skill and the unbuilt `tmux-mgr remote` design (#63) | later |
| **system** | `uptime`, `free`, `nvidia-smi --query-gpu`, `tegrastats` one-shot, `command -v` for htop/btop/nvtop/nvitop/jtop | stats row · tool list · systemd units | handoff `ssh -t <alias> htop`; `journalctl -fu <unit>` stream | later |
| **ports** | `ss -ltnp` | listening ports, labelled (ollama, VNC, web UIs) | `ssh -L <local>:127.0.0.1:<port> <alias> -N`, print the local URL | later |
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
| `internal/runner` (extended) | Turns a declared `provider.Handoff` into an `*exec.Cmd` (`ssh -t` + mux options for remote, bare argv with no shell for local); `Quote`; `RunStreamCtx` on the `Runner` interface. | the TUI, `fleet connect`, the `host/exec` bridge | `pkg/provider` (data only) |
| `cmd/tui_nav.go`, `cmd/tui_nav_view.go` | The view stack, push/pop/reload, async level loads with a generation counter, breadcrumb, the generic table renderer, `runProviderAction`. | `fleet tui` | `pkg/provider`, `internal/providers` |
| `cmd/ls.go`, `cmd/connect.go`, `cmd/providers.go` | `fleet ls` / `fleet connect`; `fleet providers list|check`; the hidden `fleet provider serve <name>`. | operators, scripts, protocol tests | `internal/providers` |

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
    Key         rune     `json:"key"`
    Label       string   `json:"label"`
    Unavailable string   `json:"unavailable"` // non-empty: LISTED but refused, with the reason
    Handoff     *Handoff `json:"handoff"`     // takes the terminal    } exactly
    Stream      *Stream  `json:"stream"`      // lines into the log pane } one is set
}
type Handoff struct {                          // DATA. Only internal/runner turns it into a process.
    Kind    HandoffKind `json:"kind"`          // "remote" (ssh -t + mux) | "local" (argv, no shell)
    Host    string      `json:"host"`
    Command string      `json:"command"`       // remote: a shell command the provider has already quoted
    Argv    []string    `json:"argv"`          // local: argv, so a hostile value is inert
}
type Stream struct { Command string `json:"command"`; Follow bool `json:"follow"` }

type Provider interface {
    Name() string
    Probe(ctx context.Context, h Host) (Node, error)               // bounded round trips; ErrAbsent still yields a Node
    Children(ctx context.Context, h Host, path []string, attrs map[string]string) ([]Node, error)
    Columns(kind string) []string                                   // unknown kind → nil → IDs only
}

// Host is what a provider is allowed to do to a machine: run a read-only command.
// In-process it wraps runner.Runner; over the wire it is the host/exec callback.
type Host interface {
    Alias() string
    Exec(ctx context.Context, argv ...string) (stdout string, err error)
}
```

Why positional cells and not a map: a map invites a lookup by column name, which is exactly the
coupling the bet forbids. Positional cells make "fleet cannot know a kind" structural — the same
technique as `sshconf.Host` making an exec directive *unrepresentable* rather than filtered.

Why `Host` rather than passing a `runner.Runner`: it is the single narrow capability a provider
needs, it serialises to one RPC method, and it is the reason a plugin cannot open its own
connection to a fleet machine.

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
  ── shutdown ────────────────────────────────────────▶

  ◀── host/exec {callId, argv[], stdin?} ─────────────   (plugin-initiated: the ONLY way out)
  ── {stdout, stderr, exitCode} ──────────────────────▶
  ◀── log {level, message} (notification)
```

Decisions inside the protocol, each with its reason:

- **Version negotiation is a handshake, not a hope.** `protocol: 1`; a major mismatch disables
  the plugin and renders its capability row as `plugin protocol 2, fleet speaks 1`. herdr's own
  versioned protocol is the cautionary precedent — fleet should fail the same way, legibly.
- **`host/exec` is the only outward capability**, and it lands on `runner.Runner`, so BatchMode,
  `ConnectTimeout` and the ControlMaster socket apply unchanged, and `runner.Fake` drives plugin
  tests with no socket. It carries **no alias**: `callId` names the `provider/*` request the
  plugin is answering, and fleet resolves that to the machine it already chose. A plugin that
  could name a host could enumerate the fleet through exec, and concurrent calls could not be
  told apart — so the escape is unrepresentable rather than filtered. The alias still travels
  *to* the plugin so it can label rows and build handoffs; it is a name, never a route, and no
  hostname, port, user, key path or credential ever crosses the wire.
- **Streams and handoffs are declared, never spawned by the plugin.** A plugin cannot take the
  operator's terminal or hold an open pipe; it returns a `Stream`/`Handoff` and fleet runs it.
  That keeps `tea.ExecProcess` — which suspends the entire dashboard — under fleet's control and
  keeps a plugin's failure modes bounded to "answered badly" or "did not answer".
- **`attrs` round-trips.** A probe puts opaque state there (herdr's resolved binary path) and
  fleet hands it back on `children`, so a plugin needs no session state and can be restarted or
  reconnected between calls — the property a remote transport will need.
- **Every call has a deadline** (per-provider `timeout`, default 10s). A plugin that misses it is
  killed and marked failed; its row says so. A hung plugin must never hang the dashboard.
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
  tests and demo frames get), `provStreams`. A `navFrame` holds `path`, `kind`, `columns`,
  `rows`, an ID-keyed `cursor`, `top`, `loading`, `err`, and its own `search` state.
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
  A `Handoff` becomes `runner.Command(h)` under `tea.ExecProcess`, completing through the
  existing `execDoneMsg{ssh:true}` path so the row is re-polled on return. No `handoffWrapper`
  banner: `herdr --remote` repaints the screen within a frame, and so will `kubectl exec`.
- **Keys:** `keyHelp` gains a level marker and the always-visible header strip filters on it, so
  a drill-down key is never implemented-but-invisible (the defect the log pane shipped with).
  Inside a level: `enter` pushes (no-op on a `Leaf`), `esc` clears the level's filter first and
  pops otherwise, `r` reloads only this level, `c` runs the cursor row's `c` action, vim motions
  and `/ n N` are scoped to the level, log-pane keys are unchanged, and `u w v space a p P A F`
  are **unbound** — a fleet-wide update is a dashboard verb, and a stray keystroke three levels
  down must not reinstall a fleet.
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
  `Attrs["binary"]`, so deeper levels invoke herdr by absolute path and never re-resolve PATH.
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

### 4.7 What this makes cheap next

- **Kubernetes** is a provider with five levels and two action types; it needs no framework
  change, and it may ship in-process or as `fleet-provider-k8s` — the decision is now a
  packaging one, not an architectural one.
- **A declarative plugin** (`fleet-provider-decl`, YAML manifest per tool) becomes the
  config-only path for simple tools without putting a template language inside fleet. It is one
  more binary speaking v1.
- **Remote providers** need a dialer and a config URL, because `attrs` round-trip, every call has
  a deadline, and no method assumes a shared filesystem or a live pipe.

## 5. Risks & blast radius

| Risk | Severity | Mitigation |
| :-- | :-- | :-- |
| **A plugin is arbitrary code** | High | Stated, not pretended away: installing a plugin is trusting an executable, exactly like installing any CLI on your PATH. Mitigations that *are* real: fleet never passes credentials, hostnames or key paths to a plugin; a plugin's only remote reach is `host/exec` on the call it is answering, so it cannot even name another machine; it cannot spawn an interactive process or hold the terminal; it runs under a deadline. Third-party plugins are opt-in via a config file the operator writes by hand. |
| **A provider-supplied value reaches a shell unquoted** | High | Structural: local handoffs are argv with no shell; every remote command value passes `runner.Quote`; `connect --dry-run` prints the resolved argv. Pinned by `TestLocalHandoffNeverInvokesAShell` and `TestRemoteHandoffQuotesEveryProviderSuppliedValue`. |
| **A hung or crashing plugin freezes the dashboard** | Medium | Per-call deadline, process kill, capability row rendered as failed; loads run off the UI thread; the registry never blocks `fleet status`, which consults no provider at all. |
| **A drill-down probe races an in-flight update or wake** | Medium | `enter` reuses `canStartConfigAction()`; no new ownership state to keep in sync. |
| **A late level load lands in the wrong frame** | Medium | Generation counter; stale replies dropped. Pinned by `TestALateLevelLoadForAPoppedViewIsDiscarded`. |
| **A provider stream leaks into the update engine** | Medium | Own message types and own map; only `appendLog` is shared. Pinned by `TestAProviderStreamNeverTouchesTheUpdateEngine`. |
| **The protocol proves wrong once k8s lands** | Medium | Bought down deliberately: v1 is dogfooded by a real provider over the wire (`fleet provider serve herdr`) before it is published, `RunStreamCtx` lands now so a followed log is cancellable, and the version handshake gives a clean break if v2 is needed. **Two residuals, accepted and named:** (a) an action needing operator *input* (`kubectl scale`, choosing `sh` vs `bash`) is not modelled — an interactive handoff asks for itself, and a real parameter form would be one new mode beside `modeAnswers`, additive; (b) the tree is rooted at a host, so a cluster reachable from two hosts appears twice — correct while ssh is the bridge. |
| **Protocol overhead on every drill-down** | Low | Built-ins stay in-process; only configured plugins spawn, lazily on first use, reused for the session. `fleet status` and the dashboard consult no provider. |
| **New dependency (`yaml.v3`) in fleet** | Low | Already in the repo's graph via `sdk/gss` and `sdk/gff`; chosen over JSON so a hand-edited plugin registry can carry comments, as `.github/gff/features.yaml` does. |
| **Behaviour regression on the dashboard** | Low | Level 0 untouched; `reg == nil` in every existing test; no existing golden frame changes. |

Blast radius: additive code in `sdk/fleet` plus one optional user config file. No `install.sh`
change, no host-side writes, no new credential material, no daemon.

## 6. Rollback

Every probe and listing is read-only on the host; the only mutations are what the operator does
inside a handed-off terminal, which is exactly what pressing `s` does today. Rolling back is
reverting the PR: no host, no config file and no socket carries state from this feature.
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
- **Live:** `fleet ls <spark> herdr` and `fleet ls <spark> herdr default` real output;
  `fleet connect <spark> herdr default --dry-run` argv; a real attach from the TUI (`enter` →
  `enter` → `c`, then return to the dashboard with the row re-polled). Hostnames sanitised.
- **Regression:** `go test ./... -cover` and `./scripts/test.sh` showing the floor met with no
  existing test modified.

> Produced via `superpowers:brainstorming` (2026-09-02: shape, scope cut, navigation mockups,
> framework choice, and the operator's plugin/local-RPC direction) with the go-team architect for
> the contracts. Registered in `../index.md`. The matching spec is `../specs/fleet-connect.md`.
