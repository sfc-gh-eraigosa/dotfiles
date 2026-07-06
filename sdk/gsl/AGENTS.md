# gsl — Go Status Line (`sdk/gsl`)

`gsl` renders a powerline-style status bar for Claude Code and Antigravity CLI.

- **Module path:** `github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl`
- **Binary:** `gsl` (installed to `~/opt/bin/gsl` by `install.sh`)
- **External install:** `go install github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl@<tag>` (tags: `sdk/gsl/vX.Y.Z`)
- **Build:** `bash sdk/gsl/build.sh` (injects version via `-ldflags -X .../sdk/gsl/internal/version.*`)
- **Skill:** [`skill/SKILL.md`](./skill/SKILL.md) drives the `gsl-status` agent skill.
- **Font helpers:** `scripts/` holds the Nerd Font installers wired into `install.sh`.

See [docs/](./docs/) for design notes and the [README](./README.md) for usage.
