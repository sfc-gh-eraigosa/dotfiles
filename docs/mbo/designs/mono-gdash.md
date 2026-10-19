# mono-gdash — Grafana dashboards in a mono pane, and panes that hold N tools — design

- **Slug:** mono-gdash
- **Date:** 2026-10-18
- **Status:** Draft
- **Relates to:** builds on [`mono`](./mono.md) ([#407](https://github.com/sfc-gh-eraigosa/dotfiles/pull/407)) · design issue [#410](https://github.com/sfc-gh-eraigosa/dotfiles/issues/410) · design PR [#411](https://github.com/sfc-gh-eraigosa/dotfiles/pull/411)
- **Author(s):** Edward Raigosa, Claude

Lines marked **(owner)** are the owner's decisions from the design conversation. Lines marked
**(my call)** are Claude's defaults; the owner can overturn any of them in review.
Everything else is left to the builder.

## 1. Problem / context

`mono` (`sdk/mono`, merged in #407) fills one terminal with stacked panes, each a real program
in its own pseudo-terminal: `htop-fit` 60%, a GPU monitor 30%, and `gsl status` 5%, refreshed
every second. Two things are missing:

1. **Metrics over time from the lab.** The lab already has dashboards for **conductor**, the
   **k3s nodes**, and **CI** in Grafana (`observability-grafana`, a ClusterIP service on port 80
   in jetson1's k3s; see [`fleet-connect-k8s`](./fleet-connect-k8s.md)). Today the owner has to
   open them in a browser. They want the same graphs as htop-style text, in a mono pane.
2. **A pane holds exactly one tool.** The owner wants the middle pane to hold N tools (the GPU
   monitor, the Grafana tool, and more later) and to move between them with vim directions
   without leaving mono.

Verified facts this design rests on:

- mono's contract is **"mono only runs tools"**. No pane gets data from mono, and tools stay
  standalone (`sdk/mono/AGENTS.md`). The Grafana renderer therefore has to be its own program.
- mono's keys all come after a prefix (`ctrl+b`). Today `ctrl+b j/k` move focus between panes
  and `h`/`l` are unused (`sdk/mono/internal/app/keys.go`). Every other key goes to the focused
  program.
- mono already drops a pane whose tool is missing or fails its `probe`, and gives its rows to
  the other panes (`internal/probe`, README "Which tool a pane gets").
- `sdk/libs/tui/keymap.Vim` already binds `h`/`l` and `←`/`→` to `PageLeft`/`PageRight`
  ("previous page / pane"); mono's `Keys()` currently strips them.
- mono's probe runs once at startup with a 3 s timeout (`internal/probe/probe.go`).

## 2. Goals, non-goals, anti-goals

**Goals**

1. **A new standalone tool, `gdash`** (my call on the name), in `sdk/gdash`. It draws Grafana
   dashboard panels as terminal graphs over time: braille line charts with an axis and the
   current value, and htop-style meters for single-value panels. It runs alone in any terminal,
   and mono runs it as a pane.
2. **Configurable Grafana URL and credentials (owner)**, so the same binary works on **any host
   that can reach Grafana on k3s**: through a port-forward, an ingress, or a node address.
3. **First content: the existing conductor, k3s nodes, and CI dashboards (owner)**, drawn from
   their Grafana definitions rather than hand-written queries.
4. **A mono pane can hold N tools, each with N views (owner).** Inside that pane:
   **left/right switch between tools**; **up/down switch between dashboards, or between
   configs, of the same tool**. The default layout makes the **middle pane** this kind of pane:
   tools `gpu` (nvtop/nvitop) and `gdash`, where gdash's views are the three dashboards.
5. **Configurable the mono way:** `mono config` verbs and the config screen cover the new
   shape; `gdash` gets its own `gdash config` verbs, following gsl and mono.

**Non-goals**

- Editing dashboards, alerting, annotations, or Grafana administration. gdash is read-only.
- Every Grafana panel type and datasource in v1. v1 handles **Prometheus-backed** time series
  and stat/gauge/bar-gauge panels (my call); anything else gets a labelled placeholder (§4.2).
- Side-by-side tools on screen at once. A multi-tool pane shows one tool at a time.
- Mouse support.

**Anti-goals (a failure even if it technically works)**

- **mono talking to Grafana**, or passing data to gdash. mono only starts gdash with arguments.
- **Credentials in mono's config, in this repo, or on a command line** that shows up in `ps`.
- **A blank pane.** A Grafana that can't be reached drops gdash from the pane (the same rule as
  nvtop on a GPU-less host), and a panel gdash can't draw says why.
- **A second keymap.** New keys go into the shared `sdk/libs/tui` table, and mono's help,
  title hint, `--help` and README render from it, as today.
- **Stealing keys from the program.** Outside the prefix every key still goes to the focused
  tool, so gdash's own scrolling and htop's `q`/`/`/F-keys keep working.

## 3. Options considered

1. **A separate `gdash` binary that mono runs in a pty, plus a "deck" pane in mono (chosen).**
   This keeps mono's contract, so gdash is useful without mono, and the deck is generic: any
   tool can be a column, not only Grafana.
2. **Built-in Grafana panes inside mono.** Fewer processes, but it breaks "mono only runs
   tools", couples mono to an HTTP client and credentials, and the graphs can't be used outside
   mono.
3. **`grafana-cli`, or rendering PNGs through the Grafana image renderer and showing them as
   sixel or kitty images.** Needs the renderer plugin on the cluster plus terminal image support
   (not available in Windows Terminal over SSH or inside a vt10x emulator). Rejected.

## 4. Decision

### 4.1 mono: deck panes (N tools × N views)

A pane is either today's single tool (`cmd` …) or a **deck**: an ordered list of **tools**
(columns), each with an ordered list of **views** (rows). One view is on screen at a time.

```
                 h ◀  tools  ▶ l
              ┌──────────┬──────────────────┐
         k ▲  │ gpu      │ gdash            │
   views      │  nvtop   │  conductor       │  ← each view is one command line
         j ▼  │          │  k3s-nodes       │
              │          │  ci              │
              └──────────┴──────────────────┘
```

Config shape (field names are the builder's to finalize; `cmd`-style panes stay valid, so
existing config files keep working: a plain pane is a 1×1 deck):

```json
{
  "name": "mid", "size": "30%",
  "tools": [
    { "name": "gpu",   "views": [ { "name": "nvtop", "cmd": "nvtop", "fallbacks": ["nvitop"] } ] },
    { "name": "gdash", "probe": "gdash doctor --quiet",
      "views": [
        { "name": "conductor", "cmd": "gdash conductor" },
        { "name": "k3s-nodes", "cmd": "gdash k3s-nodes" },
        { "name": "ci",        "cmd": "gdash ci" }
      ] }
  ]
}
```

- A view is today's pane spec without `name`/`size`: `cmd`, `every`, `fallbacks`, `probe`,
  `cwd`. A tool-level `probe` applies to all of its views.
- **Dropping is per view, then per tool, then per pane.** Unusable views are skipped, a tool with
  no usable view is skipped, and a deck with no usable tool is dropped and its rows go to the
  other panes, like today.
- **Which view runs (my call):** only the visible view's program runs. Switching away stops it
  (SIGHUP, then SIGKILL, as on quit) and switching back starts it fresh. That is simple and cheap
  for every tool in scope, since htop, nvtop and gdash all redraw from live data within a second.
  If a tool turns out slow to start, a per-tool `"keep": true` that leaves it running while
  hidden is an allowed follow-up, not v1.
- **Where you are:** the title bar's focused-pane label becomes `mid ◀ gdash ▶ · k3s-nodes 2/3`
  (arrows only when there is somewhere to go). The builder picks the exact glyphs, with an ASCII
  fallback under `FLEET_ASCII_BORDERS`-style conventions.
- **mono remembers nothing between runs (my call).** Each start shows the first usable tool and
  view. `:focus mid/gdash/ci` jumps to a pane, tool, or view by name.
- **Dropped stays dropped until asked (my call):** mono keeps its startup-only probe rule.
  `:reload` and `ctrl+b r` on a deck re-probe its tools, so gdash comes back once Grafana is
  reachable. There is no background retry.
- `mono doctor` gains a row per tool and view: what runs and why each was kept or dropped.
- `mono config` verbs extend with dotted paths (`panes.mid.tools.gdash.views.ci.cmd`) and
  `add`/`remove`/`move` at tool and view level. The config screen shows a deck as an expandable
  row. The builder settles exact verb syntax, mirroring today's.

### 4.2 Keys

Keys stay behind the prefix and come from `sdk/libs/tui`. **No new shared actions (my call):**
mono stops stripping `keymap.PageLeft`/`PageRight` (already `h`/`l`/`←`/`→` in `keymap.Vim`)
and re-labels them with mono's help text, the same way it re-labels `Up`/`Down` today. fleet
and gff are untouched.

**Nav mode (owner, 2026-10-18).** Instead of a new prefix key per move, mono gets a sticky
**nav mode**:

- **Enter:** `ctrl+b <nav key>` **or** `esc` pressed twice. The nav key is `space` by default
  (my call), configurable as `"nav_key"`.
- **Leave:** `esc` (owner: "esc cancels toggle"). Any non-nav key also leaves nav mode and is
  **not** forwarded, so a stray keystroke never reaches a program by surprise (my call).
- **Inside nav mode, plain vim keys move, with no prefix (owner: "vim jk is all you need"):**

| Key (in nav mode) | Action |
| :-- | :-- |
| `j` / `↓` | down: the next view of the focused deck's tool; past its last view (or on a plain pane), the next pane |
| `k` / `↑` | up: the previous view; before its first view (or on a plain pane), the previous pane |
| `h` / `←` | previous tool in the focused deck **(owner: left/right = tools)** |
| `l` / `→` | next tool in the focused deck |
| `esc` | leave nav mode |

My reading of the owner's answer, **to confirm in review:** j/k are the only vertical keys
needed, so they walk one line through the screen: down through the views of a deck, then on to
the next pane. Owner said "up/down = dashboards/configs of the same tool", so inside a deck
j/k change the view first. The title bar shows `NAV` while the mode is on, and the focused
pane is accented.

- The one-shot prefix keys stay as they are (`ctrl+b j/k` = pane focus, `?`, `:`, `r`, `q`), so
  muscle memory from #407 still works. Nav mode is the way to drive decks.
- **Double-`esc` cost (risk, my mitigation):** it overrides the anti-goal "`esc` always reaches
  the program" for the *second* of two quick escapes. Rule: the first `esc` is forwarded at
  once (htop/vim see it with no delay). A second `esc` within 300 ms (my call, configurable as
  `"nav_esc_ms"`; `0` disables double-esc entry) is swallowed and enters nav mode. Tools that
  need a fast double escape can turn it off.

Navigation stops at the ends; it does not wrap (my call). Keys pressed without the prefix,
including `h j k l`, go to the tool as always. Inside gdash they scroll its panels.

### 4.3 gdash: what it draws

- **Input:** a **view** from gdash's config, which names a Grafana **profile** and a
  **dashboard**, or ad hoc `gdash --profile lab --dashboard <uid|title>`.
- **Fetch:** `GET /api/dashboards/uid/<uid>` (`/api/search` resolves a title), then
  `POST /api/ds/query` per panel for its targets over the time range, with
  `maxDataPoints` = the chart's width in braille columns (2 per cell), so Grafana sends no more
  points than can be drawn.
- **Template variables:** use each variable's `current` value from the dashboard JSON, with
  overrides from the view config (`"vars": {"node": "jetson1"}`). Expand `$__interval`,
  `$__rate_interval` and `$__range` as Grafana does for the chosen width and range. A variable
  gdash can't resolve marks the panel `needs var $x`, not blank.
- **Panels (v1, my call):** `timeseries`/`graph` → braille line chart, one line per series,
  colored legend, y-axis min/max, and last value per series. `stat`/`gauge`/`bargauge` → an
  htop-style meter `CPU [||||||||     42.1%]` with unit formatting from `fieldConfig`. Other
  types, or non-Prometheus datasources, → a one-line placeholder: `▢ <title> — <type> not
  supported`. Collapsed Grafana rows stay collapsed.
- **Layout:** follow the dashboard's `gridPos` order (top-to-bottom, left-to-right) and pack
  panels into the terminal width: two columns when ≥ 120 cells, else one (my call). Panels that
  don't fit scroll with `j`/`k`/`ctrl+d`/`ctrl+u`/`g`/`G` (shared keymap). `r` refreshes now,
  `+`/`-` widen or narrow the time range, `q` quits.
- **Refresh:** the view's `refresh` (default `30s`, my call; never faster than `5s`). A fetch
  still in flight at the next tick is skipped. A failure keeps the last good data and shows its
  age plus the error on a status row: stale, never blank.
- **Rendering library:** the builder's choice. Candidates are `NimbleMarkets/ntcharts`
  (bubbletea-native braille charts) or a small in-repo braille plotter. It must work with
  bubbletea v1 (the `x/ansi` constraint in mono §8) and pass `make security`.

### 4.4 gdash: configuration and credentials

`${XDG_CONFIG_HOME:-~/.config}/gdash/config.json` (`$GDASH_CONFIG` overrides), JSON to match gsl
and mono (my call):

```json
{
  "profiles": {
    "lab": { "url": "http://localhost:3000", "auth": { "file": "~/.config/gdash/lab.cred" } }
  },
  "default_profile": "lab",
  "views": {
    "conductor": { "dashboard": "<uid or title>", "range": "1h", "refresh": "30s" },
    "k3s-nodes": { "dashboard": "<uid or title>", "vars": { "node": "All" } },
    "ci":        { "dashboard": "<uid or title>", "panels": ["<optional subset, by title>"] }
  }
}
```

- **`url` is whatever this host uses to reach Grafana (owner)**: a port-forward
  (`kubectl -n observability port-forward svc/observability-grafana 3000:80`, or fleet's `t`
  tunnel), an ingress name, or a NodePort. gdash doesn't start tunnels itself (my call; that is
  fleet's job).
- **Credentials (owner: configurable; source my call):** a Grafana **service-account token**
  (Viewer role), read from exactly one of: `auth.file` (must be mode 0600, else refuse with the
  fix), `auth.env` (the name of an environment variable), or `auth.cmd` (a command whose stdout is
  the secret, e.g. a LastPass or 1Password CLI lookup). Basic auth (`auth.user` + one of the same
  three sources for the password) is supported for Grafanas without service accounts.
- **`auth.cmd` runs once per gdash start (my call)**, non-interactively: no tty, stdin from
  `/dev/null`, 5 s timeout. It is cached only in gdash's memory, never on disk. Because mono
  restarts gdash on every view switch, the command must answer without prompting (e.g. a
  logged-in `lpass`). One that would prompt fails with a named error in the pane instead of
  asking for a master password inside it. A file or env source is the recommended setup for
  mono. Secrets are
  never written by `gdash config`, never logged, never put in argv, and redacted in `doctor`.
- **`gdash doctor [--profile p]`:** URL reachable → authenticated → each view's dashboard
  found → its datasources queryable, one line each, exit 0 only when everything passes.
- **mono's probe is `gdash doctor --quiet` (my call): reachability + authentication only**,
  with its own 2 s HTTP timeout so it fits inside mono's 3 s probe budget. A host that can't
  reach Grafana, or has no credentials, drops the gdash tool. A missing dashboard or a slow
  datasource does **not** drop it: that view runs and shows a named error
  (`dashboard "ci" not found — gdash list`), so working dashboards stay usable.
- **`gdash list [--profile p] [query]`** searches dashboards (`/api/search`) and prints uid and
  title. This is how the conductor, k3s nodes, and CI dashboard uids get into config.
- **`gdash config get|set|add|remove|path`** mirrors mono's verbs, so profiles and views can be
  managed without hand-editing JSON. `set` refuses to store an inline secret.

### 4.5 Module, install, docs

- `sdk/gdash` mirrors `sdk/mono`: cobra `cmd/`, `internal/` with a mockable Grafana client
  interface, `internal/version` via `build.sh` ldflags, `AGENTS.md` + `CLAUDE.md` symlink +
  README with a demo, ≥ 60% coverage gate. Follow the `sdk/AGENTS.md` "Adding a module"
  checklist.
- `install.sh` builds it behind gff `install.sdk.gdash` (default on), like `install.sdk.mono`.
- mono's default config gains the `mid` deck from §4.1. Hosts without gdash config or Grafana
  access see exactly today's GPU pane (or nothing, on WSL), because the gdash tool is dropped by
  its probe.
- The lab's gdash profile and view config are **per-host user config**, not checked in (they
  name internal hosts). The README documents the setup and a `gdash list` walkthrough.

## 5. Risks & blast radius

- **Nav mode's double-`esc`** can swallow an escape a tool wanted. Mitigation: first esc forwarded immediately, 300 ms window, `nav_esc_ms: 0` turns it off. Help overlay and README render from the one table.
- **Grafana's dashboard JSON varies by version** (`graph` vs `timeseries`, `targets` vs
  library panels). Mitigation: fixtures are captured from the lab's real dashboards, and
  unknown shapes become placeholders, not crashes.
- **Query load:** N panels × refresh on a Jetson-hosted Prometheus. Mitigations: maxDataPoints
  is capped to the width, the 5 s floor, skip-if-busy, and only the visible view runs (§4.1).
- **Credential leakage:** covered by the §4.4 rules, plus a test that `doctor`, errors, and
  `config get` never print the secret, and `privacy_guard` naming (no credential-looking keys
  followed by bare identifiers).
- **Blast radius:** a new binary, plus mono's config shape and keys. Plain-pane configs are
  unchanged by construction.

## 6. Rollback

- `gff set install.sdk.gdash false` stops installing gdash. mono then drops the gdash tool by
  probe.
- mono's deck support can revert as one PR. Config files that use `tools` would then fail
  validation with a field-named error, so the revert must also restore the default layout.

## 7. Evidence expectations

- **Unit + golden tests:** gdash against an `httptest` Grafana serving **fixtures captured from
  the lab's conductor, k3s nodes, and CI dashboards** (secrets and hostnames scrubbed). Golden
  frames for line chart, meter, placeholder, stale-data, and needs-var at 80×24, 120×40 and
  1-column widths. Variable expansion table tests.
- **mono deck tests** through the terminal-free core (`HandleBytes`/`Frame`): nav-mode entry (prefix+space, double-esc timing, `nav_esc_ms: 0`), leave on esc/other key, h/l/j/k
  movement including j/k walking views then panes, ends that don't wrap, view start/stop on switch, per-view/tool/pane dropping,
  backward-compatible plain panes, README key table matches the binding table.
- **Security:** a test proving secrets never reach stdout/stderr/argv. `make security` green
  with the new dependencies.
- **Real-machine capture:** `tmux capture-pane` of mono on a lab host showing the `mid` deck
  cycling gpu → gdash conductor → k3s-nodes → ci, plus `gdash doctor` output from one host that
  reaches Grafana and one that doesn't (the tool is dropped).

## 8. Open questions

| # | Question | Who | Default if unanswered |
| :-- | :-- | :-- | :-- |
| Q1 | ~~Pane-focus key~~ → **answered: nav mode** (§4.2); confirm the j/k walk-through reading | owner | as written in §4.2 |
| Q2 | Dashboard uids/titles for conductor, k3s nodes, CI, their datasources (assumed Prometheus), and the Grafana version | builder, via `gdash list` against the lab | **the plan's first task** port-forwards `observability-grafana` on jetson1 with an owner-issued Viewer service-account token, records the version, and captures the scrubbed fixtures every later task builds against; non-Prometheus panels become placeholders |
| Q3 | Which URL each fleet host uses for Grafana (port-forward vs ingress) | owner, per host | documented in README, not baked in |

> Produced via `superpowers:brainstorming`. Registered in `../index.md`. The spec goes in
> `../specs/mono-gdash.md` and the plan in `../plans/mono-gdash.md` after design approval.
