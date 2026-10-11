# pins — version pin management — spec

- **Slug:** pins
- **Date:** 2026-10-10
- **Status:** Approved
- **Relates to:** design [`../designs/pins.md`](../designs/pins.md) · plan [`../plans/pins.md`](../plans/pins.md)

## 1. Goal
Each tool's version lives in one reviewed manifest. Bumping a pin upgrades every host, and
`fleet` shows which pins are behind upstream and which hosts have drifted from the pins. You can
choose pinned or latest per tool or per host.

## 2. Use cases

- **UC1: what's behind?** Operator runs `fleet pins status` and gets one row per tool: track, pin,
  latest upstream, gap (`major|minor|patch|current`), latest release date. Exit code is 0.
- **UC2: bump.** `fleet pins bump yq sops` (or `--all`) rewrites those pins to latest in the
  manifest (or in the `@file` it points to). It prints `old → new` per tool and changes nothing else. The operator then
  lands it with `/sync` or gss.
- **UC3: this host vs the pins.** `fleet pins status --installed` probes each tool locally and reports
  `ok | drift | missing | unknown` per row.
- **UC4: converge.** `fleet pins apply` runs the installer for each `drift` row (named tools may also be `missing`). Report-only rows are listed, not run. `--dry-run` prints the plan.
- **UC5: fleet view.** `fleet status` (table and `--json`) has a PINS column per host, e.g. `ok` or `3 drift`, or `-`
  when the host's fleet predates pins.
- **UC6: fleet rollout.** `fleet update --file opt/etc/fleet/pins.yaml <hosts>` syncs dotfiles,
  rebuilds fleet and runs `fleet pins apply` on each host, with no interactive install.sh.
- **UC7: float a host.** `gff set pins.track.mode latest` makes every installer on that host follow
  latest upstream. `pin` forces pins even on rows tracked `latest`. `manifest` (the default) honours each row.
- **UC8: weekly report.** A scheduled workflow upserts one issue ("pins: upstream drift report")
  with the behind rows, and closes it when nothing is behind.
- **UC9: existing hosts converge.** After a pin bump merges, `install.sh` upgrades sops, yq, gitleaks,
  kubectl, helm and kind on hosts that already have them. Today it skips them.

## 3. Architecture

```
opt/profiles/versions.tsv ──┬─► opt/lib/pins.sh (pins_want) ──► install_*.sh / install.sh go block
                            └─► sdk/fleet/internal/pins (Parse/Set/Extract/Compare)
                                    ├─ upstream.Resolver (gh:, go:, text:) — injectable HTTP
                                    ├─ cmd/pins.go: status | bump | apply   (Runner-injected exec)
                                    └─ cmd/status.go probe §3 ──► Row.Pins ──► table/JSON column
.github/workflows/pins-report.yml ─► fleet pins status --json ─► upsert issue
opt/etc/fleet/pins.yaml ─► fleet update --file … (sync → build fleet → fleet pins apply)
```

## 4. Behavior / features

- **F1 Manifest**: format per plan §3.1. Comments (`#`) and blank lines are preserved on rewrite. Unknown
  `track` and duplicate tool are parse errors that name the line.
- **F2 pins_want**: precedence `env X_VERSION` > `GFF_PINS_TRACK=latest` (→ `latest`) > manifest row
  (`track=latest` → `latest` unless `GFF_PINS_TRACK=pin`) > fallback. Output never has a leading `v`.
- **F3 Converging installers**: sops, yq, gitleaks and k8s tools (kubectl/helm/kind) compare installed
  vs want and reinstall on mismatch. `latest` resolves upstream first. `*_FORCE=1` is still honoured.
- **F4 Manifest-sourced defaults**: uv, glow, rust floor, keyd, nerd fonts, vault and Go read their
  default from `pins_want`. The hardcoded value stays as the fallback.
- **F5 `fleet pins status`**: upstream lookup per row (`gh:owner/repo` → latest release tag;
  `go:` → go.dev `?mode=json` first stable; `text:<url>` → body trimmed; `-` → `n/a`). Gap
  classification by semver. A lookup error marks only that row `error` and the command still exits 0.
- **F6 `--installed`**: run the row's probe (`sh -c`, 5 s timeout), extract the version and compare. `floor` is ok
  when installed ≥ pin. `latest` rows report the installed version with state `ok` (nothing to compare offline).
- **F7 `fleet pins bump`**: only `pin`/`floor` rows. Named tools, or `--all` = every row with gap ≠ current.
  `--dry-run`. Writes atomically (temp + rename).
- **F8 `fleet pins apply`**: runs `bash <repo>/<installer>` once per distinct installer, for rows in `drift`. A `missing` tool is installed only when named explicitly, because it may be absent on purpose (its gff flag is off). No `*_FORCE`: the converging installers (F3) act on the mismatch themselves. Report-only rows are listed, not run. The exit code is non-zero if any installer failed.
- **F9 fleet status PINS column + JSON `pins` object** `{ok, drift, missing}`, or absent.
- **F10 pins.yaml plan + install.sh chmod normalisation for it.**
- **F11 weekly pins-report workflow** (schedule + dispatch, `issues: write`, not a required check).
- **F12 gff flag `pins.track.mode`** (gff requires 3-part keys; exported as `GFF_PINS_TRACK_MODE`) (choice: manifest|latest|pin, default manifest).

## 5. Evaluation criteria

| Feature | Fires | Must not fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- |
| F1 | valid rows parse | comment/blank → no entry | `@.go-version` resolves to the file's content; dup tool → error | Go table tests |
| F2 | env set → env wins | no manifest → fallback | `v`-prefixed manifest value stripped | `opt/lib/pins_test.sh` |
| F3 | installed 3.12 / want 3.13 → reinstall | installed == want → exit 0, no download | want=latest resolves first | per-script `*_test.sh` with stub curl |
| F5 | newer upstream → minor/major/patch | equal → current | API error → row error, exit 0 | httptest-backed tests |
| F6 | probe 4.40 vs pin 4.44 → drift | probe absent → missing | floor 1.97 vs 1.96 → ok | fake exec tests |
| F7 | bump rewrites only the version field | latest rows untouched | `@file` rewrite | golden-file test |
| F8 | drift row → runs its installer once (deduped) | report-only row → listed, not run | installer fails → non-zero exit | fake runner tests |
| F9 | probe section parsed → column | old host (no section) → `-` | malformed JSON → `-` + note | status tests |
| F11 | workflow lints (actionlint) | — | — | `make lint` / actionlint |

## 6. Verification harness
`go test ./...` in `sdk/fleet` with the module coverage gate (≥60%, matching the sdk bar). The new `internal/pins` package
targets ≥85%. Shell tests are discovered by `make shell-test`. `make lint-shell lint-portability lint-go`.
Live check on this host: `fleet pins status`, `fleet pins status --installed`, `fleet status`.

## 7. Prerequisites
gff choice flags (exist: `fleet.update.config`). fleet `--file` plan support (exists).

## 8. Out of scope
See design §4.

## 9. Rollback
Revert the PR. Scripts fall back to their hardcoded defaults when the lib or manifest is missing,
and old fleet binaries ignore the extra probe section.
