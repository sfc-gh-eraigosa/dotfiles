# prping — a Claude-native PR/push notifier (Go CLI + skill)

- **Date:** 2026-06-05 · **Revised:** 2026-06-06 — the deterministic core is now a **single Go
  CLI** (`sdk/prping`, mirroring the other sdk tools) instead of a set of bash scripts.
- **Status:** Approved (design) — pending plan regen to the Go-CLI shape.
- **Relates to:** PR #127.
- **Author:** brainstormed with the user; architecture team reviewed.

## 1. Goal

Get a phone notification, via Claude's own Remote Control / mobile-app channel, when a
**watched** PR (see §5 scope) changes state: a push lands, it becomes **ready to merge** or
**needs a branch update**, a check fails, or it merges/closes.

No OS-level desktop notifications, no changes to `gss`, no shell-script soup — one Go binary
`prping` owns the deterministic work; a thin agent skill relays its output to the phone.

### 1.1 Use cases

**UC1 — Draft-worktree auto-watch.** *Actor:* a developer working in a `gss feature` worktree on
a draft PR. *Trigger:* runs bare `prping` ("watch this when it's ready"). *Flow:* the skill
defaults `--scope current`, auto-detecting the PR for the checked-out branch (`gh pr view`); the
loop runs `prping tick` on the interval and pushes when that PR is **ready = CLEAN** — checks
green **AND** rebased on main (not `BEHIND`), per §8.3. *Acceptance:* a phone push fires exactly
when the current PR first reaches `CLEAN`; nothing while it's pending/behind; self-terminates
(exit 10) when the PR merges. (Notify-only by default; if the user asks, offer `gss feature pr
--ready` to flip the draft to ready — the skill proposes, never auto-promotes.)

**UC2 — Remote-control, multi-goal watch.** *Actor:* a developer with a Remote-Control session
where Claude is working several PRs at once. *Trigger:* starts `prping` with `--scope all` or
`--scope label:prping`. *Flow:* one watcher tracks every in-flight PR; each tick diffs the whole
watched set and pushes per-PR ready/needs-update/merged events. The loop's ~270s heartbeat + the
self-suppress-while-active behavior keep it from starving the agent's real work (it's a thin
`prping tick` call, not a blocking poll). *Acceptance:* each PR independently triggers one
ready-to-merge push when it reaches `CLEAN`; no cross-PR spam; the watched set shrinks as PRs
merge and the loop stops when empty.

**UC3 — Status table (one-shot, `prping status`).** *Actor:* a developer who asks "what's the
state of all my PRs?". *Trigger:* `prping status` (no loop, no notifications). *Flow:* prints a
table — one row per watched PR — with the columns in §3.5. *Acceptance:* the table shows, per PR:
its number/title, the linked issue(s), a short description, mergeable status, last-updated, and a
count/list of **open PR comments with no response** (unresolved review threads whose latest
comment isn't the PR author's). A `--json` form is available for scripting.

## 2. Why this shape (the load-bearing constraint)

`PushNotification` (the mobile/terminal push) is **agent-loop-only**: it needs a running Claude
session with **Remote Control** attached (Claude Code ≥ v2.1.110, mobile app signed in, "Push
when Claude decides" enabled); it auto-suppresses while the user is active (<60s idle). There is
**no standalone shell→phone path** (verified against the Remote Control / headless / hooks /
channels docs). So we don't hook `gss` or use OS notifiers — a **persistent agent watcher**
observes git/gh effects and relays. The watcher's deterministic work (fetch state, decide which
events fire, persist) lives in the `prping` binary; the only agent-side action is calling
`PushNotification` once per event line the binary prints.

## 3. Architecture — a single Go CLI + a thin skill

`prping` is a Go module at `sdk/prping`, built and tested exactly like `sdk/gss` (cobra
commands, `internal/` packages, a mockable runner, `internal/version` ldflags, `go install`-able
as `github.com/sfc-gh-eraigosa/dotfiles/sdk/prping`). The agent **skill** drives a self-paced
loop that calls `prping tick` each iteration and relays its printed lines to `PushNotification`.

