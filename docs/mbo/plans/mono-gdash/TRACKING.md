# mono-gdash — live state ledger

- **Slug:** mono-gdash
- **Started:** 2026-10-18
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Cursor:** [`TODO.md`](./TODO.md)
- **Plan (source of truth):** [`../mono-gdash.md`](../mono-gdash.md) · spec [`../../specs/mono-gdash.md`](../../specs/mono-gdash.md)

> **Update after EVERY task.** Status: `todo · in-progress · blocked · done`. A row is `done` only
> with a commit SHA **and** evidence (the command plus its real result).

## 0. Worker registry

| Leaf | Worker ref | Branch | Worktree path | PR | State |
| :-- | :-- | :-- | :-- | :-- | :-- |
| design | `mono-gdash/edward-raigosa/design` | `feature/mono-gdash/edward-raigosa/design` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/mono-gdash/edward-raigosa/design` | #411 | design + spec + plan + trio |
| gdash | `mono-gdash/edward-raigosa/gdash` | `feature/mono-gdash/edward-raigosa/gdash` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/mono-gdash/edward-raigosa/gdash` | #413 | Task 0 + A1–A8 done |
| deck | `mono-gdash/edward-raigosa/deck` | `feature/mono-gdash/edward-raigosa/deck` | `~/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/mono-gdash/edward-raigosa/deck` | — | B1 |

## 1. Task ledger

