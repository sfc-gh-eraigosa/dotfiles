# WORKER.md placement — implementation plan

- **Slug:** worker-md-placement
- **Date:** 2026-06-08
- **Status:** Draft
- **Relates to:** spec `../specs/worker-md-placement.md` · issue #132 · PR #133

## 1. Summary & verdict

Move the gss worker `WORKER.md` from the worktree root to a leaf-keyed
`<feature>/<user>/.gss-meta/<leaf>/WORKER.md` outside the worktree, routed through one pure
helper, with `done.go` cleanup, legacy migration on both writer paths, and skill/docs updates.

**Architecture-team verdict (design):** Option B, adversary-confirmed (4 confirmed refutations).
Must-fixes folded in below: (a) single helper feeds WRITE **and** APPEND; (b) migration runs on
**both** writer paths because `appendAutoLog` can create the file first; (c) explicit `done.go`
cleanup because nothing today touches the `<feature>/<user>/` level; (d) Option C (`info/exclude`)
**not** adopted — empirically disproven. This is a **single-PR, sequential** build — no CAP-B
breakout (see §6.1).

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/gss/internal/feature/paths.go` *(new)* | `workerMetaPath` + `migrateLegacyWorkerMD` helpers | F1, F5 |
| `sdk/gss/internal/feature/paths_test.go` *(new)* | helper table test + migration unit test | F1, F5 |
| `sdk/gss/internal/feature/worker.go` | WRITE → meta path; `MkdirAll`; call migration | F2, F5 |
| `sdk/gss/internal/feature/worker_test.go` | **invert**: assert WORKER.md outside worktree, at meta path; #132 regression; two-sibling guard | F2, UC-2 |
| `sdk/gss/internal/feature/auto.go` | APPEND → meta path; `MkdirAll`; call migration | F3, F5 |
| `sdk/gss/internal/feature/auto_test.go` | **repoint** reads (`:98`,`:154`) to meta path; first-writer case | F3 |
| `sdk/gss/internal/feature/done.go` | `os.RemoveAll` leaf meta dir after `Backend.Remove`; best-effort empty-parent cleanup on feature delete | F4 |
| `sdk/gss/internal/feature/done_test.go` *(new or extend)* | post-`done` meta dir gone; sibling survives | F4 |
| `sdk/gss/internal/feature/checkpoint.go` | fix stale `:61` comment (no behavior change) | F6 |
| `sdk/gss/skill/SKILL.md` | remove `WORKER.md` clause from "Cleanup (Mandatory)"; keep `FEATURE.md`; add new-path note | F6 |
| `sdk/gss/docs/` (the worktree/feature doc) | document the `.gss-meta/<leaf>/WORKER.md` on-disk contract | F6 |
| **Rebuild:** `sdk/gss/build.sh` (run, not edit) | reinstall binary so tmux-mgr's runtime shell-out isn't stale | harness |

No edits to `install.sh`, `scripts/test.sh`, or the registry schema — discovery is by-directory
and the change is package-internal.

## 3. Interface contracts

```go
// paths.go — the single source of truth for the on-disk WORKER.md location.
// Derives solely from the leaf worktree path: leaf == filepath.Base(worktree),
// <feature>/<user>/ == filepath.Dir(worktree). No registry field needed.
func workerMetaPath(worktree string) string {
    return filepath.Join(filepath.Dir(worktree), ".gss-meta", filepath.Base(worktree), "WORKER.md")
}

