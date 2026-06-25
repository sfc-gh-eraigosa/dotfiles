# sync-mcp — Design Spec

**Date:** 2026-06-24  
**Branch:** `feature/notebooklm-mcp/edward-raigosa/impl`  
**Status:** Approved  

---

## 1. Overview

`sync-mcp` is a Go CLI tool that replaces `opt/scripts/system/sync-mcp.sh`. It reads
`ai/mcp.yaml` and manages the full lifecycle of standalone MCP servers for Claude Code
and Gemini CLI: registration, status inspection, and removal.

**What it replaces:** `opt/scripts/system/sync-mcp.sh` — the yq-based shell script that
currently runs in `install.sh`. The shell script is retained as a deprecated fallback in
`v0.1.0` and deleted in `v0.2.0`.

**Why Go + Cobra + Bubbletea:**
- The shell script exercises subprocess hardening (setsid, timeout, `/dev/null` stdin
  guard) that is easier to write correctly and test in Go via `exec.CommandContext` +
  `cmd.WaitDelay` + `SysProcAttr.Setsid`, with a real fake-runner DI seam for unit tests.
- `status` and `remove` need interactive TUI components (table, multi-select checklist)
  that are straightforward in Bubbletea/Lipgloss but painful in shell.
- The `sdk/` module pattern (Cobra, `internal/version` ldflags, per-module `build.sh`,
  auto-discovered by the Makefile loop and `sdk-auto-bump.yml`) is already established by
  `gsl`, `gss`, `wol`, and `tmux-mgr`. Adding a sixth module has zero framework cost.
- Go is portable across Linux/macOS/Pi/Jetson with a single binary; the shell script
  depends on `yq`, `timeout`, `setsid`, and fragile CLI output string matching.

---

## 2. Package Layout

```
sdk/sync-mcp/
├── go.mod          # module github.com/sfc-gh-eraigosa/dotfiles/sdk/sync-mcp; go 1.26.3
├── go.sum
├── VERSION         # "0.1.0" — read by build.sh; consumed by sdk-auto-bump.yml
├── build.sh        # mirrors sdk/gsl/build.sh (goenv guard, ldflags, -o ~/opt/bin/sync-mcp)
├── main.go         # func main() { cmd.Execute() } — 3 lines
├── README.md
├── GEMINI.md
├── CLAUDE.md -> GEMINI.md
├── .gitignore      # sdk/sync-mcp/sync-mcp (the built binary)
│
├── cmd/
│   ├── root.go     # rootCmd; persistent flags: --manifest, --no-color
│   ├── sync.go     # sync-mcp sync [--dry-run]
│   ├── status.go   # sync-mcp status [--json]
│   ├── remove.go   # sync-mcp remove [<name>...] [--all] [--tool claude|gemini|all]
│   ├── version.go  # sync-mcp version
│   └── *_test.go
│
├── internal/
│   ├── version/            # ldflags target (mirrors sdk/gsl/internal/version)
│   │   └── version.go      # var Version, Commit, BuildDate, Dirty; func Get() Info
│   │
│   ├── manifest/           # pure leaf: parses ai/mcp.yaml, zero internal imports
│   │   ├── manifest.go     # Manifest, Server, ToolOpts; Load(path); DefaultPath()
│   │   ├── manifest_test.go
│   │   └── testdata/       # valid-complete.yaml, valid-minimal.yaml, invalid-*.yaml
│   │
│   ├── registry/           # Registrar interface + per-tool implementations
│   │   ├── registry.go     # Registrar interface; State enum; Action/ActionKind types
│   │   ├── claude.go       # reads ~/.claude.json .mcpServers; writes via atomic JSON merge
│   │   ├── gemini.go       # reads ~/.gemini/settings.json .mcpServers; writes via atomic JSON merge
│   │   ├── *_test.go
│   │   └── fake/
│   │       └── registrar.go  # in-memory Registrar; records Ensure/Remove calls; scriptable errors
│   │
│   ├── auth/               # AuthChecker interface + per-server strategies
│   │   ├── auth.go         # Checker interface; Status enum; Registry (map[name]Checker)
│   │   ├── notebooklm.go   # chromeProfileChecker: probes profile dir existence/non-empty
│   │   ├── noop.go         # noopChecker: returns NotApplicable for servers with no auth concept
│   │   ├── *_test.go
│   │   └── fake/
│   │       └── checker.go    # scripted Status per name
│   │
│   ├── exec/               # subprocess runner — the only seam for OS subprocess calls
│   │   ├── runner.go       # Runner interface; SystemRunner: exec.CommandContext + WaitDelay
│   │   ├── runner_linux.go # SysProcAttr{Setsid: true} (linux || darwin build constraint)
│   │   ├── runner_test.go
│   │   └── fake/
│   │       └── runner.go     # records calls; pops scripted responses (FIFO)
│   │
│   ├── status/             # orchestrator: manifest + registry + auth → Model
│   │   ├── status.go       # Reporter struct; Report(ctx) (Model, error)
│   │   ├── model.go        # Model, Row, Health enum; deriveHealth(Row) Health
│   │   ├── status_test.go
│   │   └── model_test.go   # truth-table tests for deriveHealth (15 cases)
│   │
│   ├── ui/                 # Bubbletea + Lipgloss — presentation only, no I/O
│   │   ├── theme.go        # Lipgloss color palette; Health → lipgloss.Color map
│   │   ├── statustable.go  # RenderTable(Model) string — Lipgloss render-once (not tea.Program)
│   │   ├── syncmodel.go    # tea.Model for sync spinner; emits Action tea.Msgs per server
│   │   ├── removeselect.go # tea.Model for remove multi-select checklist (Bubbletea list)
│   │   ├── json.go         # json.Encoder for status --json; whitelist fields (no env block)
│   │   └── *_test.go       # golden files in testdata/*.golden; lipgloss.SetColorProfile(Ascii)
│   │
│   └── paths/              # XDG/$HOME path resolution — pure, testable via env injection
│       ├── paths.go        # ClaudeJSONPath(), GeminiSettingsPath(), NotebookLMMCPDataDir()
│       └── paths_test.go
│
├── skill/
│   ├── SKILL.md
│   └── evals/evals.json
│
└── scripts/
    └── check-deps.sh       # seam gate: os/exec confined to internal/exec only
```

