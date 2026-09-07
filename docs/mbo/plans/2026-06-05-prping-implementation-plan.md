# prping — implementation plan (Go CLI)

- **Slug:** prping
- **Date:** 2026-06-06
- **Status:** Draft
- **Relates to:** spec [`../specs/2026-06-05-prping-design.md`](../specs/2026-06-05-prping-design.md) · PR #127
- **Supersedes:** the earlier bash-script plan that lived at this same path (git history), AND the
  first Go-CLI draft of this plan — replaced by this revision, which folds in the three
  first-class use cases (UC1/UC2/UC3) as named acceptance scenarios and the verified
  ground-truth corrections (UNSTABLE enum, GraphQL-only `reviewThreads`, the shared
  `internal/sanitize` package, the NOTIFY-ONLY posture, and the fixture-identity lint).

## 1. Summary & verdict

Build **prping** as a first-class sdk Go module at `sdk/prping`, mirroring `sdk/gss` exactly
(cobra `cmd/`, `internal/` packages, a mockable `gh.Runner` seam, `internal/version` ldflags,
`build.sh`, coverage gate). The deterministic core is the binary; the agent skill is a thin
print→`PushNotification` relay. The load-bearing decision surface — which events fire on a
`(prev, now)` snapshot transition — lives in a **pure** `internal/diff` package, golden-tested
1:1 against spec §8.1–8.8 (plus the new §8.3a draft-ready rule). `prping status` (UC3) adds a
**separate, stateless** `internal/status` model + renderer, NOT a reuse of `internal/snapshot`.

**Verdict: approved to build.** Carried forward: the spec §8→test traceability discipline and
the privacy/fixture-attribution discipline. **Folded in this revision** (verified against
`gh 2.92.0` on this repo, PR #127):

1. **READY = CLEAN-only.** `ready ⇔ mergeStateStatus=="CLEAN" ∧ ¬isDraft ∧ prev∉{CLEAN,UNKNOWN}`.
   CLEAN already subsumes "checks green AND not BEHIND (up-to-date with base)" — no extra git
   check. The live PR #127 returns `mergeStateStatus:"UNSTABLE"` (a value **missing** from the
   spec §3.3 enum), so the enum gains `UNSTABLE` + `HAS_HOOKS` and **only CLEAN is ready**. Draft
   is the orthogonal `isDraft` boolean — **never** a `mergeStateStatus` value (verified: #127 is
   `isDraft:true` WITH `mergeStateStatus:CLEAN`).
2. **NOTIFY-ONLY.** prping never calls `gh pr ready` or `gss feature pr --ready` (the latter is
   approval-token-gated and stays gss/human-owned). A CLEAN-but-draft PR fires a distinct one-shot
   `🟡` line that carries the suggested command as **text**, not an action (new rule §8.3a).
3. **`status` is its own model, its own data path.** `gh pr view --json reviewThreads` is
   **rejected** ("Unknown JSON field" — verified), so unanswered-thread detection requires a new
   `gh api graphql` runner verb the snapshot path lacks. `status` = ONE `gh pr list --json …`
   (rows + linked issues + mergeable) + ONE `gh api graphql` per OPEN non-draft PR (unanswered
   only). It uses `internal/status`'s own `StatusReport`/`StatusRow`, is **stateless** (no state
   file, no events, no `PushNotification`), and is safe to run concurrently with a `tick` loop.
4. **Shared `internal/sanitize`.** A mandatory helper routes EVERY gh-sourced string in both
   `diff` and `status` (strip C0/C1 + ANSI CSI, collapse newlines, truncate, no markdown). This
   closes a terminal-escape / event-line-injection vector AND is the only backstop for GitHub
   logins — `ai/hooks/privacy_guard.sh` Rule C keys on the local `$USER`/`$HOSTNAME`, NOT the
   GitHub committer login `sfc-gh-eraigosa` (verified). A `TestFixturesNoIdentity` substring-lint
   enforces synthetic-only fixtures.

Ground truth verified against `sdk/gss` (`cmd/`, `internal/gh/{exec.go,fake/client.go}`,
`internal/version`, `build.sh`, `go.mod`), `install.sh` (gss block 356–365; sdk build sequence
gss→tmux-mgr→wol→gsl, gsl ends at line 400), `scripts/test.sh` (COVERAGE_MIN 41–46; module
discovery globs `sdk/*/go.mod`), `.golangci.yml` (module-count comment 9–10), `sdk/GEMINI.md`
(module table), `.gitignore` (`!sdk/` + `!sdk/**` at lines 42–43 already opt in `sdk/prping/**`),
and `sdk/gss/internal/registry/schema.go` (Worker.{Branch,Worktree,PRURL,PRState}, mutable
`SchemaVersion`).

---

## 2. The three use cases as named acceptance scenarios

Each scenario names its actor / trigger / flow / **pass criteria**, the exact CLI surface + data +
events that implement it, and the §4 phase + §5 tests that prove it. These ARE the acceptance
scenarios the plan is judged against.

### UC1 — Draft-worktree auto-watch (scope=current)

- **Actor / trigger:** a developer inside a `gss feature` draft-PR worktree runs bare `prping`
  ("watch this until it's ready"). Bare `prping` ≡ `prping tick` with the default scope; on a PR
  branch that resolves to `scope=current`.
