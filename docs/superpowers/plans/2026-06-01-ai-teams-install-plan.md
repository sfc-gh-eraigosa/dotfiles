# AI Teams Install — Implementation Plan (resumable)

Spec: `docs/superpowers/specs/2026-06-01-ai-teams-install-design.md`
Branch: `feature/ai-teams-install/edward-raigosa/design` · Draft PR #92

Build order follows dependencies. Each phase ends with validation + a
`gss feature checkpoint`. The **STATE** block at the bottom is updated after every
phase so any wakeup can resume cleanly.

## Phases

- **P1 Foundation data** — `model-map.yaml`, `teams.yaml` (teams + squads),
  `_partials/{common-safety,repo-conventions,handoff-footer}.md`.
  Done: `yq` parses all; partials referenced by `compose:` exist.
- **P2 Refactor 21 personas** — comment-headers → YAML frontmatter
  (`name/team/role/tier/domain/file_globs/keywords/use_when/avoid_when/color/symbol/compose`)
  + preserved body. Fan-out workflow, one agent per file, principal review.
  Done: every file's frontmatter parses with `yq`; `tier` ∈ model-map; `team` == folder.
- **P3 Transformer** — `opt/scripts/system/install_ai_teams.sh`: parse → resolve tier →
  compose → 4 emitters (claude/gemini/antigravity/ollama) with graceful-skip + idempotency.
  Done: dry-run emits valid artifacts into a temp HOME; re-run = no diff.
- **P4 validate.sh** — zero-dep yq assertions over source + map + teams.yaml.
  Done: passes on the refactored tree; fails loudly on injected bad data.
- **P5 Tests** — `install_ai_teams_test.sh` (tier resolution, emitter validity,
  idempotency, graceful-skip, compose ordering). Done: all asserts pass.
- **P6 Discoverability** — `INDEX.md` generation, `/team` (claude `.md` + gemini `.toml`),
  per-team `GEMINI.md` + `CLAUDE.md→GEMINI.md` symlinks, root `CLAUDE.md` row.
- **P7 Wiring + docs** — call `install_ai_teams.sh` in `install.sh`; `sync-teams` alias;
  rewrite `MODEL_PARAMS.md` to reference `model-map.yaml`; update `README.md` +
  `opt/scripts/system` registry.
- **P8 Eval + CI** — `eval/cases.yaml`, `eval/route-eval.sh` (claude+gemini headless,
  top-1 team & member, confusion matrix), `.github` workflow blocking ≥90% gate.
- **P9 teams-tune skill** — `src/teams-tune/skill/SKILL.md`.
- **P10 Final** — full validate + tests, adversarial principal review, final checkpoint.

## Conventions
- All edits land in the worktree; checkpoint via `gss feature checkpoint` (draft PR).
- No literal `$HOME`/username paths in tracked content (privacy_guard) — use `~`/`${HOME}`.
- Stage by explicit name; never `git add -A`.
- Each script: `set -euo pipefail`-style safety, graceful-skip for absent tools.

---

## STATE
- **STATUS: COMPLETE.** All phases P1–P10 done, committed, checkpointed to PR #92.
  Final suite green: bash -n all shells; validate.sh ✓ 21; install_ai_teams_test 29/29;
  route-eval --check OK (49); gen-index no drift; teams-eval.yml YAML OK. Final principal
  review verdict READY; its 2 latent-Major findings (ollama `"""`, color=null) FIXED +
  guarded by validate + tests; prune added (zombie agents removed); spec reconciled
  (CI-gate advisory-until-runner-CLI, single-call eval, antigravity name). Loop stopped.
- **P10 DONE.** Earlier-phase notes retained below for history.
- **(historic) Current phase:** P10 (final review) in progress.
- **P8 DONE:** eval/cases.yaml (49 cases, all 21 members + 6 squad/ambiguous),
  eval/route-eval.sh (roster-built candidates, claude/gemini runners, top-1 team+member,
  confusion matrix, exit 0/1/77, deterministic --check), .github/workflows/teams-eval.yml
  (validate job: validate.sh + tests + gen-index drift; route-eval job: 90% gate both
  runners, 77→neutral skip, SHA-pinned). **Principal caught a CRITICAL scoring bug**
  (hyphenated team names terraform-aws/ai-ci split at first hyphen) — FIXED: scoring now
  resolves pick against the roster; verified with stub runner (team→terraform-aws, not
  terraform). --check OK; YAML OK; validate ✓; 26/26; no drift.
- **P9 DONE:** src/teams-tune/skill/SKILL.md.
- **P10 plan:** final whole-branch principal review (silent-failure/quality); confirm
  full suite; update spec risks if antigravity/CI-creds reality changed; final checkpoint;
  STOP the loop (omit ScheduleWakeup) and summarize on PR #92.
- **P7 DONE:** install.sh wiring; sync-teams alias; root GEMINI.md row; MODEL_PARAMS
  rewritten to tier+model-map; README + registry. Shells clean; validate ✓; 26/26; 8/8.
- **Completed:** spec (#92); plan; P1 (322316d); **P2** 21 personas (review ok=true);
  **P3** installer (4 emitters, idempotent); **P4** validate.sh (✓21);
  **P5** install_ai_teams_test.sh (26/26 pass); **P6** gen-index.sh → INDEX.md,
  root+per-team GEMINI.md (+CLAUDE.md symlinks), `/team` command (claude team.md +
  gemini team.toml, TOML parses, table embedded, deterministic).
- **Next action:**
  1. P7 wire install.sh (call install_ai_teams.sh after sync-plugins); `sync-teams`
     alias; add ai/teams row to root CLAUDE.md/GEMINI.md Repository Structure; rewrite
     MODEL_PARAMS.md to reference model-map.yaml; update ai/teams/README.md +
     opt/scripts/system registry. (gen-index is dev/CI-run, NOT install-run.)
  2. P8 eval/cases.yaml + route-eval.sh + CI gate (use `ci`+`aiarch`); CI also runs
     validate.sh + install_ai_teams_test.sh + `gen-index.sh && git diff --exit-code`.
  3. P9 src/teams-tune/skill/SKILL.md.  4. P10 final principal review + checkpoint.
- **Notes:** yq v4.44.6; ollama installed (create best-effort); antigravity path
  unconfirmed (emit anyway, harmless). Installer test override:
  TEAMS_DEST_HOME=<dir> + SKIP_OLLAMA_CREATE=1.
