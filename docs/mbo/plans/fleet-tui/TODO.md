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
- [ ] index.md → `in-review`; decide PR promotion with operator
