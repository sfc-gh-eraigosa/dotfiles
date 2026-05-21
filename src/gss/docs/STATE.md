# gss v1.0 — Execution State (live cursor)

This file is the **handoff snapshot** for the gss v1.0 stacked PR series.
It lives on the long-running draft PR branch `wip/gss-v1-staging` and is
refreshed after every playground PR opens or merges.

The branch this file lives on is **never merged** — it exists only so a
new machine can pick up the work without cloning the playground.

The real per-PR review surface is the stack of small PRs in the
**playground** repo (see "Cursor" below). When v1.0 is fully validated,
PR-61 (the final integration) opens *its own* non-draft PR against
`dotfiles:main` and lands the validated work.

## Cursor

- **Most recent**: PR-36 — `internal/feature` checkpoint --auto. **Batch G
  core done** (PR-32–36: start/worker, list, conflicts, checkpoint, auto).
  Playground: #50 start, #51 list, #52 conflicts, #53 checkpoint (merged
  pr28 for stack/body.go; added `GH` field to feature Service), #54 auto —
  a LINEAR feature stack pr32→…→pr36 off pr31. dotfiles internal/ now
  mirrors PR-01–36 (+ stack body/restack + tmpl from Batch F).
- **Playground branches in flight**:
  - `test_gss` — integration trunk (root of the stack).
  - `pr01-internal-errors` … `pr09-internal-repo` — PRs #23–#31 (Batch A,
    linear).
  - Batch B fan-out siblings off `pr09`: `pr10`…`pr14` — PRs #32–#36
    (approval, backup, sync, scan, status).
  - Batch C (cmd leaves PR-15–16) — **dotfiles-only**, no playground PR.
  - Batch D **linear stack** off `pr09`: `pr17-registry-schema` #37 →
    `pr18-registry-lock` #38 → `pr19-registry-reconcile` #39 →
    `pr20-worktree-backend` #40 → `pr21-worktree-git` #41.
