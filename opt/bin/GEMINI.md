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

## Usage

This directory should be in your `$PATH`. It is populated by the `./install.sh` script or individual project build scripts.
