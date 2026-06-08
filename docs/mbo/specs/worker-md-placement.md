# WORKER.md placement — spec

- **Slug:** worker-md-placement
- **Date:** 2026-06-08
- **Status:** Draft
- **Relates to:** issue #132 · PR #133 · design `../designs/worker-md-placement.md`

## 1. Goal

`gss feature worker add` must seed (and `auto-checkpoint` must append to) the worker's
`WORKER.md` scaffolding file at a location **outside** the worker's git worktree, so it can
never appear in the consumer repo's `git status` or be swept into a commit — without touching
any consumer-repo ignore rules. The file moves to a **leaf-keyed** path
`<feature>/<user>/.gss-meta/<leaf>/WORKER.md`, is created/migrated transparently, and is torn
down with the worker on `feature done`. The result: the manual "delete WORKER.md before
`pr --ready`" chore in the gss skill is retired.

## 2. Use cases

**UC-1 — Add a worker (fresh).** *Actor:* a user/agent running `gss feature worker add`.
*Trigger:* `WorkerAdd` materializes the worktree. *Flow:* the seed render is written to
`workerMetaPath(worktree)`; the meta dir is created first. *Acceptance:* `WORKER.md` exists at
the meta path; the worktree root contains **no** `WORKER.md`; `git status --porcelain` in the
worktree shows no `WORKER.md` line.

**UC-2 — Two workers, one (feature,user).** *Actor:* a user adding `design` then `impl` (or two
same-purpose workers, forcing a suffix). *Trigger:* second `worker add`. *Flow:* each leaf gets
its own `.gss-meta/<leaf>/WORKER.md`. *Acceptance:* the two `WORKER.md` files are **distinct
paths** with independent content; neither clobbers the other. (This is the regression guard that
proves Option B over the rejected Option A.)

**UC-3 — Auto-checkpoint skip writes a diagnostic.** *Actor:* the `auto-checkpoint` path.
*Trigger:* `autoSkip` → `appendAutoLog(worktree, reason)`. *Flow:* the `## Auto-checkpoint log`
line is appended to `workerMetaPath(worktree)`, creating the meta dir + file if absent.
*Acceptance:* the diagnostic lands at the meta path even when `appendAutoLog` is the **first**
writer (no prior seed); the worktree root stays free of `WORKER.md`.

**UC-4 — Tear down a worker.** *Actor:* `gss feature done <worker>`. *Trigger:* after
`Backend.Remove(worktree)`. *Flow:* `.gss-meta/<leaf>/` is removed; empty `.gss-meta` / `<user>`
parents are best-effort removed on full-feature delete. *Acceptance:* after `done`, no
`.gss-meta/<leaf>/` orphan remains under `~/.config/gss/worktrees`; the existing
`os.Remove(featDir)` is not wedged by a leftover.

**UC-5 — Legacy worktree migration.** *Actor:* a worktree created under the old layout (root
`WORKER.md`). *Trigger:* the next `worker add` WRITE path **or** the next `appendAutoLog`.
*Flow:* `migrateLegacyWorkerMD(worktree)` renames the root file to the meta path (preserving
human edits + appended log) and, if it was git-tracked, runs `git rm --cached --quiet WORKER.md`.
*Acceptance:* the file is at the meta path with content intact; `git status --porcelain` in the
worktree is clean of `WORKER.md` afterward.

## 3. Architecture

Single package: `sdk/gss/internal/feature`. No registry-schema change, no CLI surface change.

- **`workerMetaPath(worktree string) string`** — the one pure helper; the *interface contract*
  every touchpoint routes through. Derives solely from the leaf worktree path:
  `filepath.Join(filepath.Dir(worktree), ".gss-meta", filepath.Base(worktree), "WORKER.md")`.
- **`migrateLegacyWorkerMD(worktree string)`** — idempotent legacy mover; invoked from both
  writer paths.
- **WRITE** (`worker.go:128`) and **APPEND** (`auto.go:142`) call the helper (+ `os.MkdirAll`).
- **CLEANUP** (`done.go`) `os.RemoveAll`s the leaf meta dir.
- **Data flow:** `worktree path` (already in `WorkerResult`/`registry.Worker`) → helper → meta
  path. Nothing else consumes `WORKER.md` (`checkpoint.go:61` is a stale comment, no read).

## 4. Behavior / features

- **F1** `workerMetaPath` helper — single source of truth for the path.
- **F2** WRITE seeds at the meta path (dir created first).
- **F3** APPEND writes the auto-log at the meta path (dir created first; tolerant of being the
  first writer).
- **F4** `done.go` removes the leaf meta dir (+ best-effort empty-parent cleanup).
- **F5** `migrateLegacyWorkerMD` moves a pre-existing root `WORKER.md` on both writer paths,
  preserving content and un-tracking it if it was tracked.
