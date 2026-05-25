# Scripts

Personal automation scripts. This folder lives alongside the `AutoHotkey` folder
in `OneDrive\Documents`, and is the canonical home for reusable scripts. A
shortcut to this folder ("Scripts") sits on the Desktop for quick access.

## Contents

| Script | Purpose |
|--------|---------|
| `setup-apps.ps1` | Full machine provisioning: WSL + Ubuntu distros, Ubuntu Mono font, Windows Terminal themes/profiles, and a standard set of **winget** desktop apps. Every phase is idempotent. A `-Status` mode reports WSL + app state. |

## setup-apps.ps1

Runs six idempotent phases in order:

1. **WSL** — installs the WSL platform if missing (skips if already set up). Installing the platform on a machine that lacks it needs an elevated run + reboot; everything else works un-elevated.
2. **Distros** — ensures exactly two Ubuntu distributions exist and creates the Linux user:
   - `Ubuntu` — Ubuntu 26.04 LTS ("Latest", pre-existing)
   - `Ubuntu-24.04` — Ubuntu 24.04 LTS ("LTS", installed with user `wenlock` + passwordless sudo)
3. **GitHub link** — symlinks the Windows GitHub folder into each distro at `~/GitHub`. It follows the Windows symlink (`C:\Users\edwar\GitHub` → `C:\Users\edwar\OneDrive\GitHub`) to its real OneDrive target, so the link works for sync.
4. **Font** — (re)installs **Ubuntu Mono** per-user and clears any stale font-registry entry, fixing Terminal's "Unable to find the following fonts: Ubuntu Mono" warning.
5. **Windows Terminal** — adds five color schemes (Solarized Dark, Solarized Light, Ocean, Green, GitHub Dark) and one profile per scheme **per distro** (10 profiles), each with a 100,000-line scrollback and the Ubuntu Mono font. settings.json is backed up first (`settings.json.bak-<timestamp>`).
6. **Apps** — winget-installs the list below, skipping anything already present.

The two-distro pairing, the Linux username, the GitHub folder, the font, the scrollback size, and the theme list are all variables at the top of the script.

### Installed applications

- Discord
- Slack
- Obsidian
- OBS Studio
- Spotify
- Apple iTunes
- Antigravity
- Antigravity IDE
- Cursor
- Claude
- GitHub Desktop
- Docker Desktop
- Google Chrome
- Mozilla Firefox

### Run it

From a **normal (non-elevated)** PowerShell window:

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\OneDrive\Documents\Scripts\setup-apps.ps1"
```

> Run un-elevated: some installers (e.g. Spotify) refuse to install as admin.

The script prints a per-app result and a summary at the end
(`Installed` / `Already installed` / `FAILED`).

### Check what's installed

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\OneDrive\Documents\Scripts\setup-apps.ps1" -Status
```

Reports the WSL version and installed distros, then a table of each app with
its installed state, version, and install folder location. Installs nothing.

### Add more apps

Find the package ID:

```powershell
winget search "<app name>"
```

Then add an entry to the `$apps` block near the top of `setup-apps.ps1`:

```powershell
'Friendly Name' = @{ Id = 'Publisher.Id'; Match = 'RegistryDisplayName' }
```

- `Id` — the winget package id (required).
- `Match` — regex matched against the app's registry *DisplayName*, used by
  `-Status` to find the install folder. Defaults work for most apps.
- Optional `Exe = 'app.exe'` — fallback that resolves the folder via the
  registered executable's App Paths entry (used for Chrome).
- Optional `Appx = 'PackageName'` — fallback for Store/MSIX apps (used for Claude).

Re-running the script installs only the new entries.

## Requirements

- Windows with **winget** (App Installer) available — check with `winget --version`.
