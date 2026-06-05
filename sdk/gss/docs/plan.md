# gss v1.0 — Execution Plan

This plan turns [`design.md`](./design.md) into a stacked PR series. It
follows the design's [TDD implementation order](./design.md#tdd-implementation-order)
and absorbs every [Pre-v1 hardening checklist](./design.md#pre-v1-hardening-checklist)
item and every Design-review resolution (#1–#22) as a concrete
deliverable.

**Out of scope** for this plan:
- v1.1 (`sessions[]` append-only history — see [Roadmap](./design.md#roadmap)).
- Everything in [`roadmap.md`](./roadmap.md) (WORKER.md secret scanner,
  cross-machine sync hardening).

## How to use this document

- Every numbered PR-NN below is a single stacked PR sized for ≤ 20 min
  human review.
- Each PR pins: design anchor + line range, files added/modified,
  tests-first (TDD per [`src/CLAUDE.md`](../../CLAUDE.md)), acceptance
  criteria, upstream PR it stacks on, reviewer personas.
- Reviewer personas come from
  `/Users/eraigosa/GitHub/playground/.agents/workflows/` — subagents
  adopt them by reading the relevant file first.
- The executor flips each `- [ ]` to `- [x]` as PRs merge into
  `test_gss` (in `~/GitHub/playground`). Final integration PR opens
  against `dotfiles:main` only after all deliverables are merged.

## Phase 3 prerequisites (install before "execute the plan" fires)

Tooling not currently present on this machine; add before Phase 3
starts. Each install is one command; they are not deliverables, just
gate checks.

| Tool | Install | First used in | Why |
|------|---------|--------------|-----|
| `go-licenses` | `go install github.com/google/go-licenses@latest` | PR-50 | License CI gate per [src/GEMINI.md → Library standards](../../src/GEMINI.md). |
| `git-machete` | `brew install git-machete` | PR-49 | Companion rebase skill the agent invokes on conflicts. |
| `gofrs/flock` (Go module) | added via `go get` inside PR-18 | PR-18 | Registry advisory lock. |
| `gopkg.in/yaml.v3` (Go module) | added via `go get` inside PR-05 | PR-05 | Config YAML parsing. |

GitHub identity: the active `gh` account on this machine is
`sfc-gh-eraigosa`. Confirm before opening Phase 3 PRs whether
that's the correct identity for the `test_gss` branch and the
final dotfiles PR, or whether to re-auth.

---

## Resolutions coverage table

Every resolution from the [Design review](./design.md#design-review-deep-pass)
maps to ≥ 1 deliverable. Reviewers verify v1.0 coverage by skimming
this table alone.

| # | Resolution (one-liner) | Deliverable PR(s) |
|---|------------------------|-------------------|
| 1 | `worker_ref` canonical grammar | PR-07, PR-33 |
| 2 | Identifier validation grammar (regex + control-char strip) | PR-08, PR-33, PR-37 |
| 3 | YAML library pinned (`gopkg.in/yaml.v3`) | PR-05 |
| 4 | `internal/errors/` sentinels + exit-code map | PR-01 |
| 5 | Test seams (`git.Runner`, `gh.Client`, `Clock`, `RNG`) | PR-02, PR-03, PR-05, PR-07 |
| 6 | Build-time version via `internal/version` + ldflags | PR-04 |
| 7 | `internal/tmpl/tmpl.go` with `//go:embed` | PR-32 |
| 8 | `spawned_by` informational-only | PR-33 |
| 9 | Mode detection: classic verbs in worker cwd → `ErrWrongMode` | PR-22, PR-23, PR-26 |
| 10 | `registry.json` 0600, flock + atomic rename, uid check | PR-18 |
| 11 | `gss feature pr --ready` requires approval token | PR-39 |
| 12 | NWO cache + invalidation | PR-16 |
| 13 | `Backend.Enumerate(root)` added to interface | PR-20 |
| 14 | Companion CLI pin & verb allowlist (git-machete) | PR-49 |
| 15 | Pinned external deps + `go-licenses` CI gate | PR-50 |
| 16 | Auto-promote-on-merge: linear stacks only (question A) | PR-43 |
| 17 | Auto-promote requires `restack_count == 0` (question B) | PR-40, PR-43 |
| 18 | WORKER.md secret scanning → roadmap (question C) | PR-31 (verifies `roadmap.md` committed) |
| 19 | Empty-feature template byte-diff cleanup (question D) | PR-42 |
| 20 | Cross-machine sync: best-effort + `gss feature audit` (question E) | PR-44 |
| 21 | `tmux-mgr internal migrate-to-gss` full migrator (question F) | PR-58 |
| 22 | `--force-autonomous` in worker cwd → `ErrWrongMode` (question G) | PR-23, PR-51 |

---

## Parallel batches

The dependency graph: every later deliverable depends on the
foundation, but within several mid-stack batches, deliverables share
no upstream and can run in parallel under the 5-agent concurrency cap.

```
Batch A  (sequential foundation, 1 active subagent at a time)
  PR-01  internal/errors
   └─ PR-02  internal/git Runner
        └─ PR-03  internal/gh Client
             └─ PR-04  internal/version + build.sh
                  └─ PR-05  internal/config (YAML lib pin, Clock)
                       └─ PR-06  internal/identity/wordlist
                            └─ PR-07  internal/identity/suffix (RNG)
                                 └─ PR-08  internal/identity/user + purpose
                                      └─ PR-09  internal/repo (NWO)

Batch B  (classic primitives, up to 5 in parallel after Batch A)
  PR-10  internal/approval     ┐
  PR-11  internal/backup       │  4 PRs fan out from PR-09;
  PR-12  internal/sync         │  serialise on QA + reviewer
  PR-13  internal/scan         ┘  capacity, not on code deps
  PR-14  internal/status

Batch C  (cobra classic leaves — depend on relevant primitive)
  PR-15  cmd/gss/status.go + diff.go + version.go (bundle, ≤20m total)
  PR-16  cmd/gss/scan.go + backup.go + sync.go (bundle)
   └─ depend on PR-10..PR-14
   └─ may proceed in parallel with each other

Batch D  (registry + worktree backend, sequential)
  PR-17  internal/registry/schema.go (structs, JSON round-trip)
   └─ PR-18  internal/registry/lock.go (flock + atomic rename + 0600)
        └─ PR-19  internal/registry/reconcile.go
             └─ PR-20  internal/worktree backend interface + contract suite
                  └─ PR-21  internal/worktree/git impl

Batch E  (classic orchestration, sequential after Batches A+B+D)
  PR-22  internal/classic/push.go orchestrator
   └─ PR-23  cmd/gss/push.go cobra leaf (mode detect + ErrWrongMode + --force-autonomous gate)
        └─ PR-24  internal/classic/pr.go orchestrator
             └─ PR-25  cmd/gss/pr.go cobra leaf
                  └─ PR-26  Mode-detection shared helper + tests

Batch F  (stack + tmpl, parallel)
  PR-27  internal/stack/stack.go (parent/child computation)  ┐
  PR-28  internal/stack/body.go (markers + strip pass)       │  fan out
  PR-29  internal/stack/restack.go (re-target math)          │  3-wide
                                                              │
  PR-30  internal/tmpl renderer Service struct               │
  PR-31  internal/tmpl tmpl.go + *.md.tmpl embed             ┘

Batch G  (feature orchestrators, mostly sequential by topic)
  PR-32  internal/feature start + worker.go (add + update)
   └─ PR-33  internal/feature/list.go (+ --json + spawned_by informational test)
        └─ PR-34  internal/feature/conflicts.go
             └─ PR-35  internal/feature/checkpoint.go
                  └─ PR-36  internal/feature/auto.go (--auto + --dry-run)
                       └─ PR-37  internal/feature/pr.go (--ready w/ approval gate)
                            └─ PR-38  internal/feature/rebase.go
                                 └─ PR-39  internal/feature/restack.go (restack_count++)
                                      └─ PR-40  internal/feature/done.go (template-diff cleanup)
                                           └─ PR-41  internal/feature/merged.go (linear+restack_count gate)
                                                └─ PR-42  internal/feature/audit.go (read-only + --repair)
                                                     └─ PR-43  audit edge-case tests

Batch H  (cobra wiring for feature subtree)
  PR-44  cmd/gss/feature/feature.go parent + Register
   └─ PR-45  cmd/gss/feature/{start,worker,list}.go bundle
        └─ PR-46  cmd/gss/feature/{checkpoint,conflicts,pr,rebase,restack}.go bundle
             └─ PR-47  cmd/gss/feature/{done,merged,audit}.go bundle
                  └─ PR-48  cmd/gss/main.go + cmd/gss/config/ register both subtrees

Batch I  (hardening, mostly parallel after Batch H)
  PR-49  src/git-machete/skill/SKILL.md (companion CLI pin)
  PR-50  build.sh + CI: `go-licenses check ./...` gate
  PR-51  ai/hooks/safety_guard.sh + safety_guard_test.sh extension
  PR-52  sdk/gss/skill/SKILL.md (worker / power-user surface)

Batch J  (tmux-mgr refactor, sequential)
  PR-53  tmux-mgr pkg/agent/session.go schema update (+WorkerRef, -RepoRoot)
   └─ PR-54  tmux-mgr cmd/internal.go + pane-wrap shim
        └─ PR-55  tmux-mgr cmd/agent.go runAgentStart shells out to gss
             └─ PR-56  tmux-mgr cmd/agent.go runAgentCleanup shells out to gss
                  └─ PR-57  tmux-mgr feature start / add-agent / status convenience verbs
                       └─ PR-58  tmux-mgr internal migrate-to-gss (full migrator)
                            └─ PR-59  Delete tmux-mgr pkg/workspace/
                                 └─ PR-60  tmux-mgr SKILL.md updates (no gss leakage)

Batch K  (release)
  PR-61  Final integration PR: test_gss → dotfiles:main + release note
```

**Concurrency hint**: Batches B, F, and I have natural fan-out points
where up to 4–5 subagents can run in parallel against independent
deliverables; everything else serialises. The orchestrator session
itself does not count toward the 5-agent cap.

---

## Rework protocol

Per [`design.md` → Next steps](./design.md#next-steps):

- Address feedback as a new commit on the existing PR branch — never
  collapse history.
- If the change is structural and affects downstream PRs, mark them
  `- [ ] (needs restack)` in this plan, restack on the updated parent
  via `git rebase --update-refs` (or `git-machete` once installed),
  `git push --force-with-lease`, and refresh each downstream PR
  body's stack cross-links.
- Never re-order the stack without explicit human approval.

---

## Deliverables

### Batch A — Foundation (sequential)

#### PR-01: `internal/errors/` sentinels + exit-code map
- [ ] Design: [Code layout → errors](./design.md#code-layout); resolution #4.
- **Files**: `sdk/gss/internal/errors/errors.go`, `exitcodes.go`, `errors_test.go`.
- **Tests first**: sentinel identity (`errors.Is`/`As`), exit-code
  map completeness, JSON error envelope round-trip.
- **Acceptance**: every sentinel from the design table present
  (`ErrRebaseConflict`, `ErrAuthRequired`, `ErrDirtyWorktree`,
  `ErrLockHeld`, `ErrRegistryStale`, `ErrNotInWorker`,
  `ErrPRReadyNeedsToken`, `ErrInvalidIdent`, `ErrWrongMode`,
  `ErrConflictMarker`); exit-code table stable; coverage ≥ 60 %.
- **Stacks on**: (root of `test_gss`).
- **Reviewers**: Lead Developer, QA, Security (sentinels touch the
  safety surface).

#### PR-02: `internal/git/exec.go` Runner interface + recording fake
- [ ] Design: [Test seams](./design.md#test-seams-interfaces-every-external-boundary-flows-through); resolution #5.
- **Files**: `sdk/gss/internal/git/exec.go`, `exec_test.go`,
  `fake/runner.go`.
- **Tests first**: fake Runner captures args; real Runner shells out
  to `git`; context cancellation propagates.
- **Acceptance**: every higher package can import the interface and
  the fake; no `os/exec` direct calls anywhere above this package.
- **Stacks on**: PR-01.
- **Reviewers**: Lead Developer, QA, Architect (new package boundary).

#### PR-03: `internal/gh/exec.go` Client interface + recording fake
- [ ] Design: [Test seams](./design.md#test-seams-interfaces-every-external-boundary-flows-through),
  [GitHub interaction](./design.md#github-interaction--gh-only); resolution #5.
- **Files**: `sdk/gss/internal/gh/exec.go`, `pr.go` (types),
  `repo.go` (types), `exec_test.go`, `fake/client.go`.
- **Tests first**: fake supports stateful PR transitions
  (draft→ready, base edit, view returns scripted responses).
- **Acceptance**: `gh.Client` interface has `PRCreate`, `PREdit`,
  `PRReady`, `PRView`, `PRList`, `RepoView`, `AuthStatus`; fake is
  scriptable from a `testdata/gh_responses/*.json` fixture set.
- **Stacks on**: PR-02.
- **Reviewers**: Lead Developer, QA, Architect.

#### PR-04: `internal/version/` + `build.sh` ldflags update
- [ ] Design: [Build-time version](./design.md#build-time-version); resolution #6.
- **Files**: `sdk/gss/internal/version/version.go`,
  `version_test.go`, modified `sdk/gss/build.sh`.
- **Tests first**: `version.go` exports `Version`, `Commit`,
  `BuildDate`, `Dirty` set via ldflags; default empty when not set.
- **Acceptance**: `build.sh` writes the four ldflag targets via
  `-X .../internal/version.<var>`; existing `gss version` command
  reads from this package (no `main.commit` etc.).
- **Stacks on**: PR-03.
- **Reviewers**: Lead Developer, QA, DevOps Engineer (build script
  change).

#### PR-05: `internal/config/` YAML + env + flag merge
- [ ] Design: [Configuration](./design.md#configuration); resolution #3, #5.
- **Files**: `sdk/gss/internal/config/config.go`, `config_test.go`,
  `clock.go` (Clock interface + system impl).
- **Tests first**: precedence order built-in → YAML → env → flag;
  invalid YAML rejected with structured error;
  `GSS_BEHAVIOR_FORCE_WITH_LEASE=false` overrides default;
  `Clock.Now()` injectable.
- **Acceptance**: `gopkg.in/yaml.v3` pinned in `go.mod` (Apache-2.0/MIT
  dual); LICENSE URL cited in the PR description; first-run stub
  generation works; `gss config print` dumps effective config.
- **Stacks on**: PR-04.
- **Reviewers**: Lead Developer, QA, Security (new external dep).

#### PR-06: `internal/identity/wordlist.go` + `wordlist.txt` (256 words)
- [ ] Design: [Suffix wordlist](./design.md#suffix-wordlist).
- **Files**: `sdk/gss/internal/identity/wordlist.go`,
  `wordlist.txt`, `wordlist_test.go`.
- **Tests first**: `count == 256`, all entries `3 ≤ len ≤ 5`,
  lowercase ASCII only, no duplicates.
- **Acceptance**: `//go:embed wordlist.txt` succeeds; the embedded
  list passes the test; `Words()` accessor exists.
- **Stacks on**: PR-05.
- **Reviewers**: Lead Developer, QA.

#### PR-07: `internal/identity/suffix.go` (RNG-injected draw + 5-retry)
- [ ] Design: [Uniqueness](./design.md#uniqueness),
  [Worker reference](./design.md#worker-reference-worker_ref); resolution #1, #5.
- **Files**: `sdk/gss/internal/identity/suffix.go`,
  `suffix_test.go`, `rng.go` (RNG interface + crypto/rand impl).
- **Tests first**: RNG injection lets tests pin the draw sequence;
  `--suffix` rejects caller-supplied values; collision retry tops out
  at 5 attempts then errors; `worker_ref` formatter / parser
  round-trip for every form (`feature/user/purpose`,
  `feature/user/purpose-suffix`).
- **Acceptance**: every call site in higher packages will use this
  package's `WorkerRef` type (no string-typed refs).
- **Stacks on**: PR-06.
- **Reviewers**: Lead Developer, QA, Security (worker_ref is the
  cross-tool identifier).

#### PR-08: `internal/identity/user.go` + `purpose.go` (resolution + validation)
- [ ] Design: [Worker identity](./design.md#worker-identity),
  [Validation grammar](./design.md#validation-grammar); resolution #1, #2.
- **Files**: `sdk/gss/internal/identity/user.go`, `purpose.go`,
  `validate.go`, `validate_test.go`.
- **Tests first**: regex `^[a-z][a-z0-9-]{0,30}[a-z0-9]$` enforced
  for `<feature>`, `<user>`, `<purpose>`; `<purpose>` rejected if it
  collides with the wordlist; user resolution precedence
  (`gh login` → email-slug → `$USER` → `--user`); `--description`
  rejects control chars except space, NFC-normalises, counts code
  points.
- **Acceptance**: every reject path has a named sentinel
  (`ErrInvalidIdent`) and a test.
- **Stacks on**: PR-07.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-09: `internal/repo/` NWO detection + cache
- [ ] Design: [Repo identity](./design.md#repo-identity-name-with-owner); resolution #12.
- **Files**: `sdk/gss/internal/repo/nwo.go`, `nwo_test.go`,
  `cache.go`.
- **Tests first**: NWO resolved via `gh.Client.RepoView` (using
  fake); cache hit on second call; cache invalidated when
  `git remote get-url origin` diverges; `--repo <owner>/<repo>` is
  read-only shadow.
- **Acceptance**: cache file at `<worktrees_root>/.nwo` round-trips;
  refusal path when neither `gh` nor `origin` resolves NWO.
- **Stacks on**: PR-08.
- **Reviewers**: Lead Developer, QA.

### Batch B — Classic primitives (fan out after PR-09)

#### PR-10: `internal/approval/` token handshake
- [ ] Design: [Existing features → approval token](./design.md#safety-primitives-that-must-survive-verbatim).
- **Files**: `sdk/gss/internal/approval/approval.go`,
  `approval_test.go`.
- **Tests first**: token at `~/.config/gss/approval.token` is
  HEAD-SHA-bound, verified-then-consumed; missing token errors with
  `ErrPRReadyNeedsToken`; HEAD mismatch errors with `ErrLockHeld`-ish
  sentinel; `--force-autonomous` bypass path.
- **Acceptance**: bit-identical semantics to current
  `sdk/gss/cmd/push.go` token handling, regression-tested against
  golden output from PR-15.
- **Stacks on**: PR-09.
- **Reviewers**: Lead Developer, QA, Security (safety primitive).

#### PR-11: `internal/backup/` `backup/gss-TIMESTAMP` branch
- [ ] Design: [Safety primitives](./design.md#safety-primitives-that-must-survive-verbatim).
- **Files**: `sdk/gss/internal/backup/backup.go`, `backup_test.go`.
- **Tests first**: branch name template fixed-format; uses Clock
  interface for timestamp; idempotent rerun appends a
  monotonic suffix; uses git.Runner.
- **Acceptance**: byte-identical output to current
  `sdk/gss/cmd/backup.go` for the same input clock value.
- **Stacks on**: PR-09.
- **Reviewers**: Lead Developer, QA.

#### PR-12: `internal/sync/` fetch + pull --rebase
- [ ] Design: [Safety primitives](./design.md#safety-primitives-that-must-survive-verbatim).
- **Files**: `sdk/gss/internal/sync/sync.go`, `sync_test.go`.
- **Tests first**: fetch precedes pull; conflict surfaces as
  `ErrRebaseConflict`; non-fast-forward surfaces clean diagnostic.
- **Acceptance**: regression-tested against current `sdk/gss/cmd/sync.go`.
- **Stacks on**: PR-09.
- **Reviewers**: Lead Developer, QA.

#### PR-13: `internal/scan/` dirty-repo walker
- [ ] Design: [Existing features](./design.md#existing-features-that-must-survive-the-refactor).
- **Files**: `sdk/gss/internal/scan/scan.go`, `scan_test.go`,
  `testdata/scan/{clean,dirty,nested}/...`.
- **Tests first**: walks recursively; respects symlink loops; reports
  `[DIRTY]` prefix exactly as today (slash-command grep contract).
- **Acceptance**: golden output for the three testdata trees stable.
- **Stacks on**: PR-09.
- **Reviewers**: Lead Developer, QA, Tech Writer (slash-command
  output contract).

#### PR-14: `internal/status/` porcelain status formatter
- [ ] Design: [Existing features](./design.md#existing-features-that-must-survive-the-refactor).
- **Files**: `sdk/gss/internal/status/status.go`, `status_test.go`.
- **Tests first**: empty-tree case prints "No changes detected";
  dirty case lists file paths; column alignment stable for
  slash-command consumers.
- **Acceptance**: byte-identical to current `sdk/gss/cmd/status.go`
  for matching fixtures.
- **Stacks on**: PR-09.
- **Reviewers**: Lead Developer, QA, Tech Writer.

### Batch C — Classic cobra leaves (parallel after relevant primitives)

#### PR-15: `cmd/gss/{status,diff,version}.go` cobra leaves
- [ ] Design: [Code layout](./design.md#code-layout).
- **Files**: `sdk/gss/cmd/gss/status.go`, `diff.go`, `version.go`,
  `testdata/golden/classic/{status,diff,version}/*.txt`.
- **Tests first**: golden-output snapshots for each command's
  representative inputs; `version --json` schema stable.
- **Acceptance**: byte-identical output to today's binary against
  the golden fixtures; "Stable output strings" subsection of this
  plan lists every load-bearing substring (see [Stable output
  strings](#stable-output-strings) below).
- **Stacks on**: PR-14.
- **Reviewers**: Lead Developer, QA, Tech Writer (slash-command
  output contract).

#### PR-16: `cmd/gss/{scan,backup,sync}.go` cobra leaves
- [ ] Design: [Code layout](./design.md#code-layout).
- **Files**: `sdk/gss/cmd/gss/scan.go`, `backup.go`, `sync.go`,
  `testdata/golden/classic/{scan,backup,sync}/*.txt`.
- **Tests first**: golden-output snapshots.
- **Acceptance**: byte-identical; slash commands `/gss-scan`, the
  internal `gss sync` invocation by `gss push`, and `gss backup`
  all see no behavioural change.
- **Stacks on**: PR-15.
- **Reviewers**: Lead Developer, QA, Tech Writer.

### Batch D — Registry + worktree backend (sequential)

#### PR-17: `internal/registry/schema.go` (structs + JSON round-trip)
- [ ] Design: [Filesystem layout](./design.md#filesystem-layout),
  [Description and provenance](./design.md#description-and-provenance-fields).
- **Files**: `sdk/gss/internal/registry/registry.go`, `schema.go`,
  `schema_test.go`.
- **Tests first**: schema_version handling; `Feature`, `Worker`,
  `SpawnedBy` structs round-trip; `restack_count` defaults to 0;
  unknown fields preserved on re-write (forward-compat with v1.1
  `sessions[]`).
- **Acceptance**: pre/post-write byte-equality test against a
  canonical fixture; reject on schema_version > supported.
- **Stacks on**: PR-09.
- **Reviewers**: Lead Developer, QA, Architect (new package).

#### PR-18: `internal/registry/lock.go` (flock + atomic rename + 0600)
- [ ] Design: [Decisions → registry locking](./design.md#decisions); resolution #10.
- **Files**: `sdk/gss/internal/registry/lock.go`, `lock_test.go`,
  `go.mod` (adds `github.com/gofrs/flock`).
- **Tests first**: `TestConcurrentWorkerAdd` (N goroutines, exactly
  one wins); `TestDoneRacingCheckpoint`; partial-write recovery via
  tmp-file + `rename(2)`; `0600` enforced on first write; refuse if
  `stat(registry).uid != geteuid()`.
- **Acceptance**: lock acquired on `<repo>/.registry.lock`;
  `gofrs/flock` BSD-3-Clause LICENSE URL cited in PR description.
- **Stacks on**: PR-17.
- **Reviewers**: Lead Developer, QA, Security (concurrency +
  filesystem permissions).

#### PR-19: `internal/registry/reconcile.go` (vs `git worktree`, `gh pr`)
- [ ] Design: [Filesystem layout](./design.md#filesystem-layout),
  [Conflict protection](./design.md#conflict-protection).
- **Files**: `sdk/gss/internal/registry/reconcile.go`,
  `reconcile_test.go`.
- **Tests first**: stale row dropped when worktree missing; PR-state
  refresh when `gh pr view` returns different state; reconcile is
  read-only (does not write back unless `--repair` is passed —
  audit-style); observable state wins over registry.
- **Acceptance**: tested against a fake gh.Client + temp registry +
  controlled git.Runner.
- **Stacks on**: PR-18.
- **Reviewers**: Lead Developer, QA.

#### PR-20: `internal/worktree/` backend interface + contract suite
- [ ] Design: [Worktree backend abstraction](./design.md#worktree-backend-abstraction); resolution #13.
- **Files**: `sdk/gss/internal/worktree/backend.go`,
  `registry.go` (Register / Open), `backendtest/contract.go`.
- **Tests first**: contract suite covers `Create`, `Remove`,
  `Enumerate`, `Status`; idempotency after partial-failure;
  `Backend.Name()` matches registered key.
- **Acceptance**: any future backend that passes
  `backendtest.RunContractSuite(t, factory)` is wire-compatible.
- **Stacks on**: PR-19.
- **Reviewers**: Lead Developer, QA, Architect (new package
  boundary).

#### PR-21: `internal/worktree/git/` v1 backend impl
- [ ] Design: [Worktree backend abstraction → v1 implementation](./design.md#v1-implementation-git-backend).
- **Files**: `sdk/gss/internal/worktree/git/git.go`,
  `git_test.go`.
- **Tests first**: runs the contract suite from PR-20 against the
  git backend; `Create` sets `rebase.updateRefs=true` on each new
  worktree.
- **Acceptance**: every contract-suite test green.
- **Stacks on**: PR-20.
- **Reviewers**: Lead Developer, QA.

### Batch E — Classic orchestration (sequential after A, B, D)

#### PR-22: `internal/classic/push.go` orchestrator
- [ ] Design: [Existing features → push](./design.md#existing-features-that-must-survive-the-refactor).
- **Files**: `sdk/gss/internal/classic/push.go`,
  `push_test.go`.
- **Tests first**: backup → sync → push → auto-PR flow; tests use
  the fake git.Runner and gh.Client.
- **Acceptance**: behavioural equivalence to current
  `sdk/gss/cmd/push.go` against the golden snapshot from PR-15
  (status / version are visible side effects).
- **Stacks on**: PR-21.
- **Reviewers**: Lead Developer, QA, Security (push path).

#### PR-23: `cmd/gss/push.go` cobra leaf with `ErrWrongMode` + `--force-autonomous` gate
- [ ] Design: [Plain `gss` (unchanged)](./design.md#command-surface),
  [Decisions → `--force-autonomous`](./design.md#decisions); resolution #9, #22.
- **Files**: `sdk/gss/cmd/gss/push.go`, `push_test.go`.
- **Tests first**: classic mode when cwd is a regular checkout;
  `ErrWrongMode` when cwd is a registered worker worktree;
  `--force-autonomous` from a worker worktree errors with
  `ErrWrongMode` exit code (one assert per condition).
- **Acceptance**: every combination of (mode × `--force-autonomous`)
  pinned in tests; slash command `/sync` invocations still pass.
- **Stacks on**: PR-22.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-24: `internal/classic/pr.go` orchestrator
- [ ] Design: [Existing features → pr](./design.md#existing-features-that-must-survive-the-refactor).
- **Files**: `sdk/gss/internal/classic/pr.go`, `pr_test.go`.
- **Tests first**: timestamped feature-branch generation; PR
  open/create via `gh.Client`; approval-token consumed.
- **Acceptance**: behavioural equivalence to current
  `sdk/gss/cmd/pr.go`.
- **Stacks on**: PR-23.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-25: `cmd/gss/pr.go` cobra leaf with mode detection
- [ ] Design: [Plain `gss` (unchanged)](./design.md#command-surface); resolution #9, #22.
- **Files**: `sdk/gss/cmd/gss/pr.go`, `pr_test.go`.
- **Tests first**: same matrix as PR-23 applied to `pr`.
- **Acceptance**: slash command `/gss-pr` invocations still pass.
- **Stacks on**: PR-24.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-26: Mode detection shared helper + cross-cutting tests
- [ ] Design: [Plain `gss` (unchanged)](./design.md#command-surface); resolution #9.
- **Files**: `sdk/gss/internal/mode/mode.go`, `mode_test.go`.
- **Tests first**: `IsInWorker(cwd, registry)` returns
  `(workerRef, true)` or `("", false)`; canonical implementation
  used by all classic cobra leaves.
- **Acceptance**: every classic leaf (status, push, pr, sync,
  backup, scan, diff, version) routes its mode check through this
  package; refactor PR-23/PR-25 to use it.
- **Stacks on**: PR-25.
- **Reviewers**: Lead Developer, QA, Architect.

### Batch F — Stack logic + templates (parallel)

#### PR-27: `internal/stack/stack.go` parent/child computation
- [ ] Design: [Stacked PRs](./design.md#stacked-prs).
- **Files**: `sdk/gss/internal/stack/stack.go`, `stack_test.go`.
- **Tests first**: parent/child walk; cycle detection;
  bottom-of-stack identification.
- **Acceptance**: pure-logic package, no external calls; coverage
  ≥ 80 %.
- **Stacks on**: PR-26.
- **Reviewers**: Lead Developer, QA.

#### PR-28: `internal/stack/body.go` PR body rendering + marker strip pass
- [ ] Design: [PR body — stack section](./design.md#pr-body--stack-section),
  [Hardening checklist → marker stripping](./design.md#pre-v1-hardening-checklist).
- **Files**: `sdk/gss/internal/stack/body.go`, `body_test.go`,
  `testdata/pr_body/{stack_bottom,middle,top,solo}.golden.md`.
- **Tests first**: `<!-- gss:stack-* -->` markers rewritten
  idempotently; render → re-render is byte-identical; user-authored
  marker tokens are stripped before stitching (defence against PR
  body injection per security-review #7).
- **Acceptance**: golden snapshots; round-trip property test.
- **Stacks on**: PR-27.
- **Reviewers**: Lead Developer, QA, Security (PR body injection
  surface).

#### PR-29: `internal/stack/restack.go` re-target math
- [ ] Design: [`gss feature merged`](./design.md#gss-feature-merged-worker-ref-or-auto-detect).
- **Files**: `sdk/gss/internal/stack/restack.go`,
  `restack_test.go`.
- **Tests first**: single-hop re-target on merge; recursive walk for
  multi-level stacks; cycle prevention; ordering: parents re-targeted
  before children's PR base is updated.
- **Acceptance**: deterministic ordering exhaustively tested.
- **Stacks on**: PR-27.
- **Reviewers**: Lead Developer, QA, Security (stack re-targeting
  touches the laundering surface).

#### PR-30: `internal/tmpl/` renderer Service
- [ ] Design: [FEATURE.md template](./design.md#featuremd-template),
  [WORKER.md template](./design.md#workermd-template).
- **Files**: `sdk/gss/internal/tmpl/render.go`, `render_test.go`.
- **Tests first**: feature.md rendering with substituted metadata;
  worker.md rendering; ANSI / control-char stripping applied to
  user-supplied fields (description, etc.) per hardening checklist.
- **Acceptance**: golden snapshots; control-character injection
  attempts produce harmless output.
- **Stacks on**: PR-27.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-31: `internal/tmpl/tmpl.go` + `*.md.tmpl` embed
- [ ] Design: [Code layout → internal/tmpl/tmpl.go](./design.md#code-layout); resolution #7, #18.
- **Files**: `sdk/gss/internal/tmpl/tmpl.go`,
  `feature.md.tmpl`, `worker.md.tmpl`.
- **Tests first**: embed FS contains both templates; loader returns
  expected content.
- **Acceptance**: package builds (would not without `tmpl.go`);
  PR description references `roadmap.md` to verify the
  secret-scanning roadmap doc is committed (resolution #18 has no
  code, only a doc commitment).
- **Stacks on**: PR-30.
- **Reviewers**: Lead Developer, QA.

### Batch G — Feature orchestrators (sequential)

#### PR-32: `internal/feature/start.go` + `worker.go` (add + update)
- [ ] Design: [`gss feature start`](./design.md#gss-feature-start-name---purpose-p---base-branch---goal-),
  [`gss feature worker add`](./design.md#gss-feature-worker-add-feature---purpose-p---description----base-branch---user-u---suffix---goal----engine-id---session-id-id---pane-id-id---tmux-mgr-session-id---spawned-by-json-obj).
- **Files**: `sdk/gss/internal/feature/start.go`, `worker.go`,
  `start_test.go`, `worker_test.go`.
- **Tests first**: `Service` struct with injected deps; `WorkerAdd`
  validates identifiers via PR-08; emits `worker_ref` per PR-07;
  refuses without `--description`; persists `spawned_by` verbatim;
  unique tuple enforced under registry lock from PR-18.
- **Acceptance**: created workers visible via PR-33 `list`.
- **Stacks on**: PR-31.
- **Reviewers**: Lead Developer, QA, Security (worker creation =
  worktree allocation).

#### PR-33: `internal/feature/list.go` + `spawned_by` informational assertion
- [ ] Design: [`gss feature list`](./design.md#gss-feature-list---feature-name---tree); resolution #8.
- **Files**: `sdk/gss/internal/feature/list.go`, `list_test.go`.
- **Tests first**: tree rendering; `--json` schema; `--with sessions`
  reserved for v1.1 (errors with "not yet implemented" in v1);
  static-grep test asserts no code path in `internal/` reads
  `spawned_by.engine` for branching (resolution #8 informational-only
  rule).
- **Acceptance**: list shows description by default; `--json` schema
  pinned in a snapshot.
- **Stacks on**: PR-32.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-34: `internal/feature/conflicts.go`
- [ ] Design: [`gss feature conflicts`](./design.md#gss-feature-conflicts---feature-name---json).
- **Files**: `sdk/gss/internal/feature/conflicts.go`,
  `conflicts_test.go`.
- **Tests first**: file-overlap detection across workers within a
  feature; `--json` schema; never auto-resolves.
- **Acceptance**: tested against multi-worker fixtures.
- **Stacks on**: PR-33.
- **Reviewers**: Lead Developer, QA.

#### PR-35: `internal/feature/checkpoint.go`
- [ ] Design: [`gss feature checkpoint`](./design.md#gss-feature-checkpoint---message-).
- **Files**: `sdk/gss/internal/feature/checkpoint.go`,
  `checkpoint_test.go`.
- **Tests first**: fetch + rebase on `base_branch`; first push via
  `gh pr create --draft --head`; subsequent push via
  `git push --force-with-lease` + `gh pr edit`; PR body rendered
  including stack section.
- **Acceptance**: tested against fake gh.Client with scripted
  stateful transitions.
- **Stacks on**: PR-34.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-36: `internal/feature/auto.go` (`--auto` + `--dry-run`)
- [ ] Design: [`gss feature checkpoint --auto`](./design.md#gss-feature-checkpoint---auto---worker-ref).
- **Files**: `sdk/gss/internal/feature/auto.go`, `auto_test.go`.
- **Tests first**: silent on no-op; WIP commit on dirty (tracked only,
  never `git add -A`); refuses prompts; only touches draft PRs; writes
  diagnostic to WORKER.md on skipped conditions; `--dry-run` prints
  planned actions without executing.
- **Acceptance**: idempotent on back-to-back runs; non-zero exit on
  conflict / detached HEAD.
- **Stacks on**: PR-35.
- **Reviewers**: Lead Developer, QA, Security (hook-invoked
  non-interactive path).

#### PR-37: `internal/feature/pr.go` (`--ready` with approval token gate)
- [ ] Design: [`gss feature pr`](./design.md#gss-feature-pr---ready); resolution #11.
- **Files**: `sdk/gss/internal/feature/pr.go`, `pr_test.go`.
- **Tests first**: `--ready` invokes `internal/approval` from PR-10;
  refuses without token (`ErrPRReadyNeedsToken`); promotion via
  `gh pr ready`.
- **Acceptance**: token-required path tested; no path promotes
  without consuming a token.
- **Stacks on**: PR-36.
- **Reviewers**: Lead Developer, QA, Security (promotion = trust
  boundary).

#### PR-38: `internal/feature/rebase.go`
- [ ] Design: [`gss feature rebase`](./design.md#gss-feature-rebase).
- **Files**: `sdk/gss/internal/feature/rebase.go`,
  `rebase_test.go`.
- **Tests first**: rebases on current `base_branch`; updates PR
  without full checkpoint render.
- **Acceptance**: pure rebase, no description rewrite.
- **Stacks on**: PR-37.
- **Reviewers**: Lead Developer, QA.

#### PR-39: `internal/feature/restack.go` (`restack_count` increment)
- [ ] Design: [`gss feature restack`](./design.md#gss-feature-restack-worker---onto-branch); resolution #17.
- **Files**: `sdk/gss/internal/feature/restack.go`,
  `restack_test.go`.
- **Tests first**: re-target a worker onto a new base; force-push
  with-lease; PR `base` updated; **`restack_count++`** in registry;
  descendants whose effective base moved get incremented too; a
  later restack back to the original base does NOT decrement.
- **Acceptance**: `TestRestackIncrementsCount` pins the invariant.
- **Stacks on**: PR-38.
- **Reviewers**: Lead Developer, QA, Security (laundering mitigation).

#### PR-40: `internal/feature/done.go` (template byte-diff cleanup)
- [ ] Design: [`gss feature done`](./design.md#gss-feature-done-worker-ref---force); resolution #19.
- **Files**: `sdk/gss/internal/feature/done.go`,
  `done_test.go`.
- **Tests first**: classic mode prompts on empty feature; worker
  (non-interactive) mode byte-compares `FEATURE.md` against
  rendered template; delete on match
  (`TestDoneOnEmptyFeatureMatchingTemplate`); silent retain + stderr
  notice on mismatch (`TestDoneOnEmptyFeatureWithEdits`);
  whitespace-only differences do not count as edits.
- **Acceptance**: both tests pin the resolution-#19 behaviour.
- **Stacks on**: PR-39.
- **Reviewers**: Lead Developer, QA.

#### PR-41: `internal/feature/merged.go` (linear + `restack_count == 0` gate)
- [ ] Design: [`gss feature merged`](./design.md#gss-feature-merged-worker-ref-or-auto-detect); resolution #16, #17.
- **Files**: `sdk/gss/internal/feature/merged.go`,
  `merged_test.go`.
- **Tests first**:
  - `TestMergedLinearStackPromotesChild` — 1 child,
    `restack_count == 0` → child auto-ready.
  - `TestMergedFanoutDoesNotPromote` — ≥ 2 children → mechanical
    re-target only, no promotion.
  - `TestMergedRestackedChildDoesNotPromote` —
    `restack_count > 0` on the single child → re-target only, no
    promotion + stderr notice naming the disqualification reason.
  - `TestMergedCascadesTwoLevels` — multi-level scripted gh.Client.
- **Acceptance**: all four tests green; ordering rule (re-target
  before promote) pinned.
- **Stacks on**: PR-40.
- **Reviewers**: Lead Developer, QA, Security (auto-promote =
  trust boundary).

#### PR-42: `internal/feature/audit.go` (read-only + `--repair`)
- [ ] Design: [`gss feature audit`](./design.md#gss-feature-audit---feature-name---repair---json); resolution #20.
- **Files**: `sdk/gss/internal/feature/audit.go`,
  `audit_test.go`.
- **Tests first**: every check in the audit matrix has a positive
  and a negative test (worktree exists / missing; branch exists /
  missing; PR exists / 404; PR base matches / diverged; stale child
  ref; valid / invalid `spawned_by`; schema_version match / mismatch);
  `--repair` is registry-local and never force-pushes, never calls
  `gh pr create/edit/ready`.
- **Acceptance**: read-only mode is provably side-effect-free
  (no writes to disk, no gh mutating calls); JSON report schema
  pinned.
- **Stacks on**: PR-41.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-43: Audit edge-case tests (cross-machine sync)
- [ ] Design: [Cross-machine sync](./roadmap.md#cross-machine-sync) (failure modes table).
- **Files**: `sdk/gss/internal/feature/audit_xmachine_test.go`,
  `testdata/audit/xmachine/*.json`.
- **Tests first**: each row of the cross-machine-sync failure-mode
  table reproduced as a fixture + audit assertion.
- **Acceptance**: audit's "observable state wins over registry" rule
  pinned across all five failure modes.
- **Stacks on**: PR-42.
- **Reviewers**: Lead Developer, QA, Security.

### Batch H — Cobra wiring for the feature subtree

#### PR-44: `cmd/gss/feature/feature.go` parent + `Register`
- [ ] Design: [Code layout → Cobra wiring pattern](./design.md#code-layout).
- **Files**: `sdk/gss/cmd/gss/feature/feature.go`.
- **Tests first**: `Register(parent)` attaches all leaves; nested
  `worker add | update` subcommands wired correctly.
- **Acceptance**: `gss feature --help` lists every verb.
- **Stacks on**: PR-43.
- **Reviewers**: Lead Developer, QA, Architect.

#### PR-45: `cmd/gss/feature/{start,worker,list}.go` cobra leaves
- [ ] Design: [Command surface](./design.md#command-surface).
- **Files**: three leaf files + tests.
- **Tests first**: flag parsing, args validation, `RunE` shim only.
- **Acceptance**: end-to-end smoke against a temp registry.
- **Stacks on**: PR-44.
- **Reviewers**: Lead Developer, QA.

#### PR-46: `cmd/gss/feature/{checkpoint,conflicts,pr,rebase,restack}.go`
- [ ] Design: [Command surface](./design.md#command-surface).
- **Files**: five leaf files + tests.
- **Tests first**: as PR-45.
- **Acceptance**: each leaf invokes the correct `internal/feature/`
  Service.
- **Stacks on**: PR-45.
- **Reviewers**: Lead Developer, QA.

#### PR-47: `cmd/gss/feature/{done,merged,audit}.go`
- [ ] Design: [Command surface](./design.md#command-surface).
- **Files**: three leaf files + tests.
- **Tests first**: as PR-45.
- **Acceptance**: as PR-45.
- **Stacks on**: PR-46.
- **Reviewers**: Lead Developer, QA.

#### PR-48: `cmd/gss/main.go` updates + `cmd/gss/config/`
- [ ] Design: [Code layout](./design.md#code-layout).
- **Files**: `sdk/gss/cmd/gss/main.go` (move from `sdk/gss/cmd/`),
  `sdk/gss/cmd/gss/config/config.go`, `config_test.go`,
  removal of old `sdk/gss/cmd/` flat layout.
- **Tests first**: `gss --help` exhaustive; `gss config print`
  dumps effective config; `gss config check` validates required
  tools.
- **Acceptance**: build via updated `build.sh` from PR-04 succeeds;
  smoke runs of every verb pass.
- **Stacks on**: PR-47.
- **Reviewers**: Lead Developer, QA, Architect (layout change).

### Batch I — Hardening (parallel after Batch H)

#### PR-49: `src/git-machete/skill/SKILL.md` companion CLI skill
- [ ] Design: [Companion rebase tooling](./design.md#companion-rebase-tooling); resolution #14.
- **Files**: `src/git-machete/skill/SKILL.md`.
- **Tests first**: none (doc); the skill must include LICENSE blob
  SHA citation, pinned version range, and verb allowlist.
- **Acceptance**: human review only — Tech Writer signs off on
  clarity and Security signs off on the verb allowlist.
- **Stacks on**: PR-48.
- **Reviewers**: Tech Writer, Security.

#### PR-50: `build.sh` + CI: `go-licenses check ./...` gate
- [ ] Design: [Pinned external dependencies](./design.md#pinned-external-dependencies); resolution #15.
- **Files**: `sdk/gss/build.sh`, optional CI workflow if applicable.
- **Tests first**: `build.sh` exits non-zero when a banned-license
  dependency is introduced; passes on the v1 dep set.
- **Acceptance**: v1 deps (cobra, yaml.v3, gofrs/flock, x/sys) all
  pass the check; LICENSE URLs cited in PR description.
- **Stacks on**: PR-48.
- **Reviewers**: Lead Developer, QA, DevOps, Security.

#### PR-51: `safety_guard.sh` + `safety_guard_test.sh` extension
- [ ] Design: [Hardening checklist → safety_guard](./design.md#pre-v1-hardening-checklist); resolution #22.
- **Files**: `ai/hooks/safety_guard.sh` (extended),
  `ai/hooks/safety_guard_test.sh` (extended).
- **Tests first**: per `CLAUDE.md`, add ≥ 1 `assert_exit 0` and
  ≥ 1 `assert_exit 2` per new pattern, **tests added before the
  hook regex**. New patterns gated:
  - `gss feature pr --ready` outside the two-call recipe.
  - `gss feature merged`.
  - `gss feature restack`.
  - `gss feature checkpoint` outside a worker cwd (classic
    invocation, dangerous).
  - `gss (push|pr) --force-autonomous` from a worker cwd
    (resolution #22).
- **Acceptance**: full hook test driver green; existing legitimate
  shapes still pass.
- **Stacks on**: PR-48.
- **Reviewers**: Security, Lead Developer, QA.

#### PR-52: `sdk/gss/skill/SKILL.md` worker / power-user surface
- [ ] Design: [Skill doc updates → sdk/gss/skill](./design.md#skill-doc-updates).
- **Files**: `sdk/gss/skill/SKILL.md`.
- **Tests first**: none (doc); covers classic + worker rules,
  stacked PR mental model, description hygiene, when to reach for
  the git-machete skill.
- **Acceptance**: Tech Writer signs off; references the design
  rather than duplicating it.
- **Stacks on**: PR-48.
- **Reviewers**: Tech Writer.

### Batch J — tmux-mgr refactor (sequential)

#### PR-53: `tmux-mgr` `Session` schema (`+WorkerRef`, `-RepoRoot`)
- [ ] Design: [tmux-mgr refactor plan → What changes](./design.md#what-changes).
- **Files**: `src/tmux-mgr/pkg/agent/session.go`,
  `session_test.go`.
- **Tests first**: round-trip with `WorkerRef`; absence of
  `RepoRoot`; back-compat read of existing session JSON files.
- **Acceptance**: `ListSessionsFiltered` re-derives repo identity
  from `WorkerRef`.
- **Stacks on**: PR-52.
- **Reviewers**: Lead Developer, QA, Architect.

#### PR-54: `tmux-mgr` `cmd/internal.go` + `pane-wrap` shim
- [ ] Design: [tmux-mgr refactor plan → What's new (1)](./design.md#whats-new); resolution #5 (close-hook).
- **Files**: `src/tmux-mgr/cmd/internal.go` (new),
  `src/tmux-mgr/cmd/pane_wrap.go`, tests.
- **Tests first**: shim execs the agent CLI; on exit runs
  `gss feature checkpoint --auto --worker <ref>` (mocked); exit
  code forwarded; signals propagated to child (PGID handling).
- **Acceptance**: end-to-end smoke under tmux with a `/bin/true`
  agent and a `gss` shim that records the invocation.
- **Stacks on**: PR-53.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-55: `tmux-mgr` `runAgentStart` shells out to `gss feature worker add`
- [ ] Design: [tmux-mgr refactor plan](./design.md#tmux-mgr-refactor-plan).
- **Files**: modified `src/tmux-mgr/cmd/agent.go`.
- **Tests first**: shells out with all required flags
  (`--purpose`, `--description`, `--user`, `--engine`,
  `--session-id`, `--pane-id`, `--tmux-mgr-session`, `--json`);
  parses returned JSON; stores `WorkerRef` in Session; launches
  pane via `pane-wrap` (PR-54).
- **Acceptance**: existing `tmux-mgr agent start` invocations from
  the agent skill still work, now backed by gss.
- **Stacks on**: PR-54.
- **Reviewers**: Lead Developer, QA, Architect.

#### PR-56: `tmux-mgr` `runAgentCleanup` shells out to `gss feature done`
- [ ] Design: [tmux-mgr refactor plan](./design.md#tmux-mgr-refactor-plan).
- **Files**: modified `src/tmux-mgr/cmd/agent.go`.
- **Tests first**: cleanup reads `WorkerRef` from session JSON;
  forwards `--force`; tmux pane killed afterwards.
- **Acceptance**: behavioural equivalence to current
  `runAgentCleanup` for a fresh end-to-end run.
- **Stacks on**: PR-55.
- **Reviewers**: Lead Developer, QA, Security.

#### PR-57: `tmux-mgr feature start | add-agent | status` convenience verbs
- [ ] Design: [tmux-mgr refactor plan → What's new (2,3,4)](./design.md#whats-new).
- **Files**: `src/tmux-mgr/cmd/feature.go` (new) + tests.
- **Tests first**: each verb wraps the equivalent gss feature call.
- **Acceptance**: orchestrator skill (PR-60) instructs agents to use
  these.
- **Stacks on**: PR-56.
- **Reviewers**: Lead Developer, QA, Architect.

#### PR-58: `tmux-mgr internal migrate-to-gss` full migrator
- [ ] Design: [tmux-mgr refactor plan → One-shot migration](./design.md#whats-new); resolution #21.
- **Files**: `src/tmux-mgr/cmd/migrate.go` + tests.
- **Tests first**: 9-step per-session procedure with a controlled
  legacy fixture set; `--dry-run` mode prints all planned actions
  without execution; idempotent re-runs read updated `WorkerRef`
  and skip; partial-failure logging continues processing remaining
  sessions.
- **Acceptance**: dry-run produces a deterministic plan against the
  fixture; real run leaves the migration log at
  `~/.config/gss/migrate-to-gss.log`.
- **Stacks on**: PR-57.
- **Reviewers**: Lead Developer, QA, Security (worktree moves +
  branch renames).

#### PR-59: Delete `tmux-mgr/pkg/workspace/`
- [ ] Design: [tmux-mgr refactor plan → What goes away](./design.md#what-goes-away).
- **Files**: removed `src/tmux-mgr/pkg/workspace/*`; removed
  `currentRepoRoot()` from `cmd/agent.go`; removed timestamp branch
  naming.
- **Tests first**: no test consumes `pkg/workspace`; grep guard in CI.
- **Acceptance**: tmux-mgr builds and tests green after deletion.
- **Stacks on**: PR-58 (migrator must run before delete).
- **Reviewers**: Lead Developer, QA, Architect.

#### PR-60: `tmux-mgr/skill/SKILL.md` updates (no gss leakage)
- [ ] Design: [Skill doc updates → src/tmux-mgr/skill](./design.md#srctmux-mgrskillskillmd--orchestrator-surface-only).
- **Files**: `src/tmux-mgr/skill/SKILL.md`.
- **Tests first**: none (doc); content checks: no `gss feature`
  mentions; "isolated working directory" replaces "git worktree";
  auto-checkpoint described as an observable side effect.
- **Acceptance**: Tech Writer signs off; Security signs off on the
  no-leakage claim.
- **Stacks on**: PR-59.
- **Reviewers**: Tech Writer, Security.

### Batch K — Release

#### PR-61: Final integration PR — `test_gss` → `dotfiles:main`
- [ ] Design: [Definition of Done](#definition-of-done).
- **Files**: aggregate diff from all merged deliverables, plus a
  one-page release / migration note at `sdk/gss/docs/RELEASE.md`.
- **Tests first**: full `go test ./...`, `go-licenses check ./...`,
  `safety_guard_test.sh`, and a smoke run of every classic slash
  command (`/sync`, `/gss-pr`, `/gss-scan`, `/gss`) against the new
  binary.
- **Acceptance**: every checklist item in this plan ticked; release
  note explains migration steps for any user upgrading from
  pre-refactor `gss`.
- **Stacks on**: PR-60.
- **Reviewers**: Architect, Security, QA, Tech Writer, Captain.

---

## Stable output strings

Slash commands grep against gss stdout. These substrings are
**load-bearing** — any rename in a port silently breaks a slash
command. Pinned in PR-15/PR-16 golden snapshots; reviewed at every
release.

| Where it's emitted | Substring (literal) | Slash command relying on it |
|--------------------|---------------------|-----------------------------|
| `gss status` clean | `No changes detected` | `/sync` (research phase) |
| `gss scan` per dirty repo | `[DIRTY]` (prefix on the line) | `/gss-scan` |
| `gss push` backup line | `backup/gss-` (prefix) | `/sync` (verification phase) |
| `gss push` PR-create | `https://github.com/.+/pull/\d+` | `/sync`, `/gss-pr` |
| `gss version --json` keys | `"version"`, `"commit"`, `"dirty"` | (future tooling) |
| `gss feature worker add --json` keys | `"worker_ref"`, `"worktree_path"`, `"branch"`, `"base_branch"` | tmux-mgr `runAgentStart` |
| `gss feature checkpoint --auto` skip diagnostic | one-line, prefix `gss: auto-checkpoint skipped:` | tmux-mgr pane-close-hook |
| `gss push` refusal (no approval token) | `missing or unreadable approval token` (+ exit 22) | `scripts/test.sh` GSS guardrail; `/sync` safety |

---

## Definition of Done (v1.0)

- [ ] Every PR-NN above merged into `test_gss` in `~/GitHub/playground`.
- [ ] [Resolutions coverage table](#resolutions-coverage-table) fully
      populated — every row cites a merged deliverable.
- [ ] [Pre-v1 hardening checklist](./design.md#pre-v1-hardening-checklist)
      every item ticked.
- [ ] Slash commands `/sync`, `/gss-pr`, `/gss-scan`, `/gss` verified
      working against the new binary via their existing prompts
      (manual smoke per PR-61).
- [ ] PR-61 (final integration PR) opened against `dotfiles:main`
      containing the release / migration note.
- [ ] No regressions reported during human review of any merged PR.
