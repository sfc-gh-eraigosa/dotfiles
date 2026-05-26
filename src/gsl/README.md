# gsl — Go Status Line

`gsl` renders a powerline-style status line for Claude Code (reading a JSON
payload on stdin after every assistant turn) and an on-demand line for
Gemini/CLI.

## Overview

The final tool provides 4 segments: `dirgit`, `repo`, `ai`, and `time`, with
configurable styles and a preview TUI.

## Quick start

```sh
# Build and install
bash build.sh

# Show version
gsl version

# Show status line (reads Claude stdin JSON from stdin)
echo '<json>' | gsl render

# Interactive preview (CP3)
gsl preview
```

## Configuration

Config file: `${XDG_CONFIG_HOME:-~/.config}/gsl/config.json`

Run `gsl --help` for available commands.

## Development

```sh
cd src/gsl
go build ./...
go test ./... -cover
bash scripts/check-deps.sh
```