**Dependency graph (acyclic, leaf → root):**

```
manifest ──┬──► registry ──┐
           ├──► auth ───────┼──► status ──► ui ──► cmd ──► main
           ├──► exec ───────┘
           └──► paths
```

**Package boundary rule:** `cmd/` does flag parsing, presenter selection, and dependency
injection. `status` orchestration depends on `manifest`, `registry`, `auth`, `exec` — never
on `ui`. `ui` depends on `status.Model` value types only. All `internal/` packages are
headless-testable.

---

## 3. Key Interfaces

```go
// internal/manifest/manifest.go
type Manifest struct { Servers []Server `yaml:"servers"` }
type Server struct {
    Name        string    `yaml:"name"`
    Enabled     bool      `yaml:"enabled"`
    Transport   string    `yaml:"transport"`
    Command     string    `yaml:"command"`
    Args        []string  `yaml:"args"`
    Claude      *ToolOpts `yaml:"claude"`   // non-nil = register for Claude
    Gemini      *ToolOpts `yaml:"gemini"`   // non-nil = register for Gemini
    PostInstall []string  `yaml:"post_install"`
}
type ToolOpts struct { Env map[string]string `yaml:"env,omitempty"` }
type Tool string
const (ToolClaude Tool = "claude"; ToolGemini Tool = "gemini")

func (s Server) RegistersFor(t Tool) bool
func Load(path string) (Manifest, error)   // validates: transport==stdio, command non-empty, names unique
func DefaultPath() string                  // $DOTFILES_DIR/ai/mcp.yaml; SYNC_MCP_MANIFEST override
```

```go
// internal/registry/registry.go
type State int
const (NotRegistered State = iota; Registered; Drifted)

type ActionKind int
const (Created ActionKind = iota; Unchanged; Removed; Skipped; TimedOut; Failed; DryRun)

type Action struct {
    Server string
    Tool   manifest.Tool
    Kind   ActionKind
    Detail string
}

type Registrar interface {
    Tool()        manifest.Tool
    Available()   bool
    IsRegistered(s manifest.Server) (State, error)                          // reads config file; no subprocess
    Ensure(ctx context.Context, s manifest.Server, dryRun bool) (Action, error)
    Remove(ctx context.Context, s manifest.Server, dryRun bool) (Action, error)
}
```

