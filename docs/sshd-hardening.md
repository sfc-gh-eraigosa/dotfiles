# sshd-hardening — passwords off, root out, security updates on (default on)

Two `install.sh` steps, both **on by default** on every host that runs `install.sh` (so
`fleet update` carries them), each a one-line flag away from off on a host that needs it:

| Step | Flag (`boolDefault: true`) | Script | Touches |
| :-- | :-- | :-- | :-- |
| sshd hardening | `install.network.sshd-hardening` | `opt/scripts/network/sshd_harden.sh` | `/etc/ssh/sshd_config.d/10-dotfiles-hardening.conf` |
| unattended security upgrades | `install.system.unattended-upgrades` | `opt/scripts/system/setup_unattended_upgrades.sh` | the `unattended-upgrades` package, `/etc/apt/apt.conf.d/20auto-upgrades`, the two `apt-daily*` timers |

Both are gated with `gff_on` (fail-open: only an explicit `false` skips them), like the rest
of `install.sh`. [`sshd-setup.md`](./sshd-setup.md) is the companion that *turns sshd on*
(opt-in); this is what every sshd that is on gets.

## Why

The 2026-11-05 OpenSSH bug "sshole" (CVE-2026-123158, an authentication bypass for
password-less accounts on musl builds) did not reach any lab host, but the audit that proved
it showed every host on OpenSSH's stock defaults: password login on, root allowed in with a
key, no lockout, and only one of four hosts taking security updates on its own. In 30 days
the fleet had ~21 000 key logins and one password login. The maintainer's advice for the
bug was `PasswordAuthentication no`; that is the setting that covers the next bug too.

## What the sshd drop-in says

```
PasswordAuthentication no        # keys only (the one change that mattered for sshole)
KbdInteractiveAuthentication no  # no PAM prompt fallback either
PermitEmptyPasswords no
PermitRootLogin no               # root never logs in over ssh; sudo from your user
PubkeyAuthentication yes
MaxAuthTries 3                   # a scanner gets three guesses per connection, not six
LoginGraceTime 30                # an idle unauthenticated connection is dropped at 30 s
AllowUsers <the user running install.sh>
```

It is named `10-` so it sorts **before** the distro's `50-cloud-init.conf` (which sets
`PasswordAuthentication yes` on cloud images): in `sshd_config` the first value a keyword
gets wins, and Debian and Ubuntu put the `Include sshd_config.d/*.conf` line at the top of
the file. After every apply the script prints the values `sshd -T` will actually use and
**warns** when one of them is not what the drop-in asked for, which means something earlier
in `sshd_config` is winning; fix that file, do not fight it with a second drop-in.

### The allow-list

`AllowUsers` defaults to the invoking user, because that is who `fleet update` runs as and
who has been logging in. A host where another account needs ssh (a `git` user, a second
person) widens it in the environment of the run:

```bash
SSHD_ALLOW_USERS="$USER deploy" ./install.sh      # a space-separated list
SSHD_ALLOW_USERS=- ./install.sh                   # no AllowUsers line at all
```

Put that in `~/.zshrc.local` ([machine-local-overrides.md](./machine-local-overrides.md))
so the next `install.sh` keeps it.

## Safety

- **It refuses rather than locks you out.** While the invoking user has no key in
  `~/.ssh/authorized_keys`, the script prints `REFUSED`, exits 2, and writes nothing:
  passwords stay on. `install.sh` turns that into a WARNING and continues. Seed a key first
  (`opt/bin/sshd-setup keys` pulls your GitHub keys), then re-run.
- **`sshd -t` before the reload.** A configuration sshd rejects is rolled back (the previous
  file restored, or the drop-in removed) and sshd is never reloaded. Exit 1.
- **Reload, not restart.** Existing sessions, including the one running `install.sh`, stay
  up. Keep a second session open the first time anyway, and test `ssh <host>` from another
  machine before closing it.
- **Idempotent.** An unchanged drop-in means no write and no reload. A file at that path
  that is not the current render is kept as `10-dotfiles-hardening.conf.bak-<timestamp>`.
- **SKIPs where it does not apply**: no sshd, no `sshd_config`, no `Include` of
  `sshd_config.d` (the drop-in would be ignored), or not Linux. Containers and the Docker CI
  build hit one of these and pass through.

## Unattended upgrades

`setup_unattended_upgrades.sh` installs the package, writes `20auto-upgrades`
(`APT::Periodic::Update-Package-Lists "1"` and `Unattended-Upgrade "1"`) and enables
`apt-daily.timer` and `apt-daily-upgrade.timer`. **Which** packages move stays the distro's
default in `50unattended-upgrades`: the release's security origin, plus ESM on Ubuntu Pro.
Third-party repositories (NVIDIA L4T and CUDA, Docker, nodesource, GitHub CLI) are not in
that list, so a Jetson's JetPack and the Spark's CUDA never upgrade on their own. Kernel
security updates do install; reboots are never automatic (the Ubuntu default
`Automatic-Reboot "false"` is untouched), so a host that needs one says so in
`/var/run/reboot-required` and `fleet` shows it.

SKIPs without `apt-get` (macOS, Fedora) or without a running systemd (containers, a WSL
distro without `systemd=true`).

## Operations

```bash
opt/scripts/network/sshd_harden.sh --status        # applied? keys? effective sshd -T values
opt/scripts/network/sshd_harden.sh --dry-run       # the drop-in it would write, no change
opt/scripts/network/sshd_harden.sh                 # apply now (what install.sh runs)
opt/scripts/network/sshd_harden.sh --uninstall     # remove the drop-in and reload

opt/scripts/system/setup_unattended_upgrades.sh --status
opt/scripts/system/setup_unattended_upgrades.sh --dry-run
sudo unattended-upgrade --dry-run -d | tail        # what the next run would install
```

### Turn either off on one host

```bash
gff set install.network.sshd-hardening false      # install.sh skips it from now on
opt/scripts/network/sshd_harden.sh --uninstall    # and take the drop-in away now
gff set install.system.unattended-upgrades false  # the package and timers stay as they are
```

Unsetting a flag alone changes nothing already on the host; it only stops `install.sh`
from applying the step again.

### If you are locked out anyway

You cannot be locked out by this script's own apply (it refused without a key), but a key
you later remove can do it. From the console, or any session still open:
`sudo rm /etc/ssh/sshd_config.d/10-dotfiles-hardening.conf && sudo systemctl reload ssh`.

## Tests

`make shell-test` runs `sshd_harden_test.sh` (47 assertions) and
`setup_unattended_upgrades_test.sh` (30). Both drive the scripts against a sandbox
`/etc` with recording fakes for `sshd`, `systemctl`, `apt-get` and `dpkg-query`; nothing on
the build host is read or written.
