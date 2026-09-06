# fleet-connect — execution cursor

- **Slug:** fleet-connect
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../fleet-connect.md`](../fleet-connect.md) — every task/§ reference
  points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after you ran the
> command and read the output. After finishing a `###` task: capture evidence, update
> `TRACKING.md`, commit (staging by explicit path, after the interactive confirm).
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate · `EVID` capture
> output into `evidence/taskNN/` · `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` ·
> `LEDGER` update TRACKING.md.

## Preflight (once)

- [x] `cd sdk/fleet && go version` → ≥ 1.26.3 (host go is 1.26.1 with `GOTOOLCHAIN=local` pinned by a shell function; use `export GOTOOLCHAIN=auto` + `command go`)
- [x] `cd sdk/fleet && go test -race ./... && gofmt -l . && go vet ./...` → green, empty, clean
- [ ] `./scripts/test.sh` → green (fleet floor 60)
- [ ] `make lint-shell && make lint-portability` → green
- [x] `git rev-parse --show-toplevel && git branch --show-current` → this worktree, not `~/git/dotfiles` (`feature/fleet-connect/<user>/contract`)
- [ ] `fleet status <spark>` and `ssh <spark> '~/.local/bin/herdr status'` → a live herdr host exists
- [ ] `command -v herdr || ls ~/.local/bin/herdr` → a local client exists (needed for attach)
- [x] `mkdir -p docs/mbo/plans/fleet-connect/evidence` then `git status --short -- docs/mbo/plans/fleet-connect/evidence` → tracked (`!docs/**` and `!sdk/**` already allow both)

---

## Phase 1 — the contract (leaf A · **PR 1 `contract`**, base `main`)

### Task 1 — `pkg/provider` types  (plan T1)

- [x] RED: `TestEveryContractTypeRoundTripsThroughJSON`, `TestAnActionMustCarryExactlyOneOfHandoffStreamOrTunnel`, `TestATunnelWithAnOutOfRangePortIsRejected`, `TestNoActionPayloadCarriesAHostOrAddress` (reflection), `TestAReservedKeyIsRejectedAtValidation`, `TestAShortCellSliceRendersBlanksNotAPanic` (+ `TestATunnelActionUsesFleetsTunnelKey`, `TestActionPayloadsAreValidated`, `TestANodeIsOnePathSegmentWithValidActions`, `TestSentinelsSurviveWrapping`)
- [x] RUN-RED: `go test ./pkg/provider/` → **FAIL** observed (`undefined: Node …`, evidence/task01/red.txt)
- [x] GREEN: `pkg/provider/provider.go` — `Node`, `Action`, `Handoff` (no host), `HandoffKind`, `Stream`, `Tunnel` (no address), `TunnelKey`, `ReservedKeys`, `Provider`, `Host`, `ExecResult`, `ErrAbsent`, `ErrNoSuchPath`, `Validate`, `Row`
- [x] RUN-GREEN: `go test ./pkg/provider/ -cover` → **PASS**, 100.0%
- [x] VERIFY: `go list -deps ./pkg/provider` → stdlib only (third-party count 0)
- [x] EVID + ALLOWLIST + COMMIT + LEDGER

**Done when:** the five tests pass and the public package is proven stdlib-only.

### Task 2 — runner handoffs  (plan T2)

- [x] RED: `TestEveryHandoffCarriesTheMuxOptions`, `TestLocalHandoffNeverInvokesAShell`, `TestRemoteHandoffQuotesEveryProviderSuppliedValue`, `TestTheAliasComesFromFleetNotTheProvider` + `TestHandoffArgvRefusesEmptyInputs`, `TestInteractiveArgsIsTheTerminalOwningLane`, `updexec.TestShQuoteIsRunnerQuote`
- [x] RUN-RED: `go test ./internal/runner/ ./internal/updexec/` → **FAIL** observed (`undefined: HandoffArgv …`, evidence/task02/red.txt)
- [x] GREEN: `internal/runner/handoff.go` — `HandoffArgv(alias, h)` (pure), `Command(alias, h)`, `Quote`, `InteractiveArgs`; `updexec.ShQuote = runner.Quote`, `cmd.shQuote = runner.Quote`; `Exec.interactiveArgs` delegates
- [x] RUN-GREEN: `go test -race ./internal/runner/ ./internal/updexec/ ./cmd/` → **PASS**
- [x] VERIFY: remote argv is `ssh` + `InteractiveArgs(alias)` + command (has `-t` + mux, no `BatchMode`); local argv verbatim with no `sh -c`; eleven `runner.Exec{}` sites untouched
- [x] EVID + COMMIT + LEDGER

