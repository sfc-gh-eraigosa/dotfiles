# gcfg — execution cursor

- **Slug:** gcfg
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../gcfg.md`](../gcfg.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after
> you ran the command and read the output. After finishing a `###` task: update
> `TRACKING.md`, commit with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [x] `go version` matches `.go-version`
- [x] `gh auth status` shows `repo` scope
- [~] design PR merged (**not merged** — build worker stacked on `feature/gcfg/edward-raigosa/design`, PR #285, by owner decision 2026-09-05); `gss feature list --feature gcfg --json` shows the feature ✓ (row re-created after the 2026-09-05 registry audit dropped it)
- [x] worker for the phase created via `gss feature worker add --feature gcfg --purpose <leaf> --description "…" --json`; row copied into IMPLEMENTATION §2 + TRACKING §0
- [x] `git status --short` clean in the worker worktree

---

### P0-T1 — ghapp scaffold + version  (plan P0-T1)
- [x] SETUP: `sdk/ghapp/{go.mod,main.go,build.sh}` mirroring `sdk/gff` (module path with `sdk/`), `internal/version`
- [x] RED: `cmd/version_test.go` expects a non-empty version string from ldflags default `dev`
- [x] RUN-RED: `cd sdk/ghapp && go test ./cmd/` → expect **FAIL**
- [x] GREEN: cobra root + `version`
- [x] RUN-GREEN: `go test ./... -cover` → **PASS**; `go run . version`
- [x] VERIFY: `.github/workflows/ghapp-ci.yml` (vet, test, ≥80% gate); `git status --short -- sdk/ghapp` shows files tracked (ALLOWLIST)
- [x] DOCS: `sdk/ghapp/{AGENTS.md,CLAUDE.md→AGENTS.md,README.md}` stubs
- [x] COMMIT: `feat(ghapp): module scaffold + version`
- [ ] LEDGER + CHECKPOINT
**Done when:** CI green on the worker PR; version prints.

### P0-T2 — store + JWT  (plan P0-T2)
- [ ] RED: `pkg/ghapp/store_test.go` (0700 dir, 0600 PEM, round-trip, refuse 0644 PEM); `jwt_test.go` (RS256, iss/iat/exp, verifies with pubkey)
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `store.go`, `jwt.go` (golang-jwt/v5)
- [ ] RUN-GREEN: `go test ./pkg/ghapp/ -cover` → **PASS**
- [ ] COMMIT: `feat(ghapp): app store + RS256 JWT`
- [ ] LEDGER + CHECKPOINT

### P0-T3 — installation tokens  (plan P0-T3)
- [ ] RED: httptest stub for `GET /app/installations`, `POST /app/installations/{id}/access_tokens`; cache hit/miss around expiry-2m; scoping body; token-leak grep on logs
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `token.go`, `installs.go`
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `feat(ghapp): installation tokens with cache`
- [ ] LEDGER + CHECKPOINT

### P0-T4 — manifest flow + CLI  (plan P0-T4)
- [ ] RED: `manifest_test.go` (listener + port fallback; form fields; conversion exchange via stub; expired code error; injected browser opener); cmd tests for `create/install/token/status/doctor`
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `manifest.go`, cmd verbs
- [ ] RUN-GREEN → **PASS**; coverage ≥80%
- [ ] VERIFY (human, ask first): one real `ghapp create` + `ghapp token --repo <this repo>` → `evidence/ghapp/` (redacted)
- [ ] COMMIT: `feat(ghapp): manifest-flow create + CLI`
- [ ] LEDGER + CHECKPOINT
**Done when:** ghapp-ci green; token mint evidence captured.

### P1-T1 — gcfg scaffold + CI  (plan P1-T1)
- [ ] SETUP/RED/GREEN as P0-T1 for `sdk/gcfg`; `gcfg-ci.yml` with 80/90/90 gates + schema-drift placeholder
- [ ] COMMIT: `feat(gcfg): module scaffold + version + CI`
- [ ] LEDGER + CHECKPOINT

### P1-T2 — schema load + lint + JSON Schema  (plan P1-T2)
- [ ] RED: `internal/schema/load_test.go` (unknown key path error; all-optional; per-family ownership), `lint_test.go` (org block outside `.github`; dup names; enums; secret-shaped value), `jsonschema_test.go` (golden)
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `types.go`, `load.go`, `lint.go`, `jsonschema.go`; `gcfg lint`, `gcfg schema`
- [ ] RUN-GREEN: `go test ./internal/schema/ -cover` ≥90% → **PASS**
- [ ] COMMIT: `feat(gcfg): typed schema, strict load, lint, JSON Schema`
- [ ] LEDGER + CHECKPOINT

### P1-T3 — gh client + credential chain  (plan P1-T3)
- [ ] RED: `internal/gh/fake_test.go` (records calls, serves fixtures), `auth_test.go` (order GH_TOKEN → GITHUB_TOKEN → gh login → ghapp; none → error), `cmd/auth_status_test.go` (never prints token)
- [ ] RUN-RED → **FAIL**
- [ ] GREEN: `client.go`, `real.go` (go-gh REST, retry, Retry-After), `fake.go`, `auth.go`
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `feat(gcfg): gh client seam, recording fake, credential chain`
- [ ] LEDGER + CHECKPOINT

### P1-T4 — family model + general + security  (plan P1-T4)
- [ ] RED: `family_test.go` (registry), `general/general_test.go`, `security/security_test.go`: Read from fixture; Export golden; Diff matrix (declared/full/missing/not-honoured); Apply records PATCH body
- [ ] RUN-RED → **FAIL**
- [ ] GREEN
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `feat(gcfg): family model + general + security`
- [ ] LEDGER + CHECKPOINT

### P1-T5 — engine + renderers  (plan P1-T5)
- [ ] RED: `engine/{export,verify,plan,apply,ownership}_test.go` (clean; drift; unreadable→finding; full extras; apply→re-read→not-honoured survives; call order), `report/*_test.go` goldens (tty/json/markdown)
- [ ] RUN-RED → **FAIL**
- [ ] GREEN
- [ ] RUN-GREEN: `go test ./internal/engine/ -cover` ≥90% → **PASS**
- [ ] COMMIT: `feat(gcfg): engine + renderers`
- [ ] LEDGER + CHECKPOINT

### P1-T6 — verbs export/verify/plan/apply/init  (plan P1-T6)
- [ ] RED: `cmd/{export,verify,plan,apply,init}_test.go` (exit codes; non-TTY apply w/o --yes → 2 and zero writes; --only; init golden; --from via fake; refuse overwrite; token grep)
- [ ] RUN-RED → **FAIL**
- [ ] GREEN
- [ ] RUN-GREEN → **PASS**; module coverage ≥80%
- [ ] VERIFY (live, ask before apply): `go run . export` on this repo → verify 0; flip `delete_branch_on_merge` in UI → verify 1 names key; `apply` → re-read 0 → `evidence/core/`
- [ ] COMMIT: `feat(gcfg): export/verify/plan/apply/init`
- [ ] LEDGER + CHECKPOINT
**Done when:** gcfg-ci green 80/90/90; UC1–UC3 evidence captured.

### P2-T1 … P2-T10 — families  (plan P2)
For each family in plan order (rulesets · actions · labels · autolinks · environments · secrets+webhooks · collaborators+pages · org profile+members+security_defaults · org actions+rulesets · org apps):
- [ ] RED: `<family>_test.go` (fixture Read; Export golden; Diff matrix; Apply body; pagination where relevant; rulesets: import of `.github/rulesets/*.json`)
- [ ] RUN-RED → **FAIL**
- [ ] GREEN
- [ ] RUN-GREEN → **PASS**
- [ ] COMMIT: `feat(gcfg): <family> family`
- [ ] LEDGER + CHECKPOINT

### P3-T1 — auth doctor + pat  (plan P3-T1)
- [ ] RED: probe matrix over fake (read/write per family; 403 → permission name; org scope message); `pat --check` from stdin; token grep
- [ ] RUN-RED → **FAIL** · GREEN · RUN-GREEN → **PASS**
- [ ] VERIFY (live): `gcfg auth doctor` with gh token → `evidence/auth/`
- [ ] COMMIT: `feat(gcfg): auth doctor + pat checklist` · LEDGER + CHECKPOINT

### P3-T2 — auth app wrappers  (plan P3-T2)
- [ ] RED/GREEN thin verbs over `pkg/ghapp`; VERIFY (live): `gcfg auth doctor --auth app` → `evidence/auth/`
- [ ] COMMIT: `feat(gcfg): auth app verbs` · LEDGER + CHECKPOINT

### P3-T3 — actions install/uninstall  (plan P3-T3)
- [ ] RED: render goldens ×{verify,apply}×{app,pat}; pinned version+checksum; refuse overwrite; uninstall only ours; actionlint step in gcfg-ci
- [ ] RUN-RED → **FAIL** · GREEN · RUN-GREEN → **PASS** · VERIFY: `actionlint` on goldens
- [ ] COMMIT: `feat(gcfg): actions install verify|apply` · LEDGER + CHECKPOINT

### P3-T4 — e2e harness  (plan P3-T4)
- [ ] RED: `scripts/e2e.sh` scenario list fails (no binary path yet) · GREEN · RUN-GREEN: `make gcfg-e2e` → **PASS**
- [ ] COMMIT: `test(gcfg): binary-level e2e` · LEDGER + CHECKPOINT

### P4-T1 — TUI navigation + search  (plan P4-T1)
- [ ] RED teatest goldens (tree, j/k/gg/G, h/l fold, `/` smartcase + n/N, `?`, tiny terminal) · GREEN · RUN-GREEN
- [ ] COMMIT: `feat(gcfg): tui navigation + search` · LEDGER + CHECKPOINT

### P4-T2 — TUI editors + write-back  (plan P4-T2)
- [ ] RED (editors, `u`, `w` writes only changed keys with comments kept, quit-unsaved prompt, byte-identical without `w`) · GREEN · RUN-GREEN
- [ ] COMMIT: `feat(gcfg): tui editors + write-back` · LEDGER + CHECKPOINT

### P4-T3 — TUI verify/apply  (plan P4-T3)
- [ ] RED (`v` colors; `a` plan+confirm+apply via fake; recolor) · GREEN · RUN-GREEN
- [ ] COMMIT: `feat(gcfg): tui verify + apply` · LEDGER + CHECKPOINT

### P5-T1 — wiring  (plan P5-T1)
- [ ] `install.sdk.gcfg` in `.github/gff/features.yaml`; install.sh block (builds gcfg + ghapp); Makefile targets; `sdk/AGENTS.md` + `sdk/README.md` rows/sections; module docs
- [ ] VERIFY: `bash -n install.sh`; `make lint-shell lint-portability`; `make -n gcfg-test gcfg-verify`
- [ ] COMMIT: `feat(install): build gcfg + ghapp behind install.sdk.gcfg` · LEDGER + CHECKPOINT

### P5-T2 — this repo's gcfg.yaml + schema + CI verify  (plan P5-T2)
- [ ] `gcfg export` → `.github/gcfg.yaml` (review; rulesets imported); `gcfg schema` → `.github/gcfg.schema.json`
- [ ] `make gcfg-verify` job in `gcfg-ci.yml` (skips with notice when the secret is absent)
- [ ] VERIFY: job green on the PR → `evidence/adoption/`
- [ ] COMMIT: `feat(github): declare this repo's settings in .github/gcfg.yaml` · LEDGER + CHECKPOINT

### P5-T3 — workflows installed  (plan P5-T3)
- [ ] `gcfg actions install both`; first verify run green; drift PR red → apply → green; logs → `evidence/actions/`
- [ ] COMMIT: `feat(github): gcfg verify + apply workflows` · LEDGER + CHECKPOINT

### P5-T4 — retire the one-offs  (plan P5-T4)
- [ ] VERIFY precondition: P5-T2 job green on main
- [ ] remove `opt/scripts/git/{github_secret_scanning,ruleset_snapshot}.sh` (+tests), Makefile targets, AGENTS bullets, `.github/rulesets/main.json`
- [ ] VERIFY: `make shell-test hook-test`; `grep -rn 'secret_scanning.sh\|ruleset_snapshot' .` empty
- [ ] COMMIT: `chore(git): retire secret-scanning + ruleset snapshot scripts (superseded by gcfg)` · LEDGER + CHECKPOINT
**Done when:** TRACKING §3 stop condition all ticked; `index.md` state → in-review.
