# <objective> — execution cursor

- **Slug:** <slug>
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../<slug>.md`](../<slug>.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [ ] <precondition checks from IMPLEMENTATION §1, one box each>

---

### Task 1 — <name>  (plan Task 1)

- [ ] RED: …
- [ ] RUN-RED: `<command>` → expect **FAIL**
- [ ] GREEN: …
- [ ] RUN-GREEN: `<command>` → expect **PASS**
- [ ] VERIFY: <gates>
- [ ] COMMIT: `<exact message>`
- [ ] LEDGER + CHECKPOINT

**Done when:** <the task's gate>
