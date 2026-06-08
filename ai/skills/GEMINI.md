# ai/skills — shared agent skills

This directory holds **generic agent skills** (each a folder with a `SKILL.md`) that drive
**both** Claude Code and Gemini CLI. `opt/scripts/system/sync-skills.sh` discovers every
`SKILL.md` here (and under `src/<tool>/`, `sdk/<tool>/skill/`) and links it into
`~/.claude/skills` and `~/.agents/skills` — edit once, benefit twice.

## Anatomy of a skill folder

```
ai/skills/<name>/
  SKILL.md            required — YAML frontmatter (name, description) + instructions
  evals/evals.json    recommended — the trigger/behavior eval corpus (see below)
  scripts/ references/ assets/   optional bundled resources
```

A tool can instead host its skill at `<tool>/skill/SKILL.md` (e.g. `sdk/gss/skill/`); the
skill is named after `<tool>` in that layout. New skills are typically authored with the
`skill-creator` skill and routed through the [`docs/mbo`](../../docs/mbo/GEMINI.md) pipeline
(the "A new skill" row of its task→workflow table).

## Evals are part of a skill (the requirement)

Every skill should ship an **`evals/evals.json`** in the `skill-creator` format — a corpus of
realistic trigger/behavior cases used to prove the skill activates and behaves correctly:

```json
{
  "skill_name": "<name>",
  "evals": [
    { "id": 1, "prompt": "a realistic user request", "expected_output": "what should happen", "assertions": [] }
  ]
}
```

- `skill_name` **must equal the skill folder name**.
- Each case needs a unique integer `id`, a non-empty `prompt`, and a non-empty `expected_output`.
- `assertions` (optional) make a case machine-gradable; without them a case is gradable only
  qualitatively.

### Two ways evals are exercised

1. **`make skill-evals`** → `opt/scripts/system/skill-eval.sh --check` — a **deterministic,
   no-model, CI-safe** validator. It walks **every** skill folder, validates each
   `evals/evals.json` that exists (JSON well-formed, `skill_name` matches, non-empty `evals`,
   unique ids, non-empty prompt/expected_output), and prints a **SKIP** line for any skill
   folder that has no corpus yet. Exit 0 when every present corpus is valid (skips are fine);
   exit 1 on an invalid corpus. This is the gate to keep corpora honest.
2. **The `skill-creator` eval loop** — the on-demand, **model-driven** behavioral grading
   (spawn with-skill vs baseline agents per case, grade, view accuracy/lift). This is where you
   actually measure whether the skill helps; it is **not** a deterministic make target.

> Validate before you commit a skill change: `make skill-evals`. A skill with no corpus is
> reported as SKIP — acceptable, but adding one is how the skill earns a regression signal.

Per-directory docs rule: this dir has `GEMINI.md` + `CLAUDE.md → GEMINI.md`, linked from the
root `GEMINI.md` Repository Structure section.
