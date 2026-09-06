# sdk/gcfg — agent guide

`gcfg` keeps a repository's (or an org's) GitHub settings in
`.github/gcfg.yaml`: `export` what is live, `verify` the file against it (CI),
`plan`/`apply` the difference on purpose, edit it in a vim-style TUI, and
install two GitHub Actions that do the same in the repo.

Plan (contracts frozen): [`docs/mbo/plans/gcfg.md`](../../docs/mbo/plans/gcfg.md)
§3 — YAML shape (§3.1), `Client`/`Family`/engine signatures (§3.2), CLI (§3.3),
workflow templates (§3.4), exit codes (§3.5). Spec:
[`docs/mbo/specs/gcfg.md`](../../docs/mbo/specs/gcfg.md).

## Layout

- `internal/schema` — typed config, strict load (unknown key → error naming the
  path), lint, generated JSON Schema.
- `internal/gh` — the `Client` seam (`Do`/`Paginate`), the go-gh REST
  implementation, the recording fake, and the credential chain
  (`GH_TOKEN` → `GITHUB_TOKEN` → `gh` login → ghapp).
- `internal/family` — the `Family` interface + registry; one package per
  settings family (`general`, `security`, `rulesets`, …).
- `internal/engine` — export/verify/plan/apply over the families, ownership
  (`declared` vs `full`), and the re-read that turns an accepted-but-ignored
  write into a `not_honoured` finding.
- `internal/report` — tty/json/markdown renderers (goldens).
- `internal/tui`, `internal/style` — the vim-style editor.
- `internal/actions` — the two workflow templates and their install/uninstall.
- `cmd/` — one file per verb; `NewRootCmd()` builds a fresh tree per call.
  Exit-code mapping lives ONLY in `main.go`: 0 clean · 1 findings
  (`cmd.ErrFindings`, silent — the report already printed) · 2 usage / no
  credential / non-TTY apply without `--yes` (`cmd.ErrUsage`).

## Rules

- Contracts in plan §3 are **frozen** — a defect is a TRACKING blocker, never
  a silent patch.
- **No network in tests.** Every GitHub call goes through `gh.Client`; tests
  use the recording fake with `testdata/` fixtures, or `httptest`.
- **Never print, store, or export a token or secret value.** Secrets and
  webhooks are managed by name/presence only.
- Deletes only under `ownership: full`; `apply` always re-reads.
- Coverage: module ≥80%, `internal/engine` ≥90%, `internal/schema` ≥90%
  (`.github/workflows/gcfg-ci.yml`).
- Repo writes only under `.github/`, and only from `export`, `init`,
  `actions install`, or the TUI's `w`.
- The module root must stay `go run`-able — never move `main.go`.