- **Next PR**: PR-37 — `internal/feature/pr.go` (`feature pr --ready`:
  promote draft→ready via `gh pr ready`, gated on the approval token from
  PR-10; refuses without token → `ErrPRReadyNeedsToken`; resolution #11).
  Stacks linearly on pr36. Then PR-38 rebase, PR-39 restack, then
  done/merged finish Batch G. (Old PR-32 base note below is historical:)
  the biggest batch wires registry + worktree + identity + tmpl + stack —
  see the Batch E integration note; plan it like pr22 did. Playground PR.
- **PRs remaining in plan**: 30 (PR-32 through PR-61). (Playground PRs
  open: #42–#49; cmd leaves PR-15/16/23/25 dotfiles-only.)
- **Batch E integration note**: orchestrators need fanned-out Batch B
  packages. pr22 merged pr10/11/12 (approval/backup/sync) for a coherent
  base; later Batch E PRs stack linearly on the prior E PR. cmd-leaf PRs
  (23, 25) are dotfiles-only and mirror their prerequisite internal package
  into the same staging commit.
- **Snapshot cadence**: dotfiles snapshots BATCHED ~every 5 PRs (user pick
  2026-05-21). Playground PRs are individual + the source of truth;
  reconcile STATE.md against `gh pr list` when resuming.
- **This staging snapshot**: dotfiles `internal/` mirrors PR-01–21
  (through `pr21-worktree-git` @ `2e03c71`), plus dotfiles-only classic
  wiring (build.sh + cmd/version.go + cmd/config.go + cmd/{status,scan,
  backup,sync}.go from PR-15–16).
- **External dependencies** (all Allowed, licenses verified):
  `gopkg.in/yaml.v3 v3.0.1` (PR-05, MIT+Apache-2.0), `golang.org/x/text
  v0.37.0` (PR-08, BSD-3-Clause), `github.com/gofrs/flock v0.13.0` +
  `golang.org/x/sys v0.37.0` (PR-18, BSD-3-Clause).

## Handoff procedure (new machine)

1. **Clone dotfiles**:
   ```
   git clone <dotfiles-remote> ~/git/dotfiles
   cd ~/git/dotfiles
   git checkout wip/gss-v1-staging
   ```
2. **Optional — clone the playground** for the full per-PR commit history
   and PR review surface:
   ```
   git clone <playground-remote> ~/GitHub/playground
   cd ~/GitHub/playground
   git checkout pr21-worktree-git   # Batch D stack tip (contains registry + worktree)
   ```
3. **Tooling install** (Phase 3 prerequisites):
   ```
   go install github.com/google/go-licenses@latest
   brew install git-machete           # or: pip install git-machete
   ```
4. **Auth check**:
   ```
   gh auth status
   ```
   PRs in this series have been opened under `sfc-gh-eraigosa`. If this
   machine uses a different `gh` identity, switch before continuing.
5. **Resume**: invoke the agent and say "continue" — it will read
   `src/gss/docs/plan.md`, find the next un-merged deliverable per the
   cursor above, and proceed.

## What's in the staging branch right now

Additive `internal/` packages, plus the small classic-code wiring noted
below (PR-04, PR-05):

- `src/gss/LICENSE` — Apache 2.0, mirrored from upstream.
- `src/gss/internal/errors/` — 14 sentinels, exit-code map (10–29), JSON
  envelope with control-char + ANSI + worker_ref defence (PR-01).
- `src/gss/internal/git/` — `Runner` interface + `SystemRunner` real
  impl + `fake.Runner` recording fake (PR-02).
- `src/gss/internal/gh/` — `Client` interface (`PRCreate`/`PREdit`/
  `PRReady`/`PRView`/`PRList`/`RepoView`/`AuthStatus`) + `SystemClient`
  real impl over an `Exec` seam + `fake.Client` stateful, per-verb
  scriptable fake + `testdata/gh_responses/*.json` fixtures (PR-03).
- `src/gss/internal/version/` — build-metadata single source of truth:
  `Version`/`Commit`/`BuildDate`/`Dirty` (set via `-X` ldflags) + `Get()`
  with display fallbacks (PR-04).
- `src/gss/internal/config/` — layered config loader (built-in → YAML →
  `GSS_*` env → flag), `*ParseError`, first-run stub (`WriteStubIfMissing`),
  `Marshal`, and the `Clock` seam (`clock.go`) (PR-05). Depends on
  `gopkg.in/yaml.v3`.
- `src/gss/internal/identity/` — embedded 256-word suffix pool
  (`wordlist.txt` + `Words()`, PR-06); `WorkerRef` type + `ParseWorkerRef`,
  `RNG` interface + `SystemRNG` (crypto/rand), and `AllocateRef`
  (5-retry suffix draw, caller-supplied suffix rejected) (PR-07); segment
  grammar (`ValidateFeature`/`User`/`Purpose`), `ValidateDescription`
  (NFC + strip + bounds), and `ResolveUser` precedence (PR-08, +x/text).
- `src/gss/internal/repo/` — `NWO` type, `Resolver` (gh RepoView → origin
  URL fallback, `--repo` shadow), and the origin-keyed `.nwo` cache (PR-09).
- `src/gss/internal/approval/` — HEAD-bound `Verifier` (verify-then-consume
  token, `Issue`, `--force-autonomous` bypass; wraps
  `ErrApprovalTokenMissing`) (PR-10).
- `src/gss/internal/backup/` — `Service.Create` safety branch
  `backup/gss-<ts>` (Clock-driven, monotonic-suffix idempotent) (PR-11).
- `src/gss/internal/sync/` — `Service.Sync` fetch→pull --rebase; rebase
  failure wraps `ErrRebaseConflict` (PR-12).
- `src/gss/internal/scan/` — `Scanner.Scan` dirty-repo walker + `Format`
  (`[DIRTY] <path>` contract) + `GitDirty` (PR-13).
- `src/gss/internal/status/` — `Service.Status` + `Format` (porcelain →
  classic report, byte-identical) (PR-14).
- `src/gss/internal/registry/` — registry.json schema + JSON round-trip
  with unknown-field preservation (PR-17); locked/atomic `Store`
  (flock + tmp+rename + 0600 + uid guard, PR-18); `Reconciler` vs git
  worktree / gh pr with `--repair` (PR-19).
- `src/gss/internal/worktree/` — `Backend` interface + `Register`/`Open`
  + `backendtest` contract suite (PR-20); `git/` v1 backend
  (worktree add/remove/list/status, sets rebase.updateRefs) (PR-21).
- `src/gss/internal/classic/` — `Pusher` (PR-22) + `PRer` (PR-24)
  orchestrators (approval→backup→sync→push→PR; pr cuts feature/gss-<ts>).
- `src/gss/internal/mode/` — `IsInWorker(cwd, registry)` worker-mode
  detector used by all classic cobra leaves (PR-26).
- `src/gss/internal/stack/` — parent/child compute (PR-27), PR-body stack
  section + marker-strip injection defence (PR-28), merge/restack re-target
  math + auto-promote eligibility (PR-29). Pure logic.
- `src/gss/internal/tmpl/` — FEATURE.md/WORKER.md renderer with user-field
  sanitisation (PR-30) + embedded `*.md.tmpl` + loaders (PR-31).
- `src/gss/internal/feature/` — the feature `Service`: `Start` + `WorkerAdd`
  (PR-32), `List` (+spawned_by informational guard, PR-33), `Conflicts`
  (PR-34), `Checkpoint` (PR-35), `AutoCheckpoint` (PR-36). Wires registry +
  worktree + identity + tmpl + stack + gh + git.
- `src/gss/cmd/{push,pr}.go` — rewired onto `classic.{Pusher,PRer}` with
  the `mode.IsInWorker` + `classicAllowed` (`ErrWrongMode`) gate (PR-23/25,
  routed through internal/mode at PR-26).
- `src/gss/docs/STATE.md` — this file.

**Classic-code wiring (dotfiles-only).** Because the playground module is
stripped (no `cmd/`, no `build.sh`, no cobra), each PR that touches the
classic tool reviews its new package in the playground PR and applies the
wiring here. So far:

- `src/gss/build.sh` (PR-04) — ldflag targets retargeted from
  `…/gss/cmd.{Version,…}` to `…/gss/internal/version.{Version,…}`.
- `src/gss/cmd/version.go` (PR-04) — now reads `version.Get()`; its local
  `Version`/`Commit`/`Dirty`/`BuildDate` vars are removed. Verified
  end-to-end: a stamped binary renders the injected values; an unstamped
  binary falls back to dev/none/…
- `src/gss/cmd/config.go` (PR-05) — new `gss config print` (dumps the
  effective config; writes the first-run stub) and `gss config check`
  (validates the file + confirms `git`/`gh` resolve on PATH). Verified
  end-to-end with a temp `HOME`.
- `src/gss/go.mod` + `go.sum` (PR-05) — `gopkg.in/yaml.v3 v3.0.1` added as
  a direct require; `go mod tidy` promoted cobra to a direct require.
- `src/gss/cmd/status.go` (PR-15) — now delegates to `internal/status`
  (`status.NewService(git.NewSystemRunner()).Status`); output byte-identical
  (smoke-verified). First Batch C cmd leaf.
- `src/gss/cmd/{scan,backup,sync}.go` (PR-16) — rewired onto
  `internal/{scan,backup,sync}`. scan injects the kept `isDirty` into
  `scan.Scanner`; backup uses `config.SystemClock`; sync distinguishes
  `ErrRebaseConflict` from fetch errors. Output byte-identical
  (smoke-verified). Minor: sync drops the cosmetic "Attempting to
  pull/rebase onto <branch>…" progress line (branch is resolved inside
  internal/sync). `isDirty` kept in cmd/scan.go for the existing
  cmd/status_test.go (TestIsDirty); retire in a later cleanup.

Otherwise the classic gss code (`cmd/*.go`, `main.go`) is unchanged. The
remaining `internal/` packages are foundation layers the classic code
starts using at PR-22 (`internal/classic/push.go`) and beyond;
`internal/gh` first gets wired in at PR-24 (`internal/classic/pr.go`).

## Open carry-forward notes

| # | Source | Item | Disposition |
|---|--------|------|-------------|
| 1 | PR-01 Security LOW | `maxValidationField` / `maxValidationReason` constants declared but not enforced. | Defer to PR-07 (identity migration) or fold into a small follow-up. |
| 2 | PR-01 Security LOW | `workerSuffixRe` unused; `<purpose>` and `<suffix>` not structurally split. | Defer to PR-07. |
| 3 | PR-01 Security LOW | `requiredCodeOnEmpty` cosmetic constant. | Defer to PR-07. |
| 4 | PR-02 QA YELLOW | No compile-time `var _ git.Runner = (*fake.Runner)(nil)` in fake package. | ✅ Done in PR-03 — both `gh.SystemClient` and `fake.Client` assert `var _ gh.Client`. (Retro-fit on `git/fake` still open → fold into PR-50.) |
| 5 | PR-02 QA YELLOW | No `name == ""` contract test. | ✅ Done in PR-03 — empty/invalid-input contract pinned on every gh mutating verb. (git/fake retro-fit → PR-50.) |
| 6 | PR-02 QA YELLOW | Unbounded `bytes.Buffer` for huge `git log`. | Track for a future hardening PR. |
| 7 | PR-02 QA YELLOW | Combined-Stdout+Stderr+Err ordering not pinned. | ✅ Done in PR-03 (gh analog) — gh `Exec` returns stdout only, folds stderr into `*ExecError`; contract tested. (git/fake retro-fit → PR-50.) |
| 8 | PR-02 Architect | PR-50 must add CI grep enforcing "no `os/exec` outside `internal/git/` and `internal/gh/`". | Track for PR-50. |
| 9 | PR-02 Architect | PR-03 (`gh/fake`) must use per-verb scripting, not the FIFO shape from `git/fake`. | ✅ Done in PR-03 — per-verb error scripting (`ScriptError(verb, …)`); proven by `TestFakeClient_PerVerbScripting`. |
| 10 | PR-03 | `gh.systemExec.Run` (the real-`gh` subprocess shell) is uncovered by unit tests by design — exercising it needs a live `gh` + repo. | Optional: add an integration test behind a build tag / `GSS_GH_INTEGRATION=1` env gate in a future hardening PR. |

## Sync log

| Date | Synced from | SHA | Notes |
|------|-------------|-----|-------|
| 2026-05-20 | `playground:pr02-internal-git-runner` | `e8c9b8a` | Initial staging snapshot — PR-01 (sentinels + JSON envelope) + PR-02 (`internal/git` Runner + fake). Tests pass in dotfiles context; `go build ./internal/...` clean. |
| 2026-05-21 | `playground:pr03-internal-gh-client` | `4f6bedc` | PR-03 — `internal/gh` `Client` interface + `SystemClient` (over an `Exec` seam) + stateful per-verb `fake.Client` + `testdata/gh_responses/*.json`. 31 tests; coverage gh 77.7%, fake 87.8%; build + full module tests clean in dotfiles context. No new deps. `-race` skipped on the aarch64 dev host (47-bit VMA vs ThreadSanitizer's 48-bit, `unsupported VMA range`); x86 CI covers it. PR #25 stacked on PR-02. |
| 2026-05-21 | `playground:pr04-internal-version` | `91e8f44` | PR-04 — `internal/version` build-metadata single source of truth (`Version`/`Commit`/`BuildDate`/`Dirty` + `Get()` fallbacks). 4 tests; coverage 100%. Cross-repo split (per user decision): package ships in playground PR #26; dotfiles-only wiring retargets `build.sh` ldflags and cuts `cmd/version.go` over to `version.Get()` (local build vars removed). Full dotfiles module builds + all tests pass; ldflag injection verified end-to-end (stamped binary shows injected values, unstamped falls back to dev/none/…). No new deps. PR #26 stacked on PR-03. |
| 2026-05-21 | `playground:pr05-internal-config` | `81d794d` | PR-05 — `internal/config` layered loader (built-in → YAML → `GSS_*` env → flag), `*ParseError`, first-run stub round-tripping to `Default()`, `Marshal`, `Clock` seam. 16 tests; coverage 89.5%. **First external dep**: `gopkg.in/yaml.v3 v3.0.1` (dual MIT + Apache-2.0; LICENSE verified at github.com/go-yaml/yaml/blob/v3.0.1/LICENSE; both Allowed). Dotfiles-only wiring adds `cmd/config.go` (`gss config print`/`check`) and the dep to `go.mod`/`go.sum`. Full module builds + all tests pass; `gss config print`/`check` verified end-to-end with a temp HOME. PR #27 stacked on PR-04. |
| 2026-05-21 | `playground:pr06-identity-wordlist` | `e859719` | PR-06 — `internal/identity` embedded 256-word suffix pool (`//go:embed wordlist.txt` + `Words()` defensive copy). 5 tests; coverage 100%. Data file mechanically pre-validated (256 unique, all 3–5 lowercase ASCII). No new deps; playground-only (no classic wiring). PR #28 stacked on PR-05. |
| 2026-05-21 | `playground:pr07-identity-suffix` | `f7d1a19` | PR-07 — `internal/identity` `WorkerRef` (+`ParseWorkerRef` round-trip), `RNG`/`SystemRNG` (crypto/rand), `AllocateRef` (suffix-less first unless forced; 5-retry; `ErrSuffixExhausted`; caller suffix ignored). 11 tests; coverage 92.5%. No new deps; playground-only. PR #29 stacked on PR-06. |
| 2026-05-21 | `playground:pr08-identity-validate` | `d931224` | PR-08 — `internal/identity` segment grammar (`ValidateFeature`/`User`/`Purpose` wrapping `ErrInvalidIdent`), `ValidateDescription` (NFC + strip ANSI/markers/newlines + control reject + 1–240 cp), `ResolveUser` precedence (--user→gh→email-slug→$USER). 13 tests; coverage 95.8%. **New dep**: `golang.org/x/text v0.37.0` (BSD-3-Clause; verified at github.com/golang/text/blob/master/LICENSE) for NFC. Mirrored to dotfiles incl. go.mod/go.sum; no cmd wiring. PR #30 stacked on PR-07. |
| 2026-05-21 | `playground:pr09-internal-repo` | `89aa3c6` | PR-09 — `internal/repo` NWO resolution: `Resolver.Resolve` (--repo shadow → `.nwo` cache → `gh.RepoView` → origin-URL parse → refuse), `ParseNWO`/`ParseRemoteURL`, origin-keyed cache (invalidates on origin change). 9 tests; coverage 86.3%. No new deps; playground-only. Ends Batch A. PR #31 stacked on PR-08. |
| 2026-05-21 | `playground:pr10-internal-approval` | `d2afe50` | PR-10 — `internal/approval` HEAD-bound token (`Verify` verify-then-consume, `Issue`, force-autonomous bypass; typed `*Error{Missing|Mismatch}` wrapping `ErrApprovalTokenMissing`). Mirrors classic cmd/push.go semantics. 5 tests; coverage 71%. No new deps; playground-only. Starts Batch B (fan-out sibling off PR-09). PR #32. |
| 2026-05-21 | `playground:pr11-internal-backup` | `d250242` | PR-11 — `internal/backup` `Service.Create` safety branch `backup/gss-<YYYYMMDD-HHMMSS>` (config.Clock-driven, byte-identical name to classic cmd/backup.go; monotonic `-N` suffix on collision, cap 1000). 3 tests; coverage 91.7%. No new deps; playground-only (fan-out sibling off PR-09). PR #33. |
| 2026-05-21 | `playground:pr12-internal-sync` | `e2ffde7` | PR-12 — `internal/sync` `Service.Sync` (resolve branch→`fetch origin`→`pull --rebase`; fetch precedes pull; rebase failure wraps `ErrRebaseConflict`). 4 tests; cov 93.3%. No deps. PR #34 (sibling off PR-09). |
| 2026-05-21 | `playground:pr13-internal-scan` | `2385050` | PR-13 — `internal/scan` `Scanner.Scan` (WalkDir, nested repos, symlink-loop-safe) + `Format` (`[DIRTY] <path>` contract) + `GitDirty`. Test trees built at runtime (committed `.git` fixtures impossible). 5 tests; cov 95%. No deps. PR #35 (sibling off PR-09). |
| 2026-05-21 | `playground:pr14-internal-status` | `f158791` | PR-14 — `internal/status` `Service.Status` + `Format` (porcelain → classic report byte-identical: "No changes detected"/"Changes in"/" - line"). 4 tests; cov 100%. No deps. PR #36 (sibling off PR-09). Ends Batch B. |
| 2026-05-21 | (dotfiles-only) | — | **Batch C** PR-15/16 — cmd leaves rewired in place onto internal packages (`cmd/status.go` → internal/status; `cmd/{scan,backup,sync}.go` → internal/{scan,backup,sync}). Output byte-identical, smoke-verified. No playground PRs (cmd-leaf decision). |
| 2026-05-21 | `playground` PR-32–36 (#50–#54) | (this) | **Batch G core snapshot** — `internal/feature` Service: start+worker (#50), list + spawned_by informational static-grep guard (#51), conflicts/never-resolves (#52), checkpoint (fetch→rebase→PR create/edit→body render; merged pr28 for stack/body; added Service.GH) (#53), checkpoint --auto (no-op silence, tracked-only WIP commit, detached/conflict skip→WORKER.md, draft-only, --dry-run) (#54). Linear feature stack pr32→pr36 off pr31. All build + tests pass in dotfiles; coverage ~85–89%. No new deps. PR-37–39 (pr --ready / rebase / restack) + done/merged remain in Batch G. |
| 2026-05-21 | `playground` PR-27–31 (#45–#49) | (prev) | **Batch F batch snapshot** — `internal/stack` (compute #45 / body+marker-strip #46 / restack #47) + `internal/tmpl` (renderer #48 / embed #49). Pure-logic stack + template renderer with user-field sanitisation. Assembled into dotfiles from the sibling branches; all build + tests pass. Coverage: stack 98–100%, tmpl ~88–91%. No new deps. PR-bodies/markers are the PR-injection-defence surface (security #7). |
| 2026-05-21 | (dotfiles staging, PR-22–26) | `9bc2947`…(prev) | **Batch E** classic orchestration. `internal/classic/push` (PR-22 #42) + `internal/classic/pr` (PR-24 #43) + `internal/mode` (PR-26 #44), mirrored to dotfiles; `cmd/push`/`cmd/pr` rewired onto the orchestrators with the `mode.IsInWorker`/`ErrWrongMode` gate (cmd leaves PR-23/25 dotfiles-only). pr22 merged Batch B siblings pr10/11/12 for a coherent base; pr24/pr26 stack linearly. All build + tests pass; coverage classic ~82%, mode 91.7%. No new deps. |
| 2026-05-21 | `playground:pr21-worktree-git` | `2e03c71` | **Batch D batch snapshot** (PR-17–21). `internal/registry` (schema+unknown-field preservation #37; flock/atomic/0600/uid `Store` #38; `Reconciler` #39) + `internal/worktree` (`Backend` interface + contract suite #40; `git` v1 backend #41). New deps `gofrs/flock v0.13.0` + `golang.org/x/sys v0.37.0` (BSD-3-Clause). All build + tests pass in dotfiles (incl. real-git contract suite). Coverage: registry 82.9%, worktree 100% / git-backend 68.4%. |
