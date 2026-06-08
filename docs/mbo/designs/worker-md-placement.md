# WORKER.md placement — design

- **Slug:** worker-md-placement
- **Date:** 2026-06-08
- **Status:** Proposed
- **Relates to:** issue #132
- **Author(s):** architecture team (sysarch · principal · secarch · adversary) via Workflow

## 1. Problem / context

`gss feature worker add` seeds a `WORKER.md` scaffolding file into the **root of the worker's git worktree** (`sdk/gss/internal/feature/worker.go:128`, an unconditional `os.WriteFile(filepath.Join(res.Worktree, "WORKER.md"), …, 0o644)`). Because it lives inside the worktree, it shows up as `?? WORKER.md` in `git status` of the consumer repo and risks being swept into a commit (`git add -A`, or a misfired `git add .` from the wrong cwd). The auto-checkpoint path also appends to it (`auto.go:142-150`, `O_APPEND|O_CREATE`), so even a clean worktree can re-grow the file after the user deletes it.

The current mitigation is a **manual chore**: the gss skill (`sdk/gss/skill/SKILL.md:136-139`) mandates deleting and committing the removal of `WORKER.md` before `gss feature pr --ready`. Issue #132 wants a **structural fix** so accidental commits become impossible, and explicitly **rejects** adding `WORKER.md` to the consumer repo's `.gitignore` (that pollutes every consumer repo with gss-internal scaffolding).

Verified layout facts (read from source, not assumed):

- `worker.go:100`: `wtPath = filepath.Join(s.WorktreeRoot, s.NWO, opts.Feature, user, leaf)`, where `leaf = purpose[-suffix]` (`worker.go:94-98`). Default `WorktreeRoot = ~/.config/gss/worktrees`, `NWO = "<owner>/<repo>"`.
- `identity.AllocateRef` (`worker.go:90`) **deliberately allocates a distinct suffix** when a bare `(feature,user,purpose)` is taken, so multiple leaves (e.g. `design/`, `impl/`) **coexist under the same `<feature>/<user>/` parent by design**. This is the crux of the collision analysis below.
- Precedent: `FEATURE.md` already lives **outside** any leaf worktree at `<feature>/FEATURE.md` (`start.go:98`, read back at `done.go:128`). gss already keeps one scaffolding markdown a directory level above the leaves.
- `checkpoint.go:61`: `title := fmt.Sprintf("%s: %s", ref.Feature, ref.Purpose) // first H1 of WORKER.md` — the trailing comment is **stale**; the title is built from `ref`, and **no read of WORKER.md occurs anywhere**.

The complete set of `WORKER.md` touchpoints (grep-verified): (1) WRITE `worker.go:128`; (2) APPEND `auto.go:142-150`; (3) the stale comment at `checkpoint.go:61` (no behavioral read).

## 2. Goals & non-goals

**Goals**
- `WORKER.md` must never appear in the consumer worktree's `git status` — make accidental commits **structurally** impossible, not merely hidden.
- Do **not** touch the consumer repo's tracked ignore rules (`.gitignore`) — and, as the adversary proved, do **not** touch the consumer repo's `.git/info/exclude` either (see §3, Option C).
- Each worker owns its own `WORKER.md` inode: no cross-worker clobber, no interleaved auto-logs.
- Lifecycle symmetry: every file the fix creates is torn down by `feature done`; no orphan accretion under `~/.config/gss/worktrees`.
- Route all touchpoints through **one** path helper so WRITE and APPEND can never diverge.
- Migrate worktrees created under the old in-worktree layout without dropping human/agent edits or the appended auto-log.
- Retire the manual SKILL.md cleanup mandate for `WORKER.md`.

**Non-goals**
- Eliminating the `WORKER.md` file entirely / serving it from the registry (Option D) — larger refactor, changes the agent-facing contract; deferred as separate tech-debt.
- Changing `FEATURE.md` handling (already outside the worktree, cleaned by `done.go`).
- Changing file permissions (`0o644` is correct; `WORKER.md` is non-secret scaffolding).
- Extracting a parallel `FeatureMetaPath` helper for `FEATURE.md` — noted as optional follow-up to keep the diff tight.

## 3. Options considered

**Recommendation: Option B** — `<feature>/<user>/.gss-meta/<leaf>/WORKER.md` (leaf-keyed, outside the worktree). All three lenses and the adversary converge here.

