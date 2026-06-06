# prping — a Claude-native PR/push notifier

- **Date:** 2026-06-05
- **Status:** Approved (design) — pending spec review → implementation plan
- **Author:** brainstormed with the user

## 1. Goal

Get a phone notification, via Claude's own Remote Control / mobile-app channel, when:
1. an **open PR's branch head advances** on origin (a push landed), and
2. an open PR becomes **ready to merge** or **needs a branch update**.

(v1 keys all events on open PRs; bare-branch pushes with no PR yet are §10 out-of-scope.)

No OS-level desktop notifications, no changes to the `gss` binary, no shell wrappers.

## 2. Why this shape (the load-bearing constraint)

`PushNotification` (the mobile/terminal push) is **agent-loop-only**: it requires a
running Claude session with **Remote Control** attached (Claude Code ≥ v2.1.110, mobile
app signed in, "Push when Claude decides" enabled). It auto-suppresses while the user is
active (<60s since last keystroke).

There is **no standalone shell→phone path** — no HTTP endpoint or CLI a Go binary like
`gss` can call to reach the phone without a live agent. Headless `-p` mode does not expose
PushNotification; hooks can only react (desktop), not push to mobile; Channels push *into*
a session (and no GitHub channel exists yet — research preview). Sources:
`code.claude.com/docs/en/{remote-control,headless,hooks-guide,channels}`.

**Consequence:** rather than hooking `gss`, run **one persistent agent watcher** that
*observes the effects* of pushes/PRs (via `git ls-remote` + `gh`) and pushes to the phone
from inside its own agent loop. This is 100% Claude-native and needs no gss/OS changes.

## 3. Architecture

A single reusable **`prping` skill** that the user starts in a Remote-Control-attached
session. It runs a self-paced `/loop` (dynamic pacing) over the current repo (default) and,
on each iteration, diffs current state against a persisted snapshot, emitting one
`PushNotification` per **state transition**.

```
prping skill (agent /loop — thin orchestrator/RELAY only)
  ├─ pr-status.sh <owner/repo>              → JSON snapshot (branch heads + open-PR states)
  ├─ notify-diff.sh <prev.json> <now.json>  → event lines (the NOTIFICATION DECISION, pure)
  ├─ state file ~/.config/prping/<owner>-<repo>.json  (last-seen snapshot)
  └─ for each event line → PushNotification  [agent loop → Remote Control → phone]
```

**Testability rule (load-bearing for confidence — §8/§9):** *every* notification decision
lives in `notify-diff.sh`, a pure function of two snapshots. The agent never decides
*whether* to notify — it only relays the lines `notify-diff.sh` prints. This shrinks the
non-deterministic agent surface to a trivial "print → PushNotification" relay and makes the
entire decision surface deterministically testable in CI. The orchestrator also supports a
`--print` mode that runs the full pipeline but echoes the would-be notifications instead of
calling PushNotification, enabling end-to-end testing with **no agent / no Remote Control**.

### 3.1 Components & boundaries

| Unit | Responsibility | Interface | Depends on |
| :--- | :--- | :--- | :--- |
| `pr-status.sh` | Emit a deterministic JSON snapshot of the repo's current state (pure I/O→JSON, no decisions) | `pr-status.sh <owner/repo>` → JSON on stdout | `gh`, `git`, `jq` |
| `notify-diff.sh` | **Pure decision**: given prev + now snapshots, print the exact event lines to notify (the §8 rules) | `notify-diff.sh <prev.json> <now.json>` → 0+ lines on stdout | `jq` only |
| state file | Persist last-seen snapshot across loop iterations / context compaction | JSON at `~/.config/prping/<owner>-<repo>.json` (gitignored, local) | — |
| `prping` SKILL.md | Orchestrate + relay: snapshot → diff → PushNotification per line → persist; cadence; prereq checks; `--print` dry-run | invoked as a skill; current repo by default, optional `<owner/repo>` arg | the two scripts, PushNotification, ScheduleWakeup |

Both `pr-status.sh` and `notify-diff.sh` are pure/mockable and unit-testable in isolation;
the agent owns only orchestration, persistence, and the relay.

### 3.2 Snapshot shape (`pr-status.sh` output)