- **Flow:** each tick auto-detects the current PR via `gh pr view --json …` with **no PR number**
  (verified: resolves the checked-out branch's PR, #127). Build snapshot → load prev → `diff` →
  persist → print. Ready fires per §8.3 (CLEAN-only). A CLEAN-but-still-draft PR fires the §8.3a
  `🟡` one-shot. prping NOTIFIES only — it never flips the draft.
- **CLI surface:** `prping` (bare) / `prping tick --scope current [--repo r] [--state-dir d]
  [--once] [--no-pr-grace N]`.
- **Data / events:** snapshot for one PR (+ `baseRefName` for the ready line's `<base>`);
  `🟢 ready`, `🟡 draft-CI-ready`, `✓ pushed`, `🔄 needs update`, `❌ check failed`, `✅/🚪`,
  `🏁`.
- **Pass criteria (→ tests):**
  - **AC1-detect** — in a worktree whose branch has an open PR, bare `prping` resolves and watches
    exactly that #N (no PR number passed to `gh pr view`). → `cmd/tick_test.go::detect_current_pr`.
  - **AC1-ready** — `prev∈{BEHIND,BLOCKED,DIRTY}` → `now==CLEAN ∧ ¬draft` emits exactly one
    `🟢 PR #N ready to merge (checks green, up to date with <base>)`. → `diff_test.go::ready_behind_to_clean`.
  - **AC1-must-not-fire** — ready does NOT fire when already CLEAN, when `isDraft`, when
    `UNSTABLE`, or on a CLEAN→UNKNOWN→CLEAN flap. `BEHIND` emits `🔄`, not ready. →
    `ready_clean_to_clean`, `ready_draft`, `ready_unstable`, `ready_unknown_flap`, `behind_green`.
  - **AC1-draft-ready** — `now==CLEAN ∧ isDraft ∧ not-already-draft-ready` emits one
    `🟡 PR #N CI-ready but still DRAFT — run: gss feature pr --ready`; never re-fires while it
    stays draft+CLEAN. → `diff_test.go::draft_ci_ready_once`.
  - **AC1-notify-only** — no code path emits a `pr ready` argv to the runner. →
    `cmd/tick_test.go::no_mutating_verb` (asserts the fake runner's call log).
  - **AC1-failure-modes** — detached HEAD ⇒ exit 2 + "could not determine current branch";
    branch-with-no-PR ⇒ NoPRForBranch grace (default 3 polls, persisted) then exit 10;
    multiple-PRs-for-branch ⇒ refuse + warn (never silent first-match); fork
    (`isCrossRepository==true`) ⇒ out-of-scope-v1 warn. → `cmd/tick_test.go::{detached_head,
    no_pr_grace,multiple_prs_refuse,fork_warn}`.
  - **AC1-self-terminate** — watched PR OPEN→MERGED ⇒ `✅` then exit 10. →
    `cmd/tick_test.go::exit10_empty_set`.

### UC2 — Remote-control, multi-goal watch (scope=all | label:prping)

- **Actor / trigger:** a developer in a Remote-Control session where Claude drives several PRs.
  Starts `prping --scope all` or `--scope label:prping`. The skill arms a non-blocking `Monitor`
  poll loop that calls `prping tick --once` (~270s) and relays each printed line — the watch must
  not block the agent's concurrent work.
- **Flow:** each tick builds the WHOLE watched set in **one** `gh pr list --json …` call (not N
  per-PR `gh pr view`), diffs, persists-before-print, prints ≤1 line per transitioned PR, ordered
  by PR number. The watcher is read-only (`gh pr list/view`, `git`) so it cannot race the agent's
  mutating work.
- **CLI surface:** `prping tick --scope all|label:<name> --repo r --once [--state-dir d]`;
  `prping label add|remove <#…> [--label prping]` to enroll a goal's PR mid-loop.
- **Pass criteria (→ tests):**
  - **AC2-single-call** — for N watched PRs one tick issues exactly ONE `gh pr list` call (assert
    runner call-count==1 for N=10) — proves no per-PR fan-out / no agent starvation. →
    `snapshot_test.go::scope_all_one_list_call`.
  - **AC2-per-pr-ready** — given 3 labeled PRs, when #A→CLEAN and #B/#C stay BEHIND, the tick
    prints exactly one `🟢 PR #A …` and nothing for #B/#C. → `diff_test.go::multi_pr_one_ready`.
  - **AC2-dedup** — #A already CLEAN last tick prints no line this tick; a CLEAN→UNKNOWN→CLEAN flap
    fires ready at most once. → `ready_clean_to_clean`, `ready_unknown_flap`.
  - **AC2-order-stable** — lines for a multi-PR tick are ordered by ascending PR number and
    byte-stable across runs given identical (prev,now). → `diff_test.go::order_multi_pr_one_tick`.
  - **AC2-enroll** — `prping label add 130` ⇒ the next tick includes #130 and emits its `📬 opened`
    first-sight line without restarting the loop. → `cmd/label_test.go` + lifecycle fixture.
  - **AC2-self-terminate** — all watched PRs MERGED/CLOSED ⇒ exit 10 ⇒ Monitor loop breaks. →
    `cmd/tick_test.go::exit10_empty_set`.

### UC3 — Status table (one-shot, `prping status`)

- **Actor / trigger:** a developer asks "what's the state of my PRs?" ⇒ the skill runs
  `prping status` once and prints the table verbatim (NO loop, NO `PushNotification`).
- **Flow:** resolve scope (default = current-branch PR if on one, else `label:prping`). ONE
  `gh pr list --json number,title,body,updatedAt,isDraft,mergeable,mergeStateStatus,
  closingIssuesReferences[,labels]` for the row set + most cells; then ONE `gh api graphql`
  `reviewThreads(first:100)` per OPEN non-draft PR for the Unanswered cell ONLY. Classify the
  mergeable cell, count unanswered threads, derive linked issues, render an ASCII table or
  `--json`. Exit 0 (incl. empty); exit non-zero only on a hard `gh`/auth failure.
- **CLI surface:** `prping status [--scope <s>] [--repo r] [--json] [--no-color] [--limit N]`.
  Stateless: no `--state-dir`, no `--dry-run`.
- **Columns (§3.5):** `PR (#num title)` | `Issue(s)` | `Description` | `Mergeable` | `Updated` |
  `Unanswered`.
- **Pass criteria (→ tests):**
  - **AC3-empty** — no PRs in scope ⇒ header + "no PRs match scope <s>" + exit 0 (no panic). →
    `status_test.go::empty_scope`.
  - **AC3-issue-union** — issues = `closingIssuesReferences[].number` ∪ body-regex refs
    (`(?i)(close[sd]?|fix(e[sd])?|resolve[sd]?)\s+#(\d+)`), deduped + sorted; a `Fixes #7` body
    ref absent from `closingIssuesReferences` still appears; empty set renders `—`. →
    `status_test.go::{issue_union_dedupe,issue_body_only,issue_none}`.
  - **AC3-unanswered-matrix** — count = unresolved review threads whose LAST comment author ≠ PR
    author ≠ bot (`login` endsWith `[bot]`). Four must-NOT-count cases: resolved, outdated,
    bot-last, author-self-reply. → `status_test.go::unanswered_{counts,resolved,outdated,bot,self}`.
  - **AC3-graphql-degrade** — a failed/partial `gh api graphql` degrades the Unanswered cell to
    `?` and the command still exits 0. → `status_test.go::graphql_failure_totality`.
  - **AC3-mergeable-cell** — one human token per cell per the §3.5 derivation
    (DRAFT|READY|BEHIND|BLOCKED|CONFLICTING|UNSTABLE|CHECKING|MERGED|CLOSED). →
    `status_test.go::mergeable_cell_matrix`.
  - **AC3-render** — fixed-width ASCII, per-column truncation + ellipsis, relative `Updated`
    (deterministic given an injected `now`), rows sorted by number, no ANSI by default (pipe-safe),
    byte-identical output for identical input. → `status_test.go::render_golden`.
  - **AC3-json** — `--json` prints `StatusReport` (rows sorted), NO table, round-trips through
    `encoding/json`. → `status_test.go::json_roundtrip`.
  - **AC3-stateless** — `prping status` writes nothing under `--state-dir`/`~/.config/prping`. →
    `cmd/status_test.go::no_state_write`.

---

## 3. File inventory

All paths absolute-from-repo-root `sdk/prping/` unless noted. Every artifact maps to a spec
section / use case.

### Inside `sdk/prping/` (the module)

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `main.go` | `package main` → `cmd.Execute()` (mirrors gss `main.go`). | §3.2 |
| `cmd/root.go` | cobra root `prping`; persistent flags `--scope`, `--repo`; **`tick` is the default command** (bare `prping` ≡ `prping tick`); default-scope resolution (`current` on a PR branch else `label:prping`). | §3.1 |
| `cmd/snapshot.go` | `prping snapshot --scope <s> [--repo r]` → snapshot JSON on stdout. | §3.1, §3.3 |
| `cmd/diff.go` | `prping diff <prev.json> <now.json>` → event lines (pure, no runner). | §3.1, §4, §8 |
| `cmd/tick.go` | `prping tick --scope <s> [--repo r] [--state-dir d] [--dry-run] [--once] [--no-pr-grace N]` → load prev → snapshot → diff → persist → print; **exit 10 = watched set empty**; UC1 current-PR detect + failure modes. | §3.1, §3.4, §5, UC1, UC2 |
| `cmd/label.go` | `prping label add\|remove <#…> [--label prping]` (`gh pr edit --add-label/--remove-label`). | §3.1, §12.2, UC2 |
| `cmd/status.go` | `prping status --scope <s> [--repo r] [--json] [--no-color] [--limit N]` → UC3 table / JSON; **no state, no notifications**. | §3.1, §3.5, UC3 |
| `cmd/version.go` | ldflags version block + `--json` (mirrors gss). | §7 |
| `cmd/*_test.go` | one `_test.go` per command; `cmd/tick_test.go` is the §9.4 in-process lifecycle + UC1 detect-failure-mode integration test. | §9.4, UC1 |
| `internal/gh/exec.go` | `Runner` interface + `SystemRunner` (real `gh`/`git` via os/exec seam, **stdout-only + stderr folded into `*ExecError` on non-zero**); the testability seam. **Template: gss `internal/git`** (copy its name-taking `Run` signature; diverge only on stdout-vs-combined output — see §4 one-runner-contract). | §3.2 |
| `internal/gh/fake/runner.go` | scriptable fake `Runner` (FIFO-Script pattern: `Script []Response`, `Calls []CallRecord`, `Default Response`, `Reset()`) from `sdk/gss/internal/git/fake`. | §3.2, §9.2, §9.4 |
| `internal/gh/exec_test.go`, `internal/gh/fake/runner_test.go` | seam argv-assertion tests (exact argv, stdout-only, `ExecError`-on-nonzero) + fake self-tests; includes a `gh api graphql` argv case. | §9.2 |
| `internal/sanitize/sanitize.go` | **NEW, BUILD-FIRST.** `Clean(s string, max int) string`: strip C0/C1 control + ANSI CSI (`\x1b\[[0-9;]*[A-Za-z]`), collapse newlines, truncate to `max`, no markdown. Shared by `diff` AND `status`. | §Security |
| `internal/sanitize/sanitize_test.go` | golden: `\n`+ANSI+NUL ⇒ one clean line; truncation; idempotence. | §9.1, §Security |
| `internal/snapshot/snapshot.go` | **Owns `Snapshot`/`PR`/`Scope`.** `Build(ctx, runner, Scope) (Snapshot, error)`: ONE `gh pr list … --state all --json` for `all`/`label`; `gh pr view` (no number) for `current`. Schema-closed (+ `baseRefName`). | §3.2, §3.3, §8.6, UC1, UC2 |
| `internal/snapshot/snapshot_test.go` | table tests w/ fake runner; single-`gh pr list`-call assertion; per-status incl. `UNSTABLE`; missing-field totality; current-PR detect + the four failure modes. | §9.2, UC1, UC2 |
| `internal/diff/diff.go` | **PURE** — owns `Event`. `Diff(prev, now snapshot.Snapshot) []Event` + `Event.Line()` (via `sanitize`, <200 chars, ordered). The heart. Imports `internal/snapshot` + `internal/sanitize`, **never `internal/gh`**. | §4, §8.1–8.8, §8.3a |
| `internal/diff/diff_test.go` | golden table tests: one+ named case per §8 rule (incl. §8.3a, `UNSTABLE` must-not-fire) + invariants + totality + flap guards + a coverage assertion. | §8, §9.1 |
| `internal/diff/testdata/golden/*.txt` | golden event-line transcripts (synthetic identifiers only). | §9.1 |
| `internal/status/status.go` | **Owns `StatusReport`/`StatusRow`.** `Build(ctx, runner, Scope) (StatusReport, error)`. | §3.5, UC3 |
| `internal/status/classify.go` | mergeable-cell derivation (one human token per cell). | §3.5, UC3 |
| `internal/status/issues.go` | linked-issue union (`closingIssuesReferences` ∪ body regex), dedupe+sort. | §3.6, UC3 |
| `internal/status/threads.go` | `gh api graphql` `reviewThreads(first:100)` per OPEN non-draft PR + unanswered count; degrades to `?` on failure. | §3.5, UC3 |
| `internal/status/render.go` | ASCII table (per-column truncation via `sanitize`, relative `Updated`, sorted) + `--json`. | §3.5, UC3 |
| `internal/status/*_test.go` | table tests: issue union, unanswered 4-case matrix, mergeable-cell matrix, render golden, json round-trip, graphql-failure totality. | §9 (UC3) |
| `internal/status/testdata/**` | synthetic graphql + `gh pr list` fixtures (octocat/reviewer-bob, acme/widgets). | §9, §Security |
| `internal/state/state.go` | `Load(dir, repo)` / `Persist(dir, repo, Snapshot)`: atomic temp+rename, umask 077 (0700 dir / 0600 file), per-repo lock, filename derivation, seed-silent; persists the `NoPRForBranch` grace counter. | §3.3, §5, §12.1, UC1 |
| `internal/state/state_test.go` | atomic write, perms, lock, filename derivation/collision, seed-silent, grace-counter persistence across restart. | §9.3 |
| `internal/version/version.go` + `_test.go` | ldflags vars + `Get()` fallbacks (verbatim shape from gss). | §7 |
| `build.sh` | version-stamping build → `~/opt/bin/prping` (mirrors gss build.sh, swap `gss`→`prping`). | §7 |
| `VERSION` | `0.1.0`. | §7 |
| `LICENSE` | Apache-2.0 (copy gss). | §7 |
| `README.md` | tool README. | §7 |
| `go.mod` / `go.sum` | `module github.com/sfc-gh-eraigosa/dotfiles/sdk/prping`; `go 1.26.1`; cobra dep. | §7 |
| `skill/SKILL.md` | the agent skill: UC1/UC2 loop driver (relay each `tick` line via `PushNotification`, Monitor `--once` paced), UC3 manual `status` intent (run once, print verbatim, NO loop/push), prereq checks (§6), §9.7 manual-acceptance checklist; trigger `description` tuned for §9.6. | §3, §5, §6, §9.6, §9.7 |
| `fixtures_lint_test.go` (package `prping_test` at module root or `internal/fixturecheck`) | `TestFixturesNoIdentity`: walks `sdk/prping/**/testdata/**`, FAILS on a FORBIDDEN-login list (seeded `sfc-gh-eraigosa`), `/home/`÷`/Users/` paths, or a real `@`-email (case-insensitive substring — stricter than privacy_guard Rule C). | §Security |
| `GEMINI.md` + `CLAUDE.md` (symlink → `GEMINI.md`) | per-dir docs (repo convention). | §7 |

### Touch-points OUTSIDE `sdk/prping/` (verified)

| Path | Change | Anchor (exact insertion point) | Implements |
| :-- | :-- | :-- | :-- |
| `install.sh` | Add a build+install block mirroring the gss block. | **After the gsl block — the last sdk build block — ending at line 400** (closing `fi`), **before the Nerd-Font comment**. Same shape as gss block (lines 356–365) with `gss`→`prping`. (Sequence: gss→tmux-mgr(378)→wol(389)→gsl(400); gsl is last.) | §7 |
| `scripts/test.sh` | Add `[prping]=70` to `COVERAGE_MIN`. | Inside `declare -A COVERAGE_MIN=( … )`, after `[wol]=60` (line 45). Module discovery already globs `sdk/*/go.mod` — **no discovery edit**. | §7, §9.5 |
| `sdk/GEMINI.md` | Add a module-table row after `tmux-mgr`. | `| [`prping/`](./prping/GEMINI.md) | `…/sdk/prping` | `prping` | Claude-native PR/push notifier + status table. |` | §7 |
| `.golangci.yml` | Update the module-list comment (lines 9–10): "FOUR separate Go modules: sdk/gss, sdk/tmux-mgr, sdk/gsl, sdk/wol" → "FIVE … : sdk/gss, sdk/tmux-mgr, sdk/gsl, sdk/wol, **sdk/prping**" (add to the by-name list, not just bump the numeral). Comment-only; `lint-go` auto-discovers. | lines 9–10 | §7 |
| `sync-skills` | **No code change.** `sdk/prping/skill/SKILL.md` auto-discovered (dual-scans `src/`+`sdk/`) and linked as bare `prping`. Verify by running it. | n/a | §7 |
| `~/.config/prping/` | Runtime state dir, created 0700 by `internal/state` at first run; outside the repo; never committed. | n/a | §3.3, §11 |

**`.gitignore`: no source change needed.** `!sdk/` + `!sdk/**` (lines 42–43) already opt in the
entire `sdk/prping/**` tree (verified: `git check-ignore -v sdk/prping/main.go` → `.gitignore:43`).
The built binary `sdk/prping/prping` and the `~/.config/prping/` state dir must be excluded: add
`sdk/prping/prping` to `.gitignore` **with an inline comment** (mirror the existing
`sdk/tmux-mgr/tmux-mgr` entry at line 77); the state dir lives in `$HOME`, outside the repo.
**Confirm** with `git status --short -- sdk/prping` after scaffolding (do NOT `git add -f`).

---

## 4. Interface contracts

### Naming caveat — `internal/gh` is the gss `internal/git` analog (NOT gss `internal/gh`)

prping's `internal/gh` is a **thin subprocess seam** with a single
`Run(ctx, name string, args ...string) ([]byte, error)` method covering **both** the `gh` and
`git` binaries — exactly the shape of gss's **`internal/git`** (gss `internal/git` is the **only**
gss runner whose `Run` takes a *binary name* arg, which prping needs to dispatch to both `gh` and
`git`). It is NOT gss's domain-rich `internal/gh.Client`. Copy the gss **`internal/git`** package
as the template. The single seam matters because UC3's unanswered-thread fetch is a
`gh api graphql …` argv that flows through the same `Run` — there is no separate GraphQL client.

**One runner contract (resolves the stdout-vs-combined ambiguity).** Copy gss `internal/git`'s
**signature** (`Run(ctx, name, args...)`), but adapt its output handling: prping's `SystemRunner`
returns **stdout only** on success, and on a non-zero exit folds stderr into an
`*ExecError{Args, Stderr, Err}` (so JSON-parsing callers always get clean stdout, never
stderr-polluted bytes). gss `internal/git.SystemRunner` returns *combined* stdout+stderr
(`exec.go` lines 46-47, 84) — prping deliberately diverges on that single point and **nowhere
references gss `internal/gh`'s contract**. This is the only runner contract in prping; §3 and §8.1
defer to it.

### `internal/gh.Runner` (the testability seam)
```go
type Runner interface {
    Run(ctx context.Context, name string, args ...string) ([]byte, error)
}
```
`name` is `gh` or `git`. **`SystemRunner` returns stdout only**; on non-zero exit it returns
`*ExecError{Args, Stderr, Err}` (stderr folded in) — see the one-runner-contract note above
(prping copies gss `internal/git`'s name-taking signature but returns clean stdout instead of
gss `internal/git`'s combined output). The `reviewThreads` fetch is
`Run(ctx, "gh", "api", "graphql", "-f", "query=…", "-F", …)`.

### `internal/gh/fake.Runner` (FIFO-Script fake)
```go
type Response struct { Stdout, Stderr []byte; Err error }
type CallRecord struct { Name string; Args []string }
type Runner struct { Script []Response; Calls []CallRecord; Default Response }
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, error) // pops Script head, records call, returns Stdout only
func (r *Runner) Reset()
var _ gh.Runner = (*fake.Runner)(nil)
```
The recorded `Calls` log is what AC2-single-call and AC1-notify-only assert against.

### Go types (homed in their owning packages — `internal/diff` never imports `internal/gh`)
```go
// internal/snapshot — Scope is the watched-set selector (§3.1).
type Scope struct { Kind ScopeKind; Ref string } // current | all | label  (+ Ref for #N / label name)

// internal/snapshot — Snapshot is schema-closed (§3.3). Marshalled to the state file verbatim.
type Snapshot struct {
    Repo  string `json:"repo"`
    Scope string `json:"scope"`
    PRs   []PR   `json:"prs"`
}
type PR struct {
    Number           int      `json:"number"`
    Title            string   `json:"title"`            // sanitized; the one identity-adjacent persisted field
    Branch           string   `json:"branch"`
    BaseRef          string   `json:"baseRef"`          // baseRefName — for the ready line's <base>
    HeadSha          string   `json:"headSha"`          // sourced from headRefOid
    State            string   `json:"state"`            // OPEN|MERGED|CLOSED
    IsDraft          bool     `json:"isDraft"`
    MergeStateStatus string   `json:"mergeStateStatus"` // CLEAN|BEHIND|BLOCKED|DIRTY|UNSTABLE|HAS_HOOKS|UNKNOWN — NB: draft is the orthogonal IsDraft bool, NEVER a mergeStateStatus value (verified: #127 is isDraft:true WITH mergeStateStatus:CLEAN)
    Mergeable        string   `json:"mergeable"`        // MERGEABLE|CONFLICTING|UNKNOWN
    FailingChecks    []string `json:"failingChecks"`
}
// NB: no author login, no comment body, no issue title persisted — identity-minimal.

// internal/diff — Event is one diff result (§4). Kind drives the glyph/template.
type Event struct { PRNumber int; Kind EventKind; Title, Branch, Base, Sha, Status, Detail string }
func (e Event) Line() string // routes every gh-sourced field through sanitize; single line, <200 chars, no markdown

// internal/status — its OWN model (NOT internal/snapshot). Stateless; never serialized.
type StatusReport struct { Repo, Scope string; Generated time.Time; Rows []StatusRow }
type StatusRow struct {
    Number       int
    Title        string    // sanitized
    LinkedIssues []int     // closingIssuesReferences ∪ body refs, deduped+sorted
    Description  string    // first non-empty body line, sanitized
    Mergeable    string    // derived cell token
    UpdatedAt    time.Time
    Unanswered   int       // -1 / "?" sentinel when graphql failed
}
```
**Import directions (acyclic):** `internal/sanitize` imports nothing. `internal/snapshot` imports
`internal/gh` + `internal/sanitize`. `internal/diff` imports `internal/snapshot` +
`internal/sanitize` — **not** `internal/gh`. `internal/status` imports `internal/gh` +
`internal/sanitize` (its own model — **not** `internal/snapshot`). `internal/state` imports only
`internal/snapshot`.

### cobra command signatures (each in `cmd/`, `RunE` returning error; exit via `os.Exit` for the 10/2 cases)
```
(root):    tick is the default command — bare `prping` runs tick with the resolved default scope
snapshot:  RunE → snapshot.Build(ctx, gh.NewSystemRunner(), scope); json.Encode(snap)
diff:      RunE → read prev,now; diff.Diff(prev, now); print each Event.Line()   (NO runner)
tick:      RunE → resolveScope (UC1 detect for current) → state.Load → snapshot.Build →
                  diff.Diff → state.Persist(BEFORE print) → print; empty set ⇒ "🏁" + os.Exit(10);
                  detached/no-PR/multiple/fork ⇒ typed exit (2 / 10-after-grace / refuse); --once = single pass
label:     RunE → for each #N: gh pr edit N --add-label|--remove-label <label>
status:    RunE → status.Build(ctx, runner, scope) → fmt.Print(report.Render(jsonFlag, noColor))   (NO state)
version:   Run  → version.Get() block (+ --json)
```

### Pure / near-pure signatures
```go
func sanitize.Clean(s string, max int) string                          // total, no panic
func diff.Diff(prev, now snapshot.Snapshot) []Event                    // total: malformed/UNKNOWN/UNSTABLE ⇒ no panic, no spurious ready
func status.Build(ctx, r gh.Runner, s snapshot.Scope) (StatusReport, error) // graphql failure ⇒ Unanswered=-1, no error
func (r StatusReport) Render(asJSON, noColor bool) string
```

### `tick` orchestration pseudocode (the prev→now contract)
```
repo  := resolveRepo(--repo)                       // gh repo view nameWithOwner if empty
scope := resolveScope(--scope)                     // UC1: current ⇒ gh pr view (no #) ; detached/no-PR/multiple/fork handled here
prev, seeded := state.Load(stateDir, repo)
now  := snapshot.Build(ctx, runner, scope)         // ONE gh pr list for all/label; gh pr view (no #) for current
events := diff.Diff(prev, now)                      // pure; §8 incl. §8.3a draft-ready; UNSTABLE never ready
state.Persist(stateDir, repo, now)                  // ATOMIC, BEFORE printing → at-most-once (§12.1)
for e in events: println(e.Line())                  // agent relays each line to PushNotification
if len(now.PRs) == 0 { println("🏁 watch complete — stopping"); os.Exit(10) }
// scope=current no-PR: increment NoPRForBranch counter in state; ≥grace ⇒ os.Exit(10); else continue
```

---

## 5. TDD build order

Tests-first throughout. Each phase has what-to-write · how-to-verify · a concrete **done-when**
gate. Order is interface-first AND sanitize-first (it gates both `diff` and `status`).

### Phase 0 — Scaffold + version (no logic)
- **Write:** `go.mod`, `main.go`, `cmd/root.go` (with `tick` as default command), `cmd/version.go`,
  `internal/version/{version.go,version_test.go}` (copy gss, swap `gss`→`prping`), `VERSION=0.1.0`,
  `LICENSE`, `build.sh`.
- **Done-when:** `cd sdk/prping && go build ./... && go test ./internal/version/... && ./build.sh
  && ~/opt/bin/prping version` prints `prping v0.1.0`.

### Phase 1 — `internal/sanitize` (BUILD FIRST — gates diff + status)
- **Write tests first:** `sanitize_test.go` — golden: a title with `\n` + ANSI `\x1b[31m` + NUL ⇒
  one clean line, no escapes survive, truncated to `max`; idempotence (`Clean(Clean(x))==Clean(x)`).
- **Implement:** `Clean(s, max)`.
- **Done-when:** `go test ./internal/sanitize/...` green. **Pure leaf — frozen.**

### Phase 2 — `internal/gh` Runner + fake (BLOCKING interface leaf — freeze)
- **Template:** copy gss **`internal/git`** (thin `Run` seam), **not** gss `internal/gh`.
- **Write tests first:** `exec_test.go` (inject stub Exec, assert exact argv + **stdout-only** +
  `ExecError`-on-nonzero, incl. a `gh api graphql` argv case); `fake/runner_test.go` (FIFO pop,
  scripted output/errors, call recording, `Reset`).
- **Done-when:** `go test ./internal/gh/...` green; `var _ gh.Runner = (*fake.Runner)(nil)`
  compiles. **Interface frozen.**

### Phase 3 — `internal/snapshot` (owns `Snapshot`; consumes frozen Runner + sanitize)
- **Write tests first:** feed fixture payloads through `fake.Runner`; assert exact `Snapshot`:
  field extraction (`headRefOid`→HeadSha, `baseRefName`→BaseRef), `failingChecks` from
  `statusCheckRollup`, `state` from `--state all`, title sanitized. Cases: 0 PRs, each
  `mergeStateStatus` **incl. `UNSTABLE`/`HAS_HOOKS`**, multi, draft, conflicting, UNKNOWN,
  missing/null fields (totality). **AC2-single-call**: `scope_all_one_list_call` asserts
  `len(fake.Calls where cmd=="gh pr list")==1` for N=10. **UC1 detect**: `current` scope issues
  `gh pr view` with NO number; the four failure modes (`detached_head`, `no_pr`, `multiple_prs`,
  `fork`) each return a typed condition.
- **Done-when:** `go test ./internal/snapshot/...` green; snapshot byte-stable.

### Phase 4 — `internal/diff` goldens (the heart; owns `Event`; consumes snapshot + sanitize)
- **Write tests first:** one+ named case per §8.1–8.8 **and §8.3a** (fires + must-not-fire),
  §8.7 invariants (`Diff(now,now)==[]`, ordered, restart-no-replay), totality
  (UNKNOWN/null/missing ⇒ seed-silent, no panic), the new **`ready_unstable` must-not-fire**, flap
  guards (CLEAN→UNKNOWN→CLEAN no re-fire; empty→refill no manufactured check-failed),
  `draft_ci_ready_once`, sanitization goldens, and a **coverage assertion** failing if any §8 rule
  id (incl. 8.3a) lacks a case. Goldens in `testdata/golden/*.txt`.
- **Implement:** `Event` + `Diff` (pure, total), §8.3a draft-ready one-shot, precedence resolver
  (§8.8), `Event.Line()` via `sanitize`, PR-number ordering.
- **Done-when:** `go test ./internal/diff/...` green; §6 traceability reproducible; coverage ≥85%.

### Phase 5 — `internal/state` (consumes only `Snapshot`)
- **Write tests first:** atomic write (temp+rename), perms (0700/0600 under umask 077), lock
  (second writer no-ops), filename derivation (`owner/repo`→safe, `/`→`-`, collision-free),
  seed-silent on absent/empty/malformed, **NoPRForBranch grace-counter persists across restart**.
- **Done-when:** `go test ./internal/state/...` green; perms asserted via `os.Stat`.

### Phase 6 — `internal/status` (UC3; owns `StatusReport`/`StatusRow`; consumes Runner + sanitize)
- **Write tests first:** `issues.go` union (`issue_union_dedupe`, `issue_body_only`, `issue_none`);
  `threads.go` unanswered 4-case matrix (`unanswered_counts`, `resolved`, `outdated`, `bot`,
  `self`) + `graphql_failure_totality` (cell ⇒ `?`, exit 0); `classify.go` `mergeable_cell_matrix`;
  `render.go` `render_golden` (truncation, relative `Updated` with injected `now`, no-ANSI,
  byte-stable) + `json_roundtrip`; `Build` issues exactly ONE `gh pr list` + ONE graphql per OPEN
  non-draft PR. Synthetic fixtures only (octocat/reviewer-bob/acme).
- **Done-when:** `go test ./internal/status/...` green; render byte-stable.

### Phase 7 — `cmd/*` wiring + lifecycle integration (binds it all)
- **Write tests first:** `cmd/tick_test.go` — in-process, `fake.Runner` + temp `--state-dir`,
  drive the ordered fixture sequence (draft-opened → push → pending → UNSTABLE → CLEAN-while-draft
  (🟡) → ready-for-review (🟢) → behind → update → CLEAN → merged); assert transcript == golden;
  restart-resume (no replay); `exit10_empty_set`; **UC1** `detect_current_pr`, `detached_head`
  (exit 2), `no_pr_grace` (exit 10 after N), `multiple_prs_refuse`, `fork_warn`, `no_mutating_verb`.
  `cmd/status_test.go` — `no_state_write`, scope-default, `--json`. Plus
  `cmd/{snapshot,diff,label,version}_test.go`.
- **Done-when:** `go test ./cmd/...` green; empty-set ⇒ exit 10; detached ⇒ exit 2.

### Phase 8 — `TestFixturesNoIdentity` + packaging / install / docs / skill
- **Write:** `fixtures_lint_test.go` (FORBIDDEN-login substring lint over `testdata/**`);
  `install.sh` block (after gsl, line 400), `scripts/test.sh` `COVERAGE_MIN[prping]=70`,
  `sdk/GEMINI.md` row, `.golangci.yml` comment, `skill/SKILL.md`, `GEMINI.md` + `CLAUDE.md`
  symlink, `README.md`, `.gitignore` `sdk/prping/prping` exclusion (with comment).
- **Done-when:** `go test ./...` green incl. `TestFixturesNoIdentity`; `scripts/test.sh` reports
  `prping` ≥70%; `make lint-go` clean; `install.sh` builds `~/opt/bin/prping`; `sync-skills` links
  `prping`; §9.6 trigger-eval recorded; §9.7 manual sign-off on PR #127.

---

## 6. §8-rule → Go-test traceability

§8-rule → named `internal/diff` golden case (cases live in `internal/diff/diff_test.go` +
`testdata/golden/`). Every rule id (incl. the new **§8.3a**) maps 1:1 to ≥1 named case; a coverage
assertion in `diff_test.go` fails the build if any id lacks one.

| §8 rule | Fires case(s) | Must-NOT-fire case(s) |
| :-- | :-- | :-- |
| §8.1 PR opened | `opened_first_sight` | `opened_already_present`, `opened_draft`, `opened_clean_no_ready` |
| §8.2 Push landed | `push_sha_advance`, `push_force_push_nondescendant` | `push_same_sha` |
| §8.3 Ready to merge (CLEAN-only) | `ready_behind_to_clean` | `ready_clean_to_clean`, `ready_draft`, **`ready_unstable`**, `ready_unknown_flap`, `ready_behind` |
| **§8.3a Draft CI-ready (NEW)** | `draft_ci_ready_once` | `draft_ci_ready_no_refire`, `draft_ready_superseded_by_green` |
| §8.4 Needs update | `behind_green` | `behind_already`, `behind_with_failing` |
| §8.5 Check failed | `check_new_failure`, `check_clear_then_recur` | `check_same_set`, `check_empty_refill_flap` |
| §8.6 Merged / closed | `merged_state_transition`, `closed_not_merged` | `closed_no_event_after` |
| §8.7 Idempotence / order / restart / format / totality | `idempotent_now_now`, `noop_prev_eq_now`, `order_multi_pr_one_tick`, `restart_resume_no_replay`, `format_line_len_and_oneline`, `total_absent_prev_seed`, `total_unknown_mergestate`, `total_unstable_mergestate`, `total_null_checks` | — |
| §8.8 Precedence | `precedence_behind_and_failing` | — |
| Sanitization (§Security) | `sanitize_newline_in_title`, `sanitize_ansi_in_title`, `sanitize_nul_in_title` | — |

Other automated rows:

| Spec ref / UC | Named test | Package |
| :-- | :-- | :-- |
| §Security sanitize | `sanitize_clean_golden`, `sanitize_idempotent` | `internal/sanitize` |
| §9.2 snapshot + UC1/UC2 | per-status (incl. `UNSTABLE`), multi, draft, conflicting, UNKNOWN, missing-fields, scope-filter, `--state all`, `scope_all_one_list_call`, `detect_current_pr`, `detached_head`, `no_pr`, `multiple_prs`, `fork` | `internal/snapshot` |
| §9.3 state | atomic-write, perms-0700/0600, lock-second-writer-noop, `filename_slash_to_hyphen`, `filename_no_collision`, seed-silent-{absent,empty,malformed}, `grace_counter_persist` | `internal/state` |
| UC3 status | `empty_scope`, `issue_union_dedupe`, `issue_body_only`, `issue_none`, `unanswered_counts`, `unanswered_resolved`, `unanswered_outdated`, `unanswered_bot`, `unanswered_self`, `graphql_failure_totality`, `mergeable_cell_matrix`, `render_golden`, `json_roundtrip`, `build_one_list_one_graphql` | `internal/status` |
| §9.4 cmd/tick lifecycle + UC1 | `lifecycle_transcript_golden`, `restart_resume_no_replay`, `exit10_empty_set`, `detect_current_pr`, `detached_head`, `no_pr_grace`, `multiple_prs_refuse`, `fork_warn`, `no_mutating_verb` | `cmd` |
| UC3 stateless | `no_state_write` | `cmd` |
| §Security fixtures | `TestFixturesNoIdentity` | module root |
| §9.5 coverage gate | `COVERAGE_MIN[prping]=70` via `scripts/test.sh` | — |

**Human-evidenced gates (NOT CI-checkable — recorded in PR #127):**

| Spec ref | Evidence |
| :-- | :-- |
| §9.6 skill-trigger eval | phrasings→expected corpus (UC1 "watch this PR", UC2 "watch all my PRs", UC3 "show PR status"); **≥90% top-1**; tune the SKILL.md description, not behavior; fallback = add an explicit trigger phrase. |
| §9.7 manual phone-acceptance | Start `prping` in a Remote-Control session, push to a watched PR, confirm the phone push (suppressed while typing); checklist in `SKILL.md`; one sign-off on PR. |

---

## 7. Security & privacy (verified must-fix)

- **The privacy_guard gap.** `ai/hooks/privacy_guard.sh` Rule C keys on the local `$USER`/`$HOSTNAME`
  (confirmed `ME_USER`/`ME_HOST`) — it does NOT catch the GitHub committer login `sfc-gh-eraigosa`,
  the git author name, or email. All three use cases surface gh-sourced human logins, so **prping
  owns its own identity hygiene** — the hook is not a backstop here.
- **Mandatory `internal/sanitize`.** Every gh-sourced string in BOTH `diff` and `status` routes
  through `sanitize.Clean` (strip C0/C1 + ANSI CSI, collapse newlines, truncate, no markdown). This
  also closes the terminal-escape / event-line-injection vector (a crafted PR title cannot spoof a
  second event line).
- **Identity-minimal state.** The persisted snapshot carries only number/title/branch/base/sha/
  state/status — NO author login, NO comment body, NO issue title. Title is the one identity-adjacent
  field and is sanitized. `status` is **stateless** — it persists nothing.
- **`TestFixturesNoIdentity`.** Synthetic identifiers ONLY (repo `acme/widgets`, logins
  `octocat`/`reviewer-bob`, branch `feature/x`, fake SHAs). A Go substring-lint (case-insensitive,
  stricter than Rule C's word-boundary) FAILS on a FORBIDDEN-login list (seeded `sfc-gh-eraigosa`),
  `/home/`÷`/Users/` paths, or a real `@`-email. **Do NOT copy sdk/gss's real-login PR URLs** (they
  embed `sfc-gh-eraigosa` — verified) into prping fixtures.
- **gh token scope (fail-closed).** Reading PRs+issues+comments needs classic `repo` (or
  fine-grained Contents/PullRequests/Issues:read). `tick`/`status` run `gh auth status` on start and
  FAIL-CLOSED with an actionable message rather than rendering a silently-empty Unanswered column.
- **Privacy posture for `status` identity.** Default = show real logins in the TERMINAL table (max
  utility) with a docs warning against committing raw output; `--json` redacts to role placeholders.
  (Open decision — see §9.)

---

## 8. Integration & rollout

- **install.sh:** new build+install block after the gsl block (ends line 400), before the Nerd-Font
  comment, byte-mirroring the gss block (356–365) with `gss`→`prping`. Builds to `~/opt/bin/prping`.
- **scripts/test.sh:** add `[prping]=70` after `[wol]=60` (line 45). Module discovery already globs
  `sdk/*/go.mod` — no edit. **No `make shell-test`/`lint-shell` change** (the Go form needs none).
  The floor lands in warn-only mode initially (`COVERAGE_ENFORCE` defaults `0`); the pure
  `sanitize`/`diff`/`status`/`snapshot` packages push it well past 70% from the first green build.
- **sdk/GEMINI.md:** add the `prping/` row after `tmux-mgr`.
- **.golangci.yml:** add `sdk/prping` to the by-name module list in the comment (lines 9–10):
  "FOUR … sdk/gss, sdk/tmux-mgr, sdk/gsl, sdk/wol" → "FIVE … sdk/gss, sdk/tmux-mgr, sdk/gsl,
  sdk/wol, sdk/prping" (not just bump the numeral).
- **sync-skills:** no change — `sdk/prping/skill/SKILL.md` auto-discovered, linked as bare `prping`.
  Verify with a real run + `ls -l ~/.claude/skills/prping ~/.agents/skills/prping`.
- **Docs convention:** `sdk/prping/GEMINI.md` + `ln -s GEMINI.md CLAUDE.md`.
- **Tag scheme:** `VERSION=0.1.0` feeds the existing `sdk/prping/vX.Y.Z` tag-automation.
- **Rollback (§11):** pure addition — `rm -rf sdk/prping ~/.config/prping`; revert the
  install.sh / scripts/test.sh / sdk/GEMINI.md / .golangci.yml / .gitignore edits.

### 8.1 Build leaves / DAG (for parallel `gss feature` workers)

`internal/sanitize` AND `internal/gh` are the **blocking** leaves. Domain types live in their
owning packages. Edge `A → B` = "B depends on A".

| Leaf | Owns (paths) | Consumes | done-when | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| L0 scaffold+version | `main.go`, `cmd/{root,version}.go`, `internal/version/**`, `go.mod`, `build.sh`, `VERSION`, `LICENSE` | — | `go build ./... && ~/opt/bin/prping version` | yes (base) |
| L1 sanitize | `internal/sanitize/**` | L0 | `go test ./internal/sanitize/...` | **yes** (gates L3,L5) |
| L2 gh runner+fake | `internal/gh/**` (Runner + SystemRunner + fake; gss `internal/git` analog) | L0 | `go test ./internal/gh/...`; fake assertion compiles | **yes** (interface leaf) |
| L3 snapshot | `internal/snapshot/**` (owns `Snapshot`/`PR`/`Scope`) | L1, L2 | `go test ./internal/snapshot/...`; single-`gh pr list` call; UC1 detect modes | no |
| L4 diff (heart) | `internal/diff/**` (owns `Event`) | **L1, L3 — NOT L2** | `go test ./internal/diff/...`; every §8 (incl 8.3a) → case; ≥85% | no |
| L5 state | `internal/state/**` | L3 | `go test ./internal/state/...`; perms + grace-counter | no |
| L6 status (UC3) | `internal/status/**` (owns `StatusReport`/`StatusRow`), `cmd/status.go` | L1, L2 | `go test ./internal/status/... ./cmd -run Status`; one-list+one-graphql | no |
| L7 cmd/tick lifecycle | `cmd/{tick,snapshot,diff,label}.go` (+tests) | L2, L3, L4, L5 | `go test ./cmd/...`; exit 10/2; transcript==golden | no |
| L8 fixtures-lint + packaging + skill + docs | `fixtures_lint_test.go`, `install.sh`, `scripts/test.sh`, `sdk/GEMINI.md`, `.golangci.yml`, `skill/SKILL.md`, `GEMINI.md`+symlink, `README.md`, `.gitignore` | L7 | `go test ./...` (incl. fixtures-lint); `scripts/test.sh` ≥70%; `make lint-go`; `sync-skills`; §9.6/§9.7 | no |

DAG (acyclic): `L0 → {L1, L2} ; {L1,L2} → L3 ; {L1,L3} → L4 ; L3 → L5 ; {L1,L2} → L6 ;
{L2,L3,L4,L5} → L7 → L8`. L1 (sanitize) + L2 (Runner) are built first; L4 (the PURE heart) imports
`internal/snapshot` + `internal/sanitize` but **never** `internal/gh`.

---

## 9. Open decisions (carry to owner / spec §12)

1. **UC1 multiple-PRs-for-one-branch** — recommend **refuse + warn** (b) over silent
   most-recently-updated, to avoid watching the wrong PR. (Plan assumes refuse.)
2. **`status` identity posture** — recommend show-real-logins-in-terminal + docs-warn, redact in
   `--json`. (Plan assumes this.)
3. **no-pr-grace default** — recommend 3 polls (~13min at 270s) over a time-based window.
4. **Linked-issue display** — numbers-only (`#NN`) in v1; `--issue-titles` (extra `gh issue view`
   per issue) deferred.
5. **`--offer-ready` / `--mark-ready`** — deferred; v1 is NOTIFY-ONLY (no `gh pr ready`, no
   `gss feature pr --ready`).
6. **Stacked-PR "rebased on main"** — out of scope v1; CLEAN/BEHIND are computed against the PR
   BASE (a worker's parent, not main). The ready line must not imply whole-stack-on-main.

> Produced via `superpowers:writing-plans`. Execute with `superpowers:executing-plans` /
> `subagent-driven-development`, TDD throughout. Update `../index.md` state as it moves.
