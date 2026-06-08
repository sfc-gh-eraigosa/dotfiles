---
name: capture-ids-from-tool-output
description: "Capture identifiers from a tool's actual output (e.g. gss --json worker_ref); never reconstruct them from inputs you set."
metadata: 
  scope: account
  node_type: memory
  type: feedback
  originSessionId: 9afcdc3a-a822-4864-980c-1d707265ee3d
---

When a command emits an identifier, capture it from the **output**, never rebuild it from the inputs you passed.

**Why:** In the worker-md-placement session I set `GSS_DEFAULTS_USER=erai` but gss resolved the real `$USER` (`<user>`) — so a worker created as `eval/<user>/api` was referenced in my eval script as `eval/erai/api`. The `gss feature done` step targeted a non-existent worker, no-op'd, and produced a **false FAIL** on the cleanup check (the code was correct). Worse: I'd already *seen* the `<user>` resolution in an earlier scratch run and still hardcoded the wrong ref.

**How to apply:** `gss feature worker add --json` returns `{worker_ref, branch, worktree_path, base_branch}` — parse and reuse `worker_ref`/`worktree_path` verbatim for every downstream step. Generalize: env-var "defaults" (user, base) are hints gss may override; treat the JSON as ground truth. When a test/eval reconstructs an id from assumptions, that's a bug in the harness, not the system under test — confirm against observed output before reporting a failure. Related: [[gss-agent-cli-gotchas]].
