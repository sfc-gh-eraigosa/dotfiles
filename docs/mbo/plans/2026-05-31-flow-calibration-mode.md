# Flow Calibration Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an in-script, F11-toggled calibration mode to `macos.ahk` that captures, tests, saves, and reverts the Wispr Flow overlay click offsets live (with a colorized HUD), persisting them to an ini file so they survive AHK reloads and repo re-deploys.

**Architecture:** A small **config data layer** (`flow-calib.ahk`, pure functions over a file path — independently testable) holds the DEFAULT baseline and reads/writes the SAVED ini. `macos.ahk` `#Include`s it, loads the ini into **WORKING** globals at startup, and its overlay-click functions read those globals. A **calibration mode** (state flag + persistent rainbow HUD + `#HotIf`-scoped hotkeys) edits the WORKING globals and persists them with F4.

**Tech Stack:** AutoHotkey v2 (`#Include`, `Gui`, `IniRead`/`IniWrite`, `#HotIf`), validated via `AutoHotkey64.exe /validate` and a headless assertion script run from WSL through `powershell.exe`.

---

## Conventions for this plan

**This machine's paths** (used in every deploy/test command):

```
AHK   = C:\Users\<user>\AppData\Local\Programs\AutoHotkey\v2\AutoHotkey64.exe
DEST  = C:\Users\<user>\OneDrive\Desktop\Apps\scripts        (WSL: /mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts)
TEMP  = /mnt/c/Users/<user>/AppData/Local/Temp               (Windows %TEMP%)
REPO  = $HOME/git/dotfiles
SRC   = $REPO/opt/Desktop/Apps/scripts
```

**Commits:** This repo commits through the **git-safe-sync (gss)** flow with mandatory `AskUserQuestion` confirmation, and pushes with the two-call token recipe. Each task below ends with a logical commit *checkpoint* (staged files + message); batch them into a gss sync at the points the executor/user chooses rather than pushing per-task.

**Deploy helper** (used repeatedly) — copy a script to DEST, validate it, reload AHK:

```bash
# usage: deploy_validate <file.ahk>
deploy_validate() {
  cp "$SRC/$1" "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/$1"
  powershell.exe -NoProfile -Command "\$p=Start-Process '$AHK' -ArgumentList '/validate','\"C:\\Users\\<user>\\OneDrive\\Desktop\\Apps\\scripts\\$1\"' -Wait -PassThru -WindowStyle Hidden; 'validate '+\$p.ExitCode" 2>&1 | tr -d '\r'
}
```

---

## File Structure

| File | Responsibility |
|------|----------------|
| `opt/Desktop/Apps/scripts/flow-calib.ahk` | **New.** Config data layer: `_FlowCalibDefaults()`, `_FlowCalibPath()`, `_FlowCalibLoad(path)`, `_FlowCalibSave(path, map)`. Pure — no GUI/hotkeys. `#Include`d by `macos.ahk` and by the test. |
| `opt/Desktop/Apps/scripts/flow-calib-test.ahk` | **New.** Headless assertion harness for the config layer. Writes results to `%TEMP%\flow-calib-test.out`; `ExitApp 0`=pass, `1`=fail. |
| `opt/Desktop/Apps/scripts/macos.ahk` | **Modify.** `#Include flow-calib.ahk`; WORKING offset globals + startup load; refactor `_FlowStartClicks`/`_FlowStopClicks` to read them; calibration state, HUD, and hotkeys. |
| `opt/Desktop/Apps/scripts/WISPR-FLOW.md` | **Modify.** Document calibration mode + ini path; update "Overlay click coordinates". |
| `opt/Desktop/Apps/scripts/README.md` | **Modify.** Mention F11 calibration in the `macos.ahk` row; drop the `flow-coord-capture.ahk` row. |
| `opt/Desktop/Apps/scripts/flow-coord-capture.ahk` | **Delete.** Superseded by the in-script mode. |

---

## Task 1: Config data layer (TDD)

**Files:**
- Create: `opt/Desktop/Apps/scripts/flow-calib.ahk`
- Test: `opt/Desktop/Apps/scripts/flow-calib-test.ahk`

- [ ] **Step 1: Write the failing test**

