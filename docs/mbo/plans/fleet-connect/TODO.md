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

- [ ] `cd sdk/fleet && go version` → ≥ 1.26.3
- [ ] `cd sdk/fleet && go test -race ./... && gofmt -l . && go vet ./...` → green, empty, clean
- [ ] `./scripts/test.sh` → green (fleet floor 60)
- [ ] `make lint-shell && make lint-portability` → green
- [ ] `git rev-parse --show-toplevel && git branch --show-current` → this worktree, not `~/git/dotfiles`
- [ ] `fleet status <spark>` and `ssh <spark> '~/.local/bin/herdr status'` → a live herdr host exists
- [ ] `command -v herdr || ls ~/.local/bin/herdr` → a local client exists (needed for attach)
- [ ] `mkdir -p docs/mbo/plans/fleet-connect/evidence` then `git status --short -- docs/mbo/plans/fleet-connect/evidence` → tracked (else `git check-ignore -v` and add a narrow `!`-rule)

---

## Phase 1 — the contract (leaf A)

### Task 1 — `pkg/provider` types  (plan T1)

- [ ] RED: `TestEveryContractTypeRoundTripsThroughJSON`, `TestAnActionMustCarryExactlyOneOfHandoffOrStream`, `TestAShortCellSliceRendersBlanksNotAPanic`
- [ ] RUN-RED: `go test ./pkg/provider/` → expect **FAIL** (package does not exist)
- [ ] GREEN: `pkg/provider/provider.go` — `Node`, `Action`, `Handoff`, `HandoffKind`, `Stream`, `Provider`, `Host`, `ErrAbsent`, `ErrNoSuchPath`, `Validate`
- [ ] RUN-GREEN: `go test ./pkg/provider/ -cover` → **PASS**, ≥ 90%
- [ ] VERIFY: `go list -deps ./pkg/provider` → stdlib only, no third party
- [ ] EVID + ALLOWLIST + COMMIT + LEDGER

**Done when:** the three tests pass and the public package is proven stdlib-only.

### Task 2 — runner handoffs  (plan T2)

- [ ] RED: `TestEveryHandoffCarriesTheMuxOptions`, `TestLocalHandoffNeverInvokesAShell`, `TestRemoteHandoffQuotesEveryProviderSuppliedValue` + empty-input error cases
- [ ] RUN-RED: `go test ./internal/runner/` → expect **FAIL**
- [ ] GREEN: `internal/runner/handoff.go` — `HandoffArgv` (pure), `Command`, `Quote`; promote `interactiveArgs` to a package func; `cmd.shQuote` becomes an alias of `runner.Quote`
- [ ] RUN-GREEN: `go test ./internal/runner/ ./cmd/` → **PASS** (existing cmd tests still green)
- [ ] VERIFY: remote argv has `ssh -t` + every `MuxArgs()` option and **no** `BatchMode`; local argv passes a `$(…)` element verbatim with no `sh -c`
- [ ] EVID + COMMIT + LEDGER

**Done when:** both handoff kinds are asserted by argv and the module still builds.

### Task 3 — cancellable streams  (plan T3)

- [ ] RED: `TestAFollowedStreamStopsWhenItsContextIsCancelled` (also cancel-before-first-line)
- [ ] RUN-RED: `go test ./internal/runner/ -run Cancel` → expect **FAIL**
- [ ] GREEN: `RunStreamCtx` on the `Runner` interface, `Exec` (via `exec.CommandContext`) and `Fake`; `RunStream` delegates
- [ ] RUN-GREEN: `go test -race ./internal/runner/` → **PASS**, no goroutine leak
- [ ] VERIFY: `git diff --stat` shows the ten `runner.Exec{}` sites and `cmd/wake.go` untouched
- [ ] EVID + COMMIT + LEDGER

**Done when:** a followed stream is killable and nothing else moved.

### Task 4 — test harness  (plan T4)

