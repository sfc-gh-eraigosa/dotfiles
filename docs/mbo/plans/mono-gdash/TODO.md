# mono-gdash — execution cursor

- **Slug:** mono-gdash
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../mono-gdash.md`](../mono-gdash.md): every task reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after you ran
> the command and read the output. After each `###` task: update TRACKING.md, commit, checkpoint.
>
> **Legend:** `SETUP` · `RED` · `RUN-RED` (expect FAIL) · `GREEN` · `RUN-GREEN` (expect PASS) ·
> `VERIFY` · `ALLOWLIST` · `DOCS` · `EVIDENCE` · `COMMIT` · `LEDGER` · `CHECKPOINT`.

## Preflight (once)

- [x] #411 merged → `gh pr view 411 --json state -q .state` = `MERGED`
- [x] Grafana health → `curl -s -m 5 http://192.168.0.128:30300/api/health` shows `"database": "ok"`
- [x] token file → `stat -c %a ~/.config/gdash/lab.cred` = `600`, and an authenticated `/api/search?limit=1` returns 200
- [x] SETUP: `gss feature worker add --feature mono-gdash --purpose gdash --description "gdash: Grafana dashboards as terminal graphs" --json` → paste into IMPLEMENTATION §2 + TRACKING §0
- [ ] SETUP: `gss feature worker add --feature mono-gdash --purpose deck --description "mono deck panes + nav mode" --json` → paste the same way

---

## Leaf `gdash`

### Task 0 — lab facts + fixtures  (plan §4 Task 0)

- [x] SETUP: `mkdir -p sdk/gdash/internal/grafana/testdata/lab`
- [x] capture `/api/health` and `/api/search?type=dash-db` (token from the file inside the curl header; never echoed)
- [x] identify the conductor, k3s-nodes and CI dashboards; record uid, title and datasource types in TRACKING "Lab facts"
- [x] capture each dashboard JSON and one `/api/ds/query` response per panel type
- [x] scrub: `grep -rE '(glsa_|eyJ|Bearer)' sdk/gdash/internal/grafana/testdata` → nothing; no hostnames but the NodePort address
- [x] ALLOWLIST: `git status --short -- sdk/gdash` lists the fixtures
- [x] EVIDENCE: `evidence/task0/capture.txt` (endpoints, sizes, scrub check)
- [x] COMMIT `gdash: lab Grafana fixtures (conductor, k3s-nodes, ci) for the test suite` · LEDGER · CHECKPOINT

**Done when:** fixtures are committed and scrubbed, and the three uids are in TRACKING.

### Task A1 — scaffold

- [ ] RED: `cmd/cmd_test.go` asserts `gdash version` prints the stamped version
- [ ] RUN-RED → FAIL
- [ ] GREEN: go.mod, main.go, build.sh (sources `../version.sh`), version, cobra root; AGENTS.md + `CLAUDE.md -> AGENTS.md` + README stub; `scripts/test.sh` `coverage_min` row `gdash) echo 60`
- [ ] RUN-GREEN → PASS; `bash sdk/gdash/build.sh` builds
- [ ] ALLOWLIST: `git status --short -- sdk/gdash`
- [ ] EVIDENCE · COMMIT `gdash: module scaffold (cobra root, version, build.sh)` · LEDGER · CHECKPOINT

### Task A2 — config + auth  (E10 E11 E13)

- [ ] RED: config validation tables; `auth_test.go` (0644 refused, symlink resolved, cmd no-tty 5 s timeout, empty stdout error); `cmd/config_test.go` (refuse inline secret)
- [ ] RUN-RED → FAIL
- [ ] GREEN: `internal/config`, `internal/auth` (Secret type), `cmd/config.go`
- [ ] RUN-GREEN → PASS; VERIFY `go test -cover ./internal/auth` ≥ 80%
- [ ] EVIDENCE · COMMIT `gdash: config profiles/views and credential sources (file 0600, env, cmd)` · LEDGER · CHECKPOINT

