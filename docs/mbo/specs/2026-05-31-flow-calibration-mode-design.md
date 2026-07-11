# Flow Calibration Mode — Design

**Date:** 2026-05-31
**Status:** Approved (ready for implementation plan)
**Scope:** `opt/Desktop/Apps/scripts/macos.ahk` (+ docs)

## Problem

`macos.ahk` drives Wispr Flow by clicking its "Status" overlay at fixed offsets —
**start** `(440, 560)`, **stop** `(512, 538)` — hardcoded in `_FlowStartClicks` /
`_FlowStopClicks`. When the overlay layout or display scaling changes, those offsets
miss and dictation silently fails to start/stop. Today re-calibrating means running a
separate `flow-coord-capture.ahk`, reading a temp log, hand-editing the literals in
`macos.ahk`, and re-deploying. That loop is slow and easy to get wrong.

## Goal

An in-script **calibration mode**, toggled with **F11** (while the F10 dictation
toggle is on), that lets the user capture, test, save, and revert the overlay
offsets live — with a colorized on-screen HUD — and persist them so they survive
both an AutoHotkey reload and a repo re-deploy of `macos.ahk`.

## Architecture

Three layers of offset values:

| Layer | Where it lives | Role |
|-------|----------------|------|
| **DEFAULT** | baked-in constants in `macos.ahk` (`440,560` / `512,538`) | factory baseline; F5 restores |
| **SAVED** | `%LOCALAPPDATA%\dotfiles\flow-calib.ini` | last persisted values; F3 reverts to |
| **WORKING** | in-memory globals (`FlowStartX/Y`, `FlowStopX/Y`) | what the click functions actually use; F1/F2 edit |

**Startup:** `macos.ahk` reads the ini into the WORKING globals, falling back to
DEFAULT for any missing/invalid key. The existing `_FlowStartClicks` /
`_FlowStopClicks` stop hardcoding the numbers and read the WORKING globals instead —
so a saved calibration applies to real dictation automatically.

This means calibration is decoupled from the source: re-deploying `macos.ahk` from
the repo no longer wipes a user's tuned offsets (they live in the ini, outside the
repo), and the repo's baked-in defaults remain the documented baseline.

## Config file

Auto-created on the first F4 save (its parent dir is created if missing):

```ini
; %LOCALAPPDATA%\dotfiles\flow-calib.ini
[overlay]
startX=440
startY=560
stopX=512
stopY=538
```

- **Path:** `EnvGet("LOCALAPPDATA") . "\dotfiles\flow-calib.ini"`.
- **Read:** `IniRead` each key with the DEFAULT as the fallback value, so a missing
  file or a partially-corrupt file degrades gracefully to defaults (never errors).
- **Write:** `DirCreate` the parent, then `IniWrite` each key.
- **Not tracked in git** — it is per-machine runtime state, like the existing
  `flow-coords.txt` temp log.

## Controls

`F1–F5`, `F10`, and `Esc` are re-scoped **only while calibration mode is active**
(via `#HotIf CalibActive`); outside calibration they keep their normal meaning
(notably F10 stays the dictation on/off toggle). `F11` is a global toggle.

