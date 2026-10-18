# mono — design

- **Slug:** mono
- **Date:** 2026-10-17
- **Status:** Proposed
- **Relates to:** follows draft PR #404 (htop-fit); issue / PR to be opened for the build
- **Author(s):** Edward Raigosa (requirements), Claude (write-up)

## 1. Problem / context

The owner wants one screen that shows the machine at a glance: htop, a GPU view, and the
gsl status line, stacked in one terminal, with the mix adjustable per computer.

Verified facts that shape this:

- **htop can't do it alone.** Its meters are compiled in; there is no way to feed it outside
  numbers, and it has no GPU-memory meter. PR #404 already squeezes htop with a wrapper
  (`opt/etc/htop/htop-fit.sh`); that stays as is and is unrelated to mono.
- **GPU tools depend on the host.** `nvtop` reads NVIDIA (NVML) or Linux DRM drivers. On the
  WSL laptop (Intel Arc via `/dev/dxg`) it has nothing to read. Windows' performance counters
  do have the numbers and WSL can read them through `powershell.exe` (tested: `Get-Counter
  "\GPU Engine(*engtype_3D)\Utilization Percentage"` and `"\GPU Adapter Memory(*)\Shared
  Usage"` / `"Dedicated Usage"` returned `6.2% shared=2,818MB dedicated=0MB`), but each call
  takes ~2.2 s because PowerShell has to start.
- **gsl prints one line and exits.** `gsl render` reads Claude Code's JSON from stdin
  (it would block in a pane); `gsl status` never reads stdin and renders host, dirgit, repo,
  and time without a payload (`sdk/gsl/cmd/status.go`, `cmd/statusline.go`). Segments live in
  `sdk/gsl/internal/render/seg_*.go`, ordered by `config.go:148-157`, each with a priority used
  to drop segments when the line is too narrow.
- **gsl segments can already be switched off locally** (`gsl config disable <seg>`, stored in
  `~/.config/gsl/config.json`). gff today only gates gsl's clickable links
  (`gsl.links.*` in `.github/gff/features.yaml`), read at runtime by
  `sdk/gsl/internal/flags/flags.go` (100 ms budget, any failure = on).
- **Every sdk TUI shares one keymap and help style.** `sdk/libs/tui` (`keymap.Vim`, `nav`,
  `overlay`, `cmdline`) plus `sdk/libs/tui/GUIDE.md`; fleet's help overlay
  (`sdk/fleet/cmd/tui_view.go` `helpView` / `layoutHelp`) is the reference look.
- **No sdk module runs programs in a pseudo-terminal today.** bubbletea v1.3.10, lipgloss
  v1.1.0, charmbracelet/x/ansi and x/term are already in use; a PTY library and a terminal
  emulator are new dependencies.

## 2. Goals, non-goals, anti-goals

**Goals**

1. `mono` is a new Go command that fills the terminal with stacked panes, each running a
   real program in its own pseudo-terminal, like herdr/tmux do.
2. Default layout, top to bottom: **title bar 5%**, **htop 60%**, **nvtop 30%**,
   **gsl 5%** (bottom pane re-runs gsl every second).
3. **Picks tools by what this machine can actually run.** A pane whose tool is missing or
   can't work here is dropped and the remaining panes share its rows in proportion. A pane may
   list fallbacks (`nvtop`, else `nvitop`, else drop).
4. **Configurable** through both an interactive screen (`mono config`, or `:config` inside
   mono) and CLI verbs (`mono config get|set|add|remove|move`), stored in
   `~/.config/mono/config.json`.
5. **Keys follow the sdk standard** (`sdk/libs/tui`), behind a prefix: `ctrl+b`.
6. **gsl gets a `[video]` segment**, shown before the time as `[video] 6%|2.8G/16G`
   (GPU utilization, memory used / max). It works everywhere gsl runs, including the Claude
   Code status line, not only in mono.
7. **gff flags per gsl segment** — `gsl.section.{host,dirgit,repo,ai,time,video}`, all
   default on — so segments can be removed to fit.
8. Installed by `install.sh` behind gff `install.tools.mono` (default on).

**Non-goals**

- Replacing herdr or tmux. mono is a fixed dashboard, not a workspace manager: no tabs,
  sessions, detach/attach, or splitting at runtime beyond what config describes.
