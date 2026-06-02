---
name: teams_tune
description: Improve AI-team routing accuracy. Use when the routing eval misroutes tasks, when adding or editing team members, or when you want to raise team/member top-1 accuracy toward the >=90% gate. Tunes the SOURCE persona frontmatter (use_when/avoid_when/keywords/file_globs), never the generated agents.
---

# Teams Tune — self-improve AI team routing

Closes the loop from the routing **eval** back to the **source** persona files so the
AI teams route tasks to the right team and member with ≥90% top-1 accuracy.

Shared by Claude Code and Gemini CLI (linked via `sync-skills`). It edits only the
source of truth in `ai/teams/`; generated agents are always re-derived.

## When to use
- The routing eval (`ai/teams/eval/route-eval.sh`) reports team or member accuracy < 90%.
- You added/edited a team member and want to confirm it doesn't steal others' tasks.
- You're reviewing PR #-level routing quality before merge.

## Inputs / tools (all in this repo)
- `ai/teams/eval/route-eval.sh` — scores routing; `--check` validates the dataset only.
- `ai/teams/eval/cases.yaml` — labeled `task → {team, member|squad}` dataset.
- `ai/teams/validate.sh` — source consistency (run after every edit).
- `ai/teams/gen-index.sh` — regenerate INDEX/`/team`/nav after edits.
- `opt/scripts/system/install_ai_teams.sh` (alias `sync-teams`) — re-emit native agents.

## The tuning loop

1. **Baseline.** Run the eval against each runner you have creds for:
   `bash ai/teams/eval/route-eval.sh --runner claude` (and `--runner gemini`).
   Note team% and member% and read the confusion matrix + the listed misroutes.
2. **Diagnose each misroute.** For `expected X, got Y`, the cause is almost always a
   `description` collision. Compare X's and Y's `use_when` / `avoid_when` / `keywords` /
   `file_globs` in their `ai/teams/<team>/the_*.md`.
3. **Edit the SOURCE only.** Make the boundary explicit:
   - Sharpen the **winner's** `use_when` to name the exact trigger the task hit.
   - Add a **negative-scope** clause to the **loser's** `avoid_when` that routes this task
     to the correct member by name (e.g. "test authoring → The Go QA Engineer").
   - Tighten `file_globs`/`keywords` so they don't overlap across teams.
   Never edit `~/.claude/agents/**`, `~/.gemini/agents/**`, etc. — those are generated.
4. **Regenerate + re-validate.**
   `bash ai/teams/validate.sh && bash ai/teams/gen-index.sh && sync-teams`
5. **Re-eval.** Re-run step 1. Keep the change only if it improves the misrouted case
   **without** regressing others (watch the confusion matrix).
6. **Repeat** until team ≥ 90% **and** member ≥ 90% on every available runner.

## Optional: A/B a description
When one member's `use_when` is hard to tune, draft 2–3 variants, run the eval per
variant, and keep the highest-accuracy one. Vary one field at a time so the win is
attributable.

## Guardrails
- Source edits only; re-run `validate.sh` before every eval (a broken tier/ref makes the
  eval meaningless).
- Add a new `cases.yaml` entry whenever you discover a real misroute the dataset missed —
  the eval can only protect cases it contains; note any coverage you knowingly skip.
- A change that raises one case but lowers overall accuracy is a regression — revert it.
- Don't chase 100%: ambiguous tasks legitimately map to a squad. Prefer adding the squad
  expectation over contorting a member's scope.
