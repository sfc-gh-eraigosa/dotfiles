# gsl tmux Status Bar Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `--tmux` flag to `gsl status` that emits a single, tmux-interpolation-safe status line (minimal ANSI, no Powerline PUA glyphs, `#`-escaped) and document the `~/.tmux.conf` wiring in both `src/gsl/README.md` and `src/gsl/skill/SKILL.md`.

**Architecture:** A new boolean flag `--tmux` on `statusCmd` triggers a dedicated render path inside `runStatusLine` that forces the `ascii` glyph mode (via `forceASCII=true` on `style.ResolveConfig`), suppresses colored fill blocks (sets `Fill=false`), and passes the rendered string through a lightweight `EscapeTmux` helper that replaces bare `#` with `##` so tmux does not interpret them as format variables; the resulting single-line string is printed to stdout without a trailing newline so it composes cleanly with tmux's `status-right`/`status-left` format strings.

**Tech Stack:** Go, tmux

---

Closes #33.

---

## Background & Design Decisions

### Why `--tmux` on `status` and not a new subcommand?

`gsl status` is already the "no-payload, plain render" entry point. A flag is one symbol less for users to memorise and keeps the `~/.tmux.conf` snippet short (`gsl status --tmux`). A separate `gsl tmux` subcommand would be equivalent but adds a cobra entry and splits documentation. Flag wins.

### Why force `ascii` glyphs (not `emoji`)?

tmux evaluates its format string in a non-interactive sub-shell with no font context. Nerd-Font PUA codepoints and multi-byte emoji both render as garbled blocks or question marks in the status bar on most tmux versions and most default terminal fonts. ASCII is always safe. Users who want emoji can override with `gsl config style emoji` and omit `--tmux`, running `gsl status` in a loop with `tmux set-option` instead.

### Why no ANSI color sequences in tmux mode?

tmux uses its own color syntax (`#[fg=...]`) and measures segment widths on the raw string. Standard ANSI escape sequences (`\x1b[38;5;Nm`) confuse tmux's width accounting, causing misaligned right-side segments and display artifacts. The safe path is plain text. Power users who want color can pipe through `gsl status --tmux` and wrap segments in `#[fg=...]` in their tmux config themselves.

### `#` escaping

