# fleet-config — TODO cursor

Resumable cursor: **the first unchecked box is the next action.** Check a box only
when its step is done and, for a task's final step, a commit SHA exists in TRACKING.md.

- [ ] T1 cfgplan: classify add / update / unchanged
- [ ] T2 cfgplan: name withheld directives + count Include
- [ ] T3 sshconf.Update: field-level rewrite
- [ ] T4 cfgplan.Apply: render adds + non-destructive updates
- [ ] T5 config pull: read over the runner seam
- [ ] T6 pull write safety: backup, 0600, loopback guard
- [ ] T7 key readiness: name absent IdentityFile paths
- [ ] T8 keys sync --host + manual-bootstrap reporting
- [ ] T9 config push: validate-before-install, self-retarget guard
- [ ] T10 config diff + post-push probe
- [ ] T11 TUI p / P bindings
- [ ] T12 AGENTS.md invariants

## Human-evidenced gates (cannot be closed by unit tests)

- [ ] G1 real two-machine pull, before/after config captured
- [ ] G2 push validation REJECTS a deliberately malformed config
- [ ] G3 self-retarget refused without the flag
- [ ] G4 key-readiness output on a genuinely absent IdentityFile
