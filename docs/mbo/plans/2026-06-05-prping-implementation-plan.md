# prping — implementation plan

- **Slug:** prping
- **Date:** 2026-06-06
- **Status:** Draft
- **Relates to:** spec [`../specs/2026-06-05-prping-design.md`](../specs/2026-06-05-prping-design.md) · PR #127
- **Supersedes:** the earlier bash-script plan that lived at this same path (see git history) — replaced by this Go-CLI plan.

## 1. Summary & verdict

Build **prping** as a first-class sdk Go module at `sdk/prping`, mirroring `sdk/gss` exactly (cobra `cmd/`, `internal/` packages, a mockable runner seam, `internal/version` ldflags, `build.sh`, coverage gate). The deterministic core is the binary; the agent skill is a thin print→`PushNotification` relay. The load-bearing decision surface — which events fire on a `(prev, now)` snapshot transition — lives in a **pure** `internal/diff` package, golden-tested 1:1 against spec §8.1–8.8. `prping status` (UC3) adds a pure `internal/status` enrichment + renderer.

**Verdict: approved to build.** This plan supersedes the bash plan. Carried forward unchanged: the spec §8→test traceability table (now mapped to `internal/diff` golden cases, §5 below) and the privacy/fixture-attribution discipline (§5, §6). **Discarded** from the bash plan: the `src/prping/*.sh` script inventory, the `make shell-test` / `lint-shell` `find`-root edits, and "Blocker 1" (shell-test not scanning `src/`) — the Go form dissolves all three because tests run through the **existing** `scripts/test.sh` module discovery under `sdk/` with **no Makefile shell-test/lint-shell change**. Ground truth verified against `sdk/gss` (cmd/root.go, internal/git/{exec.go,fake/}, internal/gh, internal/version, build.sh, go.mod), `install.sh` (gss block lines 356–365; sdk build sequence ends with the gsl block at line 400), `scripts/test.sh` (COVERAGE_MIN lines 41–46; module discovery lines 71–93 globs `sdk/*/go.mod`), `.golangci.yml` (module comment lines 9–12), and `sdk/GEMINI.md` (module table).

**Review verdict.** This plan was authored by **go-goarch** (The Go Architect) and reviewed by **architecture-adversary** and **go-godev** (The Go Developer). All must-fix findings are folded in: (1) the `install.sh` insertion anchor is corrected to *after the gsl block (line 400)* — the actual last sdk build block — not after tmux-mgr (line 378); (2) the testability seam is explicitly documented as analogous to gss's **`internal/git`** (a thin `Run(ctx, name, args...)` seam over both `gh` and `git`), **not** gss's domain-rich `internal/gh.Client`; (3) the fake follows the **FIFO-Script** pattern from `sdk/gss/internal/git/fake` (named `fake.Runner`), not the per-verb stateful `gh/fake.Client`. Should-fixes folded in: domain types are homed in their owning packages (`Snapshot`→`internal/snapshot`, `Event`→`internal/diff`, `Row`→`internal/status`) so the PURE `internal/diff` never imports `internal/gh`; the `SystemRunner` stdout-only / stderr-in-`ExecError` contract is pinned; the coverage entry is documented as warn-only-initially.

## 2. File inventory

All paths absolute-from-repo-root `sdk/prping/` unless noted. Every artifact maps to a spec section.

