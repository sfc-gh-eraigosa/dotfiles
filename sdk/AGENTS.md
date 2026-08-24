# SDK — Go modules (`sdk/`)

One independent Go module per subdirectory, module path
`github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>` (the `sdk/` segment is part of
the canonical path — omitting it breaks `go install`), released on
path-prefixed `sdk/<tool>/vX.Y.Z` tags.

> **Go code lives here, not `src/`** (`src/` is non-Go tooling + agent skills) —
> [cutover plan](../docs/mbo/plans/2026-06-04-sdk-migration-plan.md).
>
> **[README.md](./README.md) is the user-facing tour** (per tool: problem, use
> case, demo). This file is the build + maintenance contract. Adding a module
> updates both — see [Adding a module](#adding-a-module).

## Modules

| Module | Binary | What it does |
| :----- | :----- | :----------- |
| [`gss/`](./gss/AGENTS.md) | `gss` | Git Safe Sync — backups, approval-gated pushes, stacked feature worktrees. |
| [`tmux-mgr/`](./tmux-mgr/AGENTS.md) | `tmux-mgr` | tmux session + parallel-agent orchestration (worktree-isolated). |
| [`gsl/`](./gsl/AGENTS.md) | `gsl` | Powerline status line for Claude Code / Antigravity CLI. |
| [`fleet/`](./fleet/AGENTS.md) | `fleet` | Multi-host install-drift status, TUI, wake ladder, SSH key management. |
| [`gff/`](./gff/AGENTS.md) | `gff` | git fast features — layered feature flags gating `install.sh`. |
| [`wol/`](./wol/AGENTS.md) | `wol` | Wake-on-LAN magic packets. |
| [`libs/`](./libs/AGENTS.md) | *(library)* | Shared Go packages — `log`. Not a CLI. |

## Adding a module

`fleet`, `gff`, and `libs` each shipped without being listed above; this
checklist exists so that stops recurring.

1. Module at `sdk/<tool>/` with the canonical path above.
2. `build.sh` sourcing [`version.sh`](./version.sh) (stamps version via
   `-ldflags -X`).
3. Log through [`libs/log`](#logging) — never hand-roll one.
4. `AGENTS.md` + a `CLAUDE.md -> AGENTS.md` symlink (`ln -s AGENTS.md CLAUDE.md`).
5. `README.md` — the module's deep docs.
6. Wire into `install.sh` (builds into `~/opt/bin/`).
7. Add a row to [Modules](#modules) **and** to README's "Pick your tool" table.
8. Add a [README.md](./README.md) section, matching the existing shape:
   blockquote pitch → **The problem** → **What it does about it** → **Reach for
   it when** (3–4 triggers) → `console` demo → **Gotchas** → docs footer link.
   Easiest step to skip, most costly to miss.
9. Verify tracking: `git status --short -- <path>`. `.gitignore` starts with
   `*`; `!sdk/**` opts this tree in, but confirm rather than assume
   ([allowlist](../docs/gitignore-allowlist.md)).

**Renaming/removing**: update both tables + the README section, and grep the old
name across `install.sh`, `Makefile`, `../README.md`.

**Demos must be real output.** Re-run the command and paste what it prints when
flags or output change. An invented transcript is worse than no demo — it fails
only for the reader who trusts it.

## Conventions

- **Versioning is tag-driven — no `VERSION` file.** `build.sh` derives it from
  `git describe --tags --match "sdk/<tool>/v*"`: clean release → `X.Y.Z`, dev →
  `X.Y.Z-<n>-g<sha>`. `.github/workflows/sdk-auto-bump.yml` plans the next
  semver (`opt/scripts/system/bump-sdk-version.sh --plan`) and pushes the tag
  directly — it never commits to `main`.
- **Test/lint discovery** is by directory under `sdk/` (`scripts/test.sh` + the
  `Makefile` Go loops); `src/` stays scanned until the cutover completes.

## Logging

**Every tool logs through [`libs/log`](./libs/AGENTS.md)** — no hand-rolled
logger, file writer, or rotation.

```go
import applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"

applog.SetDefaultTool("mytool")   // once, at startup
applog.Default().WithField("host", h).Info("started")
```

- **Diagnostics** (what the tool did) → `New` / `Default` — logrus JSON,
  lumberjack rotation, `$MYTOOL_LOG_FILE` / `$MYTOOL_LOG_LEVEL`.
- **Captured output** (bytes another process produced) → `NewCapture` — plain
  text per run; a captured install log's value is being readable as-is. The
  *lifecycle* is standardized, not the format.

Construction never fails: a logger that cannot open its file discards, and a nil
`*Capture` is safe to call. **Logging must never introduce a failure mode into
the thing it observes.**

gsl still uses its own `internal/observe`; migrate it to `libs/log` when next
touched.
