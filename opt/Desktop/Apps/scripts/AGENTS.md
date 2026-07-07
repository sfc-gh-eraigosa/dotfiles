# Desktop Apps scripts (Windows-side automation)

Scripts deployed to the Windows Desktop under `Apps\scripts\` by `install.sh`
(via `opt/bin/install_windows.sh`). They configure the Windows host: macOS-style
keyboard shortcuts, Wispr Flow voice dictation, app/font setup, and PowerToys.

## Inventory

- **`macos.ahk`** — AutoHotkey v2: macOS-style Cmd/Opt shortcuts **and** the Wispr
  Flow hold-to-talk dictation driver (Copilot key + user-added trigger keys).
- **`flow-triggers.ahk`** — pure data/policy layer for extra trigger keys (validate,
  normalize, dedupe, persist). Headlessly testable via `flow-triggers-test.ahk`.
- **`flow-calib.ahk` / `flow-calib-test.ahk`** — overlay click-offset calibration.
- **`copilot-key-detect.ahk` / `copilot-key-probe.ahk`** — diagnostics that log the
  raw vk/sc a key emits / whether the Copilot key auto-repeats.
- **`suppress-copilot-key.ps1`** — PowerToys KBM remap `Win+Shift+F23 → F24` (so
  Windows doesn't open the Copilot Settings page) + module setup. `-Status` reports state.
- **`install-wisprflow.ps1`** — installs the Wispr Flow app (self-elevates).
- **`setup-autostart.ps1`** — registers the elevated logon task for `macos.ahk` and,
  re-run standalone, **reloads** AutoHotkey after a re-deploy (self-elevates → UAC).
- **`setup-apps.ps1`** — winget app install + Windows Terminal profiles/themes.

User-facing runbook: **`WISPR-FLOW.md`**. Overview: **`README.md`**.

## Developing macos.ahk from WSL (the cross-boundary loop)

AutoHotkey runs on **Windows**; the repo is edited in **WSL**. There is no
hot-reload — every change is a deploy → restart-AHK → observe loop. The
**`wispr-flow-debug` skill** (`src/wispr-flow-debug/SKILL.md`) automates this; the
essentials:

1. **Deploy:** copy `macos.ahk` to the (often OneDrive-redirected) Desktop —
   resolve it with `powershell.exe [Environment]::GetFolderPath('Desktop')` piped
   through `wslpath`, never a hardcoded path.
2. **Reload:** `Stop-Process AutoHotkey*` then `Start-Process AutoHotkey64.exe`.
   **Non-elevated is enough to test trigger keys**; elevation only matters for
   dictating into admin windows and for the logon-task that survives reboot. **KBM
   and AHK must run at the same integrity** or PowerToys' injected F24 never reaches
   AHK's keyboard hook.
3. **Observe:** add temporary `FileAppend(..., A_Temp "\flow-dbg.txt")` lines tagged
   `; DEBUG (temporary)`, then read the log from WSL at
   `$(wslpath "$(powershell.exe '$env:TEMP')")/flow-dbg.txt`. **Strip every DEBUG
   line before committing** (`grep -n "flow-dbg.txt\|; DEBUG" macos.ahk` must be empty).

> The Copilot key and every extra trigger key route through the **same**
> `_FlowTriggerDown`/`_FlowTriggerUp` (`*F24::` plus the startup bind loop), so they
> cannot behave differently by code. A reported divergence is upstream: the
> PowerToys remap, an integrity mismatch, a stale deployed file, or `flow-triggers.ini`.

## AutoHotkey v2 gotchas (bugs this layer has actually hit)

- **Hotkey callbacks must take `(*)`** — AHK v2 passes the hotkey name to the
  callback; a zero-parameter function throws on every press (binds fine, fires never).
- **`On`/`Off`/`Toggle` is the Action (2nd) arg of `Hotkey()`, not an Options
  string** — `Hotkey key, "Off"` disables; `Hotkey key, callback` binds (enabled by
  default, no `"On"`). In Options they error and a surrounding `try` swallows it.
- **`GetKeyName` returns long modifier spellings** (`LControl`/`RControl`/`LMenu`/
  `RMenu`) — match both forms or a Ctrl/Alt key is misclassified into a junk chord.
- **Timer threads inherit auto-exec defaults, not the caller's** — re-assert
  `CoordMode` inside a `SetTimer fn, -1` handler; it does not see the hotkey thread's.

## flow-triggers.ini

Per-machine, **not tracked in git** (`%LOCALAPPDATA%\dotfiles\flow-triggers.ini`).
The loader **gates on `count=`** and leaves higher-index `kN` lines as inert
orphans — so `count=1` with `k1/k2/k3` present binds only `k1` (this is by design:
removing a trigger decrements `count` without deleting the orphaned line).