```json
{
  "repo": "sfc-gh-eraigosa/dotfiles",
  "branchHeads": { "feature/x": "<sha>", "...": "..." },
  "prs": [
    { "number": 126, "title": "...", "branch": "feature/x", "headSha": "<sha>",
      "isDraft": false, "mergeStateStatus": "CLEAN|BEHIND|BLOCKED|DIRTY|UNKNOWN",
      "mergeable": "MERGEABLE|CONFLICTING|UNKNOWN", "failingChecks": ["Build and Integration Test"] }
  ]
}
```
Derived from `git ls-remote --heads origin` and
`gh pr list --json number,title,headRefName,headRefOid,isDraft,mergeStateStatus,mergeable,statusCheckRollup`.
(`branchHeads` is collected for forward use — v1 events read each PR's own `headSha`; the
full `branchHeads` map enables the bare-branch-push case noted in §10 without a schema change.)

## 4. Notification events (transitions only)

Emit at most one push per PR per iteration, only when state changed vs. the snapshot:

| Event | Trigger (prev → now) | Sample push |
| :--- | :--- | :--- |
| PR opened | PR number not previously seen | `📬 PR #126 opened — <title>` |
| Push landed | open PR's `headSha` advanced | `✓ pushed to PR #126 (feature/x)` |
| Ready to merge | `mergeStateStatus` → `CLEAN` | `🟢 PR #126 ready to merge — checks green, up to date` |
| Needs update | `mergeStateStatus` → `BEHIND` (checks not failing) | `🔄 PR #126 needs branch update (behind main)` |
| Check failed | a required check rolled up to failure | `❌ PR #126 check failed: Build and Integration Test` |
| Closed/merged | PR no longer open | (optional) `✅ PR #126 merged` — then drop from tracking |

De-dup rule: a transition fires once; the new state is written to the state file so it will
not re-fire until it changes again. "Push landed" also covers the first-push case once the
PR exists (the PR-opened + push events may coincide — collapse to a single `opened` push).

**Out of scope for v1:** a bare-branch push with *no* PR yet (we key on open PRs). Noted as
a possible later addition via `branchHeads` tracking independent of PRs.

## 5. Pacing

Self-paced `/loop` (dynamic mode). Fallback heartbeat ~270s (cache-warm, and CI checks take
minutes, so sub-5-min re-checks are appropriate). Optionally arm a `Monitor` on the in-flight
CI run for earlier wake; the heartbeat is the safety net. The loop runs until the user stops
it or no open PRs remain. Notifications self-suppress while the user is active (harness
behavior) — so they land when the user has actually stepped away.

## 6. Prerequisites (skill checks at start, warns if missing)

- Claude Code ≥ v2.1.110.
- A **Remote Control** session attached ("Push when Claude decides" enabled, mobile app
  signed into the same account). The repo's existing `remote-claude-session` skill can
  establish one; the skill links to it.
- `gh` authenticated; `jq` present.

If Remote Control is not detected, the skill still runs but warns that pushes will only show
as terminal notifications (no phone), and points at `remote-claude-session`.

## 7. Packaging & repo fit

- Skill dir `src/prping/`:
  - `SKILL.md` — orchestrator/relay + `--print` mode + the §9.4 manual-acceptance checklist
  - `pr-status.sh`, `notify-diff.sh` — the two pure scripts
  - `pr-status_test.sh`, `notify-diff_test.sh`, `scenario_test.sh` — the §9 harness layers
  - `testdata/` — snapshot fixtures + the `scenario/` tick sequence + golden transcripts
  `sync-skills` discovers it (now dual-scans `src/`+`sdk/`); add a name mapping if a friendly
  slash name is wanted. `src/` is the right home (non-Go tooling/skills).
- State dir `~/.config/prping/` is local/gitignored (never committed).
- Add `src/prping/GEMINI.md` + `CLAUDE.md -> GEMINI.md` symlink per repo convention,
  and link it from `src/GEMINI.md`.

## 8. Evaluation criteria (per feature)

Each feature is a precise predicate over `(prev, now)` snapshots with explicit
**fires / must-not-fire / edge / pass** rules. `notify-diff.sh` is the single source of
truth; **every rule below maps to ≥1 golden test case in §9.2** (rule-to-test traceability
is part of the Definition of Done).

**First-sight rule (global):** a PR absent in `prev` emits exactly one consolidated `opened`
line naming its current status, and its state is seeded — so status/push/check events do
**not** also fire on first sight. All other events fire only on a genuine prev→now transition.

### 8.1 PR opened
- **Trigger:** `now.prs[N]` exists ∧ `prev.prs[N]` absent.
- **Fires:** exactly one `📬 PR #N opened — <title> (<status>)`.
- **Must NOT fire:** any later tick where N was already present.
- **Edge:** opened-as-draft → `(draft)`; opened-already-CLEAN → `(ready)`, and §8.3 does **not** separately fire that tick.
- **Pass:** first sight ⇒ 1 opened line; identical next tick ⇒ 0 lines.

