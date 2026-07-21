# Local Binaries (opt/bin)

This directory is reserved for compiled binaries and local executable artifacts. It is ignored by Git to prevent repository pollution.

## Binaries

These tools are built from source (typically in `src/`) and deposited here during installation or updates:

- `gss`: Git Safe Sync.
- `tmux-mgr`: Tmux Management tool.
- `wol`: Wake-on-LAN utility.
- `discuss`: Communication tool.
- `vault`: HashiCorp Vault binary.
- `git-sizer`: Git repository analyzer.
- `docker` / `docker-compose`: **Committed WSL shims, not binaries.** Real executables so non-interactive shells (make, scripts) resolve a working docker — the `.bash_aliases` docker.exe fallback is an alias and only exists interactively. Resolution order: Docker Desktop's WSL-integration Linux CLI → any other real docker on PATH → `docker.exe` over interop (with its credential helper dir appended to PATH). `docker-compose` delegates to `docker compose` (v2).
- `docker-credential-desktop.exe`: **Committed WSL shim.** Docker Desktop's integration writes `credsStore: "desktop.exe"` into `~/.docker/config.json` on every init and symlinks `/usr/bin/docker-credential-desktop.exe` into the `/Docker/host/bin` mount — which never appears on some machines, so every build/pull dies with "error getting credentials". This wrapper (ahead of `/usr/bin` on PATH) forwards the credential-helper protocol to the real Windows helper over interop.

## Usage

This directory should be in your `$PATH`. It is populated by the `./install.sh` script or individual project build scripts.
