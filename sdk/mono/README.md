# mono

`mono` fills one terminal with stacked panes, each running a real program in its own pseudo-terminal: by default **htop** on top, a **GPU monitor** in the middle, and **gsl** at the bottom, refreshed every second. Panes whose tool is missing, or can't work on this machine, are left out and the others grow into their rows, so the same config works on a WSL laptop, an NVIDIA box, and a Raspberry Pi.

```
┌ mono v0.1.0 — a1b2c3d   ● htop                               ctrl+b ? help ┐  title  5%
│ htop                                                                       │  htop  60%
│ nvtop  (dropped where it has no GPU to read; htop and gsl take its rows)   │  gpu   30%
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
  "panes": [
    { "name": "htop", "size": "60%", "cmd": "htop-fit", "fallbacks": ["htop"] },
    { "name": "gpu",  "size": "30%", "cmd": "nvtop", "fallbacks": ["nvitop"] },
    { "name": "gsl",  "size": "5%",  "cmd": "gsl status", "every": "1s" }
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

So on WSL with an Intel GPU, nvtop is installed but has nothing it can read, and its pane is dropped. `mono doctor` prints the decision for every pane:

```console
$ mono doctor
config: ~/.config/mono/config.json
prefix: ctrl+b

PANE   SIZE  ROWS@40  RUNS         WHY
title  5%    2        -            version, commit, focused pane, help hint
htop   60%   35       htop-fit     found htop-fit
gpu    30%   -        (not shown)  nvtop not installed; nvitop not installed
gsl    5%    3        gsl status   found gsl
```

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