### 8.2 Push landed
- **Trigger:** `prev.prs[N].headSha ≠ now.prs[N].headSha` (both present; not first-sight).
- **Fires:** exactly one `✓ pushed to PR #N (<branch>) <shortSha>`.
- **Must NOT fire:** headSha unchanged.
- **Edge:** a push usually resets checks to pending (CLEAN→BLOCKED); only the push line fires that tick (pending is not an event).
- **Pass:** sha advance ⇒ 1 push line; same sha ⇒ 0.

### 8.3 Ready to merge
- **Trigger:** `now.mergeStateStatus == CLEAN ∧ ¬now.isDraft ∧ prev.mergeStateStatus ≠ CLEAN`.
- **Fires:** one `🟢 PR #N ready to merge`.
- **Must NOT fire:** already CLEAN last tick; or `isDraft` (drafts are never "ready").
- **Pass:** ¬CLEAN→CLEAN(¬draft) ⇒ 1; CLEAN→CLEAN ⇒ 0; →CLEAN(draft) ⇒ 0.

### 8.4 Needs update
- **Trigger:** `now.mergeStateStatus == BEHIND ∧ prev ≠ BEHIND ∧ now.failingChecks == []`.
- **Fires:** one `🔄 PR #N needs branch update (behind main)`.
- **Must NOT fire:** already BEHIND; or a check is failing (§8.5 wins).
- **Pass:** →BEHIND(green) ⇒ 1; BEHIND→BEHIND ⇒ 0; →BEHIND(failing) ⇒ 0 (1 check-failed instead).

### 8.5 Check failed
- **Trigger:** `now.failingChecks` contains a check ∉ `prev.failingChecks`.
- **Fires:** one `❌ PR #N check failed: <comma-joined new failures>`.
- **Must NOT fire:** identical failing set as last tick.
- **Edge:** multiple new failures ⇒ one consolidated line; a failure that clears then recurs ⇒ fires again (new transition).
- **Pass:** new failure ⇒ 1; unchanged failures ⇒ 0.

### 8.6 Closed / merged
- **Trigger:** `prev.prs[N]` present ∧ `now.prs[N]` absent.
- **Fires:** if merged, one `✅ PR #N merged`; if only closed, none. Then drop N from state.
- **Must NOT fire:** any event for N afterward.
- **Pass:** disappearance ⇒ ≤1 line then 0; state no longer contains N.

### 8.7 Global invariants (cross-feature)
- **Idempotence:** `notify-diff(now, now)` ⇒ 0 lines.
- **No-op tick:** `prev == now` ⇒ 0 lines.
- **Deterministic order:** PRs transitioning in one tick ⇒ lines ordered by PR number, stable across runs.
- **Restart-safe dedup:** reloading the state file across a process restart ⇒ no re-fire of already-emitted transitions.
- **Format:** every emitted line is one line, <200 chars, no markdown (PushNotification limits).

## 9. Verification harness

Confidence comes from collapsing the non-deterministic surface to near zero: all decisions
are in `notify-diff.sh` (pure), all I/O is in `pr-status.sh` (mockable), the agent only
relays. Three automated layers + one manual gate.

### 9.1 Layer 1 — snapshot unit tests (`pr-status_test.sh`)
Shim `gh` and `git` on `PATH` to return fixture payloads; assert `pr-status.sh` emits the
exact snapshot JSON (field extraction, draft flag, `failingChecks` derived from
`statusCheckRollup`, `branchHeads` from `ls-remote`). Cases: 0 PRs; one PR per status;
multi-PR; draft; conflicting; missing/empty fields. Uses `ai/_test_helpers.sh`.

### 9.2 Layer 2 — decision golden tests (`notify-diff_test.sh`) — the heart of the confidence story
Each §8 rule is ≥1 case: feed `(prev.json, now.json)` fixtures, assert the **exact** event
lines (golden). Every *Fires* and every *Must-NOT-fire* in §8.1–8.7, plus the §8.7
invariants, is a named case. Pure, deterministic, fast → 100% reproducible. The §8 table is
the executable spec; a coverage check asserts every rule id has a test.

### 9.3 Layer 3 — lifecycle scenario (`scenario_test.sh`)
Drive the orchestrator in `--print` mode over an ordered fixture sequence
(`testdata/scenario/tick-00.json … tick-NN.json`) simulating a real lifecycle:
open(draft) → ready-for-review → push → checks-pending → CLEAN → behind → update-push →
CLEAN → merged. Assert the concatenated notification transcript == a golden transcript.
Proves state persistence, cross-tick dedup, multi-PR interleaving, and restart-safety
(resume from a mid-sequence state file ⇒ no replay).

