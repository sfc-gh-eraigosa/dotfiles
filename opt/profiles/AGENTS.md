# Shell Profiles & Configurations (opt/profiles)

This directory contains the core configuration files for the shell environment, editors, and various tools.

## Shell Configuration

- `.zshrc`: Main configuration for Zsh.
- `.bashrc`, `.bash_aliases`, `.bash_logout`: Configuration for Bash.
- `.profile`: General shell profile.
- `.p10k.zsh`: Configuration for the Powerlevel10k Zsh theme.
- `.inputrc`: Readline configuration (key bindings).

## Tool Specific Configurations

- `.tmux.conf`: Tmux configuration.
- `.vimrc_default`, `.vimrc_editor.vim`, `.vimrc_green`, `.vimrc_white`: Various Vim configurations.
- `.docker.sh`: Shell helpers for Docker.
- `.goenv.sh`: Environment setup for Go.
- `.locale.sh`: `locale_fallback [-q]` — drops a forwarded LC_ALL / LC_* / LANG this host never generated (ssh `SendEnv LANG LC_*`) and falls back to C.UTF-8. POSIX sh (dash reads `.profile` at GUI login); called quietly by `.profile`, `.bashrc`, `.zshrc`, and by `install.sh`. Any script can `. ~/.locale.sh && locale_fallback`.
- `.gitrepos`: Likely a list of managed git repositories.
- `.repos.env`: Environment variables related to repositories.
- `.ruby-version`: Specifies the Ruby version for the project.

## Package Management

- `Brewfile`: List of Homebrew packages to install.
- `requirements.txt`: Python dependencies.
- `versions.tsv`: the **version-pin manifest** for tools install.sh downloads (Go, uv, glow, rust, sops, yq, gitleaks, kubectl/helm/kind, herdr, keyd, Nerd Fonts, vault). Each row has a track (`pin`/`floor`/`latest`), a version (or `@file`), the upstream, the installer and a version probe. Installers read it through `opt/lib/pins.sh` (`pins_want`). Check and bump with `fleet pins status|bump|apply` (see `sdk/fleet/README.md`). Design: `docs/mbo/designs/pins.md`.

## Desktop & X11

- `.xsessionrc`: Configuration for X session.
- `.pam_environment`: PAM environment variables.

## Usage

These files are typically symlinked to the user's home directory during setup. When modifying these, ensure they are compatible across target systems (Darwin, Linux).