### Inside `sdk/prping/` (the module)

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `main.go` | `package main` → `cmd.Execute()` (mirrors gss `main.go`). | §3.2 |
| `cmd/root.go` | cobra root `prping`; persistent flags `--scope`, `--repo`; default-scope resolution (`current` on a PR branch else `label:prping`). | §3.1 |
| `cmd/snapshot.go` | `prping snapshot --scope <s> [--repo <r>]` → snapshot JSON on stdout. | §3.1, §3.3 |
| `cmd/diff.go` | `prping diff <prev.json> <now.json>` → event lines (pure, no runner). | §3.1, §4, §8 |
| `cmd/tick.go` | `prping tick --scope <s> [--repo <r>] [--state-dir <d>] [--dry-run]` → load prev → snapshot → diff → persist → print; **exit 10 = watched set empty**. | §3.1, §3.4, §5 |
| `cmd/label.go` | `prping label add\|remove <#…> [--label prping]` (`gh pr edit --add-label/--remove-label`). | §3.1, §12.2 |
| `cmd/status.go` | `prping status --scope <s> [--repo <r>] [--json]` → UC3 table / JSON; no state, no notifications. | §3.1, §3.5, UC3 |
| `cmd/version.go` | ldflags version block + `--json` (mirrors gss). | §7 |
| `cmd/*_test.go` | one `_test.go` per command; `cmd/tick_test.go` is the §9.4 in-process lifecycle integration test. | §9.4 |
| `internal/gh/exec.go` | `Runner` interface + `SystemRunner` (real `gh`/`git` via os/exec seam, **stdout-only + stderr folded into `*ExecError`**); the testability seam. **Analog: gss `internal/git`, NOT gss `internal/gh`** (see §3). | §3.2 |
| `internal/gh/fake/runner.go` | scriptable fake `Runner` following the **FIFO-Script** pattern (`Script []Response`, `Calls []CallRecord`, `Default Response`, `Reset()`) from `sdk/gss/internal/git/fake` — **not** the per-verb stateful `gh/fake.Client`. | §3.2, §9.2, §9.4 |
| `internal/gh/exec_test.go`, `internal/gh/fake/runner_test.go` | seam argv-assertion tests (exact argv, stdout-only, `ExecError`-on-nonzero) + fake self-tests. | §9.2 |
| `internal/snapshot/snapshot.go` | **Owns the `Snapshot`/`PR`/`Scope` type decls.** `Build(ctx, runner gh.Runner, Scope) (Snapshot, error)`: runs `gh pr list/view … --state all` + `git`, parses → `Snapshot` (schema-closed). | §3.2, §3.3, §8.6 |
| `internal/snapshot/snapshot_test.go` | table tests w/ fake runner; fixtures per status. | §9.2 |
| `internal/diff/diff.go` | **PURE** — owns the `Event` type. `Diff(prev, now snapshot.Snapshot) []Event` + `Event.Line()` (sanitize, <200 chars, ordered). The heart. **Imports only `internal/snapshot` for the `Snapshot` type — never `internal/gh`.** | §4, §8.1–8.8 |
| `internal/diff/diff_test.go` | golden table tests: one+ named case per §8 rule (fires + must-not-fire) + invariants + totality + flap guards + coverage assertion. | §8, §9.1 |
| `internal/diff/testdata/golden/*.txt` | golden event-line transcripts (synthetic identifiers only). | §9.1 |
| `internal/status/status.go` | **Owns the `Row` type.** `Enrich(ctx, runner gh.Runner, snapshot.Snapshot) ([]Row, error)` (linked issues ∪ body `#NN` refs; unanswered review threads via graphql) + `Render(rows, json bool) string`. Enrichment is pure over fetched data. | §3.5, UC3 |
| `internal/status/status_test.go` | table tests w/ fake runner: issue links, unanswered-thread detection (latest author ≠ PR author, bots excluded), render + `--json`. | §9 (UC3) |
| `internal/state/state.go` | `Load(dir, repo) (snapshot.Snapshot, bool, error)` / `Persist(dir, repo, snapshot.Snapshot) error`: atomic temp+rename, umask 077 (0700 dir / 0600 file), per-repo lock, filename derivation, seed-silent. | §3.3, §5, §12.1 |
| `internal/state/state_test.go` | atomic write, perms, lock, filename derivation/collision, seed-silent on absent/empty/malformed. | §9.3 |
| `internal/version/version.go` + `_test.go` | ldflags vars + `Get()` fallbacks (verbatim shape from gss). | §7 |
| `build.sh` | version-stamping build → `~/opt/bin/prping` (mirrors gss build.sh, swap `gss`→`prping`). | §7 |
| `VERSION` | `0.1.0`. | §7 |
| `LICENSE` | Apache-2.0 (copy gss). | §7 |
| `README.md` | tool README. | §7 |
| `go.mod` / `go.sum` | `module github.com/sfc-gh-eraigosa/dotfiles/sdk/prping`; `go 1.26.1`; cobra dep. | §7 |
| `skill/SKILL.md` | the agent skill: loop driver, line→`PushNotification` relay, prereq checks (§6), §9.7 manual-acceptance checklist; trigger `description` tuned for §9.6. | §3, §5, §6, §9.6, §9.7 |
| `GEMINI.md` + `CLAUDE.md` (symlink → `GEMINI.md`) | per-dir docs (repo convention). | §7 |