tmux interprets any `#` in a `status-right`/`status-left` format string as the start of a format variable (e.g., `#S` = session name, `#[...]` = style tag). A bare `#` followed by any character that tmux recognises will be silently replaced. The `EscapeTmux` helper doubles every `#` to `##` (tmux's literal-`#` escape) so git branch names like `feature/#123` and PR badges like `PR#42` survive interpolation.

### Style forced to `ascii`, `Fill=false`, `Separator="space"`

`forceASCII=true` in `style.ResolveConfig` already replaces the entire `Icons` map with `asciiIcons`. Additionally, `Fill` must be forced to `false` (no ANSI bg escapes) and `Separator` to `"space"` (no separator glyphs). These overrides are applied after `ResolveConfig` returns, so the user's configured style name still controls the theme color names (unused in tmux mode but harmless) and any user icon overrides in `styles.ascii` still apply.

### No trailing newline (`fmt.Print` not `fmt.Println`)

`set -g status-right "#(gsl status --tmux)"` — tmux captures stdout of the shell command and trims a single trailing newline automatically, so either works. However, using `fmt.Print` (no newline) makes the output predictable in tests and avoids a subtle double-newline if the user pipes the output elsewhere.

---

## Task Breakdown

### Task 1 — Failing test: `EscapeTmux` helper (new file in `internal/render`)

**File:** `src/gsl/internal/render/tmux.go` (create)
**Test file:** `src/gsl/internal/render/tmux_test.go` (create)

Write the test first. The test must fail (file does not exist yet).

**Test content:**

```go
package render

import "testing"

func TestEscapeTmux(t *testing.T) {
    cases := []struct {
        in   string
        want string
    }{
        {"no hashes here", "no hashes here"},
        {"PR#42", "PR##42"},
        {"feature/#123-fix", "feature/##123-fix"},
        {"##already doubled", "####already doubled"},
        {"#S session", "##S session"},
        {"plain text", "plain text"},
        {"", ""},
        {"#", "##"},
        {"a#b#c", "a##b##c"},
    }
    for _, tc := range cases {
        if got := EscapeTmux(tc.in); got != tc.want {
            t.Errorf("EscapeTmux(%q) = %q, want %q", tc.in, got, tc.want)
        }
    }
}
```

**Run (expected: FAIL — compile error):**

```sh
cd /path/to/src/gsl && go test ./internal/render/ -run TestEscapeTmux
```

Expected output: `cannot find package` or `undefined: EscapeTmux`

- [ ] Write `src/gsl/internal/render/tmux_test.go` with the test above.

---

### Task 2 — Implement `EscapeTmux`

**File:** `src/gsl/internal/render/tmux.go` (create)

```go
package render

import "strings"

// EscapeTmux replaces every bare '#' in s with '##' so the string is safe to
// embed in a tmux status-right / status-left format string without tmux
// interpreting '#X' sequences as format variables.
//
// tmux's literal-'#' escape is '##'; see tmux(1) §FORMATS.
func EscapeTmux(s string) string {
    return strings.ReplaceAll(s, "#", "##")
}
```

**Run (expected: PASS):**

```sh
cd /path/to/src/gsl && go test ./internal/render/ -run TestEscapeTmux -v
```

Expected output:
```
=== RUN   TestEscapeTmux
--- PASS: TestEscapeTmux (0.00s)
PASS
```

- [ ] Write `src/gsl/internal/render/tmux.go` with `EscapeTmux` as above.
- [ ] Confirm `go test ./internal/render/ -run TestEscapeTmux` passes.

**Commit:** `feat(gsl): add EscapeTmux helper for tmux-safe # escaping`

---

### Task 3 — Failing test: `RenderTmux` function in `internal/render`

This function wraps `Render` with the tmux-specific overrides (ascii glyphs, no fill, space separator) and applies `EscapeTmux`.

**File:** `src/gsl/internal/render/tmux_test.go` (append to existing file)

```go
func TestRenderTmux_ForcesAsciiAndNoFill(t *testing.T) {
    cfg := config.Default()
    cfg.Enabled = true
    // Use a powerline style (has fill:true, nerdfont glyphs) to prove overrides apply.
    st := style.Style{
        Separator: "powerline",
        Fill:      true,
        Glyphs:    "nerdfont",
        Icons: map[string]string{
            "sep_right": "", // Nerd Font PUA codepoint
            "ai":        "", // rocket
        },
        Theme: map[string]string{"ai": "cyan"},
    }
    segs := []Segment{
        &stubSegment{text: "hello", ok: true},
        &stubSegment{text: "world", ok: true},
    }
    ctx := context.Background()
    got := RenderTmux(ctx, cfg, st, segs)

    // Must not contain ANSI escape sequences.
    if strings.Contains(got, "\x1b[") {
        t.Errorf("RenderTmux: ANSI escape in output: %q", got)
    }
    // Must not contain Nerd Font PUA codepoints (U+E0B0 and U+F135).
    if strings.Contains(got, "") || strings.Contains(got, "") {
        t.Errorf("RenderTmux: Nerd Font glyph in output: %q", got)
    }
    // Must use space separator (powerline sep suppressed).
    if !strings.Contains(got, " ") {
        t.Errorf("RenderTmux: expected space separator, got: %q", got)
    }
    // Content must appear.
    if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
        t.Errorf("RenderTmux: missing content in %q", got)
    }
}

func TestRenderTmux_EscapesHash(t *testing.T) {
    cfg := config.Default()
    st := style.Style{Separator: "space", Fill: false, Glyphs: "ascii"}
    segs := []Segment{
        &stubSegment{text: "PR#42", ok: true},
    }
    ctx := context.Background()
    got := RenderTmux(ctx, cfg, st, segs)
    if !strings.Contains(got, "PR##42") {
        t.Errorf("RenderTmux: expected PR##42 after hash escape, got: %q", got)
    }
}

func TestRenderTmux_MasterOff_Empty(t *testing.T) {
    cfg := config.Default()
    cfg.Enabled = false
    st := style.Style{Separator: "space"}
    segs := []Segment{&stubSegment{text: "X", ok: true}}
    got := RenderTmux(context.Background(), cfg, st, segs)
    if got != "" {
        t.Errorf("RenderTmux master-off: want empty, got %q", got)
    }
}
```

Note: `TestRenderTmux_ForcesAsciiAndNoFill` uses `stubSegment` which is already defined in `render_test.go` (same package). The test imports needed are `context`, `strings`, `testing`, `config`, `style` — add them to the import block of `tmux_test.go`.

**Run (expected: FAIL — `RenderTmux undefined`):**

```sh
cd /path/to/src/gsl && go test ./internal/render/ -run 'TestRenderTmux' -v
```

- [ ] Append the three `TestRenderTmux_*` tests to `src/gsl/internal/render/tmux_test.go`.
- [ ] Add required imports to `tmux_test.go`: `"context"`, `"strings"`, `"testing"`, `"github.com/wenlock/dotfiles/gsl/internal/config"`, `"github.com/wenlock/dotfiles/gsl/internal/style"`.

---

### Task 4 — Implement `RenderTmux`

**File:** `src/gsl/internal/render/tmux.go` (append below `EscapeTmux`)

```go
// RenderTmux renders the status line in a mode safe for tmux status-right /
// status-left interpolation:
//
//   - Forces ascii glyph mode (no Nerd Font or emoji codepoints).
//   - Forces Fill=false (no ANSI background color escape sequences).
//   - Forces Separator="space" (no separator glyphs that confuse tmux width accounting).
//   - Applies EscapeTmux to double every '#' so tmux does not interpret the
//     output as a format variable.
//
// The returned string has no trailing newline. An empty string is returned when
// cfg.Enabled is false or all segments self-omit.
func RenderTmux(ctx context.Context, cfg config.Config, st style.Style, segs []Segment) string {
    // Override style for tmux safety.
    tmuxSt := st
    tmuxSt.Fill = false
    tmuxSt.Separator = "space"
    tmuxSt.Glyphs = "ascii"
    // Replace the entire icon map with the ASCII fallback table so no PUA
    // codepoints survive even if the caller's Icons had nerdfont entries.
    asciiMap := make(map[string]string, len(asciiIconsTable))
    for k, v := range asciiIconsTable {
        asciiMap[k] = v
    }
    // Preserve any user icon overrides that were already ASCII-safe (i.e. do
    // not contain non-ASCII bytes). This respects user customisations while
    // still guaranteeing tmux safety.
    for k, v := range st.Icons {
        if isASCIISafe(v) {
            asciiMap[k] = v
        }
    }
    tmuxSt.Icons = asciiMap

    line := Render(ctx, cfg, tmuxSt, segs)
    if line == "" {
        return ""
    }
    return EscapeTmux(line)
}

// asciiIconsTable is the package-level copy of the ASCII fallback icons so
// RenderTmux can clone it without importing the style package's unexported
// asciiIcons map. Values match style.asciiIcons exactly.
var asciiIconsTable = map[string]string{
    "dirgit":         "[dir]",
    "repo_root":      "[root]",
    "repo_worktree":  "[wt]",
    "worktree_count": "wt",
    "ai":             "[ai]",
    "mcp":            "[mcp]",
    "time":           "[time]",
    "branch":         "br:",
    "ahead":          "+",
    "behind":         "-",
    "staged":         "*",
    "unstaged":       "!",
    "untracked":      "?",
    "stash":          "$",
    "context":        "[ctx]",
    "sep_right":      "|",
    "sep_right_thin": ":",
}

// isASCIISafe returns true when every byte in s is a printable ASCII character
// (0x20–0x7E). This guards against accidentally propagating non-ASCII user icon
// overrides (emoji, Nerd Font PUA) into the tmux-safe icon set.
func isASCIISafe(s string) bool {
    for i := 0; i < len(s); i++ {
        b := s[i]
        if b < 0x20 || b > 0x7E {
            return false
        }
    }
    return true
}
```

Note: `RenderTmux` takes a `context.Context` — add `"context"` and `"github.com/wenlock/dotfiles/gsl/internal/config"` to the import block of `tmux.go`.

**Full import block for `tmux.go`:**

```go
import (
    "context"
    "strings"

    "github.com/wenlock/dotfiles/gsl/internal/config"
    "github.com/wenlock/dotfiles/gsl/internal/style"
)
```

**Run (expected: PASS):**

```sh
cd /path/to/src/gsl && go test ./internal/render/ -run 'TestEscapeTmux|TestRenderTmux' -v
```

Expected output: all four tests pass.

- [ ] Append `RenderTmux`, `asciiIconsTable`, and `isASCIISafe` to `src/gsl/internal/render/tmux.go`.
- [ ] Verify `go test ./internal/render/ -run 'TestEscapeTmux|TestRenderTmux' -v` passes.

**Commit:** `feat(gsl): add RenderTmux for tmux-safe ascii+no-fill rendering`

---

### Task 5 — Failing test: `--tmux` flag on `status` command

**File:** `src/gsl/cmd/status_test.go` (append)

```go
// TestStatusCmd_TmuxFlag verifies that --tmux produces a single line with no
// ANSI escape sequences and no unescaped '#' characters.
func TestStatusCmd_TmuxFlag(t *testing.T) {
    cfg := config.Default()
    // Use only the time segment so the test is hermetic (no git subprocess).
    cfg.Segments = []config.Segment{
        {Type: "time", Enabled: true},
    }
    withTempConfig(t, cfg, func() {
        out := captureStdout(t, func() {
            statusCmd.ResetFlags()
            statusCmd.Flags().Bool("tmux", false, "")
            if err := statusCmd.Flags().Set("tmux", "true"); err != nil {
                t.Fatalf("flag set: %v", err)
            }
            if err := runStatus(statusCmd, nil); err != nil {
                t.Errorf("runStatus --tmux: unexpected error: %v", err)
            }
        })
        line := strings.TrimRight(out, "\n")
        // Must be a single line.
        if strings.Contains(line, "\n") {
            t.Errorf("--tmux output must be a single line, got: %q", line)
        }
        // Must contain no ANSI escapes.
        if strings.Contains(line, "\x1b[") {
            t.Errorf("--tmux output must not contain ANSI escapes, got: %q", line)
        }
        // Must not contain a bare '#' (every '#' must be doubled).
        // We check by looking for a '#' not followed by another '#'.
        for i, ch := range line {
            if ch == '#' {
                next := i + 1
                if next >= len(line) || line[next] != '#' {
                    t.Errorf("--tmux output contains unescaped '#' at pos %d: %q", i, line)
                    break
                }
            }
        }
    })
}

// TestStatusCmd_TmuxFlag_MasterDisabled verifies that --tmux produces no output
// when the master switch is off.
func TestStatusCmd_TmuxFlag_MasterDisabled(t *testing.T) {
    cfg := config.Default()
    cfg.Enabled = false
    withTempConfig(t, cfg, func() {
        out := captureStdout(t, func() {
            statusCmd.ResetFlags()
            statusCmd.Flags().Bool("tmux", false, "")
            if err := statusCmd.Flags().Set("tmux", "true"); err != nil {
                t.Fatalf("flag set: %v", err)
            }
            if err := runStatus(statusCmd, nil); err != nil {
                t.Errorf("runStatus --tmux disabled: unexpected error: %v", err)
            }
        })
        if strings.TrimSpace(out) != "" {
            t.Errorf("expected empty output when master disabled + --tmux, got: %q", out)
        }
    })
}
```

**Run (expected: FAIL — `tmux` flag not registered):**

```sh
cd /path/to/src/gsl && go test ./cmd/ -run 'TestStatusCmd_Tmux' -v
```

- [ ] Append the two `TestStatusCmd_Tmux*` tests to `src/gsl/cmd/status_test.go`.
- [ ] Add `"strings"` to imports in `status_test.go` if not already present.

---

### Task 6 — Wire `--tmux` flag on `statusCmd`

**File:** `src/gsl/cmd/status.go` (full replacement)

```go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/wenlock/dotfiles/gsl/internal/payload"
)

var statusTmux bool

var statusCmd = &cobra.Command{
    Use:   "status",
    Short: "Render the status line for Gemini/CLI (no stdin payload)",
    Long: `status renders the status line without reading stdin. The AI segment
self-omits because no Claude payload is supplied. The dirgit, repo, and
time segments still render.

With --tmux the output is optimised for tmux status-right / status-left
interpolation: Nerd Font and emoji glyphs are replaced with ASCII equivalents,
ANSI colour escape sequences are suppressed, and '#' characters are doubled
('##') so tmux does not interpret them as format variables.

Example ~/.tmux.conf snippet:
  set -g status-right "#(gsl status --tmux)"`,
    RunE: runStatus,
}

func init() {
    statusCmd.Flags().BoolVar(&statusTmux, "tmux", false,
        "Emit tmux-safe output: ASCII glyphs, no ANSI escapes, '#' doubled")
    rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
    // Respect --tmux flag; pass it through to runStatusLine.
    return runStatusLine(cmd, payload.Payload{}, "", statusTmux)
}
```

Note: `runStatusLine` signature must accept a new `tmuxMode bool` parameter. That change is in the next task.

**Run (expected: FAIL — `runStatusLine` signature mismatch):**

```sh
cd /path/to/src/gsl && go build ./cmd/
```

- [ ] Replace `src/gsl/cmd/status.go` with the content above.

---

### Task 7 — Update `runStatusLine` to accept and honour `tmuxMode`

**File:** `src/gsl/cmd/statusline.go` (full replacement)

```go
package cmd

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/spf13/cobra"
    "github.com/wenlock/dotfiles/gsl/internal/config"
    "github.com/wenlock/dotfiles/gsl/internal/gh"
    "github.com/wenlock/dotfiles/gsl/internal/git"
    "github.com/wenlock/dotfiles/gsl/internal/mcp"
    "github.com/wenlock/dotfiles/gsl/internal/payload"
    "github.com/wenlock/dotfiles/gsl/internal/render"
    "github.com/wenlock/dotfiles/gsl/internal/repo"
    "github.com/wenlock/dotfiles/gsl/internal/style"
)

// runStatusLine is the shared wiring for both the render and status commands.
// It loads config, resolves the style, builds deps, runs BuildSegments+Render
// (or RenderTmux when tmuxMode is true), and prints the result.
//
//   - p:        the payload (populated from stdin by render; empty by status)
//   - cwdHint:  a preferred cwd string (render passes payload.Cwd; status passes "")
//   - tmuxMode: when true, output is piped through render.RenderTmux (ascii glyphs,
//               no ANSI escapes, '#' doubled). Print uses fmt.Print (no newline)
//               so the caller's tmux format string is not disrupted.
func runStatusLine(_ *cobra.Command, p payload.Payload, cwdHint string, tmuxMode bool) error {
    cfg, err := config.Load(config.DefaultPath())
    if err != nil {
        fmt.Fprintf(os.Stderr, "gsl: config load failed (using defaults): %v\n", err)
        cfg = config.Default()
    }

    if !cfg.Enabled {
        return nil
    }

    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    gitRunner := git.NewSystemRunner()
    ghRunner := gh.NewSystemRunner()
    mcpRunner := mcp.NewSystemRunner()

    cwd := cwdHint
    if cwd == "" {
        if wd, err := os.Getwd(); err == nil {
            cwd = wd
        }
    }

    branch := ""
    if cwd != "" {
        if info, err := git.Status(ctx, gitRunner, cwd); err == nil {
            branch = info.Branch
        }
    }

    rawStyles := configToRawStyles(cfg.Styles)
    // In tmux mode force ASCII glyphs so Nerd Font / emoji codepoints never
    // reach the tmux format string. Fill and separator overrides are applied
    // inside render.RenderTmux, not here, so the style still carries the user's
    // theme colors (unused in tmux mode but harmless).
    st := style.ResolveConfig(os.Stderr, cfg.Style, rawStyles, tmuxMode)

    deps := render.Deps{
        Payload:      p,
        Cwd:          cwd,
        Branch:       branch,
        RegistryPath: repo.DefaultRegistryPath(),
        Git:          gitRunner,
        GH:           ghRunner,
        MCP:          mcpRunner,
        MCPOpts:      mcp.ActiveCountOptions{},
        Clock:        time.Now,
    }

    segs := render.BuildSegments(cfg, deps)

    var line string
    if tmuxMode {
        line = render.RenderTmux(ctx, cfg, st, segs)
        if line != "" {
            fmt.Print(line) // no trailing newline — tmux trims it anyway
        }
    } else {
        line = render.Render(ctx, cfg, st, segs)
        if line != "" {
            fmt.Println(line)
        }
    }
    return nil
}
```

Also update `cmd/render.go` to pass the new `tmuxMode` argument (always `false` for `render`):

**File:** `src/gsl/cmd/render.go` — change the single call site:

```go
// was: return runStatusLine(cmd, p, cwdHint)
return runStatusLine(cmd, p, cwdHint, false)
```

**Run (expected: PASS for both existing and new tests):**

```sh
cd /path/to/src/gsl && go build ./... && go test ./cmd/ -run 'TestStatusCmd|TestRenderCmd' -v
```

- [ ] Replace `src/gsl/cmd/statusline.go` with the content above.
- [ ] In `src/gsl/cmd/render.go`, update the `runStatusLine` call to pass `false` as the fourth argument.
- [ ] Confirm `go build ./...` succeeds.
- [ ] Confirm `go test ./cmd/ -run 'TestStatusCmd_Tmux' -v` now passes.

**Commit:** `feat(gsl): wire --tmux flag on gsl status`

---

### Task 8 — Failing test: `isASCIISafe` unit test

**File:** `src/gsl/internal/render/tmux_test.go` (append)

```go
func TestIsASCIISafe(t *testing.T) {
    cases := []struct {
        in   string
        want bool
    }{
        {"hello", true},
        {"br:", true},
        {"+", true},
        {"[dir]", true},
        {" ", true},
        {"", true}, // empty is vacuously safe
        {"", false}, // Nerd Font PUA codepoint
        {"📁", false},     // emoji
        {"\x1b[0m", false}, // ANSI escape
        {"ab\x80c", false},  // non-ASCII byte
    }
    for _, tc := range cases {
        if got := isASCIISafe(tc.in); got != tc.want {
            t.Errorf("isASCIISafe(%q) = %v, want %v", tc.in, got, tc.want)
        }
    }
}
```

**Run (expected: PASS — `isASCIISafe` was already added in Task 4):**

```sh
cd /path/to/src/gsl && go test ./internal/render/ -run TestIsASCIISafe -v
```

- [ ] Append the `TestIsASCIISafe` test to `src/gsl/internal/render/tmux_test.go`.
- [ ] Confirm `go test ./internal/render/ -run TestIsASCIISafe -v` passes.

---

### Task 9 — Full test suite passes

**Run:**

```sh
cd /path/to/src/gsl && go test ./... -cover
```

Expected: all packages pass, no regressions. Coverage should be at or above the existing baseline for each package.

Also verify the os/exec seam check still passes (no new imports of `os/exec` outside the three allowed packages):

```sh
cd /path/to/src/gsl && bash scripts/check-deps.sh
```

- [ ] Confirm `go test ./... -cover` exits 0 with no failures.
- [ ] Confirm `bash scripts/check-deps.sh` exits 0.

**Commit:** `test(gsl): full suite green including tmux integration`

---

### Task 10 — Add `tmux` builtin style to `internal/style/builtins.go`

Although `--tmux` forces ascii+no-fill internally, users may also want to select a persistent `tmux` style via `gsl config style tmux` (e.g., for a shell prompt that mimics the tmux output). Adding a named `tmux` style also enables `gsl preview --once` to render a tmux-representative frame.

**File:** `src/gsl/internal/style/builtins.go` (append before `builtins` map)

```go
// tmuxStyle is a style optimised for use in tmux status-bar format strings:
// space separators, no fill (no ANSI background blocks), and ASCII glyphs so
// no Nerd Font or emoji codepoints appear in the tmux status bar.
//
// To activate persistently: gsl config style tmux
// Or use gsl status --tmux for a one-shot tmux-safe render without changing
// the stored style.
var tmuxStyle = Style{
    Separator: "space",
    Fill:      false,
    Glyphs:    "ascii",
    Icons:     nil, // asciiIcons applied automatically by Resolve when Glyphs=="ascii"
    Theme: map[string]string{
        "fg":            "default",
        "bg":            "default",
        "accent":        "default",
        "repo_root":     "default",
        "repo_worktree": "default",
        "ai":            "default",
        "dirgit":        "default",
        "time":          "default",
    },
}
```

Update the `builtins` map to include the new entry:

```go
var builtins = map[string]Style{
    "powerline": powerlineStyle,
    "emoji":     emojiStyle,
    "tmux":      tmuxStyle,
}
```

**Failing test first** — append to `src/gsl/internal/style/builtins.go`'s companion test file. Since there is no dedicated `builtins_test.go`, add to the nearest style test. Check for one first:

```sh
ls src/gsl/internal/style/
```

If there is no style `*_test.go` file, create `src/gsl/internal/style/style_test.go`:

```go
package style_test

import "testing"

func TestBuiltin_Tmux(t *testing.T) {
    s, ok := Builtin("tmux")
    if !ok {
        t.Fatal("Builtin(\"tmux\"): not found — tmuxStyle must be registered in builtins map")
    }
    if s.Glyphs != "ascii" {
        t.Errorf("tmux style: Glyphs = %q, want \"ascii\"", s.Glyphs)
    }
    if s.Fill {
        t.Errorf("tmux style: Fill = true, want false")
    }
    if s.Separator != "space" {
        t.Errorf("tmux style: Separator = %q, want \"space\"", s.Separator)
    }
}

func TestBuiltins_ContainsTmux(t *testing.T) {
    all := Builtins()
    if _, ok := all["tmux"]; !ok {
        t.Error("Builtins() map missing \"tmux\" entry")
    }
}
```

Note: the test uses `package style_test` (external test package) and imports `"github.com/wenlock/dotfiles/gsl/internal/style"` — adjust import accordingly.

**Run (expected: FAIL before Task 10 changes, PASS after):**

```sh
cd /path/to/src/gsl && go test ./internal/style/ -run 'TestBuiltin_Tmux|TestBuiltins_ContainsTmux' -v
```

- [ ] Check `ls src/gsl/internal/style/` for existing test files.
- [ ] Write `TestBuiltin_Tmux` and `TestBuiltins_ContainsTmux` in the appropriate test file.
- [ ] Run to confirm FAIL.
- [ ] Add `tmuxStyle` var and register it in the `builtins` map in `src/gsl/internal/style/builtins.go`.
- [ ] Run to confirm PASS.

**Commit:** `feat(gsl): add tmux builtin style (ascii, no-fill, space separator)`

---

### Task 11 — Documentation: `src/gsl/README.md`

Add a new section **"tmux integration"** to `src/gsl/README.md`. Insert it between the existing `## Subcommands` section (after the `gsl config` block) and the `## Segments` section.

**Content to insert:**

````markdown
## tmux integration

`gsl status --tmux` prints a single, tmux-safe status line to stdout:

- Nerd Font and emoji glyphs are replaced with ASCII equivalents (`[dir]`, `br:`, `[ai]`, …).
- ANSI colour escape sequences are suppressed (tmux uses its own `#[fg=...]` colour syntax; standard ANSI escapes confuse tmux's width accounting).
- Every `#` in the output is doubled to `##` so tmux does not interpret branch names like `feature/#123` or PR badges like `PR#42` as format variables.

### Quick start

1. Add to `~/.tmux.conf`:

   ```conf
   set -g status-right "#(gsl status --tmux)"
   set -g status-interval 5
   ```

2. Reload tmux config:

   ```sh
   tmux source-file ~/.tmux.conf
   ```

3. The right side of the tmux status bar now shows the gsl status line, refreshed every 5 seconds.

### Style selection

`--tmux` always overrides to ASCII glyphs regardless of the active style. To make the ASCII style persistent (e.g., for a shell prompt that mirrors the tmux output), switch to the `tmux` built-in style:

```sh
gsl config style tmux
```

The `tmux` style uses `Glyphs: ascii`, `Fill: false`, and `Separator: space`. Running `gsl status` (without `--tmux`) with this style active produces the same character set.

### Combined tmux + Gemini/Claude visibility

Because Gemini CLI has no post-turn hook, the tmux status bar provides persistent AI context visibility — `dirgit`, `repo`, and `time` always render; the `ai` segment self-omits when `gsl status --tmux` is run outside a Claude session (no payload). Run `gsl render --tmux` (see note below) to get the AI segment in tmux when piping a Claude payload.

> **Note:** `gsl render` reads a Claude JSON payload from stdin; it does not accept `--tmux` in the initial implementation. The typical pattern for tmux is `gsl status --tmux` (for always-on context) rather than `gsl render --tmux` (which would require the caller to supply a payload). A future enhancement could expose `gsl render --tmux` if there is demand.
````

- [ ] Insert the tmux integration section into `src/gsl/README.md` at the described location.
- [ ] Verify the file renders correctly with `cat src/gsl/README.md | head -60` (spot check).

**Commit:** `docs(gsl): add tmux integration section to README`

---

### Task 12 — Documentation: `src/gsl/skill/SKILL.md`

Add a new section at the bottom of `src/gsl/skill/SKILL.md`, after the existing `## Gemini command` section:

```markdown
## tmux status bar integration

`gsl status --tmux` is designed for use in `~/.tmux.conf` as a persistent status bar that works for both Claude Code and Gemini CLI:

```conf
set -g status-right "#(gsl status --tmux)"
set -g status-interval 5
```

Output characteristics:
- ASCII glyphs only (no Nerd Font / emoji dependency)
- No ANSI escape sequences (tmux-width-safe)
- `#` doubled to `##` (tmux format-variable-safe)
- Single line, no trailing newline

Built-in `tmux` style: `gsl config style tmux` activates the same ASCII/no-fill/space-separator preset persistently (without the `--tmux` flag).
```

- [ ] Append the tmux section to `src/gsl/skill/SKILL.md`.

**Commit:** `docs(gsl): document tmux integration in gsl-status skill`

---

### Task 13 — Final verification

**Run the full suite one more time:**

```sh
cd /path/to/src/gsl && go test ./... -race -cover
```

Expected: all packages pass under the race detector.

**Smoke-test the flag (no tmux required):**

```sh
gsl status --tmux
```

Expected: a single line of ASCII text, no ANSI escapes visible, no unescaped `#`.

**Verify `gsl config style tmux` works:**

```sh
gsl config style tmux
gsl config style --list    # should show "tmux" in the list with an asterisk
gsl status                 # should produce ASCII output matching --tmux format
gsl config style powerline # restore default
```

- [ ] `go test ./... -race -cover` passes.
- [ ] `gsl status --tmux` produces a single clean ASCII line.
- [ ] `gsl config style tmux` + `gsl config style --list` shows `tmux (*)`.

**Commit:** `chore(gsl): verify tmux integration end-to-end`

---

## File Manifest

| File | Change |
|------|--------|
| `src/gsl/internal/render/tmux.go` | **new** — `EscapeTmux`, `RenderTmux`, `asciiIconsTable`, `isASCIISafe` |
| `src/gsl/internal/render/tmux_test.go` | **new** — unit tests for all four functions |
| `src/gsl/cmd/status.go` | **modified** — add `--tmux` flag + `statusTmux` var; pass `statusTmux` to `runStatusLine` |
| `src/gsl/cmd/statusline.go` | **modified** — add `tmuxMode bool` parameter; branch on `RenderTmux` vs `Render`; `fmt.Print` (no newline) in tmux mode |
| `src/gsl/cmd/render.go` | **modified** — update `runStatusLine` call to pass `false` as `tmuxMode` |
| `src/gsl/cmd/status_test.go` | **modified** — add `TestStatusCmd_TmuxFlag` and `TestStatusCmd_TmuxFlag_MasterDisabled` |
| `src/gsl/internal/style/builtins.go` | **modified** — add `tmuxStyle` var; register in `builtins` map |
| `src/gsl/internal/style/style_test.go` | **new** (or append to existing) — `TestBuiltin_Tmux`, `TestBuiltins_ContainsTmux` |
| `src/gsl/README.md` | **modified** — add `## tmux integration` section |
| `src/gsl/skill/SKILL.md` | **modified** — add tmux status bar section |

---

## Open Design Questions

1. **`gsl render --tmux`**: Should the `render` command also accept `--tmux`? The render path already reads a Claude payload and could produce a tmux-safe line (useful if someone wants AI segment data in the tmux bar via a script that pipes a Claude payload). Deferred for now — `gsl status --tmux` covers the Gemini/persistent use case; `render --tmux` can be added in a follow-on issue.

2. **Status interval recommendation**: The README snippet suggests `set -g status-interval 5`. Is 5 seconds appropriate? `gsl status` runs `git status --porcelain=v2` (budgeted to ~800 ms) and may also call `gh pr view` (cached for 60 s) on every invocation. A 5-second interval means up to 12 `gsl status` subprocess spawns per minute. Consider recommending 10–15 seconds for battery-sensitive laptops and adding a note in the README.

3. **`--tmux` on `gsl preview`**: `gsl preview --once` is used for CI/golden-file checks. Should `preview --once --tmux` also print a tmux-safe frame? This would allow golden tests to assert the tmux output format. Not included in this plan; low priority.

4. **Color in tmux mode via `#[fg=...]`**: Advanced users may want colorized tmux output. Rather than adding a `--tmux-color` flag now, a user style override could map theme keys to tmux format strings (e.g., `theme.dirgit = "#[fg=green]"`). This is a design change to the render pipeline and is out of scope for this issue.

5. **`asciiIconsTable` duplication**: `render.asciiIconsTable` duplicates `style.asciiIcons` (unexported). The cleanest fix is to export `style.ASCIIIcons()` (returning a copy) and have `render.RenderTmux` call it. Deferred to avoid a breaking change to `internal/style`; add a `// TODO(issue-33-followup): use style.ASCIIIcons()` comment in `tmux.go`.