```go
// internal/auth/auth.go
type Status int
const (Unknown Status = iota; Unauthenticated; Authenticated; NotApplicable)

type Checker interface { Check(s manifest.Server) (Status, error) }

type Registry struct{ byName map[string]Checker }
func NewRegistry() *Registry
func (r *Registry) Register(name string, c Checker)
func (r *Registry) Check(s manifest.Server) (Status, error) // unknown name → Unknown
```

```go
// internal/exec/runner.go
type Runner interface {
    Run(ctx context.Context, bin string, args ...string) (output string, exitCode int, err error)
}
```

```go
// internal/status/model.go
type Health int
const (HealthInactive Health = iota; HealthRegistered; HealthActive; HealthDisabled; HealthUnknown)

type Row struct {
    Name      string
    Enabled   bool
    Transport string
    Claude    registry.State
    Gemini    registry.State
    Auth      auth.Status
    Health    Health   // derived: Active = (Claude==Registered || Gemini==Registered) && Auth==Authenticated
}
type Model struct { Rows []Row; GeneratedAt time.Time }
```

---

## 4. Command Structure

```
sync-mcp
├── --manifest <path>   (default: $DOTFILES_DIR/ai/mcp.yaml, SYNC_MCP_MANIFEST override)
├── --no-color          (force-disable ANSI; also auto-disabled when stdout is not a TTY)
│
├── sync [--dry-run]
│     Register enabled servers for each declared tool block. Ensure-only, additive.
│     Runs post_install commands after registration. Per-server failures are non-fatal.
│     --dry-run: print planned actions, change nothing; previews even when CLIs absent.
│     Exit: 0 always (failures are warnings to stderr).
│
├── status [--json]
│     Read manifest + config files directly + probe auth state → table.
│     --json: machine-readable JSON to stdout; no ANSI, no Bubbletea.
│     Exit: 0 always (unregistered/unauthenticated is data, not error).
│
├── remove [<name>...] [--all] [--tool claude|gemini|all] [--dry-run]
│     Remove one or more servers from Claude/Gemini config files.
│     Modes (mutually exclusive):
│       sync-mcp remove                    # interactive Bubbletea multi-select (TTY only)
│       sync-mcp remove notebooklm         # remove named server(s) (one or more args)
│       sync-mcp remove --all              # remove all servers in the manifest
│     --tool claude|gemini|all (default: all) — scope removal to one tool's config
│     --dry-run: print what would be removed; change nothing
│     Exit: 0 always (not-registered is a no-op, not an error).
│
└── version
      Print Version/Commit/BuildDate/Dirty from internal/version.Get().
```

---

## 5. Status Table Design

**Columns:**

| Column  | Source                    | Values                    |
|---------|---------------------------|---------------------------|
| NAME    | `manifest.Server.Name`    | string                    |
| ENABLED | `manifest.Server.Enabled` | yes / no                  |
| CLAUDE  | `registry.State`          | registered / drifted / —  |
| GEMINI  | `registry.State`          | registered / drifted / —  |
| AUTH    | `auth.Status`             | ok / needed / n/a / ?     |
| STATUS  | `status.Health`           | color-coded rollup        |

**Health color coding (Lipgloss, gated on terminal detection):**

| Health             | Color           | Meaning                                      |
|--------------------|-----------------|----------------------------------------------|
| `HealthActive`     | green `#00d700` | Registered somewhere AND authenticated       |
| `HealthRegistered` | yellow `#ffff00`| Registered but auth needed (`setup_auth`)    |
| `HealthInactive`   | red `#ff5f00`   | Not registered (run `sync-mcp sync`)         |
| `HealthDisabled`   | dim/faint       | `enabled: false` in manifest — parked        |
| `HealthUnknown`    | grey `#808080`  | Cannot determine (no checker, or tool absent)|

**Rendering:**
- TTY: Lipgloss table (render-once, not a live `tea.Program`)
- Non-TTY / piped: plain aligned text (no ANSI)
- `--json`: JSON to stdout, no TUI

**`--json` output schema (stable; never emits the `env` block):**

```json
{
  "generated_at": "2026-06-24T12:00:00Z",
  "servers": [
    {
      "name": "notebooklm",
      "enabled": true,
      "transport": "stdio",
      "claude": "registered",
      "gemini": "not_registered",
      "auth": "authenticated",
      "health": "active"
    }
  ]
}
```