### Touch-points OUTSIDE `sdk/prping/` (verified)

| Path | Change | Anchor (exact insertion point) | Implements |
| :-- | :-- | :-- | :-- |
| `install.sh` | Add a build+install block mirroring the gss block. | **After the gsl block — the last sdk build block — ending at line 400** (closing `fi`), and **before the `# Configure the Nerd Font` comment at line 402**. Insert: `if [ -f "${BASE_DIR}/sdk/prping/build.sh" ]; then … bash "${BASE_DIR}/sdk/prping/build.sh" … "${HOME}/opt/bin/prping" version … fi` — same shape as the gss block at lines 356–365 with `gss`→`prping`. (NB: tmux-mgr ends at 378, wol at 389, gsl at 400 — gsl is the last block, not tmux-mgr.) | §7 |
| `scripts/test.sh` | Add `[prping]=70` to the `COVERAGE_MIN` map. | Inside `declare -A COVERAGE_MIN=( … )`, after **line 45** (`[wol]=60`), add `    [prping]=70`. Module discovery (lines 71–93) already globs `sdk/*/go.mod` — **no discovery edit**; prping is auto-found. | §7, §9.5 |
| `sdk/GEMINI.md` | Add a module-table row. | In the `## Modules` table, after the `tmux-mgr` row, add: `| `[`prping/`](./prping/GEMINI.md)` | `github.com/sfc-gh-eraigosa/dotfiles/sdk/prping` | `prping` | Claude-native PR/push notifier. |`. | §7 |
| `.golangci.yml` | Update the module-count comment. | **Lines 9–10**: change "FOUR separate Go modules: sdk/gss, sdk/tmux-mgr, sdk/gsl, sdk/wol" → "FIVE … sdk/gss, sdk/tmux-mgr, sdk/gsl, sdk/wol, sdk/prping". Comment-only; the `lint-go` per-module loop auto-discovers. | §7 |
| `sync-skills` | **No code change.** `sdk/prping/skill/SKILL.md` is auto-discovered (dual-scans `src/`+`sdk/` since the gss migration) and linked as bare `prping`. | n/a (verify by running it, §6). | §7 |
| `~/.config/prping/` | Runtime state dir, created 0700 by `internal/state` at first run; outside the repo tree; never committed. | n/a | §3.3, §11 |

**`.gitignore`: no change.** `!sdk/` + `!sdk/**` already opt in the entire `sdk/prping/` tree (mirrors how `sdk/gss/**` is tracked). The `~/.config/prping/` state dir lives in `$HOME`, outside the repo. **Confirm** with `git status --short -- sdk/prping` after scaffolding.

## 3. Interface contracts

### Naming caveat — `internal/gh` is the gss `internal/git` analog (NOT gss `internal/gh`)

prping's `internal/gh` is a **thin subprocess seam** with a single `Run(ctx, name string, args ...string) ([]byte, error)` method covering **both** the `gh` and `git` binaries under one interface — exactly the shape of gss's **`internal/git`**, as spec §3.2 states ("mirrors gss `internal/git`"). It is **NOT** analogous to gss's `internal/gh`, which exports a domain-rich `Client` (`PRCreate`/`PREdit`/`PRList`/…). An implementer must copy the gss **`internal/git`** package as the template, not gss `internal/gh`. This divergence is intentional and is called out here and in the §6.1 DAG so no one copies the wrong source package.