- Changing htop, nvtop, or any hosted tool. They run unmodified; `htop` alone still works.
- Mouse support (v1).
- Side-by-side panes (v1 stacks vertically only). The config shape should not rule them out.

**Anti-goals (a failure even if it technically works)**

- **mono feeding data to the tools.** mono only *runs* tools. gsl must stay a standalone gsl:
  it reads the GPU itself; nothing written by mono is required for any segment.
- Keys that collide with the hosted program. Inside a pane, every key except the prefix goes
  to the program — htop's `esc`, `q`, `/`, F-keys all behave exactly as in plain htop.
- A second, mono-only keymap or help style. A user who knows fleet already knows mono.
- An empty pane on a host that can't run its tool (e.g. a blank nvtop on WSL).

## 3. Options considered

1. **Own Go app with PTYs + terminal emulator (chosen).** Each pane is a child process in a
   pseudo-terminal; mono parses its output into a screen buffer and composes the buffers.
   Full control over layout, capability checks, keys, and title bar; one binary.
2. **Generate a tmux/herdr layout.** Cheap, but mono would inherit the multiplexer's keys,
   status bar, and session model, and the owner was explicit that herdr and mono are
   different things.
3. **btop with custom collectors.** Loses htop, which the owner wants on top.

## 4. Decision

### 4.1 Screen

```
┌ mono v0.1.0 — a1b2c3d            htop ●   ctrl+b ? help ┐  title   5%
│ htop                                                    │  pane   60%
│ nvtop                                                   │  pane   30%
│ host · dir ⎇ main · repo · [video] 6%|2.8G/16G · 11:16  │  gsl     5%
```

- **Title bar** (owner): `mono <version> — <commit sha>` left; the focused pane's name and the
  prefix hint right. Version and sha come from `-ldflags` via the module's `build.sh`, like
  the other sdk tools. While the prefix is waiting for its second key the hint is highlighted.
