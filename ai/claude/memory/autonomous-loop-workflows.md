---
name: autonomous-loop-workflows
description: "How the user wants large features executed when they say \"/loop\" + \"apply workflows\" and step away."
metadata: 
  scope: account
  node_type: memory
  type: feedback
  originSessionId: 4265a14d-0235-415e-97cc-e4d9f8091622
---

When the user kicks off a large feature with "/loop", asks me to "apply workflows" / "pick N team members", and says they're stepping away (e.g. going to bed), they want **fully autonomous execution to completion**: decompose into phases with a committed **resumable STATE doc** (docs/superpowers/plans/), execute via multi-agent **Workflow** fan-out (e.g. parallel file transforms + an adversarial principal-engineer review stage), validate each phase, and **`gss feature checkpoint`** after each coherent unit — no per-step re-confirmation once authorized to "checkpoint and push till done."

**Why:** Demonstrated on the ai-teams-install feature (PR #92, 2026-06-01): the user explicitly authorized overnight autonomy and reviews the result in the draft PR. The adversarial review stage earned its keep — it caught a critical eval scoring bug a solo pass missed.

**How to apply:** Use the Workflow tool for fan-out (the user opted in via "apply workflows"). Keep STATE updated every phase so a wakeup can resume. Schedule a long fallback wakeup (1200s+) but rely on workflow task-notifications to resume. Stop the loop (omit ScheduleWakeup) only when the full test suite passes and everything is pushed, then post a completion summary on the PR. Related: [[gss-land-flow-no-interrupt]], [[gss-autonomous-backlog]].
