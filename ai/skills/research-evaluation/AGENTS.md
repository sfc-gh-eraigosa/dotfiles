# research-evaluation skill

Evaluates external tools/projects before adoption: identifies each target (web
search, NOT_FOUND allowed), runs the seven-dimension rubric (value · setup/licensing
· adversarial · security · stability · quality · docker-or-skip demo), and lands the
results as research design docs + tracking issues in the repo's `docs/mbo/` system
(or a `docs/research/` fallback), fanned out to parallel agents for multi-target
batches.

Agent-facing instructions: [`SKILL.md`](./SKILL.md). First real-world run: a
private repo's 2026-08-07 six-target batch (omni-route, hermes-agent, claude-mem,
headroom, claude-code-setup, task-observer). `evals/evals.json` holds trigger cases.
