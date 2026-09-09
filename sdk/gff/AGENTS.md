# sdk/gff — agent guide

`gff` (git fast features) is a generic git-persisted feature-flag engine:
proto-defined schema, layered resolver with provenance, cobra CLI, public Go
SDK. The dotfiles repo instruments `install.sh` with it (flags in
`.github/gff/features.yaml`, gate helper `opt/lib/gff.sh`).

## Layout

- `proto/gff/v1/features.proto` — the frozen schema (plan §3.1). Regenerate with
  `make gff-proto` (raw protoc + go.mod-pinned protoc-gen-go — **no buf**);
  output is **committed** under `gen/gff/v1/` and `make gff-proto-check` must
  stay clean.
- `internal/schema` — flag-file + override loading (protojson/yaml) and lint.
- `internal/paths` — the five well-known layer paths; every field overridable in tests.
- `internal/gitx` — repo-root discovery (`.git` dir OR file) + `gff.source`
  redirect; mockable `Runner` like gss.
- `internal/resolve` — the 5-layer merge with winning-layer attribution (≥95% cover).
- `internal/registry` — `~/.config/gff/sources.yaml` keyed by reverse-DNS
  namespace + user snapshots.
- `internal/tui` — the bubbletea TUI. Composes `sdk/libs/tui` (keymap/nav/search/cmdline/overlay);
  gff-only glue lives in `keys.go` (key table + palette), `search.go` (auto-expand, scope, anchors),
  `command.go` (`:set`/`:unset` validation → the override writers). The key table is the single
  source (pinned by `cmd/tui_keys_test.go`). Read `sdk/libs/tui/GUIDE.md` before changing keys.
- `cmd/` — one file per verb, self-registering via `init()`. Exit-code mapping
  lives ONLY in `main.go` (0 ok, 1 error, 2 unknown key/option/source/wrong-type).
- `pkg/gff` — the public SDK (`Bool`, `Selected`, `IsSelected`, typed values).
- `e2e/` — binary-level harness (build tag `e2e`), run via `make gff-e2e`.

## Rules

- Contracts in `docs/mbo/plans/gff.md` §3 are **frozen** — escalate, never edit.
- Coverage bars: `sdk/gff` overall ≥90%, `internal/resolve` ≥95%,
  `internal/schema` ≥90% (enforced by `.github/workflows/gff-ci.yml`).
- gff writes only `~/.config/gff/` + the XDG user snapshot dir. No test writes
  outside `t.TempDir()`; no test touches the network.
- The module root must stay `go run`-able (F11) — never move `main.go`.
