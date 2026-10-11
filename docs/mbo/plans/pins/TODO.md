# pins — execution cursor

- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)

## Preflight
- [x] worker `pins/edward-raigosa/build` exists
- [x] `cd sdk/fleet && go build ./...` passes in the worktree

### T1 — manifest + pins.sh + gff flag (L1)
- [x] RED `opt/lib/pins_test.sh` → RUN-RED
- [x] GREEN `opt/profiles/versions.tsv`, `opt/lib/pins.sh`, `pins.track` in features.yaml → RUN-GREEN
- [x] ALLOWLIST new paths · VERIFY lint-shell/lint-portability · COMMIT `feat(pins): version manifest + pins.sh resolver + pins.track flag`

### T2 — converging installers + manifest defaults (L1)
- [x] RED tests for sops/yq/gitleaks/k8s converge (installed≠want → reinstall) → RUN-RED
- [x] GREEN converge + `pins_want` defaults in uv/glow/rust/keyd/fonts/vault/install.sh → RUN-GREEN
- [x] VERIFY lint · COMMIT `feat(pins): pinned installers converge; defaults read from versions.tsv`

### T3 — internal/pins manifest + version (L2)
- [x] RED → GREEN → ≥85% · COMMIT `feat(fleet): internal/pins manifest + version compare`

### T4 — upstream resolver (L2)
- [x] RED → GREEN (httptest) · COMMIT `feat(fleet): pins upstream resolver (gh/go/text)`

### T5 — fleet pins status/bump (L2)
- [x] RED → GREEN · COMMIT `feat(fleet): fleet pins status + bump`

### T6 — installed + apply (L2)
- [x] RED → GREEN · COMMIT `feat(fleet): fleet pins status --installed + apply`

### T7 — status PINS column (L3)
- [x] RED → GREEN · VERIFY module cov ≥60%, lint-go · COMMIT `feat(fleet): PINS drift column in fleet status`

### T8 — rollout plan + workflow + docs (L3)
- [x] pins.yaml, install.sh chmod, pins-report.yml, docs · VERIFY actionlint, dry-run · COMMIT `feat(pins): fleet rollout plan, weekly drift report, docs`

### T9 — Go pin bump + live evidence (L3)
- [x] `fleet pins bump go --to 1.26.3`; capture live status outputs · COMMIT `fix(go): pin Go 1.26.3 to match go.mod (via fleet pins bump)`

### Close-out
- [x] code review · ledgers
- [x] index row → in-review · checkpoint · PR ready
- [x] review round 2 (8 fixed, 2 declined → #378), Go 1.26.9, CI guard + PR smoke test
- [x] merged as #377 (`eb88d18`, 2026-10-11); #376 closed
- [ ] host rollout: `install.sh` per host, or `fleet update --file opt/etc/fleet/pins.yaml <hosts>` (human)
