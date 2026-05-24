# gss v1.0 — Cleanup Plan

Things to tidy up **once PR-61 (final integration) has merged** to
`dotfiles:main`. None of these block the release; they prevent the
v1.0-development scaffolding from rotting into confusing cruft.

Ordered by when they should happen.

---

## 1. Tear down the staging scaffold (do first, right after PR-61 merges)

The development scaffolding exists only to carry v1.0 across machines. It
must not outlive the release.

- [ ] **Close PR #13** (`wip/gss-v1-staging`, the DO-NOT-MERGE handoff
      cursor) — its job ends the moment PR-61 lands. Close, don't merge.
- [ ] **Delete the `wip/gss-v1-staging` branch** (local + remote) once
      PR #13 is closed.
- [ ] **Archive the playground stack** in `~/GitHub/playground`: the
      `test_gss` trunk and every `pr01-*`…`pr60-*` branch. Keep the repo
      for history if desired, but delete or tag the per-PR branches so a
      future reader doesn't mistake them for live work.

## 2. Retire the handoff cursor doc

- [ ] **`STATE.md`** is a live development cursor, not durable
      documentation. After PR-61, either delete it or replace its body
      with a one-line tombstone pointing at `RELEASE.md` + `design.md`.
      The durable docs are `design.md`, `plan.md`, `roadmap.md`, and
      `RELEASE.md`.
- [ ] Before deleting, **migrate any still-open carry-forward items**
      (see §3) out of `STATE.md` into `roadmap.md` or a tracked issue so
      they aren't lost with the cursor.

## 3. Resolve the carry-forward backlog

The 16 carry-forward notes in `STATE.md` fall into three buckets:

- **Already done** (#4, #5, #7, #9) — verify the resolutions landed, then
  drop the rows.
- **Fold into a hardening pass** (#1, #2, #3, #6, #8, #10) — the
  unenforced validation constants, the `git/fake` retro-fits, the
  unbounded-buffer guard, the `os/exec` CI grep, the optional gh
  integration test. Batch these into a single post-v1 "internal
  hardening" PR rather than leaving them as scattered TODOs.
- **Small feature gaps** (#11–#15) — `worker update`, gh-login user
  precedence, `start --purpose`, `checkpoint --message`, `conflicts`
  default-all. Promote each to a `roadmap.md` entry or a tracked issue;
  these are real follow-ups, not cleanup.
- **Robustness** (#16) — add a `gss --version` / capability gate so
  `tmux-mgr` fails with "rebuild gss: src/gss/build.sh" instead of
  `unknown flag: --json`. Keep the `e2e-gss-integration.sh` guard.

## 4. Code-level cleanup

- [ ] **Retire `isDirty` from `cmd/scan.go`.** It was kept only so the
      existing `cmd/status_test.go::TestIsDirty` keeps compiling after the
      `internal/scan` rewire (PR-16). Move the test onto `internal/scan`'s
      exported `GitDirty` and delete the local shim.
- [ ] **Confirm the worktree-backend placeholders stay doc-only.** The
      alternative backends (overlayfs/sandboxfs/dockerfs/tmpfs) are
      documented in `roadmap.md` and must remain `// TODO` — no
      half-implemented sub-packages should land. Grep for stray
      `internal/worktree/{overlayfs,sandboxfs,...}` dirs and remove any.
- [ ] **`no-workspace-guard.sh` stays.** The grep guard wired into
      `build.sh` (PR-59) enforces that `gss` is the sole worktree owner;
      keep it as a regression fence, don't retire it with the scaffold.

## 5. Documentation reconciliation

- [ ] Ensure `src/gss/docs/` index/links are coherent after `STATE.md`
      retires: `design.md` ↔ `roadmap.md` ↔ `RELEASE.md` cross-links
      resolve.
- [ ] Confirm `tmux-mgr` docs (`docs/user_guide.md`, `README.md`,
      `skill/SKILL.md`) carry no `gss feature` / `git worktree` leakage
      (PR-60 covered `SKILL.md`; spot-check the others).
- [ ] Update top-level `CLAUDE.md` / `opt/.../GEMINI.md` registries if the
      `gss feature` surface should be discoverable there.

---

## Done criteria

Cleanup is complete when: PR #13 is closed and its branch deleted; the
playground per-PR branches are archived; `STATE.md` is gone or
tombstoned; every carry-forward item is either resolved, folded into a
hardening PR, or promoted to `roadmap.md`/an issue; and `git grep` finds
no `wip/gss-v1-staging`, no stray backend sub-packages, and no
`git worktree` leakage in `tmux-mgr` docs.
