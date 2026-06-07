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

Three first-class use cases drive the CLI surface. Each is an actor / trigger / flow / acceptance
contract; the spec deltas that implement them are consolidated in **§1.2** (read it as the
normative source — §3.3/§3.5/§8.3 are amended there).

**UC1 — Draft-worktree auto-watch (the "watch this when it's ready" path).**

- **Actor:** a developer working inside a `gss feature` worktree on a **draft** PR, actively
  pushing commits to the checked-out branch.
- **Trigger:** runs **bare `prping`** (≡ `prping tick` — `tick` is the default cobra command) with
  no PR argument and no explicit scope. Intent: "watch the PR I'm on and ping me when it's
  genuinely mergeable."
- **Flow:**
  1. The skill resolves **`--scope current`** (the default when on a PR branch) and the CLI
     **auto-detects the PR** by running `gh pr view --json …` with **no PR argument**, which
     resolves the PR for the branch checked out in `cwd` (verified: returns the current PR, e.g.
     `#127`). The gss registry (`Worker.worktree → pr_url`) is only a **secondary** fallback,
     used iff the no-arg `gh pr view` fails **and** a gss registry exists.
  2. The loop runs `prping tick --once` each iteration; the Monitor/loop owns cadence (~270s),
     not the binary.
  3. Each tick the **ready predicate (§8.3)** is evaluated: the PR is **ready ⇔ `mergeStateStatus
     == CLEAN` ∧ `¬isDraft` ∧ `prev.mergeStateStatus ∉ {CLEAN, UNKNOWN}`**. `CLEAN` already
     subsumes "all required checks green **AND** the branch is **not `BEHIND`** its base" — so no
     separate git/rebase check is performed. `UNSTABLE`/`HAS_HOOKS`/`BEHIND`/`BLOCKED`/`DIRTY`/
     `UNKNOWN` are **not** ready.
  4. A **CLEAN-but-still-draft** PR fires a distinct one-shot **🟡** line (§8.3a) carrying the
     suggested promote command **as text** — `prping` never flips the draft itself.
  5. The watcher self-terminates (exit 10) when the PR merges/closes.
- **Acceptance criteria:**
  - **AC1.1** A phone push fires **exactly once** when the current PR first reaches `CLEAN`
    (e.g. `BEHIND → CLEAN` after a rebase, or `UNSTABLE → CLEAN` after checks pass).
  - **AC1.2** No ready push fires while the PR is pending / `BEHIND` / `UNSTABLE`, nor on a
    transient `CLEAN → UNKNOWN → CLEAN` flap.
  - **AC1.3** If the PR is `CLEAN` but still **draft**, exactly one 🟡 "CI-ready but still DRAFT"
    line fires, naming `gss feature pr --ready` as text; it does **not** re-fire while it stays
    draft+CLEAN, and is superseded by the 🟢 ready line once the draft is flipped.
  - **AC1.4** `prping` **never** calls `gh pr ready` or `gss feature pr --ready` (notify-only,
    v1 — see §1.2(g)).
  - **AC1.5** Detection failure modes are first-class, tested paths (§1.2(d) scope=current):
    detached HEAD ⇒ exit 2 with guidance; branch-with-no-PR-yet ⇒ a grace counter
    (`--no-pr-grace`, default 3 polls) then exit 10; a branch mapping to **>1** open PR ⇒ refuse
    and warn (never silently watch the wrong one); a fork PR (`isCrossRepository`) ⇒ out-of-scope
    warn.
  - **AC1.6** Self-terminates (exit 10) when the watched PR merges/closes.

**UC2 — Remote-control, multi-goal watch (Claude juggling several PRs).**

- **Actor:** a developer with a **Remote-Control** session where Claude is concurrently advancing
  several PRs/goals.
- **Trigger:** starts `prping` with **`--scope all`** or **`--scope label:prping`** (mark PRs with
  `prping label add <#>`).
