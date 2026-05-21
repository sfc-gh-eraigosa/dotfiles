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

- **Most recent PR opened**: PR-08 — `internal/identity` segment validation
  + user/purpose resolution (#30 in playground).
- **Playground branches in flight**:
  - `test_gss` — integration trunk (root of the stack).
  - `pr01-internal-errors` — PR #23, awaiting human merge to `test_gss`.
  - `pr02-internal-git-runner` — PR #24, stacked on PR-01, awaiting merge.
  - `pr03-internal-gh-client` — PR #25, stacked on PR-02, awaiting merge.
  - `pr04-internal-version` — PR #26, stacked on PR-03, awaiting merge.
  - `pr05-internal-config` — PR #27, stacked on PR-04, awaiting merge.
  - `pr06-identity-wordlist` — PR #28, stacked on PR-05, awaiting merge.
  - `pr07-identity-suffix` — PR #29, stacked on PR-06, awaiting merge.
  - `pr08-identity-validate` — PR #30, stacked on PR-07, awaiting merge.
- **Next PR**: PR-09 — `internal/repo/` NWO (name-with-owner) detection
  via `gh repo view` with an origin-URL fallback + per-repo cache. Stacks
  on PR-08.
- **PRs remaining in plan**: 53 (PR-09 through PR-61).
- **This staging snapshot**: reflects playground branch
  `pr08-identity-validate` at SHA `d931224`, plus dotfiles-only classic
  wiring (build.sh + cmd/version.go + cmd/config.go) described below.
- **External dependencies**: `gopkg.in/yaml.v3 v3.0.1` (PR-05),
  `golang.org/x/text v0.37.0` (PR-08, BSD-3-Clause, for NFC).
- **First external dependency**: `gopkg.in/yaml.v3 v3.0.1` (dual
  MIT + Apache-2.0; allowed) added in PR-05.

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
   git checkout pr08-identity-validate
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
