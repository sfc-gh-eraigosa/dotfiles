# gdash — Grafana dashboards as terminal graphs (`sdk/gdash`)

> **Context DAG:** [dotfiles](../../AGENTS.md) › [sdk](../AGENTS.md) › **gdash**

gdash draws a Grafana dashboard's panels in the terminal: braille line charts
for time series, htop-style meters for single values, a labelled placeholder
for anything else. It runs alone, and `mono` runs it as a pane (mono only
starts it; it never imports this module).

**Being built** under the `mono-gdash` objective:
[design](../../docs/mbo/designs/mono-gdash.md) ·
[spec](../../docs/mbo/specs/mono-gdash.md) ·
[plan](../../docs/mbo/plans/mono-gdash.md) ·
[ledger](../../docs/mbo/plans/mono-gdash/TRACKING.md).

| Path | What |
| :-- | :-- |
| `cmd/` | cobra CLI: `gdash [view]`, `gdash version` (more per plan A2–A7) |
| `internal/version` | build metadata stamped by `build.sh` |
| `internal/grafana/testdata/lab/` | scrubbed fixtures from the lab Grafana (plan Task 0); `capture.py` re-captures |

## Rules

- **Read-only against Grafana.** No writes, ever.
- **Credentials never print**: not in output, logs, argv or fixtures (spec F6, E12).
- **Tests run against the fixtures**, never a live Grafana.
- `bash build.sh` installs to `~/opt/bin/gdash`; `go test ./...` must stay ≥ 60% covered.