```
prping skill (agent /loop — relay only)            sdk/prping (Go CLI — all determinism)
  each tick:                                          prping tick --scope <s> --repo <r>
   └─ run `prping tick ...`  ──────────────────────►   ├─ internal/gh.Runner  → gh / git (mockable)
   └─ for each printed line → PushNotification         ├─ internal/snapshot.Build(runner, scope) → Snapshot
   └─ exit 10 ⇒ watched set empty ⇒ stop the loop      ├─ internal/state (load prev; atomic persist; lock)
   └─ else re-arm Monitor / sleep ~270s, repeat        ├─ internal/diff.Diff(prev, now) → []Event  (§8 rules)
                                                        └─ persist now, THEN print event lines (at-most-once)
```

### 3.1 CLI surface (subcommands)

| Command | Purpose | Output / exit |
| :-- | :-- | :-- |
| `prping snapshot --scope <s> [--repo <owner/repo>]` | Pure-ish: build the JSON snapshot of the watched set from `gh`/`git` | snapshot JSON on stdout |
| `prping diff <prev.json> <now.json>` | **Pure decision**: emit the event lines for the prev→now transition (the §8 rules) | 0+ lines on stdout |
| `prping tick --scope <s> [--repo <r>] [--state-dir <d>] [--dry-run]` | One watch iteration: load prev → `snapshot` → `diff` → **persist** → print events | event lines; **exit 10 = watched set empty (done)** |
| `prping label add\|remove <#…> [--label prping]` | Mark/unmark PRs to watch (`gh pr edit --add-label/--remove-label`) | — |
| `prping status --scope <s> [--repo <r>] [--json]` | **One-shot read-only** PR status table (UC3, §3.5) — no loop, no notifications | ASCII table (or JSON) on stdout |
| `prping version` | ldflags version (like every sdk tool) | version block |

`--scope` ∈ `current` (the PR for the current branch, or `#N`) · `all` · `label:<name>`
(default label `prping`). Default scope: `current` when on a PR branch, else `label:prping`.
`--dry-run` computes + prints but does not persist (test/inspection aid). The skill normally
runs `tick`; `snapshot`/`diff` are exposed for composition and golden testing.

### 3.2 Package layout (`sdk/prping/`, mirrors `sdk/gss`)

```
sdk/prping/
  main.go                      → cmd.Execute()
  cmd/                         cobra: root.go, snapshot.go, diff.go, tick.go, label.go, status.go, version.go (+ *_test.go)
  internal/
    gh/        Runner interface { Run(ctx, name string, args ...string) ([]byte, error) }
               exec.go (real) + fake/ (records calls, returns scripted output) — mirrors gss internal/git
    snapshot/  Build(runner, scope) (Snapshot, error): runs gh/git, parses → Snapshot  (table-tested w/ fake)
    diff/      Diff(prev, now Snapshot) []Event + Event.Line() formatting (sanitize, <200 chars)  (golden-tested — the heart)
    status/    Enrich(runner, Snapshot) []Row (linked issues + unanswered review threads) + Render(rows)→table/JSON (UC3 §3.5; pure, table-tested)
    state/     load/persist (temp-file+rename, 0700 dir / 0600 file, umask 077), per-repo lockfile, filename derivation, seed-silent
    version/   ldflags vars (Version/Commit/BuildDate/Dirty)
  build.sh  VERSION  LICENSE  README.md  go.mod  go.sum
  skill/SKILL.md               the agent skill (drives the loop, relays, prereqs, manual-acceptance checklist)
  GEMINI.md  CLAUDE.md→GEMINI.md
```