Create `opt/Desktop/Apps/scripts/flow-calib-test.ahk`:

```ahk
#Requires AutoHotkey v2.0
#Include flow-calib.ahk
; Headless test for the calibration config layer. Results -> %TEMP%\flow-calib-test.out
; Exit code 0 = all pass, 1 = a failure (message in the .out file).

RESULT := A_Temp "\flow-calib-test.out"
try FileDelete RESULT

_assert(cond, msg) {
    global RESULT
    if (!cond) {
        FileAppend "FAIL: " msg "`n", RESULT
        ExitApp 1
    }
}

tmp := A_Temp "\flow-calib-test-" A_TickCount ".ini"
try FileDelete tmp

; 1. Missing file -> defaults
d := _FlowCalibDefaults()
m := _FlowCalibLoad(tmp)
for k, v in d
    _assert(m[k] = v, "missing-file should return default for " k)

; 2. Save then load round-trips exactly
saved := Map("startX", 111, "startY", 222, "stopX", 333, "stopY", 444)
_FlowCalibSave(tmp, saved)
m2 := _FlowCalibLoad(tmp)
for k, v in saved
    _assert(m2[k] = v, "roundtrip " k " expected " v " got " m2[k])

; 3. Partial file falls back per-key to default
IniDelete tmp, "overlay", "stopY"
m3 := _FlowCalibLoad(tmp)
_assert(m3["startX"] = 111, "partial file keeps startX=111")
_assert(m3["stopY"] = d["stopY"], "partial file falls back stopY to default")

try FileDelete tmp
FileAppend "OK: all calibration config tests passed`n", RESULT
ExitApp 0
```

- [ ] **Step 2: Run the test, verify it FAILS (no implementation yet)**

```bash
cd $REPO
AHK="/mnt/c/Users/<user>/AppData/Local/Programs/AutoHotkey/v2/AutoHotkey64.exe"
cp opt/Desktop/Apps/scripts/flow-calib-test.ahk "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/flow-calib-test.ahk"
powershell.exe -NoProfile -Command "\$p=Start-Process '$AHK' -ArgumentList '\"C:\Users\<user>\OneDrive\Desktop\Apps\scripts\flow-calib-test.ahk\"' -Wait -PassThru -WindowStyle Hidden; 'exit '+\$p.ExitCode" 2>&1 | tr -d '\r'
```

Expected: a non-zero exit (the `#Include flow-calib.ahk` fails to load because the file does not exist yet → AHK error dialog / non-zero). This confirms the harness runs and the implementation is missing.

- [ ] **Step 3: Implement the config layer**

Create `opt/Desktop/Apps/scripts/flow-calib.ahk`:

```ahk
#Requires AutoHotkey v2.0
; flow-calib.ahk — Wispr Flow overlay-offset calibration DATA layer.
; Pure functions (path in, data in/out): no GUI, no hotkeys, no side effects,
; so they can be exercised headlessly by flow-calib-test.ahk. #Include'd by macos.ahk.

; The baked-in factory baseline (documented default; F5 restores these).
; Single source of truth for the default offsets.
_FlowCalibDefaults() {
    return Map("startX", 440, "startY", 560, "stopX", 512, "stopY", 538)
}

; Per-machine runtime store (NOT tracked in git).
_FlowCalibPath() {
    return EnvGet("LOCALAPPDATA") . "\dotfiles\flow-calib.ini"
}

; Read each key with its default as the fallback, coerced to Integer, so a missing
; or partially-corrupt file degrades gracefully to defaults and never throws.
_FlowCalibLoad(path) {
    d := _FlowCalibDefaults()
    m := Map()
    for key, dflt in d
        m[key] := Integer(IniRead(path, "overlay", key, dflt))
    return m
}

; Persist the working values; create the parent directory if absent.
_FlowCalibSave(path, m) {
    SplitPath path, , &dir
    if (dir && !DirExist(dir))
        DirCreate dir
    for key, val in m
        IniWrite val, path, "overlay", key
}
```

- [ ] **Step 4: Run the test, verify it PASSES**

