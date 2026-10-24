# gdash — Grafana dashboards as terminal graphs (`sdk/gdash`)

> **Context DAG:** [dotfiles](../../AGENTS.md) › [sdk](../AGENTS.md) › **gdash**

gdash draws a Grafana dashboard's panels in the terminal: braille line charts
for time series, htop-style meters for single values, text tables for table
panels, a labelled placeholder for anything else. It runs alone, and `mono` runs it as a pane (mono only
starts it; it never imports this module).

Built under the `mono-gdash` objective:
[design](../../docs/mbo/designs/mono-gdash.md) ·
[spec](../../docs/mbo/specs/mono-gdash.md) ·
[plan](../../docs/mbo/plans/mono-gdash.md) ·
[ledger](../../docs/mbo/plans/mono-gdash/TRACKING.md).

| Path | What |
| :-- | :-- |
| `cmd/` | cobra CLI: `gdash [view]` (and `--dashboard/--profile/--range/--refresh`), `config`, `doctor [--quiet]`, `list`, `version` |
| `internal/config` | profiles + views, validation, dotted get/set, 0600 save; refuses inline credentials |
| `internal/auth` | the credential from exactly one of file (0600) / env / cmd; `Secret` never prints; credentials redact themselves |
| `internal/grafana` | read-only client (health, search, dashboard, datasources, ds/query → series); `Resolve`; `FetchAll` (≤ 4 at once) |
| `internal/grafana/grafanatest` | a fake Grafana serving the lab fixtures; used by every test |
| `internal/grafana/testdata/lab/` | scrubbed fixtures from the lab Grafana (plan Task 0); `capture.py` re-captures |
| `internal/dash` | dashboard JSON → panels (chart / meter / table / row / placeholder), gridPos order, refIds assigned |
| `internal/vars` | template variables and `$__interval` family; datasource references |
| `internal/render` | pure drawing: braille charts, meters, tables, rows, placeholders, units; golden frames |
| `internal/app` | the bubbletea TUI: layout, scroll, refresh (skip while busy), stale-not-blank; the live fetcher |
| `security_test.go` | builds the binary and proves the credential never reaches stdout/stderr (E12) |

## Rules

- **Read-only against Grafana.** No writes, ever.
- **Credentials never print**: not in output, logs, argv or fixtures (spec F6, E12).
- **Tests run against the fixtures**, never a live Grafana.
- `bash build.sh` installs to `~/opt/bin/gdash`; `go test ./...` must stay ≥ 60% covered.
