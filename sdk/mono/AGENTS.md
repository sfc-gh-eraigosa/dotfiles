# mono — stacked-pane system dashboard (`sdk/mono`)

`mono` fills one terminal with stacked panes, each a real program in its own
pseudo-terminal: htop, a GPU monitor, and `gsl status` refreshed every second
by default. User docs: [README.md](./README.md). Design:
[docs/mbo/designs/mono.md](../../docs/mbo/designs/mono.md).

- **Module path:** `github.com/sfc-gh-eraigosa/dotfiles/sdk/mono`
- **Binary:** `mono` (installed to `~/opt/bin/mono` by `install.sh`, gff `install.sdk.mono`)
- **Build:** `bash sdk/mono/build.sh` (stamps `internal/version` via `-ldflags -X`)
- **Config:** `${XDG_CONFIG_HOME:-~/.config}/mono/config.json` (`$MONO_CONFIG` overrides)

## Layout

| Package | Owns |
| :-- | :-- |
| `cmd/` | cobra CLI: `mono` (dashboard), `mono config [get\|set\|add\|remove\|move\|path]`, `mono doctor`, `mono version` |
| `internal/config` | the file: defaults, validation (errors name the field), dotted-key get/set, atomic save; `deck.go`: tools × views, `zoomable`, `middle` (`Effective()`), nav settings |
| `internal/layout` | percentages → whole rows (pure) |
| `internal/probe` | which command each pane gets on this host (PATH + hardware probes) |
| `internal/pane` | one program in a pty + a `vt10x` emulated screen; `every` re-runs; resize, restart, stop |
| `internal/screen` | emulator cells → ANSI rows; diffing frame writer |
| `internal/input` | raw bytes → program, except the prefix; decodes the key after it |
| `internal/app` | dashboard core (`HandleBytes`, `Resize`, `Frame`, terminal-free and tested) + `Run` (raw mode, alt screen, signals); `deck.go`: slots (a deck shows one view), zoom heights over the middle, nav mode, double-esc |
| `internal/configui` | the `mono config` screen (bubbletea, `sdk/libs/tui`) |

## Rules

- **Keys come from `sdk/libs/tui`** (`keymap.Vim` trimmed and merged in
  `internal/app/keys.go`). The overlay, title hint, `--help` and the README
  table render from that one table; `cmd/cmd_test.go` pins that the README
  agrees. Don't add a key without its `Binding`.
- **Everything but the prefix reaches the program.** Don't intercept `esc`,
  `q`, F-keys or arrows outside the prefix.
- **mono only runs tools.** No pane gets data from mono; tools stay standalone.
- **Never re-exec mono's own binary from code tests can reach without a
  `testing.Testing()` guard** (`cmd/root.go` `configArgv`). A re-exec'd test
  binary runs the whole suite again; that fork-bombed WSL via gsl on
  2026-10-17.
- **Emulators must answer terminal queries** (`vt10x.WithWriter(tty)`), or
  bubbletea-based tools (gsl) stall 5 s at startup.
- **Tests spawn only `/bin/sh`, `cat`, `stty`, `echo`** — never this binary. Run
  them under a process cap: `ulimit -u $(( $(ps -L -u $USER --no-headers | wc -l) + 1500 ))`.
