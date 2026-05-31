# Wispr Flow (voice dictation)

[Wispr Flow](https://wisprflow.ai) is the voice-dictation engine on Windows. The
physical **Copilot key** drives it: hold it to dictate into the field your mouse is
hovering, release, and Flow transcribes and pastes the text there. It **replaces**
the old AutoHotkey "Copilot-key → Windows Voice Typing → Claude Desktop" macro,
retired to [`archive/macos-copilot-claude-voice.ahk`](../../../../archive/macos-copilot-claude-voice.ahk).

## How the integration works (read this — it's non-obvious)

Three facts forced an unusual design, discovered the hard way:

1. The Copilot key emits `LWin+LShift+F23`. **Windows opens its "Customize Copilot
   key" Settings page** on that chord, and suppressing it in AutoHotkey is an
   unreliable race (Windows sometimes wins).
2. **Wispr Flow ignores *injected* keystrokes** — so AutoHotkey cannot drive Flow's
   hotkey by `Send`-ing it (verified). No combo, tier, or send-mode works.
3. **Flow *does* accept injected mouse clicks** on its always-visible **"Status"
   overlay**, and it auto-copies + auto-pastes the finished transcript.

So the chain is:

```
Copilot key (Win+Shift+F23)
   → PowerToys Keyboard Manager remaps it to F24      (before Windows sees it: no Settings page, no race)
   → macos.ahk's  *F24  handler:
         on key-down  → save the window+point under the mouse, click Flow's overlay START (2 clicks)
         on key-up    → click the overlay STOP
   → Flow transcribes → clipboard → auto-paste; macos.ahk re-focuses your saved
     target first so the text lands where you were hovering.
```

**Controls** (all in `macos.ahk`):

| Key | Action |
|-----|--------|
| **Copilot key** (hold) | Dictate into the field under the mouse; release to finish |
| **Esc** *(only while dictating)* | Cancel — reset, no paste; normal Esc otherwise |
| **F10** | Toggle the whole dictation flow on/off (centered popup shows ON/OFF) |

> The pieces: **`suppress-copilot-key.ps1`** configures PowerToys (the F24 remap +
> module setup); **`macos.ahk`** does the overlay-click flow; **`install-wisprflow.ps1`**
> installs the Flow app. PowerToys is required.

## Install the Flow app

There is **no fully unattended install**: Wispr Flow ships a machine-wide MSI that
requires elevation (a UAC prompt), which can't be driven from the unattended WSL
provisioning context. So `install.sh` / `install_windows.sh` does **not** auto-install
it — it points here.

**Run the installer interactively** from a normal Windows PowerShell window (it
self-elevates; approve the UAC prompt). Scripts deploy to your Desktop under
`Apps\scripts\`:

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\Desktop\Apps\scripts\install-wisprflow.ps1"
```

Idempotent (skips if installed), pins a known-good version, caches the download,
falls back to "latest" if the pin ages out. Switches: `-Status` / `-Latest` /
`-Version 1.5.530` / `-Force` / `-Uninstall`.

## Configure PowerToys (scripted)

`suppress-copilot-key.ps1` makes the Copilot key reach `macos.ahk` cleanly. It
(idempotent, backs up each file first):

1. writes the KBM remap **`Win+Shift+F23` → `F24`** in PowerToys Keyboard Manager —
   so Windows never opens the Copilot Settings page (KBM consumes the chord first);
2. in PowerToys settings: **enables Keyboard Manager** and **disables FancyZones /
   PowerToys Run / Shortcut Guide** — those grab Win-key shortcuts and would fight
   `macos.ahk`'s Cmd = Left-Win layer;
3. **restarts PowerToys** to apply.

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\Desktop\Apps\scripts\suppress-copilot-key.ps1"
```

`install-wisprflow.ps1` runs it automatically. Switches: `-Status` (report remap +
module state), `-Remove` (drop the F24 remap). **Requires PowerToys** — if absent it
warns + links <https://github.com/microsoft/PowerToys/releases> and exits.

## One-time manual setup (cannot be scripted)

Wispr's MDM guide states preferences can't be injected (its settings live in a
binary, cloud-synced `…\Packages\WisprFlow.WisprFlow_*\Settings\settings.dat`), so:

1. **Sign in** — launch Wispr Flow; sign in via the browser (Google/Microsoft/SSO/email).
2. **Microphone** — Windows Settings → Privacy & Security → Microphone → allow desktop apps.
3. **Move ALL of Flow's shortcuts off the Win key** — Flow → **Settings → General →
   Shortcuts**. Flow's defaults are `Ctrl+Win`-based, and Flow's hook on the **Win**
   key breaks `macos.ahk`'s `Cmd+C`/`Cmd+V`/etc. Change every shortcut to any
   **non-Win** combo (e.g. `Ctrl+Shift+F12` / `F11` / `F10`). The exact combos don't
   matter — **the Copilot key drives Flow by clicking its overlay, not via Flow's
   hotkey** — they just must not use the Win key, so Flow's hook stops fighting the
   macOS layer.
4. **Keep the Flow overlay visible** — `macos.ahk` clicks Flow's on-screen "Status"
   overlay to start/stop dictation, so it must be shown (the default).
5. **Start at login** — enable it in Flow (the Run-key mechanism isn't scriptable).

## Overlay click coordinates

`macos.ahk` clicks the Flow "Status" overlay at fixed offsets within that window
(anchored via `WinGetPos`, so they survive the widget moving): **start** ≈
`(440, 560)`, **stop** ≈ `(512, 538)`, captured at 200% display scaling. If Flow's
overlay layout changes (or these miss), re-capture with **`flow-coord-capture.ahk`**
(press `F1`/`F2` over the start/stop targets; `F4` verifies a click) and update the
offsets in `macos.ahk`'s `_FlowStartClicks` / `_FlowStopClicks`.

> Diagnostics kept for re-tuning: `flow-coord-capture.ahk` (grab/verify overlay
> coords) and `copilot-key-probe.ahk` (confirm the Copilot key's key events).

## Migrating a machine that had the old macro

Run the cleanup helper once — it finds a deployed `macos.ahk` that still has the old
voice block, backs it up (`~/.config/dotfiles/ahk-voice-backups/`), re-deploys the
cleaned `macos.ahk`, and restarts AutoHotkey:

```bash
opt/scripts/system/retire-ahk-voice-macro.sh            # clean if found
opt/scripts/system/retire-ahk-voice-macro.sh --dry-run  # preview only
```

No-op (with a message) if nothing old is found or you're not on WSL.

## Caveats

- **Cloud-only** — Flow needs an internet connection; no offline transcription.
- **Plan limits** — free tier ~2,000 words/week; Pro is paid. <https://wisprflow.ai/pricing>
- **Account required** — first launch needs interactive sign-in.
- **x64 Windows 10/11 only** — no ARM build.
- **PowerToys required** — the F24 remap depends on Keyboard Manager.
- **Not elevated** — when `macos.ahk` runs non-elevated it can't dictate into
  elevated/admin windows; run it elevated (the logon task) if you need that, but keep
  PowerToys at the same integrity so KBM's F24 reaches it.

## Restore the old AHK macro

See [`archive/README.md`](../../../../archive/README.md): paste the body of
`archive/macos-copilot-claude-voice.ahk` back into `macos.ahk`, remove the KBM F24
remap (`suppress-copilot-key.ps1 -Remove`), re-deploy, and restart AutoHotkey.

## Sources

- Deploy via MDM / MSI: <https://docs.wisprflow.ai/articles/9363440133-deploy-wispr-flow-via-mdm>
- Hotkey rules: <https://docs.wisprflow.ai/articles/2612050838-supported-unsupported-keyboard-hotkey-shortcuts>
- System requirements: <https://docs.wisprflow.ai/articles/1036674442-supported-devices-and-system-requirements>
- Copilot key = LWin+LShift+F23; PowerToys Keyboard Manager: <https://learn.microsoft.com/en-us/windows/powertoys/keyboard-manager>
