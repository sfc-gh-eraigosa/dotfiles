# Networking & Remote Access (opt/scripts/network)

- `proxy.sh`: Script for managing proxy settings.
- `remote-setup.sh`: Sets up remote environment.
- `ssh-find`: Helper for finding SSH hosts.
- `sshd_run.sh`: Starts an already-installed SSH daemon. Start-only and gated by `install.sh` on the gff flag `install.network.sshd`; it no longer reads `~/.sshd.env` (migrated by `opt/lib/sshdflags.sh`). Install path: `opt/bin/sshd-setup`.
- `import-cert.sh`: Imports SSL certificates.
- `vault-login.sh`, `vault-setup.sh`: HashiCorp Vault management.
