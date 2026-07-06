# mbo-plan skill

Drives the `docs/mbo` Management-By-Objective planning pipeline: capture an objective
(GitHub issue or current `gss` draft PR) → classify → route to the right skill workflow →
produce `design/spec/plan` artifacts under `docs/mbo/{designs,specs,plans}/<slug>.md` from the
templates → register in `docs/mbo/index.md` → attach to a `gss` draft PR.

The agent-facing instructions are in [`SKILL.md`](./SKILL.md); routing policy + conventions
live in [`docs/mbo/AGENTS.md`](../../../docs/mbo/AGENTS.md). `evals/evals.json` holds trigger
test prompts.
