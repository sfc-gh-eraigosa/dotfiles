# SDK — Go modules (`sdk/`)

This tree holds the repository's **Go modules**. Each subdirectory is an independent
module, `go install`-able by its canonical path
`github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>` and versioned with a path-prefixed
tag of the form `sdk/<tool>/vX.Y.Z`.

> **Go code lives here, not under `src/`.** `src/` is for non-Go tooling and agent
> skills. This relocation is the `src/` → `sdk/` cutover tracked in
> [docs/plans/2026-06-04-sdk-migration-plan.md](../docs/plans/2026-06-04-sdk-migration-plan.md).

## Modules

| Module | Path | Binary | Notes |
| :----- | :--- | :----- | :---- |
| [`gsl/`](./gsl/GEMINI.md) | `github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl` | `gsl` | Go status line for Claude Code / Gemini CLI. |
| `gss/` | `github.com/sfc-gh-eraigosa/dotfiles/sdk/gss` | `gss` | Git Safe Sync. *(migration pending)* |
| [`wol/`](./wol/GEMINI.md) | `github.com/sfc-gh-eraigosa/dotfiles/sdk/wol` | `wol` | Wake-on-LAN utility. |
| `tmux-mgr/` | `github.com/sfc-gh-eraigosa/dotfiles/sdk/tmux-mgr` | `tmux-mgr` | tmux session + agent orchestration. *(migration pending)* |

## Conventions

- **Module path = `github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>`** (canonical org **and** the `sdk/` segment). External install: `go install github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>@<tag>`.
- **Build** each module with its own `build.sh` (injects version via `-ldflags -X`); `install.sh` builds them into `~/opt/bin/`.
- **Test/lint discovery**: `scripts/test.sh` and the `Makefile` Go loops discover modules by directory under `sdk/` (the migration keeps `src/` scanned transitionally until the cutover completes).
- **Per-directory docs**: every module has a `GEMINI.md` + a `CLAUDE.md -> GEMINI.md` symlink.