```bash
cd $REPO
AHK="/mnt/c/Users/<user>/AppData/Local/Programs/AutoHotkey/v2/AutoHotkey64.exe"
cp opt/Desktop/Apps/scripts/flow-calib.ahk "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/flow-calib.ahk"
cp opt/Desktop/Apps/scripts/flow-calib-test.ahk "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/flow-calib-test.ahk"
powershell.exe -NoProfile -Command "\$p=Start-Process '$AHK' -ArgumentList '\"C:\Users\<user>\OneDrive\Desktop\Apps\scripts\flow-calib-test.ahk\"' -Wait -PassThru -WindowStyle Hidden; 'exit '+\$p.ExitCode" 2>&1 | tr -d '\r'
cat "/mnt/c/Users/<user>/AppData/Local/Temp/flow-calib-test.out" | tr -d '\r'
```

Expected: `exit 0` and `OK: all calibration config tests passed`.

- [ ] **Step 5: Commit checkpoint**

```bash
git add opt/Desktop/Apps/scripts/flow-calib.ahk opt/Desktop/Apps/scripts/flow-calib-test.ahk
# commit subject:
# feat(wispr): calibration config data layer (ini load/save) + headless test
```

---

## Task 2: Wire config into macos.ahk + refactor click offsets

**Files:**
- Modify: `opt/Desktop/Apps/scripts/macos.ahk` (top include; Flow globals ~line 154; `_FlowStartClicks` ~line 262; `_FlowStopClicks` ~line 273; `*F24` handler ~line 279)

- [ ] **Step 1: Add the include**

Near the top of `macos.ahk` (after the `#SingleInstance`/`SetTitleMatchMode` preamble, before the Flow code), add:

```ahk
#Include flow-calib.ahk
```

- [ ] **Step 2: Add WORKING globals + startup load**

In the Flow globals block (after `FlowState := "IDLE"` etc., around line 154-161), add:

```ahk
; Overlay click offsets — WORKING set, used by the click functions below.
; Loaded from the calibration ini at startup (falls back to _FlowCalibDefaults()).
; _flowCalib holds the SAVED snapshot for the dirty check + F3 revert.
_flowCalib := _FlowCalibLoad(_FlowCalibPath())
FlowStartX := _flowCalib["startX"]
FlowStartY := _flowCalib["startY"]
FlowStopX  := _flowCalib["stopX"]
FlowStopY  := _flowCalib["stopY"]
CalibActive   := false        ; F11 calibration mode flag
_flowCalibGui := ""           ; persistent HUD handle
```

- [ ] **Step 3: Refactor `_FlowStartClicks` to read the WORKING globals**

Replace the hardcoded start offset. Change:

```ahk
_FlowStartClicks() {
    global FlowState, FlowX, FlowY
    if !_FlowClickOverlay(440, 560) {                    ; START: hover-dwell then click (calibrated)
```

to:

```ahk
_FlowStartClicks() {
    global FlowState, FlowX, FlowY, FlowStartX, FlowStartY
    if !_FlowClickOverlay(FlowStartX, FlowStartY) {       ; START: WORKING offset (calibratable via F11)
```

- [ ] **Step 4: Refactor `_FlowStopClicks` to read the WORKING globals**

Change:

```ahk
_FlowStopClicks() {
    global FlowX, FlowY
    _FlowClickOverlay(512, 538)                          ; STOP: hover-dwell then click
```

to:

```ahk
_FlowStopClicks() {
    global FlowX, FlowY, FlowStopX, FlowStopY
    _FlowClickOverlay(FlowStopX, FlowStopY)               ; STOP: WORKING offset (calibratable via F11)
```

- [ ] **Step 5: Guard the Copilot key during calibration**

In the `*F24::` handler, add `CalibActive` to the globals line and an early return. Change the opening:

```ahk
*F24::{                        ; Copilot key (remapped to F24 by PowerToys KBM) -> start
    global FlowState, FlowWin, FlowX, FlowY, FlowEnabled
    if (!FlowEnabled)
        return
```

to:

```ahk
*F24::{                        ; Copilot key (remapped to F24 by PowerToys KBM) -> start
    global FlowState, FlowWin, FlowX, FlowY, FlowEnabled, CalibActive
    if (!FlowEnabled || CalibActive)   ; ignore the Copilot key while calibrating
        return
```

- [ ] **Step 6: Validate + reload, confirm normal dictation still works**