| Option | Shape | Verdict | Why |
|---|---|---|---|
| **B (chosen)** | `<feature>/<user>/.gss-meta/<leaf>/WORKER.md` | **Adopt** | A's correct instinct (move out of the worktree; don't touch consumer ignore rules) made **collision-safe** by keying on `leaf`, and **lifecycle-symmetric** by namespacing under a single dir that `done.go` can `RemoveAll`. |
| A (the issue's proposal) | `<feature>/<user>/WORKER.md` (parent of leaf) | **Reject** | Puts a **per-worker (1:N)** file at a **shared parent** dir. Real, source-confirmed data-loss when siblings coexist. False analogy to `FEATURE.md`. Orphaned forever by `done.go`. |
| C | Keep file in worktree; add it to `info/exclude` | **Reject** | **Empirically disproven** by the adversary: a linked worktree does **not** consult a per-worktree `info/exclude`; `git rev-parse --git-path info/exclude` resolves to the **shared common** `.git/info/exclude`. So C either fails to hide the file *or* writes into the file shared with the consumer's own checkout — i.e. the exact consumer-repo pollution #132 rejected. Cannot deliver "per-clone + invisible + no consumer pollution" simultaneously. |
| D | No file; render from registry on demand | **Defer** | Best blast-radius profile, but changes the agent contract (`WORKER.md` is a real artifact agents `cat`/edit and `auto.go` writes to) and relocates auto-log diagnostics into JSON. Out of scope for a path-contract fix; route to a separate objective. |

**Shared-parent collision analysis (why A does not survive).** A worker_ref is `<feature>/<user>/<purpose>[-suffix]`. `AllocateRef` exists precisely to let `design/` and `impl/` (or two same-purpose workers) live under one `<feature>/<user>/`. Option A maps **both** to the single `<feature>/<user>/WORKER.md`:
- **WRITE collision** — `worker.go:128` is an unconditional `os.WriteFile`; the second `worker add` **clobbers** the first worker's seeded file.
- **APPEND collision** — `appendAutoLog` (`auto.go:142`, `O_APPEND|O_CREATE`) interleaves two distinct workers' auto-checkpoint diagnostics into one file, making the log ambiguous.
- **Race** — the worktree materialization + `WriteFile` happen **after** the `Store.Update` lock is released (`worker.go:118-130`; lock only covers the registry row). Two concurrent `worker add` calls compound A's sequential clobber into a genuine write race. B is immune: distinct leaves, distinct paths.

**Why the `FEATURE.md` precedent does not rescue A.** `FEATURE.md` is safe at `<feature>/` because that dir is **1:1** (exactly one feature, matching cleanup at `done.go:111`). A borrows the precedent's *path shape* (one level above the leaf) while breaking its *invariant* (single owner per dir). The faithful analogue keys by the leaf → Option B.

**Cleanup gap (applies to A and B alike, must be fixed).** `done.go:69` `Backend.Remove(w.Worktree)` removes only the **leaf inode**. `FEATURE.md`/`featDir` are removed only when the feature fully empties (`done.go:110-112`, non-recursive best-effort `os.Remove`). **Nothing ever touches the `<feature>/<user>/` level.** So A's parent file accretes forever; B's `.gss-meta/<leaf>/` likewise requires an **explicit** `os.RemoveAll` in the worker-removal path. (A leftover under `featDir` would also make the existing `os.Remove(featDir)` silently fail on a non-empty dir.) Cleanup is therefore in-scope, not optional.

**Latent benefit of B (surfaced by the adversary).** `done.go:55-57` refuses teardown when `git status --porcelain` is non-empty. Today an untracked `WORKER.md` makes the worktree perpetually "dirty," which already pushes users toward `--force` or the manual-delete chore. Moving the file out makes the dirty-check **honest** — only real work shows as dirty. This is a second motivation for the fix.

## 4. Decision

Adopt **Option B**. Do **not** layer Option C on top (its premise is disproven). Defer Option D.

**Path contract.** Per worker, `WORKER.md` lives at:

```
~/.config/gss/worktrees/<owner>/<repo>/<feature>/<user>/.gss-meta/<leaf>/WORKER.md
```

i.e. a leaf-keyed sibling of the leaf worktree, namespaced under `.gss-meta/`. This is a new public-ish on-disk contract; document it in `sdk/gss/docs/` and `SKILL.md`. Future gss-internal scaffolding nests under the same `.gss-meta/` namespace rather than proliferating sibling dot-files.

**Single path helper (one source of truth).** Add one **pure** helper in `internal/feature` (a small `paths.go`, or in `worker.go`):

```go
func workerMetaPath(worktree string) string {
    return filepath.Join(filepath.Dir(worktree), ".gss-meta", filepath.Base(worktree), "WORKER.md")
}
```

It derives **solely** from the leaf worktree path: `leaf == filepath.Base(worktree)` and `<feature>/<user>/` is `filepath.Dir(worktree)`. This is the decisive resolution of the one inter-lens disagreement: the Principal lens leaned toward persisting a `MetaDir` field on the registry row to feed `auto.go`; the adversary refuted the necessity — because `.gss-meta` is a **sibling of the leaf** (not above the `user` level), the pure helper resolves correctly from `w.Worktree` alone, with **no registry-schema change**. We take the **derive** path. (Persist a field *only if* a future layout change makes `leaf != filepath.Base(worktree)`; until then a schema change is unjustified coupling.)

**Concrete change at each touchpoint:**

1. **WRITE — `worker.go:128`.** Replace `filepath.Join(res.Worktree, "WORKER.md")` with `metaPath := workerMetaPath(res.Worktree)`; call `os.MkdirAll(filepath.Dir(metaPath), 0o755)` before the `os.WriteFile(metaPath, []byte(content), 0o644)` (mirrors the `MkdirAll` pattern start.go uses for `FEATURE.md`). The file is now physically **outside** `res.Worktree`, so it cannot appear in the consumer's `git status` — the point of #132.

2. **APPEND — `auto.go:142-150`.** In `appendAutoLog`, replace `filepath.Join(worktree, "WORKER.md")` with `workerMetaPath(worktree)`. Keep `O_APPEND|O_CREATE|O_WRONLY, 0o644`. Add a defensive `os.MkdirAll(filepath.Dir(path), 0o755)` before the open, because `appendAutoLog` can be the **first** writer (`O_CREATE`) if an auto-checkpoint skip fires before any successful manual add — without the guard the open fails when `.gss-meta/<leaf>/` doesn't yet exist. `appendAutoLog` keeps its `(worktree, reason)` signature; the helper needs nothing more, so **no signature change** is required.

3. **`checkpoint.go:61` — comment only.** No code change. Update the stale `// first H1 of WORKER.md` comment so it no longer implies a read (e.g. `// title from worker_ref (feature:purpose), not read from WORKER.md`).

**Cleanup / lifecycle — `done.go`.** After `Backend.Remove(w.Worktree)` succeeds (`done.go:69`), add:
```go
_ = os.RemoveAll(filepath.Join(filepath.Dir(w.Worktree), ".gss-meta", filepath.Base(w.Worktree)))
```
to tear down the leaf's meta dir, plus a best-effort `os.Remove` of the now-possibly-empty `.gss-meta` parent. In the `deleteFeature` branch (`done.go:110-112`), also best-effort-remove the now-empty `<user>/.gss-meta` and `<user>` dirs (same pattern as the existing `os.Remove(featDir)`), so a fully-done feature leaves no orphan. This restores the write↔cleanup symmetry A lacks.

**Backward-compat / migration of existing worktrees.** Worktrees created under the old layout have `WORKER.md` at the worktree root (untracked, possibly committed, possibly dirty). Migration must run on **both** writer paths (the adversary's edge: `auto.go` can be the first writer):
- Factor a small `migrateLegacyWorkerMD(worktree string)` invoked at the top of the WRITE path (`worker.go`) **and** before the append in `appendAutoLog` (`auto.go`). It: detects `filepath.Join(worktree, "WORKER.md")`; if present, `os.MkdirAll` the meta dir and `os.Rename` the legacy file to `workerMetaPath(worktree)` (**preserving** human edits + the appended auto-log); if the legacy file was git-tracked, run `git -C <worktree> rm --cached --quiet WORKER.md` so it leaves `git status`.
- A naive "just write the new path" is rejected: it would leave the old root-level file in `git status` — the exact symptom #132 fixes. Migration is a first-class, tested step.

**SKILL.md cleanup — `sdk/gss/skill/SKILL.md:136-139`.** The "Cleanup (Mandatory)" bullet currently mandates deleting/committing `WORKER.md` before `gss feature pr --ready`. Once `WORKER.md` lives outside the worktree this step is **structurally unreachable** and leaving it is actively misleading (agents will hunt a file that can't be there → false "I forgot to delete it" churn). Remove the `WORKER.md` clause; **keep the `FEATURE.md` wording** (handled separately by `done.go`'s template-clean compare-and-remove). Add one line stating `WORKER.md` now lives at `<feature>/<user>/.gss-meta/<leaf>/WORKER.md` outside the worktree and is auto-removed on `feature done`. Document the new on-disk path in `sdk/gss/docs/` per repo convention.

**Test contract (must land in the same commit).** Invert the existing assertions and add the collision guard:
- `worker_test.go:46` (`Stat(Join(res.Worktree, "WORKER.md"))` succeeds) → assert WORKER.md is **NOT** under `res.Worktree` and **IS** at `workerMetaPath(res.Worktree)`; add a direct #132 regression assertion that the worktree root is free of `WORKER.md`.
- `auto_test.go:98` and `:154` → repoint reads from `Join(wt, "WORKER.md")` to `workerMetaPath(wt)`.
- **New** two-sibling test: two workers under the same `(feature,user)` (forcing a suffix) get **distinct** `WORKER.md` files — the regression guard that proves B over A.
- Migration test: a pre-seeded legacy root-level `WORKER.md` is moved to the meta path with content preserved, and `git status --porcelain` in the worktree is clean afterward.

## 5. Risks & blast radius

- **WRITE/APPEND divergence (medium).** If only one site moves, the other re-creates `WORKER.md` inside the worktree and reintroduces the pollution. Mitigated: both route through the single `workerMetaPath` helper — that helper *is* the deliverable.
- **Cleanup omission (medium).** Forgetting the `done.go` `RemoveAll` trades a git-status nuisance for disk-litter under `~/.config/gss/worktrees` and can wedge the existing `os.Remove(featDir)`. Mitigated: explicit cleanup is in-scope (§4) with the test asserting a done feature leaves no orphan.
- **Migration drops edits or leaves the legacy file (medium).** A naive write strands the old file in `git status`. Mitigated: `os.Rename` (not write-new) + conditional `git rm --cached`, invoked on **both** writer paths, with a test asserting content preservation + clean porcelain.
- **`auto.go` first-writer ordering (low/medium).** Auto-checkpoint skip before any manual add. Mitigated by the defensive `MkdirAll` in `appendAutoLog` and running migration on the auto path.
- **Path-traversal via user names (low / non-issue, recorded so it isn't re-litigated).** `identity.ValidateSegment` (`identity/validate.go:18`, `^[a-z][a-z0-9-]{0,30}[a-z0-9]$`) forbids `/`, `.`, `..`, control chars; `ValidatePurpose` runs (`worker.go:49`) before any `filepath.Join`; suffix is wordlist-drawn. No segment can escape the gss-private tree. Any future loosening of the segment grammar, or sourcing the leaf from raw input, would reopen this.
- **Permissions / at-rest (none).** `0o644` unchanged; non-secret scaffolding; destination is the same `~/.config`, same-uid, gss-private tree where `FEATURE.md` already lives. No trust boundary crossed.
- **New dot-dir per worker (low).** One extra dir + one small md per worker. Negligible inode/storage cost; documented as a public on-disk contract.

Blast radius is contained to `internal/feature` (`worker.go`, `auto.go`, `done.go`, `checkpoint.go` comment), its tests, and `skill/SKILL.md` + `sdk/gss/docs/`. No registry schema change, no CLI surface change, no consumer-repo file touched.

## 6. Rollback

Pure-additive path move with no schema migration, so rollback is low-cost:
- **Code:** revert the `internal/feature` commit. New worktrees revert to seeding `WORKER.md` in the worktree root (old behavior); the manual SKILL.md cleanup mandate is restored in the same revert.
- **On-disk state:** worktrees created while the fix was live have `WORKER.md` under `.gss-meta/<leaf>/`. After rollback, the reverted code looks for it at the worktree root and won't find it; the auto-log path simply recreates a root-level file on next append (no data corruption, only the relocated history is stranded). A one-line operator cleanup (`find ~/.config/gss/worktrees -type d -name .gss-meta -prune -exec rm -rf {} +`) removes the stranded meta dirs if desired — they are gss-private and safe to delete.
- **No consumer-repo footprint** was ever created (no `.gitignore`/`info/exclude` writes), so there is nothing to unwind in any consumer checkout — a key reason Option C was rejected.

> Produced via an architecture-team `Workflow` (sysarch · principal · secarch · adversary). Register in `../index.md`. Spec → `../specs/worker-md-placement.md`.
