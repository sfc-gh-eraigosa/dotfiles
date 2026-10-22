# gsl — Go Status Line (`sdk/gsl`)

`gsl` renders a powerline-style status bar for Claude Code and Antigravity CLI.

- **Module path:** `github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl`
- **Binary:** `gsl` (installed to `~/opt/bin/gsl` by `install.sh`)
- **External install:** `go install github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl@<tag>` (tags: `sdk/gsl/vX.Y.Z`)
- **Build:** `bash sdk/gsl/build.sh` (injects version via `-ldflags -X .../sdk/gsl/internal/version.*`)
- **Skill:** [`skill/SKILL.md`](./skill/SKILL.md) drives the `gsl-status` agent skill.
- **Font helpers:** `scripts/` holds the Nerd Font installers wired into `install.sh`.
- **MCP:** `internal/mcp` — `Get` reads a per-cwd cache and **never spawns** (it is on the
  render path); `Refresh` is the only probe, run detached by `gsl mcp refresh`. Fixtures are
  byte captures of real `claude mcp list` (`internal/mcp/testdata/README.md`) — never retype them.
- **Colour:** `internal/style` (`ResolveBlockColors`, `ParseColor`, `DetectColorMode`) is the
  one colour engine; segments emit **no escapes** (roles travel as `LinkSpan.Role`), the
  render join layer paints. Palettes come from `sdk/libs/tui/theme`.
- **Studio:** `internal/tui` — bare `gsl config`, value-semantic, built on `sdk/libs/tui`
  (read its GUIDE first). Its README key table renders from `tui.Keys()` (pinned by a test).
- **Tests:** the `cmd` suite is hermetic (TestMain stubs git/gh/MCP/video); CI runs `-race`,
  `go vet` and `GSL_STRICT_CHECK=1 scripts/check-deps.sh` (`.github/workflows/docker-image.yml`).

See [docs/](./docs/) for design notes and the [README](./README.md) for usage.
