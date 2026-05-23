# System & Environment Setup (opt/scripts/system)

This directory contains scripts for configuring the operating system, environment variables, and base development tools.

## Scripts

- `gemini_install.sh`: Installs and configures the Gemini CLI and environment.
- `install_gemini_skills.sh`: Sets up specialized skills for the Gemini CLI.
- `install_sops.sh`: Installs the `sops` secrets-management binary into `~/opt/bin` (Linux/WSL fetches the official release; macOS uses the Brewfile).
- `nvm`: Helper for Node Version Manager setup.
- `google-cli-setup.sh`: Configures the Google Cloud SDK.
- `setup_jtop.sh`: Installs and configures jetson-stats (jtop) for NVIDIA Jetson devices.
- `terminal-theme.sh`: Configures terminal colors and themes.
- `perf-toggle.sh`: Toggles system performance modes (e.g., on Jetson).
- `enable-vmx.sh`: Helper to check/enable virtualization support.
- `coco_install.sh`: Installer for the COCO dataset tools or similar.
- `crouton-alias.sh`: Aliases for Crouton (Chromebook) environments.

## Environment Profile

The `gemini_install.sh` script generates `~/.gemini.profile`, which is sourced by your shell to provide Gemini-specific aliases and functions.
