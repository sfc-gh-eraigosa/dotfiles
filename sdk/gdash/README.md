# gdash — Grafana dashboards as terminal graphs

> Your Grafana dashboards, drawn in the terminal: braille line charts for time
> series, htop-style meters for single values, refreshed in place. Read-only.

gdash draws a dashboard's panels the way Grafana defines them: same queries,
same template variables, same units and thresholds. It runs on its own in any
terminal, and `mono` runs it as a pane (a host that can't reach Grafana simply
gets no gdash pane). Design, spec and plan:
[`docs/mbo/`](../../docs/mbo/plans/mono-gdash.md).

## Install

`./install.sh` builds it into `~/opt/bin/gdash` behind the gff flag
`install.sdk.gdash` (on by default). By hand: `bash sdk/gdash/build.sh`.

## Set up a host (once)

1. **A read-only token.** In Grafana: Administration → Service accounts → add
   one with the **Viewer** role → add a token. Save it to a file only you can
   read; gdash refuses a token file others can read:

   ```bash
   umask 077; cat > ~/.config/gdash/lab.cred     # paste the token, then ctrl+d
   ```

2. **A profile** — where Grafana is and where the token comes from (a 0600
   file, an environment variable, or a command that prints it):

   ```bash
   gdash config add profile lab --url http://192.168.0.128:30300 --auth-file ~/.config/gdash/lab.cred
   gdash doctor
   ```

3. **Find the dashboards**, then add a view per dashboard:

   ```console
   $ gdash list nodes
   058020e04168bfdea0c52269cb699df2   Kubernetes / Compute Resources / Nodes Overview   General
   7d57716318ee0dddbac5a7f451fb7753   Node Exporter / Nodes   General
   $ gdash config add view k3s-nodes --dashboard 058020e04168bfdea0c52269cb699df2
   $ gdash config set views.k3s-nodes.vars.cluster ""
   ```

   The lab's four views (mono's middle pane uses these names):

   | View | Dashboard uid | Variables to set |
   | :-- | :-- | :-- |
   | `conductor` | `conductor-metrics` | — |
   | `k3s-nodes` | `058020e04168bfdea0c52269cb699df2` | `cluster ""` |
   | `nodes` | `7d57716318ee0dddbac5a7f451fb7753` | `cluster ""`, `instance <node-ip>:9100` |
   | `ci` | `ci-lab` | — |

   A panel whose query uses a variable with no saved value says
   `needs var $x` with the exact `gdash config set` command to fix it.

The config (`~/.config/gdash/config.json`, or `$GDASH_CONFIG`) never holds a
credential; `gdash config set` refuses a key named like one.

## Use

```console
$ gdash doctor
  ok   reachable                http://192.168.0.128:30300 (Grafana 13.1.3)
  ok   authenticated            profile lab
  ok   view ci                  dashboard "CI — lab runners" (ci-lab)
  ok   view conductor           dashboard "Conductor — metrics against goals" (conductor-metrics)
  ok   view k3s-nodes           dashboard "Kubernetes / Compute Resources / Nodes Overview" (058020e04168bfdea0c52269cb699df2)
  ok   view nodes               dashboard "Node Exporter / Nodes" (7d57716318ee0dddbac5a7f451fb7753)
  ok   datasource Prometheus    answers a query
$ gdash k3s-nodes          # full screen; q quits
Node & Pod Count
50│⠉⠉⠉⠉⠉⠒⠒⠒⠒⠒⠒⠒⠒⠒⠤⠤⠤⠤⠤⠤⠤⠤⠤⢄⣀⣀⣀⣀⣀⣀⣀⣀⣀⠤⠤⠤⠤⠤⠤⠤⠤⠤⠒⠒⠒⠒⠒⠒⠒⠒⠒⠉⠉⠉⠉⠉
  │
  │
  │
  │
 4│⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀
● nodes 4  ● pods 50
```

`gdash doctor --quiet` checks only that Grafana answers and the token is
accepted, in under 2 seconds, and prints nothing — it is mono's probe.

## Managing views

A view is a name for one dashboard, plus how gdash shows it. Add as many as
you like; `gdash <view>` opens one, and mono's `mid` pane can list them too.

```bash
gdash list alert                          # find the uid
gdash config add view alerts --dashboard alertmanager-overview --range 6h --refresh 15s
gdash alerts
```

