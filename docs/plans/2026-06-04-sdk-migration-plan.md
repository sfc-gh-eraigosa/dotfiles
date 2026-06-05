# Migration Plan: Relocate all Go modules from `src/` to a new top-level `sdk/` tree

- **Date:** 2026-06-04
- **Status:** Proposed
- **Relates to:** #114 — **Supersedes:** #115 (the in-place `src/gss` rename design)
- **Author:** The Systems Architect
- **Reviewers (sign-off required before Phase 2):** Go lead, CI lead, Security/EM

---

## 1. Goal & Scope

Move **all four Go modules** out of `src/` into a new top-level `sdk/` tree so their module paths become the canonical, externally-installable `github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>`. Each tool is then installable via:

```
go install github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>@sdk/<tool>/vX.Y.Z
```

The migration is performed **one module at a time** (move dir + fix module path/imports + fix every reference + verify), never big-bang. Each module lands as its own atomic PR that leaves `main` green and releasable.

**In scope (move to `sdk/`):** `gss`, `gsl`, `wol`, `tmux-mgr` (the only four `go.mod` files; no `go.work`, no cross-module `replace` — verified independent).

**Out of scope (stay in `src/`):** `git-machete`, `ssh-host-finder`, `ssh-key-sync`, `teams-tune`, plus `src/CLAUDE.md` and `src/GEMINI.md`. After migration, `src/` keeps these non-Go tools and docs; an empty `src/` root is acceptable (no cleanup needed).

**Explicitly unrelated, leave alone:** PR #116 (privacy_guard shebang fix).

---

## 2. Inventory

