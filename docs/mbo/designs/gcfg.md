# gcfg — GitHub config as code — design

- **Slug:** gcfg
- **Date:** 2026-09-05
- **Status:** Draft
- **Relates to:** issue [#284](https://github.com/sfc-gh-eraigosa/dotfiles/issues/284) / design draft PR (this PR) / spec `../specs/gcfg.md` / plan `../plans/gcfg.md`
- **Author(s):** repo owner (`sfc-gh-eraigosa`) with Claude

## 1. Problem / context

Every GitHub repository carries a long tail of settings that live **only in GitHub's
database**: merge strategies, branch rulesets, secret scanning and push protection,
Actions permissions, labels, autolinks, topics, environments, installed apps. None of
it has a file representation in the repo, so none of it is reviewed, versioned, or
reproducible.

Verified facts that motivated this objective (all observed on `sfc-gh-eraigosa/dotfiles`
on 2026-09-05):

- PR #275 enabled secret scanning + push protection with a one-off script
  (`opt/scripts/git/github_secret_scanning.sh`). It handles exactly one setting family.
  The next family (rulesets) already has its own one-off: `ruleset_snapshot.sh` writes
  `.github/rulesets/main.json` as a **documentation** snapshot GitHub never reads back.
  Two settings, two bespoke scripts, zero enforcement.
- Enabling `secret_scanning_non_provider_patterns` returned HTTP 200 and left the
  setting `disabled` (it needs GitHub Secret Protection). A setting can *look* applied
  and not be; only a read-back verify catches that.
- The repo's live state today: rulesets 1 (`main`, active), branch protection none,
  Actions `allowed_actions: all`, default workflow permissions `write`,
  `can_approve_pull_request_reviews: true`, 30 labels, 6 topics, 1 collaborator,
  code-scanning default setup `not-configured`. None of it is declared anywhere.
- Credentials: the user's `gh` token (`repo`, `read:org`, `gist`, `admin:public_key`)
  reads and writes every repo-level setting family above. Org-level Actions
  permissions and org rulesets refused it (`admin:org` scope needed). A non-admin
  token (probed against `cli/cli`) cannot read `security_and_analysis`, merge
  settings, or Actions permissions at all — verification itself needs admin rights.
- In GitHub Actions, `GITHUB_TOKEN` has **no** `administration` permission (the
  `permissions:` keys are actions, attestations, checks, contents, deployments,
  discussions, id-token, issues, packages, pages, pull-requests, security-events,
  statuses, vulnerability-alerts, …). A workflow that verifies or applies settings
  must bring its own credential: a fine-grained PAT or a GitHub App installation
  token. Per GitHub's fine-grained-permission reference, the repo endpoints we need
  are all under repository **Administration** (read for GET Actions permissions,
  write for everything else) and the org endpoints under organization
  **Administration** (`GET /orgs/{org}/installations` is read).
- Prior art: `github/safe-settings` is org-scoped, requires a hosted Probot app and a
  central admin repo ("settings files cannot be in individual repositories"). The
  older Probot `settings` app reads a per-repo `.github/settings.yml` but applies on
  push with no verify mode and covers a fixed subset. Terraform's github provider
  models everything but drags in state files and HCL for what is a per-repo YAML
  problem. None serve a personal account with a CLI + TUI and no hosted service.

## 2. Goals & non-goals

**Goals**

- G1 One file, `.github/gcfg.yaml`, declares a repo's GitHub settings (and, in an
  org's `.github` repo only, that org's settings). Each repo carries its own file.
- G2 `verify` reports drift between the file and live GitHub, machine-readably, with
  exit codes usable as a CI gate. `apply` converges live GitHub to the file, always
  showing the plan first, and **re-reads to prove** each change stuck.
- G3 `export` generates the file from an existing repo/org so adoption is one command;
  `init` seeds a smart default or copies another repo's file as a template.
- G4 A TUI in the house style (gff/fleet): vim navigation, `/` search, edit values,
  write back to the YAML, run verify/apply from inside.