- **Flow:** one watcher tracks every in-flight PR. The watched set is built from **a single
  `gh pr list … --json …` call** (verified to return per-PR `mergeStateStatus` + inline
  `statusCheckRollup` for all rows), **not** a per-PR `gh pr view` fan-out. Each tick diffs the
  whole set and emits **at most one line per PR**. Because the agent is doing real work
  concurrently, pacing is critical: `prping tick --once` is a thin call (single `gh pr list`), and
  the loop re-arms the Monitor (~270s, re-armed past its ~1h cap) **between** ticks so the watch
  never blocks the main work. The watched set shrinks as PRs merge; the loop stops when empty.
- **Acceptance criteria:**
  - **AC2.1** Each PR independently triggers exactly **one** ready-to-merge push when it reaches
    `CLEAN` (per §8.3); no cross-PR spam, and lines are ordered by PR number, stable across ticks.
  - **AC2.2** A single `prping tick` over **N** PRs issues exactly **one** `gh pr list` call for
    the row set (call-count assertion is a golden test) — no N× `gh pr view` fan-out for the watch
    path.
  - **AC2.3** The tick returns promptly and never blocks; the loop, not the binary, owns cadence
    (`--once` enforces single-pass).
  - **AC2.4** The watched set shrinks as PRs merge/close; `tick` exits 10 and the loop stops when
    the set is empty.

**UC3 — Status table (one-shot, `prping status`).**

- **Actor:** a developer who asks "what's the state of all my PRs right now?".
- **Trigger:** **`prping status [--scope …] [--json]`** — **no loop, no notifications, no state
  file** (stateless; safe to run **concurrently** with a UC1/UC2 tick loop).