### `internal/gh.Runner` (the testability seam — mirrors gss `internal/git`)
```go
type Runner interface {
    Run(ctx context.Context, name string, args ...string) ([]byte, error)
}
```
`name` is `gh` or `git`. **`SystemRunner` returns stdout only**; on non-zero exit it returns `*ExecError{Args, Stderr, Err}` with stderr folded into the error (matching gss's `internal/gh` `systemExec` stdout-only contract, **not** gss's `internal/git.SystemRunner` which returns *combined* stdout+stderr). The fake therefore carries separate `Stdout`/`Stderr` fields in its `Response`, but `Run()` returns `Stdout` only — **do not copy a `combined()` helper from `git/fake`.**

### `internal/gh/fake.Runner` (FIFO-Script fake — mirrors gss `internal/git/fake`)
Follows the **FIFO-Script** pattern, not the per-verb stateful `gh/fake.Client`:
```go
type Response struct { Stdout, Stderr []byte; Err error }
type CallRecord struct { Name string; Args []string }
type Runner struct {
    Script  []Response   // consumed FIFO, one per Run call
    Calls   []CallRecord // recorded in order
    Default Response     // returned when Script is exhausted
}
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, error) // pops Script head, records call, returns Stdout only
func (r *Runner) Reset()
var _ gh.Runner = (*fake.Runner)(nil) // compile-time assertion
```

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
    Title            string   `json:"title"`
    Branch           string   `json:"branch"`
    HeadSha          string   `json:"headSha"`
    State            string   `json:"state"`            // OPEN|MERGED|CLOSED
    IsDraft          bool     `json:"isDraft"`
    MergeStateStatus string   `json:"mergeStateStatus"` // CLEAN|BEHIND|BLOCKED|DIRTY|UNKNOWN
    Mergeable        string   `json:"mergeable"`        // MERGEABLE|CONFLICTING|UNKNOWN
    FailingChecks    []string `json:"failingChecks"`
}

// internal/diff — Event is one diff result (§4). Kind drives the glyph/template.
type Event struct { PRNumber int; Kind EventKind; Title, Branch, Sha, Status, Detail string }
func (e Event) Line() string // sanitized, single line, <200 chars, no markdown

// internal/status — Row is one UC3 status row (§3.5).
type Row struct {
    Number int; Title, Description, Mergeable, Updated string
    Issues []string; Unanswered []int // ids of open PR comments w/ no response
}
```
**Import directions (acyclic):** `internal/snapshot` imports `internal/gh` (Runner). `internal/diff` imports **only** `internal/snapshot` (for `Snapshot`) — *not* `internal/gh`. `internal/status` imports `internal/gh` + `internal/snapshot`. `internal/state` imports **only** `internal/snapshot`. This keeps the PURE heart (`internal/diff`) free of any runner dependency.

### cobra command signatures (each in `cmd/`, `RunE` returning error; exit via `os.Exit` for the 10 case)
```
snapshot:  RunE → snapshot.Build(ctx, gh.NewSystemRunner(), scope); json.NewEncoder(stdout).Encode(snap)
diff:      RunE → read prevFile,nowFile; diff.Diff(prev, now); print each Event.Line()   (NO runner)
tick:      RunE → state.Load(dir,repo) → snapshot.Build → diff.Diff → state.Persist(BEFORE print) →
                  print lines; if len(now.PRs)==0 { print "🏁 …"; os.Exit(10) }; --dry-run skips Persist