### Task A3 — client  (E1 E2)

- [ ] RED: `grafana/client_test.go` against an httptest server serving `testdata/lab`
- [ ] RUN-RED → FAIL
- [ ] GREEN: HTTP client, title resolution, at most 4 concurrent panel queries, timeouts
- [ ] RUN-GREEN → PASS
- [ ] EVIDENCE · COMMIT `gdash: Grafana client (search, dashboard, ds/query) over the lab fixtures` · LEDGER · CHECKPOINT

### Task A4 — dash + vars  (E3 E4)

- [ ] RED: dash model per fixture; `vars_test.go` table (all forms, All/multi, formats, `$__interval`, `$__rate_interval`, `$__range`, unresolved → needs var)
- [ ] RUN-RED → FAIL
- [ ] GREEN: `internal/dash`, `internal/vars`
- [ ] RUN-GREEN → PASS; VERIFY vars ≥ 80%
- [ ] EVIDENCE · COMMIT `gdash: dashboard model and template-variable expansion` · LEDGER · CHECKPOINT

### Task A5 — render  (E5 E6)

- [ ] DECIDE: ntcharts vs in-repo braille plotter (bubbletea v1 + `make security`); record in TRACKING notes
- [ ] RED: golden frames (chart, meter, placeholder, stale, needs-var × 80×24, 120×40, 1-col); `units_test.go`
- [ ] RUN-RED → FAIL
- [ ] GREEN: `internal/render`; `-update` the goldens, then **review each one by eye**
- [ ] RUN-GREEN → PASS; VERIFY render ≥ 80%
- [ ] EVIDENCE · COMMIT `gdash: braille charts, meters and placeholders (golden frames)` · LEDGER · CHECKPOINT

### Task A6 — app  (E7 E8 E9)

- [ ] RED: `stale_test.go`, `TestRefreshSkipIfBusy`, `keys_test.go`, app frame goldens
- [ ] RUN-RED → FAIL
- [ ] GREEN: `internal/app`, `cmd/view.go`
- [ ] RUN-GREEN → PASS; run `gdash conductor` against the fixture server by hand
- [ ] EVIDENCE · COMMIT `gdash: the dashboard TUI (layout, scroll, refresh, stale-not-blank)` · LEDGER · CHECKPOINT

### Task A7 — doctor + list  (E14 E15)

- [ ] RED: `cmd/doctor_test.go` (ok / 401 / down / missing dashboard; `--quiet` hang exits 1 in ≤ 2.5 s); list output
- [ ] RUN-RED → FAIL
- [ ] GREEN: `cmd/doctor.go`, `cmd/list.go`
- [ ] RUN-GREEN → PASS
- [ ] EVIDENCE · COMMIT `gdash: doctor (+ --quiet probe for mono) and list` · LEDGER · CHECKPOINT

### Task A8 — security + install + docs  (E12)

- [ ] RED: `security_test.go` (the fixture secret never appears in captured stdout/stderr across doctor, errors, config get, %v, JSON, panic)
- [ ] RUN-RED → FAIL
- [ ] GREEN: any redaction fixes; `install.sh` block (`gff_on install.sdk.gdash`); `.github/gff/features.yaml` flag; `sdk/AGENTS.md` + `sdk/README.md` rows and section; README demo from a **real** run
- [ ] VERIFY: `make security` · `make lint-go` · `make lint-markdown` · `./install.sh` builds gdash
- [ ] EVIDENCE · COMMIT `gdash: secret-never-printed guard, install.sh + gff flag, sdk docs` · LEDGER · CHECKPOINT
- [ ] label the gdash PR `ready-for-merge` once every check is green

---

## Leaf `deck`

### Task B1 — config schema  (E16)