- **Sizes are percentages of terminal rows**, rounded to whole rows; the title bar and any
  pane sized ≤ 5% get **at least one row** (my call), and rounding leftovers go to the largest
  pane. Panes are separated by a one-row divider carrying the pane name (my call; the builder
  may fold it into the pane's first row if rows are too tight).
- **Resize** re-lays out and resizes every pseudo-terminal (`SIGWINCH` to the child).
- **Focus** starts on the first pane (htop). The focused pane's divider is accented.

### 4.2 Panes and tools

A pane is one of two kinds (my call — needed so gsl can be "refreshed every second by mono"
without mono touching gsl's data):

- **`run`** — a long-lived interactive program in a pseudo-terminal (htop, nvtop). If it
  exits, the pane shows `exited (status N) — ctrl+b r restart` and mono stays up (my call).
- **`every`** — a command re-run on an interval, its latest output shown (gsl:
  `gsl status`, every `1s`). It still runs in a pseudo-terminal sized to the pane, so
  gsl sees the real width and drops low-priority segments itself. Overlapping runs are
  skipped, not queued.

**Capability check at startup.** For each pane, try its tool and then its fallbacks in order;
the first that passes wins:

1. the command is on `PATH`, and
2. its optional `probe` succeeds (a short command, run with a timeout, exit 0 = usable).

Built-in probes (my call; overridable in config): `nvtop` / `nvitop` usable only when
`nvidia-smi -L` succeeds or a `/sys/class/drm/card*/device/driver` is one nvtop reads
(amdgpu, i915, xe, nouveau, msm, panfrost, panthor, v3d — same list as
`opt/etc/htop/htop-fit.sh`). So on the WSL laptop nvtop is dropped and htop + gsl share the
rows. A pane with no usable tool is removed and the others scale up in proportion to their
configured sizes. `mono doctor` (my call) prints what was found and why each pane was kept or
dropped.

### 4.3 Keys

From `sdk/libs/tui` (`keymap.Vim` merged with mono's bindings), behind a **prefix**
(owner: `ctrl+b`, configurable as `"prefix"` in config):

| Keys | Action |
| :-- | :-- |
| any key | goes to the focused pane's program (including `esc`, `q`, `/`, F-keys) |
| `ctrl+b` `j` / `k` (`↓` / `↑`) | focus next / previous pane |
| `ctrl+b` `?` / `f1` | help overlay |
| `ctrl+b` `:` | command line (`:config`, `:layout`, `:q`, …) |
| `ctrl+b` `r` | restart the focused pane's program |
| `ctrl+b` `q` / `ctrl+c` | quit mono (children get SIGHUP, then SIGKILL after a grace period) |
| `ctrl+b` `esc` | cancel the prefix |
| `ctrl+b` `ctrl+b` | send a literal `ctrl+b` to the program |

- Help, the title-bar hint, `mono --help`, and the README key table all render from the one
  binding table; a test pins that they agree (GUIDE §2, §9).
- Help overlay looks and behaves like fleet's: `❓ keys` heading, grouped sections
  (*Panes*, *Mono*), two columns from 80 cells wide, trims from the end with
  `… N more — make the window taller`, any key closes.
- `ctrl+b` is free in the owner's setup (tmux and herdr both use `ctrl+a`); anyone running
  mono inside a tmux on its default prefix changes `"prefix"`.

### 4.4 Configuration

`~/.config/mono/config.json` (honours `$XDG_CONFIG_HOME`; JSON to match gsl — my call).
Missing file = built-in default. Shape (field names are the builder's to finalize):

```json
{
  "prefix": "ctrl+b",
  "title": { "size": "5%" },
  "panes": [
    { "name": "htop", "size": "60%", "run": "htop" },
    { "name": "gpu",  "size": "30%", "run": "nvtop", "fallbacks": ["nvitop"] },
    { "name": "gsl",  "size": "5%",  "every": "1s", "cmd": "gsl status" }
  ]
}
```

- **CLI verbs** (mirrors gsl's `config` verbs): `mono config get [key]`,
  `mono config set <key> <value>` (e.g. `panes.gpu.run nvitop`, `panes.gpu.size 30%`),
  `mono config add|remove|move`. A bad value is rejected naming the field and the value;
  nothing is written. Writes are atomic (temp file + rename).
- **Interactive screen** (`mono config` with no args, or `:config` in mono): one row per pane
  (name, tool, size, fallbacks, whether it's usable on this host), `j`/`k` to move, `enter`
  to edit, `J`/`K` to reorder, a live preview of the stacked layout, `:w` to save, `q` with
  unsaved changes asks via `overlay.Confirm`. Every `:` command mirrors a CLI verb.
  Built on `sdk/libs/tui` like gff's settings screen (my call).
- Sizes that don't add up to 100% are scaled to fit, with a warning from `mono doctor` and
  the config screen (my call).

### 4.5 gsl changes (gsl stays standalone)

**`video` segment** — new `seg_video.go` + `seg_video_data.go` in
`sdk/gsl/internal/render/`, placed immediately before `time` in the default order, with a
low priority so it drops before host/dirgit on narrow lines (builder sets the number).
Renders `[video] <util>%|<used>/<max>`, with human-sized memory (`2.8G/16G`). Hidden when no
GPU source is found.

Data source, first that works:

| Host | Source |
| :-- | :-- |
| NVIDIA (incl. WSL with NVIDIA) | `nvidia-smi --query-gpu=utilization.gpu,memory.used,memory.total --format=csv,noheader,nounits` (sum or first GPU — builder's call, documented) |
| WSL without NVIDIA | Windows counters via `powershell.exe`: utilization = sum of `\GPU Engine(*engtype_3D)\Utilization Percentage`; used = `Dedicated Usage` if > 0 else `Shared Usage`; max = dedicated memory if > 0, else the adapter's shared-memory limit (DXGI `SharedSystemMemory`, normally ~half of RAM) — builder picks the query |
| Linux AMD / Intel | DRM sysfs (`gpu_busy_percent`, `mem_info_vram_used/total`) where the driver provides it |

**Caching so the status line stays fast** (my call; anti-goal: no data from mono). The data
file lives in gsl's own state dir (`$XDG_STATE_HOME/gsl/video.json`). On render, gsl reads it;
if it is older than 1 s, gsl starts **one** detached refresher (guarded by a lock file so
renders every second don't pile up PowerShell processes) and renders the last value. With no
data yet, the segment hides. On WSL that means values lag by ~2 s; on NVIDIA and DRM the
refresh is fast enough to run inline within gsl's existing time budget.

**gff per segment** — add to `.github/gff/features.yaml`, all `boolDefault: true`:
`gsl.section.host`, `gsl.section.dirgit`, `gsl.section.repo`, `gsl.section.ai`,
`gsl.section.time`, `gsl.section.video`. Resolved alongside `gsl.links.*` in
`internal/flags` (same 100 ms budget, failure = on). A segment shows only when its flag is on
**and** it is enabled in gsl's own config (my call: the flag decides whether a segment can
appear at all; `gsl config disable` still hides it locally).

### 4.6 Module, install, docs

- `sdk/mono/`, module `github.com/sfc-gh-eraigosa/dotfiles/sdk/mono`, following the
  `sdk/AGENTS.md` "Adding a module" checklist exactly: cobra `cmd/`, `internal/`, `build.sh`
  sourcing `version.sh`, `libs/log` for logging, `AGENTS.md` + `CLAUDE.md` symlink,
  `README.md`, rows in the Modules and "Pick your tool" tables, and a section in
  `sdk/README.md` with a real demo.
- New dependencies: a PTY library (e.g. `creack/pty`) and a VT terminal emulator that can
  render a child's screen into a cell buffer (candidates: `charmbracelet/x/vt`,
  `hinshun/vt10x`). The builder picks after a short spike that runs htop and nvtop inside
  it and checks colors, alternate screen, and resizing.
- `install.sh`: build + install to `~/opt/bin/mono` behind gff `install.tools.mono`
  (default on), mirroring how the other sdk tools are wired. Config phase, not deps (per
  `docker/AGENTS.md`). Flag off removes the binary; `~/.config/mono` is left alone.
- Register the objective in `docs/mbo/index.md`; spec and plan follow.
- Ships as its own objective / PR, not part of PR #404. The gsl changes can land first as a
  separate PR, since they stand alone.

## 5. Risks & blast radius

- **Terminal emulation fidelity** is the main risk: htop and nvtop use colors, the alternate
  screen, and cursor addressing. A weak emulator shows garbage. Mitigated by the spike in 4.6
  before committing to a library.
- **Rendering cost:** composing three panes at refresh rate. Redraw only on child output or
  timer ticks, and cap frame rate (builder: ~30 fps).
- **gsl is the Claude Code status line.** The `video` segment and per-segment flags touch a
  hot path; a slow GPU read or flag lookup must never delay it (cache + lock + existing time
  budget; failure = segment hidden, flags = on).
- **PowerShell processes on WSL.** One detached refresher at a time, killed on timeout (~5 s).
- **Orphaned children** if mono crashes: children run in their own process group and get
  SIGHUP when the pseudo-terminal closes.

## 6. Rollback

- `gff set install.tools.mono false` + re-run install removes the binary; nothing else
  depends on mono.
- `gff set gsl.section.video false` hides the new segment everywhere without a rebuild.
- Each piece (gsl segment + flags, mono) is its own PR and reverts independently.

## 7. Evidence expectations

- **Unit tests:** layout math (sizes → rows at 24/40/60/80 rows, dropped panes rescaled, minimum
  one row), capability selection with fake `PATH` and probes, config get/set/validation and
  atomic writes, prefix state machine (`ctrl+b` then each key; `ctrl+b ctrl+b` passes
  through; every other key reaches the child), keymap ↔ help ↔ `--help` ↔ README agreement.
- **PTY integration test:** a tiny fake program in a pane; assert keystrokes reach it and its
  output lands in the right rows, and that resize reaches it.
- **gsl:** `video` segment renders from fixture data for each source; stale-cache path starts
  exactly one refresher; per-segment flags hide segments; status line unchanged when all
  flags are on and no GPU is found.
- **Real-terminal transcripts** (tmux `capture-pane`, per GUIDE §9) under
  `docs/mbo/plans/mono/evidence/`: the WSL laptop (htop + gsl, nvtop dropped, `[video]` from
  Windows counters) and an NVIDIA host (all three panes, `[video]` from `nvidia-smi`), the
  help overlay, and `mono config` editing a pane.
- `make lint-portability`, shellcheck, `go test ./...` with the sdk coverage floor.

> Produced via `superpowers:brainstorming`. Register in `../index.md`; spec goes in
> `../specs/mono.md`.
