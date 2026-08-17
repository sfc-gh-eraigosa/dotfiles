# fleet-tui — implementation procedure

- **Slug:** fleet-tui
- **Plan (source of truth):** [`../fleet-tui.md`](../fleet-tui.md) · spec [`../../specs/fleet-tui.md`](../../specs/fleet-tui.md) · design [`../../designs/fleet-tui.md`](../../designs/fleet-tui.md)
- **Ledger:** [`TRACKING.md`](./TRACKING.md) · **Cursor:** [`TODO.md`](./TODO.md)

## 1. Operating mode

Single gss worker, sequential Tasks 1–14 (plan §6.1: no CAP-B breakout — one
model struct, parallel leaves would collide). TDD per task:
RED → RUN-RED → GREEN → RUN-GREEN → VERIFY (vet + full suite) → evidence →
commit → update TRACKING/TODO.

## 2. Session preflight (every session, before any edit)

1. `git log --oneline origin/main..HEAD` — anything before your own commits
   means the branch is on a stale local main (hit 3× on `fleet`): fix with
   `git reset --hard origin/main` (before Task 1) or rebase `--onto`.
2. `go test ./... -count=1` in `sdk/fleet` — start green or stop.
3. Read TODO.md — first unchecked box is the next action.

## 3. Build worker

```
gss feature worker add --feature fleet-tui --purpose build \
  --description "fleet tui v2 build per docs/mbo/plans/fleet-tui.md" --json
```

(then preflight #1 in the new worktree — this is where the stale-main hazard
bites). Checkpoint (`gss feature checkpoint --worker fleet-tui/<user>/build`)
after every task or two; it maintains the draft PR.

## 4. Hard rules

- Tests pin `lipgloss.SetColorProfile(termenv.Ascii)` in `TestMain` — CI has
  no TTY; goldens must be byte-stable.
- `cursor`/`selected` are **alias-keyed** — any index-keyed regression breaks
  F1b/F7a; the tests exist to catch it.
- No `time.Now()` in model/view — `now` is injected (fleet invariant).
- Headless commands' behavior is frozen: `status`/`update` tests must pass
  **unmodified** except the Task 5 `collectOne` extraction (which adds a test,
  changes none).
- Privacy: placeholders in all committed content incl. evidence
  (`<user>`/`<host>`/`host-pi`…); sanitize tmux captures before add.
- Every ExecProcess completion re-enters Update() and advances state — a
  wedged queue is a bug class the F8c test pins.

## 5. Evidence protocol

`docs/mbo/plans/fleet-tui/evidence/taskNN/<name>.txt` — dated header, the
exact command, its real output (`tee`), append-only, committed with the task.
Task 14 (live capture) is HUMAN-gated: operator at the terminal, tmux
capture-pane, sanitized.

## 6. Done

Plan §4 Task 13 gate green + Task 14 capture committed + TRACKING §3 boxes all
ticked + `index.md` state → `in-review` + PR promoted per operator decision.

## 7. Rollback

Revert the PR. No persistent/remote state (design §6).

## 8. Kickoff prompt (for a fresh session)

> Work the fleet-tui objective. Read
> `docs/mbo/plans/fleet-tui/{TODO.md,TRACKING.md}` and
> `docs/mbo/plans/fleet-tui.md` (plan = source of truth). Run the §2 preflight.
> Then execute the first unchecked TODO box, TDD, capturing evidence per §5,
> updating the trio after each task. Stop at HUMAN STOP boxes and report.