// migrateLegacyWorkerMD moves a pre-existing root-level WORKER.md to the meta
// path (preserving content) and un-tracks it if it was committed. Idempotent:
// a no-op when no legacy file exists. Invoked from BOTH writer paths.
func (s *Service) migrateLegacyWorkerMD(ctx context.Context, worktree string)
```

Write/append sites both become: `p := workerMetaPath(wt); os.MkdirAll(filepath.Dir(p), 0o755); …`.

## 4. TDD build order

**Phase 1 — helper (F1).** *Write first:* `paths_test.go` table test mapping representative
worktree paths (incl. a suffix'd leaf `design-foo`) → exact expected meta path. *Verify:*
`go test ./internal/feature -run TestWorkerMetaPath`. **Done-when:** helper exists, table green.

**Phase 2 — WRITE moves out (F2, UC-2).** *Write first:* invert `worker_test.go:46` to assert
`Stat(workerMetaPath(res.Worktree))==nil` **and** `Stat(Join(res.Worktree,"WORKER.md"))!=nil`;
add the two-sibling guard (two workers under one feature/user → distinct meta paths + independent
content). *Then:* change `worker.go:128` to write the meta path with a preceding `os.MkdirAll`.
*Verify:* `go test ./internal/feature -run 'TestWorker'`. **Done-when:** new+inverted tests green;
no test still expects a root file.

**Phase 3 — APPEND moves out (F3).** *Write first:* repoint `auto_test.go:98`/`:154` reads to the
meta path; add a first-writer case (`appendAutoLog` with no prior seed lands at the meta path).
*Then:* update `appendAutoLog` to use `workerMetaPath` + defensive `MkdirAll`. *Verify:*
`go test ./internal/feature -run 'TestAuto'`. **Done-when:** auto-log tests green incl. first-writer.

**Phase 4 — cleanup on done (F4).** *Write first:* `done_test.go` — after `feature done`, the
leaf `.gss-meta/<leaf>/` is gone and an unrelated sibling's meta dir survives; full-feature delete
leaves no `.gss-meta` orphan. *Then:* add `os.RemoveAll` (+ best-effort empty-parent `os.Remove`)
in `done.go` after `Backend.Remove` (and in the `deleteFeature` branch). *Verify:*
`go test ./internal/feature -run 'TestDone'`. **Done-when:** cleanup tests green.

**Phase 5 — legacy migration (F5).** *Write first:* `paths_test.go` migration cases — (a)
untracked root `WORKER.md` → moved to meta path, content byte-identical; (b) tracked root file →
moved **and** `git status --porcelain` clean of `WORKER.md`; (c) no legacy file → no-op. *Then:*
implement `migrateLegacyWorkerMD` and call it at the top of the WRITE path and before the append
in `appendAutoLog`. *Verify:* `go test ./internal/feature -run 'TestMigrat'`. **Done-when:** all
three migration cases green.

**Phase 6 — docs/skill + comment (F6).** Remove the `WORKER.md` delete clause from
`SKILL.md` "Cleanup (Mandatory)" (keep `FEATURE.md`); add the new-path note; fix `checkpoint.go:61`
comment; document the path in `sdk/gss/docs/`. *Verify:* `grep -n "WORKER.md" sdk/gss/skill/SKILL.md`
shows only the new descriptive line, not a delete mandate. **Done-when:** grep + review pass.

**Phase 7 — gate + rebuild.** Run `go test ./...` in `sdk/gss` with coverage ≥60% for
`internal/feature`; `bash sdk/gss/build.sh`; run `scripts/e2e-gss-integration.sh`; do the
human-evidenced scratch-repo check (`worker add` → `git status` clean, file at meta path).
**Done-when:** all gates green + evidence captured.

## 5. Verification mapping

| Spec rule | Test case |
| :-- | :-- |
| F1 helper exact path (incl. suffix) | `TestWorkerMetaPath` (table) |
| F2 file outside worktree / at meta path | `worker_test.go` inverted Stat asserts + `TestWorkerAdd_NoRootWorkerMD` |
| UC-2 two-sibling distinct files | `TestWorkerAdd_SiblingLeavesDistinct` |
| F3 auto-log at meta path | `auto_test.go` repointed reads |
| F3 first-writer tolerance | `TestAppendAutoLog_FirstWriter` |
| F4 leaf cleanup + sibling survives | `TestDone_RemovesMetaDir` |
| F5 untracked move / tracked move-clean / no-op | `TestMigrateLegacyWorkerMD_*` |
| F6 skill no longer mandates delete | `grep` assertion / review |
| Coverage ≥60% | `scripts/test.sh` / CI gate |
| Integration intact | `scripts/e2e-gss-integration.sh` |

## 6. Integration & rollout

- **Discovery:** by-directory; no `scripts/test.sh` / `Makefile` edit needed.
- **Rebuild:** `bash sdk/gss/build.sh` → `~/opt/bin/gss` (tmux-mgr runtime dependency).
- **Skill sync:** `SKILL.md` lives under `sdk/gss/skill/`; `sync-skills` relinks on install.
- **Manual acceptance:** in a throwaway git repo, `gss feature start` + `worker add`, confirm
  `git -C <worktree> status --porcelain` has no `WORKER.md` and the file is at
  `<feature>/<user>/.gss-meta/<leaf>/WORKER.md`; run an `auto-checkpoint` skip and confirm the
  diagnostic appends there.

### 6.1 Build leaves / DAG

**Not broken out.** The whole change is one cohesive, sequential edit to a single package
(`internal/feature`) whose phases share files (`worker.go`+`worker_test.go`,
`auto.go`+`auto_test.go`) — splitting would be a false split (overlapping paths, perpetual
rebase). Build it as **one `gss` worker / one PR**, TDD phases 1→7 in order. The path helper
(Phase 1) is the internal "frozen interface" the later phases import, but it ships in the same PR.

> Produced from the spec. Execute with TDD throughout (`superpowers:test-driven-development`).
> Update `../index.md` state as it moves.