Field enum values — `claude`/`gemini`: `registered | drifted | not_registered`;
`auth`: `authenticated | unauthenticated | not_applicable | unknown`;
`health`: `active | registered | inactive | disabled | unknown`.

---

## 6. Sync Command Design

**Subprocess hardening** replaces the shell's `setsid + timeout + /dev/null` pattern:

```go
// internal/exec/runner_linux.go  (//go:build linux || darwin)
func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// internal/exec/runner.go
func (r *SystemRunner) Run(ctx context.Context, bin string, args ...string) (string, int, error) {
    cmd := exec.CommandContext(ctx, bin, args...)
    cmd.Stdin = nil                      // replaces </dev/null
    cmd.SysProcAttr = sysProcAttr()      // replaces setsid -w
    cmd.WaitDelay = 15 * time.Second     // SIGTERM → SIGKILL (replaces -k KILL_GRACE)
    // ...
}
```

Per-call context deadline from `SYNC_MCP_TIMEOUT` env (default 300s), set in `cmd/sync.go`.

**Registration writes** use atomic JSON merge directly on config files — no `claude mcp add`
subprocess. Idempotency is a map key-existence check, not CLI output string matching.
Writes use a temp-file-then-rename pattern to avoid corrupt config on interruption.

**Sync loop:**
```
for each enabled server:
    validate transport == "stdio" (else warn + skip)
    validate command != "" (else warn + skip)
    if claude available && server.RegistersFor(ToolClaude): claudeReg.Ensure(...)
    if gemini available && server.RegistersFor(ToolGemini): geminiReg.Ensure(...)
    for each post_install cmd: exec.Runner.Run(...) — warn on failure, never fatal
```

---

## 7. Remove Command Design

**Three invocation modes:**

### Interactive (default on TTY, no args, no --all)

`ui/removeselect.go` renders a Bubbletea multi-select checklist using
`github.com/charmbracelet/bubbles/list`. Each item shows the server name + current health
(color-coded from `status.Report()`) so the user sees what's actually registered.
Space toggles selection; Enter confirms; Escape/q aborts with no changes.

Non-TTY with no args and no `--all`: prints an error and exits 2
(`no server specified; use --all or pass server names, or run interactively on a TTY`).

### Named (`sync-mcp remove <name>...`)

One or more server names as positional arguments. Each name validated against the manifest
(unknown name → warn + skip, not fatal). Passes through `Registrar.Remove()`.

### `--all`

Iterates all servers in the manifest (enabled and disabled) and calls `Registrar.Remove()`
for each. Standard "wipe everything for a clean reinstall" path.

**`--tool` flag (default: all):**

Scopes removal to one tool's config. `--tool claude` only edits `~/.claude.json`;
`--tool gemini` only edits `~/.gemini/settings.json`; `--tool all` removes from both.

**`--dry-run`:** prints `DRY-RUN: remove <server> from <tool>` for each affected server.

**`Registrar.Remove()` — atomic key deletion:**

```go
func (r *ClaudeRegistrar) Remove(ctx context.Context, s manifest.Server, dryRun bool) (Action, error) {
    // read → delete key → atomic rename write
    // not-registered → Action{Kind: Unchanged} (not an error)
    // dryRun → Action{Kind: DryRun} (no write)
}
```

Not-registered is `Unchanged` (not an error): `sync-mcp remove --all` on a clean host
exits 0 silently.

---

## 8. Auth Detection

**Strategy registry** — extensible by server name:

```go
// cmd/root.go wiring
authReg := auth.NewRegistry()
authReg.Register("notebooklm", auth.NewNotebookLMChecker(paths.NotebookLMMCPDataDir()))
// future servers: authReg.Register("my-server", auth.NewMyServerChecker(...))
```

**NotebookLM platform paths (`internal/paths`):**

```go
func NotebookLMMCPDataDir() string {
    if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
        return filepath.Join(dir, "notebooklm-mcp")
    }
    switch runtime.GOOS {
    case "darwin":
        return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "notebooklm-mcp")
    default: // linux (Pi/Jetson/WSL)
        return filepath.Join(os.Getenv("HOME"), ".local", "share", "notebooklm-mcp")
    }
}
```

**Checker:** `chrome_profile/` must exist and be non-empty → `Authenticated`. Missing or
empty → `Unauthenticated`. Constructor takes a resolved `dataDir string` — fully testable
against `t.TempDir()`.

---

## 9. TTY / Non-TTY Handling

