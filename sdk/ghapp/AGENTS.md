# sdk/ghapp — agent guide

`ghapp` is the GitHub App credential toolkit: create an App by the manifest
flow, keep its id + PEM under `~/.config/ghapp/` (0700 dir, 0600 PEM), sign
RS256 App JWTs, mint (and cache) installation tokens. `gcfg` consumes it via
`pkg/ghapp`; anyone else can `go run` it on its own.

Plan (contracts frozen): [`docs/mbo/plans/gcfg.md`](../../docs/mbo/plans/gcfg.md)
§3.2 (`App`, `Create`, `Token`, `Installations`, `Store`) and §4 P0.

## Layout

- `pkg/ghapp/` — the public library: `manifest.go` (create flow), `store.go`
  (PEM/app store), `jwt.go` (RS256), `token.go` (installation tokens, cached
  until expiry−2m), `installs.go`, `repo.go` (doctor probe), `browser.go`.
- `cmd/` — one file per verb; `NewRootCmd()` builds a fresh tree per call
  (`create · install · token · status · doctor · version`). Persistent flags
  `--config-dir`, `--app`, hidden `--api-url`/`--web-url` are the test seams,
  plus the `openBrowser` package var. Exit-code mapping lives ONLY in
  `main.go` (0 ok · 1 error · 2 usage / no credential = `cmd.ErrUsage`).
- `internal/version` — build metadata stamped by `build.sh` (tag-driven, no
  `VERSION` file; see `../version.sh`).

## Rules

- **Never print, store, or export a token value** — except `ghapp token`,
  whose whole stdout *is* the token (for `GH_TOKEN=$(ghapp token …)`). Tests
  mint the fixture token `ghs_FIXTURE_TOKEN_DO_NOT_PRINT` and assert it never
  appears anywhere else; CI greps the verbose test log and fails on a hit.
  Conversion webhook/client secrets are dropped, never stored.
- No network in tests: every GitHub call takes an injected base URL / client
  and is exercised against `httptest`.
- Writes only under `~/.config/ghapp/`; no test writes outside `t.TempDir()`.
- Coverage ≥80% module-wide (`.github/workflows/ghapp-ci.yml`).
- The module root must stay `go run`-able — never move `main.go`.
