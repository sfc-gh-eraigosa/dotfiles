# SDK — Go modules (`sdk/`)

This tree holds the repository's **Go modules**. Each subdirectory is an independent
module, `go install`-able by its canonical path
`github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>` and versioned with a path-prefixed
tag of the form `sdk/<tool>/vX.Y.Z`.

> **Go code lives here, not under `src/`.** `src/` is for non-Go tooling and agent
> skills. This relocation is the `src/` → `sdk/` cutover tracked in
> [docs/mbo/plans/2026-06-04-sdk-migration-plan.md](../docs/mbo/plans/2026-06-04-sdk-migration-plan.md).

## Modules

| Module | Path | Binary | Notes |
| :----- | :--- | :----- | :---- |
| [`gsl/`](./gsl/AGENTS.md) | `github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl` | `gsl` | Go status line for Claude Code / Gemini CLI. |
| [`gss/`](./gss/AGENTS.md) | `github.com/sfc-gh-eraigosa/dotfiles/sdk/gss` | `gss` | Git Safe Sync. |
| [`wol/`](./wol/AGENTS.md) | `github.com/sfc-gh-eraigosa/dotfiles/sdk/wol` | `wol` | Wake-on-LAN utility. |
| [`tmux-mgr/`](./tmux-mgr/AGENTS.md) | `github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr` | `tmux-mgr` | tmux session + agent orchestration. |

## Conventions

- **Module path = `github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>`** (canonical org **and** the `sdk/` segment). External install: `go install github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>@<tag>`.
- **Build** each module with its own `build.sh` (injects version via `-ldflags -X`); `install.sh` builds them into `~/opt/bin/`.
- **Versioning is tag-driven — there is no `VERSION` file.** `build.sh` sources [`sdk/version.sh`](./version.sh) and derives the version from `git describe --tags --match "sdk/<tool>/v*"`, so a clean release build stamps `X.Y.Z` and a dev build stamps `X.Y.Z-<n>-g<sha>`. Release tags are cut by `.github/workflows/sdk-auto-bump.yml`, which plans the next semver per module with `opt/scripts/system/bump-sdk-version.sh --plan` and pushes `sdk/<tool>/vX.Y.Z` directly — it never commits to `main`.
- **Test/lint discovery**: `scripts/test.sh` and the `Makefile` Go loops discover modules by directory under `sdk/` (the migration keeps `src/` scanned transitionally until the cutover completes).
- **Per-directory docs**: every module has a `AGENTS.md` + a `CLAUDE.md -> AGENTS.md` symlink.

## Logging

**Every tool logs through [`libs/log`](./libs/AGENTS.md).** Do not hand-roll a
logger, a file writer, or log rotation.

```go
import applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"

applog.SetDefaultTool("mytool")   // once, at startup
applog.Default().WithField("host", h).Info("started")
```

- **Diagnostics** (what the tool did) → `New` / `Default` — logrus JSON,
  lumberjack rotation, `$MYTOOL_LOG_FILE` / `$MYTOOL_LOG_LEVEL`.
- **Captured output** (bytes another process produced) → `NewCapture` — plain
  text per run, because a captured install log's value is being readable
  as-is; the *lifecycle* is what gets standardized, not the format.

Construction never fails: a logger that cannot open its file discards, and a
nil `*Capture` is safe to call. Logging must never introduce a failure mode
into the thing it observes.

gsl established the pattern in `internal/observe` and should migrate to
`libs/log` when it is next touched.
