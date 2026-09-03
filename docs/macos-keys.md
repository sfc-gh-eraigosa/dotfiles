# macOS-Style Keyboard, Everywhere

One set of muscle memory across every machine in this repo: `Cmd+C` copies on the
Windows desktop, on the Linux desktop, and on the Mac. **This is on by default** —
see [the README section](../README.md#-macos-style-keyboard-on-by-default) for the
user-facing summary, and *Turning it off* below for the single kill switch.

`Super` is the Command key. `Ctrl` is deliberately left alone, so `Ctrl+C` remains
SIGINT in a terminal.

## Platform coverage

| OS | Mechanism | Component | Scope |
| :-- | :-- | :-- | :-- |
| **Windows** | AutoHotkey v2 | `opt/Desktop/Apps/scripts/macos.ahk` | All apps |
| **Linux** — in-app keys | [keyd](https://github.com/rvaiya/keyd) (evdev) | `opt/scripts/system/macos-keys-linux.sh` | All apps |
| **Linux** — desktop actions | `gsettings` | `opt/scripts/system/gnome-desktop-defaults.sh` | GNOME |
| **macOS** | — | none needed | Native |

### Why two components on Linux

They operate at different layers, and each can only do its half:

- **keyd** rewrites key events at the **evdev layer, below X11/Wayland**. That is
  the only way `Cmd+C` can work inside Firefox, VS Code, Nautilus, and Electron
  apps — none of which consult GNOME's shortcut settings. It cannot perform
  window-manager actions.
- **gsettings** owns the **desktop actions** (`Cmd+Tab`, `Cmd+Space`, `Cmd+M/H`).
  keyd deliberately forwards those chords through as a real `Super` combination so
  GNOME can act on them.

`gsettings` alone was tried first and is not enough: it reaches GNOME's own
shortcuts and `gnome-terminal`, and nothing else. `xmodmap`/`setxkbmap
ctrl:swap_lwin_lctl` cannot be made application-aware and steals `Ctrl` outright.

## The key map

| Shortcut | Action | Implemented by |
| :--- | :--- | :--- |
| `Cmd+C` / `V` / `X` | Copy / paste / cut | keyd |
| `Cmd+A` / `Z` / `Shift+Z` | Select all / undo / redo | keyd |
| `Cmd+S` / `Shift+S` | Save / save as | keyd |
| `Cmd+F` / `G` / `Shift+G` | Find / next / previous | keyd |
| `Cmd+O` / `N` / `P` | Open / new / print | keyd |
| `Cmd+T` / `Shift+T` / `W` | New tab / reopen closed / close | keyd |
| `Cmd+1`…`9` | Jump to tab N (`Alt+N` on Linux) | keyd |
| `Cmd+R` | Reload | keyd |
| `Cmd+L` | **Lock screen** | GNOME (passed through) |
| `Cmd+←` / `→` | Start / end of line | keyd |
| `Cmd+↑` / `↓` | Top / bottom of document | keyd |
| `Cmd+Shift+`arrows | Select while moving | keyd (Shift composes) |
| `Cmd+Backspace` | Delete to start of line | keyd macro |
| `Opt+Delete` | Delete previous word | keyd `[alt]` layer |
| `Ctrl+←` / `→` | Previous / next word (Linux-native) | — |
| `Cmd+[` / `]` | Browser back / forward | keyd |
| `Cmd+Q` | Quit application (`Alt+F4`) | keyd |
| `Cmd+Tab` | Switch application (hold to cycle) | GNOME (passed through) |
| `Cmd+Space` | Spotlight → Activities search | GNOME (passed through) |
| `Cmd+M` / `H` | Minimize / hide | GNOME (passed through) |
| `Cmd+Opt+`arrows | Tile / maximize window | keyd → `Ctrl+Alt` → GNOME |
| tap `Cmd` alone | Nothing (as on macOS) | `overlay-key` cleared |

### Why the layer is `[cmd:M]` and not `[cmd:C]`

A `[cmd:C]` layer inherits `Ctrl`, so every unlisted key becomes `Ctrl+<key>` for
free. It is shorter, it was tried first, and **it breaks hold-to-cycle**: with
`:C` the physical `Super` is consumed to enter the layer, so `tab = M-tab` can
only emit a *discrete* `Super+Tab` — pressed and released in one shot. GNOME's
application switcher stays open only while `Super` is genuinely held, so `Cmd+Tab`
flashed the switcher and dismissed it instead of cycling. No remapped output can
express "held for as long as the user holds the key".

`[cmd:M]` keeps `Meta` actually held, so every key **not** listed in the layer
reaches GNOME as a true `Super` chord and behaves as GNOME intends. `tab`,
`space`, `m`, `h` and `l` are therefore deliberately left unmapped. The price is one
line per editing key instead of inheritance.

Per `keyd(1)`, *"bindings are not affected by the modifiers of the layer in which
they are defined"* — so `c = C-c` emits `Ctrl+C` with no `Meta` attached, and
`left = home` emits a bare `Home`. A still-held `Shift` **is** applied to the
output, which is why `Cmd+Shift+←` produces `Shift+Home` and `Cmd+Shift+Z`
produces redo, with no bindings of their own.

### Two deliberate departures from the Windows map

| macos.ahk on Windows | Here | Why |
| :-- | :-- | :-- |
| `Cmd+L` focuses the address bar | `Cmd+L` **locks the screen** | Locking the machine is worth more than a browser shortcut. `Ctrl+L` still focuses the address bar. |
| `Opt+←`/`→` move by word | left to the browser's Back/Forward | On Linux that chord *is* Back/Forward. `Ctrl+←`/`→` already moves by word natively, and `Cmd+[`/`]` gives Back/Forward on the real macOS keys. |

## Terminals are a special case

`Cmd+<key>` becoming `Ctrl+<key>` is correct in a GUI app and **dangerous in a
terminal**, where several of those chords are control codes:

| Chord | Naive result | Consequence | What we do instead |
| :-- | :-- | :-- | :-- |
| `Cmd+C` | `Ctrl+C` | **SIGINT** — kills the running job | `Ctrl+Shift+C` (copy) |
| `Cmd+Z` | `Ctrl+Z` | SIGTSTP — suspends the job | inert |
| `Cmd+S` | `Ctrl+S` | XOFF — **freezes the terminal** | inert |
| `Cmd+D` | `Ctrl+D` | EOF — logs the shell out | inert |
| `Cmd+A` | `Ctrl+A` | readline start-of-line | `Ctrl+Shift+A` |

These overrides live in `opt/etc/keyd/app.conf` and are applied by
`keyd-application-mapper`, which watches the focused window.

**This is why the installer is fail-closed.** A machine where keyd runs but the
mapper does not is strictly *worse* than a stock machine: you lose copy and gain a
stray interrupt. `macos-keys-linux.sh` therefore verifies the mapper's
window-detection backend **before** installing any keyd config, and rolls the
whole thing back if the mapper cannot reach keyd's socket.

## Turning it off

One flag, every OS:

```bash
gff set keyboard.macos.enabled false && ./install.sh
```

That skips the Linux keyd layout, the GNOME gsettings, the Windows AutoHotkey
logon task, and the PowerToys Copilot-key remap. Per-component flags still exist
if you only want to drop one piece:

| Flag | Covers |
| :-- | :-- |
| `keyboard.macos.enabled` | **Everything below, on every OS** |
| `install.desktop.macos-keys` | Linux keyd layout |
| `install.desktop.gnome-keys` | Linux GNOME gsettings |
| `install.windows.ahk-autostart` | Windows `macos.ahk` logon task |
| `install.windows.copilot-key` | Windows Copilot-key remap |

To remove it from a machine that already has it:

```bash
~/opt/scripts/system/macos-keys-linux.sh --uninstall
```

## Recovery

| Situation | Fix |
| :-- | :-- |
| A remap misbehaves, need a stock keyboard **now** | Hold **Backspace + Esc + Enter** together — keyd's built-in panic chord |
| Keyboard unusable, you have SSH | `sudo systemctl stop keyd` |
| Want to know what state you are in | `~/opt/scripts/system/macos-keys-linux.sh --doctor` |
| Undo everything permanently | `…/macos-keys-linux.sh --uninstall` |

`--uninstall` restores `/etc/keyd/default.conf.pre-dotfiles` if the installer
found a pre-existing config to back up.

## Troubleshooting

**`Cmd+C` sends SIGINT in my terminal.** The application mapper is not running or
cannot reach keyd. Run `--doctor`; if "mapper running" is `no`, or the log at
`~/.config/keyd/app.log` shows `Failed to connect`, you are probably not yet in
the `keyd` group — the installer adds you, but an already-open session does not
pick up a new group until you log out and back in.

**Every Cmd shortcut stopped working right after `install.sh` / `fleet update`
re-ran.** Run `--doctor`: if keyd is `inactive` and both configs are `absent`, the
installer rolled itself back. Before 2026-09-02 it judged an *already-running*
mapper by that mapper's cumulative `app.log`, which collects keyd's IPC error every
time the socket blinks — including the `systemctl restart keyd` the installer
itself performs — so a re-run on a healthy host could read old errors and
uninstall everything. The installer now stops any running mapper (including a
legacy `~/.config/autostart/keyd-application-mapper.desktop` instance, which runs
without the `keyd` group and can never connect) and judges only the fresh
supervised one. Recovery is simply re-running `macos-keys-linux.sh`.

**`Cmd+V` prints `^V` in gnome-terminal (and `Cmd+C` may interrupt).** The mapper
is fine; gnome-terminal's *own* copy/paste accelerators are no longer the stock
`Ctrl+Shift+C/V` that `app.conf` emits. A pre-keyd version of
`gnome-desktop-defaults.sh` bound them to `<Super>c/v`, which keyd now consumes
before the terminal sees it, and VTE writes the raw control code when no
accelerator matches. Check with `dconf dump /org/gnome/terminal/legacy/keybindings/`
and re-run `~/opt/scripts/system/gnome-desktop-defaults.sh`, which resets the
stale binds.

**herdr says "copied to clipboard" but `Cmd+V` pastes something older.** Not a
keyd problem: the chord arrives as `Ctrl+Shift+V` and gnome-terminal dutifully
pastes the X clipboard — herdr just never wrote to it. herdr's Linux clipboard
path is `wl-copy` → `xclip` → `xsel`, and only when none of those exists does
it fall back to the OSC 52 escape, which gnome-terminal/VTE silently drops
([VTE #2495](https://gitlab.gnome.org/GNOME/vte/-/issues/2495),
[herdr #2399](https://github.com/herdrdev/herdr/issues/2399)). `packages.tsv`
installs `xclip` and `xsel` on every apt host for exactly this reason; on a
machine provisioned before that, `sudo apt-get install xclip xsel` fixes the
running herdr immediately (no restart — it resolves the tool per copy). On a
Wayland session install `wl-clipboard` instead. Holding `Shift` while
drag-selecting bypasses herdr's mouse capture and copies through gnome-terminal
itself, if you need a one-off without installing anything.

**Nothing is remapped at all.** `systemctl is-active keyd`, then
`sudo keyd check` to validate the config. Note that keyd **rejects a trailing
comment on a binding line** (`c = C-c  # copy`) with `invalid key or action` and
silently drops that binding — keep comments on their own lines. The test driver
guards this.

**I'm on Wayland.** The Xlib backend cannot see native Wayland windows, so the
mapper needs keyd's GNOME shell extension instead:

```bash
ln -s /usr/local/share/keyd/gnome-extension-45 \
      ~/.local/share/gnome-shell/extensions/keyd
gnome-extensions enable keyd   # then log out and back in
```

Until that is in place the installer refuses to remap anything, by design. On an
X11 session no extension is needed — the installer hides `XDG_CURRENT_DESKTOP`
from the mapper so it selects the dependency-free Xlib backend.

**A GNOME `Super+<key>` shortcut stopped working.** Only keys listed in the
`[cmd:M]` layer are taken over; anything else still reaches GNOME normally. The two
useful defaults that *are* taken (arrows for tiling, `L` for lock) are re-homed onto
the `[cmd+alt]` composite layer: tiling is `Cmd+Opt+`arrows, lock is `Cmd+Opt+L`. To
rescue another, add `<key> = M-<key>` to that layer — or simply delete its line from
`[cmd:M]`, which hands the chord straight back to GNOME.

**I want to change a binding.** Edit `opt/etc/keyd/default.conf` (system-wide) or
`opt/etc/keyd/app.conf` (per-application) in the repo, then re-run
`macos-keys-linux.sh`. Never edit the installed copies — they are overwritten.

### A crash worth knowing about

The obvious way to hand a chord back to GNOME from the `[cmd+alt]` composite layer
is to emit `M-<key>`. **Do not.** The emitted `Meta` re-enters the Meta-triggered
`[cmd:M]` layer, which emits `Meta` again, and keyd recurses until it dies —
observed on keyd v2.6.0 as a `SIGSEGV` with a 147 MB memory peak, eleven seconds
after loading such a config.

`sudo keyd check` does **not** catch it: the config parses perfectly and only
crashes once a key is pressed. So the composite layer emits a plain `Ctrl+Alt`
chord instead, and `gnome-desktop-defaults.sh` points GNOME's tiling and lock
shortcuts at `Ctrl+Alt` to receive it. `macos-keys-linux_test.sh` asserts that no
`M-` emission ever reappears in a Meta-triggered layer.

A crashed keyd leaves a **stock keyboard**, which is the safe direction — but the
installer also adds a `Restart=on-failure` systemd drop-in (with a burst limit, so
a genuinely crash-looping keyd gives up rather than thrashing).

## Implementation notes

- keyd is pinned to a tag and built from source (`~/.cache/dotfiles/keyd`); it is
  not packaged for Ubuntu 24.04. It is plain C and builds in ~1.5s on arm64.
- The daemon runs as root and grabs `/dev/input`. It only sees **physical**
  keyboards, so keystrokes injected by a remote-desktop server (RDP/VNC via
  XTEST) bypass it entirely.
- Verified on: NVIDIA DGX Spark, Ubuntu 24.04.4 aarch64, GNOME Shell 46, X11.
