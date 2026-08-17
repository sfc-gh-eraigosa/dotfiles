# fleet-tui — resumable cursor

> First unchecked box = the next action. Details live in the plan
> ([`../fleet-tui.md`](../fleet-tui.md) §4); evidence + status go to
> [`TRACKING.md`](./TRACKING.md). HUMAN STOP boxes need the operator.

## Phase 0 — setup

- [x] Design (`docs/mbo/designs/fleet-tui.md`)
- [x] Spec (`docs/mbo/specs/fleet-tui.md`)
- [x] Plan (`docs/mbo/plans/fleet-tui.md`) + this trio
- [x] Register in `docs/mbo/index.md`
- [x] Issue created (#226); draft PR #227 checkpointed
- [x] Built in the design worker / PR #227 (operator: same PR); preflight clean

## Phase 1 — skeleton stays green (Tasks 1–4)

- [x] Task 1: split tui.go → model/view/keys/cmds; alias-keyed cursor; tests unchanged-green
- [x] Task 2: theme struct + Ascii goldens (populated, empty)
- [x] Task 3: viewport + WindowSizeMsg + clampViewport + n/total
- [x] Task 4: j/k, gg (pending-key), G, ctrl+d/u/f/b

## Phase 2 — live data (Tasks 5–6)

- [x] Task 5: collectOne extraction (headless frozen) + streaming Init + spinner rows
- [x] Task 6: `r` refresh keeping cursor+selection

## Phase 3 — find & select (Tasks 7–9)

- [x] Task 7: `/` incremental regex, smartcase, invalid-regex UX, esc/enter
- [x] Task 8: n/N wraparound
- [x] Task 9: space toggle, v visual range, alias-keyed persistence

## Phase 4 — act (Tasks 10–13)

- [x] Task 10: concurrent bg update engine — precheck routing, ≤ `--jobs` slots,
      BatchMode fast-FAIL, TUI stays live, refresh excludes updating, confirm
      strip, `--update-ref` (validRef) + `--jobs`
- [x] Task 11: interactive fallback queue (precheck-failed hosts, serial `ssh -t`
      handoffs after the wave)
- [x] Task 12: `s` ssh handoff (blocked while that host updates)
- [x] Task 13: `?` overlay (from keymap table) + quit-during-updates guard

## Phase 5 — prove (Tasks 14–15)

- [x] Task 14: README/AGENTS + full gate (fleet ≥60%, cmd ≥55%, vet, leak sweep)
- [ ] **HUMAN STOP** Task 15: live tmux capture — streaming, search, **two hosts
      updating concurrently while navigating + `r`**, FAIL refills its slot,
      fallback handoff with sudo prompt, ssh action, fresh stamps after

## Phase 6 — reachability rescue (Tasks 18–24)

> Motivated by the design §1.1 incident: a Wi-Fi power-saving host missed the
> broadcast ARP that a cold neighbour cache must send, so it read as
> `unreachable` for hours while a peer on the same AP reached it in 6 ms.

- [x] Task 18: `RunVia(peer, host, argv…)` on the `Runner` interface + `Exec`
      (`ssh -J`) + `Fake` (records the hop) — **blocks Task 17**
- [x] Task 19: `internal/reach` ladder (retry → local-prime → peer-relay), one
      context deadline, peer ranking, injected clock. Write **F14b first** — the
      relay-succeeds-but-direct-probe-fails guard is the invariant that keeps
      wake from turning a partition into a false green
- [x] Task 20: auto-wake in `probeHost` + `--no-wake` / `--wake-timeout`;
      `woke via <peer>` note; concurrency assertion (N sleepers ≈ one budget)
- [x] Task 21: `fleet wake [host…]` verb — rung-by-rung output, `--json`, exit
      code, **non-mutation argv assertion**, no `ping -W`
- [x] Task 22: TUI `w` key, `waking` ownership state, `keyHelp` row, re-poll on
      completion
- [x] Task 23: README/AGENTS (incl. the operator note that the permanent cure is
      disabling Wi-Fi power save on the sleeper) + full gate + portability lint
- [ ] **HUMAN STOP** Task 24: hardware reproduction — neighbour table with **no
      entry** → `fleet wake` escalating to `peer-relay` → neighbour table with a
      resolved MAC → row at its true class with `woke via <peer>`

## Phase 7 — land

- [ ] index.md → `in-review`; decide PR promotion with operator
