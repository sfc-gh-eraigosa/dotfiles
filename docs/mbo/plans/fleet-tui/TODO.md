# fleet-tui — resumable cursor

> First unchecked box = the next action. Details live in the plan
> ([`../fleet-tui.md`](../fleet-tui.md) §4); evidence + status go to
> [`TRACKING.md`](./TRACKING.md). HUMAN STOP boxes need the operator.

## Phase 0 — setup

- [x] Design (`docs/mbo/designs/fleet-tui.md`)
- [x] Spec (`docs/mbo/specs/fleet-tui.md`)
- [x] Plan (`docs/mbo/plans/fleet-tui.md`) + this trio
- [ ] Register in `docs/mbo/index.md` (state `planning`)
- [ ] Issue created; design draft PR checkpointed
- [ ] Build worker `fleet-tui/<user>/build` created; **preflight: no foreign commits**

## Phase 1 — skeleton stays green (Tasks 1–4)

- [ ] Task 1: split tui.go → model/view/keys/cmds; alias-keyed cursor; tests unchanged-green
- [ ] Task 2: theme struct + Ascii goldens (populated, empty)
- [ ] Task 3: viewport + WindowSizeMsg + clampViewport + n/total
- [ ] Task 4: j/k, gg (pending-key), G, ctrl+d/u/f/b

## Phase 2 — live data (Tasks 5–6)

- [ ] Task 5: collectOne extraction (headless frozen) + streaming Init + spinner rows
- [ ] Task 6: `r` refresh keeping cursor+selection

## Phase 3 — find & select (Tasks 7–9)

- [ ] Task 7: `/` incremental regex, smartcase, invalid-regex UX, esc/enter
- [ ] Task 8: n/N wraparound
- [ ] Task 9: space toggle, v visual range, alias-keyed persistence

## Phase 4 — act (Tasks 10–12)

- [ ] Task 10: batch update queue + confirm strip + --update-ref (validRef)
- [ ] Task 11: `s` ssh handoff
- [ ] Task 12: `?` overlay (from keymap table) + quit-during-batch guard

## Phase 5 — prove (Tasks 13–14)

- [ ] Task 13: README/AGENTS + full gate (fleet ≥60%, cmd ≥55%, vet, leak sweep)
- [ ] **HUMAN STOP** Task 14: live tmux capture — streaming, search, 2-host batch
      with sudo prompt, FAIL-advances-queue, ssh action, fresh stamp after
- [ ] index.md → `in-review`; decide PR promotion with operator