- [ ] RED: `compat_test.go` (the #407 default round-trips byte-identical); validation (cmd XOR tools, unique names)
- [ ] RUN-RED → FAIL
- [ ] GREEN: Tools/Views; 1×1 normalization; save preserves plain panes
- [ ] RUN-GREEN → PASS
- [ ] EVIDENCE · COMMIT `mono: deck panes in the config (tools × views); plain panes unchanged` · LEDGER · CHECKPOINT

### Task B2 — probe + doctor  (E17)

- [ ] RED: `probe/deck_test.go`, doctor rows
- [ ] RUN-RED → FAIL · GREEN · RUN-GREEN → PASS
- [ ] EVIDENCE · COMMIT `mono: drop per view, tool, pane; doctor rows per view` · LEDGER · CHECKPOINT

### Task B3 — deck runtime  (E18 E19)

- [ ] RED: `TestOnlyVisibleViewRuns` (fake runner: HUP→KILL ≤ 500 ms), title label, `:focus` command tests
- [ ] RUN-RED → FAIL · GREEN · RUN-GREEN → PASS
- [ ] EVIDENCE · COMMIT `mono: deck runtime (only the visible view runs), title label, :focus` · LEDGER · CHECKPOINT

### Task B4 — nav mode  (E20 E21 E22 E23)

- [ ] RED: `nav_test.go` with an injected clock (prefix+space; esc esc at 200/400 ms; `nav_esc_ms: 0`; walk, ends, other key leaves without forwarding); README key-table test
- [ ] RUN-RED → FAIL
- [ ] GREEN: `app/nav.go`; `keys.go` stops stripping PageLeft/PageRight and re-labels them; help/title/README from the one table
- [ ] RUN-GREEN → PASS
- [ ] EVIDENCE · COMMIT `mono: nav mode (prefix+space or esc esc; hjkl through decks and panes)` · LEDGER · CHECKPOINT

### Task B5 — config verbs + UI

- [ ] RED: dotted-path verb tables; configui deck-row frame test
- [ ] RUN-RED → FAIL · GREEN · RUN-GREEN → PASS
- [ ] EVIDENCE · COMMIT `mono: config verbs and screen for decks` · LEDGER · CHECKPOINT

### Task B6 — boundary + docs  (E24)

- [ ] RED: `boundary_test.go` (no `sdk/gdash` import or go.mod require)
- [ ] RUN-RED: temporarily add a bad import → FAIL, then remove it
- [ ] GREEN: README/AGENTS deck + nav sections; `scripts/test.sh` `mono) echo 60`
- [ ] VERIFY: `make lint-go` · `make security` · `make lint-markdown`
- [ ] EVIDENCE · COMMIT `mono: boundary guard and deck/nav docs` · LEDGER · CHECKPOINT
- [ ] label the deck PR `ready-for-merge` once every check is green

---

## Integration (after both leaves merge)

### Task C1 — default `mid` deck

- [ ] RED: default-config golden expects the `mid` deck with four gdash views; E16 compat golden still passes
- [ ] RUN-RED → FAIL · GREEN · RUN-GREEN → PASS
- [ ] COMMIT `mono: default mid pane becomes a deck (gpu + gdash conductor/k3s-nodes/ci)` · LEDGER · CHECKPOINT

### Task C2 — live evidence

- [ ] spark1: `gdash doctor` passes → `evidence/e2e/doctor-spark1.txt`
- [ ] host without credentials: `gdash doctor` exit 1; mono `mid` = GPU pane → `evidence/e2e/doctor-nocred.txt`
- [ ] mono on spark1 cycling gpu → conductor → k3s-nodes → nodes → ci (`tmux capture-pane` each) → `evidence/e2e/deck-cycle.txt`
- [ ] `ps` shows one gdash process while cycling → `evidence/e2e/one-process.txt`
- [ ] TRACKING §2 and §3 all ticked; `index.md` → `done`
- [ ] COMMIT · CHECKPOINT · `ready-for-merge`