- G5 Two installable Actions workflows: verify (on PR + schedule) and apply (on push
  to default branch, manual dispatch), with the same binary and the same output.
- G6 Credentials are a first-class feature: `gcfg auth` diagnoses what a token can do
  and helps set up either a fine-grained PAT or a GitHub App (created via the App
  Manifest flow, private key managed locally, installation tokens minted on demand).
  The App machinery is a **separate reusable module** (`sdk/ghapp`) so other tools in
  this repo can mint admin tokens the same way.
- G7 Ownership is a toggle: `declared` (default: only declared keys are managed;
  extras are reported as unmanaged) or `full` (the file is the whole truth; extras
  are drift and `apply` removes them).
- G8 Never handle secret *values*: Actions secrets, webhook secrets, and deploy keys
  are managed by **name/presence** only. Nothing gcfg writes to the repo can leak a
  credential; the privacy guard and gitleaks judge its output like any other file.

**Non-goals (v1)**

- Multi-repo fan-out from one file (safe-settings' model). One repo, one org, one file.
- Team/membership management, billing, GitHub Pages content, Discussions categories.
- Managing app *installations* on a personal account (no API exists without app
  auth); apps are report-only, and on orgs only.
- A hosted service or webhooks. gcfg runs where you run it: a shell or an Actions job.

## 3. Options considered

1. **Adopt `safe-settings`.** Complete and maintained, but org-only, needs a hosted
   Probot app plus a central admin repo, and cannot serve a personal account's repos.
   Rejected: the operating model is the opposite of "each repo carries its own file".
2. **Terraform github provider.** Broad coverage, but a state file per repo, HCL,
   `terraform import` for adoption, and a heavyweight CI story for what is a YAML
   diff. No TUI. Rejected as the primary; kept in mind as an export target later.
3. **Keep growing one-off scripts** (`github_secret_scanning.sh`, `ruleset_snapshot.sh`).
   Cheapest today, unbounded tomorrow: no schema, no verify, no TUI, every family a new
   script. Rejected.
4. **A Go CLI under `sdk/` (`gcfg`) with a typed YAML schema, a settings-family
   plugin model, a bubbletea TUI, and Actions wrappers** — recommended. Mirrors how
   `gff` and `fleet` were built (cobra `cmd/`, `internal/` behind mockable seams,
   `libs/log`, tag-driven versions), reuses the user's existing `gh` login via
   `go-gh`'s token resolution so there is no second login, and puts the credential
   problem inside the tool where it can be diagnosed and fixed.

## 4. Decision

Build option 4 as two modules:

### 4.1 `sdk/ghapp` — GitHub App credential toolkit (reusable)

- `pkg/ghapp`: create an App through the **manifest flow** (`POST` a manifest to
  `github.com/settings/apps/new`, open the browser, receive the code on a localhost
  callback, exchange it at `POST /app-manifests/{code}/conversions` → app id, slug,
  PEM, webhook secret); store the PEM at `~/.config/ghapp/<slug>.pem` (0600) with
  app id + installation ids in `~/.config/ghapp/apps.yaml`; sign RS256 JWTs; mint
  installation tokens (`POST /app/installations/{id}/access_tokens`, optionally
  scoped to repositories/permissions); list installations; refresh with caching until
  expiry.
- `ghapp` CLI: `create`, `install` (opens the install page, records the installation),
  `token [--repo] [--org]`, `status`, `doctor`. Private use: the manifest is ours, the
  docs tell others how to create their own from the same manifest.
- In Actions: no ghapp binary needed — the workflow uses
  `actions/create-github-app-token` with `APP_ID` + `APP_PRIVATE_KEY` secrets; gcfg
  only consumes the resulting token from the environment.

### 4.2 `sdk/gcfg` — the tool

Units, each independently testable:

- `internal/schema` — Go structs for `gcfg.yaml` (`version`, `ownership`, `repo`,
  `org`), yaml.v3 load with unknown-field rejection, JSON Schema generation
  (`gcfg schema` → committed `.github/gcfg.schema.json`), and `lint` (org block only
  in a `.github` repo; ruleset names unique; enum values; no secret values anywhere).
- `internal/gh` — one HTTP client over `go-gh/v2` (token from env `GH_TOKEN` /
  `GITHUB_TOKEN`, else the user's `gh` login, else a ghapp-minted token); behind a
  `Client` interface with a recording fake for tests; rate-limit and retry.
- `internal/family` — the **settings-family plugin model**. A family implements
  `Read(ctx, target) (State, error)`, `Diff(desired, live) []Finding`, and
  `Apply(ctx, target, desired, plan) error`, and declares the permission it needs.
  v1 families, repo: `general` (merge/features/visibility/default-branch/topics/
  signoff), `security` (secret scanning + push protection + non-provider, dependabot
  alerts + security updates, private vulnerability reporting, code-scanning default
  setup), `rulesets` (imports `.github/rulesets/*.json` on export), `actions`
  (permissions, allowed actions, default workflow permissions, PR approval), `labels`,
  `autolinks`, `environments` (names + protection rules), `secrets` (names only),
  `webhooks` (URL/events, no secret), `collaborators` (report; write behind
  `ownership: full`), `pages` (report). Org: `profile`, `members`, `security-defaults`,
  `actions`, `rulesets`, `apps` (installed apps, report-only).
- `internal/engine` — orchestrates families: `Export`, `Verify` (findings → exit 0
  clean / 1 drift / 2 cannot read), `Plan`, `Apply` (plan → confirm → apply →
  **re-read verify**; a setting that does not stick is a hard failure with the exact
  key named, the lesson from non-provider patterns).
- `internal/report` — human (colored), `--json`, and `--markdown` (for
  `$GITHUB_STEP_SUMMARY` and PR comments) renderers from one `Finding` type.
- `internal/tui` — bubbletea: tree of families → keys; vim keys (`j/k/gg/G/h/l`,
  `/` regex search with `n/N`, `?` help), edit (`e`/`enter`) with typed editors
  (bool toggle, enum picker, string/list editors), `w` writes the YAML back, `v`
  runs verify inline and colors drift, `a` opens the apply plan and confirms.
  Style tokens shared from gff's `internal/style` shape.
- `internal/actions` — renders the two workflow files from templates:
  `gcfg-verify.yml` (pull_request + schedule + workflow_dispatch; posts step summary;
  fails on drift) and `gcfg-apply.yml` (push to default branch touching
  `.github/gcfg.yaml` + workflow_dispatch; concurrency group; re-verify after apply).
  Both fetch the pinned gcfg release and take the credential from `GH_TOKEN`, minted
  by `actions/create-github-app-token` when App secrets exist, else from a PAT secret.
- `cmd/` — cobra: `init [--from owner/repo]`, `export`, `lint`, `schema`, `verify`,
  `plan`, `apply`, `tui`, `actions install verify|apply`, `auth status|doctor|pat|app`,
  `version`.

**Data flow**

```
.github/gcfg.yaml ──lint──▶ desired ──┐
                                      ├─ diff ──▶ findings ──▶ {tty, --json, --markdown, TUI}
GitHub REST (token) ──read──▶ live ───┘                │
                                                       └─ apply plan ──confirm──▶ PATCH/PUT/DELETE ──▶ re-read verify
```

**Ownership** (`ownership: declared|full`, default `declared`): under `declared`, a
live item with no declaration is `unmanaged` (informational); under `full` it is
`drift` and `apply` deletes it. Per-family override allowed (e.g. `labels: {ownership:
full}`) so a repo can own its labels fully while leaving collaborators declared-only.

**Credential resolution** (same order everywhere, printed by `gcfg auth status`):
`GH_TOKEN` → `GITHUB_TOKEN` → `gh` login (via go-gh) → ghapp installation token for
the target (`--auth app`). `auth doctor` probes each family's endpoint and reports
read/write capability per family, so a PAT missing a permission is a clear line, not a
403 mid-apply.

### 4.3 Dotfiles adoption

- `install.sh` builds `gcfg` and `ghapp` into `~/opt/bin` behind `install.sdk.gcfg`
  (gff, default on). Make targets `gcfg-build|test|install|e2e`, `ghapp-*`, plus
  `gcfg-verify` / `gcfg-apply` for this repo.
- This repo gets its own `.github/gcfg.yaml` via `gcfg export`, absorbing what
  `github_secret_scanning.sh` and `ruleset_snapshot.sh` did. Both scripts are retired
  once `make gcfg-verify` is green in CI (the `.github/rulesets/*.json` snapshot is
  imported into the rulesets family and then removed).
- The two workflows are installed here first; the dotfiles repo is the reference
  deployment.

## 5. Risks & blast radius

- **`apply` is a write to production settings.** Mitigations: plan-then-confirm on a
  TTY (non-TTY requires `--yes`), per-family `--only`, `ownership: declared` default,
  deletes only under `full`, re-read verify, and `export --out` as a snapshot before
  the first apply (rollback material).
- **Settings that silently do not stick** (observed). Mitigation: re-read after every
  write; a non-sticking key is a failure naming the key and the likely plan gate.
- **Credential sprawl.** A PAT with Administration write is powerful. Mitigations:
  fine-grained PAT scoped to selected repos; prefer the App path (short-lived
  installation tokens, permissions declared in the manifest); `auth doctor` shows what
  a token can do; gcfg never prints tokens; the privacy guard's secret rules judge any
  file gcfg writes.
- **API surface churn.** Families are isolated; one family breaking does not break
  verify for the rest (`--only`, and a family read error is reported, not fatal, unless
  `--strict`).
- **Rate limits** on `export` for label-heavy repos: paginate, cache within a run,
  respect `Retry-After`.
- **Blast radius** is one repo (or one org when run in its `.github` repo). No cross-repo
  fan-out exists to misfire.

## 6. Rollback

- Tool: `gff set install.sdk.gcfg false` stops building it; remove `~/opt/bin/gcfg`.
- Settings: `gcfg export --out snapshot.yaml` before an apply; `gcfg apply -f
  snapshot.yaml` restores. Deletions under `ownership: full` are listed in the plan
  and in the `--json` apply log with the deleted object's export, so a label or ruleset
  can be re-created from the log.
- Actions: `gcfg actions uninstall` removes the two workflow files; a PR reverting the
  files is equivalent.
- Repo scripts: `github_secret_scanning.sh` / `ruleset_snapshot.sh` are only removed
  after `gcfg verify` covers their families in CI; reverting that commit restores them.

## 7. Evidence expectations

The plan must capture, under `plans/gcfg/evidence/`:

- **Unit/table tests** per family and per engine verb with a recording fake client
  (no network), coverage ≥80% overall (`sdk/` floor is 60%; gff set 90 — gcfg's
  surface is mostly API plumbing, 80 is the honest bar) and ≥90% for
  `internal/engine` and `internal/schema`.
- **Golden files**: `export` of a fixture repo, `--json` and `--markdown` findings,
  rendered workflow files, JSON Schema.
- **Live evidence (human-run, recorded)**: `gcfg export` against this repo; a
  deliberate drift (flip `delete_branch_on_merge` in the UI) caught by `gcfg verify`
  with exit 1 and the exact key; `gcfg apply` converging it back with the re-read
  proof; the non-provider-patterns key reported as not-sticking; `auth doctor` output
  for the `gh` token and for a ghapp-minted token; the verify workflow red on a
  drifted PR and green after; the apply workflow run log.
- **TUI**: teatest golden frames for navigation, search, edit, write-back, and the
  verify coloring.
- **ghapp**: manifest-flow creation recorded once (redacted), token mint + `auth
  doctor` proving Administration read/write on the target repo.

> Produced via `superpowers:brainstorming` under the `mbo-plan` pipeline. Registered in
> `../index.md`. Spec: `../specs/gcfg.md`.