label:     RunE → for each #N: gh pr edit N --add-label|--remove-label <label>
status:    RunE → snapshot.Build → status.Enrich → fmt.Print(status.Render(rows, jsonFlag))   (no state)
version:   Run  → version.Get() block (+ --json)
```

### Pure-function signatures
```go
func diff.Diff(prev, now snapshot.Snapshot) []Event   // total: malformed/UNKNOWN ⇒ seed-silent, never panics
func status.Enrich(ctx, r gh.Runner, s snapshot.Snapshot) ([]Row, error)
func status.Render(rows []Row, asJSON bool) string
```

### `tick` orchestration pseudocode (the prev→now contract)
```
repo  := resolveRepo(--repo)                       // gh repo view nameWithOwner if empty
prev, seeded := state.Load(stateDir, repo)         // seeded=false ⇒ no prior file (seed-silent)
now  := snapshot.Build(ctx, runner, scope)         // gh pr list/view --state all  (§3.3, §8.6)
events := diff.Diff(prev, now)                      // pure; on first sight emits consolidated "opened"
state.Persist(stateDir, repo, now)                  // ATOMIC, BEFORE printing → at-most-once (§12.1)
for e in events: println(e.Line())                  // agent relays each line to PushNotification
if len(now.PRs) == 0 { println("🏁 watch complete — stopping"); os.Exit(10) }
```

## 4. TDD build order

Tests-first throughout. Each phase has what-to-write · how-to-verify · a concrete **done-when** gate. Order is interface-first: the `gh.Runner` seam is frozen before any consumer.

### Phase 0 — Scaffold + version (no logic)
- **Write:** `go.mod` (`…/sdk/prping`), `main.go`, `cmd/root.go`, `cmd/version.go`, `internal/version/{version.go,version_test.go}` (copy gss shape, swap `gss`→`prping`), `VERSION=0.1.0`, `LICENSE`, `build.sh`.
- **Verify:** `version_test.go` asserts `Get()` fallbacks (`dev`/`none`/`unknown`/`false`).
- **Done-when:** `cd sdk/prping && go build ./... && go test ./internal/version/... && ./build.sh && ~/opt/bin/prping version` prints `prping v0.1.0`.

### Phase 1 — `internal/gh` Runner + fake (BLOCKING interface leaf — freeze first)
- **Template:** copy gss **`internal/git`** (the thin `Run` seam), **not** gss `internal/gh`.
- **Write tests first:** `internal/gh/exec_test.go` (inject stub Exec, assert exact argv + **stdout-only** + `ExecError`-on-nonzero with stderr folded in); `internal/gh/fake/runner_test.go` (FIFO Script pop, scripted output/errors, call recording, `Reset`).
- **Implement:** `Runner` interface + `SystemRunner` + `fake.Runner` (FIFO-Script).
- **Done-when:** `go test ./internal/gh/...` green; `var _ gh.Runner = (*fake.Runner)(nil)` compiles. **Interface frozen — §3 is now the contract every consumer imports.**

### Phase 2 — `internal/snapshot` (owns `Snapshot`; consumes the frozen Runner)
- **Write tests first:** `snapshot_test.go` feeding fixture `gh`/`git` payloads through `fake.Runner`; assert exact `Snapshot`: field extraction, `isDraft`, `failingChecks` from `statusCheckRollup`, `state` from `--state all`, scope filtering, merged/closed. Cases: 0 PRs, each `mergeStateStatus`, multi, draft, conflicting, UNKNOWN, missing/null fields.
- **Implement:** the `Snapshot`/`PR`/`Scope` type decls + `Build` with `// empty`/`// []`-style defaulting; PRs sorted by number; sorted `failingChecks`; schema-closed (no extra keys).
- **Done-when:** `go test ./internal/snapshot/...` green; snapshot byte-stable across two runs on the same fixtures.

### Phase 3 — `internal/diff` goldens (the heart; owns `Event`; consumes only `snapshot.Snapshot`)
- **Write tests first:** `diff_test.go` — one+ named case per §8.1–8.8 (fires + must-not-fire), §8.7 invariants (`Diff(now,now)==[]`, ordered, restart-no-replay), totality on degraded payloads (UNKNOWN/null/missing ⇒ seed-silent, no panic), flap guards (CLEAN→UNKNOWN→CLEAN no re-fire ready; empty→refill no manufactured check-failed), sanitization (newline/ANSI in title ⇒ stripped, one line, <200), and a **coverage assertion** failing if any §8 rule id lacks a case. Goldens in `testdata/golden/*.txt`.
- **Implement:** the `Event` type + `Diff` (pure, total — imports `internal/snapshot` only, never `internal/gh`), first-sight consolidation, precedence resolver (§8.8: check-failed > needs-update; push independent), `Event.Line()` sanitize+truncate, PR-number ordering.
- **Done-when:** `go test ./internal/diff/...` green; the §5 traceability table reproducible from case names; coverage of `internal/diff` ≥85%.

