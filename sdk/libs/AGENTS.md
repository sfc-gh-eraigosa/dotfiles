# 📚 sdk/libs — shared packages for sdk tools

> **Up:** [`sdk/`](../AGENTS.md)

Code that more than one sdk tool needs. Its own Go module, so a tool opts in by
requiring it — nothing here is forced on anyone.

| Package | What it is |
|---------|-----------|
| [`log/`](./log) | **The logging standard.** logrus for structured diagnostics, lumberjack for rotation, `Capture` for raw captured output. |

## Using it from a tool

Each sdk tool is a separate module and there is no `go.work`, so a consumer
needs a `replace` — these are built from the repo by `build.sh`, never fetched
by version:

```
require github.com/sfc-gh-eraigosa/dotfiles/sdk/libs v0.0.0
replace github.com/sfc-gh-eraigosa/dotfiles/sdk/libs => ../libs
```

## Logging: use `libs/log`, do not hand-roll

gsl proved the shape (logrus + lumberjack in `internal/observe`); this package
generalizes it so every tool gets the same behaviour instead of a worse
fraction of it. fleet was writing install output with bare `fmt.Fprintf` and
its own pruning when this was extracted — that is the thing to stop doing.

```go
import fleetlog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"

fleetlog.SetDefaultTool("fleet")            // once, at startup
fleetlog.Default().WithField("host", h).Info("update started")
```

**Two writers, on purpose:**

- **Diagnostics** — what the tool did and why. `New` / `Default`. JSON,
  rotated, greppable.
- **Captured output** — bytes some *other* process produced (a remote
  install's stdout). `NewCapture`. Plain text, because its whole value is
  being readable as-is in `less`; JSON-wrapping it destroys the one thing it
  is for. What is standardized is the file's lifecycle — location, `0600`,
  header, per-line timestamps, retention.

**Construction is total.** Nothing returns an error for a logging problem; a
logger that cannot open its file writes to `io.Discard`, and `NewCapture`
returns a nil `*Capture` that is safe to call. A tool that dies because it
could not log is strictly worse than one that runs unlogged.

**Environment**, per tool: `$FLEET_LOG_FILE`, `$FLEET_LOG_LEVEL`
(hyphens become underscores: `tmux-mgr` → `$TMUX_MGR_LOG_FILE`).

## Adding a package here

It belongs here once a **second** tool needs it — not in anticipation. Until
then it lives in the tool's own `internal/`, where it can change freely.
