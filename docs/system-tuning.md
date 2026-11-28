# system-tuning — journal capped at 2G, apt cache emptied weekly (default on)

One `install.sh` step, **on by default** on every Linux host that runs `install.sh` (so
`fleet update` carries it to every node), a one-line flag away from off:

| Flag (`boolDefault: true`) | Script | Touches |
| :-- | :-- | :-- |
| `install.system.tuning` | `opt/scripts/system/setup_system_tuning.sh` | `/etc/systemd/journald.conf.d/10-dotfiles-tuning.conf`, `/etc/apt/apt.conf.d/52dotfiles-tuning` |

## Why

The lab's Raspberry Pi and Jetson nodes boot from SD cards, and the root disk filled with
things nobody reads. On 2026-11-27 one Pi carried a 3.9 GB journal and a 4.3 GB
`/var/cache/apt`: about 8 GB of the card for old logs and `.deb` files that were already
installed. Neither one shrinks unless something empties it.

## What it writes

```ini
# /etc/systemd/journald.conf.d/10-dotfiles-tuning.conf
[Journal]
SystemMaxUse=2G
```

```
// /etc/apt/apt.conf.d/52dotfiles-tuning   (apt hosts only)
APT::Periodic::CleanInterval "7";
```

- **The journal**: journald keeps at most 2 GB of persistent journal, deleting the oldest
  files first. That is still weeks of logs on a busy node. When the drop-in changes, the
  script restarts `systemd-journald` to load it. Every run then calls
  `journalctl --vacuum-size=2G`, so a journal that is already larger shrinks right away
  and doesn't have to wait for journald to rotate.
- **The apt cache**: `apt-daily.timer` (turned on by `install.system.unattended-upgrades`)
  runs `apt-get clean` every 7 days. Installed packages stay as they are. Only the
  downloaded `.deb` files go, and apt can fetch them again if it ever needs them.

## Running it

```bash
opt/scripts/system/setup_system_tuning.sh --dry-run   # the plan; writes nothing
opt/scripts/system/setup_system_tuning.sh --status    # what is in place, journal size
opt/scripts/system/setup_system_tuning.sh             # apply (idempotent)
ST_JOURNAL_MAX=4G opt/scripts/system/setup_system_tuning.sh   # a different cap
gff set install.system.tuning false                   # leave this host alone
```

It SKIPs (exit 0) on macOS and anywhere systemd is not running (containers, WSL without
systemd). It does only the journald part on a host without `apt-get`. Before replacing a
file it didn't write, it keeps a copy as `<file>.<timestamp>.bak`. When sudo wants a
password and there is no terminal, it prints one WARNING and exits 0, so an unattended
`fleet update` never stalls.

Test: `bash opt/scripts/system/setup_system_tuning_test.sh` (sandboxed: fake `systemctl`,
`journalctl` and `apt-get`, nothing on the host is touched).