### Phase 4 — `internal/state` (consumes only `snapshot.Snapshot`)
- **Write tests first:** `state_test.go` — atomic write (temp+rename, truncation-safe), perms (0700 dir / 0600 file under umask 077), lock (second writer no-ops/refuses), filename derivation (`owner/repo`→safe name, collision-free, `/`→`-` documented + tested), seed-silent on absent/empty/malformed prev.
- **Implement:** `Load`/`Persist` with a `--state-dir` override default `~/.config/prping`.
- **Done-when:** `go test ./internal/state/...` green; perms asserted via `os.Stat`.

### Phase 5 — `internal/status` (UC3; owns `Row`; consumes Runner + `snapshot.Snapshot`)
- **Write tests first:** `status_test.go` w/ `fake.Runner` — issue links (`closingIssuesReferences` ∪ body `#NN`), unanswered-thread detection (`reviewThreads(isResolved:false)`, latest author ≠ PR author, bots excluded), `Render` table + `--json`.
- **Implement:** the `Row` type + `Enrich` (pure over fetched data) + `Render`.
- **Done-when:** `go test ./internal/status/...` green; `Render` table byte-stable.

### Phase 6 — `cmd/tick` lifecycle integration (binds it all)
- **Write tests first:** `cmd/tick_test.go` — in-process, `fake.Runner` + temp `--state-dir`, drive the ordered fixture sequence (open(draft) → ready-for-review → push → pending → CLEAN → behind → update → CLEAN → merged); assert printed transcript == golden; restart-resume (no replay); exit-10 empty-set signal. Also `cmd/{snapshot,diff,label,status,version}_test.go`.
- **Implement:** wire the commands per §3 pseudocode; `tick` persists **before** printing.
- **Done-when:** `go test ./cmd/...` green; `prping tick` on the empty-set fixture exits 10.

### Phase 7 — Packaging / install / docs / skill
- **Write:** `install.sh` block (after the gsl block, line 400), `scripts/test.sh` COVERAGE_MIN entry, `sdk/GEMINI.md` row, `.golangci.yml` comment, `skill/SKILL.md`, `GEMINI.md` + `CLAUDE.md` symlink, `README.md`.
- **Done-when:** `scripts/test.sh` reports `prping` ≥70%; `make lint-go` clean for prping; `install.sh` builds `~/opt/bin/prping`; running `sync-skills` links `prping` into `~/.claude/skills` + `~/.agents/skills`; §9.6 trigger-eval recorded; §9.7 manual sign-off on PR #127.

## 5. Verification mapping

**Automated gates (CI, via `scripts/test.sh` Go discovery — green = these pass):**