| Key | Action |
|-----|--------|
| **F11** | Toggle calibration mode on/off. Enters **only when `FlowState = IDLE`**. Fires a rainbow `CALIBRATION ON` / grey `CALIBRATION OFF` toast (`_FlowToast`). |
| **F1** | Capture the current mouse position as the WORKING **start** offset (mouse-screen minus the overlay window's top-left, via `WinGetPos`). |
| **F2** | Capture the current mouse position as the WORKING **stop** offset. |
| **F3** | Revert WORKING ← last **SAVED** (the ini; or DEFAULT if no file). Discards unsaved edits. |
| **F5** | Restore WORKING ← baked-in **DEFAULT**. |
| **F4** | **Save** WORKING → ini (`SAVED ← WORKING`; clears the unsaved marker). |
| **F10** | **Dry-run**: click START at the WORKING start offset, wait ~2 s, click STOP at the WORKING stop offset — so both clicks are observed without real dictation. |
| **Esc** | Exit calibration mode (identical to pressing F11 again). |

While calibration mode is active, the Copilot key (`*F24`) is **ignored**, so a stray
press can't kick off real dictation mid-calibration.

## HUD

A persistent GUI panel:

- `+AlwaysOnTop -Caption +ToolWindow`, shown with `NoActivate` so it never steals
  focus from the field/overlay being calibrated.
- **Position:** top-center of the screen, so it never overlaps the bottom-anchored
  Flow overlay (whose click targets are ~`y560`).
- **Title:** "FLOW CALIBRATION" rendered per-character in the same pastel-rainbow
  palette as `_FlowToast` / the install banner.
- **Body:** the live WORKING start & stop offsets, each tagged `● unsaved` or
  `✓ saved` driven by a dirty flag (`dirty = WORKING ≠ SAVED`), followed by the
  keymap (F1/F2/F3/F4/F5/F10/F11).
- **Lifecycle:** built on enter, **rebuilt/refreshed** after every F1/F2/F3/F4/F5
  (so values + markers update live), destroyed on exit.

Enter/exit also fire the brief ~1 s `_FlowToast` (`CALIBRATION ON/OFF`), reusing the
existing toast so the feel matches the F10 dictation toggle.

## Data flow

```
F11 (idle) ─► CalibActive = true ─► build HUD ─► toast "CALIBRATION ON"
   F1 ─► mouse-offset ─► WORKING.start ─► dirty=true ─► refresh HUD
   F2 ─► mouse-offset ─► WORKING.stop  ─► dirty=true ─► refresh HUD
   F3 ─► WORKING ← SAVED ─► dirty=false ─► refresh HUD
   F5 ─► WORKING ← DEFAULT ─► dirty=(DEFAULT≠SAVED) ─► refresh HUD
   F10 ─► click START(WORKING) ; sleep 2s ; click STOP(WORKING)
   F4 ─► ini ← WORKING ; SAVED ← WORKING ; dirty=false ─► refresh HUD
F11 / Esc ─► CalibActive = false ─► destroy HUD ─► toast "CALIBRATION OFF"
            (WORKING stays live for the session; only F4 persisted it)
```

## Edge cases

- **F1/F2 with no overlay present** → the "Status ahk_exe Wispr Flow.exe" window
  isn't found, so `WinGetPos` can't anchor the offset. Show a warning toast/tip and
  capture nothing (leave WORKING unchanged).
- **Exit with unsaved edits** → WORKING stays live for the rest of the session (you
  can immediately test with the real Copilot key), but is **not** persisted; the next
  AHK reload reverts to SAVED. The HUD's `● unsaved` marker makes this explicit
  before you exit.
- **F11 while dictating** (`FlowState != IDLE`) → ignored, so calibration can't be
  entered mid-dictation.
- **Corrupt/partial ini** → per-key `IniRead` fallback to DEFAULT; never throws.

## Files

- **`opt/Desktop/Apps/scripts/macos.ahk`** — DEFAULT constants + WORKING globals;
  `_FlowCalibPath()`, `_FlowCalibLoad()`, `_FlowCalibSave()` (pure, testable);
  refactor `_FlowStartClicks` / `_FlowStopClicks` to read WORKING globals; calibration
  state, HUD build/refresh/destroy, and the `F11` + `#HotIf CalibActive` hotkeys.
- **`opt/Desktop/Apps/scripts/WISPR-FLOW.md`** — new "Calibration mode" section, a
  controls-table row for F11, and note the ini path; update the "Overlay click
  coordinates" section to point at the in-script mode.
- **`opt/Desktop/Apps/scripts/README.md`** — update the `macos.ahk` row to mention
  the F11 calibration mode.
- **`flow-coord-capture.ahk`** — superseded by the in-script mode; retire it
  (remove the script + its README/WISPR-FLOW references) once calibration is verified.

## Testing

AutoHotkey has no GUI unit-test framework, so testing splits in two:

1. **Automated (config layer):** `_FlowCalibLoad` / `_FlowCalibSave` are pure
   functions over a file path (no GUI, no Flow dependency). A headless test script
   run via `AutoHotkey64.exe`: write known values, read them back, assert equality,
   assert the missing-file path returns DEFAULT, then `ExitApp` with `0` on success /
   non-zero on failure. This is the real regression test and can run unattended.
2. **Manual (UI/clicks):** `/validate` syntax check on `macos.ahk`, then a manual
   checklist — F11 shows/hides HUD + toast; F1/F2 update the live offsets; F3/F5
   revert/restore; F4 writes the ini (verify the file); F10 dry-run clicks both
   targets; Esc exits; a saved calibration persists across an AHK reload.

## Out of scope (YAGNI)

- Calibrating anything other than the two overlay offsets (e.g. dwell time).
- A GUI for typing coordinates by hand — capture-by-hover + the three reset layers
  cover the need.
- Multi-monitor profiles — one offset pair per machine.
