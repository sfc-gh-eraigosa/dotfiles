# mono-gdash — spec

- **Slug:** mono-gdash
- **Date:** 2026-10-18
- **Status:** Approved (owner, 2026-10-18: "the plan already covers what we need and the spec adds execution detail")
- **Amended:** 2026-10-19 (owner): gdash gets **four** views, not three:
  - `k3s-nodes` is "Kubernetes / Compute Resources / Nodes Overview";
  - "Node Exporter / Nodes" joins as `nodes`;
  - both "are also good ones, use them".
- **Amended:** 2026-10-19 (owner): **the middle of the screen, zoom, and one-pane mode** (new feature F10).
  - Panes can be zoomed, and the middle can be configured as a single pane.
  - gdash always uses the whole middle.
  - The other tools share the middle and can be zoomed.
  - The title and the gsl footer never move.
- **Relates to:** design [`../designs/mono-gdash.md`](../designs/mono-gdash.md) · issue [#410](https://github.com/sfc-gh-eraigosa/dotfiles/issues/410) · PR [#411](https://github.com/sfc-gh-eraigosa/dotfiles/pull/411) · plan [`../plans/mono-gdash.md`](../plans/mono-gdash.md)

The design is the "why and shape". This spec turns each of its decisions into a testable rule.
Where the design left a detail to the builder, the spec fixes it here, marked **(spec)**. The
design wins on intent; this file wins on exact behaviour.

## 1. Goal

From any fleet host, `mono` shows the lab's **conductor**, **k3s nodes** and **CI** Grafana
dashboards as htop-style text graphs in its middle pane, next to the GPU monitor. You move
between them with vim keys in a sticky nav mode. `gdash`, the tool that draws them, also runs
on its own in any terminal. A host that can't reach Grafana sees today's mono, unchanged.

## 2. Use cases

| # | Actor / trigger | Flow | Acceptance |
| :-- | :-- | :-- | :-- |
| U1 | Owner runs `gdash conductor` on spark1 | gdash loads the `conductor` view, fetches the dashboard and its panels from `http://192.168.0.128:30300`, and draws them | Every Prometheus timeseries panel is a braille chart with a legend and the last value; stat panels are meters; anything else is a labelled placeholder. Refreshes every 30 s |
| U2 | Owner starts `mono` on a host with gdash configured | The `mid` deck shows `gpu`; `esc esc`, then `l`, switches to `gdash · conductor`; `j` goes to `k3s-nodes`, `nodes`, then `ci`, then past the last view into the next pane | The title bar reads `NAV` and `mid ◀ gdash ▶ · k3s-nodes 2/4`; only the visible view's process runs |
| U3 | mono on a host with no Grafana access (e.g. WSL off-LAN) or no gdash credentials | gdash's probe fails, so the gdash tool is dropped | The `mid` pane is today's GPU pane, or is dropped entirely where there's no GPU; `mono doctor` names why |
| U4 | Grafana goes away while gdash runs | The next refresh fails | The last good data stays on screen with its age and the error on a status row; it is never blank |
| U5 | Owner sets up a new host | `gdash config add profile lab --url … --auth-file ~/.config/gdash/lab.cred`, then `gdash list conductor`, then `gdash config add view conductor --dashboard <uid>` | `gdash doctor` passes every row; no secret is in any file gdash writes or anything it prints |
| U6 | An existing `~/.config/mono/config.json` from #407 | mono starts | It behaves exactly as before; plain panes are 1×1 decks internally |

## 3. Architecture

```text
sdk/gdash  (new, standalone; mono never imports it)
  cmd/            cobra: gdash [view], list, doctor, config, version
  internal/config profiles + views + validation; XDG path; $GDASH_CONFIG
  internal/auth   token|basic from file(0600)|env|cmd; Secret type that never prints
  internal/grafana Client interface: Search, Dashboard, Query, Health; HTTP impl
  internal/dash   dashboard JSON → panels (gridPos order, rows, types, targets)
  internal/vars   template variables + $__interval/$__rate_interval/$__range
  internal/render braille line chart · meter · placeholder · status row (pure: model → []string)
  internal/app    bubbletea model: layout, scroll, refresh ticker, stale handling
sdk/mono  (changed)
  internal/config Pane → Deck{Tools[]{Views[]}}; plain pane = 1×1 deck
  internal/probe  drop per view → tool → pane
  internal/app    deck runtime (visible view runs), nav mode, title label, :focus
  internal/app/keys.go  stop stripping PageLeft/PageRight; nav bindings in sdk/libs/tui
  internal/configui deck rows
```

- **The boundary:** mono runs `gdash <view>` and `gdash doctor --quiet` as commands. No import, no
  data passed. **(spec)** A test in `sdk/mono` fails if its `go.mod` or any import path names
  `sdk/gdash`.
- **Testability:** `grafana.Client` is an interface. Every gdash test runs against an `httptest`
  server serving the fixtures captured in plan Task 0. `render` is pure (model in, lines out),
  so golden files need no terminal. mono's deck and nav logic is tested through the existing
  terminal-free core (`HandleBytes`/`Frame`).

## 4. Behavior / features

### F1 — gdash fetch

- `GET /api/dashboards/uid/<uid>`. A title resolves through `GET /api/search?query=<title>&type=dash-db`;
  zero or more than one exact match is an error naming the candidates.
- `POST /api/ds/query`, one request per panel, with `from`/`to` from the view's range and
  `maxDataPoints = 2 × chart width in cells`. **(spec)** `intervalMs` = range ÷ maxDataPoints,
  rounded up to a whole second, and never below the datasource's `timeInterval` when it sets one.
- **(spec)** HTTP timeout: 10 s per request in the TUI; 2 s in `doctor --quiet`.
- **(spec)** Panels are queried at most 4 at a time.

### F2 — template variables

- A variable takes its value from the view's `vars`, else from the dashboard's `current`.
- `All` and multi-value expand Prometheus-style: `(a|b|c)` inside a regex matcher, else `.*` for `All` with no explicit list.
- `$__interval` = intervalMs as a duration (`15s`, `1m`); `$__rate_interval` = max(4 × scrape
  interval, `$__interval` + scrape interval), with the scrape interval from the datasource's
  `timeInterval`, default `15s`; `$__range` = the range (`1h`).
- **Forms:** `$var`, `${var}` and `[[var]]` are all expanded. **(spec)** `${var:format}` supports `regex`, `pipe`,
  `csv` and `raw`; any other format leaves the panel marked `needs var $var`.
- An unresolved variable marks that panel `needs var $x`. The other panels still draw.

### F3 — panels and layout

- **Types:** `timeseries`/`graph` → line chart. `stat`/`gauge`/`bargauge` → meter, one per series, using
  the last non-null value. Anything else, or a non-Prometheus datasource →
  `▢ <title> — <type> not supported`. A `row` panel is a header; a collapsed row hides its panels.
- **Library panels** (`libraryPanel`) → placeholder `library panel` in v1 **(spec)**.
- **Order:** gridPos `y`, then `x`. Two columns at ≥ 120 cells, else one.
  **(spec)** Panel height: charts take 8 rows (title + 6 plot rows + legend), meters take 1 row,
  placeholders take 1 row.
- **Units:** `percent`, `percentunit`, `bytes`/`decbytes`, `s`/`ms`, `short` and `none` are formatted
  (`42.1%`, `1.9 GiB`, `3.2 s`, `12.3k`). Unknown units print the raw number with 3 significant
  figures **(spec)**.
- **Thresholds:** a meter is colored by its `fieldConfig` thresholds (green/yellow/red). Without color
  (`NO_COLOR`), it uses ASCII markers instead **(spec)**.

### F4 — refresh and staleness

- **Ticks:** at the view's `refresh` (default `30s`, floor `5s`). A tick that finds the previous fetch
  still running is skipped and counted.
- **On error:** keep the last good frame and show `stale 2m14s · <error>` on the status row.
  A first fetch that fails shows the error with a retry countdown, not an empty screen.

### F5 — keys (gdash)

- `j`/`k`/`ctrl+d`/`ctrl+u`/`g`/`G` scroll, from the shared `sdk/libs/tui` keymap. `r` refreshes now.
  `+`/`-` step the range through `5m 15m 1h 3h 6h 12h 24h 7d`. `q`/`ctrl+c` quit.
- `?` shows the key help, rendered from the binding table.

### F6 — config and credentials

- **Config:** `${XDG_CONFIG_HOME:-~/.config}/gdash/config.json`; `$GDASH_CONFIG` overrides it. The
  file is written 0600 and its directory 0700.
- **Auth:** exactly one of `auth.file`, `auth.env`, `auth.cmd`; `auth.user` adds basic auth.
  Zero or several sources is a validation error naming the field.
  - **`auth.file`:** must be mode 0600 or stricter and owned by the user, else gdash refuses with
    `chmod 600 <path>` as the fix.
  - **`auth.cmd`:** run via `sh -c`, with stdin `/dev/null`, no controlling tty, a 5 s timeout and
    stdout trimmed. A non-zero exit or a timeout gives a named error. It runs once per process.
- **The `Secret` type:** `String()`, `GoString()`, `MarshalJSON` and `fmt` all print `‹redacted›`.
- **`gdash config get|set|add|remove|path`:** `set`/`add` refuse a value for any key named `token`,
  `password` or `secret`, and refuse an `auth.file` that isn't 0600.

### F7 — doctor and list

- **`gdash doctor [--profile p]`:** one line per check. The checks are: the URL answers `/api/health`;
  authentication works (`/api/user` or `/api/search?limit=1`); each view's dashboard is found;
  each referenced datasource answers a 1-point query. The exit code is 0 only if every check
  passes.
- **`gdash doctor --quiet`:** the first two checks only, with a 2 s timeout and no output. Exit 0 or 1.
- **`gdash list [query]`:** prints `uid<TAB>title<TAB>folder`, sorted by title.

### F8 — mono decks

- **Config:** a pane has either `cmd` (plain) or `tools` (deck), never both. Each tool has a `name`, an
  optional `probe` and one or more `views`. Each view has a `name`, and `cmd`, `every`,
  `fallbacks`, `probe` and `cwd` as today. **Names are unique** within their parent.
- **Plain panes:** loaded as a 1×1 deck with the tool and the view both named after the pane. They are
  saved back unchanged **(spec)**.
- **Dropping:** a view fails if its own probe fails or the tool's probe fails; a tool goes when
  it has no usable view; the pane goes when it has no usable tool. This is decided at startup
  and on `:reload` / `ctrl+b r` (re-probe the focused deck).
- **Runtime:** only the visible view's process runs. On a switch away, it is sent SIGHUP, then
  SIGKILL after 500 ms **(spec)**. On a switch back, it starts fresh at the pane's current size.
- **Title:** `<pane> ◀ <tool> ▶ · <view> i/n`, with arrows only where there's somewhere to
  move. Under `FLEET_ASCII_BORDERS=1` it is `< >` instead **(spec)**.
- **`:focus <pane>[/<tool>[/<view>]]`:** jumps there. An unknown name is a command-line error
  that lists the valid names.
- **`mono doctor`:** one row per pane/tool/view with `runs`, `fallback <cmd>` or `dropped: <reason>`.
- **`mono config` dotted paths:** `panes.<p>.tools.<t>.views.<v>.<field>`, plus
  `add|remove|move` at the tool and view levels, all mirroring today's verbs.
- **Default layout:** the panes are as follows.
  - **htop:** 60%, unchanged.
  - **`mid`:** 30%, a deck. Its tools are `gpu` (nvtop → nvitop) and `gdash`.
    `gdash` has `"zoom": true`, `probe: "gdash doctor --quiet"` and views
    `conductor`, `k3s-nodes`, `nodes`, `ci`.
  - **gsl:** 5%, `"zoomable": false`.
  - **`middle`:** `split`. `mono config set middle single` turns htop, gpu and
    gdash into one 90% deck instead. *(Amended 2026-10-20, owner: the default
    is now `single` with `mid` first and gdash its first tool, so the deck
    opens on gdash · conductor, then gpu, then htop.)* The lab
  dashboards behind the views (owner, 2026-10-19):

  | View | Dashboard uid | Title |
  | :-- | :-- | :-- |
  | `conductor` | `conductor-metrics` | Conductor — metrics against goals |
  | `k3s-nodes` | `058020e04168bfdea0c52269cb699df2` | Kubernetes / Compute Resources / Nodes Overview |
  | `nodes` | `7d57716318ee0dddbac5a7f451fb7753` | Node Exporter / Nodes (per node, `$instance`) |
  | `ci` | `ci-lab` | CI — lab runners |

  These uids go in the README's per-host setup walkthrough. They are not compiled in: views
  stay user config (design §4.5).

### F10 — the middle, zoom, and one-pane mode (amended 2026-10-19)

- **The middle** is every pane with `zoomable` unset or `true`. A pane with
  `"zoomable": false` (the gsl footer in the default layout) keeps its rows in
  every mode, and the title bar is never part of the middle.
- **Zoom:** `z` in nav mode, or `<prefix> z`, toggles the focused middle pane
  to take every middle row. The other middle panes get no rows and keep
  running unseen; toggling again restores the split. `<prefix> z` on a
  non-zoomable pane says so on the title bar.
- **Always-zoomed tools:** a deck tool with `"zoom": true` fills the middle
  whenever it is the tool on screen, and gives the rows back when you switch
  away. The default layout's `gdash` tool has it.
- **One-pane mode:** `"middle": "split"` (the default) or `"single"`
  (`mono config set middle single`). `single` merges every middle pane into
  one deck named `mid`. It sits where the first middle pane was, its size is
  the sum of theirs, and its tools are each pane's tools in order (a plain
  pane becomes one tool named after it). Non-middle panes are untouched.
