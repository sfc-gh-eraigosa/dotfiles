# Security Scan green + required — execution cursor

- **Slug:** security-scan-green
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../security-scan-green.md`](../security-scan-green.md)

> First unchecked box = next action. Legend: RED · RUN-RED · GREEN · RUN-GREEN · VERIFY · COMMIT · LEDGER · CHECKPOINT.

## Preflight (once)
- [x] gosec v2.29.0 installed; Go 1.27.2 via goenv
- [x] `fix` worker created (`gss feature worker add … --purpose fix`)

### Task 1 — policy (plan §4.1)
- [ ] GREEN: Makefile `security` target: pinned install, flags, rationale comment
- [ ] VERIFY: `make security` → only the plan §2 findings remain → `evidence/policy/make-security-before.txt`
- [ ] VERIFY: bare `#nosec` rejected → `evidence/policy/bare-nosec-rejected.txt`
- [ ] COMMIT · LEDGER · CHECKPOINT

### Task 2 — gsl (plan §4.2)
- [ ] RED: mode tests (config Save; gh writeCache; mcp writeCache + CreateTemp; usage Write dir)
- [ ] RUN-RED → FAIL · GREEN · RUN-GREEN → PASS
- [ ] VERIFY: gosec zero → `evidence/gsl/gosec.txt`; `go test ./...` → `evidence/gsl/go-test.txt`
- [ ] COMMIT · LEDGER · CHECKPOINT

### Task 3 — gss (plan §4.3)
- [ ] RED: mode tests (approval dir; config stub; repo cache; registry dir)
- [ ] RUN-RED · GREEN (incl. feature dirs 0750, FEATURE.md/WORKER.md 0600, G101 nosec) · RUN-GREEN
- [ ] VERIFY → `evidence/gss/` · COMMIT · LEDGER · CHECKPOINT

### Task 4 — tmux-mgr (plan §4.4)
- [ ] RED: SaveSession file/dir modes; layout file mode
- [ ] RUN-RED · GREEN (incl. migrate log, agent.log, config dir, aliases; rc-append nosec) · RUN-GREEN
- [ ] VERIFY → `evidence/tmux-mgr/` · COMMIT · LEDGER · CHECKPOINT

### Task 5 — fleet / gff / gcfg / wlink / ghapp / libs (plan §4.5)
- [ ] RED: fleet default plan 0600 · GREEN · gff overrides dir 0750; nosec with consumer on gen/export, gcfg export/schema, wlink snapshot sites + G101, ghapp G704
- [ ] VERIFY each module → `evidence/<module>/` · COMMIT · LEDGER · CHECKPOINT

### Task 6 — whole repo (plan §4.6)
- [ ] VERIFY: `make security` OK → `evidence/policy/make-security-after.txt`; `make lint-go`
- [ ] CHECKPOINT; CI Security Scan green on the fix PR; human: label ready-for-merge

### Task 7 — promote (plan §4.7, separate PR after green main)
- [ ] workflow `continue-on-error` removed; mergify protection line; docs/mergify.md updated
- [ ] human: ruleset mirror; red proof PR → `evidence/promote/`
