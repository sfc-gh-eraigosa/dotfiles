# Security Scan green + required — execution cursor

- **Slug:** security-scan-green
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../security-scan-green.md`](../security-scan-green.md)

> First unchecked box = next action. Legend: RED · RUN-RED · GREEN · RUN-GREEN · VERIFY · COMMIT · LEDGER · CHECKPOINT.

## Preflight (once)
- [x] gosec v2.29.0 installed; Go 1.27.2 via goenv
- [x] `fix` worker created (`gss feature worker add … --purpose fix`)

### Task 1 — policy (plan §4.1)
- [x] GREEN: Makefile `security` target: pinned install, flags, rationale comment
- [x] VERIFY: `make security` → only the plan §2 findings remain → `evidence/policy/make-security-before.txt`
- [x] VERIFY: bare `#nosec` rejected → `evidence/policy/bare-nosec-rejected.txt`
- [x] COMMIT · LEDGER · CHECKPOINT

### Task 2 — gsl (plan §4.2)
- [x] RED: mode tests (config Save; gh writeCache; mcp writeCache + CreateTemp; usage Write dir)
- [x] RUN-RED → FAIL · GREEN · RUN-GREEN → PASS
- [x] VERIFY: gosec zero → `evidence/gsl/gosec.txt`; `go test ./...` → `evidence/gsl/go-test.txt`
- [x] COMMIT · LEDGER · CHECKPOINT

### Task 3 — gss (plan §4.3)
- [x] RED: mode tests (approval dir; config stub; repo cache; registry dir)
- [x] RUN-RED · GREEN (incl. feature dirs 0750, FEATURE.md/WORKER.md 0600, G101 nosec) · RUN-GREEN
- [x] VERIFY → `evidence/gss/` · COMMIT · LEDGER · CHECKPOINT

### Task 4 — tmux-mgr (plan §4.4)
- [x] RED: SaveSession file/dir modes; layout file mode
- [x] RUN-RED · GREEN (incl. migrate log, agent.log, config dir, aliases; rc-append nosec) · RUN-GREEN
- [x] VERIFY → `evidence/tmux-mgr/` · COMMIT · LEDGER · CHECKPOINT

### Task 5 — fleet / gff / gcfg / wlink / ghapp / libs (plan §4.5)
- [x] RED: fleet default plan 0600 · GREEN · gff overrides dir 0750; nosec with consumer on gen/export, gcfg export/schema, wlink snapshot sites + G101, ghapp G704
- [x] VERIFY each module → `evidence/<module>/` · COMMIT · LEDGER · CHECKPOINT

### Task 6 — whole repo (plan §4.6)
- [x] VERIFY: `make security` OK → `evidence/policy/make-security-after.txt`; `make lint-go`
- [x] CHECKPOINT; CI Security Scan green on the fix PR (`evidence/ci/pr396-checks.txt`)
- [x] /code-review round applied (8 findings; evidence regenerated)
- [ ] CI green on the review-fix push; label ready-for-merge

### Task 7 — promote (plan §4.7, separate PR after green main)
- [x] workflow `continue-on-error` removed; mergify protection line; docs/mergify.md updated (draft #397, stacked on fix)
- [ ] human: ruleset mirror; red proof PR → `evidence/promote/`