§8-rule → named `internal/diff` golden case (carried forward and adapted from the superseded plan's §7 table; cases now live in `internal/diff/diff_test.go` + `testdata/golden/`):

| §8 rule | Fires case(s) | Must-NOT-fire case(s) |
| :-- | :-- | :-- |
| §8.1 PR opened | `opened_first_sight` | `opened_already_present`, `opened_draft`, `opened_clean_no_ready` |
| §8.2 Push landed | `push_sha_advance`, `push_force_push_nondescendant` | `push_same_sha` |
| §8.3 Ready to merge | `ready_behind_to_clean` | `ready_clean_to_clean`, `ready_draft`, `ready_unknown_flap` |
| §8.4 Needs update | `behind_green` | `behind_already`, `behind_with_failing` |
| §8.5 Check failed | `check_new_failure`, `check_clear_then_recur` | `check_same_set`, `check_empty_refill_flap` |
| §8.6 Merged / closed | `merged_state_transition`, `closed_not_merged` | `closed_no_event_after` |
| §8.7 Idempotence / no-op / order / restart / format / totality | `idempotent_now_now`, `noop_prev_eq_now`, `order_multi_pr_one_tick`, `restart_resume_no_replay`, `format_line_len_and_oneline`, `total_absent_prev_seed`, `total_unknown_mergestate`, `total_null_checks` | — |
| §8.8 Precedence | `precedence_behind_and_failing` | — |
| Sanitization | `sanitize_newline_in_title`, `sanitize_ansi_in_title` | — |

Other automated rows:

| Spec ref | Named test | Package |
| :-- | :-- | :-- |
| §9.2 snapshot extraction | `snapshot_test.go`: per-status, multi, draft, conflicting, UNKNOWN, missing-fields, scope-filter, `--state all` | `internal/snapshot` |
| §9.3 state | `state_test.go`: atomic-write, perms-0700/0600, lock-second-writer-noop, `filename_slash_to_hyphen`, `filename_no_collision`, seed-silent-{absent,empty,malformed} | `internal/state` |
| §9.4 cmd/tick lifecycle | `tick_test.go`: `lifecycle_transcript_golden`, `restart_resume_no_replay`, `exit10_empty_set` | `cmd` |
| §9.5 coverage gate | `COVERAGE_MIN[prping]=70` enforced by `scripts/test.sh` | — |

**Human-evidenced gates (NOT CI-checkable — kept explicitly separate; recorded in PR #127):**

| Spec ref | Evidence |
| :-- | :-- |
| §9.6 skill-trigger eval | phrasings→expected corpus; **≥90% top-1** number recorded; tune the SKILL.md description, not behavior; fallback = add an explicit trigger phrase (do not soften the gate). |
| §9.7 manual phone-acceptance | Start `prping` in a Remote-Control session, push to a watched PR, confirm the phone push (suppressed while typing); checklist in `SKILL.md`; one sign-off on the PR. |

**Privacy/fixture attribution (carried from the superseded §6, verified against `ai/hooks/privacy_guard.sh`):** all `testdata/**` uses synthetic identity-free identifiers (`acme/widgets`, `feature/x`, fake titles/SHAs). The committer login as a bare ≥3-char word trips **Rule C** (word-bounded); `/home/<login>` paths trip **Rule B** (use `$HOME`/`~`); the §3.3 sample literal `acme/widgets` trips nothing — using it is consistency/hygiene. Never embed real logins, `$HOME` basenames, or tokens in fixtures.

## 6. Integration & rollout

- **install.sh:** new build+install block **after the gsl block — the last sdk build block — ending at line 400** (closing `fi`), **before the `# Configure the Nerd Font` comment at line 402**, byte-mirroring the gss block (356–365) with `gss`→`prping`. Builds to `~/opt/bin/prping`; prints `prping version`. (The sdk build sequence is gss → tmux-mgr (ends 378) → wol (ends 389) → gsl (ends 400); gsl, not tmux-mgr, is the last block.)
- **scripts/test.sh:** add `[prping]=70` after `[wol]=60` (line 45). Module discovery already globs `sdk/*/go.mod` — no discovery edit; CI runs prping via the existing Go path. **No `make shell-test` / `lint-shell` change** (the bash plan's edits are discarded; the Go form needs none). **`[prping]=70` lands in warn-only mode initially** (this repo's `COVERAGE_ENFORCE` defaults to `0`, as the lines 28–45 comment block documents for the under-floor wol/tmux-mgr entries). Because `internal/diff` and `internal/snapshot` are pure/nearly-pure and `internal/diff` targets ≥85%, the 70% floor should clear from the **first green build** — no separate coverage-backfill issue is needed, unlike tmux-mgr/wol.
- **sdk/GEMINI.md:** add the `prping/` row to the `## Modules` table after `tmux-mgr`.
- **.golangci.yml:** update the "FOUR … modules" comment (lines 9–10) to "FIVE …, sdk/prping". `make lint-go` auto-discovers per-module.
- **sync-skills:** no change — `sdk/prping/skill/SKILL.md` is auto-discovered and linked as bare `prping`. Verify with a real `sync-skills` run + `ls -l ~/.claude/skills/prping ~/.agents/skills/prping` (no `--dry-run` exists).
- **Docs convention:** `sdk/prping/GEMINI.md` + `ln -s GEMINI.md CLAUDE.md`.
- **Rollback (§11):** pure addition — `rm -rf sdk/prping ~/.config/prping`; revert the install.sh / scripts/test.sh / sdk/GEMINI.md / .golangci.yml edits. Nothing else touched.

### 6.1 Build leaves / DAG

Authoritative dependency graph for parallel `gss feature` workers. `internal/gh` is the **blocking** interface leaf every runner-consuming package imports; once its `Runner` interface (§3) is frozen, the rest are parallel-ish behind it. **Domain types live in their owning packages** (`Snapshot`→snapshot, `Event`→diff, `Row`→status), so the PURE `internal/diff` consumes the `Snapshot` type from **L2 (snapshot)**, never from L1 (gh). Edge `A → B` = "B depends on A's interface/type".

| Leaf | Owns (paths) | Consumes (in-edges) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| L0 scaffold+version | `main.go`, `cmd/root.go`, `cmd/version.go`, `internal/version/**`, `go.mod`, `build.sh`, `VERSION`, `LICENSE` | — | `go build ./... && go test ./internal/version/... && ~/opt/bin/prping version` | yes (base) |
| L1 gh runner+fake | `internal/gh/**` (incl. `fake/`) — **only the §3 `Runner` interface + `SystemRunner` + `fake.Runner`** (gss `internal/git` analog; NO domain types) | L0 | `go test ./internal/gh/...`; `var _ gh.Runner = (*fake.Runner)(nil)` compiles | **yes** (interface leaf) |
| L2 snapshot | `internal/snapshot/**` (**owns `Snapshot`/`PR`/`Scope` types**) | L1 | `go test ./internal/snapshot/...` byte-stable | no (type-defining for L3/L4/L5/L6) |
| L3 diff (heart) | `internal/diff/**` (+ `testdata/golden/`, **owns `Event`**) | **L2 (`Snapshot` type only) — NOT L1** | `go test ./internal/diff/...` green; every §8 rule → named case; ≥85% | no |
| L4 state | `internal/state/**` | **L2 (`Snapshot` type)** | `go test ./internal/state/...`; perms 0700/0600 asserted | no |
| L5 status (UC3) | `internal/status/**` (**owns `Row`**), `cmd/status.go` (+test) | L1, L2 | `go test ./internal/status/... ./cmd -run Status` | no |
| L6 cmd/tick lifecycle | `cmd/{tick,snapshot,diff,label}.go` (+ `*_test.go`) | L1, L2, L3, L4 | `go test ./cmd/...`; empty-set ⇒ exit 10; transcript==golden | no (integration) |
| L7 packaging+skill+docs | `install.sh` block, `scripts/test.sh` COVERAGE_MIN, `sdk/GEMINI.md`, `.golangci.yml`, `skill/SKILL.md`, `GEMINI.md`+symlink, `README.md` | L6 | `scripts/test.sh` ≥70% prping; `make lint-go` clean; `sync-skills` links `prping`; §9.6/§9.7 recorded | no |

DAG (acyclic): `L0 → L1 → L2 → {L3, L4} ; {L1,L2} → L5 ; {L1,L2,L3,L4} → L6 → L7`. L1 (the `Runner` interface) is built first (Interface-First); **L2 (snapshot) is the type-defining leaf the PURE L3/L4 depend on** — so L2 unblocks L3/L4, and `internal/diff` (L3) imports `internal/snapshot` (L2) but **never** `internal/gh` (L1). L5/L6 join; L7 closes. This table is mirrored (not re-authored) into the design-issue body as the worker DAG (`--base` = the in-edge) per the `mbo-plan` skill.

> Produced via `superpowers:writing-plans`. Execute with `superpowers:executing-plans` / `subagent-driven-development`, TDD throughout. Update `../index.md` state as it moves.