- **Title:** `ZOOM` shows on the title bar while a pane is zoomed (manually or
  by its tool).

### F9 — nav mode

- **Enter:** `<prefix> <nav_key>` (default `space`), or `esc` twice within `nav_esc_ms`
  (default 300; `0` turns double-esc off). Both escapes are forwarded at once — mono never eats a
  key outside the prefix; the second also enters nav mode.
- **In nav mode:** `j`/`↓` and `k`/`↑` walk the deck's views, then carry on to the
  neighbouring panes. `h`/`←` and `l`/`→` switch tools within the deck. Nothing wraps.
- **Leave:** `esc` leaves nav mode. Any other key leaves it and is handled as if nav mode were
  off: text reaches the program and the prefix starts a command, so `esc esc :wq` in vim loses
  nothing. (Changed after the #414 review: the first cut dropped that key.)
- **Title:** the title bar shows `NAV`, and the focused pane's border or label is accented.
- **One-shot prefix keys** (`<prefix> j/k ? : r q`) are unchanged.
- **Key tables:** mono's `Keys()` keeps `PageLeft`/`PageRight` and re-labels them. The help
  overlay, title hint, `--help` and the README key table all render from one binding table.

## 5. Evaluation criteria (per feature) — each row is a test

| ID | Feature | Trigger | Must | Must not | Edge | Pass = test |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| E1 | F1 | view with uid | one dashboard GET, one ds/query per panel, maxDataPoints = 2×width | query more than 4 panels at once | width 1 → maxDataPoints 2 | `grafana/client_test.go TestFetchDashboardAndPanels` |
| E2 | F1 | title with 0 / 2 exact matches | error naming candidates | pick one silently | case differences | `TestResolveTitleAmbiguous` |
| E3 | F2 | `$node`, `${node:regex}`, `[[node]]`, All, multi | expand per §4 F2 | leave `$node` in a sent query | unknown format → needs var | `vars/vars_test.go` table |
| E4 | F2 | `$__rate_interval` 1h at 120 cols | value per formula | use Grafana's default when the ds sets timeInterval | ds without timeInterval → 15s | `TestRateInterval` |
| E5 | F3 | each fixture dashboard | golden frames at 80×24, 120×40, 1-col | panic or blank on unknown types | collapsed row, library panel | `render/golden_test.go` + `app/frame_test.go` |
| E6 | F3 | units list | formatted per §4 F3 | lose sign/NaN | NaN/null → `—` | `render/units_test.go` |
| E7 | F4 | fetch error after a good fetch | keep frame + `stale <age> · <err>` | blank the screen | first fetch fails → error + countdown | `app/stale_test.go` |
| E8 | F4 | tick while busy | skip + count | start a second fetch | refresh < 5s → 5s | `TestRefreshSkipIfBusy` |
| E9 | F5 | keys from table | scroll/range/refresh/quit | hand-rolled key strings | `+` at 7d stays 7d | `app/keys_test.go` |
| E10 | F6 | auth.file mode 0644 | refuse with `chmod 600` fix | read it | symlink to a 0600 file → resolve then check | `auth/auth_test.go` |
| E11 | F6 | auth.cmd that waits on stdin / sleeps 10s | named error in ≤ 5 s | prompt or hang | exit 0 with empty stdout → error | `TestCmdNoTTYTimeout` |
| E12 | F6 | any print path (`doctor`, errors, `config get`, `%v`, JSON, panic) | `‹redacted›` | the secret bytes anywhere in stdout/stderr | secret inside an error from net/http | `security_test.go TestSecretNeverPrinted` (scans captured output for the fixture secret) |
| E13 | F6 | `gdash config set …token …` | refuse | write it | `auth.env` name is allowed | `cmd/config_test.go` |
| E14 | F7 | doctor against fixture server: ok / 401 / down / missing dashboard | rows + exit codes per §4 F7 | exit 0 on any failure | `--quiet` ignores dashboards | `cmd/doctor_test.go` |
| E15 | F7 | `--quiet` against a server that hangs | exit 1 within 2.5 s | exceed mono's 3 s probe | — | `TestDoctorQuietTimeout` |
| E16 | F8 | #407 config file | identical behaviour + byte-identical save | require `tools` | — | `config/compat_test.go` (golden of the #407 default) |
| E17 | F8 | probes failing at view / tool / all | drop at the right level, rows reassigned | leave an empty pane | deck with one usable view | `probe/deck_test.go`, `app/deck_test.go` |
| E18 | F8 | switch view | old process HUP→KILL ≤ 500 ms, new starts at pane size | two views running | rapid switching | `app/deck_test.go TestOnlyVisibleViewRuns` (fake pane runner) |
| E19 | F8 | `:focus mid/gdash/ci`, bad name | jump / error listing names | crash | partial path `mid` | `app/command_test.go` |
| E20 | F9 | prefix+space; esc esc at 200 ms; at 400 ms; `nav_esc_ms:0` | enter / enter / no / no | delay the first esc | esc esc inside nav → stays out | `app/nav_test.go` (injected clock) |
| E21 | F9 | j at last view → next pane; k at first pane; h/l at ends | walk / stop / stop | wrap | plain pane j/k = panes; fixed (`zoomable: false`) panes are never a stop for nav j/k or `<prefix> j/k` (amended 2026-10-20) | `TestNavWalk`, `TestCycleSkipsFixedPanes` |
| E22 | F9 | `x` in nav mode | leave, not forwarded | reach the program | — | `TestNavOtherKeyLeaves` |
| E23 | F8/F9 | README key table vs binding table | identical | drift | — | `app/readme_test.go` (extends #407's) |
| E24 | §3 | mono imports | none from sdk/gdash | — | — | `sdk/mono/boundary_test.go` |
| E25 | F10 | `z` on a middle pane, then again | it takes every middle row; title and gsl unchanged; then the split returns | `z` on gsl: a message, no change | the title or a non-zoomable pane moves | `app/zoom_test.go TestZoomKeepsTitleAndFooter` |
| E26 | F10 | the deck switches to a `zoom: true` tool, then away | auto-zoom, then restore | manual zoom combined with an auto tool | the middle stays zoomed after switching away | `app/zoom_test.go TestToolZoom` |
| E27 | F10 | `middle: single` | one `mid` deck: tools htop, gpu, gdash; 90% | a layout with a single middle pane is left alone | the gsl footer is merged | `config/middle_test.go` |

## 6. Verification harness

- **Unit and golden tests:** `go test ./...` in `sdk/gdash` and `sdk/mono`. Golden files live under
  `testdata/golden/` and are refreshed with `-update`. The fixtures from plan Task 0 are under
  `sdk/gdash/internal/grafana/testdata/lab/`, scrubbed: no tokens, no hostnames except the
  documented NodePort address, and label values rewritten where they name people.
- **Coverage:** ≥ 60% per module, enforced by `coverage_min()` in `scripts/test.sh`, with rows
  added for `gdash` and `mono`. `render`, `vars` and `auth` are held to 80% by a package-level
  check in the plan.
- **Static checks:** `make lint-go`, `make security` (govulncheck and gosec) and
  `make lint-markdown` must all be green.
- **Live evidence** (human-observable, captured to `plans/mono-gdash/evidence/`):
  - `gdash doctor` from spark1 (passes) and from a host without credentials (fails, exit 1);
  - `tmux capture-pane` of mono on a lab host cycling `gpu → gdash conductor → k3s-nodes → nodes → ci`;
  - `ps` showing a single gdash process while cycling.

## 7. Prerequisites / dependencies

- **Grafana 13.1.3** at `http://192.168.0.128:30300`, which answers from the LAN with no tunnel.
- **A Viewer service-account token** for plan Task 0 and the live evidence, stored 0600 at
  `~/.config/gdash/lab.cred` on the capture host and never committed.
- **#407** is merged: mono's core, `sdk/libs/tui` keymap and probe.
- **Charting:** `NimbleMarkets/ntcharts` or an in-repo braille plotter. It must build with
  bubbletea v1 and pass `make security`; the decision is recorded in plan Task A5.

## 8. Out of scope (and why)

- **Editing dashboards, alerts, annotations, Grafana admin:** gdash is read-only.
- **Non-Prometheus datasources and library panels:** placeholders in v1, to keep the query
  layer small.
- **Side-by-side tools, mouse, view state remembered between mono runs:** design non-goals.
- **HTTPS vanity URL:** a config change once `grafana.theranch.test` resolves; no code needed.
- **`keep: true` (a hidden view keeps running):** allowed as a follow-up if a tool is slow to start.

## 9. Rollback

- **`gff set install.sdk.gdash false`:** gdash isn't installed, and mono drops the tool through its probe.
- **mono deck support:** reverts as one PR, and the default layout reverts with it. Plain-pane
  configs are unaffected either way (E16).

> Produced via `superpowers:brainstorming` (design) → this spec. The plan is
> `../plans/mono-gdash.md`. Registered in `../index.md`.
