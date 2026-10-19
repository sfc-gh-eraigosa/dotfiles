# mono-gdash — implementation plan

- **Slug:** mono-gdash
- **Date:** 2026-10-18
- **Status:** Approved (owner, 2026-10-18)
- **Relates to:** spec [`../specs/mono-gdash.md`](../specs/mono-gdash.md) · design [`../designs/mono-gdash.md`](../designs/mono-gdash.md) · issue [#410](https://github.com/sfc-gh-eraigosa/dotfiles/issues/410) · design PR [#411](https://github.com/sfc-gh-eraigosa/dotfiles/pull/411) · execution trio [`./mono-gdash/`](./mono-gdash/IMPLEMENTATION.md)

## 1. Summary & verdict

The build comes in two independent leaves and one integration step:

- **Leaf `gdash`:** the new `sdk/gdash` module. Its first task, Task 0, captures real fixtures from
  the lab's Grafana. Everything else in the leaf builds against them.
- **Leaf `deck`:** mono's deck panes and nav mode, in `sdk/mono`. It doesn't depend on gdash; the
  default layout only names `gdash …` as command strings.
- **Integration:** the default `mid` deck goes live on a lab host, plus the live evidence.

There are no open questions; the design's Q1–Q3 are closed. The design and spec were approved
by the owner on 2026-10-18.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/gdash/go.mod`, `main.go`, `build.sh` | module, entry, version via `../version.sh` ldflags | design §4.5 |
| `sdk/gdash/internal/version/version.go` | ldflags target | §4.5 |
| `sdk/gdash/cmd/{root,view,list,doctor,config,version}.go` (+ `_test.go`) | cobra CLI | spec F5–F7 |
| `sdk/gdash/internal/config/` | profiles, views, validation, XDG path, 0600/0700 writes | F6 |
| `sdk/gdash/internal/auth/` | `Secret`, file/env/cmd sources, basic auth | F6, E10–E13 |
| `sdk/gdash/internal/grafana/` | `Client` interface + HTTP impl; `testdata/lab/` fixtures | F1, E1–E2 |
| `sdk/gdash/internal/dash/` | dashboard JSON → panel model (types, rows, gridPos, targets) | F3 |
| `sdk/gdash/internal/vars/` | template-variable expansion | F2, E3–E4 |
| `sdk/gdash/internal/render/` | braille chart, meter, placeholder, status row, units; `testdata/golden/` | F3, E5–E6 |
| `sdk/gdash/internal/app/` | bubbletea model: layout, scroll, refresh, stale | F4–F5, E7–E9 |
| `sdk/gdash/security_test.go` | secret never printed, across every output path | E12 |
| `sdk/gdash/{AGENTS.md,CLAUDE.md→AGENTS.md,README.md}` | module docs; README demo from real output | §4.5 |
| `sdk/mono/internal/config/config.go` (+ `compat_test.go`) | `Tools`/`Views`, plain pane = 1×1 deck, byte-identical save | F8, E16 |
| `sdk/mono/internal/probe/` (+ `deck_test.go`) | drop per view → tool → pane | E17 |
| `sdk/mono/internal/app/{app,deck,nav,keys}.go` (+ tests) | deck runtime, nav mode, title label, `:focus` | F8–F9, E18–E23 |
| `sdk/mono/internal/configui/configui.go` | deck as an expandable row | F8 |
| `sdk/mono/cmd/{config,doctor}.go` | dotted paths, tool/view verbs, per-view doctor rows | F8 |
| `sdk/mono/boundary_test.go` | no import of `sdk/gdash` | E24 |
| `sdk/mono/{README.md,AGENTS.md}` | deck + nav docs; key table | E23 |
| `sdk/libs/tui/keymap/` | nav-mode bindings (only if a new action is truly needed; design says reuse PageLeft/PageRight) | F9 |
| **outside the modules** | | |
| `install.sh` | gdash build block behind `gff_on install.sdk.gdash`, after mono's | §4.5 |
| `.github/gff/features.yaml` | `install.sdk.gdash` (boolDefault true) | §4.5 |
| `scripts/test.sh` | `coverage_min`: `gdash 60`, `mono 60` | spec §6 |
| `sdk/AGENTS.md`, `sdk/README.md` | Modules row, "Pick your tool" row, README section | sdk "Adding a module" |
| `docs/mbo/index.md` | state moves | MBO |
| `docs/mbo/plans/mono-gdash/evidence/` | per-task captured output | §7 |

## 3. Interface contracts

```go
// sdk/gdash/internal/grafana
type Client interface {
    Health(ctx context.Context) (Health, error)                  // GET /api/health
    Whoami(ctx context.Context) error                           // auth check
    Search(ctx context.Context, q string) ([]Hit, error)        // GET /api/search?type=dash-db
    Dashboard(ctx context.Context, uid string) (dash.Raw, error) // GET /api/dashboards/uid/<uid>
    Query(ctx context.Context, r QueryRequest) (Frames, error)   // POST /api/ds/query
}
func NewHTTP(baseURL string, cred auth.Credential, timeout time.Duration) Client

// sdk/gdash/internal/auth
type Secret struct{ /* unexported */ }        // String/GoString/MarshalJSON → "‹redacted›"
type Credential interface{ Apply(*http.Request) }
func Load(a config.Auth) (Credential, error)  // exactly one of file|env|cmd; 0600 rule; cmd: no tty, 5s

// sdk/gdash/internal/render (pure)
func Chart(p dash.Panel, f grafana.Frames, w, h int, opt Opts) []string
func Meter(p dash.Panel, f grafana.Frames, w int, opt Opts) []string
func Placeholder(p dash.Panel, reason string, w int) []string
```

**CLI.**
- `gdash [view]` runs the TUI.
- `gdash list [query]` prints `uid\ttitle\tfolder`.
- `gdash doctor [--profile p] [--quiet]` has exit codes 0 (all checks pass) and 1 (any fails).
- `gdash config get|set|add|remove|path`.
- `gdash version`.

**mono config.**
- A pane has either `cmd` (plain) or `tools: [{name, probe?, views: [{name, cmd, every?, fallbacks?, probe?, cwd?}]}]`.
- New top-level keys: `nav_key` (default `"space"`) and `nav_esc_ms` (default `300`).

**mono deck pseudocode.**
```
switchView(pane, tool, view):
    if same → return
    pane.running.Stop(HUP, kill after 500ms)
    pane.running = start(view.resolvedCmd, pane.size)
    title.update()
```

## 4. TDD build order

Each task follows the same loop: write the test and watch it fail, implement, watch it pass, run
the gates, then capture evidence to `plans/mono-gdash/evidence/<task>/` and commit. Each task
lists the spec rules it proves.

### Leaf `gdash` (worker `gdash`, base `main`)

| Task | Write first (RED) | Implement | Done-when | Spec |
| :-- | :-- | :-- | :-- | :-- |
| **0 — lab facts + fixtures** | — (capture task) | Use the owner-issued Viewer token (`~/.config/gdash/lab.cred`, 0600) against `http://192.168.0.128:30300`. Record the Grafana version, the uids/titles of the conductor, k3s-nodes and CI dashboards, and their datasource types. Capture `/api/health`, `/api/search`, each dashboard JSON, and one `/api/ds/query` response per panel type into `internal/grafana/testdata/lab/`. Scrub tokens; leave only the documented NodePort address as a host | fixtures committed; `grep -rE '(glsa_|eyJ)' testdata` finds nothing; TRACKING records the three uids and the version | Q2 |
| A1 — scaffold | `cmd_test`: `gdash version` prints the stamped version | go.mod, main, build.sh, version, cobra root, AGENTS/CLAUDE/README stubs; `coverage_min gdash 60` | `bash sdk/gdash/build.sh` builds; tests pass; `git status --short -- sdk/gdash` shows files tracked | §4.5 |
| A2 — config + auth | E10, E11, E13, plus config validation tables | `internal/config`, `internal/auth`, `cmd/config.go` | listed tests pass; auth ≥ 80% coverage | F6 |
| A3 — client | E1, E2 against fixtures | `internal/grafana` HTTP impl, concurrency cap 4, timeouts | pass | F1 |
| A4 — dash + vars | E3, E4, plus a dash model built from each fixture | `internal/dash`, `internal/vars` | pass; vars ≥ 80% | F2–F3 |
| A5 — render | E5 (render level), E6 | charting choice recorded (ntcharts or in-repo); `internal/render` | golden frames reviewed and committed; render ≥ 80% | F3 |
| A6 — app | E7, E8, E9, E5 (app frames) | `internal/app`, `cmd/view.go` | pass; `gdash conductor` against fixture server renders | F4–F5 |
| A7 — doctor + list | E14, E15 | `cmd/doctor.go`, `cmd/list.go` | pass | F7 |
| A8 — security + install + docs | E12 | `security_test.go`; `install.sh` block; gff flag; sdk tables + README section with a real demo | `make security`, `make lint-go`, `make lint-markdown` green; `./install.sh` on a host builds gdash | §4.5, E12 |

### Leaf `deck` (worker `deck`, base `main`; parallel with `gdash`)

| Task | Write first (RED) | Implement | Done-when | Spec |
| :-- | :-- | :-- | :-- | :-- |
| B1 — config schema | E16, plus validation tables (cmd XOR tools, unique names) | `config.go` Tools/Views; 1×1 normalization; byte-identical save | pass; #407 golden unchanged | F8 |
| B2 — probe + doctor | E17 | `probe` per view/tool/pane; `mono doctor` rows | pass | F8 |
| B3 — deck runtime | E18, E19 | `app/deck.go`: visible view runs, title label, `:focus`, `r` re-probe | pass | F8 |
| B4 — nav mode | E20, E21, E22, E23 | `app/nav.go`, `keys.go` (stop stripping PageLeft/Right), help/README from table | pass | F9 |
| B5 — config verbs + UI | dotted-path verb tables; configui deck row frame test | `cmd/config.go`, `configui` | pass | F8 |
| B6 — boundary + docs | E24 | `boundary_test.go`; README/AGENTS deck + nav sections | pass; `make lint-go`, `make security` green | §3 |

### Integration (worker `deck`, after both leaves merge)

| Task | Do | Done-when |
| :-- | :-- | :-- |
| C1 — default `mid` deck | the default config gains the `mid` deck (`gpu` + `gdash` views, probe `gdash doctor --quiet`) | E16's compat golden still passes for user configs; the default-config golden is updated |
| C2 — live evidence | on spark1: `gdash doctor`; mono cycling the deck (`tmux capture-pane`); `ps` showing one gdash; on a host without credentials: `gdash doctor` exit 1 and mono's `mid` = GPU pane | captures in `evidence/e2e/`; TRACKING proof matrix all ticked |

## 5. Verification mapping

| Spec rule | Test | Task |
| :-- | :-- | :-- |
| E1, E2 | `grafana/client_test.go` | A3 |
| E3, E4 | `vars/vars_test.go` | A4 |
| E5 | `render/golden_test.go`, `app/frame_test.go` | A5, A6 |
| E6 | `render/units_test.go` | A5 |
| E7, E8, E9 | `app/stale_test.go`, `app/keys_test.go` | A6 |
| E10, E11, E13 | `auth/auth_test.go`, `cmd/config_test.go` | A2 |
| E12 | `security_test.go` | A8 |
| E14, E15 | `cmd/doctor_test.go` | A7 |
| E16 | `mono/internal/config/compat_test.go` | B1 |
| E17 | `probe/deck_test.go`, `app/deck_test.go` | B2 |
| E18, E19 | `app/deck_test.go`, `app/command_test.go` | B3 |
| E20–E23 | `app/nav_test.go`, `app/readme_test.go` | B4 |
| E24 | `mono/boundary_test.go` | B6 |

## 6. Integration & rollout

- **Order:** #411 (design + spec + plan + trio) merges first. Then the `gdash` and `deck` leaves run
  in parallel, each as its own PR, and integration C1–C2 runs once both have merged. Each PR is
  labelled `ready-for-merge` once its gates are green.
- **Install:** `install.sdk.gdash` defaults on. A host without gdash config simply drops the tool
  in mono.
- **Per-host setup** (README, not committed config): write the token file 0600, then
  `gdash config add profile lab --url http://192.168.0.128:30300 --auth-file …`, then
  `gdash list`, then `gdash config add view …` for the three dashboards.

### 6.1 Build leaves / DAG

| Leaf | Owns (paths) | Consumes (in-edges) | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| `gdash` | `sdk/gdash/**`, its `install.sh` block, the `install.sdk.gdash` flag, `scripts/test.sh` gdash row, sdk tables' gdash rows | — (Task 0 needs the owner's token) | Tasks 0, A1–A8 done-when; `go test ./...` ≥ 60%; lint + security green | no |
| `deck` | `sdk/mono/**` (except default layout until C1), `sdk/libs/tui/keymap` if touched, `scripts/test.sh` mono row | — | B1–B6 done-when; lint + security green | no |
| `integrate` | `sdk/mono` default config, `evidence/e2e/` | `gdash` (binary + `doctor --quiet` contract, spec F7), `deck` (deck schema, spec F8) | C1–C2 done-when | — |