| Task | Status | Commit | Evidence (command → result) | Notes |
| :-- | :-- | :-- | :-- | :-- |
| 0 lab facts + fixtures | done | (this commit) | `capture.py` → 4 dashboards, 10 panel queries, all with frames; scrub greps → 0 files (`evidence/task0/capture.txt`) | issued by the service account `gdash-viewer` (Viewer; admin API → 403), created 2026-10-19 with the owner's go-ahead |
| A1 scaffold | done | (this commit) | RED: `TestVersionPrintsStampedVersion` printed the root help (no version cmd); GREEN: `go test -cover ./...` → cmd 81.8%; `build.sh` builds, `gdash version` prints the stamped version; golangci-lint 0 issues (`evidence/a1/a1.txt`) | |
| A2 config + auth | done | (this commit) | RED: build failures (no package) + `unknown command "config"`; GREEN: cmd 81.1%, auth 90.6% (≥ 80 bar), config 75.1%; E10/E11/E13 tests pass; lint 0 issues; gosec (repo flags) exit 0 (`evidence/a2/a2.txt`) | |
| A3 client | done | (this commit) | RED: setup failed (no grafanatest); GREEN: 9 tests pass incl. E1 (`TestFetchAllCapsConcurrency`: 10 queries, peak ≤ 4) and E2 (`TestResolve`); grafana 89.2%; lint 0; gosec exit 0 (`evidence/a3/a3.txt`) | `grafanatest` serves the lab fixtures for every later package |
| A4 dash + vars | done | (this commit) | RED: build failed (no packages, no `Datasources`); GREEN: dash 81.3% (all 4 lab dashboards parse to the expected kinds), vars 95.8% (≥ 80 bar), E3 `TestExpand` + E4 `TestRateInterval` pass; lint 0; gosec exit 0 (`evidence/a4/a4.txt`) | added `Client.Datasources` + `datasources.json` fixture: `$datasource` resolves by uid or name, `default`/unset → the default ds; scrape interval 30s from the ds |
| A5 render | done | (this commit) | RED: build failed (no package); GREEN: render 91.6% (≥ 80 bar); 37 goldens (10 lab panels × 80/120/1 col + edges), reviewed by eye; width sweep 1–130 × color on/off: no line wider than its width; NO_COLOR threshold markers; lint 0; gosec exit 0 (`evidence/a5/a5.txt`) | **charting: in-repo braille plotter, not ntcharts** (no dependency, deterministic goldens). Review fix: `-0.0%` axis label → `0.0%` |
| A6 app | done | (this commit) | RED: setup failed; fixes on the way: injectable tick (tea.Tick slept 30s in tests), gdash keys first in help; GREEN: app 86.0%, cmd 79.7%; E7 stale/E8 skip-if-busy/E9 keys pass; 6 frame goldens incl. 120-col two-column CI; **live** on the lab: k3s-nodes, conductor, ci, nodes all draw (`evidence/a6/a6.txt`, `live-*.txt`) | live found a real bug: multi-target panels without refId → HTTP 400; `dash.Parse` now assigns refIds; HTTP errors carry Grafana's message. Follow-up (not v1): resolve `label_values()` query variables via the ds resource API (a Viewer may call it) so `$cluster`/`$instance` need no override |
| A7 doctor + list | done | (this commit) | RED: build failed; GREEN: 8 doctor/list tests incl. E14 (ok / 401 / down / missing dashboard) and E15 (`--quiet` against a hanging Grafana fails in 2.01 s); cmd 80.4%; lint 0; gosec exit 0; **live**: doctor all ok (4 views + Prometheus), `--quiet` 35 ms, no-config host exit 1 silently (`evidence/a7/`) | |
| A8 security + install + docs | done | (this commit) | RED: build failed (no EchoAuth/Redact); GREEN: E12 `TestSecretNeverPrinted` builds the binary and runs 6 credential setups × 8 commands against a fake Grafana that echoes the Authorization header: no leak; mutation (redaction off) → fails; module coverage 81.2%; lint 0; pinned govulncheck: no vulnerabilities; pinned gosec exit 0; gff lint ok; markdownlint 0 on touched docs; install.sh block builds/skips by flag (`evidence/a8/a8.txt`) | credentials redact themselves (bearer: the token; basic: the password and base64 user:password) and the client scrubs every error message |
| B1 config schema | done | (this commit) | RED: build failed; GREEN: E16 `TestCompat407RoundTrip` (the #407 file round-trips byte for byte), deck validation (11 bad shapes refused), get/set at tool/view level, add/remove tool/view; config 85.0%; all mono packages pass (`evidence/b1/b1.txt`) | also B7's config half: `zoomable`, tool `zoom`, `middle: single` → `Effective()` (E27) |
| B2 probe + doctor | todo | | | |
| B3 deck runtime | todo | | | |
| B4 nav mode | todo | | | |
| B5 config verbs + UI | todo | | | |
| B7 zoom + one-pane mode | todo | | | amended 2026-10-19 |
| B6 boundary + docs | todo | | | |
| C1 default mid deck | todo | | | after both leaves merge |
| D1 gdash docs: managing views | todo | | | owner 2026-10-19 |
| E1 one color scheme across the sdk | todo | | | owner 2026-10-19; last |
| C2 live evidence | todo | | | |

**Lab facts (Task 0, 2026-10-19):**

- **Grafana:** `13.1.3`; dashboard schemaVersion 39; every target's datasource is `prometheus`, via the `$datasource` variable.
- **conductor:** `conductor-metrics`, "Conductor — metrics against goals". 17 panels: 3 row, 6 stat, 8 timeseries.
- **CI:** `ci-lab`, "CI — lab runners". 37 panels: 5 row, 16 stat, 10 timeseries, 4 table, 2 bargauge.
- **k3s-nodes and nodes (owner, 2026-10-19): use both.** `k3s-nodes` is the Nodes Overview, and Node Exporter becomes the extra view `nodes`. The spec amendment lists all four views.
  - `7d57716318ee0dddbac5a7f451fb7753` "Node Exporter / Nodes": 12 panels (4 row, 6 timeseries, 1 gauge, 1 table); vars `cluster`, `instance`.
  - `058020e04168bfdea0c52269cb699df2` "Kubernetes / Compute Resources / Nodes Overview": 5 timeseries; var `cluster`.
- **Not supported in v1:** `table` panels (4 in CI, 1 in Node Exporter) become placeholders under spec F3, so A5 must render the placeholder for them.

## 2. Feature → proof matrix (spec §5)

| Feature | Automated proof | Live proof | Notes |
| :-- | :-- | :-- | :-- |
| F1 fetch | [x] E1 E2 | [x] `gdash conductor` live (from wslgiga1; spark1 in C2) | |
| F2 vars | [x] E3 E4 | [ ] k3s-nodes `$node` panels draw | |
| F3 panels/layout | [x] E5 E6 | [x] live captures at 80 and 120 cols (`evidence/a6/live-*.txt`) | |
| F4 refresh/stale | [x] E7 E8 | [ ] Grafana unreachable → stale row | |
| F5 keys | [x] E9 | — | |
| F6 config/auth | [x] E10–E13 | [x] no secret in any output (E12, real binary) | |
| F7 doctor/list | [x] E14 E15 | [x] doctor passes live (wslgiga1); exit 1 with no config. spark1 in C2 | |
| F8 decks | [ ] E16–E19 E24 | [ ] one gdash process while cycling | |
| F9 nav | [ ] E20–E23 | [ ] capture of the deck cycle | |

## 3. Validation done-when — the stop condition

- [ ] gdash leaf merged (coverage ≥ 60%; auth/vars/render ≥ 80%; lint + security green)
- [ ] deck leaf merged (coverage ≥ 60%; E16 compat golden green)
- [ ] C1 default `mid` deck merged
- [ ] C2 evidence in `evidence/e2e/`
- [ ] `index.md` row → `done`

## 4. Blockers & escalations

| Date | Task | Blocker | Command + observed output | Resolution |
| :-- | :-- | :-- | :-- | :-- |

## 5. Session log (append-only)

| Date | Session | What advanced |
| :-- | :-- | :-- |
| 2026-10-18 | design session (claude-mono) | design #411; owner answered nav mode |
| 2026-10-18 | playground session | Grafana via LAN NodePort (Q2/Q3 closed); spec + plan + trio written; owner approved |
| 2026-10-19 | playground session | #411 merged; gdash worker created; Viewer token issued; Task 0 fixtures captured + scrubbed |
| 2026-10-19 | playground session | owner picked k3s-nodes = Nodes Overview and kept Node Exporter as a 4th view `nodes`; spec F8 amended |
| 2026-10-19 | playground session | A1–A4 done: scaffold, config + auth, client, dash + vars |
| 2026-10-19 | playground session | A5–A6 done: render (in-repo braille), app + `gdash <view>`; first live run against the lab, refId bug found and fixed |
| 2026-10-19 | playground session | A7–A8 done: doctor/list (live: all ok, --quiet 35 ms), E12 leak test on the real binary, install.sh + gff flag, docs; gdash leaf complete |
| 2026-10-19 | playground session | independent review of the gdash leaf: 5 findings (multi-value PromQL escaping, `$1` treated as a var, redact-before-truncate, password in URL, collapsed-row selection) fixed test-first; -race clean (`evidence/review/review.txt`) |
| 2026-10-19 | playground session | owner: zoom + one-pane middle (F10, E25–E27), gdash always uses the whole middle; tasks B7, D1, E1 added; deck worker created |
