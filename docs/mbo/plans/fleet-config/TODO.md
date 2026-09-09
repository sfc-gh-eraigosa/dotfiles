# fleet-config — TODO cursor

Resumable cursor: **the first unchecked box is the next action.** Check a box only
when its step is done and, for a task's final step, a commit SHA exists in TRACKING.md.

- [x] T1 cfgplan: classify add / update / unchanged
- [x] T2 cfgplan: name withheld directives + count Include
- [x] T3 sshconf.Update: field-level rewrite
- [x] T4 cfgplan.Apply: render adds + non-destructive updates
- [x] T5 config pull: read over the runner seam
- [x] T6 pull write safety: backup, 0600, loopback guard
- [x] T7 key readiness: name absent IdentityFile paths
- [x] T8 keys sync --host + manual-bootstrap reporting
- [x] T9 config push: validate-before-install, self-retarget guard
- [x] T10 config diff + post-push probe
- [x] T11 TUI p / P bindings
- [x] T12 AGENTS.md invariants

## Human-evidenced gates (cannot be closed by unit tests)

- [x] G1 real pull over SSH, before/after captured (self-pull via LAN IP — see TRACKING note)
- [x] G2 push validation REJECTS a deliberately malformed config
- [x] G3 self-retarget refused without the flag
- [x] G4 key-readiness output on a genuinely absent IdentityFile
