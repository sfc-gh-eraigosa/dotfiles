# fleet-connect — live state ledger

- **Slug:** fleet-connect
- **Started:** 2026-09-02 (planning); build not started
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../fleet-connect.md`](../fleet-connect.md) · spec
  [`../../specs/fleet-connect.md`](../../specs/fleet-connect.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`.
> **Evidence** = the exact command run plus its real result. A row is `done` only with a commit
> SHA **and** evidence. Never write a result you did not observe.

## 0. Worker registry

| Leaf/worker | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design (these artifacts) | `fleet-connect/edward-raigosa/design` | `feature/fleet-connect/edward-raigosa/design` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-connect/edward-raigosa/design` | [#267](https://github.com/sfc-gh-eraigosa/dotfiles/pull/267) | draft |
| PR 1 contract (A, T1–T4) | `fleet-connect/<user>/contract` | `feature/fleet-connect/<user>/contract` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/fleet-connect/<user>/contract` | [#305](https://github.com/sfc-gh-eraigosa/dotfiles/pull/305) | **ready for review** (11/11 CI green, `CLEAN`) — base `main`, **blocking** |
| PR 2 protocol (B, T5–T8) | *not created yet* | `…/protocol` | | | base PR 1 — **blocking** |
| PR 3 registry (C, T9–T12) | *not created yet* | `…/registry` | | | base PR 2 — blocking for 4, 6, k8s |
| PR 4 herdr (D, T13–T18) | *not created yet* | `…/herdr` | | | base PR 3 |
| PR 5 tui (E, T19–T22) | *not created yet* | `…/tui` | | | base PR 1 |
| PR 6 cli (F, T23–T24) | *not created yet* | `…/cli` | | | base PR 3 |
| PR 7 bridges (H, T25–T28) | *not created yet* | `…/bridges` | | | base PR 6; restack after PR 5 |
| PR 8 integrate (G, T29) | *not created yet* | `…/integrate` | | | base PR 7; after PR 4 |

Captured verbatim from `gss feature worker add --json` (feature `fleet-connect`; commands in plan
§6.1) when execution starts. One sub-issue per PR under #266.

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T1 `pkg/provider` types | done | (this commit) | `go test ./pkg/provider/ -cover` → ok, 100.0%; `go list -deps` third-party count 0; gofmt empty; vet ok; `-race` ok (evidence/task01/{red,green}.txt) | leaf A — contract refinement: a `Tunnel` action must use `TunnelKey` (`t`), the one reserved key a provider declares |
| T2 runner handoffs | done | (this commit) | `go test -race ./internal/runner/ ./internal/updexec/ ./cmd/` → ok ×3; gofmt empty; vet ok; `runner.Exec{` non-test count 11; diffstat: `tui_cmds.go` +1/-1, `runner.go` +1/-4, `script.go` +5/-5 (evidence/task02/{red,green}.txt) | leaf A |
| T3 runner bridges + `RunCtx` (`BridgeArgv`, `RunBridgeCtx`, `RunCtx`) | done | (this commit) | `go test -race ./internal/runner/` → ok 4.4s; 10 named tests PASS incl. the pre-existing `TestRunStreamCtxKillsTheChildOnDeadline`; module 14 pkgs ok; gofmt empty; vet ok; `runner.Exec{` count 11; `cmd/wake.go` diff empty (evidence/task03/{setup,red,green}.txt) | leaf A — plan said "on the interface"; built as **optional capability interfaces** `CtxRunner`/`BridgeRunner` instead, per the module's `interactiveCtxRunner` precedent: widening `runner.Runner` would break ~8 test doubles across `cmd` and `updexec` for no gain |
| T4 test harness (FakeProvider, StubPlugin) | done | (this commit) | `go test ./pkg/provider/... -cover` → provider 100.0%, providertest 94.2%; 7 named tests PASS; `go list -deps ./pkg/provider/providertest` shows only `pkg/provider`; module 15 pkgs ok under `-race`; gofmt empty; vet ok (evidence/task04/{red,green}.txt) | leaf A — **contract frozen at this SHA** |
| T5 wire + framing | todo | | | leaf B |
| T6 `Serve` (plugin side) | todo | | | leaf B |
| T7 `Client` + handshake | todo | | | leaf B |
| T8 `host/exec` bridge | todo | | | leaf B — **protocol freezes here** |
| T9 registry | todo | | | leaf C |
| T10 config loader | todo | | | leaf C — `yaml.v3` already required; `go mod tidy` must be a no-op |
| T11 plugin lifecycle | todo | | | leaf C |
| T12 `providers list\|check` + `provider serve` | todo | | | leaf C |
| T13 herdr parsers | todo | | | leaf D — real fixtures |
| T14 herdr probe | todo | | | leaf D |
| T15 herdr sessions level | todo | | | leaf D |
| T16 herdr agents level | todo | | | leaf D |
| T17 herdr actions + degraded states | todo | | | leaf D |
| T18 dual-path equality | todo | | | leaf D — **keystone** |
| T19 nav stack + loads | todo | | | leaf E |
| T20 ownership + keymap | todo | | | leaf E |
| T21 nav view + golden frames | todo | | | leaf E |
| T22 provider streams | todo | | | leaf E |
| T23 `fleet ls` | todo | | | leaf F |
| T24 `fleet connect` | todo | | | leaf F |
| T25 bridge manager (`internal/bridge`) | todo | | | leaf H — one `ssh -N` per host; no real port in tests |
| T26 ports provider | todo | | | leaf H — real `ss` fixtures |
| T27 TUI bridges (`t`/`T`, `⇄`, quit teardown) | todo | | | leaf H — after E |
| T28 `fleet bridge` + `connect` on a tunnel | todo | | | leaf H — after F |
| T29 register + docs + live | todo | | | leaf G |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 contract | [x] T1 (100% cov) | — | JSON round-trip, validation (one of three kinds, port ranges, no host field), short cells; `TunnelKey` |
| F2 handoff execution | [x] T2 · [ ] T24 | [ ] live dry-run argv | mux + `-t`; local argv no shell; alias stamped by fleet |
| F3 cancellable streams | [x] exists (`runner_ctx_test.go`, #270) | — | needed by k8s `logs -f`; nothing to build |
| F4 protocol handshake | [ ] T7 | [ ] `providers check` transcript | mismatch + immediate exit |
| F5 protocol methods | [ ] T5, T6 | [ ] `providers check` transcript | framing, id correlation, attrs echo |
| F6 `host/exec` callback | [ ] T8 | [ ] transcript shows the exec | leak sweep + `-32001`; same `ExecResult` both paths (F6c) |
| F7 plugin lifecycle | [ ] T7, T11 | [ ] broken-plugin row | deadline = plugin time only (F7a), kill, re-dial, isolation, reuse; built-in ctx cancel (F7d) |
| F8 configuration | [ ] T10 | [ ] external-plugin stanza | missing = builtins; shadow; order |
| F9 `providers list\|check` | [ ] T12 | [ ] both transcripts | no spawn without `--probe` |
| F10 dogfooding | [ ] T18 | [ ] identical trees live | the framework's keystone |
| F11 herdr probe | [ ] T14 | [ ] live capability row | one round trip; absent row |
| F12 herdr sessions | [ ] T15 | [ ] live session list | two round trips for any N |
| F13 herdr agents | [ ] T16 | [ ] live agent states | one round trip; leaves |
| F14 actions + degraded | [ ] T17 | [ ] live attach + a refusal | mismatch names both numbers |
| F15 push views | [ ] T19, T21 | [ ] three-level walk | breadcrumb; log pane kept |
| F16 async loads | [ ] T19 | — | generation drop |
| F17 ownership | [ ] T20 | — | refusal on a busy host |
| F18 level keymap | [ ] T20 | [ ] header strip screenshot | `keyHelp` is the one source |
| F19 provider streams | [ ] T22 | — | engine isolation |
| F20 `fleet ls` | [ ] T23 | [ ] live JSON | never `null` |
| F21 `fleet connect` | [ ] T24, T28 | [ ] live dry-run + refusal | exact argv; stream to stdout; tunnel = one-entry bridge |
| F22 bridge execution | [x] T3 | [ ] live `bridge --dry-run` argv | `-N`, `ExitOnForwardFailure`, loopback both sides, `Pdeathsig` on Linux |
| F23 bridge manager | [ ] T25 | [ ] live add/add/remove | one process per host; port policy; `Close()` |
| F24 ports provider | [ ] T26 | [ ] live `fleet ls <spark> ports` | one round trip; bind rules; labels |
| F25 TUI bridges | [ ] T27 | [ ] live toggle + `q` teardown | `⇄` marker; `⇄N` NOTE; survives `esc` |
| F26 `fleet bridge` | [ ] T28 | [ ] live two-host table | one process per alias; exit code on failure |

## 3. Validation done-when — the stop condition

- [ ] Every `TODO.md` box ticked.
- [ ] Every §1 row `done` with a commit SHA **and** observed evidence.
- [ ] Every §2 feature has at least one ticked automated proof (and its live proof where listed).
- [ ] `pkg/provider`, `internal/providers`, `internal/provider/herdr`, `internal/provider/ports`,
      `internal/bridge` each ≥ 90% coverage.
- [ ] `cd sdk/fleet && go test -race ./...` green; `gofmt -l .` empty; `go vet ./...` clean.
- [ ] `./scripts/test.sh` green (fleet floor 60 not breached).
- [ ] `make lint-shell && make lint-portability` green (the remote `sh` scripts).
- [ ] No existing test or golden frame modified (`git diff` on `cmd/*_test.go` shows additions only).
- [ ] `pkg/provider` proven stdlib-only (`go list -deps`).
- [ ] The eleven `runner.Exec{}` sites and `cmd/wake.go`'s type assertion untouched.
- [ ] No bridge test binds a real port (`listen`/`dial` injected everywhere).
- [ ] Live gate 1: `fleet providers check herdr --host <spark>` raw exchange captured.
- [ ] Live gate 2: three-level drill-down + real attach + dashboard returns with the row re-polled.
- [ ] Live gate 3: the same tree via `fleet provider serve herdr` as an external plugin, identical.
- [ ] Live gate 4: a host without herdr renders the absent row naming the paths tried.
- [ ] Live gate 5: `fleet ls <spark> herdr default --json` and `connect … --dry-run` captured.
- [ ] Live gate 6: a deliberately broken plugin explains itself; other providers still render.
- [ ] Live gate 7: two bridges toggled on `<spark>`, `curl` through one, `q`, `ss -ltn` clean.
- [ ] Live gate 8: `fleet bridge <spark>:3080 <nano>:11434` table, Ctrl-C, `ss -ltn` clean.
- [ ] The eleven-step manual checklist in plan §6 signed off.
- [ ] `sdk/fleet/AGENTS.md` carries the 18 drill-down/provider/bridge invariants; READMEs carry
      **real** pasted demos.
- [ ] `docs/mbo/index.md` row advanced to `in-review`.

## 4. Blockers & escalations

Failing command + its **real** output. Contract defects go here and get escalated — never
silently patched.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-02 | planning | Probed all four fleet hosts read-only (runtimes, sessions, ports) and verified herdr 0.8.2's CLI surface, including the non-login PATH gotcha. Brainstormed shape, scope cut (framework + herdr now, k8s next), and navigation (push views + breadcrumb). Operator then asked for tools as **plugins over a local, MCP-like RPC** — design reworked around a versioned JSON-RPC protocol with a `host/exec` callback. Wrote design, spec (21 features, per-feature criteria), plan (25 tasks, DAG) and this trio. Found and folded in one protocol correction: `host/exec` takes a `callId`, not an alias. Anchored the objective: design issue #266 (kept open as the build-tracking parent) and draft design PR #267 on the `fleet-connect` gss feature. No code written. |
| 2026-09-05 | design review + amendment | `/code-review` on #267 returned ten confirmed findings. The operator added one feature: N-port ssh bridges per host, started/stopped/shown across hosts, **never outliving fleet** (design §3.4, option A chosen: one `ssh -N` per host carrying every `-L`). Folded in: a third action kind `Tunnel`; no host field on any action payload (finding 1 — fleet stamps the alias); `internal/bridge`; the `ports` provider; `t`/`T` keys; `fleet bridge`; leaf H (T25–T28); T3 rewritten around `BridgeArgv`/`RunBridgeCtx` because `RunStreamCtx` already exists (finding 7); 29 tasks, 26 features. Remaining findings addressed in the follow-up commit. No code written. |
| 2026-09-06 | build T4 + FREEZE (PR 1 `contract`) | `providertest` ships `FakeProvider` (three five-column kinds, a leaf carrying one action of each kind plus one `Unavailable`) scriptable into absent / probe-error / hang, and `BuildStub` + `stubplugin`. **Two deviations, recorded:** (1) the `runner.Fake`-backed `Host` adapter lives in the test file, not in `providertest`, so the public harness depends on `pkg/provider` alone and stays importable by a plugin author — the production adapter is leaf C's `internal/providers/host.go`; (2) `StubPlugin` is protocol-AGNOSTIC (canned `-reply`, plus `-sleep`/`-exit-at-once`/`-half-line`/`-stderr`), because leaf A owns no wire format — leaf B supplies the JSON and needs no change to the stub. Coverage gate found the harness had run ahead of its tests: the CLI's no-probe entry, the deep-path `ErrNoSuchPath`, the stale-attrs guard and the per-kind columns are now pinned (82.6% → 94.2%). **THE CONTRACT (plan §3.1) IS FROZEN as of this commit.** |
| 2026-09-06 | build T2–T3 (PR 1 `contract`) | T2: `HandoffArgv`/`Command`/`InteractiveArgs` + `Quote` moved into `runner` with `updexec.ShQuote`/`cmd.shQuote` as aliases (pinned by a same-function test). T3: the bridge lane (`Forward`, `BridgeArgv`, `RunBridgeCtx`, `Pdeathsig` on Linux) and `RunCtx`. **Plan deviation, recorded:** T3's "on the interface" became `CtxRunner`/`BridgeRunner` optional capabilities — `runner.Runner` has ~8 test doubles in `cmd`/`updexec`, and `updexec.interactiveCtxRunner` is the module's own precedent for exactly this. `streamCombined` extracted from `RunStreamCtx` so the two streaming lanes share one drain/kill path. |
| 2026-09-06 | build T1 (PR 1 `contract`) | Worker `fleet-connect/<user>/contract` created off main. T1 RED observed (package absent), GREEN at 100% coverage, stdlib-only. **Contract refinement found by the tests:** the design binds `t` inside a level to "toggle the row's Tunnel" *and* had providers declare tunnel actions with key `t`, which `ReservedKeys` would reject — resolved by making `t` the tunnel key the way enter is the drill key: `provider.TunnelKey`; every Tunnel action must carry it and no other kind may (`TestATunnelActionUsesFleetsTunnelKey`). Design §4.2 note added in this PR. Host gotcha: `go` is a shell function pinning `GOTOOLCHAIN=local`; use `export GOTOOLCHAIN=auto` + `command go`. |
| 2026-09-05 | PR split + k8s | Operator set the priority herdr → ports → k8s resources and asked for one PR per blocking leaf with complete plans. Plan §6.1 now names eight stacked PRs (contract → protocol → registry → {herdr, tui, cli} → bridges → integrate) with worker refs, bases and review gates; TODO phases carry their PR and a CHECKPOINT line; this ledger lists the planned workers. Non-goals cut to two rejected items; the roadmap became design goal 8 with index rows for containers, sessions, system, a declarative plugin and remote transports. Two contract additions before the freeze: `Tunnel.Keeper` (a host-side command the bridge keeps alive — what `kubectl port-forward` needs) and `ReservedKeys` + the F18c rule that any declared key runs the cursor row's action. Probed the three Kubernetes hosts and wrote the `fleet-connect-k8s` design, spec, plan (8 tasks, PRs kA–kD) and trio. Anchored 2026-09-05 21:16 PDT: sub-issues #289–#296 under #266, k8s design issue #297 with #298–#301, #266's DAG refreshed to the eight-PR stack, program label `fleet-connect` on #266/#267; hook bug filed as #302. Pushed via classic `gss push` from the worktree (the worker row is still missing from the gss registry). |
