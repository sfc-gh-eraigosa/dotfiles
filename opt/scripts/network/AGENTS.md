# Networking & Remote Access (opt/scripts/network)

- `proxy.sh`: Script for managing proxy settings.
- `remote-setup.sh`: Sets up remote environment.
- `ssh-find`: Helper for finding SSH hosts.
- `sshd_run.sh`: Starts an already-installed SSH daemon. Start-only and gated by `install.sh` on the gff flag `install.network.sshd`; it no longer reads `~/.sshd.env` (migrated by `opt/lib/sshdflags.sh`). Install path: `opt/bin/sshd-setup`.
- `sshd_harden.sh`: **The dotfiles sshd hardening drop-in** (gff `install.network.sshd-hardening`, config phase, **default on**). Writes `/etc/ssh/sshd_config.d/10-dotfiles-hardening.conf` (`PasswordAuthentication no`, `KbdInteractiveAuthentication no`, `PermitRootLogin no`, `PermitEmptyPasswords no`, `MaxAuthTries 3`, `LoginGraceTime 30`, `AllowUsers <this user>`; `SSHD_ALLOW_USERS` widens or `-` omits), runs `sshd -t` before a reload and rolls back on failure (exit 1), prints the effective `sshd -T` values and warns when an earlier directive wins. **Refuses** (exit 2, nothing written) while this user has no key in `~/.ssh/authorized_keys`; SKIPs without sshd, without an `Include sshd_config.d` line, or off Linux. `--dry-run`, `--status`, `--uninstall`. Idempotent. Companion test: `sshd_harden_test.sh`. Docs: `docs/sshd-hardening.md`.
- `import-cert.sh`: Imports SSL certificates.
- `vault-login.sh`, `vault-setup.sh`: HashiCorp Vault management.
