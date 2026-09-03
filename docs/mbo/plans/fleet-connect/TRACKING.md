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
| (single worker — default) | — | `worktree/fleet-more-than-ssh` | `~/.herdr/worktrees/dotfiles/worktree-fleet-more-than-ssh` | — | design docs only |

If the operator elects the plan §6.1 fan-out, paste each `gss feature worker add --json` result
verbatim into this table (worker_ref, branch, worktree_path, base_branch, PR).

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| T1 `pkg/provider` types | todo | | | leaf A |
| T2 runner handoffs | todo | | | leaf A |
| T3 cancellable streams | todo | | | leaf A |
| T4 test harness (FakeProvider, StubPlugin) | todo | | | leaf A — **contract freezes here** |
| T5 wire + framing | todo | | | leaf B |
| T6 `Serve` (plugin side) | todo | | | leaf B |
| T7 `Client` + handshake | todo | | | leaf B |
| T8 `host/exec` bridge | todo | | | leaf B — **protocol freezes here** |
| T9 registry | todo | | | leaf C |
| T10 config loader | todo | | | leaf C — adds `yaml.v3` |
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
| T25 register + docs + live | todo | | | leaf G |

## 2. Feature → proof matrix (from spec §5)

| Feature | Automated proof | Human/live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 contract | [ ] T1 | — | JSON round-trip, validation, short cells |
| F2 handoff execution | [ ] T2 | [ ] live dry-run argv | mux + `-t`; local argv no shell |
| F3 cancellable streams | [ ] T3 | — | needed by k8s `logs -f` |
| F4 protocol handshake | [ ] T7 | [ ] `providers check` transcript | mismatch + immediate exit |
| F5 protocol methods | [ ] T5, T6 | [ ] `providers check` transcript | framing, id correlation, attrs echo |
| F6 `host/exec` callback | [ ] T8 | [ ] transcript shows the exec | leak sweep + `-32001` |
| F7 plugin lifecycle | [ ] T11 | [ ] broken-plugin row | deadline, kill, isolation, reuse |
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
| F21 `fleet connect` | [ ] T24 | [ ] live dry-run + refusal | exact argv |

## 3. Validation done-when — the stop condition

- [ ] Every `TODO.md` box ticked.
- [ ] Every §1 row `done` with a commit SHA **and** observed evidence.
- [ ] Every §2 feature has at least one ticked automated proof (and its live proof where listed).
- [ ] `pkg/provider`, `internal/providers`, `internal/provider/herdr` each ≥ 90% coverage.
- [ ] `cd sdk/fleet && go test -race ./...` green; `gofmt -l .` empty; `go vet ./...` clean.
- [ ] `./scripts/test.sh` green (fleet floor 60 not breached).
- [ ] `make lint-shell && make lint-portability` green (the remote `sh` scripts).
- [ ] No existing test or golden frame modified (`git diff` on `cmd/*_test.go` shows additions only).
- [ ] `pkg/provider` proven stdlib-only (`go list -deps`).
- [ ] The ten `runner.Exec{}` sites and `cmd/wake.go`'s type assertion untouched.
- [ ] Live gate 1: `fleet providers check herdr --host <spark>` raw exchange captured.
- [ ] Live gate 2: three-level drill-down + real attach + dashboard returns with the row re-polled.
- [ ] Live gate 3: the same tree via `fleet provider serve herdr` as an external plugin, identical.
- [ ] Live gate 4: a host without herdr renders the absent row naming the paths tried.
- [ ] Live gate 5: `fleet ls <spark> herdr default --json` and `connect … --dry-run` captured.
- [ ] Live gate 6: a deliberately broken plugin explains itself; other providers still render.
- [ ] The eight-step manual checklist in plan §6 signed off.
- [ ] `sdk/fleet/AGENTS.md` carries the 15 drill-down/provider invariants; READMEs carry **real**
      pasted demos.
- [ ] `docs/mbo/index.md` row advanced to `in-review`.

## 4. Blockers & escalations

Failing command + its **real** output. Contract defects go here and get escalated — never
silently patched.

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only — never rewrite history)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-09-02 | planning | Probed all four fleet hosts read-only (runtimes, sessions, ports) and verified herdr 0.8.2's CLI surface, including the non-login PATH gotcha. Brainstormed shape, scope cut (framework + herdr now, k8s next), and navigation (push views + breadcrumb). Operator then asked for tools as **plugins over a local, MCP-like RPC** — design reworked around a versioned JSON-RPC protocol with a `host/exec` callback. Wrote design, spec (21 features, per-feature criteria), plan (25 tasks, DAG) and this trio. Found and folded in one protocol correction: `host/exec` takes a `callId`, not an alias. No code written. |