```bash
cd $REPO
AHK="/mnt/c/Users/<user>/AppData/Local/Programs/AutoHotkey/v2/AutoHotkey64.exe"
cp opt/Desktop/Apps/scripts/macos.ahk "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/macos.ahk"
powershell.exe -NoProfile -Command "\$p=Start-Process '$AHK' -ArgumentList '/validate','\"C:\Users\<user>\OneDrive\Desktop\Apps\scripts\macos.ahk\"' -Wait -PassThru -WindowStyle Hidden; 'validate '+\$p.ExitCode" 2>&1 | tr -d '\r'
powershell.exe -NoProfile -Command "taskkill /F /IM AutoHotkey64.exe 2>\$null; Start-Sleep 1; Start-Process '$AHK' -ArgumentList '\"C:\Users\<user>\OneDrive\Desktop\Apps\scripts\macos.ahk\"'" 2>&1 | tr -d '\r'
```

Expected: `validate 0`. Manual: hold the Copilot key — dictation still starts/stops exactly as before (now reading the ini-loaded offsets, which equal the defaults until calibrated).

- [ ] **Step 7: Commit checkpoint**

```bash
git add opt/Desktop/Apps/scripts/macos.ahk
# feat(wispr): load overlay offsets from calibration ini; click fns read WORKING globals
```

---

## Task 3: Calibration HUD + capture/dirty helpers

**Files:**
- Modify: `opt/Desktop/Apps/scripts/macos.ahk` (add a calibration section, e.g. after `_FlowToastDestroy` ~line 200, before `_FlowHoverClick`)

- [ ] **Step 1: Add the HUD + capture helpers**

Add this block to `macos.ahk`:

```ahk
; --- Calibration mode (F11) -----------------------------------------------------
; A persistent, NoActivate HUD pinned top-center (never covers the bottom overlay)
; showing the live WORKING offsets + a per-row saved/unsaved marker + the keymap.

_FlowCalibStartDirty() {
    global FlowStartX, FlowStartY, _flowCalib
    return (FlowStartX != _flowCalib["startX"] || FlowStartY != _flowCalib["startY"])
}
_FlowCalibStopDirty() {
    global FlowStopX, FlowStopY, _flowCalib
    return (FlowStopX != _flowCalib["stopX"] || FlowStopY != _flowCalib["stopY"])
}

_FlowCalibDestroy() {
    global _flowCalibGui
    if (_flowCalibGui) {
        try _flowCalibGui.Destroy()
        _flowCalibGui := ""
    }
}

; Build (or rebuild) the HUD from the current WORKING values.
_FlowCalibShow() {
    global _flowCalibGui, FlowStartX, FlowStartY, FlowStopX, FlowStopY
    static rainbow := ["FF8787","FFAF87","FFD787","AFFFAF","87D7FF","AFAFFF","D7AFFF","FFAFFF"]
    _FlowCalibDestroy()
    g := Gui("+AlwaysOnTop -Caption +ToolWindow +Border", "Flow Calibration")
    g.BackColor := "0B0E14"
    g.MarginX := 16, g.MarginY := 12

    ; rainbow per-char title
    g.SetFont "s15 Bold", "Consolas"
    Loop Parse "FLOW CALIBRATION" {
        g.SetFont "c" rainbow[Mod(A_Index - 1, rainbow.Length) + 1]
        g.Add("Text", (A_Index = 1 ? "xm ym" : "x+0 yp"), A_LoopField)
    }

    ; offset rows with per-row markers
    startMark := _FlowCalibStartDirty() ? "● unsaved" : "✓ saved"
    stopMark  := _FlowCalibStopDirty()  ? "● unsaved" : "✓ saved"
    g.SetFont "s12 Norm cD0D0D0", "Consolas"
    g.Add("Text", "xm y+14", Format("START   {1}, {2}     {3}", FlowStartX, FlowStartY, startMark))
    g.Add("Text", "xm y+4",  Format("STOP    {1}, {2}     {3}", FlowStopX,  FlowStopY,  stopMark))

    ; keymap
    g.SetFont "s11 c8A8A8A", "Consolas"
    g.Add("Text", "xm y+12", "F1 set START      F2 set STOP")
    g.Add("Text", "xm y+4",  "F3 revert         F4 save")
    g.Add("Text", "xm y+4",  "F5 defaults       F10 test")
    g.Add("Text", "xm y+4",  "F11 / Esc   end calibration")

    ; show off-screen to measure, center horizontally near the top, then reveal
    g.Show("NoActivate AutoSize Hide")
    g.GetPos(, , &w)
    g.Move((A_ScreenWidth - w) // 2, 40)
    g.Show("NoActivate")
    _flowCalibGui := g
}

; Capture the mouse position (relative to the Flow overlay) into a WORKING offset.
_FlowCalibCapture(which) {
    global FlowStartX, FlowStartY, FlowStopX, FlowStopY
    overlay := "Status ahk_exe Wispr Flow.exe"
    if !WinExist(overlay) {
        _FlowTip("✗  Flow overlay not found")
        SetTimer () => _FlowTip(""), -1500
        return
    }
    CoordMode "Mouse", "Screen"
    MouseGetPos &mx, &my
    WinGetPos &wx, &wy, , , overlay
    if (which = "start") {
        FlowStartX := mx - wx, FlowStartY := my - wy
    } else {
        FlowStopX := mx - wx, FlowStopY := my - wy
    }
    _FlowCalibShow()
}
```