The `gh.Runner` interface is the testability seam (exactly like gss's `git.Runner`): real impl
shells out to `gh`/`git`; the `fake` returns fixture payloads so `snapshot` and `tick` are
deterministic in unit tests. `diff` is pure (two snapshots in, lines out) and needs no runner.

### 3.3 Snapshot + state JSON (schema-closed)

```json
{ "repo": "acme/widgets", "scope": "label:prping",
  "prs": [
    { "number": 126, "title": "…", "branch": "feature/x", "headSha": "<sha>",
      "state": "OPEN|MERGED|CLOSED", "isDraft": false,
      "mergeStateStatus": "CLEAN|BEHIND|BLOCKED|DIRTY|UNKNOWN",
      "mergeable": "MERGEABLE|CONFLICTING|UNKNOWN",
      "failingChecks": ["Build and Integration Test"] } ] }
```
`snapshot` queries the watched set with `gh pr list/view … --state all` (so a just-merged PR is
present with `state:"MERGED"` — §8.6 keys on a *state transition*, not on a PR vanishing, which
keeps `diff` a pure two-snapshot comparison). The state file is the last-seen snapshot at
`~/.config/prping/<owner>-<repo>.json` (gitignored, local; never committed).

### 3.4 Determinism boundary
Everything except the literal `PushNotification` call is in the binary and unit-testable. The
agent does exactly two non-deterministic things: relay each printed line to PushNotification, and
pace the loop. `prping tick` is the contract between them.

### 3.5 The `status` command (UC3 — one-shot table)

A read-only sibling to `tick`: it reuses `internal/snapshot` for the watched set and adds an
`internal/status` package that enriches each PR with issue links + unanswered-comment detection,
then renders a table. No state file, no notifications. Columns:

| Column | Source |
| :-- | :-- |
| **PR** (`#num title`) | snapshot |
| **Issue(s)** | `gh pr view <n> --json closingIssuesReferences` (the "Closes #" links) ∪ `#NN` refs parsed from the PR body |
| **Description** | first sentence of the PR body (truncated) |
| **Mergeable** | `mergeStateStatus` + `mergeable` from the snapshot (`CLEAN`/`BEHIND`/`BLOCKED`/`CONFLICTING`…) |
| **Updated** | `updatedAt` (relative, e.g. "2h ago") |
| **Unanswered** | count + ids of **open PR comments with no response** (see definition) |

**"Open PR comment with no response" (the load-bearing definition):** an **unresolved review
thread** whose **latest comment's author ≠ the PR author** — i.e. a reviewer asked something and
the author hasn't replied or resolved it. Source: `gh api graphql` on
`pullRequest.reviewThreads(isResolved:false){ comments(last:1){ author } }`. Bot authors are
excluded. (Top-level issue-comments are noisier and excluded from v1 — review threads are the
actionable "you owe a reply" signal.) Like `diff`, the enrichment is a **pure function** of the
fetched data → table-tested with the fake `gh.Runner`.

Rendering: fixed-width ASCII columns with per-cell truncation; `--json` emits the structured rows
(same data) for scripting. Scope follows §3.1 (`current`/`all`/`label:<name>`).

## 4. Notification events (produced by `prping diff`)

`diff` emits at most one line per PR per tick, only on a genuine prev→now transition. Sample
lines (full rules in §8): `📬 PR #126 opened — <title> (<status>)` · `✓ pushed to PR #126
(feature/x) <sha>` · `🟢 PR #126 ready to merge` · `🔄 PR #126 needs branch update (behind main)`
· `❌ PR #126 check failed: <check>` · `✅ PR #126 merged` / `🚪 PR #126 closed (not merged)` ·
final `🏁 watch complete — stopping` when the watched set empties (drives self-terminate).

## 5. Pacing, scope & lifecycle

- **Self-paced loop** via the `loop` skill; fallback heartbeat ~270s; optional `Monitor`
  (persistent, re-armed each iteration to survive its ~1h cap) for early wake on in-flight CI.
  (`ScheduleWakeup`/`CronCreate` are **not** used — the former doesn't exist as a primitive here
  and cron auto-expires / only fires while idle.)
- **Scope** (§3.1) chosen on start: `current` / `all` / `label:<name>`. `prping label add <#>`
  lets the user mark PRs to watch under the default `prping` label.
- **Self-terminate:** when every watched PR has merged/closed (or none match the label),
  `prping tick` exits 10; the skill emits the final line and stops the loop.
- Notifications self-suppress while the user is active — they land once they've stepped away.

## 6. Prerequisites (skill checks on start; warns if missing)

Claude Code ≥ v2.1.110; a **Remote Control** session attached ("Push when Claude decides" on,
mobile app signed in — the repo's `remote-claude-session` skill establishes one); `gh`
authenticated; the `prping` binary on `PATH` (`install.sh` builds it to `~/opt/bin/prping`). If
Remote Control is absent the skill still runs but warns pushes are terminal-only.

## 7. Packaging & sdk integration

`prping` is a first-class sdk module — everything that applies to `gss/gsl/wol/tmux-mgr` applies:

- **Module path** `github.com/sfc-gh-eraigosa/dotfiles/sdk/prping`; `build.sh` injects version via
  `-ldflags -X …/internal/version.*`; `VERSION` starts `0.1.0`; `LICENSE` Apache-2.0; tag scheme
  `sdk/prping/vX.Y.Z` (the existing tag-automation workflow picks it up from `VERSION`).
- **install.sh:** add a build+install block (mirrors the gss block) → `~/opt/bin/prping`.
- **Tests/coverage:** `scripts/test.sh` discovers it under `sdk/` automatically; add a
  `COVERAGE_MIN[prping]=70` entry (the `diff`/`snapshot` packages are pure → easily >85%). CI runs
  it via the existing Go path — **no shell-test harness changes needed** (this supersedes the
  prior plan's "patch `make shell-test`" blocker; that was an artifact of the bash design).
- **Skill:** `sdk/prping/skill/SKILL.md` is discovered by `sync-skills` (which dual-scans
  `src/`+`sdk/` since the gss migration); name maps to bare `prping` (add a case-map entry only if
  a friendlier slash name is later wanted).
- **Docs:** `sdk/prping/GEMINI.md` + `CLAUDE.md→GEMINI.md`; add a row to `sdk/GEMINI.md`’s module
  table; `.golangci.yml` module comment lists it.
- **Lint:** golangci-lint per-module via the existing `make lint-go` sdk loop; `go vet` clean.

## 8. Evaluation criteria (per feature)

Each feature is a predicate over `(prev, now)` snapshots with explicit **fires / must-not-fire /
edge / pass** rules, implemented in `internal/diff` and **mapped 1:1 to a golden test case in
§9.2** (traceability is a DoD item).

**First-sight rule (global):** a PR absent in `prev` emits one consolidated `opened` line naming
its current status and seeds its state — status/push/check events do not also fire on first sight.

### 8.1 PR opened — `now.prs[N]` exists ∧ `prev` absent ⇒ one `📬 …opened…(<status>)`; never re-fires; draft→`(draft)`, already-CLEAN→`(ready)` (no separate ready that tick).
### 8.2 Push landed — `prev.headSha ≠ now.headSha` (both present) ⇒ one `✓ pushed…<sha>`; not on unchanged sha; a push resetting checks to pending is not a status event that tick.
### 8.3 Ready to merge — `now.mergeStateStatus==CLEAN ∧ ¬draft ∧ prev≠CLEAN ∧ prev≠UNKNOWN` ⇒ one `🟢 ready`; not when already CLEAN, draft, or recovering from a transient `UNKNOWN` flap.
### 8.4 Needs update — `now==BEHIND ∧ prev≠BEHIND ∧ now.failingChecks==[]` ⇒ one `🔄 needs update`; not when already BEHIND or a check is failing (§8.5 wins).
### 8.5 Check failed — `now.failingChecks` has a check ∉ `prev.failingChecks` (and `now≠UNKNOWN`) ⇒ one `❌ check failed: <new failures>`; not for an unchanged failing set; a transient empty-then-refill must not manufacture a failure.
### 8.6 Merged / closed — `prev.state==OPEN ∧ now.state∈{MERGED,CLOSED}` ⇒ `✅ merged` or `🚪 closed (not merged)`, then drop N. (Snapshot carries `state` via `--state all`, so `diff` stays pure.)
### 8.7 Global invariants — `Diff(now,now)==[]`; `prev==now ⇒ []`; lines ordered by PR number, stable; restart reloads state with no re-fire; every line is one line, <200 chars, no markdown. **Totality:** `UNKNOWN`/missing/null/malformed fields ⇒ seed-silent, emit nothing, never panic.
### 8.8 Precedence (when several could fire in one tick) — check-failed > needs-update; push-landed is independent and may co-emit; opened collapses status on first sight; merged/closed is terminal for that PR.

## 9. Verification harness (Go testing)

The Go form makes confidence cheaper than the bash form: the decision surface is a pure function
under test, and everything runs through the existing `scripts/test.sh` → CI Go path.

- **9.1 `internal/diff` golden table tests (the heart).** One+ named case per §8.1–8.8 rule
  (fires + must-not-fire) + §8.7 invariants + totality on degraded payloads + the eventual-
  consistency flap guards (CLEAN→UNKNOWN→CLEAN must not re-fire ready; empty→refill must not
  manufacture check-failed). Pure, deterministic; a coverage assert maps every rule id to a case.
- **9.2 `internal/snapshot` tests with the fake `gh.Runner`.** Feed fixture `gh`/`git` payloads;
  assert the exact `Snapshot` (field extraction, draft, `failingChecks` from `statusCheckRollup`,
  `state` from `--state all`, scope filtering, merged/closed). Cases: 0 PRs, each status, multi,
  draft, conflicting, UNKNOWN, missing fields.
- **9.3 `internal/state` tests.** Atomic write (temp+rename; truncation-safe), perms, lockfile
  (second writer no-ops/refuses), filename derivation (`owner/repo`→safe name, collision-free),
  seed-silent on absent/empty/malformed prev.
- **9.4 `cmd/tick` lifecycle integration test (in-process, fake runner + temp `--state-dir`).**
  Drive an ordered fixture sequence (open(draft) → ready-for-review → push → pending → CLEAN →
  behind → update → CLEAN → merged) and assert the printed transcript == golden, plus restart-
  resume (no replay) and the exit-10 empty-set signal. This is the §9.3 lifecycle proof in Go.
- **9.5 Coverage gate.** `COVERAGE_MIN[prping]=70` via `scripts/test.sh`; `go vet` + golangci-lint
  clean. Runs in CI through the existing Go discovery.
- **9.6 Skill-trigger eval (human-evidenced).** A phrasings→expected set ("watch my PRs", "ping me
  when a PR's ready") with variance analysis; **target ≥90% top-1**; tune the description, not
  behavior. Fallback if it plateaus: add an explicit trigger phrase, do not soften the gate.
- **9.7 Manual phone-acceptance (human-evidenced; CI can't do it).** Start `prping` in a Remote-
  Control session, push to a watched PR, confirm the phone push (suppressed while typing). The one
  hop CI cannot cover; a one-line relay; checklist in `SKILL.md`, signed off on the PR.
- **DoD:** 9.1–9.5 green in CI; every §8 rule → ≥1 named test; trigger-eval ≥90% recorded; one
  manual sign-off. Automated vs human-evidenced gates are kept explicitly separate.

## 10. Explicitly NOT in scope (v1)

- Manual `gss push` in a bare terminal with no watcher → phone (impossible Claude-natively).
- gss/OS/tmux notifiers; cross-repo watching; Channels/webhook ingestion.
- Fork/external-contributor PRs (private/single-author assumption → title/branch sanitization is
  integrity hygiene, not an external-attacker path).
- The `prping` binary calling `PushNotification` itself (it can't — agent-only; binary prints, agent relays).

## 11. Rollback

Pure addition: a new `sdk/prping` module + a local state dir. Remove `sdk/prping/` and
`~/.config/prping/`, revert the `install.sh` / `scripts/test.sh` / `sdk/GEMINI.md` / `.golangci.yml`
additions; nothing else is touched. (Same shape as the sdk-migration module adds.)

## 12. Resolved design decisions (owner) — carried forward, mapped to the CLI

1. **Delivery = at-most-once** — `tick` persists the new snapshot **before** printing events; a
   crash drops at most one line, never duplicates. No emitted-ids ledger in v1.
2. **Scope = selectable + label-driven** — `--scope current|all|label:<name>` (default label
   `prping`); `prping label add/remove` marks PRs; skill asks scope on start.
3. **Self-terminate on empty watched set** — `tick` exit 10; skill stops.
4. **Merged vs closed distinguished** — via `snapshot --state all` carrying `state`; §8.6 keys on
   the state transition; `diff` stays pure.
5. **Name = bare `prping`** (no case-map entry).
6. **TODO defaults** — fork PRs out of scope v1; trigger-eval fallback = explicit phrase.
7. **(New, this revision)** The deterministic core is a **Go CLI**, not bash — same standards as
   the other sdk tools (cobra, `internal/` + mockable runner, ldflags version, `go install`-able,
   coverage-gated). This removes the bash shell-test wiring (and its CI blocker) entirely.