| Context                       | `status` output      | `sync` output      | `remove` behavior       |
|-------------------------------|----------------------|--------------------|-------------------------|
| TTY, no flags                 | Lipgloss table       | Bubbletea spinner  | Bubbletea multi-select  |
| TTY, `--json` / `--dry-run`   | JSON stdout          | Plain lines stderr | Plain lines stderr      |
| Non-TTY (piped, `install.sh`) | Plain text stdout    | Plain lines stderr | Error: specify servers  |
| CI / headless                 | Plain text stdout    | Plain lines stderr | Error: specify servers  |

`tea.WithOutput(os.Stderr)` on all `tea.NewProgram` calls — TUI never clobbers stdout.
TTY detection via `github.com/charmbracelet/x/term`.

---

## 10. Build and Install Wiring

**`sdk/sync-mcp/build.sh`** — mirrors `sdk/gsl/build.sh`. Key ldflags:
```
-X github.com/sfc-gh-eraigosa/dotfiles/sdk/sync-mcp/internal/version.Version=$VERSION
-X github.com/sfc-gh-eraigosa/dotfiles/sdk/sync-mcp/internal/version.Commit=$COMMIT
-X github.com/sfc-gh-eraigosa/dotfiles/sdk/sync-mcp/internal/version.BuildDate=$DATE
-X github.com/sfc-gh-eraigosa/dotfiles/sdk/sync-mcp/internal/version.Dirty=$DIRTY
```
Output: `~/opt/bin/sync-mcp`. Also links `skill/` to `~/.agents/skills/sync-mcp`.

**`.gitignore`** — add `sdk/sync-mcp/sync-mcp` alongside `sdk/tmux-mgr/tmux-mgr`.

**`install.sh`** — two edits:
1. Add build block after other SDK modules.
2. Replace `sync-mcp.sh` call with `sync-mcp sync` (with shell-script fallback for v0.1.0).

**Makefile / CI:** zero changes — `sdk/sync-mcp/` auto-discovered by all existing loops.

**`scripts/test.sh`:** add `[sync-mcp]=75` to `COVERAGE_MIN` map.

---

## 11. Test Strategy

**Target: ≥ 75% statement coverage, `go test -race ./...`**

Key test files and case counts:

| Package | File | Cases |
|---------|------|-------|
| `manifest` | `manifest_test.go` | 12 |
| `registry` | `claude_test.go`, `gemini_test.go` | 7 each |
| `auth` | `notebooklm_test.go` | 6 |
| `exec` | `runner_test.go` | 8 |
| `status` | `model_test.go` (truth table), `status_test.go` | 15 + 9 |
| `ui` | `json_test.go`, `statustable_test.go`, `removeselect_test.go` | 3 + 4 + 3 |
| `cmd` | `status_test.go`, `sync_test.go`, `remove_test.go` | 4 + 5 + 6 |

All tests use fake.Registrar, fake.Checker, fake.Runner — no real config files or CLIs
needed. Registry tests inject config paths via `CLAUDE_CONFIG_DIR` env var (existing gsl
pattern). Auth tests use `t.TempDir()`.

---

## 12. Migration Plan

**v0.1.0 (this PR):**
- `sdk/sync-mcp/` created: all packages, tests, `build.sh`, `skill/`.
- `install.sh`: adds build block + replaces `sync-mcp.sh` call with `sync-mcp sync` + fallback.
- `opt/scripts/system/sync-mcp.sh`: retained; header gains deprecation comment.
- `docs/ai-mcp.md`: updated to reference `sync-mcp` binary.

**v0.2.0 (follow-up PR):**
- `opt/scripts/system/sync-mcp.sh` deleted.
- Fallback branch in `install.sh` removed.
- Remaining `sync-mcp.sh` references in docs/CI cleaned up.

---

## 13. Out of Scope

- `sync-mcp auth` — interactive browser `setup_auth` (stays manual; security boundary unchanged)
- HTTP/SSE transport (stdio only; non-stdio entries warned and skipped)
- `sync-mcp add` — ad-hoc registration outside the manifest
- Server pruning on `sync` — use `remove` then `sync` to reinstall
- Windows native support — `SysProcAttr.Setsid` is linux/darwin only
- Plugin management — `ai/plugins.yaml` / `sync-plugins.sh` are a separate concern
- `sync-mcp` skill evals grading — corpus created; behavioral grading is a follow-up loop