- **Flow:** prints an ASCII table, one row per PR in scope, with the columns in §1.2(f). Data
  path: **one `gh pr list --json …`** supplies the row set and most cells (number, title, body,
  `updatedAt`, `isDraft`, `mergeable`, `mergeStateStatus`, `closingIssuesReferences`), then **one
  `gh api graphql`** per **open non-draft** PR supplies the unanswered-thread count only (thread
  resolution is **GraphQL-only** — `gh pr view --json reviewThreads` is **rejected** with "Unknown
  JSON field", verified). `--json` emits the same structured rows for scripting.
- **Acceptance criteria:**
  - **AC3.1** Each row shows: **PR** (`#num title`), **Issue(s)**, **Description**, **Mergeable**,
    **Updated**, **Unanswered** (count of open review comments owing a reply) — per the column
    table in §1.2(f).
  - **AC3.2** "Unanswered" counts **only** review threads that are `isResolved == false` **and**
    whose last comment's author is neither the PR author nor a bot (`login` ends with `[bot]`).
    Resolved, outdated, author-self-reply, and bot-last threads are excluded (4-case must-not-count
    matrix is a golden test).
  - **AC3.3** Linked issues render as deduped, ascending `#NN` numbers (from
    `closingIssuesReferences` ∪ a body-regex scan of `close[sd]?|fix(e[sd])?|resolve[sd]?\s+#\d+`);
    an empty issue set renders `—`.
  - **AC3.4** A failed/partial `gh api graphql` call degrades the Unanswered cell to `?`
    (totality) — the command still exits 0 and renders the rest of the table.
  - **AC3.5** `--json` round-trips the structured rows; the table form truncates per-column without
    breaking alignment.
  - **AC3.6** `status` reads/writes **no** state file and emits **no** PushNotification.

### 1.2 Use-case spec deltas (normative — amends §3.3, §3.5, §8.3)

These deltas are the source of truth for the three use cases; where they touch an existing section
they **supersede** it.

**(a) `mergeStateStatus` enum fix — amends §3.3.** The enum is
**`CLEAN | BEHIND | BLOCKED | DIRTY | UNSTABLE | HAS_HOOKS | UNKNOWN`** (and the PR may
independently be `isDraft` — draft is the orthogonal `isDraft` boolean, **never** a
`mergeStateStatus` value; verified: PR #127 is `isDraft:true` **with** `mergeStateStatus:CLEAN`).
`UNSTABLE` (a non-required check failing/pending — **verified live on PR #127**) and `HAS_HOOKS`
were missing from §3.3 and are load-bearing: only **`CLEAN`** is ready, so any "unknown value ⇒
not-CLEAN" shortcut would silently never fire ready on an `UNSTABLE` PR. Snapshots over
`UNSTABLE`/`HAS_HOOKS`/missing fields are **seed-silent** (totality, §8.7). Also add `baseRefName`
to the snapshot schema (the ready line's `<base>`); `headSha` is sourced from `headRefOid`.

**(b) Ready predicate = CLEAN-only — restates §8.3.**
`ready ⇔ now.mergeStateStatus == "CLEAN" ∧ ¬now.isDraft ∧ prev.mergeStateStatus ∉ {CLEAN, UNKNOWN}`.
`CLEAN` subsumes "checks green AND up-to-date with base (not `BEHIND`)", so **no separate
git/rebase check** is needed. The previously-floated extras (`mergeable != CONFLICTING`,
`failingChecks == []`, `reviewDecision != CHANGES_REQUESTED`) are **redundant under CLEAN and are
dropped from the predicate** — they survive only as inputs to the §1.2(f) Mergeable cell.
**Caveat:** `BEHIND`/`CLEAN` are computed against the PR **base** (a stacked gss worker's parent,
not necessarily `main`); "the whole stack is rebased on `main`" is **out of scope v1** and the
ready line must not imply it.

**(c) New §8.3a — draft-CI-ready (one-shot).**
`now.mergeStateStatus == "CLEAN" ∧ now.isDraft ∧ (PR not already in draft-CI-ready state)` ⇒ one
`🟡 PR #N CI-ready but still DRAFT — run: gss feature pr --ready`. It never re-fires while the PR
stays draft+CLEAN, and is superseded by the §8.3 🟢 line once the draft is flipped. The suggested
command is **text, not an action** — see §1.2(g).

**(d) `tick` is the default command; scope semantics — amends §3.1.** Bare `prping` ≡
`prping tick` with the default scope: **`current`** when on a PR branch, else **`label:prping`**.
New flags: **`--once`** (single pass — the loop/Monitor owns cadence; required for the UC2
non-blocking contract) and **`--no-pr-grace N`** (default 3 — the UC1 no-PR-yet window).

`--scope current` is precisely "the PR for the branch checked out in `cwd`, via `gh pr view
--json …` with **no** PR argument" (verified resolves the current PR). Its four failure modes are
first-class, tested paths:

| Condition | Behavior |
| :-- | :-- |
| Detached HEAD ("could not determine current branch") | exit **2** + guidance |
| Branch has **no** PR yet | `NoPRForBranch` grace counter (default **3** polls, persisted in state) then exit **10** |
| Branch maps to **>1** open PR | **refuse + warn** (never silent first-match) |
| Fork PR (`isCrossRepository == true`) | out-of-scope-v1 **warn** |

The gss-registry (`Worker.worktree → pr_url`) fallback is **secondary**, gated on "no-arg
`gh pr view` failed **AND** a gss registry exists" — primary detection is branch-based via `gh`,
gss-registry-independent.

**(e) Snapshot schema — amends §3.3.** Add **`baseRefName`** (for the ready line's `<base>`);
`headSha` is sourced from `headRefOid`. Keep the schema **identity-minimal**: persist **only**
`number, title, branch, sha, state, isDraft, mergeStateStatus, mergeable, failingChecks,
baseRefName` — **no** author login, **no** comment bodies, **no** issue titles; `title` is the one
identity-adjacent field and is **sanitized** (see §1.2(h)). For `scope ∈ {all, label}` the snapshot
is built from **one** `gh pr list --state all --json number,title,headRefName,headRefOid,isDraft,mergeable,mergeStateStatus,statusCheckRollup,state,baseRefName` call (`+ --label` for label scope) — **verified** one call returns inline `statusCheckRollup`.

**(f) `status` table model + columns — amends/clarifies §3.5.** `status` uses its **own**
`internal/status` model (not `internal/snapshot`):

```go
type StatusReport struct { Repo, Scope, Generated string; Rows []StatusRow }
type StatusRow struct {
    Number       int
    Title        string
    LinkedIssues []int
    Description  string
    Mergeable    string   // single human token (table below)
    UpdatedAt    string   // relative, e.g. "2h ago"
    Unanswered   int       // -1 ⇒ rendered "?" (graphql failed)
}
```

| Column | Source |
| :-- | :-- |
| **PR** (`#num title`) | `gh pr list` row (title sanitized) |
| **Issue(s)** | `closingIssuesReferences[].number` ∪ body-regex `#NN` refs, deduped + sorted asc; `—` if empty |
| **Description** | first sentence of the PR `body`, sanitized + truncated |
| **Mergeable** | derived single token (Mergeable-cell table below) |
| **Updated** | `updatedAt`, relative |
| **Unanswered** | count of open review threads owing a reply (definition below); `?` on graphql failure |

**Mergeable-cell derivation (one token per cell):** `DRAFT` if `isDraft`; `MERGED`/`CLOSED` from
`state`; `CONFLICTING` if `mergeable == CONFLICTING`; else map `mergeStateStatus`:
`CLEAN→READY`, `BEHIND→BEHIND`, `BLOCKED→BLOCKED`, `DIRTY→CONFLICTING`, `UNSTABLE→UNSTABLE`,
`HAS_HOOKS→CHECKING`, `UNKNOWN→CHECKING`. (This is the **only** place the `mergeable != CONFLICTING`
/ `failingChecks` inputs live — they inform the cell, **not** the §8.3 ready predicate.)

**Unanswered definition (v1, load-bearing):** `count = | { thread : thread.isResolved == false ∧
last(thread.comments).author.login ≠ prAuthor.login ∧ ¬last(...).author.login.endsWith("[bot]") }
|`. **Excludes** resolved threads, author-self-reply threads, bot-last threads, and (v1) all
top-level non-review issue comments. Source: **`gh api graphql`** per **open non-draft** PR (a
draft has no reviewers owing a reply) with the **verified-working** query shape —
`reviewThreads()` takes a `first:` connection argument, **not** an `isResolved:` filter (that is a
per-node field, filtered client-side):

```graphql
pullRequest(number: $n) {
  reviewThreads(first: 100) {
    nodes { isResolved isOutdated comments(last: 1) { nodes { author { login } } } }
  }
}
```

The richer "PLUS dangling top-level issue-comments" arm is **deferred** (disjoint data source,
noisier, double-count risk). The `first:100` cap is documented; a failed/partial call degrades the
cell to `?` (`Unanswered = -1`), the command still exits 0.

**Linked-issue display:** numbers-only (`#NN`) in v1 — `closingIssuesReferences` carries **no
title** (verified keys: `number`, `url`, `repository`); titles would cost an extra `gh issue view`
per issue and are deferred behind a `--issue-titles` flag.

**(g) Notify-only policy (v1) — amends §10/§12.** `prping` **never** calls `gh pr ready` or
`gss feature pr --ready` (the latter is approval-token-gated and must stay gss/human-owned). The
🟢 (§8.3) and 🟡 (§8.3a) lines carry any suggested command as **text only**. An opt-in
`--offer-ready` / `--mark-ready` is **deferred**.

**(h) Security & Privacy (new section).**

- **privacy_guard gap (verified).** `ai/hooks/privacy_guard.sh` Rule C keys on the **local**
  `$USER`/`$HOSTNAME` (`ME_USER`/`ME_HOST`) — it does **not** catch the GitHub committer login
  `sfc-gh-eraigosa`, the git author name, or an email. (That login is already embedded in
  `sdk/gss`'s `internal/gh/testdata/gh_responses/*.json` fixtures — verified — so the hook is not a
  backstop here.) `prping` therefore owns its **own** identity hygiene.
- **Mandatory shared `internal/sanitize` helper** applied to **every** gh-sourced string in **both**
  `diff` and `status` (titles, descriptions, branch names, check names): strip C0/C1 control bytes
  and ANSI CSI (`\x1b\[[0-9;]*[A-Za-z]`), collapse newlines to a space, truncate (`<200` chars /
  per-column), emit no markdown. This also closes a **terminal-escape / event-line-injection**
  vector (a hostile PR title reaching the agent's `PushNotification` line or the table).
- **State file is identity-minimal** (§1.2(e)) and **never committed** (`~/.config/prping/…`,
  gitignored); `status` is **non-persisting**.
- **`gh` token scope.** Status/unanswered detection needs PRs + issues + comments read (classic
  `repo`, or fine-grained `Contents`/`PullRequests`/`Issues: read`). The skill runs `gh auth
  status` on start and **fails closed** with an actionable message rather than silently rendering an
  empty Unanswered column.
- **Fixture hygiene.** prping fixtures use **synthetic identifiers only** (repo `acme/widgets`,
  logins `octocat`/`reviewer-bob`, branch `feature/x`, fake SHAs). A Go **`TestFixturesNoIdentity`**
  substring-lint (case-insensitive, **stricter** than Rule C's word-boundary) **fails** on a
  forbidden-login list (seeded with `sfc-gh-eraigosa`), on `/home/`–/`/Users/`–style home paths, or
  a real `@`-email. **Do not** copy `sdk/gss`'s real-login PR-URL fixtures into prping.

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
      "baseRefName": "main",
      "state": "OPEN|MERGED|CLOSED", "isDraft": false,
      "mergeStateStatus": "CLEAN|BEHIND|BLOCKED|DIRTY|UNSTABLE|HAS_HOOKS|UNKNOWN",
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

A read-only sibling to `tick` that uses its **own** `internal/status` model (`StatusReport` /
`StatusRow` — see §1.2(f)), **not** `internal/snapshot`. No state file, no notifications;
stateless and safe to run concurrently with a tick loop.

**Data path (normative — supersedes any per-PR `gh pr view` fan-out):** **one**
`gh pr list --json number,title,body,updatedAt,isDraft,mergeable,mergeStateStatus,closingIssuesReferences,state`
supplies the row set and every cell except Unanswered; then **one** `gh api graphql` per **open
non-draft** PR supplies the unanswered count. Thread resolution is **GraphQL-only** —
`gh pr view --json reviewThreads` is rejected with "Unknown JSON field" (verified), so the
graphql runner verb is mandatory. The full column table, the Mergeable-cell derivation, the
unanswered definition, and the **verified-working** GraphQL query (`reviewThreads(first:100){
nodes { isResolved isOutdated comments(last:1){ nodes { author { login } } } } }` — note
`reviewThreads()` takes `first:`, **not** an `isResolved:` filter arg; `isResolved` is filtered
client-side) are specified in **§1.2(f)** — that section is the normative source for this command.

Like `diff`, `internal/status` rendering is a **pure function** of the fetched data → table-tested
with the fake `gh.Runner`. Rendering: fixed-width ASCII columns with per-cell truncation; `--json`
emits the structured rows (same data) for scripting. Scope follows §3.1
(`current`/`all`/`label:<name>`).

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
### 8.3 Ready to merge — `now.mergeStateStatus==CLEAN ∧ ¬isDraft ∧ prev∉{CLEAN,UNKNOWN}` ⇒ one `🟢 ready`; not when already CLEAN, draft, or recovering from a transient `UNKNOWN` flap. `CLEAN` subsumes "checks green AND not `BEHIND` (up-to-date with **base**)" — no separate git/rebase check; `UNSTABLE`/`HAS_HOOKS`/`BEHIND`/`BLOCKED`/`DIRTY`/`UNKNOWN` are never ready. **Caveat:** base ≠ `main` for a stacked gss worker — whole-stack-on-`main` is out of scope v1 (see §1.2(b)).
### 8.3a Draft CI-ready (one-shot) — `now.mergeStateStatus==CLEAN ∧ now.isDraft ∧ ¬already-draft-CI-ready` ⇒ one `🟡 PR #N CI-ready but still DRAFT — run: gss feature pr --ready` (suggested command is **text, not an action** — notify-only, §1.2(g)); never re-fires while it stays draft+CLEAN; superseded by the §8.3 🟢 line once the draft is flipped to ready.
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
