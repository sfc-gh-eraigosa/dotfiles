# SDK — Go modules (`sdk/`)

This tree holds the repository's **Go modules**. Each subdirectory is an independent
module, `go install`-able by its canonical path
`github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>` and versioned with a path-prefixed
tag of the form `sdk/<tool>/vX.Y.Z`.

> **Go code lives here, not under `src/`.** `src/` is for non-Go tooling and agent
> skills. This relocation is the `src/` → `sdk/` cutover tracked in
> [docs/mbo/plans/2026-06-04-sdk-migration-plan.md](../docs/mbo/plans/2026-06-04-sdk-migration-plan.md).

> **User-facing landing page: [README.md](./README.md).** That page sells each
> tool — problem, use case, demo. This file is the *contract* for building and
> maintaining them. Both must be updated when a module is added; see
> [Adding a module](#adding-a-module).

## Modules

Every module path is `github.com/sfc-gh-eraigosa/dotfiles/sdk/<dir>`, so the
table lists the binary and the job instead of repeating the prefix.

| Module | Binary | What it does |
| :----- | :----- | :----------- |
| [`gss/`](./gss/AGENTS.md) | `gss` | Git Safe Sync — backups, approval-gated pushes, stacked feature worktrees. |
| [`tmux-mgr/`](./tmux-mgr/AGENTS.md) | `tmux-mgr` | tmux session + parallel-agent orchestration (worktree-isolated). |
| [`gsl/`](./gsl/AGENTS.md) | `gsl` | Powerline status line for Claude Code / Antigravity CLI. |
| [`fleet/`](./fleet/AGENTS.md) | `fleet` | Multi-host install-drift status, TUI, wake ladder, SSH key management. |
| [`gff/`](./gff/AGENTS.md) | `gff` | git fast features — layered feature flags gating `install.sh`. |
| [`wol/`](./wol/AGENTS.md) | `wol` | Wake-on-LAN magic packets. |
| [`libs/`](./libs/AGENTS.md) | *(library)* | Shared Go packages — `log` (rotation, capture, XDG paths). Not a CLI. |

## Adding a module

The module table above had drifted — `fleet`, `gff`, and `libs` all shipped
without being listed. This checklist exists so that stops happening. Work it
top to bottom when adding a tool:

1. **Create the module** at `sdk/<tool>/` with module path
   `github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>` (the `sdk/` segment is part
   of the canonical path — omitting it breaks `go install`).
2. **Ship `build.sh`**, sourcing [`version.sh`](./version.sh) so the version is
   derived from the `sdk/<tool>/v*` tag rather than hardcoded.
3. **Log through [`libs/log`](./libs/AGENTS.md)** — never hand-roll a logger,
   file writer, or rotation. See [Logging](#logging).
4. **Write `AGENTS.md` + a `CLAUDE.md -> AGENTS.md` symlink**
   (`ln -s AGENTS.md CLAUDE.md`) in the module directory.
5. **Write `README.md`** in the module directory — the human-facing deep docs.
6. **Wire into `install.sh`** so a fresh clone builds it into `~/opt/bin/`.
7. **Add a row to the [Modules](#modules) table** above.
8. **Add a section to [README.md](./README.md)** — this is the step that is
   easiest to skip and most costly to miss. Match the existing shape:
   - a blockquote pitch (one sentence, what it is)
   - **The problem** — the concrete pain *without* the tool, written for
     someone who has felt it but never named it
   - **What it does about it** — how it resolves that specific pain
   - **Reach for it when** — 3–4 concrete trigger situations
   - a `console` demo with **real output** (run the command; don't invent a
     transcript)
   - **Gotchas** — what a new user trips on
   - a `→ Full docs / Agent context` footer link
9. **Add a row to the "Pick your tool" table** at the top of `README.md`.
10. **Check `git status --short -- <path>` on every new file.** `.gitignore`
    starts with `*`; `!sdk/**` opts this tree in, but verify rather than assume
    (see [docs/gitignore-allowlist.md](../docs/gitignore-allowlist.md)).

**Renaming or removing a tool?** Update the same two tables plus its README
section, and grep for the old name across `install.sh`, `Makefile`, and
`../README.md`.

**Demos must stay honest.** The README's value is that its transcripts are real.
When a command's output or flags change, re-run it and paste what it actually
prints — a plausible-looking invented transcript is worse than no demo, because
it fails only for the reader.

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