### 9.4 Layer 4 — manual acceptance (documented; CI cannot do it)
CI has no Remote Control, so the literal PushNotification→phone hop is verified once by a
human: start `prping` in a Remote-Control session, push to a PR branch, confirm the phone
push lands (and is suppressed while typing). The **only** step CI can't cover; it exercises a
one-line relay. Recorded as a checklist in `src/prping/SKILL.md` and signed off in the PR.

### 9.5 Skill-trigger eval (does the skill activate on intent?)
Separate from behavior: measure that the skill's *description* triggers on real phrasings
("watch my PRs", "ping me when a PR's ready to merge", "tell me when #127 can merge") and
does **not** on near-misses ("review this PR", "merge this"). Use the skill-creator eval
methodology (phrasings→expected set + variance analysis); **target ≥90% top-1 trigger
accuracy**. Tune the description, never the behavior, to hit it.

### 9.6 CI wiring & Definition of Done
Layers 1–3 are `*_test.sh` drivers under `src/prping/`, auto-discovered by `make shell-test`
→ run on every PR. **Done when:** all three layers green; every §8 rule maps to ≥1 named
§9.2 case (traceability table in the PR); skill-trigger eval ≥90%; one manual-acceptance run
signed off. Residual non-determinism = the agent's print→PushNotification relay — bounded to
one line and covered by §9.4.

## 10. Explicitly NOT in scope (and why)

- **Manual `gss push` in a bare terminal with no watcher running → phone:** impossible
  Claude-natively (no shell→phone API). The always-on watcher is the answer.
- **gss code changes / shell `gss()` wrapper / OS `notify-send` / tmux tier:** dropped — the
  watcher observes effects instead.
- **Cross-repo watching:** current repo only (v1).
- **Channels/webhook ingestion:** revisit if/when a GitHub channel ships.

## 11. Rollback

Pure addition (a skill dir + a local state file). Remove `src/prping/` and the
`~/.config/prping/` dir; nothing else is touched.

## 12. Resolved design decisions (owner, 2026-06-05) — supersede earlier sections where noted

1. **Delivery = at-most-once** (persist-snapshot-THEN-relay). A crash mid-notify may drop one
   push, never duplicates. Confirms the orchestrator ordering; no emitted-ids ledger in v1.

2. **Watch scope is selectable + label-driven** (expands §1 / §3 / §5). On start, prping
   resolves a **watched set** of PRs by one of:
   - `current` — the PR for the current branch (or an explicit `#N`);
   - `all` — every open PR in the repo;
   - `label:<name>` — open PRs carrying a watch label (default label `prping`).
   The skill asks which scope on start (default `current` when on a PR branch, else `label`).
   prping can **add/remove the watch label** on PRs (`gh pr edit <n> --add-label/--remove-label`)
   so the user marks exactly which PRs to watch. `pr-status.sh` gains a `--scope` filter; only
   the watched set enters the snapshot. This refines §1 ("current repo") to a scoped set.

3. **Self-terminate when the watched set is empty** (supersedes §5's "or no open PRs remain").
   When every watched PR has merged/closed — or no PR matches the label — prping emits a final
   `🏁 watch complete — stopping` line and exits the loop. (A `label:` scope may optionally be
   kept standing to catch newly-labeled PRs; v1 default is self-terminate on empty.)

4. **§8.6 distinguishes merged vs closed — via the snapshot, not absence.** To keep
   `notify-diff.sh` pure, `pr-status.sh` reports each *watched* PR's `state`
   (`OPEN | MERGED | CLOSED`), not just open ones — so a just-merged PR still appears in `now`
   with `state:"MERGED"`. The §8.6 trigger therefore keys on a **state transition**
   (`prev.state == OPEN ∧ now.state ∈ {MERGED, CLOSED}`), not on a PR vanishing from the list,
   and emits `✅ PR #N merged` vs `🚪 PR #N closed (not merged)`, then drops it from tracking.
   Mechanics by scope: `current`/`#N` query that PR's state directly (always known);
   `label:<name>` uses `gh pr list --label <name> --state all` (the label persists post-merge);
   `all` uses `--state all` with a recency limit to catch just-closed PRs. `notify-diff` stays a
   pure two-snapshot diff (§9.2 goldens unaffected); the cost is `pr-status` querying `--state all`
   for the watched set.

5. **Name = bare `prping`** (no sync-skills casemap entry).

6. **Remaining TODO defaults:** fork/external PRs are **out of scope for v1** (private /
   single-author assumption → identity sanitization is integrity hygiene, not an
   external-attacker path); if §9.5 trigger tuning plateaus <90%, add an explicit trigger
   phrase / the `prping` keyword rather than soften the ≥90% gate.