| Module | Current dir | Current module path | Target module path | LDFLAGS version pkg | Import refs | Skill? | GEMINI.md today? | Coverage (floor 60%, gss 70%) |
|---|---|---|---|---|---|---|---|---|
| gss | `src/gss` | `github.com/wenlock/dotfiles/gss` | `github.com/sfc-gh-eraigosa/dotfiles/sdk/gss` | `internal/version.*` | ~262 | yes | **no (create)** | ~80% PASS |
| gsl | `src/gsl` | `github.com/wenlock/dotfiles/gsl` | `github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl` | `internal/version.*` | ~102 | yes | **no (create)** | 88.7% PASS |
| wol | `src/wol` | `github.com/wenlock/dotfiles/wol` | `github.com/sfc-gh-eraigosa/dotfiles/sdk/wol` | **`cmd.*`** (not internal/version) | 1 | no | **no (create)** | 55.6% **RED** (#51) |
| tmux-mgr | `src/tmux-mgr` | `github.com/eraigosa/dotfiles/src/tmux-mgr` | `github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr` | **`cmd.*`** | 13 | yes | yes (git mv it) | 48.8% **RED** (#50) |

**Two distinct old-path roots** (a blanket find/replace is WRONG):
- gss/gsl/wol: `github.com/wenlock/dotfiles/<tool>` (wrong org, no path segment).
- tmux-mgr: `github.com/eraigosa/dotfiles/src/tmux-mgr` (wrong org *and* already has a `src/` segment — the rewrite must collapse `eraigosa→sfc-gh-eraigosa` AND `src→sdk` in one anchored substitution).

Install target for every module is the **module root** (`main.go`); there is no `cmd/<tool>/` install package.

---

## 3. Target Module Paths & Tag Scheme

- Canonical path: `github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>`.
- **Path-prefixed tags are mandatory** for sub-directory modules in a monorepo: `sdk/<tool>/vX.Y.Z` (e.g. `sdk/gss/v0.1.0`). A plain `vX.Y.Z` tag will **not** resolve a sub-dir module — this is the single most common Go monorepo footgun and must be documented prominently.
- `@latest` resolves to **nothing** until the first `sdk/<tool>/vX.Y.Z` tag exists. During the migration window (Phase 2 → Phase 6) external consumers must pin `@<commit>` or `@<branch>` (see Cross-Cutting Decision 6).

---

## 4. Cross-Cutting Decisions

These are recorded here and ratified in the umbrella ADR (Phase 1). Several are **blocking and irreversible** — they must be answered before any tag is cut.

1. **Public vs private posture (BLOCKING, irreversible).** The target path `github.com/sfc-gh-eraigosa/...` does **not** match the verified local `GOPRIVATE=github.com/snowflake-eng/*`, so by default the first public fetch permanently records path+hash in `sum.golang.org` and caches on `proxy.golang.org`. **Owner: EM + Snowflake org security.** No tag may be cut until this is decided. If private, document `GOPRIVATE=github.com/sfc-gh-eraigosa/*` + credentialed access for consumers.

2. **Repo visibility.** Confirm `github.com/sfc-gh-eraigosa/dotfiles` is public (proxy/sumdb will serve it) or private (public `go install` cannot resolve; sumdb bypassed). **Owner: EM.**

3. **Abandoned-namespace retention.** `github.com/wenlock` and `github.com/eraigosa` are the soon-stale orgs. Confirm both stay registered under the user's control to block typosquat/namespace-takeover of the old import paths; if not controlled, escalate as a security risk. **Owner: Security + EM.**

4. **Tag scheme + signing.** Ratify `sdk/<tool>/vX.Y.Z`, **start each module at v0.x** to defer v1 immutability lock-in, require **signed annotated tags** + a GitHub tag-protection ruleset on `sdk/<tool>/v*`. No CI tag automation exists today (verified) — tagging is manual; confirm with CI. **Owner: CI + Security.**

5. **Coverage-gate posture.** `wol` (55.6%) and `tmux-mgr` (48.8%) are below the 60% floor (#51/#50). Structural file motion does **not** change coverage %. Confirm `scripts/test.sh` runs **warn-only** (`COVERAGE_ENFORCE`/`TESTS_HARD_FAIL=0`) so the move is judged on "no new failures / coverage % unchanged" and is neither blamed for nor credited with closing the backlog. **Owner: EM + Go QA.**

6. **Interim external-install contract (no-tag window).** Between Phase 2 and Phase 6 there is no tag for any `sdk/` path, so `@latest` resolves to nothing. **If any external consumer needs the new path before the first tag, they pin `@<commit-sha>` or `@<branch>`.** State this explicitly in the ADR and any consumer comms. (Resolves adversary must-fix #6.)

7. **PR #115 disposition.** Repurpose its design content into the umbrella ADR (recommended) rather than closing cold; its problem statement (broken external `go install`) is exactly what `sdk/` solves, but its solution (in-place `src/gss` rename) is superseded. **Owner: Architecture.**

8. **Per-module PRs vs stacked branch.** Four sequential atomic PRs (green `main` at every boundary, per-module review, e2e guard meaningful at the gss/tmux-mgr boundary). **Owner: EM.**

9. **Discovery-loop end state.** Keep `test.sh`/`Makefile` scanning **both** `src/` and `sdk/` permanently (safer; supports any future `src/` Go tool) rather than collapsing to `sdk/`-only. **Owner: Go QA + CI.**

10. **`docs/plans/gsl-status-line*.md` disposition.** Leave the old module-path strings as historical record, annotate with a one-line "superseded by sdk/" note, and add to the acceptance-grep exclude-list. **Owner: Architecture.**

11. **`.gitignore:75` correction (verified DEAD rule).** The rule reads `src/tmux-mgr/tmux-mg` (truncated — missing trailing `r`). `git check-ignore -v src/tmux-mgr/tmux-mgr` proves the binary is matched by `!src/**` (line 41), **not** by line 75 — the rule is non-functional today and the local binary is currently trackable. **Do not preserve the typo.** Re-path to the correct `sdk/tmux-mgr/tmux-mgr` so the local build artifact is actually ignored under the new tree. (Resolves adversary must-fix #3.) **Owner: CI.**

---

## 5. Execution Phases

### Phase 0 — Posture, namespace & tag decisions (PRE-TAG GATE, blocking)
- **Owning team:** Security (EM + Snowflake org security)
- **Steps:** Resolve Cross-Cutting Decisions 1–5 (public/private posture, repo visibility, namespace retention, tag scheme + signing, coverage warn-mode). Produce a go-install acceptance-test env spec (`GOPRIVATE`/`GONOSUMCHECK` values).
- **Deliverable:** Signed-off decision memo. No code changes.
- **Depends on:** Nothing — this is the gate.
- **Done when:** EM has recorded answers to all five decisions; no tag may be cut until the memo exists.

### Phase 1 — Umbrella ADR + RFC; reconcile PR #115
- **Owning team:** Architecture
- **Steps:** Author the ADR under `docs/adr/` (tracked via `!docs/**`) recording the path scheme, the path-prefixed tag scheme + `@latest`-until-first-tag consequence, the interim `@<commit>/@<branch>` install contract, and three alternatives (in-place `src/` rename / flat `sdk/` no path segment / `sdk/<tool>` with path segment — **chosen**). Repurpose PR #115 into this ADR. Run the **RFC 2-business-day comment window** (cross-team: Go, CI, Security). Freeze the per-module Definition-of-Done and the two-grep acceptance gate. Decide the `docs/plans/gsl-status-line*.md` disposition (Decision 10).
- **Deliverable:** Merged ADR; PR #115 reconciled; published DoD + acceptance-grep spec.
- **Depends on:** Phase 0.
- **Done when:** ADR merged to `main`; PR #115 closed-or-rebased; DoD + exclude-list agreed by Go + CI + Security leads after the comment window.

### Phase 2 — Migrate PILOT module (gsl) + land ALL shared-harness edits
- **Owning team:** Go
- **Steps:**
  1. **FIRST edit:** add `!sdk/` + `!sdk/**` to `.gitignore`; verify `git status --short -- sdk/` and `git check-ignore -v sdk/gsl/main.go` (sdk/ is invisible until opted in).
  2. `git mv src/gsl sdk/gsl`; set `go.mod` line 1 to `…/sdk/gsl`; rewrite all in-module imports anchored on the full old path `github.com/wenlock/dotfiles/gsl`. **Do NOT touch the 3 gsl testdata `/pull/` URL false positives.**
  3. Repoint `build.sh` LDFLAGS `-X` keys (gsl = `internal/version.*`); run `(cd sdk/gsl && go build ./... && go vet ./... && go mod tidy && git diff --quiet go.sum)` — **go.sum must be byte-identical** (drift = stop-the-line).
  4. **CROSS-CUTTING one-time harness edits (call out in PR body as the single exception):**
     - `scripts/test.sh:77` discovery → scan **both** `src/` and `sdk/`; and `scripts/test.sh:91` — **pin the module-dir resolution to the `dirname` of each `go.mod`, not `find -name "$mod" | head -n1`**, to avoid basename collisions under dual roots (resolves adversary must-fix #5). Keep `COVERAGE_MIN` basename keys unchanged.
     - `Makefile:17` (`bin`, gates on `build.sh`) and `Makefile:79` (`lint-go` loop, gates on `go.mod`) → iterate `src/* sdk/*`.
     - **`Makefile:73` `gofmt -l ./src` → `gofmt -l ./src ./sdk`** (resolves adversary must-fix #1; otherwise all migrated Go code silently escapes the gofmt gate from Phase 2 onward).
     - **CI `cache-dependency-path` at `docker-image.yml:70` AND `:121` → REPLACE `"src/**/*.sum"` with `"sdk/**/*.sum"`** (not append). A retained `src/**/*.sum` glob zero-matches after the final `src/` go.sum departs in Phase 5 and **fails `actions/setup-go`** (resolves adversary must-fix #2 / ordering hazard). Since this PR moves the first go.sum into `sdk/`, the `sdk/` glob is non-empty from here on.
  5. gsl-specific refs: `install.sh` gsl block + `GSL_FONT_SCRIPTS` (line 406) `src/gsl/scripts → sdk/gsl/scripts`; move `skill/SKILL.md` with the dir; create `sdk/gsl/GEMINI.md` + `CLAUDE.md → GEMINI.md` symlink; create `sdk/GEMINI.md` tree doc + symlink and link from root `GEMINI.md`/`CLAUDE.md` Repository Structure; add `sdk/` row to `README.md` structure table; update `sync-skills.sh:110` comment `src/gsl/skill → sdk/gsl/skill`.
  6. Rebuild `opt/bin/gsl`; run `gsl version` (confirm embedded path now `…/sdk/gsl`); run `scripts/test.sh` and both acceptance greps.
- **Deliverable:** One atomic PR — gsl fully under `sdk/gsl`, all shared-harness loops dual-scan, gofmt+cache fixed, `.gitignore` + docs scaffold established, `main` green.
- **Depends on:** Phase 1.
- **Done when:** `main` green; both acceptance greps empty for gsl (module-path word-boundary match, excluding `/pull/`); `gsl version` prints new path; test.sh + Makefile discover gsl under `sdk/`; `go.sum` unchanged.

### Phase 3 — Migrate wol (validates the LDFLAGS divergence)
- **Owning team:** Go
- **Steps:** `git mv src/wol sdk/wol`; `go.mod → …/sdk/wol`; rewrite the single import anchored on `github.com/wenlock/dotfiles/wol`. Repoint `build.sh` LDFLAGS — **wol injects into `cmd.*`, NOT `internal/version.*`** (do not template from gsl). Build/vet/tidy; assert `go.sum` unchanged. Update `install.sh` wol block; create `sdk/wol/GEMINI.md` + symlink (none today) and add to `sdk/GEMINI.md`. Rebuild `opt/bin/wol`; run `wol version` and confirm a **non-empty** version with `…/sdk/wol` (Go silently drops `-X` on an unresolved symbol — this is the catch). Coverage stays warn-only (wol 55.6% red, #51).
- **Deliverable:** Atomic PR moving wol to `sdk/wol` with correct `cmd.*` LDFLAGS; `main` green.
- **Depends on:** Phase 2 (harness already dual-scans; doc scaffold exists).
- **Done when:** `wol version` prints non-empty version with `…/sdk/wol`; acceptance greps clean; `go.sum` unchanged; coverage % unchanged (still 55.6%, warn-only).

### Phase 4 — Migrate gss (the dependency PROVIDER) and rebuild+install its binary
- **Owning team:** Go
- **Precondition (explicit):** `install.sh` / `build.sh` for gss **must be run from the `main` checkout, never a gss feature worktree** — a worktree install points `~/opt/bin/gss` at a transient path that vanishes, silently poisoning the Phase 5 e2e validity (resolves adversary ordering hazard / must-fix #5-install).
- **Steps:** `git mv src/gss sdk/gss`; `go.mod → …/sdk/gss`; rewrite all ~262 imports anchored on `github.com/wenlock/dotfiles/gss`. Repoint `build.sh` LDFLAGS (gss = `internal/version.*`); move `scripts/check-deps.sh` with the dir and update **`docker-image.yml:142` `src/gss/scripts/check-deps.sh → sdk/gss/scripts/check-deps.sh`**; build/vet/tidy with go.sum-unchanged assertion. Update `install.sh` gss block; move `skill/SKILL.md`; create `sdk/gss/GEMINI.md` + symlink; add to `sdk/GEMINI.md`. **Rebuild AND reinstall** `opt/bin/gss` (not just move — a stale binary breaks tmux-mgr silently); run `gss version`; confirm the `gss feature worker add --json` CLI contract is unchanged (rename is import-only). Run `scripts/test.sh` (gss floor 70%, ~80% PASS) and both greps.
- **Deliverable:** Atomic PR moving gss to `sdk/gss`; freshly-built `~/opt/bin/gss` on PATH; CI check-deps path updated; `main` green.
- **Depends on:** Phase 2. Independent of Phase 3, but **MUST precede Phase 5**.
- **Done when:** `gss version` prints `…/sdk/gss`; gss coverage gate still passes; CI check-deps runs from `sdk/gss/scripts`; greps clean; `go.sum` unchanged; `~/opt/bin/gss` rebuilt from new source.

### Phase 5 — Migrate tmux-mgr LAST (the runtime CONSUMER) + e2e guard
- **Owning team:** Go
- **Steps:** `git mv src/tmux-mgr sdk/tmux-mgr`; `go.mod → …/sdk/tmux-mgr`. **Anchor the rewrite on the full string `github.com/eraigosa/dotfiles/src/tmux-mgr`** (collapses org *and* `src→sdk`); a bare `src→sdk` sed elsewhere would be wrong. **Do NOT rewrite the gss shell-out strings** — two call sites (`cmd/agent_gss.go:18`, `cmd/pane_wrap.go:117`), both `exec.Command("gss", …)` resolving from PATH, stay verbatim. Repoint `build.sh` LDFLAGS (tmux-mgr = `cmd.*`); update `install.sh` tmux-mgr block (skill-symlink line works unchanged in the new dir); **re-path `.gitignore:75` to the correct `sdk/tmux-mgr/tmux-mgr`** (fix the dead typo per Decision 11). Move `scripts/e2e-gss-integration.sh` with the dir; the existing `GEMINI.md` + `CLAUDE.md` symlink moves via `git mv`; drop tmux-mgr from `src/GEMINI.md` Projects list. Rebuild `opt/bin/tmux-mgr`; run `tmux-mgr version`; then **manually run `scripts/e2e-gss-integration.sh`** (sandboxed via `GSS_*` env) against the freshly-built `~/opt/bin/gss`. Coverage warn-only (tmux-mgr 48.8% red, #50).
- **e2e enforcement caveat (resolves adversary must-fix #4):** `e2e-gss-integration.sh` is **NOT wired into any Make/CI target** (verified — CI's `make integration-test` runs only `gss version && tmux-mgr version && wol version` inside Docker via `test.sh:154`). This phase's e2e step is therefore a **manual operator gate**, not CI-enforced. Either (a) wire the script into a Make target in this PR before relying on it as a done-when, or (b) explicitly mark it manual so reviewers don't assume CI enforces the gss/tmux-mgr seam. Recommended: (a) add a `make e2e-gss` target and call it out.
- **Deliverable:** Atomic PR moving tmux-mgr to `sdk/tmux-mgr`; e2e green against the new gss; `src/` no longer contains any Go module; `main` green.
- **Depends on:** Phase 4 (a freshly-built `sdk/gss` binary installed on PATH).
- **Done when:** `tmux-mgr version` prints `…/sdk/tmux-mgr`; e2e passes against the rebuilt gss (manual gate or new `make e2e-gss`); both shell-out call sites intact; greps clean (anchored on the eraigosa/src path); `go.sum` unchanged; `.gitignore:75` corrected.

### Phase 6 — Final cleanup, anti-regression guard, first tags, stakeholder summary
- **Owning team:** CI
- **Steps:** Keep dual-scan in test.sh/Makefile (Decision 9). Update `.golangci.yml:9-12` comment to `sdk/<tool>` paths; refresh `.ci-baseline-issues.md` (~20 lines) and `GEMINI.md` example lines (55/58) to `sdk/`; annotate `docs/plans/gsl-status-line*.md` as historical. Add an **anti-regression guard**: extend `safety_guard_test.sh` (and the hook) to reject new `github.com/wenlock/dotfiles` or `github.com/eraigosa/dotfiles/src` imports in `.go` files — add one `assert_exit 0` (legit) and one `assert_exit 2` (blocked) case; run the driver to green before committing. Generalize the supply-chain gate: add `check-deps.sh` + a `govulncheck` CI step to gsl/wol/tmux-mgr (only gss has one today). Cut the first **signed** `sdk/<tool>/v0.x.y` tags per Phase-0 posture, then run the clean-cache `go install …@sdk/<tool>/v0.x.y` acceptance per module and confirm `<tool> version` shows the new path. EM publishes a plain-language stakeholder summary (what moved, the new install command per tool, PR #115 reconciled, backlog #50/#51 untouched).
- **Deliverable:** Cleaned harness + baseline docs; anti-regression hook with passing tests; per-module govulncheck/check-deps gates; first signed tags + passing go-install acceptance; executive summary.
- **Depends on:** Phases 2–5.
- **Done when:** `safety_guard_test.sh` passes with new block/allow cases; `go install …@sdk/<tool>/<tag>` succeeds and prints the new path for all four; no stale-path linter refs remain except sanctioned historical docs; summary published.

---

## 6. Per-Module Migration Runbook (repeatable one-at-a-time recipe)

Substitute `<TOOL>`, `<OLD>` (full old module path), `<VERPKG>` (`internal/version` for gss/gsl, `cmd` for wol/tmux-mgr).

1. **(First module only)** Add `!sdk/` + `!sdk/**` to `.gitignore`; verify with `git status --short -- sdk/` and `git check-ignore -v sdk/<TOOL>/main.go`.
2. `git mv src/<TOOL> sdk/<TOOL>` (preserve history — never copy+delete).
3. `sdk/<TOOL>/go.mod` line 1 → `module github.com/sfc-gh-eraigosa/dotfiles/sdk/<TOOL>`.
4. Rewrite all in-module imports anchored on the **full** `<OLD>` string:
   `grep -rl '<OLD>' sdk/<TOOL> --include='*.go' | xargs sed -i 's#<OLD>#github.com/sfc-gh-eraigosa/dotfiles/sdk/<TOOL>#g'` then `goimports -w`.
   - **gss/gsl/wol:** `<OLD>=github.com/wenlock/dotfiles/<TOOL>`.
   - **tmux-mgr:** `<OLD>=github.com/eraigosa/dotfiles/src/tmux-mgr` (anchored — do NOT use a bare `src→sdk` sed).
5. Repoint `sdk/<TOOL>/build.sh` LDFLAGS `-X` keys to `…/sdk/<TOOL>/<VERPKG>.*`.
6. Verify: `(cd sdk/<TOOL> && go build ./... && go vet ./... && go mod tidy && git diff --quiet go.sum)`. **A dirty `go.sum` is stop-the-line** (unintended dependency change).
7. Update `install.sh` block for `<TOOL>` (`src/<TOOL>/build.sh → sdk/<TOOL>/build.sh`; gsl also `GSL_FONT_SCRIPTS`).
8. Move `skill/SKILL.md` (gss/gsl/tmux-mgr); create `sdk/<TOOL>/GEMINI.md` + `ln -s GEMINI.md CLAUDE.md` (gss/gsl/wol new; tmux-mgr moves via git mv); link from `sdk/GEMINI.md`.
9. Rebuild `opt/bin/<TOOL>` via `build.sh`; run `<TOOL> version` and confirm the new module path + non-empty version (proves LDFLAGS landed).
10. Run `scripts/test.sh` and the **two-grep acceptance gate**.

---

## 7. Exhaustive Reference-Class Sweep Checklist

Per module, every box checked before the PR is mergeable:

- [ ] `go.mod` module line.
- [ ] All in-module `.go` imports (anchored on full old path; gsl `/pull/` testdata URLs are **false positives — do not touch**).
- [ ] `build.sh` LDFLAGS `-X` keys (gss/gsl=`internal/version`, **wol/tmux-mgr=`cmd`** — not templatable).
- [ ] `.gitignore`: `!sdk/` + `!sdk/**` (first PR); `.gitignore:75 → sdk/tmux-mgr/tmux-mgr` (tmux-mgr PR; fix the dead typo).
- [ ] `install.sh` build block (+ `GSL_FONT_SCRIPTS` for gsl).
- [ ] `scripts/test.sh:77` dual-scan discovery + `:91` dirname-pinned resolution (COVERAGE_MIN keys unchanged).
- [ ] `Makefile:17` (`bin`), `Makefile:79` (`lint-go` loop), **`Makefile:73` (`gofmt -l ./src ./sdk`)**.
- [ ] `.golangci.yml:9-12` comment block.
- [ ] CI `docker-image.yml:70` + `:121` `cache-dependency-path` **REPLACED** with `sdk/**/*.sum`; `:142` check-deps path.
- [ ] Per-module `GEMINI.md` + `CLAUDE.md → GEMINI.md` symlink; `sdk/GEMINI.md` tree doc; root `GEMINI.md`/`README.md` structure rows; drop tmux-mgr from `src/GEMINI.md`.
- [ ] `skill/SKILL.md` moved; `sync-skills.sh:110` comment.
- [ ] `opt/bin/<tool>` **rebuilt** (not moved); `<tool> version` shows new path.
- [ ] tmux-mgr shell-out strings (`agent_gss.go:18`, `pane_wrap.go:117`) **left verbatim**.
- [ ] Both acceptance greps clean; `go.sum` byte-identical.

**Two-grep acceptance gate** (single `grep` on the bare org string yields 3 confirmed false positives in gsl testdata `/pull/` URLs):
1. `grep -rn 'github.com/wenlock/dotfiles/<tool>\b' . --exclude-dir=.git` (tmux-mgr: also `github.com/eraigosa/dotfiles/src/tmux-mgr\b`) → empty.
2. `grep -rn 'src/<tool>\b' . | grep -v '/pull/'` → only sanctioned historical-doc lines (the `docs/plans/gsl-status-line*.md` exclude-list).

---

## 8. CI Strategy (green at every commit + grep-guard)

- **Green `main` at every boundary.** Each module is an independent, self-contained PR. The dual-scan harness edits land **once** in Phase 2 so later PRs are pure single-module changes.
- **Cache-glob replace, not append** (`docker-image.yml:70,121`): `src/**/*.sum → sdk/**/*.sum`. A retained `src/` glob **zero-matches and fails `actions/setup-go`** at the Phase 5 boundary — replacing it in Phase 2 (when the first go.sum is already in `sdk/`) keeps the glob non-empty throughout.
- **gofmt gate** (`Makefile:73`): widened to `./src ./sdk` so migrated Go code stays covered by `make lint` (`docker-image.yml:103`).
- **Per-module CI acceptance signal:** `test.sh:154` already runs `gss version && tmux-mgr version && wol version` in Docker — the embedded version path is a cheap, CI-wired post-move check (add `gsl version` if desired).
- **e2e guard is manual today.** `e2e-gss-integration.sh` is in no Make/CI target. Phase 5 either wires it into a new `make e2e-gss` target or flags the step as a manual operator gate (Decision in Phase 5).
- **Anti-regression hook (Phase 6):** `safety_guard.sh` rejects re-introduced `github.com/wenlock/dotfiles` / `github.com/eraigosa/dotfiles/src` imports in `.go` files, with new allow/deny test cases in `safety_guard_test.sh`.

---

## 9. Supply-Chain Checklist

- [ ] Posture (public/private), repo visibility, namespace retention resolved **before any tag** (Phase 0).
- [ ] Signed annotated tags + GitHub tag-protection ruleset on `sdk/<tool>/v*`.
- [ ] Start at v0.x (defer v1 sumdb immutability).
- [ ] `govulncheck ./...` per module added to CI (absent today).
- [ ] `check-deps.sh` license/seam gate generalized to gsl/wol/tmux-mgr (only gss has one).
- [ ] First `go install` acceptance run with the correct `GOPRIVATE`/`GONOSUMCHECK` env from the Phase 0 spec.

---

## 10. Verification / Acceptance (post-tag, per module)

From a clean cache with the Phase-0 env:
```
GOPATH=$(mktemp -d) go install github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>@sdk/<tool>/v0.x.y
<tool> version   # must print github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool> + non-empty version
```
Plus: `go.sum` byte-identical across the move; both acceptance greps empty; gss coverage ≥70%; tmux-mgr `e2e-gss-integration.sh` green against the rebuilt gss.

---

## 11. Rollback

- **Per-module, pre-tag:** revert the single atomic PR (`git mv` is reversible; no tag exists yet, so nothing is published to the sumdb). `main` returns to green immediately.
- **Post-tag:** tags are immutable in the sumdb — you **roll forward** (cut `sdk/<tool>/v0.x.(y+1)`), never re-point. A bad path/posture decision discovered after the first public fetch is **not reversible**; this is why Phase 0 is the hard gate.
- **Harness rollback:** the Phase 2 dual-scan edits are backward-compatible (still scan `src/`), so reverting a later module PR leaves the harness functional for the not-yet-moved modules.

---

## 12. Phase → Team Matrix

| Phase | Owning team |
|---|---|
| Phase 0 — Posture, namespace & tag decisions (pre-tag gate) | Security (EM + org security) |
| Phase 1 — Umbrella ADR + RFC; reconcile PR #115 | Architecture |
| Phase 2 — Migrate pilot gsl + shared-harness edits | Go |
| Phase 3 — Migrate wol (LDFLAGS divergence) | Go |
| Phase 4 — Migrate gss (provider) + rebuild binary | Go |
| Phase 5 — Migrate tmux-mgr (consumer) + e2e guard | Go |
| Phase 6 — Cleanup, anti-regression, first tags, summary | CI |
