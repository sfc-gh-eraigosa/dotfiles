---
name: wispr-flow-debug
description: Debug and test the Wispr Flow AutoHotkey dictation triggers (Copilot key + extra keys) from WSL by deploying macos.ahk to the Windows Desktop, reloading AutoHotkey, and reading the debug log. Use when an activation/trigger key isn't starting/stopping dictation, when the overlay clicks land wrong, or when verifying a macos.ahk change on Windows.
---

# Debug Wispr Flow triggers (WSL → Windows dev loop)

The Wispr Flow dictation layer lives in `opt/Desktop/Apps/scripts/macos.ahk` (see
`WISPR-FLOW.md` for the user runbook). AutoHotkey runs on **Windows**, but the repo
is edited from **WSL**, so testing is a deploy → reload → read-log loop across the
boundary. This skill encodes that loop and the gotchas that bite every time.

## Mental model (read first)

- **Copilot key path:** physical Copilot key emits `Win+Shift+F23` → PowerToys
  Keyboard Manager remaps it to **F24** → `macos.ahk`'s `*F24::_FlowTriggerDown`.
- **Extra keys path:** keys saved in `%LOCALAPPDATA%\dotfiles\flow-triggers.ini`
  are bound at startup to the **same** `_FlowTriggerDown`/`_FlowTriggerUp`. The
  Copilot key and extra keys share one code path — they cannot diverge by design,
  so "extra key behaves differently" almost always means an upstream/config issue
  (PowerToys remap, integrity mismatch, or stale deployed script), not the handler.

## Resolve paths (OneDrive-redirected Desktop is common)

```bash
PS=powershell.exe
WIN_DESKTOP=$("$PS" -NoProfile -Command "[Environment]::GetFolderPath('Desktop')" | tr -d '\r')
DESKTOP=$(wslpath "$WIN_DESKTOP")                       # e.g. /mnt/c/Users/<you>/OneDrive/Desktop
DEPLOYED="$DESKTOP/Apps/scripts/macos.ahk"
WIN_TEMP=$("$PS" -NoProfile -Command '$env:TEMP' | tr -d '\r')
DBG="$(wslpath "$WIN_TEMP")/flow-dbg.txt"              # where FileAppend DEBUG lines land
INI="$(wslpath "$("$PS" -NoProfile -Command '$env:LOCALAPPDATA' | tr -d '\r')")/dotfiles/flow-triggers.ini"
```

## The loop

### 1. Deploy the edited script
```bash
cp -f opt/Desktop/Apps/scripts/macos.ahk "$DEPLOYED"
diff -q <(tr -d '\r' < "$DEPLOYED") opt/Desktop/Apps/scripts/macos.ahk && echo "deployed == repo"
```
AutoHotkey does **not** hot-reload a changed `.ahk` — you must restart it.

### 2. Reload AutoHotkey (non-elevated is enough for trigger testing)
```bash
"$PS" -NoProfile -Command '
$exe = "$env:LOCALAPPDATA\Programs\AutoHotkey\v2\AutoHotkey64.exe"
$script = Join-Path ([Environment]::GetFolderPath("Desktop")) "Apps\scripts\macos.ahk"
Get-Process AutoHotkey* -EA SilentlyContinue | Stop-Process -Force -EA SilentlyContinue
Start-Sleep -Milliseconds 400
Start-Process $exe -ArgumentList ("`"{0}`"" -f $script)
Start-Sleep -Milliseconds 900
if (Get-Process AutoHotkey* -EA SilentlyContinue) { "OK: loaded (no fatal parse error)" } else { "WARN: not running -> load error" }'
```
> Elevation only matters for (a) dictating into **admin** windows and (b) surviving
> reboot via the logon task. For verifying that a key fires, non-elevated is fine —
> but PowerToys KBM and AutoHotkey **must be at the same integrity** or KBM's
> injected F24 won't reach AHK's hook.

### 3. Instrument, then read the log from WSL
Add temporary `FileAppend` lines (tag them `; DEBUG (temporary)`) to the relevant
handlers, writing to `A_Temp "\flow-dbg.txt"`. After a reproduce:
```bash
cat "$DBG"
```
Useful probes: `STARTUP` + `BOUND <key>` (did it bind?), `DOWN <key> en= st=` (did
the press fire the handler?), and a `CLICK key= … -> screen=X,Y` line inside
`_FlowClickOverlay` (where the overlay click actually lands).

### 4. Strip ALL debug before shipping
```bash
grep -n "flow-dbg.txt\|; DEBUG" opt/Desktop/Apps/scripts/macos.ahk   # must be empty before commit
```

## Triage checklist when a trigger "doesn't work"

1. **Did it bind?** Look for `BOUND <key>` at startup. If missing, check
   `flow-triggers.ini`: the loader **gates on `count=`** and treats higher-index
   `kN` lines as inert orphans — `count=1` with `k1/k2/k3` only binds `k1`.
2. **Did the press reach AHK?** No `DOWN` line → the key never arrived. For the
   Copilot key, verify the remap: `suppress-copilot-key.ps1 -Status` (PowerToys
   running + KBM enabled + `Win+Shift+F23 → F24` present). Capture the raw key with
   `copilot-key-detect.ahk` (logs vk/sc next to the script).
3. **Integrity mismatch?** Compare elevation of `PowerToys.KeyboardManagerEngine`
   and `AutoHotkey64` — KBM's injected F24 won't cross from elevated → non-elevated.
4. **Stale deploy?** `diff` the deployed file against the repo before blaming code.

## AutoHotkey v2 gotchas (the bugs this layer has actually hit)

- **Hotkey callbacks must accept a parameter.** AHK v2 invokes a hotkey callback
  with the hotkey name as an argument. A zero-parameter `Fn()` throws on every
  press (binds fine, fires never). Declare `Fn(*)`.
- **`On`/`Off`/`Toggle` is the Action (2nd) arg, not an Options string.** Use
  `Hotkey key, "Off"` to disable; `Hotkey key, callback` to (re)bind (hotkeys are
  enabled by default — no `"On"` needed). Passing them in Options raises an
  "invalid option" error that a surrounding `try` will silently swallow.
- **`GetKeyName` returns long modifier spellings** (`LControl`/`RControl`/`LMenu`/
  `RMenu`), not `LCtrl`/`LAlt` — match both when classifying a captured key, or a
  Ctrl/Alt press is misread as a base key and builds a junk chord.
- **Timer threads inherit auto-exec defaults, not the caller's settings.** A
  `SetTimer fn, -1` thread does not see a `CoordMode` set in the hotkey thread; set
  it again inside the timer/click helper.

## See also
- `opt/Desktop/Apps/scripts/WISPR-FLOW.md` — user runbook (install, calibration, F9 triggers).
- `opt/Desktop/Apps/scripts/setup-autostart.ps1` — elevated logon-task reload (self-elevates; UAC).