**Done when:** both handoff kinds are asserted by argv and the module still builds.

### Task 3 — runner bridges + `RunCtx`  (plan T3)

- [x] SETUP: confirmed `RunStreamCtx` already exists — `TestRunStreamCtxKillsTheChildOnDeadline` PASSes untouched (evidence/task03/setup.txt)
- [x] RED: `TestBridgeArgvTargetsOnlyTheHostsLoopback`, `TestBridgeArgvCarriesTheMuxOptionsAndExitOnForwardFailure`, `TestACancelledBridgeIsKilledWithinWaitDelay` (+ `TestBridgeArgvRefusesBadInput`, `TestFakeBridgeHonoursBlockAndErr`); `TestRunCtxReturnsStderrAndExitCodeWithoutAnError`, `TestRunCtxIsCancelledByItsContext`, `TestFakeRunCtxHonoursOutErrStdinAndBlock`, `TestExecAndFakeCarryTheCtxAndBridgeCapabilities`
- [x] RUN-RED: `go test ./internal/runner/ -run 'Bridge|RunCtx|Capabilit'` → **FAIL** observed (`undefined: Forward`, `no field or method BridgeArgv`; evidence/task03/red.txt)
- [x] GREEN: `internal/runner/bridge.go` — `Forward`, `BridgeArgv` (pure), `RunBridgeCtx`, `RunCtx` on `Exec` and `Fake`, plus the `CtxRunner`/`BridgeRunner` **optional capability** interfaces (the module's `interactiveCtxRunner` precedent — widening `Runner` itself would break every package's test double); `bridge_linux.go` sets `Pdeathsig`, `bridge_other.go` is the named macOS residual; `streamCombined` extracted so the stream and bridge lanes share one drain/kill path; `Fake.Argv` records what ran
- [x] RUN-GREEN: `go test -race ./internal/runner/` → **PASS**; whole module 14 packages ok
- [x] VERIFY: argv has `-N`, `ExitOnForwardFailure=yes`, every base/mux option, `-L 127.0.0.1:l:127.0.0.1:r` per forward, alias last, no `-t`, no remote command
- [x] VERIFY: `git diff --stat` — `cmd/wake.go` empty, `runner.Exec{` non-test count still 11
- [x] EVID + COMMIT + LEDGER

**Done when:** a set of tunnels is one killable `ssh -N`, the batch lane has a context and an exit code, and nothing else moved.

### Task 4 — test harness  (plan T4)