- [ ] **Step 2: Validate**

```bash
cd $REPO
deploy_validate macos.ahk    # (define deploy_validate from the Conventions section first)
```

Expected: `validate 0`. (No behavior change yet — these helpers are not wired to keys until Task 4.)

- [ ] **Step 3: Commit checkpoint**

```bash
git add opt/Desktop/Apps/scripts/macos.ahk
# feat(wispr): calibration HUD (rainbow, top-center) + capture/dirty helpers
```

---

## Task 4: Calibration hotkeys (F11 toggle + scoped F1–F5/F10/Esc)

**Files:**
- Modify: `opt/Desktop/Apps/scripts/macos.ahk` (add after the existing `F10::{ ... }` dictation-toggle block, near the end of the file)

- [ ] **Step 1: Add the F11 global toggle and the scoped calibration keys**

Append this block to `macos.ahk` (after the existing F10 dictation toggle):

```ahk
; F11 toggles calibration mode. Only enters from IDLE; the Copilot key is ignored
; while active (see *F24). Rainbow ON / grey OFF toast, same as the F10 toggle.
F11::{
    global FlowState, CalibActive
    if (FlowState != "IDLE")
        return
    CalibActive := !CalibActive
    if (CalibActive)
        _FlowCalibShow()
    else
        _FlowCalibDestroy()
    _FlowToast(CalibActive ? "  Calibration  ON  " : "  Calibration  OFF  ", CalibActive)
}

; These keys are live ONLY while calibrating, so they keep their normal meaning
; otherwise (notably F10 stays the dictation toggle).
#HotIf CalibActive
F1::_FlowCalibCapture("start")
F2::_FlowCalibCapture("stop")
F3::{                                  ; revert WORKING <- last SAVED
    global FlowStartX, FlowStartY, FlowStopX, FlowStopY, _flowCalib
    FlowStartX := _flowCalib["startX"], FlowStartY := _flowCalib["startY"]
    FlowStopX  := _flowCalib["stopX"],  FlowStopY  := _flowCalib["stopY"]
    _FlowCalibShow()
}
F5::{                                  ; restore WORKING <- baked-in DEFAULT
    global FlowStartX, FlowStartY, FlowStopX, FlowStopY
    d := _FlowCalibDefaults()
    FlowStartX := d["startX"], FlowStartY := d["startY"]
    FlowStopX  := d["stopX"],  FlowStopY  := d["stopY"]
    _FlowCalibShow()
}
F4::{                                  ; save WORKING -> ini
    global FlowStartX, FlowStartY, FlowStopX, FlowStopY, _flowCalib
    m := Map("startX", FlowStartX, "startY", FlowStartY, "stopX", FlowStopX, "stopY", FlowStopY)
    _FlowCalibSave(_FlowCalibPath(), m)
    _flowCalib := m.Clone()             ; SAVED snapshot now matches WORKING (clears dirty)
    _FlowCalibShow()
    _FlowTip("✓  saved")
    SetTimer () => _FlowTip(""), -1200
}
F10::{                                 ; dry-run: click START, wait, click STOP
    global FlowStartX, FlowStartY, FlowStopX, FlowStopY
    _FlowClickOverlay(FlowStartX, FlowStartY)
    Sleep 2000
    _FlowClickOverlay(FlowStopX, FlowStopY)
}
Esc::{                                 ; exit calibration (same as F11)
    global CalibActive
    CalibActive := false
    _FlowCalibDestroy()
    _FlowToast("  Calibration  OFF  ", false)
}
#HotIf
```

