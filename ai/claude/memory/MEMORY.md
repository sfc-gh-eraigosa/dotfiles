<!-- Account-scoped canonical memory index (scope: account only).
     install_claude_skills.sh REGENERATES the LIVE ~/.claude/projects/<slug>/memory/MEMORY.md
     from the UNION of these account entries + any host-local memories already on the machine
     (it never blind-copies this file). Host-local memories (e.g. transient PR backlogs) are NOT
     listed here and are never promoted to the shared repo. See docs/mbo/designs/memory-provisioning.md. -->

- [gss land flow — no interrupt](gss-land-flow-no-interrupt.md) — after the user picks a land option on gss PRs, run commit→push→PR→snapshot without per-step re-confirmation.
- [rebuild gss after source change](gss-tmux-mgr-rebuild-after-source-change.md) — tmux-mgr shells out to the installed gss; a stale binary breaks it silently (unit tests mock the runner). Rebuild via sdk/gss/build.sh; guard: sdk/tmux-mgr/scripts/e2e-gss-integration.sh.
- [autonomous loop + workflows](autonomous-loop-workflows.md) — when user says "/loop" + "apply workflows" and steps away: execute fully autonomously, phase-by-phase with resumable STATE, multi-agent Workflow fan-out + adversarial review, checkpoint after each phase.
- [gss agent CLI gotchas](gss-agent-cli-gotchas.md) — use $HOME (privacy_guard), pass --worker not cwd to checkpoint/pr (safety_guard static-analyzes paths), re-apply PR body last (checkpoint regenerates it).
- [capture ids from tool output](capture-ids-from-tool-output.md) — reuse gss --json worker_ref verbatim; never reconstruct ids from env-var "defaults" gss may override (caused a false-FAIL in an eval).
