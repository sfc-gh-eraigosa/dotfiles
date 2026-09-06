# fleet-connect — implementation plan

- **Slug:** fleet-connect
- **Date:** 2026-09-02
- **Status:** Draft
- **Relates to:** spec [`../specs/fleet-connect.md`](../specs/fleet-connect.md) · design
  [`../designs/fleet-connect.md`](../designs/fleet-connect.md) · issue
  [#266](https://github.com/sfc-gh-eraigosa/dotfiles/issues/266) · PR recorded in
  [`../index.md`](../index.md)

## 1. Summary & verdict

Build, in `sdk/fleet`: a public provider contract and a versioned local-RPC plugin protocol; the
registry, config loader and lifecycle that make onboarding a tool a `providers.yaml` stanza; the
herdr provider as the protocol's first real consumer; drill-down navigation in the TUI; the
`ls` / `connect` / `providers` CLI verbs; and — added 2026-09-05 — port bridges: a third action
kind (`Tunnel`), the `ports` provider, a bridge manager that runs one `ssh -N` per host, the
`t`/`T` keys, and `fleet bridge`. 29 tasks, strict TDD, one commit each.

**Verdict:** proceed. The design's decisions survive contact with the code — `internal/runner` is
already the sole remote seam, `reach.Deps` is the injection precedent, `keyHelp` is the one
keymap source, and `TestDemoFrames` gives golden frames with a width guard for free.

**One correction found while writing this plan** (and now reflected in the design and spec): the
`host/exec` callback must **not** take an alias. A plugin that could name a machine could
enumerate the fleet through exec, and with concurrent calls the bridge could not tell which
provider call an exec belonged to. It takes `callId` instead — the id of the `provider/*`
request it is answering — and fleet resolves that to the alias it already chose. The escape is
unrepresentable rather than filtered, the same technique `sshconf.Host` uses for exec directives.

**Amendment 2026-09-05 (design review + operator decision):** `Action` carries exactly one of
three kinds — `Handoff`, `Stream`, `Tunnel` — and none of them names a host: fleet stamps the
level's alias when the action becomes a process. A bridge is one `ssh -N` per host carrying
every `-L` (design §3.4 A), owned by `internal/bridge`, and **never outlives the fleet process**
that opened it. Leaf **H** (T25–T28) builds it; the contract pieces land in leaf A before the
freeze.

**Review corrections folded in (2026-09-05, `/code-review` on #267):** `Host.Exec` takes a
context *and* stdin and returns `ExecResult{stdout, stderr, exitCode}` on both paths, over a
new `runner.RunCtx` — the batch lane had no context, so a hung built-in could hang `loadLevel`,
and the wire and in-process shapes differed; the plugin deadline measures plugin time only and
fails the call, not the session; `Action.Key` is a one-rune string so the JSON golden reads
`"key":"c"` without the adapter F1c forbids; `Attrs["binary"]` is absolute; `runner.Quote` is
`updexec.ShQuote` moved (an alias the other way is an import cycle); `StubPlugin` is its own
`main` package under `providertest/stubplugin`; T4's gate names the §3.1 interfaces, not leaf
C's registry; §1.3's stale seam claims are corrected (`RunStreamCtx` exists, `yaml.v3` is
already required, eleven `Exec{}` sites); the read-only claim is scoped to built-ins.

**Three must-hold constraints inherited from the module** (design §5, spec §6):

- No package but `internal/runner` opens a connection to a host; providers reach a machine only
  through `Host.Exec` / `host/exec`; a bridge is a `runner` process under a context
  `internal/bridge` owns.
- No new in-flight TUI state: drill-down reuses `canStartConfigAction()`.
- A bridge binds `127.0.0.1` locally and targets `127.0.0.1` on the host, and dies with fleet.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/fleet/pkg/provider/provider.go` | `Node`, `Action`, `Handoff`, `HandoffKind`, `Stream`, `Tunnel`, `Provider`, `Host`, `ExecResult`, `ErrAbsent`, `ErrNoSuchPath`, `Validate` | F1 |
| `sdk/fleet/pkg/provider/provider_test.go` | JSON round-trip, cells/columns mismatch, action validation (one of three), tunnel port ranges | F1a–d |
| `sdk/fleet/pkg/provider/wire.go` | JSON-RPC 2.0 envelope, method/param/result types, `codec` (newline framing), id correlation | F5 |
| `sdk/fleet/pkg/provider/wire_test.go` | framing, malformed lines, concurrent id correlation | F5b, F5c |
| `sdk/fleet/pkg/provider/serve.go` | `Serve(context, Provider, io.Reader, io.Writer)` — the plugin side; `hostExec` client stub handed to the Provider | F5a, F6, F10 |
| `sdk/fleet/pkg/provider/client.go` | `Dial`/`Client` — the fleet side; handshake, version check, per-call deadline, `host/exec` dispatch to an injected `ExecFunc` | F4, F5, F7a |
| `sdk/fleet/pkg/provider/*_test.go` | handshake table (match/mismatch), immediate-exit, deadline, echo-attrs | F4a–b, F5a, F7a |
| `sdk/fleet/pkg/provider/providertest/fake.go` | `FakeProvider` (arbitrary five-column kind; one action of each of the three kinds); `BuildStub(t)` builds the stub once per test binary into `t.TempDir()` | harness for E/F/H leaves, protocol tests |
| `sdk/fleet/pkg/provider/providertest/stubplugin/main.go` | `StubPlugin`: its own `main` package (a library package cannot hold `func main`), scripted by flags/env to answer, sleep, exec, exit, or claim protocol 2 | protocol tests |
| `sdk/fleet/internal/providers/registry.go` | ordered registry; builtin + plugin entries; failure isolation; `Get`, `All`, `Status` | F7b–c, F8b |
| `sdk/fleet/internal/providers/config.go` | `providers.yaml` load: order, `enabled`, `provides` shadow, duplicate detection, absent = builtins | F8 |
| `sdk/fleet/internal/providers/host.go` | the `host/exec` bridge: `callId` → alias → `runner.Runner.RunCtx` under the call's ctx, reply = `ExecResult`; a paused deadline while outstanding; nothing else exposed | F6 |
| `sdk/fleet/internal/providers/*_test.go` | loader table, shadow, disable, one-plugin-failure isolation, spawn count, exec authorization + leak sweep | F6a–b, F7, F8 |
| `sdk/fleet/internal/provider/herdr/herdr.go` | `New(Deps) provider.Provider`: `Probe`, `Children`, `Columns`, action construction, degraded-state rules | F11–F14 |
| `sdk/fleet/internal/provider/herdr/parse.go` | pure `parseStatus`, `parseSessions`, `parseSnapshot`, `splitSections` | F11c, F12b |
| `sdk/fleet/internal/provider/herdr/script.go` | pure `probeScript()`, `snapshotScript(binary, names)` — POSIX sh, every value quoted | F11a, F12a |
| `sdk/fleet/internal/provider/herdr/testdata/{status,status-stopped,sessions,snapshot,truncated}.json` | **real captured** herdr 0.8.2 output | F11c |
| `sdk/fleet/internal/provider/herdr/*_test.go` | round-trip counts, absent, stopped, mismatch, hostile session name | F11–F14 |
| `sdk/fleet/cmd/tui_nav.go` | `navFrame`, push/pop/reload, `loadLevel`, nav messages, `runProviderAction` (stamps `navHost`), `navGen` | F2d, F15–F17, F19 |
| `sdk/fleet/cmd/tui_bridge.go` | `t`/`T` handling, `bridgeUpMsg`/`bridgeDoneMsg`, the `⇄` gutter marker, the level bridge line, the `⇄N` NOTE, `Close()` on quit | F25 |
| `sdk/fleet/cmd/tui_bridge_test.go` | toggle, reload keeps the marker, host-scoped `T`, `⇄N` at level 0, quit teardown | F25a–e |
| `sdk/fleet/cmd/tui_nav_view.go` | breadcrumb, generic kind-agnostic table, level status bar | F15 |
| `sdk/fleet/cmd/tui_nav_test.go` | push/pop, esc precedence, generation drop, ownership refusal, unbound keys, stream isolation | F15–F19 |
| `sdk/fleet/cmd/ls.go` | `fleet ls <host> [path…] [--json]` | F20 |
| `sdk/fleet/cmd/connect.go` | `fleet connect <host> <path…> [--action k] [--dry-run]` — handoff, stream to stdout, or a one-entry bridge | F21 |
| `sdk/fleet/cmd/bridge.go` | `fleet bridge <alias>:<remote>[:<local>] … [--dry-run]`: spec parsing, the table, hold until signal, `Close()`, exit code | F26 |
| `sdk/fleet/cmd/bridge_test.go` | one process per alias, table, dry-run, one-of-three failure exit code, malformed spec refused | F26a–b |
| `sdk/fleet/internal/bridge/{manager,set,ports}.go` | `Manager`, `Set`, `Forward`, local-port policy, readiness dial, `Status`, `Close` | F23 |
| `sdk/fleet/internal/bridge/*_test.go` | one process per alias, restart per change, port policy, busy explicit port, self-exit, `Close` | F23a–f |
| `sdk/fleet/internal/provider/ports/{ports,parse,labels}.go` | the ports provider: `ss` script, parser, label table, bind rules, `t` tunnel actions | F24 |
| `sdk/fleet/internal/provider/ports/testdata/ss-{spark,pi}.txt` | **real captured** `ss -H -ltnp` output | F24a |
| `sdk/fleet/internal/provider/ports/*_test.go` | bind rules, labels, absent `ss`, empty level | F24a–c |
| `sdk/fleet/cmd/providers.go` | `fleet providers list|check`; hidden `fleet provider serve <name>` | F9, F10 |
| `sdk/fleet/cmd/provider_registry.go` | the ONE place built-ins are constructed and the config is applied | F8, F10 |
| `sdk/fleet/cmd/{ls,connect,providers}_test.go` | JSON golden, dry-run argv, refusal exit code, check transcript, **dual-path equality** | F9, F10a, F20, F21 |
| `sdk/fleet/cmd/tui_model.go`, `tui_keys.go`, `tui_view.go`, `tui.go` *(edits)* | 5 fields, 5 `Update` cases, level-aware `keyHelp`/`headerHints`, registry injection | F15–F19 |
| `sdk/fleet/cmd/tui_demo_test.go` *(edit)* | golden frames: capability, sessions, agents, absent, plugin-failed, ports (one bridged), dashboard with `⇄N` | F15, F11b, F7a, F25d |
| `sdk/fleet/internal/runner/handoff.go` | `HandoffArgv(alias, h)` (pure), `Command(alias, h)`, `Quote`; `interactiveArgs` promoted to a package func | F2 |
| `sdk/fleet/internal/runner/bridge.go` | `Forward`, `BridgeArgv(alias, forwards)` (pure), `RunBridgeCtx` on the interface (`Exec` with `Pdeathsig` on Linux via a build-tagged file; `Fake` mirrored with `Block`) | F22 |
| `sdk/fleet/internal/runner/runner.go` *(edit)* | `RunCtx(ctx, host, stdin, argv…) (ExecResult, error)` and `RunBridgeCtx` added to the interface, `Exec` and `Fake` | F3, F22 |
| `sdk/fleet/internal/updexec/script.go` *(edit)* | `ShQuote` becomes `= runner.Quote` (the implementation moves; `updexec` already imports `runner`) | F2 |
| `sdk/fleet/internal/runner/handoff_test.go` | mux/`-t` presence, no-shell-for-local, quoting, bad input, alias stamping | F2a–d |
| `sdk/fleet/internal/runner/bridge_test.go` | `-N`/`ExitOnForwardFailure`/mux/`-L` shape, loopback-only, zero forwards, cancel kills | F22a–b |
| `sdk/fleet/AGENTS.md` (+ `CLAUDE.md` symlink) | new "Drill-down & providers" invariants section; `ls`/`connect`/`providers` rows in Commands | §6 |
| `sdk/fleet/README.md`, `sdk/README.md` | the drill-down and plugin-authoring tour; **real** pasted demos | §6 |
| `docs/mbo/plans/fleet-connect/evidence/**` | per-task captured output | §7 |

Touch-points deliberately **not** changed: `install.sh`, `scripts/test.sh` (the floor of 60
already covers fleet), `cmd/status.go`'s `Row`, the eleven `runner.Exec{}` construction sites,
`cmd/wake.go`'s type assertion, and `go.mod` (`yaml.v3` is already required).

## 3. Interface contracts

### 3.1 The contract (frozen at the end of Task 4 — leaf A's exit)

As written in design §4.2, plus `Validate() error` on `Node` and `Action` (exactly one of
`Handoff`/`Stream`/`Tunnel`; a `Key` that is exactly one printable rune, carried as a string; a
`Tunnel` with `RemotePort` in 1–65535
and `LocalPort` in 0–65535). None of the three action payloads carries a host or an address:
`internal/runner` takes the alias as a parameter (`HandoffArgv(alias, h)`, `BridgeArgv(alias,
fwds)`), and the only caller that supplies it is fleet. Also:

```go
// Host is the ONLY capability a provider has over a machine.
type Host interface {
    Alias() string                                                    // for labels
    Exec(ctx context.Context, stdin string, argv ...string) (ExecResult, error)
}
type ExecResult struct{ Stdout, Stderr string; ExitCode int }        // non-zero exit is a result, not an error
```

In-process, `Host` wraps `runner.Runner.RunCtx` for the alias the registry chose, under the
call's context. Over the wire it is a stub that issues `host/exec` and blocks for the reply,
which carries the same three fields. Both paths deliver `stdin`.

### 3.2 The protocol (frozen at the end of Task 8 — leaf B's exit)

JSON-RPC 2.0, one object per line, over stdio. `protocol: 1`.

```
fleet → plugin   initialize        {fleetVersion, protocol}
                                 ← {name, version, protocol, capabilities{levels,streams,actions}}
fleet → plugin   provider/probe    {callId, alias}
                                 ← {node} | {absent:{reason, node?}}
fleet → plugin   provider/children {callId, alias, path[], attrs{}}
                                 ← {kind, columns[], nodes[]}
fleet → plugin   provider/columns  {kind}                     ← {columns[]}
fleet → plugin   shutdown          {}                         ← {}
plugin → fleet   host/exec         {callId, argv[], stdin?}   ← {stdout, stderr, exitCode}
plugin → fleet   log               {level, message}           (notification)
```

**`host/exec` carries no alias.** `callId` is the id of the `provider/*` request being answered;
fleet maps it to the alias it dispatched, so a plugin cannot reach a machine it was not asked
about, and concurrent calls cannot be confused. An exec whose `callId` is unknown or already
completed is refused with `-32001 unknown call`.

`alias` is still sent *to* the plugin on `provider/*` so it can label rows; it is a name, never a
route — no hostname, port, user, key path or credential ever crosses the wire — and no action
payload can carry it back.

**The deadline measures plugin time.** The per-call clock pauses while a `host/exec` from that
call is outstanding (host time is bounded by the runner's own `ConnectTimeout` and the call's
context on `RunCtx`); a breach fails the call with `timed out after <d>`, kills the process, and
the next call re-dials it once.

### 3.3 Config (frozen at the end of Task 10)

```yaml
providers:
  - name: herdr           # built-in; present only to set order or disable
    enabled: true
  - name: k8s
    command: ~/opt/bin/fleet-provider-k8s
    args: []
    timeout: 10s          # default 10s
  - name: herdr-next
    provides: herdr       # shadows the built-in of that name
    command: fleet
    args: [provider, serve, herdr]
```

Absent file ⇒ built-ins, enabled, declaration order. Duplicate `name` (or two entries
`provides:` the same capability) is an error naming both. `~` expands via `$HOME`; `command` is
resolved on PATH when it has no separator.

### 3.4 `fleet ls --json` (frozen at the end of Task 23; a de-facto public contract)

```json
{ "host": "<alias>", "path": ["herdr"], "kind": "herdr-session",
  "columns": ["SESSION","STATE","AGENTS","DIR"],
  "nodes": [ { "id": "default", "kind": "herdr-session",
               "cells": ["default","running","2","~/.config/herdr"],
               "detail": "default session", "leaf": false,
               "attrs": {"binary":"/home/<user>/.local/bin/herdr"},
               "actions": [ {"key":"c","label":"attach herdr session default",
                             "unavailable": "",
                             "handoff": {"kind":"local","command":"",
                                         "argv":["herdr","--remote","<alias>","--session","default"]},
                             "stream": null, "tunnel": null} ] } ] }
```

`nodes` and `columns` are always arrays. `id` is the next path segment verbatim. Adding a key is
non-breaking; renaming or removing one is not. The golden is **generated by marshalling the
frozen §3.1 structs**, never hand-written, so it cannot drift from the contract (F1c forbids an
adapter): `key` is the one-rune string, `unavailable` is the refusal reason, and the three
payload keys are always present, two of them `null`.

### 3.5 `fleet bridge` (frozen at the end of Task 28)

```
$ fleet bridge <spark>:3080 <spark>:11434 <nano>:11434
HOST     REMOTE  LOCAL                    STATE     NOTE
<spark>  3080    http://127.0.0.1:3080    up        pid 41230
<spark>  11434   127.0.0.1:11434          up        pid 41230
<nano>   11434   127.0.0.1:41234          up        pid 41231 · 11434 busy locally → allocated
^C
stopped 3 bridges on 2 hosts
```

A spec is `<alias>:<remote>[:<local>]`; a malformed one is refused before anything starts. The
table is re-printed for a row whose state changes; the exit code is non-zero if any row ever
read `failed`. `--dry-run` prints one argv per host and starts nothing.

## 4. TDD build order

Each task: write the test, **observe it fail**, implement minimally, observe it pass, run the
gates, capture evidence into `plans/fleet-connect/evidence/taskNN/`, commit by explicit path.
Global gates for every task: `go test -race ./...`, `gofmt -l .` empty, `go vet ./...` clean.

### Phase 1 — the contract (leaf A)

**T1 · `pkg/provider` types.** Tests: JSON round-trip equality for every type including `nil`
`Attrs`/`Actions` (F1c); `Validate` rejects an `Action` with two or more, or none, of
`Handoff`/`Stream`/`Tunnel` (F1b) and a `Tunnel` whose ports are out of range (F1d); a `Node`
with fewer cells than columns yields blanks in a rendering helper, and zero cells does not panic
(F1a); a compile-time check that `Handoff`, `Stream` and `Tunnel` have no field named `Host` or
`Addr` (reflection over the struct, so the escape cannot be reintroduced quietly). Implement the
types and `Validate`. **Done when** the tests pass and the package imports stdlib only (`go list
-deps` shows no third-party). *Evidence:* `go test` output plus the `go list -deps` proof of a
stdlib-only public package.

**T2 · runner handoffs.** Tests: a remote handoff's argv contains `ssh`, `-t` and every
`MuxArgs()` option and **no** `BatchMode` (F2a, extending `TestEveryRemotePathCarriesTheMuxOptions`
to cover the new path); a local handoff execs `argv[0]` with a `$(…)`-bearing element surviving
verbatim and no `sh -c` anywhere (F2b); a value interpolated into a remote command appears
`Quote`d (F2c); the host element of the argv is the `alias` parameter and nothing in the
`Handoff` can change it (F2d); empty alias/command and empty argv error. Implement
`handoff.go` with `HandoffArgv(alias string, h provider.Handoff)`; promote `interactiveArgs` to
a package function; move the body of `updexec.ShQuote` to `runner.Quote` — the only direction
that avoids an import cycle, since `updexec` imports `runner` — leaving `updexec.ShQuote =
runner.Quote` and `cmd.shQuote = runner.Quote` so existing tests keep passing. **Done when** those pass and the whole module still builds.
*Evidence:* the asserted argv for both kinds.

**T3 · runner bridges + `RunCtx`.** `RunStreamCtx` already exists on the interface with tests
(`runner_ctx_test.go`, landed with fleet-update #270), so F3a is covered by
`TestRunStreamCtxKillsTheChildOnDeadline` and this task builds the bridge lane on the same
pattern. Tests: `BridgeArgv(alias, [2 forwards])` yields `ssh -N -o ExitOnForwardFailure=yes`,
every base/mux option, two `-L 127.0.0.1:l:127.0.0.1:r`, the alias last, no `-t`, no remote
command and no address but `127.0.0.1` (F22a); zero forwards errors; a running bridge whose
context is cancelled is killed and `done` closes within `WaitDelay`, also when cancelled before
it is up (F22b, `Fake.Block`); `RunCtx` returns stdout, stderr and the exit code for a non-zero
command with a nil error, delivers stdin, and is cancelled by its context when the command
blocks (F6c's runner half, F7d's runner half). Implement `bridge.go` (`Forward`, `BridgeArgv`,
`RunBridgeCtx` on `Exec` and `Fake`) plus a `//go:build linux` file setting `Pdeathsig`, and
`RunCtx` on the interface, `Exec` (`exec.CommandContext`, separate stdout/stderr buffers,
`ExitError` → `ExitCode`) and `Fake` (`Out`/`Err`/`Block` honoured). **Done when** the tests
pass with `-race` and the eleven `runner.Exec{}` call sites plus `cmd/wake.go`'s assertion are
untouched (`git diff --stat` proves it). *Evidence:* the asserted argv and the diffstat.

**T4 · test harness.** `providertest.FakeProvider` (an arbitrary five-column kind, three levels,
one leaf, one action of each of the three kinds) and `StubPlugin` in
`providertest/stubplugin/main.go` — its own `main` package, because a library package cannot
hold `func main`; `providertest.BuildStub(t)` runs `go build` once per test binary into
`t.TempDir()` and returns the path. **Done when** a smoke test drives `FakeProvider` through
the `Provider` and `Host` interfaces of §3.1 alone (a `Host` backed by `runner.Fake`, no
registry — leaf A has no in-edges), and `BuildStub` yields a binary that answers `initialize`.
*Evidence:* the smoke test output. **Leaf A exits here — the contract is frozen.**

### Phase 2 — the protocol (leaf B)

**T5 · wire + framing.** Tests: an object per line round-trips; a malformed line errors with a
decode reason and does not corrupt the next reply (F5b); two concurrent calls get their own
replies with interleaved arrival (F5c). Implement `wire.go` (envelope, methods, `codec`, a
pending-call map). **Done when** all three pass under `-race`. *Evidence:* the transcript from
the framing test.

**T6 · `Serve` (plugin side).** Test: a `Serve`d `FakeProvider` answers `initialize`,
`provider/probe`, `provider/children` with `attrs` echoed **verbatim** (F5a), and `provider/columns`.
Implement `serve.go`, including the `Host` stub that turns `Host.Exec` into a `host/exec` request
carrying the in-flight `callId`. **Done when** the test drives `Serve` over an in-memory pipe.
*Evidence:* the full JSON transcript.

**T7 · `Client` (fleet side) + handshake.** Tests: `protocol: 1` enables; `protocol: 2` disables
with `"plugin protocol 2, fleet speaks 1"` (F4a); a plugin exiting before `initialize` is marked
failed with its exit status and captured stderr, and is not retried in a loop (F4b); a call
whose plugin think-time outlives its deadline errors and the process is killed, while a call
blocked on a `host/exec` that fleet's `Fake` answers slowly does **not** fire — the clock pauses
on an outstanding exec (F7a). Implement `client.go` with an
injected `ExecFunc` for the callback. **Done when** the four cases pass using `StubPlugin`.
*Evidence:* each failure mode's rendered reason.

**T8 · `host/exec` bridge.** Tests: a plugin's `host/exec` reaches `runner.Runner.RunCtx` for
the alias fleet dispatched, under BatchMode and the call's context (F6a); a non-zero exit with
stderr and a stdin payload produce the same `ExecResult` over the wire as in-process (F6c); the params contain no hostname, port, user,
identity path or password (a leak sweep over the marshalled bytes, mirroring
`TestSudoSecretNeverAppearsInTheRemoteCommand`); an exec with an unknown or completed `callId` is
refused `-32001` (F6b). Implement `internal/providers/host.go`. **Done when** all pass with
`runner.Fake` and no socket. *Evidence:* the refusal transcript and the leak-sweep assertion.
**Leaf B exits here — the protocol is frozen.**

### Phase 3 — registry, config, verbs (leaf C)

**T9 · registry.** Tests: render order is declaration order, never map iteration (F8b); one
failing provider does not stop the others (F7b); a plugin is spawned once and reused across three
calls (F7c). Implement `registry.go`. **Done when** those pass. *Evidence:* spawn-count assertion.

**T10 · config loader.** Tests: absent file ⇒ built-ins enabled in declaration order, and nothing
is written (F8a, mirroring `TestMissingConfigIsAnEmptyFleetNotAnError`); an empty file behaves the
same; reordering the file reorders rendering; a duplicate name errors naming both (F8b);
`enabled: false` removes the provider from every level *and* stops it being probed (F8c);
`provides: herdr` shadows the built-in and both never run (F8d). `yaml.v3` is already required,
so `go mod tidy` must be a no-op. **Done when** the table passes. *Evidence:* `go test` plus the
empty `go mod tidy` diff.

**T11 · lifecycle.** Tests: a deadline breach kills the process and renders the capability as
failed without hanging (F7a); a plugin's stderr reaches the log; a plugin that dies or is killed
mid-session is re-dialed on the next call — once per call, never a loop — and, failing again,
reported; a built-in whose `Host.Exec` blocks is cancelled by the call's context and rendered
failed (F7d). Implement lazy spawn, kill, and
status. **Done when** the timed tests pass. *Evidence:* the failed-capability row.

**T12 · `providers` verbs.** Tests: `list` prints name · source · state · protocol · command and
does **not** spawn a plugin without `--probe` (F9a); `check <name> --host <alias>` prints the
handshake, one probe and the raw exchange with exit 0, non-zero with a named reason on failure,
and exit 0 for a legitimate `absent` answer (F9b). Implement `cmd/providers.go` including hidden
`fleet provider serve <name>`. **Done when** the golden output matches. *Evidence:* both
transcripts.

### Phase 4 — the herdr provider (leaf D)

**T13 · parsers.** Tests over **real captured** fixtures: `status.json`, `status-stopped.json`
(no `server` block), `sessions.json`, `snapshot.json`, `truncated.json` → a parse reason, never a
crash and never a "running" verdict (F11c). Implement `parse.go` with narrow structs.
**Done when** the table passes. *Evidence:* the fixtures' provenance header (herdr `--version`
and the capture date) plus test output.

**T14 · probe.** Tests: one round trip for the whole capability row (F11a) via a recording
runner; a host where only `~/.local/bin/herdr` exists still resolves (the verified non-login PATH
case); no herdr anywhere ⇒ `ErrAbsent` with a `Node` naming the paths tried, `Leaf`, no actions
(F11b). Implement `script.go`'s `probeScript()` and `Probe`, carrying the resolved path in
`Attrs["binary"]` as an absolute path (the script expands `$HOME` itself; a test asserts no
`~` in the attr). **Done when** the round-trip count is exactly 1 and the absent row renders.
*Evidence:* the recorded argv and the absent row.

**T15 · sessions level.** Tests: exactly two round trips for N = 0, 1, 5 sessions (F12a); agent
counts come from the already-fetched snapshots, with `-` when one fails (F12b); every session name
is quoted in the generated script. **Done when** the counts hold for all three N. *Evidence:* the
generated script and the round-trip counts.

**T16 · agents level.** Tests: one round trip; rows are `Leaf` so `enter` does nothing; a session
with no agents renders an empty level with its header (F13a). **Done when** those pass.
*Evidence:* rendered rows for a real snapshot.

**T17 · actions and degraded states.** Tests: a session's `c` yields the local handoff
`[herdr, --remote, <alias>, --session, <name>]`, with a metacharacter-laden name inert (F14a);
protocol mismatch or `compatible:false` lists every attach with both numbers named and attempts
nothing (F14b); a stopped server still lists sessions and keeps attach available (F14c); no local
herdr refuses with that reason (F14d). Implement `Deps{LocalBinary, LocalStatus}` (injected,
cached once per process). **Done when** the four cases pass. *Evidence:* the four `Unavailable`
strings and the argv.

**T18 · dual-path equality (the keystone).** Test: the herdr tree rendered in-process and again
with herdr configured as an external plugin (`command: fleet, args: [provider, serve, herdr]`) is
**byte-identical** at all three levels, including the absent case (F10a). **Done when** the
comparison passes with a `runner.Fake` behind both paths. *Evidence:* both renderings and the
diff (empty).

### Phase 5 — the TUI (leaf E)

**T19 · nav stack and loads.** Tests: `enter` replaces the host list with the capability table and
a breadcrumb, keeping banner, log pane and status bar (F15a); `esc` pops restoring that frame's
cursor, and pops to the dashboard at depth 0 (F15b); a load in flight shows a spinner and a second
`r` is a no-op with a status line (F16a); enter A → `esc` → enter B, then A's reply arrives ⇒
discarded (F16b). Implement `tui_nav.go` against `FakeProvider`, with `reg` injected in
`cmd/tui.go`. **Done when** all pass and no existing model test changes. *Evidence:* the
generation-drop transcript.

**T20 · ownership and keymap.** Tests: `enter` on a pending/updating/waking host is refused with a
status line and probes nothing, then succeeds once free (F17a); a background update on another
host continues and still logs while drilled in (F17b); every level-bound key is declared in
`keyHelp` with its level and a level-only key does not show at level 0 (F18a); each of
`u w v space a p P A F` does nothing inside a level (F18b). Implement the `routeNav` branch and
the level-aware `keyHelp`/`headerHints`. **Done when** all pass. *Evidence:* the refusal line and
the keymap coverage output.

**T21 · view and golden frames.** Tests: `esc` clears an active level filter before it pops
(F15c); `TestDemoFrames` gains capability, sessions, agents, absent and plugin-failed frames, all
inside the width guard; an unknown five-column kind renders with no TUI change. Implement
`tui_nav_view.go`. **Done when** the frames are byte-stable in the ASCII profile. *Evidence:* the
new frames.

**T22 · provider streams.** Test: a `Stream` action's lines reach the log pane with the host's
colour and touch neither `running`/`updating` nor trigger a re-poll (F19a), including a stream
that ends immediately. Implement `beginProviderStream` over `RunStreamCtx` with its own message
types. **Done when** the isolation assertion passes. *Evidence:* the log pane frame plus the
engine-state assertion.

### Phase 6 — the CLI (leaf F)

**T23 · `fleet ls`.** Tests: `--json` matches the golden shape with `nodes`/`columns` as arrays,
including a zero-node level (F20a); `fleet ls <host> herdr default` renders the agents table
without a TUI, and an unknown path segment errors naming it (F20b). The golden is produced by
marshalling the frozen structs (§3.4), so a contract change fails this test rather than a
hand-edit hiding it. **Done when** the golden JSON matches byte-for-byte. *Evidence:* the golden file and the human table.

**T24 · `fleet connect`.** Tests: `--dry-run` prints the exact argv, with no credential and no
unquoted provider value, for a hostile session name (F21a); an action whose `Unavailable` is set
is refused with its reason and a non-zero exit, as is an `--action` key that does not exist
(F21b); a `Stream` action's lines reach stdout with no tty (F21c); the argv runs against the
`<host>` argument, whatever the provider returned (F2d). **Done when** all pass. *Evidence:* the
dry-run argv and the refusal exit code.

### Phase 7 — bridges (leaf H)

**T25 · bridge manager.** Tests, all with `runner.Fake`/a recording runner and injected
`listen`/`dial`: `Add` twice then `Remove` once on one alias runs exactly one process at a time,
started three times (F23a); two aliases → two processes, each argv naming its own alias, and
`Remove` on one leaves the other running (F23b); `LocalPort: 0` takes the remote number when
free and allocates with a note when busy (F23c); an explicit busy port is `failed` with ssh's
reason, never moved (F23d); a process exiting on its own is `failed` with its last stderr line
and not restarted (F23e); `Close()` cancels every set and observes every `done`, and is
idempotent (F23f). Implement `internal/bridge`. **Done when** all pass under `-race` and the
package binds no real port (`strace`-free proof: the injected `listen` is the only listener).
*Evidence:* the add/add/remove process transcript and the allocation note.

**T26 · ports provider.** Capture **real** `ss -H -ltnp` output from `<spark>` and `<pi>` into
`testdata/` with a provenance header (host kind, `ss --version`, date). Tests: the four bind
classes render the right tunnel/`Unavailable` split in one round trip, and an empty listener
set is an empty level with its header (F24a); label-table ports get their LABEL and scheme
guess, unknown ports fall back to process name then blank (F24b); missing `ss` names the tool
on the capability row and a failing `ss` carries its stderr (F24c). Implement
`internal/provider/ports` (POSIX-`sh` wrapper; `make lint-portability` applies). **Done when**
the tests pass and the round-trip count is exactly 1. *Evidence:* the fixtures' provenance and
the rendered rows for both hosts.

**T27 · TUI bridges.** Tests against `FakeProvider`'s tunnel action: `t` adds `(alias,
remotePort)` and shows `⇄`, `t` again removes it, and neither touches `running`/`updating` nor
re-polls (F25a); `r` keeps the marker on the same port and a port that vanished keeps its bridge
with a marked line (F25b); `T` stops only this host's bridges (F25c); `esc` to level 0 keeps the
bridges up and shows `⇄N` on both host rows — a golden frame (F25d); `q` and the force-quit path
call `Close()` before `tea.Quit` and the last frame shows no bridge (F25e). Implement
`cmd/tui_bridge.go`, the `keyHelp` entries for `t`/`T` at level ≥ 1, the gutter marker, the
level bridge line and the NOTE marker. **Done when** all pass and no existing test or frame
changes. *Evidence:* the toggle transcript and the two new golden frames.

**T28 · `fleet bridge` + `connect` on a tunnel.** Tests: three specs across two aliases start
two processes, print the §3.5 table, and stop everything on the interrupt (F26a); `--dry-run`
prints two argvs and starts nothing; one failing bridge leaves the others up and makes the exit
code non-zero, and a malformed spec is refused before anything starts (F26b); `fleet connect
<host> ports 3080` is a one-entry bridge with the same table. Implement `cmd/bridge.go` and the
tunnel branch of `cmd/connect.go`. **Done when** all pass. *Evidence:* the table and the exit
codes. **Leaf H exits here.**

### Phase 8 — integration (leaf G)

**T29 · register, document, prove live.** Register herdr and ports as built-ins in
`cmd/provider_registry.go`. Write the AGENTS.md "Drill-down & providers" invariants (§6 below),
the README drill-down, bridge and plugin-authoring sections, and the `sdk/README.md` row —
**with real pasted output**. Run the live gates on `<spark>` and `<nano>`. **Done when**
`./scripts/test.sh` is green and every live gate in §7 is captured. *Evidence:* all of §7's live
captures.

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| F1a | `TestAShortCellSliceRendersBlanksNotAPanic` |
| F1b | `TestAnActionMustCarryExactlyOneOfHandoffStreamOrTunnel` |
| F1c | `TestEveryContractTypeRoundTripsThroughJSON` |
| F1d | `TestATunnelWithAnOutOfRangePortIsRejected` · `TestNoActionPayloadCarriesAHostOrAddress` |
| F2a | `TestEveryHandoffCarriesTheMuxOptions` |
| F2b | `TestLocalHandoffNeverInvokesAShell` |
| F2c | `TestRemoteHandoffQuotesEveryProviderSuppliedValue` |
| F2d | `TestTheAliasComesFromFleetNotTheProvider` |
| F3a | `TestRunStreamCtxKillsTheChildOnDeadline` (exists; `runner_ctx_test.go`) |
| F4a | `TestAProtocolMismatchDisablesThePluginWithBothNumbers` |
| F4b | `TestAPluginThatExitsBeforeInitializeIsReportedNotRetriedForever` |
| F5a | `TestAttrsRoundTripToThePluginVerbatim` |
| F5b | `TestAMalformedLineDoesNotCorruptTheNextReply` |
| F5c | `TestConcurrentCallsNeverCrossDeliverReplies` |
| F6a | `TestHostExecLandsOnTheRunnerSeamUnderBatchMode` · `TestHostExecParamsCarryNoRouteOrCredential` |
| F6b | `TestHostExecForAnUnknownCallIdIsRefused` |
| F6c | `TestHostExecSeesTheSameResultInProcessAndOverTheWire` |
| F7a | `TestAPluginThatMissesItsDeadlineIsKilledAndReportedAsARow` · `TestASlowHostExecDoesNotCountAgainstThePluginDeadline` |
| F7d | `TestAHungBuiltinExecIsCancelledByItsContext` |
| F7b | `TestOneFailingPluginNeverStopsTheOthers` |
| F7c | `TestAPluginIsSpawnedOnceAndReused` |
| F8a | `TestMissingProvidersConfigIsTheBuiltinSetNotAnError` |
| F8b | `TestRenderOrderIsFileOrder` · `TestDuplicateProviderNamesAreRefusedNamingBoth` |
| F8c | `TestADisabledProviderIsNeverProbed` |
| F8d | `TestAPluginCanShadowABuiltinByName` |
| F9a | `TestProvidersListDoesNotSpawnWithoutProbe` |
| F9b | `TestProvidersCheckPrintsTheExchangeAndExitsNonZeroOnFailure` |
| F10a | `TestTheHerdrTreeIsIdenticalInProcessAndOverTheWire` |
| F11a | `TestHerdrProbeCostsOneRoundTrip` · `TestHerdrResolvesABinaryMissingFromTheNonLoginPath` |
| F11b | `TestAbsentHerdrIsARowNotAnOmission` |
| F11c | `TestParseStatusFromRealHerdrOutput` · `TestTruncatedStatusNeverReportsARunningServer` |
| F12a | `TestSessionsLevelCostsTwoRoundTripsRegardlessOfSessionCount` |
| F12b | `TestAgentCountsComeFromTheFetchedSnapshots` |
| F13a | `TestAgentsLevelCostsOneRoundTripAndRowsAreLeaves` |
| F14a | `TestAttachUsesTheLocalBinaryAndTheRemoteAlias` · `TestAHostileSessionNameStaysAnInertArgvElement` |
| F14b | `TestAttachIsRefusedOnAProtocolMismatchWithBothNumbers` |
| F14c | `TestServerStoppedStillListsSessionsAndKeepsAttach` |
| F14d | `TestAttachIsRefusedWithoutALocalHerdr` |
| F15a | `TestEnterPushesTheCapabilityLevelKeepingTheLogPane` |
| F15b | `TestEscPopsOneLevelRestoringItsCursor` |
| F15c | `TestEscClearsAFilterBeforeItPopsALevel` |
| F16a | `TestASecondRefreshWhileLoadingIsANoOp` |
| F16b | `TestALateLevelLoadForAPoppedViewIsDiscarded` |
| F17a | `TestDrillingIntoABusyHostIsRefused` |
| F17b | `TestABackgroundUpdateContinuesWhileDrilledIn` |
| F18a | `TestKeyHelpCoversEveryBoundNavKeyAtItsLevel` |
| F18b | `TestUpdateKeysAreUnboundInsideALevel` |
| F19a | `TestAProviderStreamNeverTouchesTheUpdateEngine` |
| F20a | `TestLsJSONShapeIsStable` · `TestLsNodesIsNeverNull` |
| F20b | `TestLsRendersADeepLevelAndNamesAnUnknownSegment` |
| F21a | `TestConnectDryRunPrintsTheExactArgv` |
| F21b | `TestConnectRefusesAnUnavailableActionWithItsReason` |
| F21c | `TestConnectStreamsToStdoutWithoutATty` |
| F22a | `TestBridgeArgvTargetsOnlyTheHostsLoopback` · `TestBridgeArgvCarriesTheMuxOptionsAndExitOnForwardFailure` |
| F22b | `TestACancelledBridgeIsKilledWithinWaitDelay` |
| F23a | `TestOneProcessPerHostRestartedPerChange` |
| F23b | `TestBridgesOnTwoHostsAreIndependent` |
| F23c | `TestABusyLocalPortIsAllocatedAroundAndReported` |
| F23d | `TestAnExplicitBusyPortFailsWithSshsReason` |
| F23e | `TestASelfExitedBridgeIsFailedWithItsLastStderrLine` |
| F23f | `TestClosingTheManagerStopsEveryBridge` |
| F24a | `TestPortsLevelSplitsLoopbackReachableFromLanOnlyBinds` · `TestPortsProbeCostsOneRoundTrip` |
| F24b | `TestPortLabelsComeFromTheTableThenTheProcess` |
| F24c | `TestMissingSsIsARowNamingTheTool` |
| F25a | `TestTToggleABridgeWithoutTouchingTheUpdateEngine` |
| F25b | `TestReloadKeepsTheBridgeMarkerOnItsPort` |
| F25c | `TestTStopsOnlyThisHostsBridges` |
| F25d | `TestBridgesSurviveEscAndShowOnTheDashboard` |
| F25e | `TestQuitTearsDownEveryBridgeBeforeExit` |
| F26a | `TestBridgeVerbRunsOnePerHostAndPrintsTheTable` · `TestBridgeDryRunStartsNothing` |
| F26b | `TestOneFailedBridgeLeavesTheOthersUpAndExitsNonZero` · `TestAMalformedBridgeSpecIsRefusedBeforeStart` |

## 6. Integration & rollout

- **Build/test discovery** is by directory under `sdk/`, so the new packages are picked up by
  `scripts/test.sh` and the `Makefile` loops with no wiring. The coverage floor for `fleet` stays
  60; the new pure packages target ≥ 90 and should raise the module figure.
- **No `install.sh` change.** `fleet` is already built and installed under
  `gff install.sdk.fleet`; nothing new needs a flag, because a plugin is opt-in by the operator's
  own config file.
- **Docs.** `sdk/fleet/AGENTS.md` gains a "Drill-down & providers" invariants section:
  1. A provider never opens a socket; its only reach is `Host.Exec` / `host/exec`.
  2. `host/exec` carries a `callId`, never an alias — a plugin cannot name a machine.
  3. A level costs a **bounded** number of round trips, never one per row.
  4. The path is the contract: `fleet ls <host> <path…>`, the breadcrumb and `Children(path)` are
     one `[]string`; the `Node` shape is the TUI row, the JSON element and the wire element.
  5. No package but `internal/runner` spells `ssh` in argv; handoffs are declared data.
  6. Every provider value in a remote command is `Quote`d; local handoffs are argv with no shell.
  7. An absent capability is a row with a reason, and a failed plugin is a row with a reason —
     never an omission.
  8. An action that cannot run is listed with why, never hidden.
  9. Drilling in claims no row and is refused on a host an async path owns.
  10. A late level load for a popped view is discarded.
  11. Fleet knows no provider's kinds; columns come from `Columns(kind)`, widths from the data.
  12. `Row` stays install-drift; provider rows are `provider.Node`.
  13. Provider streams never touch the update engine.
  14. `keyHelp` remains the single keymap source, now level-aware.
  15. A missing `providers.yaml` is the built-in set, not a failure.
  16. No action payload names a host or an address: fleet stamps the level's alias, and a
      tunnel targets only that host's loopback.
  17. One `ssh -N` per bridged host, owned by `internal/bridge`; a change restarts it.
  18. A bridge never outlives fleet: `q`, Ctrl-C and `Close()` tear every set down before exit.
  Also: `README.md` gets the drill-down tour, a "bridge a port" section (`t`, `T`, `fleet
  bridge`, the lifetime rule), **and** a "write a provider plugin" section (the protocol table,
  a 30-line stub, and `fleet providers check`), and `sdk/README.md`'s fleet section gains the
  drill-down demo. Demos must be re-run and pasted, never invented.
- **Manual acceptance checklist** (the operator, on real hardware):
  1. `fleet` → `enter` on `<spark>` → capability row shows herdr's version, protocol, server
     state and session count.
  2. `enter` → sessions; `enter` → agents with live states; `esc` `esc` back to the dashboard.
  3. `c` on a session → herdr attaches; quit → the dashboard returns and the row is re-polled.
  4. `enter` on a host without herdr → a row saying so, naming the paths tried.
  5. `fleet ls <spark> herdr --json | jq .` → the documented shape.
  6. `fleet connect <spark> herdr default --dry-run` → the argv, nothing secret in it.
  7. Configure herdr as an external plugin; repeat 1–3; the trees match.
  8. Configure a deliberately broken plugin; the row explains it and the others still render.
  9. `enter` on `<spark>` → `ports`; `t` on 3080 and 11434 → `⇄` on both, the level line names
     both local URLs; `curl -sI http://127.0.0.1:3080` answers.
  10. `esc` `esc` → `⇄2` on `<spark>`; `enter` on `<nano>` → `ports` → `t` on 11434 → the
      allocation note; `T` → `<nano>`'s bridge gone, `<spark>`'s still up.
  11. `q` → `ss -ltn` on the workstation shows neither 3080 nor 11434; `fleet bridge
      <spark>:3080 <nano>:11434` prints the table, Ctrl-C prints the stop line.

### 6.1 Build leaves / DAG

**Default: one worker, tasks 1 → 29 in order.** The tasks chain through one contract and one model
struct; per MBO policy the breakout is offered, not assumed.

If the operator asks for parallel execution, the graph is:

```
A(contract) ──▶ B(protocol) ──▶ C(registry+config+verbs) ──┐
     │               │                                      ├──▶ G(integrate+docs+live)
     ├──────────────▶ D(herdr) ────────────────────────────┤
     ├──▶ E(TUI nav) ─────────────────────────────────────┤
     │               F(CLI ls/connect) ◀── C ─────────────┤
     └──▶ H(bridges: manager, ports, tui, verb) ◀── E, F ─┘
```

| Leaf | Owns (paths) | Consumes (in-edges) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| **A** contract | `pkg/provider/{provider,provider_test}.go`, `pkg/provider/providertest/**`, `internal/runner/{handoff,bridge}*.go`, `internal/runner/runner.go` | — | T1–T4 green; `pkg/provider` stdlib-only; ≥ 90% cov; the eleven `Exec{}` sites and `wake.go`'s assertion untouched | **yes (base)** |
| **B** protocol | `pkg/provider/{wire,serve,client}*.go` | A (§3.1) | T5–T8 green incl. the leak sweep and `-32001` refusal; ≥ 90% cov | **yes** (C, D-T18 consume it) |
| **C** registry+config+verbs | `internal/providers/**`, `cmd/providers.go` | A, B (§3.2) | T9–T12 green; missing-config = built-ins; ≥ 90% cov | no |
| **D** herdr | `internal/provider/herdr/**` | A, B (for T18) | T13–T18 green; real fixtures with provenance; round-trip counts 1/2/1; dual-path identical; ≥ 90% cov; zero `cmd` imports | no |
| **E** TUI nav | `cmd/tui_nav*.go`, edits to `cmd/tui_{model,keys,view}.go`, `cmd/tui.go`, `cmd/tui_demo_test.go` | A (tests use `FakeProvider`, not herdr) | T19–T22 green; new frames inside the width guard; **no existing test or frame changed** | no |
| **F** CLI ls/connect | `cmd/ls.go`, `cmd/connect.go`, their tests | A, C (§3.3) | T23–T24 green; golden JSON committed as the contract | no |
| **H** bridges | `internal/bridge/**`, `internal/provider/ports/**`, `cmd/tui_bridge*.go`, `cmd/bridge*.go`, the tunnel branch of `cmd/connect.go`, `keyHelp` rows for `t`/`T` | A (T3's `RunBridgeCtx`), E (the nav model), F (`connect`) | T25–T28 green; no real port bound in tests; ≥ 90% cov; one process per host proven by count; `Close()` before every exit path | no |
| **G** integrate+docs+live | `cmd/provider_registry.go` (final), `sdk/fleet/AGENTS.md`, `sdk/fleet/README.md`, `sdk/README.md` | B, C, D, E, F, H | T29: `./scripts/test.sh` green; all §7 live captures committed; the 11-step manual checklist signed off | no |

`cmd/` is touched by C, E, F, H and G, but in disjoint files except `cmd/connect.go` (F creates
it, H adds the tunnel branch) and `keyHelp` (E makes it level-aware, H adds two rows) — so H
starts after E and F land; `cmd/provider_registry.go` is created by C with an empty built-in
set and filled by G, so no leaf races the registration site. Run `gss feature conflicts --json`
before fan-out and rebase F onto E if they drift, never the reverse (E's edits to
`tui_keys.go`/`tui_view.go` are the larger surface).

## 7. Validation & evidence (show the work)

Evidence tree `docs/mbo/plans/fleet-connect/evidence/task01..29/`, append-only, dated headers,
hostnames sanitised, committed with each task. A feature without captured evidence is not done.

**Coverage bars.** `pkg/provider`, `internal/providers`, `internal/provider/herdr`,
`internal/provider/ports` and `internal/bridge` ≥ 90%; module floor 60 in `scripts/test.sh`
unchanged and not breached; `go test -race ./...` green.

**Adversarial scenarios covered by tests, not hope:** both-and-neither action payloads (F1b); a
shell-metacharacter session name through a local handoff (F14a) and a remote command (F2c); a
plugin claiming protocol 2 (F4a); a plugin exiting instantly (F4b); a plugin sleeping past its
deadline (F7a); a plugin writing a half line (F5b); a plugin issuing `host/exec` with a foreign or
stale `callId` (F6b) — the fleet-enumeration escape; interleaved concurrent replies (F5c); a
truncated `status --json` claiming nothing (F11c); a snapshot failing for one of five sessions
(F12b); a late reply for a popped level (F16b); drill-down racing an update or wake (F17a); a
provider stream trying to move the update engine (F19a); an action payload trying to name
another host — unrepresentable by type, asserted by reflection (F1d, F2d); a tunnel with a port
out of range (F1d); a busy local port, explicit and allocated (F23c, F23d); a bridge that dies
on its own (F23e); quitting with live bridges (F25e); one of three bridges failing (F26b).

**Live gates** (cannot be unit-tested; captured under `evidence/live/`):

1. `fleet providers check herdr --host <spark>` — the raw handshake + probe + `host/exec`
   exchange.
2. The three-level drill-down and a real attach, then the dashboard returning with the row
   re-polled.
3. The **same tree via the external-plugin path** (`command: fleet, args: [provider, serve,
   herdr]`) shown identical — the framework's keystone proof.
4. A host with no herdr rendering the absent row with the paths it tried.
5. `fleet ls <spark> herdr default --json` and `fleet connect <spark> herdr default --dry-run`.
6. A deliberately broken plugin: its row explains itself and the others still render.
7. Two bridges toggled from `<spark>`'s ports level, `curl -sI http://127.0.0.1:3080` through
   one, `⇄2` on the dashboard, then `q` and `ss -ltn` on the workstation showing both local
   ports gone.
8. `fleet bridge <spark>:3080 <nano>:11434` — the table with pids, Ctrl-C, the stop line, and
   `ss -ltn` clean afterwards.

> Produced via `superpowers:writing-plans`; amended 2026-09-05 (leaf H — bridges; the
> `/code-review` corrections in §1; T3 rewritten because `RunStreamCtx` already exists; 29
> tasks). Execute with `superpowers:executing-plans`,
> TDD throughout, using the trio in [`./fleet-connect/`](./fleet-connect/). Update
> [`../index.md`](../index.md) state as it moves.