- [ ] **Step 2: Validate + reload**

```bash
cd $REPO
AHK="/mnt/c/Users/<user>/AppData/Local/Programs/AutoHotkey/v2/AutoHotkey64.exe"
cp opt/Desktop/Apps/scripts/macos.ahk "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/macos.ahk"
powershell.exe -NoProfile -Command "\$p=Start-Process '$AHK' -ArgumentList '/validate','\"C:\Users\<user>\OneDrive\Desktop\Apps\scripts\macos.ahk\"' -Wait -PassThru -WindowStyle Hidden; 'validate '+\$p.ExitCode" 2>&1 | tr -d '\r'
powershell.exe -NoProfile -Command "taskkill /F /IM AutoHotkey64.exe 2>\$null; Start-Sleep 1; Start-Process '$AHK' -ArgumentList '\"C:\Users\<user>\OneDrive\Desktop\Apps\scripts\macos.ahk\"'" 2>&1 | tr -d '\r'
```

Expected: `validate 0`.

- [ ] **Step 3: Manual test checklist** (perform on the Windows desktop)

  - [ ] Press **F11** → rainbow "CALIBRATION ON" toast + the HUD appears top-center showing `START 440,560 ✓ saved` / `STOP 512,538 ✓ saved`.
  - [ ] Hover a new spot, press **F1** → START row updates to the new offset and shows `● unsaved`.
  - [ ] Press **F3** → START row reverts to `440,560 ✓ saved`.
  - [ ] Press **F5** → values become the defaults (still `440,560`/`512,538`; row marker reflects vs saved).
  - [ ] With Flow's overlay visible, press **F10** → cursor clicks START, pauses ~2 s, clicks STOP (watch both land).
  - [ ] Re-capture with **F1/F2**, press **F4** → `✓ saved` markers; verify the file exists:
    ```bash
    cat "/mnt/c/Users/<user>/AppData/Local/dotfiles/flow-calib.ini" | tr -d '\r'
    ```
  - [ ] Press **F11** (or **Esc**) → "CALIBRATION OFF" toast, HUD disappears.
  - [ ] Confirm scoping: outside calibration, **F10** still toggles dictation ON/OFF (not a dry-run).
  - [ ] Reload AHK (re-run the deploy command) → the saved offsets load (real Copilot-key dictation uses them).

- [ ] **Step 4: Commit checkpoint**

```bash
git add opt/Desktop/Apps/scripts/macos.ahk
# feat(wispr): F11 calibration mode — capture/test/save/revert overlay offsets
```

---

## Task 5: Docs + retire flow-coord-capture.ahk

**Files:**
- Modify: `opt/Desktop/Apps/scripts/WISPR-FLOW.md`
- Modify: `opt/Desktop/Apps/scripts/README.md`
- Delete: `opt/Desktop/Apps/scripts/flow-coord-capture.ahk`

- [ ] **Step 1: Update WISPR-FLOW.md — replace the "Overlay click coordinates" section**

Replace the current section (the paragraph about `flow-coord-capture.ahk` and the diagnostics note) with:

```markdown
## Overlay click coordinates & calibration

`macos.ahk` clicks the Flow "Status" overlay at offsets within that window
(anchored via `WinGetPos`, so they survive the widget moving). The baked-in
defaults are **start** `(440, 560)`, **stop** `(512, 538)` (200% display scaling);
your tuned values persist to `%LOCALAPPDATA%\dotfiles\flow-calib.ini` and override
the defaults at startup (so re-deploying `macos.ahk` won't lose them).

**To re-calibrate, press `F11`** (calibration mode). A rainbow HUD shows the live
offsets + keymap:

| Key | Action |
|-----|--------|
| `F1` / `F2` | capture the mouse position as the **start** / **stop** offset |
| `F3` | revert to the last **saved** values |
| `F5` | restore the baked-in **defaults** |
| `F4` | **save** to the ini |
| `F10` | **dry-run**: click start, wait ~2 s, click stop (watch both land) |
| `F11` / `Esc` | end calibration |

Hover the exact spot, `F1`/`F2` to capture, `F10` to test, `F4` to save.
```