| To… | Run |
| :-- | :-- |
| change how far back it shows | `gdash config set views.alerts.range 24h` (or `+`/`-` while viewing). With none set, a view opens on the dashboard's own time range, else 1h |
| change how often it updates | `gdash config set views.alerts.refresh 1m` (never under `5s`) |
| set a dashboard variable | `gdash config set views.nodes.vars.instance 192.168.0.128:9100` |
| show only some panels | `gdash config set views.ci.panels "Active jobs,Pass rate (1h)"` |
| see one setting | `gdash config get views.alerts.range` |
| remove it | `gdash config remove view alerts` |
| put it in mono's middle pane | `mono config add-view mid gdash alerts -- gdash alerts` |

A panel that says `needs var $x` is waiting for a variable the dashboard
fills in by running a query; the panel prints the exact `gdash config set`
that fixes it. A titled panel that doesn't exist says `no panel with this
title on the dashboard`.

## Another host

The config (`~/.config/gdash/config.json`) holds no secret, so copy it as is;
copy the token file too and keep it private:

```bash
scp ~/.config/gdash/config.json <host>:~/.config/gdash/config.json
scp ~/.config/gdash/lab.cred <host>:~/.config/gdash/lab.cred
ssh <host> 'chmod 600 ~/.config/gdash/lab.cred && gdash doctor'
```

## Troubleshooting

`gdash doctor` says which step fails; `mono doctor` shows whether mono will
show gdash (its probe is `gdash doctor --quiet`).

| `gdash doctor` says | Cause | Fix |
| :-- | :-- | :-- |
| `FAIL reachable … connection refused` | wrong URL or port, or Grafana down | `gdash config get profiles.lab.url`; try it with `curl <url>/api/health` |
| `FAIL authenticated … HTTP 401 (the credential was refused): Invalid API key` | the token is wrong, expired or deleted | make a new Viewer token in Grafana and write it to the token file |
| `FAIL credential … is mode 0644, readable by others: chmod 600 …` | the token file is readable by others | run the `chmod 600` it prints |
| `FAIL view x … no dashboard matches` | the dashboard was renamed or deleted | `gdash list`, then `gdash config set views.x.dashboard <uid>` |
| `Error: no Grafana profile yet` | this host has no gdash config | see [Set up a host](#set-up-a-host-once) |
| `Error: no view named "x" (views: …)` | a typo, or the view is on another host | one of the names it lists |

In mono, a gdash view that is missing while the others show usually means
the probe failed when mono started; `ctrl+b r` on the `mid` pane checks again.

## Keys

| Key | Action |
| :-- | :-- |
| `r` | refresh now |
| `+` `=` / `-` | wider / narrower time range (5m … 7d) |
| `j` `down` / `k` `up` | scroll a line |
| `ctrl+d` / `ctrl+u` | half page down / up |
| `ctrl+f` `pgdown` / `ctrl+b` `pgup` | page down / up |
| `gg` / `G` | top / bottom |
| `?` `f1` | help |
| `q` `ctrl+c` | quit |

## What it draws

- `timeseries` and `graph` panels: a braille chart with min/max on a y-axis
  and each series' last value in the legend (`legendFormat` honoured). Series
  with `stacking.mode: normal` stack; a long legend wraps onto up to three
  lines rather than being cut.
- `stat`, `gauge`, `bargauge`: a meter, with a bar when the panel has a known
  maximum (percentages, or `max` set) and threshold colors — or `!`/`!!`
  markers under `NO_COLOR`. A `bargauge` with no maximum scales its bars to
  its largest value. `noValue` text replaces the `—` of an empty query.
- `table`: a row per label set, a column per label plus the value (`Value #A`,
  `Value #B`… with several queries). The `organize`, `groupBy` and `sortBy`
  transformations, the panel's `sortBy` option and per-column `unit`
  overrides apply; other transformations are skipped. Eight rows show, then
  `… N more`; in two columns a table too wide for its half takes the row.
- Anything else (library panels, non-Prometheus datasources, other panel
  types): one line saying what it is and that gdash doesn't draw it.
- Two columns at 120 cells and wider. Refresh every 30 s by default (`refresh`
  per view, never under 5 s); a fetch that fails keeps the last good data on
  screen with its age — stale, never blank.

## Gotchas

- Variables with no saved value (Grafana fills them by running a query) need a
  value per view: `gdash config set views.<view>.vars.<name> <value>`.
- The lab URL is plain HTTP, so the Viewer token crosses the LAN unencrypted.
  Point the profile at `https://grafana.theranch.test` once that name resolves.
- `auth.cmd` must answer without prompting (mono restarts gdash on every view
  switch); a logged-in password-manager CLI, a file or an env var all work.

→ [Agent context](./AGENTS.md)
