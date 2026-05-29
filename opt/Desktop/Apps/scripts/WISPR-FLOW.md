# Wispr Flow (voice dictation)

[Wispr Flow](https://wisprflow.ai) is now the voice-dictation engine on Windows.
It listens on a global hotkey and types the transcribed text into whatever field
has focus. It **replaces** the old AutoHotkey "Copilot-key → Windows Voice Typing
→ Claude Desktop" macro, which has been retired to
[`archive/macos-copilot-claude-voice.ahk`](../../../../archive/macos-copilot-claude-voice.ahk).

## What changed

| Before | After |
|--------|-------|
| Copilot key caught by `macos.ahk` (`*F23::ClaudeKey()`) | Copilot key left **unbound** in AHK; bound inside Wispr Flow |
| Tap → focus Claude Desktop + Win+H; tap → Enter | Flow dictates into the focused field directly |
| Windows Voice Typing (local) | Wispr Flow (cloud) |
| Lived in `macos.ahk` | App installed by `install-wisprflow.ps1`; macro archived |

`macos.ahk` still provides the macOS-style Cmd shortcuts, screenshots, and hot
corners — only the voice block was removed.

## Install

**Automatic** — `install.sh` runs the Windows desktop step
(`opt/bin/install_windows.sh`), which calls `install-wisprflow.ps1` when you opt
into Windows customization.

**Manual / re-run** — from a normal PowerShell window:

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\OneDrive\Documents\Scripts\install-wisprflow.ps1"
```

The installer is idempotent (skips if already installed), pins a known-good
version, and falls back to "latest" if the pin ages out. Useful switches:

| Switch | Effect |
|--------|--------|
| `-Status` | Report whether Flow is installed, and its version |
| `-Latest` | Resolve + install the latest version |
| `-Version 1.5.530` | Install a specific version |
| `-Force` | Reinstall even if present |
| `-Uninstall` | Silently uninstall (uses the registered product code) |

> Not on winget or the Store — Flow ships its own machine-wide MSI, so the script
> downloads and `msiexec`-installs it. A UAC prompt appears for the elevated MSI.

## One-time manual setup (cannot be scripted)

Wispr's own MDM guide states preferences can't be injected, so these are manual,
once per machine:

1. **Sign in** — launch Wispr Flow; sign in via the browser (Google/Microsoft/SSO/email).
2. **Microphone** — Windows Settings → Privacy & Security → Microphone → allow desktop apps.
3. **Bind the Copilot key** — open Flow's tray icon → **Edit shortcut** → press the
   **Copilot key** once. This makes the same physical key you used before trigger
   Flow.
4. **Start at login** — enable it in Flow (the Run-key mechanism isn't scriptable).

### If Flow won't accept the Copilot key (fallback shim)

The Copilot key emits `LWin+LShift+F23`. Flow documents function keys only up to
F12, so it may reject `F23`, or Windows may swallow the chord before Flow sees it.
If "Edit shortcut" won't capture the Copilot key, add a one-line shim to
`macos.ahk` that translates it into a combo Flow *does* accept, then bind that
combo in Flow instead:

```ahk
; Copilot key -> a Flow-friendly combo (bind Ctrl+Alt+F12 inside Wispr Flow)
*F23::Send "^!{F12}"
```

Put it back where the old voice block was (just below the Screenshots section),
re-deploy (`install.sh`), and restart AutoHotkey. In Flow: Edit shortcut →
press `Ctrl+Alt+F12`.

## Migrating a machine that had the old macro

Run the cleanup helper once. It finds a deployed `macos.ahk` that still has the
voice block, backs it up locally (`~/.config/dotfiles/ahk-voice-backups/`),
re-deploys the cleaned `macos.ahk`, and restarts AutoHotkey:

```bash
opt/scripts/system/retire-ahk-voice-macro.sh            # clean if found
opt/scripts/system/retire-ahk-voice-macro.sh --dry-run  # preview only
```

It's a no-op (with a message) if nothing old is found or you're not on WSL.

## Caveats

- **Cloud-only** — Flow requires an internet connection; there is no offline
  transcription. (Windows Voice Typing worked locally — this is a behaviour change.)
- **Plan limits** — the free tier is ~2,000 words/week; Pro is a paid subscription.
  Check current limits at <https://wisprflow.ai/pricing>.
- **Account required** — first launch needs an interactive sign-in.
- **x64 Windows 10/11 only** — no ARM build.

## Restore the old AHK macro

See [`archive/README.md`](../../../../archive/README.md). In short: paste the body
of `archive/macos-copilot-claude-voice.ahk` back into `macos.ahk`, unbind the
Copilot key in Flow, re-deploy, and restart AutoHotkey.

## Sources

- Deploy via MDM / MSI: <https://docs.wisprflow.ai/articles/9363440133-deploy-wispr-flow-via-mdm>
- Setup guide (sign-in, Edit shortcut): <https://docs.wisprflow.ai/articles/3152211871-setup-guide>
- Hotkey rules: <https://docs.wisprflow.ai/articles/2612050838-supported-unsupported-keyboard-hotkey-shortcuts>
- System requirements: <https://docs.wisprflow.ai/articles/1036674442-supported-devices-and-system-requirements>
- Copilot key = LWin+LShift+F23: Microsoft PowerToys Keyboard Manager docs.
