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
- **Current phase:** P1 (in progress)
- **Completed:** spec (#92), this plan
- **Next action:** write model-map.yaml, teams.yaml, _partials/*; then launch P2 workflow
- **Notes:** yq v4.44.6 present; ollama IS installed (ollama create best-effort);
  antigravity path unconfirmed (emitter warns+skips if dir absent).
