# opt/etc — tracked system configuration files

Configuration that a script **copies into a system or user location**, kept in the
repo as the single source of truth. Nothing here is read from this path at
runtime; nothing here is symlinked into place.

**Edit the file here, then re-run its installer.** The deployed copies are
overwritten on every install, so a hand-edit to `/etc/...` is always lost.

## `keyd/` — macOS-style keyboard layout

Deployed by [`opt/scripts/system/macos-keys-linux.sh`](../scripts/system/macos-keys-linux.sh).
Full documentation: [docs/macos-keys.md](../../docs/macos-keys.md).

| File | Deployed to | Purpose |
| :-- | :-- | :-- |
| `default.conf` | `/etc/keyd/default.conf` | System-wide layout: `Super` becomes Command |
| `app.conf` | `~/.config/keyd/app.conf` | Per-application overrides, applied by `keyd-application-mapper` |

### Editing rules for these two files

- **Never put a comment on the same line as a binding.** keyd's parser reports
  `invalid key or action` and *silently drops that binding*. Comments go on their
  own line. `macos-keys-linux_test.sh` asserts this for the whole file.
- **The layer must stay `[cmd:M]`, never `[cmd:C]`.** `:C` inherits `Ctrl` and
  saves ~25 lines, but it consumes the physical `Super`, so `Cmd+Tab` can only emit
  a discrete chord and GNOME's switcher will not hold open to cycle. `:M` keeps
  `Meta` held, so any key left out of the layer reaches GNOME as a real `Super`
  chord. `tab`, `space`, `m` and `h` are unmapped **on purpose** — do not "fix"
  them by adding bindings.
- **`app.conf` is a safety mechanism, not a nicety.** In a terminal the naive
  translation makes `Cmd+C` = SIGINT, `Cmd+S` = XOFF (a frozen terminal), and
  `Cmd+D` = logout. Adding a terminal emulator means adding a section for it.
- Validate any change with `sudo keyd check` before committing.