```
gdash ──┐
        ├──► integrate
deck  ──┘
```

Shared files touched by both leaves are `scripts/test.sh` (one row each) and the
`sdk/AGENTS.md`/`sdk/README.md` tables (gdash only). `gss feature conflicts` must report
nothing beyond the one-line `coverage_min` rows. If it reports more, merge `deck` first.

## 7. Validation & evidence

- **Coverage:** ≥ 60% per module (`scripts/test.sh`), and ≥ 80% for `auth`, `vars` and `render`,
  checked in A2/A4/A5 with `go test -cover ./internal/<pkg>`.
- **Adversarial cases:** 401 from Grafana, Grafana down, a hanging Grafana, an unknown panel type,
  a library panel, a collapsed row, an unresolved variable, a NaN or null series, width 1,
  `auth.cmd` hanging, a token file at 0644, and the secret inside an error string.
- **Demo:** the README demos are pasted from real runs (sdk rule: "Demos must be real output").
- **Evidence protocol:** `docs/mbo/plans/mono-gdash/evidence/<task>/` holds each task's done-when
  command output, `tee`'d with a dated header and committed with that task. `evidence/e2e/`
  holds C2. A feature without captured evidence isn't done.

> Produced via `superpowers:writing-plans`. Execute with the trio in `./mono-gdash/`, TDD
> throughout. Update `../index.md` state as it moves.
