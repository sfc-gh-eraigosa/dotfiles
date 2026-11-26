# fleet-herdr-machines — live state ledger

## 0. Worker registry

| Worker | Branch | PR |
| :-- | :-- | :-- |
| `fleet-herdr-machines/<user>/sync-verb` | `feature/fleet-herdr-machines/<user>/sync-verb` | (pending) |

## 1. Task ledger

| Task | Status | Commit | Evidence |
| :-- | :-- | :-- | :-- |
| 1 herdrmach | done (uncommitted) | | `go test -cover ./internal/herdrmach` ok, 93.3% |
| 2 fleet herdr sync | done (uncommitted) | | `go test ./cmd -run Sync` 7/7 PASS |
| 3 S onto herdrmach | done (uncommitted) | | `go test ./cmd` ok 78.0% (3 pre-existing env failures skipped, see §4) |
| 4 docs + land | done (uncommitted) | | live dry-run below |

## 2. Feature → proof matrix (from spec §5)

| Feature | Proof | Status |
| :-- | :-- | :-- |
| F1 | `TestParseCapturedMachineList`, `TestParseRejectsNonArrayAndToleratesNewFields`, `TestListSurfacesRunnerErrors` | done |
| F2 | `TestSavedMatchesTargetNotLabel` | done |
| F3 | `TestAddArgvIsInertToMetacharacters` | done |
| F5 | `TestSyncDryRun`, `TestSyncWithoutTTYReportsNeedsApproval`, `TestSyncJSONShapeAndExplicitHosts` | done |
| F4, F6–F8 | later cut | deferred |

## 3. Validation done-when — the stop condition

- [x] `go vet ./...` clean; `go test` green for every package this cut touches (see §4 for pre-existing failures)
- [x] live `fleet herdr sync --dry-run` output recorded below

## 4. Blockers & escalations

- Pre-existing on `main` on the operator's macOS host (fail identically without this change):
  `TestGitRevParseRefusesADirectoryInsideAnotherRepo` (git commit exit 128 — empty
  `user.email`), `TestLoadPlanUsesBuiltInWhenNoFile`, `TestLoadPlanReadsTheConfiguredPath`
  (plan path resolves to `~/Library/Application Support`, not the test's XDG dir). Not in scope.
- `internal/runner` hit the 10-minute timeout once under a full parallel `./...` run; it passes
  in 3.4s on its own. Load-related, untouched by this cut.

## 5. Session log (append-only — never rewrite history)

- 2026-11-25 — plan + trio written; worker created.
- 2026-11-25 — tasks 1–4 built. Live on the operator's Mac (6 fleet hosts, this machine skipped):
  `--dry-run` → 5 `already`, 1 `would-add` (`herdr machine add pi2 --label pi2`), exit 0;
  same run with stdin from `/dev/null` → that host `needs-approval`, exit 1.
