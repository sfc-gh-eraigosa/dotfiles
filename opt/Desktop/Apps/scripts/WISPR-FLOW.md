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
| **extra trigger keys** (hold) | Same hold-to-talk as the Copilot key — add your own via F9 (see below) |
| **Esc** *(only while dictating)* | Cancel — reset, no paste; normal Esc otherwise |
| **F1** (hold, dictation ON) | Help overlay listing every live binding; release to dismiss |
| **F9** | Toggle manage-triggers mode — add/remove your own trigger keys (centered rainbow HUD) |
| **F10** | Toggle the whole dictation flow on/off (centered popup shows ON/OFF) |
| **F11** | Toggle calibration mode — re-capture the overlay click offsets (centered rainbow HUD) |

> While dictating, the "🎤 Listening…" / "⏳ Transcribing…" tips carry a trailing
> ` F1 help` hint (the "✗ cancelled" tip does not). Hold **F1** any time dictation is
> ON to pop the FLOW HELP overlay; it's suppressed during calibration (F11) and
> manage (F9) modes and returns once you exit them.

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

## Overlay click coordinates & calibration

`macos.ahk` clicks the Flow "Status" overlay at offsets within that window
(anchored via `WinGetPos`, so they survive the widget moving). The baked-in
defaults are **start** `(440, 560)`, **stop** `(512, 538)` (200% display scaling);
your tuned values persist to `%LOCALAPPDATA%\dotfiles\flow-calib.ini` and override
the defaults at startup (so re-deploying `macos.ahk` won't lose them).

**To re-calibrate, press `F11`** (calibration mode). A centered rainbow HUD shows
the live offsets + keymap:

| Key | Action |
|-----|--------|
| `F1` / `F2` | capture the mouse position as the **start** / **stop** offset |
| `F3` | revert to the last **saved** values |
| `F5` | restore the baked-in **defaults** |
| `F4` | **save** to the ini |
| `F10` | **dry-run**: click start, wait ~2 s, click stop (watch both land) |
| `F11` / `Esc` | end calibration |

Hover the exact spot, `F1`/`F2` to capture, `F10` to test, `F4` to save.

> Diagnostic kept for re-tuning: `copilot-key-probe.ahk` (confirm the Copilot key's
> key events).

## Adding extra trigger keys (F9 manage mode)

The Copilot key isn't the only way to start dictation — you can bind **extra
trigger keys** that route through the exact same hold-to-talk handlers. Press and
hold any one of them to dictate, just like the Copilot key.

**Press `F9`** to enter manage-triggers mode. A centered rainbow MANAGE TRIGGERS HUD
lists the locked Copilot default plus every key you've added, with the rule line
*"Press a key to add · press again to remove · Esc exits"*. While the HUD is up:

- **Press any key (or modifier+key combo) to add it** as a trigger.
- **Press an already-added key again to remove it** — add/remove is a single toggle.
- **`Esc`** (or `F9` again) **exits** manage mode.

The capture is consuming: the key you press to add/remove is swallowed, not typed
into whatever's behind the HUD. The Copilot key (F24) is the **locked default** — it's
always listed with a 🔒 and can't be added or removed.

**Allowed trigger shapes** (anything else is refused with an on-screen reason):

- a function key **F6–F8** or **F12–F22**, bare or with modifiers;
- a **media / browser / launch** key (Volume, Media, Browser, Launch), bare or with modifiers;
- **any key with at least one modifier** (Ctrl / Alt / Shift / Cmd), e.g. `Ctrl+Alt+D`;
- **Right Cmd (RWin)** — the one bare modifier you may use on its own.

**Refused with a reason** (the HUD shows a brief `✗ … — <why>` tip and adds nothing):

- **reserved driver keys** — Esc, F1–F5, F9, F10, F11, F23, F24, and Left Cmd (LWin)
  are owned by the driver / the Cmd layer;
- **lone modifiers other than Right Cmd** — *"add a key, not a lone modifier"*;
- **bare nav / edit / printable keys** (arrows, Tab, Space, letters, F1–F5, …) —
  *"add an F6–F22 / media key, a modifier+key combo, or Right Cmd"*;
- **common OS / editor shortcuts** — `Ctrl+C/V/X/Z/S/F/…`, `Alt+Tab`, `Alt+F4`,
  `Cmd+Tab/D/L/E/R`, `Ctrl+Shift+Esc` — *"that's a common OS/editor shortcut"*;
- anything **already bound** by `macos.ahk`'s Cmd / Opt layers — *"already bound by macos.ahk"*.

> **First-release-wins:** if you hold two triggers at once, releasing *either* one
> stops dictation (there's no per-key ownership); a second trigger pressed mid-session
> is swallowed as an auto-repeat no-op.

### Where the triggers live

Added triggers persist per-machine to **`%LOCALAPPDATA%\dotfiles\flow-triggers.ini`**
— same class as `flow-calib.ini`: **not tracked in git**, and it **survives
re-deploying `macos.ahk`**. A missing, blank, or hand-corrupted file degrades to
*"no extra triggers"* and never crashes startup. The Copilot key is never written to
the file (it's the built-in default).

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
- **Right Cmd, while added, is captured — even when dictation is OFF.** If you've
  added **Right Cmd (RWin)** as a trigger (see [Adding extra trigger keys](#adding-extra-trigger-keys-f9-manage-mode)),
  a lone RWin tap no longer opens the Start menu: AHK swallows the key for as long as
  `macos.ahk` runs. **Toggling dictation off with F10 does NOT restore it** — F10 gates
  whether dictation *fires*, not whether trigger keys are *captured*, so a captured
  RWin stays "dead" (no Start menu, no Win-modifier) regardless of the F10 state. This
  is deliberate. To return Right Cmd (or any added key) to its OS role, **remove it via
  F9** (or quit `macos.ahk`). Non-Win triggers like `F13` / `Ctrl+Alt+D` are swallowed
  the same way, but harmlessly.

## Restore the old AHK macro

See [`archive/README.md`](../../../../archive/README.md): paste the body of
`archive/macos-copilot-claude-voice.ahk` back into `macos.ahk`, remove the KBM F24
remap (`suppress-copilot-key.ps1 -Remove`), re-deploy, and restart AutoHotkey.

## Sources

- Deploy via MDM / MSI: <https://docs.wisprflow.ai/articles/9363440133-deploy-wispr-flow-via-mdm>
- Hotkey rules: <https://docs.wisprflow.ai/articles/2612050838-supported-unsupported-keyboard-hotkey-shortcuts>
- System requirements: <https://docs.wisprflow.ai/articles/1036674442-supported-devices-and-system-requirements>
- Copilot key = LWin+LShift+F23; PowerToys Keyboard Manager: <https://learn.microsoft.com/en-us/windows/powertoys/keyboard-manager>