- [x] SETUP: `FakeProvider` — three kinds of five columns each (`fake-capability` → `fake-widget` → `fake-gadget`), the gadget a `Leaf` carrying one action of each of the three kinds plus one `Unavailable`
- [x] RED: the smoke test drives every level through `Provider` + `Host` alone (a `runnerHost` adapter over `runner.Fake` lives in the TEST, so leaf A keeps no in-edge to the registry) + `BuildStub(t)`
- [x] RUN-RED: `go test ./pkg/provider/providertest/...` → **FAIL** observed (`no non-test Go files`; evidence/task04/red.txt)
- [x] GREEN: `providertest/fake.go` (`NewFakeProvider` + `Absent`/`ProbeError`/`Hang` options), `providertest/stub.go` (`BuildStub`, `sync.Once`), `providertest/stubplugin/main.go` (its own `main`, flags `-reply -half-line -sleep -exit-at-once -stderr`)
- [x] RUN-GREEN: `go test ./pkg/provider/...` → **PASS**, providertest 94.2%, `pkg/provider` 100%
- [x] EVID + COMMIT + LEDGER
- [x] **FREEZE:** recorded in `TRACKING.md` §5 — the contract (plan §3.1) is frozen as of the T4 commit
- [x] CHECKPOINT: `gss feature checkpoint` → **PR [#305](https://github.com/sfc-gh-eraigosa/dotfiles/pull/305)**, marked ready; 11/11 CI checks green, `mergeStateStatus: CLEAN`. **`ready-for-merge` is the operator's to apply after review.**

**Done when:** later leaves can be built and tested without herdr. **Leaf A exits — PR 1.**

---

## Phase 2 — the protocol (leaf B · **PR 2 `protocol`**, base PR 1)

### Task 5 — wire + framing  (plan T5)

- [ ] RED: `TestAMalformedLineDoesNotCorruptTheNextReply`, `TestConcurrentCallsNeverCrossDeliverReplies` + round-trip framing
- [ ] RUN-RED: `go test ./pkg/provider/ -run Wire` → expect **FAIL**
- [ ] GREEN: `pkg/provider/wire.go` — JSON-RPC 2.0 envelope, method/param/result types, newline `codec`, pending-call map
- [ ] RUN-GREEN: `go test -race ./pkg/provider/` → **PASS**
- [ ] EVID: the framing test's transcript
- [ ] COMMIT + LEDGER

**Done when:** framing survives a half-written line and ids never cross-deliver.

### Task 6 — `Serve` (plugin side)  (plan T6)

- [ ] RED: `TestAttrsRoundTripToThePluginVerbatim` + `initialize`/`probe`/`children`/`columns` over an in-memory pipe
- [ ] RUN-RED: `go test ./pkg/provider/ -run Serve` → expect **FAIL**
- [ ] GREEN: `serve.go` — `Serve(ctx, Provider, io.Reader, io.Writer)` and the `Host` stub that turns `Host.Exec` into a `host/exec` request carrying the in-flight `callId`
- [ ] RUN-GREEN: `go test -race ./pkg/provider/` → **PASS**
- [ ] EVID: the full JSON transcript
- [ ] COMMIT + LEDGER

**Done when:** a `Serve`d `FakeProvider` answers every method and echoes `attrs` verbatim.

### Task 7 — `Client` + handshake  (plan T7)

- [ ] RED: `TestAProtocolMismatchDisablesThePluginWithBothNumbers`, `TestAPluginThatExitsBeforeInitializeIsReportedNotRetriedForever`, `TestAPluginThatMissesItsDeadlineIsKilledAndReportedAsARow`, `TestASlowHostExecDoesNotCountAgainstThePluginDeadline` (the clock pauses on an outstanding exec)
- [ ] RUN-RED: `go test ./pkg/provider/ -run Client` → expect **FAIL**
- [ ] GREEN: `client.go` — `Dial`/`Client`, handshake, version check, per-call deadline, `host/exec` dispatch to an injected `ExecFunc`
- [ ] RUN-GREEN: `go test -race ./pkg/provider/` → **PASS** (all three failure modes)
- [ ] EVID: each failure mode's rendered reason
- [ ] COMMIT + LEDGER

**Done when:** every protocol failure mode produces a legible reason, not a hang.

### Task 8 — `host/exec` bridge  (plan T8)

- [ ] RED: `TestHostExecLandsOnTheRunnerSeamUnderBatchMode`, `TestHostExecParamsCarryNoRouteOrCredential` (leak sweep over marshalled bytes), `TestHostExecForAnUnknownCallIdIsRefused`, `TestHostExecSeesTheSameResultInProcessAndOverTheWire` (non-zero exit + stderr + stdin)
- [ ] RUN-RED: `go test ./internal/providers/ -run HostExec` → expect **FAIL**
- [ ] GREEN: `internal/providers/host.go` — `callId` → alias → `runner.Runner.RunCtx` under the call's ctx, reply = `ExecResult`; refuse unknown/completed ids with `-32001`
- [ ] RUN-GREEN: `go test -race ./internal/providers/` → **PASS** with `runner.Fake`, no socket
- [ ] VERIFY: two concurrent calls for different hosts each exec on their own alias
- [ ] EVID: the refusal transcript + the leak-sweep assertion
- [ ] COMMIT + LEDGER
- [ ] **FREEZE:** record in `TRACKING.md` §5 that the protocol (plan §3.2) is frozen as of this SHA
- [ ] CHECKPOINT: PR 2 draft → ready

**Done when:** a plugin cannot name a machine, and every exec rides the runner seam. **Leaf B exits — PR 2.**

---

## Phase 3 — registry, config, verbs (leaf C · **PR 3 `registry`**, base PR 2)

### Task 9 — registry  (plan T9)

- [ ] RED: `TestRenderOrderIsFileOrder`, `TestOneFailingPluginNeverStopsTheOthers`, `TestAPluginIsSpawnedOnceAndReused`
- [ ] RUN-RED: `go test ./internal/providers/ -run Registry` → expect **FAIL**
- [ ] GREEN: `registry.go` — ordered entries (builtin + plugin), `Get`, `All`, `Status`, failure isolation
- [ ] RUN-GREEN: `go test ./internal/providers/ -cover` → **PASS**, ≥ 90%
- [ ] EVID: the spawn-count assertion
- [ ] COMMIT + LEDGER

**Done when:** order is declaration order and one bad provider cannot take out a level.

### Task 10 — config loader  (plan T10)

- [ ] RED: `TestMissingProvidersConfigIsTheBuiltinSetNotAnError`, `TestDuplicateProviderNamesAreRefusedNamingBoth`, `TestADisabledProviderIsNeverProbed`, `TestAPluginCanShadowABuiltinByName` (+ empty file, reorder)
- [ ] RUN-RED: `go test ./internal/providers/ -run Config` → expect **FAIL**
- [ ] GREEN: `config.go` (`gopkg.in/yaml.v3` is already required — do not touch `go.mod`); `~` expansion; PATH resolution for a bare `command`
- [ ] RUN-GREEN: `go test ./internal/providers/` → **PASS**; `go mod tidy` is a no-op
- [ ] VERIFY: an absent config writes **nothing** to disk
- [ ] EVID: test output + the empty `go mod tidy` diff
- [ ] COMMIT + LEDGER

**Done when:** onboarding is a file, and a missing file is the built-in set.

### Task 11 — plugin lifecycle  (plan T11)

- [ ] RED: deadline breach → killed + failed row (`timed out after …`); stderr reaches the log; a plugin that dies or is killed mid-session is re-dialed on the next call (once per call, never a loop), then reported; `TestAHungBuiltinExecIsCancelledByItsContext` (F7d)
- [ ] RUN-RED: `go test ./internal/providers/ -run Lifecycle` → expect **FAIL**
- [ ] GREEN: lazy spawn, kill, status, stderr capture through `libs/log`
- [ ] RUN-GREEN: `go test -race ./internal/providers/` → **PASS**, no hang
- [ ] EVID: the failed-capability row
- [ ] COMMIT + LEDGER

**Done when:** a hung plugin cannot hang fleet.

### Task 12 — `providers` verbs  (plan T12)

- [ ] RED: `TestProvidersListDoesNotSpawnWithoutProbe`, `TestProvidersCheckPrintsTheExchangeAndExitsNonZeroOnFailure` (+ `absent` still exits 0)
- [ ] RUN-RED: `go test ./cmd/ -run Providers` → expect **FAIL**
- [ ] GREEN: `cmd/providers.go` — `providers list|check`, hidden `provider serve <name>`; `cmd/provider_registry.go` with an **empty** built-in set for now
- [ ] RUN-GREEN: `go test ./cmd/ -run Providers` → **PASS**
- [ ] EVID: both transcripts
- [ ] COMMIT + LEDGER

- [ ] CHECKPOINT: PR 3 draft → ready (unblocks herdr, cli, and `fleet-connect-k8s` kA)

**Done when:** a plugin author has a debugging surface. **Leaf C exits — PR 3.**

---

## Phase 4 — the herdr provider (leaf D · **PR 4 `herdr`**, base PR 3)

### Task 13 — parsers  (plan T13)

- [ ] SETUP: capture **real** herdr 0.8.2 output into `testdata/{status,status-stopped,sessions,snapshot,truncated}.json` with a provenance header (version + date)
- [ ] RED: `TestParseStatusFromRealHerdrOutput`, `TestTruncatedStatusNeverReportsARunningServer` (+ sessions, snapshot)
- [ ] RUN-RED: `go test ./internal/provider/herdr/` → expect **FAIL**
- [ ] GREEN: `parse.go` — narrow structs, `splitSections`
- [ ] RUN-GREEN: `go test ./internal/provider/herdr/ -cover` → **PASS**
- [ ] EVID: fixture provenance + test output
- [ ] ALLOWLIST (`testdata/*.json`) + COMMIT + LEDGER

**Done when:** every fixture parses or reports a reason; none crashes.

### Task 14 — probe  (plan T14)

- [ ] RED: `TestHerdrProbeCostsOneRoundTrip`, `TestHerdrResolvesABinaryMissingFromTheNonLoginPath`, `TestAbsentHerdrIsARowNotAnOmission`
- [ ] RUN-RED: `go test ./internal/provider/herdr/ -run Probe` → expect **FAIL**
- [ ] GREEN: `script.go`'s `probeScript()` (POSIX sh, expands `$HOME` itself) + `Probe`; resolved **absolute** path into `Attrs["binary"]` (a test asserts no `~`)
- [ ] RUN-GREEN: `go test ./internal/provider/herdr/` → **PASS**, round-trip count exactly 1
- [ ] VERIFY: `make lint-portability` green (the embedded `sh` script)
- [ ] EVID: the recorded argv + the absent row
- [ ] COMMIT + LEDGER

**Done when:** one dial yields version, protocol, server state and session count.

### Task 15 — sessions level  (plan T15)

- [ ] RED: `TestSessionsLevelCostsTwoRoundTripsRegardlessOfSessionCount` (N = 0, 1, 5), `TestAgentCountsComeFromTheFetchedSnapshots`
- [ ] RUN-RED: `go test ./internal/provider/herdr/ -run Sessions` → expect **FAIL**
- [ ] GREEN: `snapshotScript(binary, names)` with every name quoted; `Children(nil)`
- [ ] RUN-GREEN: `go test ./internal/provider/herdr/` → **PASS**, counts hold for all three N
- [ ] EVID: the generated script + round-trip counts
- [ ] COMMIT + LEDGER

**Done when:** cost is two dials for any session count, and a failed snapshot renders `-`.

### Task 16 — agents level  (plan T16)

- [ ] RED: `TestAgentsLevelCostsOneRoundTripAndRowsAreLeaves` (+ empty level keeps its header)
- [ ] RUN-RED: `go test ./internal/provider/herdr/ -run Agents` → expect **FAIL**
- [ ] GREEN: `Children([session])` + `Columns("herdr-agent")`
- [ ] RUN-GREEN: `go test ./internal/provider/herdr/` → **PASS**
- [ ] EVID: rendered rows from a real snapshot
- [ ] COMMIT + LEDGER

**Done when:** agents render with live states and are leaves.

### Task 17 — actions + degraded states  (plan T17)

- [ ] RED: `TestAttachUsesTheLocalBinaryAndTheRemoteAlias`, `TestAHostileSessionNameStaysAnInertArgvElement`, `TestAttachIsRefusedOnAProtocolMismatchWithBothNumbers`, `TestServerStoppedStillListsSessionsAndKeepsAttach`, `TestAttachIsRefusedWithoutALocalHerdr`
- [ ] RUN-RED: `go test ./internal/provider/herdr/ -run Attach` → expect **FAIL**
- [ ] GREEN: `Deps{LocalBinary, LocalStatus}` (injected, cached once per process) + action construction
- [ ] RUN-GREEN: `go test ./internal/provider/herdr/ -cover` → **PASS**, ≥ 90%
- [ ] EVID: the four `Unavailable` strings + the attach argv
- [ ] COMMIT + LEDGER

**Done when:** every degraded state is a row with a reason and no impossible action is offered.

### Task 18 — dual-path equality (keystone)  (plan T18)

- [ ] RED: `TestTheHerdrTreeIsIdenticalInProcessAndOverTheWire` (all three levels + the absent case)
- [ ] RUN-RED: `go test ./cmd/ -run Identical` → expect **FAIL**
- [ ] GREEN: wire the in-process provider and a `provider serve herdr` client behind one `runner.Fake`
- [ ] RUN-GREEN: `go test ./cmd/ -run Identical -v` → **PASS**, diff empty
- [ ] EVID: both renderings + the (empty) diff
- [ ] COMMIT + LEDGER

- [ ] CHECKPOINT: PR 4 draft → ready

**Done when:** the protocol is proven by a real provider, not a paper design. **Leaf D exits — PR 4.**

---

## Phase 5 — the TUI (leaf E · **PR 5 `tui`**, base PR 1)

### Task 19 — nav stack + loads  (plan T19)

- [ ] RED: `TestEnterPushesTheCapabilityLevelKeepingTheLogPane`, `TestEscPopsOneLevelRestoringItsCursor`, `TestASecondRefreshWhileLoadingIsANoOp`, `TestALateLevelLoadForAPoppedViewIsDiscarded`
- [ ] RUN-RED: `go test ./cmd/ -run Nav` → expect **FAIL**
- [ ] GREEN: `cmd/tui_nav.go` — `navFrame`, push/pop/reload, `loadLevel`, nav messages, `navGen`; inject `reg` in `cmd/tui.go` beside `ansPath`
- [ ] RUN-GREEN: `go test -race ./cmd/` → **PASS**, existing model tests unchanged
- [ ] EVID: the generation-drop transcript
- [ ] COMMIT + LEDGER

**Done when:** levels push and pop and a stale reply can never land in the wrong frame.

### Task 20 — ownership + keymap  (plan T20)

- [ ] RED: `TestDrillingIntoABusyHostIsRefused`, `TestABackgroundUpdateContinuesWhileDrilledIn`, `TestKeyHelpCoversEveryBoundNavKeyAtItsLevel`, `TestUpdateKeysAreUnboundInsideALevel`, `TestAProviderKeyRunsTheCursorRowsActionAndShowsInTheHeader`
- [ ] RUN-RED: `go test ./cmd/ -run "Busy|Unbound|KeyHelp"` → expect **FAIL**
- [ ] GREEN: the `routeNav` branch; level-aware `keyHelp` + `headerHints`; `enter` gated by `canStartConfigAction()`
- [ ] RUN-GREEN: `go test ./cmd/` → **PASS**
- [ ] VERIFY: each of `u w v space a p P A F` individually does nothing inside a level
- [ ] EVID: the refusal line + keymap coverage output
- [ ] COMMIT + LEDGER

**Done when:** drill-down claims no row and no dashboard verb fires from inside a level.

### Task 21 — view + golden frames  (plan T21)

- [ ] RED: `TestEscClearsAFilterBeforeItPopsALevel`, `TestNavViewRendersAnUnknownKindWithoutTUIChanges`, new `TestDemoFrames` cases (capability, sessions, agents, absent, plugin-failed)
- [ ] RUN-RED: `go test ./cmd/ -run "DemoFrames|NavView"` → expect **FAIL**
- [ ] GREEN: `cmd/tui_nav_view.go` — breadcrumb, generic table with data-derived widths, level status bar
- [ ] RUN-GREEN: `go test ./cmd/ -run DemoFrames -v` → **PASS**, all frames inside the width guard
- [ ] VERIFY: `FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames` reads correctly in colour
- [ ] EVID: the five new frames
- [ ] COMMIT + LEDGER

**Done when:** every level renders byte-stably and an unknown kind needs no TUI change.

### Task 22 — provider streams  (plan T22)

- [ ] RED: `TestAProviderStreamNeverTouchesTheUpdateEngine` (+ a stream that ends immediately)
- [ ] RUN-RED: `go test ./cmd/ -run ProviderStream` → expect **FAIL**
- [ ] GREEN: `beginProviderStream` over `RunStreamCtx`, own message types + own map, lines via `appendLog`
- [ ] RUN-GREEN: `go test -race ./cmd/` → **PASS**; `running`/`updating` unchanged
- [ ] EVID: the log-pane frame + the engine-state assertion
- [ ] COMMIT + LEDGER

- [ ] CHECKPOINT: PR 5 draft → ready

**Done when:** a provider stream shares the log pane and nothing else. **Leaf E exits — PR 5.**

---

## Phase 6 — the CLI (leaf F · **PR 6 `cli`**, base PR 3)

### Task 23 — `fleet ls`  (plan T23)

- [ ] RED: `TestLsJSONShapeIsStable` (golden), `TestLsNodesIsNeverNull` (zero-node level), `TestLsRendersADeepLevelAndNamesAnUnknownSegment`
- [ ] RUN-RED: `go test ./cmd/ -run Ls` → expect **FAIL**
- [ ] GREEN: `cmd/ls.go`
- [ ] RUN-GREEN: `go test ./cmd/ -run Ls` → **PASS**, golden (generated by marshalling the frozen structs — `"key":"c"`, `unavailable`, `handoff`/`stream`/`tunnel`) matches byte-for-byte
- [ ] EVID: the golden JSON + the human table
- [ ] ALLOWLIST (golden fixture) + COMMIT + LEDGER

**Done when:** the tree is scriptable and the JSON is a contract.

### Task 24 — `fleet connect`  (plan T24)

- [ ] RED: `TestConnectDryRunPrintsTheExactArgv` (hostile session name), `TestConnectRefusesAnUnavailableActionWithItsReason` (+ unknown `--action` key), `TestConnectStreamsToStdoutWithoutATty`, the F2d case (argv host = the `<host>` argument)
- [ ] RUN-RED: `go test ./cmd/ -run Connect` → expect **FAIL**
- [ ] GREEN: `cmd/connect.go` (handoff + stream branches; the tunnel branch lands in Task 28)
- [ ] RUN-GREEN: `go test ./cmd/ -run Connect` → **PASS**
- [ ] VERIFY: no credential and no unquoted provider value in the printed argv
- [ ] EVID: the dry-run argv + the refusal exit code
- [ ] COMMIT + LEDGER

- [ ] CHECKPOINT: PR 6 draft → ready

**Done when:** connecting works from the CLI and refuses legibly. **Leaf F exits — PR 6.**

---

## Phase 7 — bridges (leaf H · **PR 7 `bridges`**, base PR 6, restack after PR 5)

### Task 25 — bridge manager  (plan T25)

- [ ] RED: `TestOneProcessPerHostRestartedPerChange`, `TestBridgesOnTwoHostsAreIndependent`, `TestABusyLocalPortIsAllocatedAroundAndReported`, `TestAnExplicitBusyPortFailsWithSshsReason`, `TestASelfExitedBridgeIsFailedWithItsLastStderrLine`, `TestClosingTheManagerStopsEveryBridge`, `TestAKeeperRunsUnderTheBridgeContextAndStopsWithIt`
- [ ] RUN-RED: `go test ./internal/bridge/` → expect **FAIL** (package does not exist)
- [ ] GREEN: `internal/bridge/{manager,set,ports,keeper}.go` — `Manager` keyed by alias, `Set`, `Forward` (+ optional keeper process via `RunStreamCtx`), local-port policy (0 = prefer remote number, else allocate + note), injected `listen`/`dial`, `Status()`, `Close()` (idempotent)
- [ ] RUN-GREEN: `go test -race ./internal/bridge/ -cover` → **PASS**, ≥ 90%
- [ ] VERIFY: a recording runner shows one process per alias at any time; no test binds a real port
- [ ] EVID: the add/add/remove transcript + the allocation note
- [ ] ALLOWLIST + COMMIT + LEDGER

**Done when:** N ports on M hosts are M processes, and `Close()` leaves none.

### Task 26 — ports provider  (plan T26)

- [ ] SETUP: capture **real** `ss -H -ltnp` from `<spark>` and `<pi>` into `internal/provider/ports/testdata/` with a provenance header (host kind, `ss --version`, date)
- [ ] RED: `TestPortsLevelSplitsLoopbackReachableFromLanOnlyBinds`, `TestPortsProbeCostsOneRoundTrip`, `TestPortLabelsComeFromTheTableThenTheProcess`, `TestMissingSsIsARowNamingTheTool` (+ empty level keeps its header)
- [ ] RUN-RED: `go test ./internal/provider/ports/` → expect **FAIL**
- [ ] GREEN: `ports.go` (POSIX-sh wrapper, `Probe`, `Children` = the rows, `Columns`), `parse.go`, `labels.go`; loopback-reachable → `t` `Tunnel{RemotePort, 0, guess}`; LAN-only → `Unavailable` naming the address
- [ ] RUN-GREEN: `go test ./internal/provider/ports/ -cover` → **PASS**, ≥ 90%, round-trip count 1
- [ ] VERIFY: `make lint-portability` green
- [ ] EVID: fixture provenance + rendered rows for both hosts
- [ ] ALLOWLIST (`testdata/*.txt`) + COMMIT + LEDGER

**Done when:** every listening port is a row, and only loopback-reachable ones offer a tunnel.

### Task 27 — TUI bridges  (plan T27)

- [ ] RED: `TestTToggleABridgeWithoutTouchingTheUpdateEngine`, `TestReloadKeepsTheBridgeMarkerOnItsPort`, `TestTStopsOnlyThisHostsBridges`, `TestBridgesSurviveEscAndShowOnTheDashboard` (golden), `TestQuitTearsDownEveryBridgeBeforeExit` (+ force-quit path)
- [ ] RUN-RED: `go test ./cmd/ -run Bridge` → expect **FAIL**
- [ ] GREEN: `cmd/tui_bridge.go` — `t`/`T`, `bridgeUpMsg`/`bridgeDoneMsg`, `⇄` gutter marker keyed on `(alias, remotePort)`, level bridge line, `⇄N` NOTE at level 0, `bridges.Close()` before `tea.Quit`; `keyHelp` rows for `t`/`T` at level ≥ 1
- [ ] RUN-GREEN: `go test -race ./cmd/` → **PASS**; `running`/`updating` untouched; no existing test or frame changed
- [ ] VERIFY: `go test ./cmd/ -run DemoFrames -v` → the ports frame and the `⇄N` dashboard frame inside the width guard
- [ ] EVID: the toggle transcript + the two new frames
- [ ] COMMIT + LEDGER

**Done when:** bridges toggle from any level, survive `esc`, and cannot survive `q`.

### Task 28 — `fleet bridge` + `connect` on a tunnel  (plan T28)

- [ ] RED: `TestBridgeVerbRunsOnePerHostAndPrintsTheTable`, `TestBridgeDryRunStartsNothing`, `TestOneFailedBridgeLeavesTheOthersUpAndExitsNonZero`, `TestAMalformedBridgeSpecIsRefusedBeforeStart` (+ `connect <host> ports 3080` = one-entry bridge)
- [ ] RUN-RED: `go test ./cmd/ -run "BridgeVerb|BridgeSpec|BridgeDryRun|FailedBridge"` → expect **FAIL**
- [ ] GREEN: `cmd/bridge.go` (spec parsing, the plan §3.5 table with pids, re-print on state change, hold until SIGINT/SIGTERM, `Close()`, exit code); tunnel branch of `cmd/connect.go`
- [ ] RUN-GREEN: `go test ./cmd/ -run Bridge` → **PASS**
- [ ] VERIFY: three specs on two aliases → exactly two processes in the recording runner
- [ ] EVID: the table + both exit codes
- [ ] COMMIT + LEDGER

- [ ] CHECKPOINT: PR 7 draft → ready (after `gss feature restack` onto the merged PR 5 + 6 base)

**Done when:** a script can open N bridges on M hosts with one command and Ctrl-C leaves none. **Leaf H exits — PR 7.**

---

## Phase 8 — integration (leaf G · **PR 8 `integrate`**, base PR 7, after PR 4)

### Task 29 — register, document, prove live  (plan T29)

- [ ] GREEN: register herdr and ports as built-ins in `cmd/provider_registry.go`
- [ ] RUN-GREEN: `go test -race ./... && go test ./... -cover` → **PASS**, new packages ≥ 90%
- [ ] VERIFY: `./scripts/test.sh` green; `make lint-shell && make lint-portability` green
- [ ] DOCS: `sdk/fleet/AGENTS.md` — the 18 "Drill-down & providers" invariants + `ls`/`connect`/`bridge`/`providers` rows in Commands
- [ ] DOCS: `sdk/fleet/README.md` — drill-down tour, "bridge a port" (`t`, `T`, `fleet bridge`, the lifetime rule) **and** "write a provider plugin" (protocol table, ~30-line stub, `providers check`); `sdk/README.md` fleet section demo — **real pasted output**
- [ ] LIVE 1: `fleet providers check herdr --host <spark>` → capture the raw handshake + probe + `host/exec` exchange
- [ ] LIVE 2: three-level drill-down + real attach; on exit the dashboard returns and the row re-polls
- [ ] LIVE 3: configure herdr as an external plugin (`command: fleet, args: [provider, serve, herdr]`) → the tree is identical
- [ ] LIVE 4: a host without herdr → the absent row naming the paths tried
- [ ] LIVE 5: `fleet ls <spark> herdr default --json` + `fleet connect <spark> herdr default --dry-run`
- [ ] LIVE 6: a deliberately broken plugin → its row explains itself; the others still render
- [ ] LIVE 7: `t` on 3080 and 11434 in `<spark>`'s ports level → `curl -sI http://127.0.0.1:3080` answers; `q` → `ss -ltn` shows neither local port
- [ ] LIVE 8: `fleet bridge <spark>:3080 <nano>:11434` → the table with pids; Ctrl-C → the stop line; `ss -ltn` clean
- [ ] CHECKLIST: the eleven manual steps in plan §6 signed off
- [ ] EVID: everything above into `evidence/live/`, hostnames sanitised
- [ ] COMMIT + LEDGER; advance `docs/mbo/index.md` state to `in-review`

**Done when:** the stop condition in `TRACKING.md` §3 is fully ticked.