- [ ] SETUP: decide `FakeProvider`'s five-column kind, three levels, one leaf, one action of each type
- [ ] RED: a smoke test drilling `FakeProvider` through the registry seam's interface
- [ ] RUN-RED: `go test ./pkg/provider/providertest/` → expect **FAIL**
- [ ] GREEN: `providertest/fake.go` — `FakeProvider` + `StubPlugin` (a tiny `main` the protocol tests compile)
- [ ] RUN-GREEN: `go test ./pkg/provider/...` → **PASS**
- [ ] EVID + COMMIT + LEDGER
- [ ] **FREEZE:** record in `TRACKING.md` §5 that the contract (plan §3.1) is frozen as of this SHA

**Done when:** later leaves can be built and tested without herdr. **Leaf A exits.**

---

## Phase 2 — the protocol (leaf B)

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

- [ ] RED: `TestAProtocolMismatchDisablesThePluginWithBothNumbers`, `TestAPluginThatExitsBeforeInitializeIsReportedNotRetriedForever`, `TestAPluginThatMissesItsDeadlineIsKilledAndReportedAsARow`
- [ ] RUN-RED: `go test ./pkg/provider/ -run Client` → expect **FAIL**
- [ ] GREEN: `client.go` — `Dial`/`Client`, handshake, version check, per-call deadline, `host/exec` dispatch to an injected `ExecFunc`
- [ ] RUN-GREEN: `go test -race ./pkg/provider/` → **PASS** (all three failure modes)
- [ ] EVID: each failure mode's rendered reason
- [ ] COMMIT + LEDGER

**Done when:** every protocol failure mode produces a legible reason, not a hang.

### Task 8 — `host/exec` bridge  (plan T8)

- [ ] RED: `TestHostExecLandsOnTheRunnerSeamUnderBatchMode`, `TestHostExecParamsCarryNoRouteOrCredential` (leak sweep over marshalled bytes), `TestHostExecForAnUnknownCallIdIsRefused`
- [ ] RUN-RED: `go test ./internal/providers/ -run HostExec` → expect **FAIL**
- [ ] GREEN: `internal/providers/host.go` — `callId` → alias → `runner.Runner.Run`; refuse unknown/completed ids with `-32001`
- [ ] RUN-GREEN: `go test -race ./internal/providers/` → **PASS** with `runner.Fake`, no socket
- [ ] VERIFY: two concurrent calls for different hosts each exec on their own alias
- [ ] EVID: the refusal transcript + the leak-sweep assertion
- [ ] COMMIT + LEDGER
- [ ] **FREEZE:** record in `TRACKING.md` §5 that the protocol (plan §3.2) is frozen as of this SHA

**Done when:** a plugin cannot name a machine, and every exec rides the runner seam. **Leaf B exits.**

---

## Phase 3 — registry, config, verbs (leaf C)

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
- [ ] GREEN: `config.go` + `gopkg.in/yaml.v3` in `go.mod`; `~` expansion; PATH resolution for a bare `command`
- [ ] RUN-GREEN: `go test ./internal/providers/` → **PASS**; `go mod tidy` leaves the graph clean
- [ ] VERIFY: an absent config writes **nothing** to disk
- [ ] EVID: test output + the `go.mod` diff
- [ ] COMMIT + LEDGER

**Done when:** onboarding is a file, and a missing file is the built-in set.

### Task 11 — plugin lifecycle  (plan T11)

- [ ] RED: deadline breach → killed + failed row; stderr reaches the log; a plugin that dies mid-session is re-dialed once, then reported
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

**Done when:** a plugin author has a debugging surface.

---

## Phase 4 — the herdr provider (leaf D)

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
- [ ] GREEN: `script.go`'s `probeScript()` (POSIX sh) + `Probe`; resolved path into `Attrs["binary"]`
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

**Done when:** the protocol is proven by a real provider, not a paper design. **Leaf D exits.**

---

## Phase 5 — the TUI (leaf E)

### Task 19 — nav stack + loads  (plan T19)

- [ ] RED: `TestEnterPushesTheCapabilityLevelKeepingTheLogPane`, `TestEscPopsOneLevelRestoringItsCursor`, `TestASecondRefreshWhileLoadingIsANoOp`, `TestALateLevelLoadForAPoppedViewIsDiscarded`
- [ ] RUN-RED: `go test ./cmd/ -run Nav` → expect **FAIL**
- [ ] GREEN: `cmd/tui_nav.go` — `navFrame`, push/pop/reload, `loadLevel`, nav messages, `navGen`; inject `reg` in `cmd/tui.go` beside `ansPath`
- [ ] RUN-GREEN: `go test -race ./cmd/` → **PASS**, existing model tests unchanged
- [ ] EVID: the generation-drop transcript
- [ ] COMMIT + LEDGER

