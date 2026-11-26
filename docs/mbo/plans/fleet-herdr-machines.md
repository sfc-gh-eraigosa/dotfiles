# fleet-herdr-machines — implementation plan

- **Slug:** fleet-herdr-machines
- **Date:** 2026-11-25
- **Spec:** [`../specs/fleet-herdr-machines.md`](../specs/fleet-herdr-machines.md) · **Design:** [`../designs/fleet-herdr-machines.md`](../designs/fleet-herdr-machines.md)
- **Issue:** [#371](https://github.com/sfc-gh-eraigosa/dotfiles/issues/371) · **Worker:** `fleet-herdr-machines/<user>/sync-verb`

## 1. Summary & verdict

Build cut 1 of the approved spec: the `herdrmach` package (F1–F3) and the `fleet herdr sync`
verb (F5). UC1 already shipped as the `S` key (#392); this cut moves that key's private helpers
(`savedHerdrTargets`, `herdrMachineAddArgv`) onto `herdrmach` so there is one parser and one argv
builder. The `R` key (F4/F6), the herdr plugin (F7) and installer support (F8) stay in the spec
for a later cut. **Single PR, no breakout** — one Go module, two small units, one interface.

## 2. File inventory

| Path | Change |
| :-- | :-- |
| `sdk/fleet/internal/herdrmach/herdrmach.go` | new — `Machine`, `Runner`, `Parse`, `List`, `Saved`, `AddArgv` (spec §3, frozen) |
| `sdk/fleet/internal/herdrmach/herdrmach_test.go` | new — F1a–F1c, F2a, F3a |
| `sdk/fleet/internal/herdrmach/testdata/machine-list.json` | new — fixture, shape captured from herdr 0.9.5 (aliases genericised) |
| `sdk/fleet/cmd/herdr.go` | new — `fleet herdr sync [host...]` with `--dry-run`, `--json` |
| `sdk/fleet/cmd/herdr_test.go` | new — F5a–F5c + no-local-herdr + list-failure cases |
| `sdk/fleet/cmd/tui_cmds.go` | `S` helpers delegate to `herdrmach` (behaviour unchanged; existing `tui_herdr_test.go` is the guard) |
| `sdk/fleet/AGENTS.md`, `sdk/fleet/README.md` | document the verb |
| `docs/mbo/index.md` | row → `building` |

## 3. Interface contracts

`herdrmach` exactly as spec §3. One clarification: `AddArgv(alias)` returns the argv starting with
the literal `herdr`; callers that resolved a local binary path replace `argv[0]`. `List` calls the
runner with `herdr machine list --json` under the same rule.

`fleet herdr sync` result classes: `already | saved | needs-approval | failed`, plus `would-add`
under `--dry-run` only (dry-run never fails on it). `--json` emits `[{host,result,detail}]`.
Exit non-zero (`exitError{1}`) when any host is `needs-approval` or `failed`.

## 4. TDD build order

1. `herdrmach` — fixture + F1–F3 tests RED → implement → GREEN; coverage ≥ 60% (aim ~100%).
2. `cmd/herdr.go` — F5 tests against injected deps (hosts, local, herdr bin, runner, tty, add) RED → GREEN.
3. Refactor `S` helpers onto `herdrmach`; `go test ./...` stays green.
4. Docs + index; `go vet`.

## 5. Verification mapping

| Spec criterion | Test |
| :-- | :-- |
| F1a | `TestParseCapturedMachineList` |
| F1b | `TestParseRejectsNonArrayAndToleratesNewFields` |
| F1c | `TestListSurfacesRunnerErrors` |
| F2a | `TestSavedMatchesTargetNotLabel` |
| F3a | `TestAddArgvIsInertToMetacharacters` |
| F5a | `TestSyncDryRun` |
| F5b | `TestSyncWithoutTTYReportsNeedsApproval` |
| F5c | `TestSyncJSONShapeAndExplicitHosts` |

## 6. Integration & rollout

One worker PR off `main`. It references #371 without closing it (the issue stays open for
F4/F6–F8). Hosts pick it up via `fleet update` (fleet is rebuilt by `install.sh`).

## 7. Validation & evidence (show the work)

- `go test ./...` and `go vet ./...` in `sdk/fleet`, output in `fleet-herdr-machines/TRACKING.md`.
- Live: `fleet herdr sync --dry-run` on the operator's machine.
