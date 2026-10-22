# mono

`mono` fills one terminal with panes, each running a real program in its own pseudo-terminal. By default a title bar sits on top, **gsl** at the bottom (refreshed every second), and between them **one full-height middle** showing one tool at a time: [gdash](../gdash/README.md) (the lab's Grafana dashboards), then a **GPU monitor**, then **htop**. Tools that are missing, or can't work on this machine, are left out, so the same config works on a WSL laptop, an NVIDIA box, and a Raspberry Pi.

```text
┌ mono v0.1.0 — a1b2c3d   ● mid ◀ gdash ▶ · conductor 1/4      ctrl+b ? help ┐  title  5%
│ gdash conductor   (ctrl+b l → k3s-nodes, nodes, ci, then nvtop, htop)      │  mid   90%
│ 📁 dotfiles 🌿 main · 🎮 6%|1.9G/7.7G · ⏰ 10:26:13 PM PDT                 │  gsl    5%
```

mono only *runs* tools. htop, nvtop and gsl are unmodified and still work on their own.

The htop pane runs **`htop-fit`** when it is installed: your dotfiles htop view (history graphs filling the header, only the top few processes), fitted to the pane's height. It is the same wrapper `htop` uses in your shell, installed as a command (`~/opt/bin/htop-fit`, gff `install.tools.htop-fit`) because mono starts programs without an interactive shell, where the `htop()` function does not exist. Without it the pane runs plain htop. The fit is computed when htop starts, so after resizing the window `ctrl+b r` re-fits it.

## Keys

Every key goes to the focused program, so htop's `q`, `/`, `esc` and F-keys behave exactly as in plain htop. The one exception is the prefix, **`ctrl+b`** by default. Press it, then:

| Keys | What it does |
| :-- | :-- |
| `j` `down` | focus the next pane |
| `k` `up` | focus the previous pane |
| `h` `left` | previous tool in a deck pane |
| `l` `right` | next tool in a deck pane |
| `z` | zoom the pane over the middle |
| `space` | nav mode: hjkl and z, no prefix |
| `:` | command line (see Commands below) |
| `?` `f1` | this help |
| `q` `ctrl+c` | quit mono (stops every pane) |
| `r` | restart the focused pane's program |
| `esc` | cancel the prefix |
| `ctrl+b` | send ctrl+b itself to the program |

The keys come from the shared sdk keymap (`sdk/libs/tui`, `keymap.Vim`), so they mean the same thing as in fleet and gff. While the prefix waits, the title bar lists them; `ctrl+b ?` shows the full help.

Commands (`ctrl+b :`):

| Command | What it does |
| :-- | :-- |
| `:config` | open the config screen full size; mono reloads when it closes |
| `:restart [pane]` | restart a pane's program (default: the focused one) |
| `:focus <pane>[/<tool>[/<view>]]` | focus a pane, a deck's tool, or one of its views |
| `:reload` | re-read the config and re-check tools |
| `:q` | quit |

When a program exits (you pressed `q` in htop), its pane keeps its last screen with `htop exited — ctrl+b r restart` on the bottom row. mono stays up.

## Configuration

Settings live in `${XDG_CONFIG_HOME:-~/.config}/mono/config.json` (`$MONO_CONFIG` overrides). A missing file is the default:

```json
{
  "prefix": "ctrl+b",
  "title": { "size": "5%" },
  "middle": "single",
  "panes": [
    { "name": "mid",  "size": "30%", "tools": [
        { "name": "gdash", "probe": "gdash doctor --quiet", "zoom": true, "views": [
            { "name": "conductor", "cmd": "gdash conductor" },
            { "name": "k3s-nodes", "cmd": "gdash k3s-nodes" },
            { "name": "nodes",     "cmd": "gdash nodes" },
            { "name": "ci",        "cmd": "gdash ci" } ] },
        { "name": "gpu", "views": [ { "name": "nvtop", "cmd": "nvtop", "fallbacks": ["nvitop"] } ] } ] },
    { "name": "htop", "size": "60%", "cmd": "htop-fit", "fallbacks": ["htop"] },
    { "name": "gsl",  "size": "5%",  "cmd": "gsl status", "every": "1s", "zoomable": false }
  ]
}
```

| Field | Meaning |
| :-- | :-- |
| `prefix` | `ctrl+<letter>`, not `c`, `i`, `j` or `m` (programs need those) |
| `title.size` | title bar height, percent of the terminal |
| `panes[].name` | lowercase letters, digits, `-`, `_`; unique |
| `panes[].size` | height, percent of the terminal |
| `panes[].cmd` | command line, run with `/bin/sh -c "exec <cmd>"` |
| `panes[].every` | re-run interval (`1s`, at least `100ms`). Empty means an interactive program that stays up |
| `panes[].fallbacks` | commands tried in order when `cmd` can't run here |
| `panes[].probe` | `""` built-in check, `"none"` to skip it, or a command whose exit 0 means usable |
| `panes[].cwd` | working directory; `""` is where mono started |
| `panes[].tools` | instead of `cmd`: a **deck**, a list of tools, each with `views` (a view takes `cmd`, `every`, `fallbacks`, `probe`, `cwd`). One view is on screen at a time |
| `panes[].tools[].probe` | a command that must exit 0 for the tool to show (`gdash doctor --quiet`); it runs once |
| `panes[].tools[].zoom` | `true`: the tool fills the whole middle while it is on screen |
| `panes[].zoomable` | `false` keeps the pane out of the middle: zoom never moves it and `j`/`k` skip it (the gsl footer; `:focus gsl` still reaches it) |
| `middle` | `single` (default): the middle panes merge into one deck holding all their tools, in pane order. `split`: they share their rows (sized by `panes[].size`), `z` zooms one |
| `nav_key` | the key after the prefix that enters nav mode (`space`) |
| `nav_esc_ms` | the esc-esc window for entering nav mode (`300`); `0` turns double-esc off |

Edit it from the command line:

```bash
mono config set panes.gpu.cmd nvitop
mono config set panes.htop.size 50%
mono config add logs --size 15% --at 1 -- journalctl -f
mono config move logs down
mono config remove logs
mono config get                 # the whole file, as JSON
mono config get panes.gsl.every # one value
```

A rejected value names the field (`panes.htop.size: size must be a percentage from 1% to 100%, got "0%"`) and nothing is written. Saves are atomic.

Or run `mono config` with no arguments for the config screen: one row per pane with its tool check (`✓ htop`, `✗ not shown: nvtop not installed`), and a preview of the rows each pane gets at your terminal's height. `enter` edits a row, `s` its size, `a` adds, `d` removes (asks first), `J`/`K` reorder, `:w` saves, `q` asks before discarding changes. Every `:` command there is a `mono config` verb (`:set`, `:add`, `:remove`, `:move`).

### Sizes and dropped panes

- The title keeps its share of the screen; panes split the rest in proportion to their sizes, so a dropped pane's rows go to the others.
- Every region gets at least one row, so a 5% pane never rounds away on a short terminal.
- At 40 rows the default is title 2, htop 24, gpu 12, gsl 2. With the GPU pane dropped it is title 2, htop 35, gsl 3.

### Which tool a pane gets

A candidate (`cmd`, then each fallback) is usable when it is on `PATH` and passes its probe. The built-in probe only matters for GPU tools:

| Tool | Usable when |
| :-- | :-- |
| `nvtop` | `nvidia-smi -L` succeeds, or a `/sys/class/drm` card is bound to `amdgpu`, `i915`, `xe`, `nouveau`, `msm`, `panfrost`, `panthor` or `v3d` |
| `nvitop`, `gpustat` | `nvidia-smi -L` succeeds |
| anything else | it is on `PATH` |

So on WSL with an Intel GPU, nvtop is installed but has nothing it can read, and it is dropped. `mono doctor` prints the decision for every pane and deck view (here an NVIDIA host that reaches the lab Grafana):

```console
$ mono doctor
config: ~/.config/mono/config.json
prefix: ctrl+b
middle: single

PANE                 SIZE  ROWS@40  RUNS             WHY
title                5%    2        -                version, commit, focused pane, help hint
mid                  90%   36       deck             6 of 6 views usable
  └ gdash/conductor                 gdash conductor  found gdash · zoom: fills the middle
  └ gdash/k3s-nodes                 gdash k3s-nodes  found gdash · zoom: fills the middle
  └ gdash/nodes                     gdash nodes      found gdash · zoom: fills the middle
  └ gdash/ci                        gdash ci         found gdash · zoom: fills the middle
  └ gpu/nvtop                       nvtop            found nvtop
  └ htop/htop                       htop-fit         found htop-fit
gsl                  5%    2        gsl status       found gsl
```

## Decks, nav mode and zoom

A **deck** pane holds several tools, each with several views; only the view
on screen runs, and switching starts its program fresh. By default the whole
middle is one deck: [gdash](../gdash/README.md) (the lab's Grafana
dashboards), then the GPU monitor, then htop. On a host that can't reach
Grafana, gdash's probe fails and the deck opens on the GPU monitor.

**Nav mode** moves without the prefix: `esc esc` (both escapes still reach
the program) or `ctrl+b space`, then `j`/`k` walk the deck's views and on to
the next middle pane (never the gsl footer), `h`/`l` switch tools, `z` zooms, and `esc` leaves. Any other
key leaves too and goes where it would have gone — typed text to the program,
`ctrl+b` to start a command — so nothing you type is lost. On a deck, `ctrl+b r`
also re-checks its tools (say gdash after Grafana comes back) without
re-reading the config file; `:reload` does that. The title bar says where you are, `mid ◀ gdash ▶ · ci 4/4`, and
shows `NAV` and `ZOOM`.

**The middle** is every pane except the fixed ones (`zoomable: false`, the
gsl footer); the title bar is never part of it. `z` gives the focused middle
pane every middle row and `z` again restores the split. gdash has
`zoom: true`, so it takes the whole middle whenever it is on screen and gives
the rows back when you switch away. The default middle is **one pane**
(`middle: single`); to stack the middle panes instead, htop 60% over a 30%
deck of gdash and the GPU monitor:

```bash
mono config set middle split      # back: mono config set middle single
mono config add-view mid gdash alerts -- gdash alerts   # another gdash view
```

A host that already has a `~/.config/mono/config.json` keeps its layout; to
pick up the new default, move that file aside (or `mono config remove gpu`
and add the deck with `mono config add-tool`).

## How it works

Each pane is a child process in its own pseudo-terminal (`creack/pty`). mono feeds what it draws into a terminal emulator (`hinshun/vt10x`), and redraws the real terminal from those screens, only the rows that changed, at most ~30 times a second. Keystrokes go to the focused pane's pseudo-terminal; resizing sends each program `SIGWINCH`.

- **`every` panes** run the command in a fresh pseudo-terminal each tick, sized to the pane, and show its screen when it finishes. A run still going at the next tick is skipped, and a run taking longer than 5 s (or 5 ticks) is killed.
- **Terminal queries are answered**, as a real terminal would (cursor position, background color). bubbletea asks both when a program starts and waits up to 5 s for the reply; unanswered, `gsl status` took 5 s every run.
- **Quitting** sends `SIGHUP` to each program's process group, then `SIGKILL` after 0.5 s, so nothing is left running.
- mono never re-runs its own binary from code its tests reach (`:config` refuses under `go test`): re-exec'ing a test binary runs the whole suite again, which is how the gsl video refresher fork-bombed WSL on 2026-10-17.

## Install

`install.sh` builds it into `~/opt/bin/mono` behind the gff flag `install.sdk.mono` (on by default):

```bash
bash sdk/mono/build.sh        # or: go install github.com/sfc-gh-eraigosa/dotfiles/sdk/mono@latest
gff set install.sdk.mono false
```

Design: [docs/mbo/designs/mono.md](../../docs/mbo/designs/mono.md).