- **F6** Docs/skill: remove the `WORKER.md` clause from `SKILL.md` "Cleanup (Mandatory)" (keep
  `FEATURE.md`); fix the stale `checkpoint.go:61` comment; document the new on-disk path in
  `sdk/gss/docs/`.

## 5. Evaluation criteria (per feature)

| Feature | Fires (must) | Must-not-fire | Edge | Pass predicate (→ a test) |
| :-- | :-- | :-- | :-- | :-- |
| **F1** helper | returns `…/<feature>/<user>/.gss-meta/<leaf>/WORKER.md` for a leaf worktree | — | suffix'd leaf (`design-foo`) keys on the full leaf base | table test: input worktree → exact expected path, incl. suffix case |
| **F2** WRITE | `WORKER.md` present at meta path after `WorkerAdd` | `WORKER.md` present in worktree **root** | meta dir absent → created | `worker_test.go`: Stat(meta)==nil **and** Stat(root WORKER.md)!=nil |
| **F3** APPEND | diagnostic at meta path after `autoSkip` | nothing written into worktree root | `appendAutoLog` is first writer (no seed) | `auto_test.go`: read meta path contains the conflict/diagnostic string |
| **F4** cleanup | `.gss-meta/<leaf>/` gone after `done` | unrelated sibling leaf's meta dir removed | empty `.gss-meta` parent removed on full delete | `done_test.go`: post-`done`, meta dir does not exist; sibling survives |
| **F5** migration | legacy root `WORKER.md` ends at meta path, content preserved | a fresh worktree (no legacy file) is disturbed | tracked legacy file → `git rm --cached`, porcelain clean | migration test: seed root file → trigger writer → assert moved + clean porcelain |
| **F6** docs | `SKILL.md` no longer mentions deleting `WORKER.md` | `FEATURE.md` cleanup wording removed | — | grep assertion in repo test / manual review: `SKILL.md` ∌ "WORKER.md" delete clause |

**Two-sibling collision guard (UC-2 / proves B>A):** add a dedicated test creating two workers
under one `(feature,user)` and asserting `workerMetaPath(wtA) != workerMetaPath(wtB)` and the
two files hold independent content.

## 6. Verification harness

- **Unit (authoritative):** `go test ./internal/feature/...` in `sdk/gss`. The existing
  `worker_test.go` / `auto_test.go` assertions are **inverted** (file outside worktree, at meta
  path); new tests: helper table test, two-sibling guard, `done` cleanup, legacy migration
  (incl. tracked-file porcelain-clean). Tests use the existing temp-dir backend so the meta path
  is a real on-disk dir.
- **Coverage gate:** the repo's `sdk/` Go **≥60%** coverage gate (`scripts/test.sh` / `Makefile`
  / CI) must still pass for `internal/feature`.
- **Integration guard:** `scripts/e2e-gss-integration.sh` (the tmux-mgr↔gss guard) must pass
  against the rebuilt binary — **rebuild `gss` via `sdk/gss/build.sh`** after the change (tmux-mgr
  shells out to the installed binary; a stale binary silently breaks it).
- **Human-evidenced:** one real `gss feature worker add` in a scratch repo → confirm
  `git status` is clean of `WORKER.md` and the file exists at the meta path
  (`superpowers:verification-before-completion`).

## 7. Prerequisites / dependencies

None new. Touches only `sdk/gss` (`internal/feature`, `skill/SKILL.md`, `docs/`). No new Go
deps, no registry-schema migration, no CLI flags.

## 8. Out of scope (and why)

- **Option D** (no file; render `WORKER.md` from the registry on demand) — changes the
  agent-facing contract (`auto.go` writes to it; agents `cat`/edit it); larger refactor; tracked
  separately if pursued.
- **A parallel `FeatureMetaPath` helper for `FEATURE.md`** — `FEATURE.md` already lives outside
  the worktree and is cleaned by `done.go`; refactoring it is optional polish, kept out to hold
  the diff tight.
- **Permissions / encryption changes** — `0o644` is correct for non-secret scaffolding.

## 9. Rollback

Revert the `internal/feature` commit (pure path move, no schema migration). New worktrees revert
to root-seeding; the `SKILL.md` mandate is restored in the same revert. Stranded `.gss-meta`
dirs under `~/.config/gss/worktrees` are gss-private and removable with
`find ~/.config/gss/worktrees -type d -name .gss-meta -prune -exec rm -rf {} +`. No
consumer-repo footprint was ever created, so nothing to unwind in any consumer checkout.

> Produced from the architecture-team design (`../designs/worker-md-placement.md`). The matching
> plan goes in `../plans/worker-md-placement.md`. Register / update `../index.md`.
