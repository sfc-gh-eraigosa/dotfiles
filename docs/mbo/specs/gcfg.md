# gcfg — GitHub config as code — spec

- **Slug:** gcfg
- **Date:** 2026-09-05
- **Status:** Draft
- **Relates to:** design `../designs/gcfg.md` · issue [#284](https://github.com/sfc-gh-eraigosa/dotfiles/issues/284) · plan `../plans/gcfg.md`

## 1. Goal

A repository owner declares every GitHub setting that matters for a repo (and, from an
org's `.github` repo, for the org) in one reviewed file, `.github/gcfg.yaml`, and can at
any time prove the live repo matches it (`verify`), converge it (`apply`), adopt an
existing repo in one command (`export`), browse and edit it in a TUI, and have GitHub
Actions do the proving and converging on every change — with credentials the tool
itself helps set up (fine-grained PAT or a GitHub App) and diagnoses.

## 2. Use cases

**UC1 — Adopt this repo.** Actor: owner, local shell. Trigger: `gcfg export`. Flow:
gcfg resolves the `gh` token, reads every family, writes `.github/gcfg.yaml`
(comments on each key), imports `.github/rulesets/*.json`. Acceptance: file lints
clean; `gcfg verify` immediately exits 0; secret scanning + push protection appear
under `repo.security` exactly as PR #275 left them; `non_provider_patterns` is
exported with a comment that it is not honoured on this plan.

**UC2 — Catch drift.** Actor: verify workflow, on a PR and nightly. Trigger: someone
flips "Automatically delete head branches" in the UI. Flow: `gcfg verify --markdown`
→ step summary lists `repo.general.delete_branch_on_merge: want true, live false`;
exit 1; the check is red. Acceptance: the exact key path and both values are in the
summary; a clean repo exits 0 with a one-line summary.

**UC3 — Converge.** Actor: owner (TTY) or apply workflow. Trigger: `gcfg apply`.
Flow: plan printed (per family: change / create / delete), confirmation (TTY) or
`--yes`, writes, **re-read verify**, exit 0 only if the re-read is clean. Acceptance:
a key that GitHub does not honour (non-provider patterns) fails the run naming the key
and the reason; nothing else is touched by that failure.

**UC4 — Bootstrap a new repo.** Actor: owner. Trigger: `gcfg init` in a repo with no
file. Flow: smart defaults (squash-only merges, delete branch on merge, secret
scanning + push protection on, Actions default permissions `read`, dependabot alerts
on, a `main` ruleset requiring PRs + linear history) written with comments;
`gcfg init --from sfc-gh-eraigosa/dotfiles` instead copies that repo's file (fetched
via the API, falling back to `export` of its live settings when it has none).
Acceptance: lints clean; `gcfg plan` shows what would change; nothing applied.

**UC5 — Browse and edit.** Actor: owner. Trigger: `gcfg tui`. Flow: tree of families
and keys with live status colors after `v`; `/deleteb` jumps to the key; `e` toggles;
`w` writes YAML; `a` shows the apply plan and asks. Acceptance: YAML round-trips with
comments preserved; `q` without `w` writes nothing.

**UC6 — Set up credentials.** Actor: owner. Trigger: `gcfg auth doctor`. Flow: prints
the resolved token source and, per family, `read ✓ / write ✓|✗` from a live probe;
`gcfg auth pat` prints the exact fine-grained PAT permission checklist for the target
and verifies a pasted token; `gcfg auth app create` runs the ghapp manifest flow and
`gcfg auth app install` records the installation. Acceptance: with the `gh` token,
every repo family reads and writes; org families report the missing `admin:org`
scope by name; with an App token, `doctor` shows Administration read/write.

**UC7 — Install the Actions.** Actor: owner. Trigger: `gcfg actions install verify`
then `apply`. Flow: two workflow files rendered with the pinned gcfg version; the
verify job uses `actions/create-github-app-token` when `APP_ID`/`APP_PRIVATE_KEY`
secrets are referenced, else `GCFG_TOKEN`. Acceptance: `actionlint` clean; the first
verify run on this repo is green; a deliberate drift PR is red; the apply run converges
it and re-verifies green.

**UC8 — Org settings from the `.github` repo.** Actor: org admin. Trigger: `gcfg
export --org` in `<org>/.github`. Flow: `org:` block with profile, member privileges,
security defaults for new repos, Actions permissions, rulesets, and installed apps
(report-only). Acceptance: the same file in any other repo fails lint with "org block
only allowed in <org>/.github".

## 3. Architecture

Two Go modules under `sdk/`, both mirroring `sdk/gss`/`sdk/gff` (cobra `cmd/`,
`internal/` behind mockable seams, `libs/log`, tag-driven `build.sh` versions):

- **`sdk/ghapp`** — `pkg/ghapp` (manifest-flow create, PEM store, RS256 JWT,
  installation token mint with cache, installations list) + `ghapp` CLI. Depends only
  on stdlib + `go-gh/v2` for host/token discovery. Consumed by gcfg's credential chain
  and available to any future tool.
- **`sdk/gcfg`**:
  - `internal/schema` — typed structs, strict YAML load, JSON Schema gen, lint.
  - `internal/gh` — `Client` interface (`Get/Patch/Put/Post/Delete(ctx, path, body)
    (status, json)`), real impl over `go-gh` REST with retries/rate-limit, and a
    recording **fake** driven by fixture files (every test runs offline).
  - `internal/family` — `Family` interface + one package per family
    (`general, security, rulesets, actions, labels, autolinks, environments,
    secrets, webhooks, collaborators, pages`; org: `profile, members,
    security_defaults, actions, rulesets, apps`). Each declares `Name()`,
    `Permission()`, `Read`, `Diff`, `Apply`, `Export`.
  - `internal/engine` — `Export`, `Verify`, `Plan`, `Apply` over families; ownership
    semantics; re-read verify; exit-code mapping.
  - `internal/report` — `Finding{Family, Key, Kind(drift|unmanaged|unreadable|
    not_honoured), Want, Live, Reason}` → tty/json/markdown.
  - `internal/tui` — bubbletea model (tree, search, editors, write-back, verify/apply).
  - `internal/actions` — workflow templates + renderer + uninstall.
  - `cmd/` — verbs in §4.

Data flow: file → schema → desired; REST → families → live; engine diffs → findings →
report/TUI; apply plan → confirm → family.Apply → re-read → findings must be empty.

## 4. Behavior / features

- **F1 Schema & lint.** `gcfg.yaml`: `version: 1`, `ownership: declared|full`,
  `repo:` families, optional `org:`. Unknown keys rejected with path. Per-family
  `ownership` override. `gcfg schema` emits JSON Schema; `.github/gcfg.schema.json`
  committed and CI-checked for drift. Lint: org block only in `<owner>/.github`;
  unique ruleset/label/env names; enum values; **no field may hold a secret value**
  (secrets/webhooks are names + metadata; a value-shaped string is a lint error).
- **F2 Export.** `gcfg export [--out path] [--org]` reads all families, writes
  commented YAML, imports `.github/rulesets/*.json` when present, marks known
  not-honoured keys with a comment. Idempotent: export → verify exits 0.
- **F3 Verify.** `gcfg verify [--only fam,…] [--json|--markdown] [--strict]`. Exit
  0 clean, 1 drift (or unmanaged under `full`), 2 a family could not be read (auth /
  network) — with `--strict`, unreadable is also 1. Markdown output is the step
  summary shape.
- **F4 Plan & apply.** `gcfg plan` prints the change set without writing. `gcfg apply
  [--only] [--yes] [--dry-run]`: TTY confirmation unless `--yes`; non-TTY without
  `--yes` exits 2 with the plan printed. Creates/updates always; deletes only under
  `full`. After writes: re-read verify; any remaining finding → exit 1 naming keys;
  `not_honoured` findings say so explicitly. `--json` apply log includes the pre-image
  of every deleted object.
- **F5 Init & templates.** `gcfg init [--from owner/repo] [--force]`: smart default
  or a copied file (API fetch of `.github/gcfg.yaml`, else export of the source's live
  settings). Never overwrites without `--force`.
- **F6 TUI.** Vim keys (`j k h l gg G ctrl-d ctrl-u`), `/` incremental regex search
  with smartcase + `n/N`, `?` help overlay, `e`/`enter` typed editors (bool toggle,
  enum picker, string, string-list), `u` undo last edit, `w` write YAML (comments
  preserved via yaml.v3 nodes), `v` verify (status colors: ok / drift / unmanaged /
  unreadable), `a` plan + confirm + apply, `q` quit (prompt if unsaved). Colors from a
  shared style package; `NO_COLOR` honoured; min terminal size message.
- **F7 Actions install.** `gcfg actions install verify|apply|both [--version vX]
  [--auth app|pat]` renders `.github/workflows/gcfg-verify.yml` / `gcfg-apply.yml`;
  `uninstall` removes them. Verify: `pull_request`, `schedule` (daily), `workflow_dispatch`;
  writes `$GITHUB_STEP_SUMMARY`; fails on drift. Apply: `push` to default branch on
  `.github/gcfg.yaml`, `workflow_dispatch`; `concurrency: gcfg-apply`; runs `apply
  --yes --json`, then verify. Both pin the gcfg release by version + checksum and take
  `GH_TOKEN` from `create-github-app-token` (App) or the `GCFG_TOKEN` secret (PAT).
- **F8 Auth.** Resolution order `GH_TOKEN` → `GITHUB_TOKEN` → `gh` login → ghapp
  token (`--auth app`). `auth status` prints the source (never the token). `auth
  doctor` probes each family and prints read/write per family plus the missing scope
  or permission name. `auth pat` prints the fine-grained permission checklist for the
  target and validates a token pasted on stdin. `auth app create|install|token|status`
  wrap ghapp for the current target.
- **F9 Ownership.** `declared` (default): undeclared live items → `unmanaged`
  (informational, listed, exit 0). `full`: undeclared → `drift`, `apply` deletes.
  Per-family override. Verify output always states the effective mode per family.
- **F10 Families v1** (repo): general, security, rulesets, actions, labels,
  autolinks, environments, secrets (names), webhooks (no secret), collaborators
  (report; write under `full`), pages (report). Org: profile, members,
  security_defaults, actions, rulesets, apps (report). Each family is independently
  readable; one failing family never blocks the others' report.
- **F11 ghapp module.** `ghapp create` (manifest flow, localhost callback, browser
  open, PEM 0600), `install`, `token --repo|--org [--permissions]` (cached until
  expiry), `status`, `doctor`. Library API for gcfg. Docs: how someone else creates
  their own App from the same manifest.
- **F12 Dotfiles adoption.** `install.sdk.gcfg` gff flag; make targets; this repo's
  `.github/gcfg.yaml` exported and both workflows installed; `github_secret_scanning.sh`
  and `ruleset_snapshot.sh` retired after `make gcfg-verify` is green in CI.

## 5. Evaluation criteria (per feature)

| Feature | Fires | Must not fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- |
| F1 lint | unknown key (path named); org block outside `.github`; dup names; secret-shaped value | valid file; org block in `<org>/.github` | empty file → defaults with warning; `version` missing → error | table tests + schema golden |
| F2 export | writes every family present live; imports rulesets JSON; not-honoured comment | writing secret values; writing when `--out` exists without `--force` | family unreadable → key omitted + WARN, exit 0 | fake-client fixtures + golden YAML; export→verify 0 |
| F3 verify | drift → exit 1 with key/want/live; unreadable → 2 | clean → 0, no findings | `--strict` turns unreadable into 1; `--only` limits families | engine table tests; markdown golden |
| F4 apply | plan shows create/update/delete; non-TTY w/o `--yes` → 2 no writes; re-read failure → 1 naming key | deletes under `declared`; writes with `--dry-run` | not-honoured key → `not_honoured` finding, others still applied | fake client records call order; pre-image in json log |
| F5 init | default file lints clean; `--from` copies; existing file → refuse w/o `--force` | overwrite | source has no file → export path used | golden default; fake fetch |
| F6 TUI | `/` jumps; `e` edits; `w` writes only changed keys, comments kept; `v` colors | writes on navigate/quit; edits without `w` persisting | tiny terminal; `NO_COLOR` | teatest golden frames + yaml round-trip test |
| F7 actions | files render with pinned version + checksum; actionlint clean; App vs PAT branches | rendering when files exist w/o `--force` | `both` renders two; `uninstall` removes only ours | golden workflows + actionlint in CI |
| F8 auth | doctor reports per-family read/write; names missing scope/permission; status never prints token | token in any output | no credential → exit 2 with the 4-step resolution list | fake probe matrix; output grep for token |
| F9 ownership | `full`: extra label → drift + delete in plan; `declared`: extra → unmanaged, exit 0 | deletes under declared | per-family override wins over top-level | engine matrix |
| F10 families | each family Read/Diff/Apply round-trips a fixture; one 403 family leaves others reported | fatal on one family | pagination (labels > 100) | per-family table tests with fixtures |
| F11 ghapp | manifest exchange → pem/app id stored 0600; token minted + cached; expiry refresh | PEM in logs/stdout | callback port busy → next port; conversion code expired → clear error | httptest server tests; file-mode test |
| F12 adoption | `make gcfg-verify` green in CI on this repo; scripts removed only after | removing scripts before verify covers them | non-provider key present as not-honoured | CI run + live evidence |

## 6. Verification harness

- Go: table-driven unit tests per package with the recording fake client (fixtures
  under `testdata/`); **≥80% overall**, ≥90% `internal/engine` and `internal/schema`;
  `go vet`; `gcfg-ci.yml` (unit + coverage gate + schema-drift check + actionlint on
  rendered workflows) and `ghapp-ci.yml`.
- Golden files: default init, export of the fixture repo, findings json/markdown,
  rendered workflows, JSON Schema, TUI frames (teatest).
- Binary-level e2e (`scripts/e2e.sh`): compiled binary + fake HOME + an httptest
  GitHub stub serving fixtures: init → lint → verify (0) → mutate stub → verify (1) →
  apply --yes → verify (0); apply without `--yes` non-TTY → 2.
- Shell edits (install.sh, make targets): `make lint-shell` + `make lint-portability`.
- Human-evidenced gates (recorded in `plans/gcfg/evidence/`): live export of this
  repo; UI-induced drift caught; apply + re-read; auth doctor for gh token and App
  token; verify workflow red→green; apply workflow log; ghapp create once (redacted).

## 7. Prerequisites / dependencies

- Go toolchain from `.go-version`; cobra, bubbletea/lipgloss/bubbles, yaml.v3,
  `github.com/cli/go-gh/v2` (token + REST), `github.com/invopop/jsonschema`
  (schema gen), `github.com/golang-jwt/jwt/v5` (ghapp RS256).
- A `gh` login locally (already present) or a token in env. For orgs: `admin:org`
  scope on the `gh` token, or an App with organization Administration.
- CI: `actionlint` available in the lint job (already used by the repo's lint).
- `.gitignore`: `!sdk/**` covers both modules; `!.github/**` covers
  `.github/gcfg.yaml`, `gcfg.schema.json`, and the workflows.

## 8. Out of scope (and why)

- Multi-repo/central admin model — safe-settings exists for that; gcfg is per-repo.
- Teams/membership, billing, Pages content, Discussions — not settings drift we suffer.
- Managing app installations on personal accounts — no API without app auth.
- Secret values — by design never touched (names only).
- A hosted webhook service — Actions cover the automation we need.
- Terraform export — possible later; not needed to reach the goal.

## 9. Rollback

`gff set install.sdk.gcfg false`; remove binaries; `gcfg actions uninstall`;
`gcfg export --out snapshot.yaml` before first apply and `gcfg apply -f snapshot.yaml`
to restore; deletions recoverable from the `--json` apply log pre-images.

> Produced via `superpowers:brainstorming` under the `mbo-plan` pipeline. Plan: `../plans/gcfg.md`.