**Done when:** levels push and pop and a stale reply can never land in the wrong frame.

### Task 20 — ownership + keymap  (plan T20)

- [ ] RED: `TestDrillingIntoABusyHostIsRefused`, `TestABackgroundUpdateContinuesWhileDrilledIn`, `TestKeyHelpCoversEveryBoundNavKeyAtItsLevel`, `TestUpdateKeysAreUnboundInsideALevel`
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

**Done when:** a provider stream shares the log pane and nothing else. **Leaf E exits.**

---

## Phase 6 — the CLI (leaf F)

### Task 23 — `fleet ls`  (plan T23)

- [ ] RED: `TestLsJSONShapeIsStable` (golden), `TestLsNodesIsNeverNull` (zero-node level), `TestLsRendersADeepLevelAndNamesAnUnknownSegment`
- [ ] RUN-RED: `go test ./cmd/ -run Ls` → expect **FAIL**
- [ ] GREEN: `cmd/ls.go`
- [ ] RUN-GREEN: `go test ./cmd/ -run Ls` → **PASS**, golden matches byte-for-byte
- [ ] EVID: the golden JSON + the human table
- [ ] ALLOWLIST (golden fixture) + COMMIT + LEDGER

**Done when:** the tree is scriptable and the JSON is a contract.

### Task 24 — `fleet connect`  (plan T24)

- [ ] RED: `TestConnectDryRunPrintsTheExactArgv` (hostile session name), `TestConnectRefusesAnUnavailableActionWithItsReason` (+ unknown `--action` key)
- [ ] RUN-RED: `go test ./cmd/ -run Connect` → expect **FAIL**
- [ ] GREEN: `cmd/connect.go`
- [ ] RUN-GREEN: `go test ./cmd/ -run Connect` → **PASS**
- [ ] VERIFY: no credential and no unquoted provider value in the printed argv
- [ ] EVID: the dry-run argv + the refusal exit code
- [ ] COMMIT + LEDGER

**Done when:** connecting works from the CLI and refuses legibly. **Leaf F exits.**

---

## Phase 7 — integration (leaf G)

### Task 25 — register, document, prove live  (plan T25)

- [ ] GREEN: register herdr as a built-in in `cmd/provider_registry.go`
- [ ] RUN-GREEN: `go test -race ./... && go test ./... -cover` → **PASS**, new packages ≥ 90%
- [ ] VERIFY: `./scripts/test.sh` green; `make lint-shell && make lint-portability` green
- [ ] DOCS: `sdk/fleet/AGENTS.md` — the 15 "Drill-down & providers" invariants + `ls`/`connect`/`providers` rows in Commands
- [ ] DOCS: `sdk/fleet/README.md` — drill-down tour **and** "write a provider plugin" (protocol table, ~30-line stub, `providers check`); `sdk/README.md` fleet section demo — **real pasted output**
- [ ] LIVE 1: `fleet providers check herdr --host <spark>` → capture the raw handshake + probe + `host/exec` exchange
- [ ] LIVE 2: three-level drill-down + real attach; on exit the dashboard returns and the row re-polls
- [ ] LIVE 3: configure herdr as an external plugin (`command: fleet, args: [provider, serve, herdr]`) → the tree is identical
- [ ] LIVE 4: a host without herdr → the absent row naming the paths tried
- [ ] LIVE 5: `fleet ls <spark> herdr default --json` + `fleet connect <spark> herdr default --dry-run`
- [ ] LIVE 6: a deliberately broken plugin → its row explains itself; the others still render
- [ ] CHECKLIST: the eight manual steps in plan §6 signed off
- [ ] EVID: everything above into `evidence/live/`, hostnames sanitised
- [ ] COMMIT + LEDGER; advance `docs/mbo/index.md` state to `in-review`

**Done when:** the stop condition in `TRACKING.md` §3 is fully ticked.
