# prping — a Claude-native PR/push notifier

- **Date:** 2026-06-05
- **Status:** Approved (design) — pending spec review → implementation plan
- **Author:** brainstormed with the user

## 1. Goal

Get a phone notification, via Claude's own Remote Control / mobile-app channel, when:
1. an **open PR's branch head advances** on origin (a push landed), and
2. an open PR becomes **ready to merge** or **needs a branch update**.

(v1 keys all events on open PRs; bare-branch pushes with no PR yet are §9 out-of-scope.)

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
prping skill (agent /loop)
  ├─ pr-status.sh <owner/repo>   → JSON snapshot of branch heads + open-PR states
  ├─ state file  ~/.config/prping/<owner>-<repo>.json  (last-seen snapshot)
  ├─ diff(prev, now) → list of transition events
  └─ PushNotification(one line per event)   [agent loop → Remote Control → phone]
```

### 3.1 Components & boundaries

| Unit | Responsibility | Interface | Depends on |
| :--- | :--- | :--- | :--- |
| `pr-status.sh` | Emit a deterministic JSON snapshot of the repo's current state | `pr-status.sh <owner/repo>` → JSON on stdout | `gh`, `git`, `jq` |
| state file | Persist last-seen snapshot across loop iterations / context compaction | JSON at `~/.config/prping/<owner>-<repo>.json` (gitignored, local) | — |
| `prping` SKILL.md | Drive the loop: snapshot → diff → notify → persist; cadence; prereq checks | invoked as a skill; current repo by default, optional `<owner/repo>` arg | `pr-status.sh`, PushNotification, ScheduleWakeup |

`pr-status.sh` is pure I/O→JSON (no notification logic) so it is unit-testable in isolation.
The skill owns the diff/notify/persist logic and the agent-only PushNotification calls.

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
full `branchHeads` map enables the bare-branch-push case noted in §9 without a schema change.)

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

- Skill dir: `src/prping/` with `SKILL.md` + `pr-status.sh` (+ `pr-status_test.sh`).
  `sync-skills` discovers it (now dual-scans `src/`+`sdk/`); add a name mapping if a friendly
  slash name is wanted. `src/` is the right home (non-Go tooling/skills).
- State dir `~/.config/prping/` is local/gitignored (never committed).
- Add `src/prping/GEMINI.md` + `CLAUDE.md -> GEMINI.md` symlink per repo convention,
  and link it from `src/GEMINI.md`.

## 8. Testing

- `pr-status_test.sh` (repo shell-test framework, `ai/_test_helpers.sh`): feed mocked
  `gh`/`git` output (via PATH shims) and assert the JSON snapshot shape + field extraction.
- Diff/transition logic: a small pure function (in the helper or a tiny `notify-diff.sh`)
  fed two snapshots, asserting the exact event set — positive (each transition) and negative
  (no event when unchanged) cases.
- Manual end-to-end: start the watcher, push to a PR branch, confirm the agent emits the
  expected PushNotification lines (terminal-visible even when phone is suppressed).

## 9. Explicitly NOT in scope (and why)

- **Manual `gss push` in a bare terminal with no watcher running → phone:** impossible
  Claude-natively (no shell→phone API). The always-on watcher is the answer.
- **gss code changes / shell `gss()` wrapper / OS `notify-send` / tmux tier:** dropped — the
  watcher observes effects instead.
- **Cross-repo watching:** current repo only (v1).
- **Channels/webhook ingestion:** revisit if/when a GitHub channel ships.

## 10. Rollback

Pure addition (a skill dir + a local state file). Remove `src/prping/` and the
`~/.config/prping/` dir; nothing else is touched.
