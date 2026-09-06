# gcfg — GitHub config as code — implementation plan

- **Slug:** gcfg
- **Date:** 2026-09-05
- **Status:** Draft
- **Relates to:** spec `../specs/gcfg.md` · design `../designs/gcfg.md` · issue [#284](https://github.com/sfc-gh-eraigosa/dotfiles/issues/284) · execution trio [`gcfg/`](./gcfg/)

## Global constraints

- Two modules: `github.com/sfc-gh-eraigosa/dotfiles/sdk/ghapp` and
  `github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg`; go directive from `.go-version`.
  Layout mirrors `sdk/gss`/`sdk/gff`: `cmd/`, `internal/`, `main.go`, `build.sh` sourcing
  `sdk/version.sh` (tag-driven version via ldflags; no `VERSION` file), `libs/log`.
- **Coverage bars:** ≥80% overall per module (CI-enforced), ≥90% `sdk/gcfg/internal/engine`
  and `sdk/gcfg/internal/schema`. No network in tests: every GitHub call goes through the
  `gh.Client` seam; tests use the recording fake + `testdata/` fixtures, or `httptest`.
- **Never print, store, or export a token or secret value.** Tests grep outputs for the
  fixture token string and fail on a hit.
- Writes to the user's machine: `~/.config/ghapp/` (0700; PEM 0600) and
  `~/.config/gcfg/` (cache). Repo writes only via explicit verbs (`export`, `init`,
  `actions install`, TUI `w`) and only under `.github/`.
- Exit codes (all verbs): 0 ok/clean · 1 drift or apply-left-findings · 2 usage, no
  credential, unreadable family (verify), or non-TTY apply without `--yes`.
- Shell edits obey `docs/mbo/specs/shell-portability.md`; `make lint-shell` +
  `make lint-portability` before each shell commit; all gff gates fail open.
- Per-directory docs rule: `sdk/gcfg/AGENTS.md`, `sdk/ghapp/AGENTS.md`, each with
  `CLAUDE.md → AGENTS.md`; rows in `sdk/AGENTS.md` + `sdk/README.md`.
- Commits end with the session's attribution trailers; stage files by explicit name;
  confirm via the interactive prompt before every commit/checkpoint.

## 1. Summary & verdict

Build `gcfg` (settings-as-code CLI + TUI + Actions) and `ghapp` (GitHub App
credential toolkit) as designed. Verdict from the design review: **build**, in six
phases, with the credential toolkit first (everything else needs a token to prove
itself live), the engine + two families next (the vertical slice that makes UC1–UC3
real on this repo), then breadth (families), automation (Actions), the TUI, and
adoption (this repo's own file, retiring two scripts).

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/ghapp/{go.mod,main.go,build.sh,AGENTS.md,CLAUDE.md,README.md}` | module scaffold | F11 |
| `sdk/ghapp/pkg/ghapp/{manifest.go,store.go,jwt.go,token.go,installs.go}` + `_test.go` | manifest flow, PEM/app store, RS256 JWT, installation tokens (cached), installations | F11 |
| `sdk/ghapp/cmd/{root,create,install,token,status,doctor}.go` | CLI | F11 |
| `sdk/gcfg/{go.mod,main.go,build.sh,AGENTS.md,CLAUDE.md,README.md}` | module scaffold | — |
| `sdk/gcfg/internal/schema/{types.go,load.go,lint.go,jsonschema.go}` + tests + `testdata/` | typed config, strict load, lint, JSON Schema | F1, F9 |
| `sdk/gcfg/internal/gh/{client.go,real.go,fake.go,auth.go}` + tests | `Client` seam, go-gh REST impl, recording fake, credential chain | F8 |
| `sdk/gcfg/internal/family/family.go` | `Family` interface, registry, `Finding` | F10 |
| `sdk/gcfg/internal/family/<name>/` ×11 repo + ×6 org (+ `testdata/`) | one package per family | F10 |
| `sdk/gcfg/internal/engine/{export,verify,plan,apply,ownership}.go` + tests | verbs over families; re-read verify; ownership | F2–F4, F9 |
| `sdk/gcfg/internal/report/{finding.go,tty.go,json.go,markdown.go}` + goldens | renderers | F3 |
| `sdk/gcfg/internal/tui/{model,view,keys,search,edit,write}.go` + teatest goldens | TUI | F6 |
| `sdk/gcfg/internal/actions/{render.go,templates/gcfg-verify.yml.tmpl,gcfg-apply.yml.tmpl}` + goldens | workflow install/uninstall | F7 |
| `sdk/gcfg/internal/style/style.go` | lipgloss tokens (gff shape) | F6 |
| `sdk/gcfg/cmd/{root,init,export,lint,schema,verify,plan,apply,tui,actions,auth,version}.go` + tests | cobra verbs | all |
| `sdk/gcfg/scripts/e2e.sh` + `e2e/` | binary-level e2e with httptest GitHub stub | §7 |
| `.github/workflows/{gcfg-ci.yml,ghapp-ci.yml}` | vet, tests, coverage gates, schema drift, actionlint | §7 |
| `.github/gcfg.yaml`, `.github/gcfg.schema.json` | this repo's declared settings + published schema | F12, F1 |
| `.github/workflows/{gcfg-verify.yml,gcfg-apply.yml}` | installed by `gcfg actions install both` | F7, F12 |
| `Makefile` | `gcfg-build/test/install/e2e/verify/apply`, `ghapp-build/test/install`, `gcfg-schema-check` | F12 |
| `install.sh`, `.github/gff/features.yaml` | `install.sdk.gcfg` block + flag (builds both binaries) | F12 |
| `sdk/AGENTS.md`, `sdk/README.md`, `docs/mbo/index.md` | module rows, tour sections, objective row | docs |
| `opt/scripts/git/{github_secret_scanning.sh,ruleset_snapshot.sh}` (+tests, Makefile targets, AGENTS bullets), `.github/rulesets/main.json` | **removed** in P5 after `make gcfg-verify` is green in CI | F12 |

## 3. Interface contracts (frozen)

### 3.1 `gcfg.yaml` (v1) — shape

```yaml
version: 1
ownership: declared            # declared | full (per-family override allowed)
repo:
  general:
    description: "…"
    homepage: "…"
    topics: [dotfiles, shell]
    visibility: public          # public | private | internal
    default_branch: main
    features: {issues: true, projects: true, wiki: false, discussions: false}
    merge:
      squash: true
      merge_commit: false
      rebase: false
      auto_merge: true
      delete_branch_on_merge: true
      allow_update_branch: true
      squash_title: COMMIT_OR_PR_TITLE      # PR_TITLE | COMMIT_OR_PR_TITLE
      squash_message: COMMIT_MESSAGES       # PR_BODY | COMMIT_MESSAGES | BLANK
    web_commit_signoff_required: false
    allow_forking: true
  security:
    secret_scanning: true
    push_protection: true
    non_provider_patterns: true            # exported with "# not honoured on this plan" when observed
    dependabot_alerts: true
    dependabot_security_updates: true
    private_vulnerability_reporting: false
    code_scanning_default_setup: not-configured   # not-configured | configured
  actions:
    enabled: true
    allowed_actions: all                   # all | local_only | selected
    sha_pinning_required: false
    default_workflow_permissions: read     # read | write
    can_approve_pull_request_reviews: false
  rulesets:
    - name: main
      target: branch
      enforcement: active
      conditions: {ref_name: {include: ["~DEFAULT_BRANCH"], exclude: []}}
      bypass_actors: []
      rules: [ {type: pull_request, parameters: {required_approving_review_count: 0, dismiss_stale_reviews_on_push: true}}, {type: non_fast_forward}, {type: deletion} ]
  labels:
    ownership: full
    items: [ {name: bug, color: d73a4a, description: "Something isn't working"} ]
  autolinks: [ {key_prefix: "JIRA-", url_template: "https://…/<num>", is_alphanumeric: false} ]
  environments: [ {name: production, wait_timer: 0, reviewers: [], deployment_branch_policy: protected_branches} ]
  secrets: {names: [GCFG_TOKEN]}            # presence only — never values
  webhooks: [ {url: "https://…", events: [push], active: true, content_type: json} ]   # no secret
  collaborators: [ {login: someone, permission: push} ]                               # write only under full
  pages: {enabled: false}                    # report-only in v1
org:                                         # lint: only in <org>/.github
  profile: {description: "…", blog: "…", location: "…"}
  members: {default_repository_permission: none, members_can_create_repositories: false, two_factor_required: true}
  security_defaults: {secret_scanning_new_repos: true, push_protection_new_repos: true, dependabot_alerts_new_repos: true}
  actions: {allowed_actions: selected, default_workflow_permissions: read}
  rulesets: [ … same shape … ]
  apps: [ {slug: mergify, repository_selection: all} ]   # report-only
```

Rules: unknown keys → error with path; every key optional (declared-only management);
`ownership` at top or per family; `items:` form is used when a family needs its own
`ownership`. JSON Schema is generated from the structs and committed at
`.github/gcfg.schema.json`; `gcfg-ci` fails if regeneration drifts.

### 3.2 Go interfaces

```go
// internal/gh
type Client interface {
    Do(ctx context.Context, method, path string, body any, out any) (status int, err error)
    Paginate(ctx context.Context, path string, each func(json.RawMessage) error) error
}
type Source int // SourceEnv, SourceGHToken, SourceGHLogin, SourceApp, SourceNone
func Resolve(ctx context.Context, opts AuthOpts) (Client, Source, error) // GH_TOKEN → GITHUB_TOKEN → gh login → ghapp

// internal/family
type Target struct{ Owner, Repo, Org string }            // Repo=="" ⇒ org target
type Kind int // Drift, Unmanaged, Unreadable, NotHonoured
type Finding struct{ Family, Key, Op string; Kind Kind; Want, Live any; Reason string }
type Change struct{ Family, Key, Op string; Want, Live any; PreImage json.RawMessage } // Op: update|create|delete
type Family interface {
    Name() string
    Scope() Scope                                        // ScopeRepo | ScopeOrg
    Permission() string                                  // e.g. "repo:Administration:write"
    Read(ctx context.Context, c gh.Client, t Target) (Live, error)
    Export(live Live) (node *yaml.Node, err error)
    Diff(desired *yaml.Node, live Live, own Ownership) ([]Finding, []Change)
    Apply(ctx context.Context, c gh.Client, t Target, changes []Change) error
}
func Register(f Family); func All(scope Scope) []Family

// internal/engine
func Export(ctx, c, t, opts) (*schema.File, []Finding, error)   // Unreadable findings, never fatal
func Verify(ctx, c, t, f *schema.File, opts) (Report, error)    // exit mapping in cmd
func Plan(ctx, c, t, f *schema.File, opts) ([]Change, Report, error)
func Apply(ctx, c, t, f *schema.File, plan []Change, opts) (Report, error) // re-reads; Report must be clean

// sdk/ghapp/pkg/ghapp
type App struct{ ID int64; Slug string; PEMPath string; Installations map[string]int64 }
func Create(ctx, manifest Manifest, opts CreateOpts) (App, error)          // manifest flow
func (a App) Token(ctx, inst int64, scope TokenScope) (Token, error)        // cached until expiry-2m
func (a App) Installations(ctx) ([]Installation, error)
type Store interface{ Load() (Apps, error); Save(Apps) error }              // ~/.config/ghapp (0700/0600)
```

### 3.3 CLI contract

```
gcfg init [--from owner/repo] [--force] [-f path]
gcfg export [--org] [--out path] [--force]
gcfg lint [-f path]                 | gcfg schema [--out path]
gcfg verify [--only a,b] [--json|--markdown] [--strict] [--org]
gcfg plan   [--only a,b] [--json] [--org]
gcfg apply  [--only a,b] [--yes] [--dry-run] [--json] [--org] [-f path]
gcfg tui    [-f path]
gcfg actions install verify|apply|both [--version vX.Y.Z] [--auth app|pat] [--force]
gcfg actions uninstall
gcfg auth status | doctor [--org] | pat [--check] | app create|install|token|status
gcfg version
ghapp create [--name] [--manifest file] [--org] · install · token --repo o/r|--org o [--permissions k=v] · status · doctor
```

Common flags: `-R owner/repo` (default: origin of CWD via go-gh), `--auth
env|gh|app|auto`, `--no-color`. All `--json` outputs are stable, documented shapes.

### 3.4 Workflow templates (rendered by `actions install`)

- `gcfg-verify.yml`: `on: pull_request, schedule (cron daily), workflow_dispatch`;
  `permissions: contents: read`; steps: checkout (pinned SHA) → token (either
  `actions/create-github-app-token@<pinned>` with `app-id`/`private-key` from
  `secrets.GCFG_APP_ID`/`secrets.GCFG_APP_PRIVATE_KEY`, or `GH_TOKEN: ${{ secrets.GCFG_TOKEN }}`)
  → download gcfg release `vX.Y.Z` + sha256 check → `gcfg verify --markdown >> $GITHUB_STEP_SUMMARY`.
- `gcfg-apply.yml`: `on: push (branches: [default], paths: [.github/gcfg.yaml]),
  workflow_dispatch`; `concurrency: gcfg-apply`; same token step; `gcfg apply --yes --json
  | tee apply.json` (uploaded as artifact) → `gcfg verify --markdown >> $GITHUB_STEP_SUMMARY`.

### 3.5 Exit codes & output

0 clean/ok · 1 drift / apply left findings · 2 usage, no credential, unreadable (verify
without `--strict` still 2 for *all* families unreadable; a single unreadable family is a
finding + exit 0 unless `--strict`), non-TTY apply without `--yes`. `--markdown` emits a
table `| family | key | want | live | kind |` under a one-line headline with counts.

## 4. TDD build order

Each task: tests first · verify RED · implement · verify GREEN · gate · evidence
(`tee`'d into `plans/gcfg/evidence/<task>/`) · commit.

### P0 — ghapp (credential toolkit)

**P0-T1 scaffold + version.** `sdk/ghapp` module, `build.sh`, `cmd/version`,
`ghapp-ci.yml` (vet, test, ≥80%). Done-when: `go run . version` prints a tag-derived
version; CI green. Commit: `feat(ghapp): module scaffold + version`.

**P0-T2 store + JWT.** RED: `store_test` (0700 dir, 0600 PEM, round-trip, refuses a
world-readable PEM), `jwt_test` (RS256 header/claims iat-60s/exp+9m, iss=app id;
verify with the public key). GREEN. Commit: `feat(ghapp): app store + RS256 JWT`.

**P0-T3 installation tokens.** RED: httptest GitHub stub for `GET /app/installations`
and `POST /app/installations/{id}/access_tokens`; cache hit before expiry-2m, miss
after; permissions/repositories scoping in body; token never logged (grep stub). GREEN.
Commit: `feat(ghapp): installation tokens with cache`.

**P0-T4 manifest flow.** RED: `Create` starts a localhost listener (port fallback),
builds the manifest form (hook url optional, permissions map, redirect url), exchanges
code at `POST /app-manifests/{code}/conversions` (stub), persists PEM + id; expired
code → clear error; browser opener injected. GREEN. `create/install/token/status/doctor`
verbs. Commit: `feat(ghapp): manifest-flow create + CLI`.
Human evidence: one real `ghapp create` (redacted) + `ghapp token --repo` → `evidence/ghapp/`.

### P1 — gcfg core (vertical slice: schema → client → engine → 2 families → verbs)

**P1-T1 scaffold + version + CI.** As P0-T1 for `sdk/gcfg`; `gcfg-ci.yml` with the
80/90/90 gates and a schema-drift check placeholder.

**P1-T2 schema load + lint + JSON Schema.** RED: `load_test` (strict unknown-key error
with path; every key optional; per-family ownership), `lint_test` (org block outside
`.github` repo; dup names; enum values; secret-shaped value in any string field),
`jsonschema_test` (golden). GREEN. `gcfg lint`, `gcfg schema`. Commit:
`feat(gcfg): typed schema, strict load, lint, JSON Schema`.

**P1-T3 gh client + credential chain.** RED: `fake` records calls + serves fixtures;
`Resolve` order test with env/gh-config/ghapp seams; `auth status` never prints token
(grep). GREEN over `go-gh/v2` REST (retry on 5xx, honour `Retry-After`). Commit:
`feat(gcfg): gh client seam, recording fake, credential chain`.

**P1-T4 family interface + `general` + `security`.** RED per family: `Read` from
fixture JSON → `Live`; `Export` golden YAML; `Diff` matrix (declared vs full; missing
key; not-honoured `non_provider_patterns` when live says disabled after apply); `Apply`
records the exact PATCH body. GREEN. Commit: `feat(gcfg): family model + general + security`.

**P1-T5 engine verify/export/plan/apply + report.** RED: engine table tests (clean→0
findings; drift; unreadable family → finding not error; `full` extras; apply →
re-read → not-honoured finding survives; call order recorded), report goldens (tty,
json, markdown). GREEN. Commit: `feat(gcfg): engine + renderers`.

**P1-T6 verbs: export/verify/plan/apply/init.** RED `cmd/*_test` (exit codes; non-TTY
apply without `--yes` → 2 and zero writes; `--only`; `init` default golden + `--from`
via fake fetch; refuse overwrite). GREEN. Commit: `feat(gcfg): export/verify/plan/apply/init`.
**Live evidence (UC1–UC3 on this repo, `gh` token):** export → verify 0; UI-flipped
`delete_branch_on_merge` → verify 1 naming the key; apply → re-read 0 → `evidence/core/`.

### P2 — families (parallelizable leaves)

One task per family, identical shape to P1-T4 (fixtures, export golden, diff matrix,
apply body). Repo: **P2-T1 rulesets** (+ import of `.github/rulesets/*.json` on export;
update-by-name; delete under `full`), **P2-T2 actions**, **P2-T3 labels** (pagination
>100; `full` deletes), **P2-T4 autolinks**, **P2-T5 environments**, **P2-T6 secrets
(names)** + **webhooks (no secret)**, **P2-T7 collaborators (report; write under
full) + pages (report)**. Org: **P2-T8 profile + members + security_defaults**,
**P2-T9 org actions + org rulesets**, **P2-T10 apps (report)**. Commit per task:
`feat(gcfg): <family> family`.

### P3 — auth doctor + Actions install

**P3-T1 auth doctor + pat.** RED: probe matrix over fake (per family read/write; 403
→ missing permission name from `Family.Permission()`; org scope message); `pat --check`
validates a stdin token via probes; nothing echoes the token. GREEN. Commit:
`feat(gcfg): auth doctor + pat checklist`.
**P3-T2 auth app wrappers.** Thin verbs over `pkg/ghapp` bound to the current target.
**P3-T3 actions install/uninstall.** RED: goldens for both templates × {app, pat};
pinned version + checksum line; refuse overwrite; uninstall removes only ours;
`actionlint` on goldens in CI. GREEN. Commit: `feat(gcfg): actions install verify|apply`.
**P3-T4 e2e harness.** `scripts/e2e.sh`: compiled binary + fake HOME + httptest stub:
init → lint → verify 0 → mutate stub → verify 1 → apply --yes → verify 0; non-TTY apply
→ 2. Commit: `test(gcfg): binary-level e2e`.

### P4 — TUI

**P4-T1 model + navigation + search.** RED teatest: tree render, `j/k/gg/G`, fold
`h/l`, `/` incremental smartcase regex + `n/N`, `?` overlay, tiny-terminal message.
**P4-T2 editors + write-back.** RED: bool toggle, enum picker, string/list editors,
`u` undo, `w` writes only changed keys with comments preserved (yaml.v3 node
round-trip test), `q` with unsaved → prompt, quit without `w` → file byte-identical.
**P4-T3 verify/apply in TUI.** RED: `v` colors rows from findings; `a` shows plan,
confirm, applies via engine (fake), re-verify recolors. Commit per task:
`feat(gcfg): tui <part>`.

### P5 — adoption in this repo

**P5-T1 wiring.** `install.sdk.gcfg` flag + install.sh block (builds gcfg + ghapp),
Makefile targets, `sdk/AGENTS.md` + `sdk/README.md` rows/sections, module
`AGENTS.md`/`CLAUDE.md`/`README.md`. `make lint-shell lint-portability`.
**P5-T2 this repo's file.** `gcfg export` → `.github/gcfg.yaml` (reviewed; rulesets
imported from `.github/rulesets/main.json`); `gcfg schema` → `.github/gcfg.schema.json`;
`make gcfg-verify` in CI (uses a `GCFG_TOKEN`/App secret; the job is skipped with a
notice when the secret is absent, e.g. fork PRs).
**P5-T3 workflows.** `gcfg actions install both`; first verify run green; drift PR
red→green; apply run log → `evidence/actions/`.
**P5-T4 retire the one-offs.** Remove `github_secret_scanning.sh`, `ruleset_snapshot.sh`,
their tests, Makefile targets, AGENTS bullets, and `.github/rulesets/main.json` —
**only after** P5-T2's CI job is green. Commit: `chore(git): retire secret-scanning +
ruleset snapshot scripts (superseded by gcfg)`.

## 5. Verification mapping (spec §5 → test)

| Spec rule | Test |
| :-- | :-- |
| F1 lint / schema | `internal/schema/{load,lint,jsonschema}_test.go`; `gcfg-ci` schema-drift step |
| F2 export idempotent; rulesets import; not-honoured comment | `engine/export_test.go`; `family/rulesets/import_test.go`; `cmd/export_test.go` golden |
| F3 verify exit codes / markdown | `engine/verify_test.go`; `report/markdown_test.go` golden; `cmd/verify_test.go` |
| F4 apply plan/confirm/re-read/pre-image | `engine/apply_test.go` (call order, pre-image); `cmd/apply_test.go` (non-TTY → 2) |
| F5 init default / --from / no overwrite | `cmd/init_test.go` + `testdata/init.golden` |
| F6 TUI | `internal/tui/*_test.go` teatest goldens + `write_test.go` round-trip |
| F7 actions render / actionlint | `internal/actions/render_test.go` goldens; `gcfg-ci` actionlint step |
| F8 auth chain / doctor / no token leak | `gh/auth_test.go`; `cmd/auth_test.go` matrix; token-grep helper in every cmd test |
| F9 ownership matrix | `engine/ownership_test.go` |
| F10 each family | `family/<name>/<name>_test.go` (fixture round-trip) |
| F11 ghapp | `pkg/ghapp/*_test.go` (httptest, file modes, cache) |
| F12 adoption | `gcfg-verify` CI job on this repo; `evidence/adoption/` |
| UC1–UC3, UC6–UC7 live | human-evidenced captures under `plans/gcfg/evidence/` |

## 6. Integration & rollout

- Build/test discovery: `gcfg-ci.yml`, `ghapp-ci.yml` (label-event guard like the other
  workflows; required checks added to mergify config only after the first green run).
- Docs: module `AGENTS.md`/`README.md`; `sdk/AGENTS.md` + `sdk/README.md`; `docs/mbo/index.md`.
- Rollout order = phases; this repo is the reference deployment (P5). Other repos adopt
  with `gcfg init --from sfc-gh-eraigosa/dotfiles` then `gcfg actions install both`.
- Manual acceptance checklist = spec §6 human gates.

### 6.1 Build leaves / DAG (authoritative — mirrored to the design issue and gss bases)

| Leaf | Owns (paths) | Consumes (← edge) | done-when gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| `ghapp` | `sdk/ghapp/**`, `.github/workflows/ghapp-ci.yml` | — | ghapp-ci green ≥80%; live token mint evidence | yes (for `auth-app`, `adoption`) |
| `core` | `sdk/gcfg/{go.mod,main.go,build.sh,cmd/{root,version,lint,schema,export,verify,plan,apply,init}.go,internal/{schema,gh,engine,report,family/family.go,family/general,family/security}}`, `gcfg-ci.yml` | — | gcfg-ci green 80/90/90; UC1–UC3 live evidence | yes (base for every gcfg leaf) |
| `fam-repo-a` | `internal/family/{rulesets,actions,labels}` | core (§3.2 Family) | package tests + fixtures; export golden | no |
| `fam-repo-b` | `internal/family/{autolinks,environments,secrets,webhooks,collaborators,pages}` | core | same | no |
| `fam-org` | `internal/family/org*` (`profile,members,security_defaults,actions,rulesets,apps`) | core | same + org lint case | no |
| `auth` | `cmd/auth*.go`, `internal/gh/doctor.go` | core, ghapp | doctor matrix; no-token-leak grep | no |
| `actions` | `internal/actions/**`, `cmd/actions.go`, `scripts/e2e.sh`, `e2e/` | core | goldens + actionlint; e2e green | no |
| `tui` | `internal/tui/**`, `internal/style/**`, `cmd/tui.go` | core | teatest goldens; round-trip | no |
| `adoption` | `install.sh`, `Makefile`, `.github/gff/features.yaml`, `.github/gcfg.yaml`, `.github/gcfg.schema.json`, `.github/workflows/gcfg-{verify,apply}.yml`, `sdk/{AGENTS,README}.md`, removals in P5-T4 | every leaf above | `make gcfg-verify` green in CI; workflows red→green evidence | no (last) |

Edges: `core → {fam-repo-a, fam-repo-b, fam-org, auth, actions, tui}`; `ghapp → auth`;
all → `adoption`. Blocking-first order: `ghapp` ∥ `core`, then the six leaves in
parallel, then `adoption`.

## 7. Validation & evidence (show the work)

- Coverage: `gcfg-ci`/`ghapp-ci` fail under 80% module-wide; `internal/engine` and
  `internal/schema` under 90% (per-package `go test -cover` grep, as gff does).
- E2E: `scripts/e2e.sh` happy path + adversarial (non-TTY apply, expired App code,
  token never in output, unreadable family, `full` deletes with pre-image).
- Demo: a scripted transcript `evidence/demo/` running init → export → drift → verify →
  apply → verify on the fixture stub; and the live evidence list from design §7.
- **Evidence protocol:** `plans/gcfg/evidence/<phase-task>/` — every done-when command
  `tee`'d with a dated header, append-only, committed with the task. A feature without
  captured evidence is not done.

> Produced via `superpowers:writing-plans` under the `mbo-plan` pipeline. Execute with
> the trio in [`gcfg/`](./gcfg/) (IMPLEMENTATION · TRACKING · TODO), TDD throughout.
