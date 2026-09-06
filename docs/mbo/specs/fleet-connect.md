# fleet-connect — provider plugin framework (local RPC) + herdr provider — spec

- **Slug:** fleet-connect
- **Date:** 2026-09-02
- **Status:** Draft
- **Relates to:** design [`../designs/fleet-connect.md`](../designs/fleet-connect.md) · builds on
  `fleet` (PR #224) and `fleet-tui` (PR #227) · issue
  [#266](https://github.com/sfc-gh-eraigosa/dotfiles/issues/266) · PR recorded in `../index.md`
- **Amended:** 2026-09-05 — port bridges (UC6, F22–F26; design §3.4, §4.7–§4.8).

## 1. Goal

An operator looking at a host on the `fleet` dashboard presses `enter` and sees what lives on
that machine — starting with herdr: its client/server version and protocol, its sessions, and
the agents inside each session with their live state. They drill to the level that matters,
press `c`, and land in it. Every level is read-only on the host and rides the ssh connection
fleet already multiplexed.

What each *tool* contributes is a **provider plugin**: a process fleet addresses over a
versioned, MCP-shaped JSON-RPC protocol on a local transport, whose only route to a machine is a
callback into fleet's own ssh seam. Adding a tool is a stanza in `~/.config/fleet/providers.yaml`
naming an executable — no fleet rebuild, no code change, and the same protocol later serves a
remote endpoint. herdr ships as the framework's first provider, in-process for speed and
simultaneously servable over the wire so v1 is proven by a real consumer rather than by a
paper design.

The same tree also answers "what is *listening* on that machine": a `ports` level lists the
host's open ports, and pressing `t` on one bridges it to `127.0.0.1` on the workstation over
the ssh connection fleet already holds — as many ports as the operator wants, on as many hosts
as they want, all shown and all torn down when fleet exits.

## 2. Use cases

### UC1 — Reach an agent session on a remote workstation
**Actor:** the operator. **Trigger:** an agent is working on `<spark>` and they want to watch or
steer it. **Flow:** `fleet` → cursor to `<spark>` → `enter` (capability list: `herdr 0.8.2,
proto 20, running, 2 sessions`) → `enter` on `herdr` (session list) → `enter` on `default`
(agents: `claude working`, `agy idle`, with titles and cwds) → `esc` back to sessions → `c` on
`default`. **Acceptance:** the herdr UI takes the terminal; on exit the dashboard returns
intact and `<spark>`'s row is re-polled. Total ssh round trips to render all three levels: four.

### UC2 — Decide whether attaching is even possible
**Actor:** the operator. **Trigger:** a host was not updated in a while. **Flow:** `enter` on the
host; the capability row shows herdr's version, protocol and server state. **Acceptance:** when
the local client and the remote server disagree on protocol, `c` is *listed* and refused with
both numbers named; when herdr is absent, the row says so and names the paths tried; when the
server is stopped, sessions still list and `c` still works because attaching starts it.

### UC3 — Onboard a new tool without touching fleet
**Actor:** the operator (or a teammate). **Trigger:** they want Kubernetes, or their own tool, in
the tree. **Flow:** write a plugin that speaks protocol v1, drop it on PATH, add a stanza to
`~/.config/fleet/providers.yaml`, run `fleet providers check <name>`. **Acceptance:** the
capability appears in the TUI and in `fleet ls` in the file's order, with no fleet rebuild; a
plugin that fails the handshake or the deadline appears as a row explaining why, and never
prevents another provider from rendering.

### UC4 — Script the tree
**Actor:** an operator or another tool (the `wlink` precedent consumes `fleet discover --json`).
**Trigger:** they need the same data without a TUI. **Flow:** `fleet ls <host> --json`, then
`fleet ls <host> herdr --json`, then `fleet connect <host> herdr default --dry-run`.
**Acceptance:** the JSON node shape is identical to the plugin wire shape; `--dry-run` prints
the exact argv that would run, and no credential or unquoted value appears in it.

### UC5 — Debug a misbehaving provider
**Actor:** whoever wrote the plugin. **Trigger:** a level renders empty or wrong. **Flow:**
`fleet providers check <name> --host <alias>`. **Acceptance:** the handshake result, one probe,
and the raw JSON-RPC exchange (including any `host/exec` the plugin requested and the plugin's
stderr) are printed; exit is non-zero when the handshake or probe failed.

### UC6 — Reach a web UI that only listens on the remote
**Actor:** the operator. **Trigger:** dsh on `<spark>` serves `127.0.0.1:3080` and ollama on
`<nano>` serves `11434`; neither is reachable from the workstation. **Flow:** `fleet` → `enter`
on `<spark>` → `enter` on `ports` (rows: `3080 · 127.0.0.1 · dsh · dsh`, `11434 · 0.0.0.0 ·
ollama · ollama`, `5900 · 192.168.0.5 · x11vnc · VNC (unavailable: bound to 192.168.0.5 only)`)
→ `t` on `3080` → the row gains `⇄` and the level line reads `⇄ 3080→http://127.0.0.1:3080 up`
→ `t` on `11434` (a second `-L` on the same process; the first blips and returns) → `esc` `esc`
→ `enter` on `<nano>` → `ports` → `t` on `11434` → `11434 busy locally → allocated`, so the
line names `127.0.0.1:41234`. The dashboard shows `⇄2` on `<spark>` and `⇄1` on `<nano>`.
**Acceptance:** `curl -sI http://127.0.0.1:3080` succeeds while the TUI runs; `T` inside a
host's ports level stops all of that host's bridges; `q` stops every bridge on every host and
`ss -ltn` on the workstation shows none of the local ports. From a script: `fleet bridge
<spark>:3080 <nano>:11434` prints the same table, holds until Ctrl-C, and tears down on exit.

## 3. Architecture

Components, each independently testable:

| Component | Path | Boundary |
| :-- | :-- | :-- |
| Contract + wire + plugin SDK | `pkg/provider` (**public**) | `Node`/`Action`/`Handoff`/`Stream`/`Provider`/`Host`, the JSON-RPC types, `Serve(Provider)` for plugin authors, `Client` for fleet. Pure data and transport: no ssh, no remote exec. |
| Registry + loader + host bridge | `internal/providers` | Built-in entries, `providers.yaml`, lazy plugin dial, per-call deadlines, health, and the `host/exec` bridge onto `runner.Runner`. |
| herdr provider | `internal/provider/herdr` | `New(Deps)`; pure parsers over captured JSON; pure quoted remote scripts; degraded-state rules. |
| Handoff execution + streams + bridges | `internal/runner` (extended) | The one place a declared `Handoff` becomes an `*exec.Cmd` and a set of `Tunnel`s becomes one `ssh -N` (`BridgeArgv`, `RunBridgeCtx`); `Quote`. The alias is always a parameter fleet passes. |
| Bridge manager | `internal/bridge` | One `Set` per alias under a `Manager`: `Add`/`Remove` (restart the host's process), `Status()`, `Close()`; local-port policy; readiness via an injected loopback dial. No ssh knowledge — it calls `runner`. |
| ports provider | `internal/provider/ports` | One `ss -ltnp` round trip; label table; a `t` `Tunnel` action per loopback-reachable port; non-loopback-only binds listed and refused. |
| Drill-down UI | `cmd/tui_nav.go`, `cmd/tui_nav_view.go`, `cmd/tui_bridge.go` | View stack, async loads with a generation counter, breadcrumb, generic table, `runProviderAction`; `t`/`T` bridge keys, the `⇄` gutter marker, the level bridge line, the `⇄N` dashboard note, teardown on `q`. |
| CLI verbs | `cmd/ls.go`, `cmd/connect.go`, `cmd/bridge.go`, `cmd/providers.go` | `fleet ls`/`connect`/`bridge`, `fleet providers list|check`, hidden `fleet provider serve <name>`. |

**Data flow (one level):** a key or a CLI verb names `(alias, path)` → the registry resolves
`path[0]` to a built-in `Provider` value or a dialed plugin `Client` → `Probe`/`Children` runs
with a deadline → a provider that needs machine data calls `Host.Exec(argv…)`, which in-process
is `runner.Runner.Run` and over the wire is the `host/exec` callback landing on the same method
→ `[]Node` returns → the TUI renders `Columns(kind)` against `Node.Cells` positionally, or the
CLI marshals the same structs to JSON.

**The single schema.** `Node` is the TUI's row, the `fleet ls --json` element and the RPC result
element. Four consumers, one shape; no adapter, nothing to drift.

**Layering rule.** `pkg/provider` depends on stdlib only. `internal/runner` may read
`pkg/provider`'s data types; no provider may import `cmd`; nothing but `internal/runner` opens a
connection to a host — a bridge included: `internal/bridge` owns state and contexts and asks
`runner` for the process.

## 4. Behavior / features

**Framework**

- **F1 Contract.** `Node` (id, kind, positional cells, detail, leaf, attrs, actions), `Action`
  (key, label, unavailable, exactly one of handoff/stream/tunnel), `Handoff` (remote command or
  local argv — **no host field**), `Stream` (command, follow), `Tunnel` (remotePort 1–65535,
  localPort 0–65535 where 0 means "prefer the remote number, else allocate", scheme — **no
  address field**), `Provider` (name, probe, children, columns), `Host` (alias, exec). All
  JSON-serialisable; `ErrAbsent`/`ErrNoSuchPath` sentinels.
- **F2 Handoff execution.** A remote handoff runs `ssh -t` with `MuxArgs` and no BatchMode; a
  local handoff runs a bare argv with no shell. `runner.Quote` is the only quoting path for
  values interpolated into a remote command string. The alias every handoff, stream and tunnel
  runs against is supplied by fleet from the level (or the CLI argument), never read from the
  action.
- **F3 Cancellable streams.** `RunStreamCtx` joins the `Runner` interface (`Exec` via
  `exec.CommandContext`, `Fake` mirrored); `RunStream` delegates to it.
- **F4 Protocol handshake.** `initialize` exchanges `{fleetVersion, protocol}` for
  `{name, version, protocol, capabilities}`; a major-version mismatch disables the plugin with a
  legible reason.
- **F5 Protocol methods.** `provider/probe`, `provider/children` (with round-tripped `attrs`),
  `provider/columns`, `shutdown` — newline-framed JSON-RPC 2.0 over stdio, transport addressed
  by URL so a socket or remote endpoint changes only the dialer.
- **F6 `host/exec` callback.** The plugin's only outward capability; lands on `runner.Runner`,
  so BatchMode, `ConnectTimeout` and the ControlMaster socket apply. It carries a `callId` — the
  `provider/*` request being answered — and **no alias**, so a plugin cannot name a machine it
  was not asked about; fleet resolves the call to the alias it dispatched. A plugin receives an
  alias for labelling only, and never a hostname, port, user, key path or credential. It cannot
  spawn an interactive process or hold an open pipe.
- **F7 Plugin lifecycle.** Lazy spawn on first use, reuse for the process lifetime, per-call
  deadline (default 10s, per-provider override), kill on breach, stderr captured to the log, and
  a failed plugin rendered as a row rather than an omission or a crash.
- **F8 Configuration.** `~/.config/fleet/providers.yaml`: file order is render order; `enabled`
  toggles; a plugin may shadow a built-in by name via `provides:`; an absent file means the
  built-ins, enabled, in declaration order.
- **F9 `fleet providers list|check`.** `list` shows name · source · state · protocol · command.
  `check <name> [--host <alias>]` performs the handshake and one probe and prints the raw
  exchange; non-zero exit on failure.
- **F10 Dogfooding.** Hidden `fleet provider serve <name>` serves a built-in over the protocol,
  so the same herdr code can be configured as an external plugin and must render identically.

**herdr provider**

- **F11 Probe.** One round trip: resolve the binary (`command -v herdr`, `~/opt/bin/herdr`,
  `~/.local/bin/herdr`), then `status --json` and `session list --json`, delimiter-separated.
  Capability row: `CAPABILITY · VERSION · PROTO · SERVER · SESSIONS`. Resolved path in
  `Attrs["binary"]`.
- **F12 Sessions level.** Two round trips regardless of session count: `session list --json`,
  then one script looping `--session <name> api snapshot` over quoted names. Columns
  `SESSION · STATE · AGENTS · DIR`.
- **F13 Agents level.** One round trip; `Leaf`. Columns `AGENT · STATE · PANE · TITLE · CWD`.
- **F14 Actions and degraded states.** `c` attaches via a **local** handoff
  `[herdr, --remote, <alias>, --session, <name>]` (capability level: the default session).
  Absent → dimmed row naming the paths tried, no actions. Server stopped → `SERVER=stopped`,
  sessions still list, attach still available. Protocol mismatch or `compatible:false` → attach
  listed and refused with both numbers. No local herdr → attach refused with that reason.

**TUI**

- **F15 Push views and breadcrumb.** `enter` pushes; `esc` pops; the breadcrumb is derived from
  the host plus the top frame's path; the banner, log pane and status bar are unchanged; level 0
  is the existing dashboard.
- **F16 Async level loads.** Loads run off the UI thread with a per-frame spinner; a reply whose
  generation no longer matches the stack is discarded.
- **F17 Ownership.** `enter` is refused for a host owned by an async path, reusing
  `canStartConfigAction()`; no new in-flight state; other hosts' background work continues while
  drilled in.
- **F18 Level-aware keymap.** `keyHelp` stays the single source of truth and gains a level
  marker; the header strip shows the current level's keys; `u w v space a p P A F` are unbound
  inside a level.
- **F19 Provider streams.** A `Stream` action's lines reach the existing log pane through
  `appendLog` (host colour, legend, `logCap`) using their own message types, never the update
  engine's.

**CLI**

- **F20 `fleet ls <host> [path…] [--json]`.** Human table or JSON `{host, path, kind, columns,
  nodes[]}`; `nodes` and `columns` never null; `id` is the next path segment verbatim.
- **F21 `fleet connect <host> <path…> [--action k] [--dry-run]`.** Runs the action on `<host>`:
  a handoff with the terminal attached; a stream with its lines on stdout until it ends or
  Ctrl-C; a tunnel as a one-entry `fleet bridge` (F26). `--dry-run` prints the resolved argv;
  an unavailable action is refused with its reason and a non-zero exit.

**Bridges**

- **F22 Bridge execution.** `runner.BridgeArgv(alias, forwards)` is pure:
  `ssh -N -o ExitOnForwardFailure=yes` + the batch/mux base options + one
  `-L 127.0.0.1:<local>:127.0.0.1:<remote>` per forward + the alias. `runner.RunBridgeCtx`
  runs it under `exec.CommandContext` with the `RunStreamCtx` `WaitDelay` discipline, stderr
  delivered as lines, and `Pdeathsig=SIGTERM` on Linux. `Fake` mirrors it (`Block` = a process
  that runs until cancelled).
- **F23 Bridge manager.** `bridge.Manager` keyed by alias; `Add(alias, Tunnel)` / `Remove(alias,
  remotePort)` restart that host's single process with the new forward list; `Status()` returns
  every set's forwards, local addresses, state (`starting · up · failed · stopped`) and reason;
  `Close()` cancels and waits for every set. Local port policy: `0` prefers the remote number,
  allocates when busy and records that it did; an explicit busy port fails with ssh's reason.
  Readiness is a loopback dial, injected. A set that exits on its own is `failed` with its
  last stderr line and is not restarted automatically.
- **F24 ports provider.** One round trip (`ss -H -ltnp`); rows `PORT · BIND · PROCESS · LABEL`,
  `Leaf`; an in-tree label table; a loopback-reachable port (`0.0.0.0`, `::`, `127.0.0.1`,
  `::1`) carries `t: Tunnel{RemotePort, 0, scheme-guess}`; a port bound only to a non-loopback
  address is listed with `Unavailable` naming the address; `ss` missing → the capability row
  says so.
- **F25 TUI bridges.** `t` toggles the cursor row's tunnel on the level's host; `T` stops every
  bridge on that host; a row with an active tunnel shows `⇄` in the gutter (keyed on `(alias,
  remotePort)`, so it survives `r`); the level status line lists the host's bridges with their
  local URLs and states; a level-0 host row with bridges shows `⇄N` in NOTE; bridge events use
  their own message types and reach the log pane via `appendLog`; `q` (and force-quit) call
  `Close()` before `tea.Quit`. Bridges survive `esc`.
- **F26 `fleet bridge <alias>:<remote>[:<local>] … [--dry-run]`.** Opens every requested bridge
  (several per host ride one process; several hosts run concurrently), prints the table
  `HOST · REMOTE · LOCAL · STATE · NOTE` and re-prints a row when its state changes, holds until
  Ctrl-C/SIGTERM, and exits non-zero if any bridge failed to come up. `--dry-run` prints one
  argv per host. It prints each process's pid when started.

## 5. Evaluation criteria (per feature)

Every rule becomes a named test. Format: **trigger · fires · must-not-fire · edge · pass**.

- **F1a** a `Node` with fewer cells than its kind's columns · renders trailing blanks · must not
  panic or overflow the row · zero cells · width-guard test.
- **F1b** an `Action` with two or three of `Handoff`/`Stream`/`Tunnel` set · rejected at
  construction/validation · must never be executed ambiguously · none set · validation test.
- **F1d** a `Tunnel` with `RemotePort` 0 or 65536, or `LocalPort` above 65535 · rejected by
  `Validate` · must never reach `BridgeArgv` · `LocalPort: 0` is valid · validation test.
- **F1c** every contract type · round-trips through JSON unchanged · must not require a custom
  adapter between wire and TUI · a `nil` `Attrs`/`Actions` · marshal/unmarshal equality test.
- **F2a** a remote handoff · argv contains `ssh`, `-t` and every `MuxArgs` option · must not
  contain `BatchMode` · empty host or command → error · argv assertion (extends
  `TestEveryRemotePathCarriesTheMuxOptions`).
- **F2b** a local handoff · exec's `argv[0]` directly · must never invoke a shell, and must not
  pass through `sh -c` · a value containing `$(…)`, `;` or a quote survives verbatim as one
  element · argv assertion.
- **F2c** a provider interpolating a value into a remote command · the value is `Quote`d ·
  an unquoted interpolation must fail review and the test · a value containing `'` · substring
  assertion on the built command.
- **F2d** an action run from a level for `<spark>` · the argv's host element is `<spark>` ·
  a provider must have no way to make it another alias (the types carry no host) · the same
  action via `fleet connect <nano> …` runs against `<nano>` · argv assertion for all three
  kinds.
- **F3a** a followed stream whose context is cancelled · the process is killed and both channels
  close · must not leak a goroutine or block forever · cancel before first line · timed test.
- **F4a** a plugin answering `protocol: 1` · initialize succeeds and the provider is enabled ·
  must not be enabled on a major mismatch · `protocol: 2` → disabled with `"plugin protocol 2,
  fleet speaks 1"` · handshake table test.
- **F4b** a plugin that exits before answering `initialize` · registry marks it failed with the
  exit status and captured stderr · must not retry forever or panic · immediate exit · fake-plugin
  test.
- **F5a** `provider/children` with `attrs` from a prior probe · the plugin receives the same map
  verbatim · fleet must not synthesise or mutate `attrs` · empty map · echo-plugin test.
- **F5b** a malformed JSON line from a plugin · that call errors with a decode reason; the
  transport stays usable or is torn down cleanly · must not corrupt a later call's reply · a
  half-written line · framing test.
- **F5c** two concurrent calls to one plugin · replies match their request ids · must never
  cross-deliver · interleaved responses · id-correlation test.
- **F6a** a plugin calling `host/exec` · the argv reaches `runner.Runner.Run` for the alias fleet
  dispatched, under BatchMode · the plugin must never receive a hostname, port, user, key path or
  password · a `stdin` payload · `runner.Fake` assertion plus a leak sweep over the marshalled
  params.
- **F6b** a plugin calling `host/exec` with an unknown or already-completed `callId` · refused
  `-32001 unknown call` · must not allow a plugin to reach a machine it was not asked about, or
  to enumerate the fleet through exec · two concurrent calls for different hosts, each exec
  landing on its own · authorization test.
- **F7a** a plugin exceeding its deadline · the call errors, the process is killed, the
  capability renders as failed · must not hang the TUI or the CLI · a plugin that sleeps forever ·
  timed test with a stub plugin.
- **F7b** one plugin failing · every other provider still renders · a failure must never abort
  the level · two plugins, one broken · registry test.
- **F7c** a plugin spawned once · reused across levels within a process · must not spawn per call ·
  three consecutive calls · spawn-count assertion.
- **F8a** no `providers.yaml` · built-ins enabled in declaration order · must not error or write
  a file · an empty file · loader test (mirrors `TestMissingConfigIsAnEmptyFleetNotAnError`).
- **F8b** a `providers.yaml` reordering entries · render order matches the file · must not fall
  back to map iteration or alphabetical order · duplicate names → error naming both · order test.
- **F8c** `enabled: false` · that provider is absent from every level and from `ls` · must not
  merely hide the row while still probing · disabling a built-in · loader + registry test.
- **F8d** a plugin with `provides: herdr` · shadows the built-in of that name · must not run both ·
  shadow plus `enabled: false` · shadow test.
- **F9a** `fleet providers list` · one row per configured provider with source, state and
  protocol · must not spawn a plugin merely to list it unless `--probe` is given · a failed
  plugin listed with its reason · golden-output test.
- **F9b** `fleet providers check <name> --host <alias>` · prints the handshake, one probe and the
  raw exchange, exit 0 · non-zero and a named reason on handshake or probe failure · a plugin
  that answers `absent` (still exit 0) · exit-code + transcript test.
- **F10a** herdr configured as an external plugin via `fleet provider serve herdr` · the rendered
  tree is byte-identical to the in-process tree at every level · any divergence is a failure ·
  the absent case over the wire · dual-path equality test.
- **F11a** a host with herdr on PATH · one round trip; the capability row carries version,
  protocol, server state and session count · must not issue a second dial for any of those
  columns · a host where only `~/.local/bin/herdr` exists (non-login PATH) · recording-runner
  round-trip count.
- **F11b** a host with no herdr anywhere · `ErrAbsent` plus a `Node` whose detail names the paths
  tried · must not be omitted from the capability list, and must expose no actions · a herdr that
  exists but errors · absent-is-a-row test.
- **F11c** a stamp-style malformed `status --json` · the row renders with a parse reason rather
  than crashing · must not report a working server on unparsable output · truncated JSON · parse
  table test over captured fixtures.
- **F12a** a host with N sessions · exactly two round trips · must not scale with N · N = 0, 1, 5 ·
  round-trip count assertion.
- **F12b** the sessions level · agent counts come from the snapshots already fetched · must not
  re-dial per session for a count · a session whose snapshot fails (count renders `-`) ·
  parse + render test.
- **F13a** the agents level for a session · one round trip; rows are `Leaf` · `enter` must do
  nothing on an agent row · a session with no agents (empty level, header still rendered) ·
  level test.
- **F14a** a session row · `c` yields a local handoff `[herdr, --remote, <alias>, --session,
  <name>]` · must not shell out or wrap the local client in fleet's ssh · a session named with
  shell metacharacters (inert argv element) · argv assertion.
- **F14b** local client protocol ≠ remote server protocol (or `compatible:false`) · every attach
  action is listed with `Unavailable` naming both numbers · must not hide the action, and must not
  attempt the attach · equal protocols → available · mismatch test.
- **F14c** a host whose herdr server is stopped · sessions still list and attach stays available;
  `SERVER` reads `stopped` · must not mark the capability absent or refuse the attach · no
  `server` block at all · stopped-server test.
- **F14d** no herdr on the workstation · attach refused with that reason · must not offer an
  attach that cannot run · a local herdr that fails `status` · local-deps test.
- **F15a** `enter` on a host row · the host list is replaced by the capability table and the
  breadcrumb names host and level · must not lose the log pane, banner or status bar · `enter` on
  an empty fleet · golden frame.
- **F15b** `esc` at the deepest level · pops one level, restoring that frame's cursor · must not
  exit the TUI or jump to level 0 · `esc` at `nav[0]` returns to the dashboard · stack test.
- **F15c** `esc` with an active filter in the level · clears the filter and stays · must not pop
  while a filter is set · filter set then cleared then `esc` · precedence test.
- **F16a** a level load in flight · the frame shows a spinner and remains navigable-safe · must
  not block keystrokes · two `r` presses (second is a no-op with a status line) · async test.
- **F16b** enter host A, `esc`, enter host B, then A's reply arrives · the reply is discarded · A's
  rows must never appear in B's frame · a reply for a popped depth · generation test.
- **F17a** `enter` on a host that is pending, updating or waking · refused with a status line ·
  must not probe or claim the row · the host becomes free, then `enter` succeeds · ownership test.
- **F17b** a background update on another host while drilled in · continues and still logs · must
  not be paused or lost by drill-down · popping shows the settled row · interleaving test.
- **F18a** every key bound inside a level · declared in `keyHelp` with its level · an
  implemented-but-undeclared key is a failure · a level-only key must not show at level 0 ·
  keymap coverage test.
- **F18b** `u`, `w`, `v`, `space`, `a`, `p`, `P`, `A`, `F` pressed inside a level · no action · a
  fleet-wide update must never start from inside a level · each key individually · unbound test.
- **F19a** a `Stream` action · lines reach the log pane with the host's colour · must not touch
  `running`, `updating` or trigger a re-poll · a stream that ends immediately · engine-isolation
  test.
- **F20a** `fleet ls <host> --json` · the documented shape, with `nodes` and `columns` as arrays ·
  must never emit `null` for either · a level with zero nodes · golden JSON.
- **F20b** `fleet ls <host> herdr default` · renders the agents table with its own columns · must
  not require the TUI · an unknown path segment → error naming it · CLI test.
- **F21a** `fleet connect <host> herdr default --dry-run` · prints the exact argv · no credential
  and no unquoted provider value may appear · a hostile session name · substring + argv assertion.
- **F21b** `fleet connect` on an action whose `Unavailable` is set · refused with the reason,
  non-zero exit · must not run the handoff anyway · `--action` naming a key that does not exist ·
  exit-code test.
- **F21c** `fleet connect` on a `Stream` action · lines reach stdout until the stream ends ·
  must not attach a tty or invoke `tea` · Ctrl-C ends it with exit 0 · CLI stream test.
- **F22a** two forwards for one alias · `BridgeArgv` yields `ssh`, `-N`,
  `ExitOnForwardFailure=yes`, every base/mux option, and two `-L 127.0.0.1:l:127.0.0.1:r` ·
  must contain no `-t`, no remote command, and no address other than `127.0.0.1` on either
  side · zero forwards → error · argv assertion.
- **F22b** a running bridge whose context is cancelled · the process is killed and `done`
  closes · must not leak a goroutine or block past `WaitDelay` · cancel before it is up ·
  timed test (`Fake.Block`).
- **F23a** `Add` twice then `Remove` once for one alias · exactly one process is running at any
  time and it was started three times · must never run two processes for one alias · `Remove`
  of the last forward leaves no process · recording-runner count.
- **F23b** two aliases each with a bridge · two processes, each argv naming its own alias ·
  `Remove` on one must not restart the other · `Close()` stops both · manager test.
- **F23c** `LocalPort: 0` with the remote number free locally · the local port equals the
  remote port · must not allocate when it need not · the remote number busy → an allocated
  port and a note saying so · injected-listen test.
- **F23d** an explicit `LocalPort` that is busy · the set is `failed` with ssh's reason ·
  must not silently move to another port · the reason reaches `Status()` · failure test.
- **F23e** a set whose process exits on its own · state `failed`, reason = last stderr line ·
  must not restart automatically or hang `Status()` · exit before the readiness dial succeeds ·
  exit test.
- **F23f** `Close()` with three sets across two hosts · every context cancelled and every
  `done` observed before it returns · must not return with a live process · `Close()` twice is
  a no-op · teardown test.
- **F24a** a host with ports bound to `0.0.0.0`, `127.0.0.1`, `::` and `192.168.0.5` · one round
  trip; the first three rows carry a `t` tunnel, the last is listed with `Unavailable` naming
  `192.168.0.5` · must not offer a tunnel to a non-loopback-only bind, and must not omit it ·
  a host with no listeners (empty level, header rendered) · fixture-driven test over real
  `ss` output.
- **F24b** a port in the label table · LABEL is the table's name and the scheme guess is
  applied · an unknown port must fall back to the process name, then blank · `ss` without
  `-p` data (blank PROCESS) · label test.
- **F24c** a host without `ss` · the capability row names the missing tool · must not error the
  level · `ss` present but failing → the row carries its stderr · absent-tool test.
- **F25a** `t` on a row with a tunnel · the manager gains `(alias, remotePort)` and the row
  shows `⇄`; `t` again removes it · must not touch `running`/`updating` or re-poll · `t` on a
  row with no tunnel → status line, nothing else · toggle test.
- **F25b** `r` on a level with an active bridge · the port list reloads and `⇄` stays on the
  same port · must not stop or restart the bridge · the port vanishing from the reload keeps
  the bridge and marks the line · reload test.
- **F25c** `T` in a host's level · every bridge on that host stops; other hosts' bridges stay ·
  must not stop bridges on other hosts · `T` with no bridges → status line · host-scoped stop.
- **F25d** `esc` to level 0 with bridges on two hosts · both host rows show `⇄N` and the bridges
  are still up · must not tear down on pop · golden frame.
- **F25e** `q` with bridges up · `Close()` runs before `tea.Quit` and the last frame shows no
  bridge · must not exit with a live process · the busy-host force-quit path does the same ·
  quit test.
- **F26a** `fleet bridge <a>:3080 <a>:11434 <b>:11434` · two processes (one per alias), a
  three-row table, exit on Ctrl-C after `Close()` · must not start one process per port ·
  `--dry-run` prints two argvs and starts nothing · CLI test with a recording runner.
- **F26b** one of three bridges fails to come up · its row reads `failed` with the reason, the
  others stay `up`, and the exit code is non-zero after Ctrl-C · must not tear the others down
  on one failure · a malformed spec (`<a>:notaport`) is refused before anything starts ·
  exit-code test.

## 6. Verification harness

- **Unit (pure).** Table tests for `pkg/provider` (marshalling, validation, cells/columns),
  `internal/provider/herdr`'s parsers and script builders over **real captured** herdr 0.8.2
  fixtures in `testdata/`, and `internal/runner`'s `HandoffArgv`.
- **Protocol.** An in-tree stub plugin (a tiny binary compiled by the test) exercises the
  handshake, version mismatch, deadline breach, immediate exit, malformed framing, concurrent id
  correlation and the `host/exec` callback. `runner.Fake` backs the bridge, so no socket opens.
- **Dual-path equality (F10a).** The herdr tree is rendered in-process and again through
  `fleet provider serve herdr`, and the two are asserted identical — the framework's keystone
  test.
- **Command.** `runner.Fake` and a recording runner drive `ls`, `connect --dry-run`,
  `bridge`, and `providers list|check`; round-trip counts are asserted, not assumed.
- **Bridges.** `bridge.Manager` is driven with `runner.Fake` (`Block` for a process that runs
  until cancelled, `Err` for one that fails at once) plus injected `listen`/`dial` functions,
  so no local port is ever bound in a unit test; process counts and argvs come from a
  recording runner. The ports provider parses **real captured** `ss -H -ltnp` output from two
  hosts in `testdata/`.
- **TUI.** Model tests against `providertest.FakeProvider` (an arbitrary five-column kind, so nav
  never depends on herdr, carrying one action of each of the three kinds); `TestDemoFrames`
  gains golden frames for the capability, sessions, agents, absent, plugin-failed, ports and
  bridged-dashboard states, inheriting the width guard.
- **Race.** `go test -race ./...`, as the module already requires.
- **Human-evidenced gates** (cannot be faked in unit tests), captured under
  `plans/fleet-connect/evidence/`: a live three-level drill-down and attach on `<spark>`; the
  same tree via the external-plugin path; a real `providers check` transcript; the argv from
  `connect --dry-run`; two bridges toggled live with a `curl` through one, then `q` and an
  `ss -ltn` on the workstation showing both gone; the `fleet bridge` table for two hosts.
- **Coverage.** New packages ≥ 90% (`pkg/provider`, `internal/providers`, `internal/provider/
  herdr`, `internal/provider/ports`, `internal/bridge`), consistent with `internal/drift` (100%)
  and `internal/sshfail` (93.3%); the module floor of 60 in `scripts/test.sh` must not fall.

## 7. Prerequisites / dependencies

- `internal/runner` gains `Handoff` execution, `Quote` (moved from `cmd.shQuote`, alias kept),
  `interactiveArgs` promoted to a package function, and `RunStreamCtx` on the `Runner` interface.
  The ten literal `runner.Exec{}` constructions and the `cmd/wake.go` type assertion stay as they
  are.
- One new dependency in fleet: `gopkg.in/yaml.v3`, already in the repo's graph via `sdk/gss` and
  `sdk/gff`, chosen so a hand-edited plugin registry can carry comments.
- `pkg/provider` is public API; `sdk/gff` and `sdk/tmux-mgr` set the `pkg/` precedent.
- herdr ≥ 0.8.2 on a host for the live gates; the local client for attach. No `install.sh`
  change, no host-side writes, no daemon.

## 8. Out of scope (and why)

| Item | Why |
| :-- | :-- |
| Kubernetes, docker, tmux/Claude sessions, system stats | The plugin roadmap (design §3.1). Each is additive under v1; k8s is the next objective and stresses depth and streaming, which is why the framework ships first. |
| Bridges that outlive fleet | Rejected by the operator (design §3.4 D): a bridge dies with the fleet process that opened it, so no forward exists on the workstation without a visible owner. A detached forwarder is `expose`'s model, not fleet's. |
| LAN exposure of a bridge (`-L 0.0.0.0:…`) | A bridge binds loopback only. Re-exposing a local port to the LAN is what `expose` does. |
| Remote-side addresses other than the host's loopback | A `Tunnel` has no address field by design: a plugin must not be able to use a fleet machine as a jump host. A port bound only to a LAN address is listed and refused with the reason. |
| Automatic restart of a bridge that died | A failed set stays `failed` with its reason; `t` again retries. A watchdog is `expose`'s job and would hide the reason. |
| A declarative YAML-manifest plugin | It is itself a plugin, so it costs no framework change later. Putting a template language inside fleet now would buy the hard 20% at the price of the whole engine. |
| Remote / socket transports | The protocol is URL-addressed and stateless per call so they stay cheap, but only local stdio is built and tested here. |
| Sandboxing or signing plugins | Installing a plugin is trusting an executable, like any CLI on PATH. The honest mitigations (no credentials, alias-scoped exec, no terminal, deadlines) are in scope; a sandbox is a different project. |
| Auto-refresh of a level | `r` reloads on demand. A tick is additive because stale replies are already discarded by generation. |
| A parameter form for actions needing input (`kubectl scale`) | Would be a new TUI mode. Interactive handoffs cover exec/attach, which is what the fleet needs now. |
| Provider-driven writes to a host | Every probe and listing is read-only; mutation happens only inside a terminal the operator was handed, exactly as `s` does today. |
| Cluster-first (host-independent) views | The tree is rooted at a host by construction while ssh is the bridge; a cluster reachable from two hosts appears under both. |

## 9. Rollback

Reverting the PR removes the feature completely: no host, config file or socket carries state
from it, and a bridge is in-memory state of one fleet process. Below that, two graduated
switches — emptying or deleting `~/.config/fleet/providers.yaml` returns to built-ins only, and a
`nil` injected registry disables drill-down entirely while leaving the dashboard untouched.

> Produced via `superpowers:brainstorming`; amended 2026-09-05 (port bridges, UC6, F22–F26) after
> the design review. The matching plan goes in `../plans/fleet-connect.md`. Register / update
> `../index.md`.