- [ ] **Step 2: Update WISPR-FLOW.md — add F11 to the Controls table**

In the "Controls" table near the top, add a row:

```markdown
| **F11** | Toggle calibration mode — re-capture the overlay click offsets (rainbow HUD) |
```

- [ ] **Step 3: Update README.md**

In the script table, change the `macos.ahk` row to mention F11, and **remove** the `flow-coord-capture.ahk` row. New `macos.ahk` row:

```markdown
| `macos.ahk` | macOS-style Windows shortcuts (Cmd→Ctrl, screenshots, hot corners) **and** the Wispr Flow hover-dictate driver: the Copilot key (via the KBM `F24` remap) clicks Flow's overlay to dictate into the field under the mouse. `Esc` cancels; `F10` toggles dictation; **`F11`** opens calibration mode to re-tune the overlay offsets. |
```

Also remove the `flow-coord-capture.ahk` row from the table.

- [ ] **Step 4: Delete the superseded capture tool**

```bash
cd $REPO
git rm opt/Desktop/Apps/scripts/flow-coord-capture.ahk
rm -f "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/flow-coord-capture.ahk"
```

(If `flow-coord-capture.ahk` is referenced anywhere else, grep and clean: `grep -rn flow-coord-capture opt/ docs/`.)

- [ ] **Step 5: Commit checkpoint**

```bash
git add opt/Desktop/Apps/scripts/WISPR-FLOW.md opt/Desktop/Apps/scripts/README.md
# (flow-coord-capture.ahk deletion already staged via git rm)
# docs(wispr): document F11 calibration mode; retire flow-coord-capture.ahk
```

---

## Task 6: Final verification + sync

- [ ] **Step 1: Re-run the config test (regression)**

```bash
cd $REPO
AHK="/mnt/c/Users/<user>/AppData/Local/Programs/AutoHotkey/v2/AutoHotkey64.exe"
powershell.exe -NoProfile -Command "\$p=Start-Process '$AHK' -ArgumentList '\"C:\Users\<user>\OneDrive\Desktop\Apps\scripts\flow-calib-test.ahk\"' -Wait -PassThru -WindowStyle Hidden; 'exit '+\$p.ExitCode" 2>&1 | tr -d '\r'
cat "/mnt/c/Users/<user>/AppData/Local/Temp/flow-calib-test.out" | tr -d '\r'
```

Expected: `exit 0`, `OK: all calibration config tests passed`.

- [ ] **Step 2: Final `/validate` of macos.ahk** (expected `validate 0`).

- [ ] **Step 3: Sync to PR #53** via the gss flow with `AskUserQuestion` confirmation, then refresh the PR body (re-derive What/Why/Impact/Testing from `git log origin/main..HEAD`), per repo policy. Two-call token recipe for the push.

---

## Notes / decisions baked in

- **Persistence:** external ini at `%LOCALAPPDATA%\dotfiles\flow-calib.ini` (survives reload + re-deploy); repo defaults are the baseline. *(spec decision)*
- **UI:** persistent top-center rainbow HUD + brief enter/exit toast. *(spec decision)*
- **Reset:** `F3` reverts to last saved, `F5` restores defaults. *(spec decision)*
- **F10 test:** dry-run START→STOP. *(spec decision)*
- **Testability:** the config layer is a separate `#Include` with zero side effects, so `flow-calib-test.ahk` can exercise it headlessly; the GUI/clicks are validated manually + `/validate`.
- **Deploy note:** `flow-calib.ahk` must be deployed alongside `macos.ahk` (same dir) because of the `#Include`. The whole `opt/Desktop/Apps/scripts/` dir already deploys together, so no install-path change is required — but verify both land in `DEST`.
```
