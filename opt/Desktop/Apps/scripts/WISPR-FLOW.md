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

There is **no fully unattended install**: Wispr Flow ships a machine-wide MSI
that requires elevation (a UAC prompt), which can't be driven from the unattended
WSL provisioning context. So `install.sh` / `install_windows.sh` does **not**
auto-install it — it prints a pointer to this runbook and the installer.

**Run the installer interactively** — from a normal Windows PowerShell window
(it self-elevates; approve the UAC prompt). The scripts are deployed to your
Desktop under `Apps\scripts\`:

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\Desktop\Apps\scripts\install-wisprflow.ps1"
```

The installer is idempotent (skips if already installed), pins a known-good
version, caches the download, and falls back to "latest" if the pin ages out.
Useful switches:

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
3. **Move ALL THREE Flow shortcuts off the Win key** — Flow → **Settings →
   General → Shortcuts**. Every shortcut defaults to a `Ctrl+Win` combo, and Flow's
   hook on the **Win** key is what breaks `macos.ahk` (see the warning below). You
   must change **all three** — leaving even one on a `Win` combo re-breaks
   `Cmd+C`/`Cmd+V`:
   - **Push-to-talk** → **`Ctrl+Shift+F12`**. This is the combo the Copilot key emits
     via the `macos.ahk` shim, so the physical Copilot key then drives dictation.
   - **Hands-free** → any combo that does **not** use Win (e.g. `Ctrl+Shift+F11`).
   - **Command mode** → any combo that does **not** use Win (e.g. `Ctrl+Shift+F10`).
   This step **cannot be scripted**: Flow keeps these in a binary, cloud-synced
   settings store (`…\Packages\WisprFlow.WisprFlow_*\Settings\settings.dat`), and
   Wispr's MDM guide says preferences can't be injected — so it's a manual, once-per
   -machine step.
4. **Start at login** — enable it in Flow (the Run-key mechanism isn't scriptable).

> ⚠️ **Critical if you use `macos.ahk` (macOS-style shortcuts).** All of Wispr Flow's
> **default** shortcuts on Windows are **`Ctrl+Win`**-based. To detect them, Flow
> installs a low-level keyboard hook that watches the **Win** key — the exact key
> `macos.ahk` remaps as **Cmd** for every shortcut. With Flow on its defaults, the
> two fight over Win and your `Cmd+C`/`Cmd+V`/etc. silently break (they type a
> literal letter or trigger Windows' own `Win+C`). **Fix:** change **all three** Flow
> shortcuts (push-to-talk, hands-free, command mode) off `Ctrl+Win` per step 3 above.
> Once *no* Flow shortcut uses the Win key, Flow and the macOS layer coexist.

## Suppressing the Copilot key (PowerToys)

> If you run `macos.ahk`, prefer its built-in AHK shim instead of PowerToys (see
> [AutoHotkey shim](#autohotkey-shim-preferred-when-you-run-macosahk) below) —
> PowerToys' keyboard hook conflicts with the Cmd = Left-Win mappings. Use the
> PowerToys route only when you don't run `macos.ahk`.

The Copilot key emits `Win(Left)+Shift(Left)+F23`, which causes two problems:
Windows itself acts on that chord (it launches Windows Copilot) and can swallow it
before Flow sees it, and Flow's **Edit shortcut** often refuses `F23` (its docs
only cover function keys up to F12).

`suppress-copilot-key.ps1` fixes both by remapping the chord to **`Ctrl+Shift+F12`**
in **PowerToys Keyboard Manager** — a combo Flow accepts. Windows Copilot, which
only listens for `Win+Shift+F23`, never fires; the physical key now emits a clean,
bindable combo. (It *remaps* rather than fully disabling, so the key still reaches
Flow — a plain "Disable" would stop Flow from seeing it too.)

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\Desktop\Apps\scripts\suppress-copilot-key.ps1"
```

`install-wisprflow.ps1` runs this automatically at the end of a successful install;
you can also run it standalone any time.

| Switch | Effect |
|--------|--------|
| _(none)_ | Remap `Win+Shift+F23` → `Ctrl+Shift+F12` |
| `-Status` | Report whether PowerToys is installed and whether the remap is in place |
| `-Remove` | Remove the remap and restore default Copilot-key behaviour |

- **Requires PowerToys.** If it isn't installed the script prints a warning plus the
  download link (<https://github.com/microsoft/PowerToys/releases>) and exits
  without error.
- **Idempotent.** Re-running never duplicates the entry; an already-applied remap is
  a no-op and writes no new backup.
- **Non-destructive.** It *merges* into `…\Keyboard Manager\default.json`, preserving
  any other remaps, and backs up the existing file as `default.json.bak-<timestamp>`
  before writing.
- **Restart to apply.** PowerToys reads the file at startup — restart PowerToys (or
  toggle Keyboard Manager off and on) for the remap to take effect, then bind
  `Ctrl+Shift+F12` in Flow.

### AutoHotkey shim (preferred when you run `macos.ahk`)

If you use the macOS-style shortcuts, `macos.ahk` **already** translates the Copilot
key to the Flow combo — and this is the preferred route, because PowerToys' keyboard
hook conflicts with `macos.ahk`'s Cmd = Left-Win mappings (it breaks `Cmd+C` etc.).
The shim already in `macos.ahk`:

```ahk
; Copilot key (LWin+LShift+F23) -> clean Ctrl+Shift+F12 for Wispr Flow.
; '*F23' (no '~') also SUPPRESSES the chord so Windows' "assign Copilot key"
; Settings page never opens. Drop the Win/Shift the key carries first.
*F23::Send "{LWin up}{LShift up}^+{F12}"
```

Bind `Ctrl+Shift+F12` in Flow (Settings → General → Shortcuts). If you pick a
different combo, change both the shim and the Flow binding to match. Use the
PowerToys remap above only if you do **not** run `macos.ahk`.

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
- PowerToys Keyboard Manager (remap shortcuts): <https://learn.microsoft.com/en-us/windows/powertoys/keyboard-manager>
