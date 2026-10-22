# gsl-ultra — evidence (2026-10-20, one PR for mcp → style → tui → rollout)

Real-terminal transcripts (tmux `capture-pane`, 110×40, a scratch
`XDG_CONFIG_HOME`); `<host>` and `<xdg>` replace the machine name and the
scratch path.

## mcp

```text
$ gsl mcp refresh            # the real `claude mcp list`, ~16 s
MCP: 13 connected · 5 failed · 26 needs auth · 6 configured — probed 0s ago
  failed: plugin:data:definite, plugin:github:github, …
  needs authentication: claude.ai Linear, …

# a fake `claude` first on PATH logs every run
fresh cache: render            → claude runs: 0
stale cache: 3 renders, ~60 ms → claude runs (detached refresher, single-flight): 1
```

`go test -tags integration -run RealCLI ./internal/mcp/` — 66 servers parsed,
none unrecognised.

## style

```text
NO_COLOR=1 gsl render   → 0 escape bytes
gsl render --color 16   → 0 "38;5;"/"48;5;" sequences
gsl render              → 0 unpainted chevrons; the MCP breakdown as chips:
  ESC[48;5;42m ESC[38;5;16m 13✓  ESC[48;5;203m ESC[38;5;16m 5✗  ESC[48;5;214m ESC[38;5;16m 26!
```

## tui — `gsl config`

Opened:

```text
gsl config — <xdg>/gsl/config.json

> style     powerline
  palette   auto
  links     underline
  timezone  America/Los_Angeles
  time      15:04:05
  date      2006-01-02
  [x] host
  [x] dirgit
  [x] repo
  [x] ai
  [x] video
  [x] time

Preview  (110 cols — < > to change)
 🪟 <host>   dotfiles  main     Sonnet 4.6  30% 60k/200k  13✓ 5✗ 5h 10%   23:15:31 PDT
····+····1····+····2····+····3····+····4····+····5····+····6····+····7····+····8····+····9····+····0····+····1

MCP  13✓ 5✗ 26! · 6 configured · probed 7m28s ago
  failed: plugin:github:github, plugin:zoom-plugin:zoom-mcp, …

j/k move  : command  ? help  q quit  space toggle  w save
```

After `j space` (palette → dark) and `G space` (time off) — the preview
re-fits at once:

```text
  palette   dark
…
> [ ] time

Preview  (110 cols — < > to change)
 🪟 <host>   dotfiles  main   11   Sonnet 4.6  30% 60k/200k  13✓ 5✗ 26! 5h 10%
```

`w` then `q`: config.json holds `palette: dark`, `time` disabled, and the next
real `gsl render` draws no time segment:

```text
 🪟 <host>   dotfiles  main   11   Opus 4.8  12%  13✓ 5✗ 26!   0%|2.5G/24G
```
