# Wispr Flow Hover-Dictate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Copilot key hold-to-talk in Wispr Flow and drop the transcript into the window/field the mouse was hovering, with lightweight processing feedback.

**Architecture:** A small state machine in `macos.ahk` (AutoHotkey v2): on Copilot-key down it saves the window+point under the mouse, focuses+caret there, and holds Flow's combo; on key up it releases; when Flow finishes (clipboard change) it re-focuses the saved target and pastes. Two unknowns are resolved by a manual spike first, each with a working fallback.

**Tech Stack:** AutoHotkey v2 (`AutoHotkey64.exe`), Wispr Flow, Windows/WSL deploy via `cp` to the OneDrive Desktop. No headless test harness exists for interactive hotkeys, so gates are `/validate` (static) plus an explicit manual behavioral checklist.

---

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `opt/Desktop/Apps/scripts/copilot-key-probe.ahk` | One-off diagnostic: log `F23` down/up + hold behavior to confirm R1 | Create |
| `opt/Desktop/Apps/scripts/macos.ahk` | Replace the single-line `*F23` shim with the hover-dictate state machine | Modify (the `*F23` block only) |
| `opt/Desktop/Apps/scripts/WISPR-FLOW.md` | Document the new hold-to-talk + hover behavior and the Flow clipboard setting | Modify |

**Spec:** `docs/superpowers/specs/2026-05-30-wispr-flow-hover-dictate-design.md`

**Deploy + reload (used by several tasks).** macos.ahk runs from the local Desktop copy; after editing the repo file, sync and reload the elevated instance:
```bash
cp opt/Desktop/Apps/scripts/macos.ahk "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/macos.ahk"
```
Then, in a Windows terminal (one UAC prompt), restart it:
```
taskkill /F /IM AutoHotkey64.exe
"C:\Users\<user>\AppData\Local\Programs\AutoHotkey\v2\AutoHotkey64.exe" "C:\Users\<user>\OneDrive\Desktop\Apps\scripts\macos.ahk"
```

---

## Task 1: Spike — confirm R1 (key holds?) and R2 (Flow clipboard?)

**This task is a hard gate.** Its results choose the implementation path in Task 2. STOP and report results before continuing.

**Files:**
- Create: `opt/Desktop/Apps/scripts/copilot-key-probe.ahk`

- [ ] **Step 1: Write the probe script**

```ahk
#Requires AutoHotkey v2.0
#SingleInstance Force
; Logs every F23 event so we can see whether the Copilot key auto-repeats while
; held (R1=holds) or fires once (R1=momentary). Watch the tooltip while you press
; and HOLD the Copilot key for ~2 seconds, then release.
_n := 0
*F23::{
    global _n
    _n += 1
    ToolTip "F23 DOWN #" _n "  (t=" A_TickCount ")"
}
*F23 up::{
    global _n
    ToolTip "F23 UP after " _n " down event(s)"
    SetTimer () => ToolTip(), -2500
    _n := 0
}
Esc::ExitApp
```

- [ ] **Step 2: Deploy + run the probe**

```bash
cp opt/Desktop/Apps/scripts/copilot-key-probe.ahk "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/copilot-key-probe.ahk"
```
Run it from a Windows terminal:
```
"C:\Users\<user>\AppData\Local\Programs\AutoHotkey\v2\AutoHotkey64.exe" "C:\Users\<user>\OneDrive\Desktop\Apps\scripts\copilot-key-probe.ahk"
```

- [ ] **Step 3: Observe (manual)**

Press and HOLD the Copilot key ~2s, then release.
- **R1 = HOLDS** if the tooltip shows "DOWN #2, #3, …" climbing while held (auto-repeat) and a single "UP" on release.
- **R1 = MOMENTARY** if it shows only "DOWN #1" no matter how long you hold (no repeat), or no "UP".
Press `Esc` to exit the probe. Record R1.

- [ ] **Step 4: Check R2 in Flow (manual)**

Open Wispr Flow → Settings → look for an option like "Copy to clipboard", "Paste from clipboard", or "Don't auto-insert / don't type". Record R2 = CLIPBOARD-AVAILABLE or R2 = TYPES-ONLY.

- [ ] **Step 5: Commit the probe**

```bash
git add opt/Desktop/Apps/scripts/copilot-key-probe.ahk
git commit -m "test(wispr): add copilot-key-probe.ahk to verify key-hold behavior"
```

- [ ] **Step 6: Decide the path**
  - R1 HOLDS → use **hold** handlers (Task 2 primary). R1 MOMENTARY → use **toggle** variant (Task 2A).
  - R2 CLIPBOARD → use **Model B** paste (Task 2 primary). R2 TYPES-ONLY → use **Model A** variant (Task 2B); set Flow's shortcut to push-to-talk (or toggle) accordingly.

---

## Task 2: Hover-dictate state machine (primary: hold + Model B)

Use this when **R1 = HOLDS and R2 = CLIPBOARD**. (In Flow, set push-to-talk = `Ctrl+Shift+F12` and enable copy-to-clipboard / disable auto-typing.)

**Files:**
- Modify: `opt/Desktop/Apps/scripts/macos.ahk` (replace the `*F23::Send …` line and its comment block)

- [ ] **Step 1: Replace the `*F23` shim with the state machine**

Replace the existing line `*F23::Send "{LWin up}{LShift up}^+{F12}"` (and update the comment above it) with:

```ahk
; The Copilot key now drives Flow "hover-dictate": hold to record into the field
; under the mouse; on release Flow transcribes to the clipboard; we then re-focus
; that field and paste. See WISPR-FLOW.md and docs/superpowers/specs/.
global FlowState := "IDLE"          ; IDLE | DICTATING | AWAITING_CLIP
global FlowWin   := 0
global FlowX     := 0
global FlowY     := 0

_FlowTip(msg) {
    CoordMode "ToolTip", "Screen"
    if (msg = "") {
        ToolTip
    } else {
        CoordMode "Mouse", "Screen"
        MouseGetPos &tx, &ty
        ToolTip msg, tx + 18, ty + 18
    }
}

*F23::{           ; Copilot key down
    global FlowState, FlowWin, FlowX, FlowY
    if (FlowState != "IDLE")
        return                       ; swallow auto-repeat while held
    CoordMode "Mouse", "Screen"
    MouseGetPos &FlowX, &FlowY, &FlowWin
    if FlowWin
        WinActivate "ahk_id " FlowWin
    Click FlowX, FlowY               ; place the caret under the mouse
    FlowState := "DICTATING"
    Send "{LWin up}{LShift up}{Ctrl down}{Shift down}{F12 down}"
    _FlowTip("🎤  Listening…")
}

*F23 up::{        ; Copilot key released
    global FlowState
    if (FlowState != "DICTATING")
        return
    Send "{F12 up}{Shift up}{Ctrl up}"
    FlowState := "AWAITING_CLIP"
    _FlowTip("⏳  Transcribing…")
    SetTimer _FlowTimeout, -15000    ; safety net if no transcript arrives
}

_FlowTimeout() {
    global FlowState
    if (FlowState = "AWAITING_CLIP") {
        FlowState := "IDLE"
        _FlowTip("")
    }
}

_FlowOnClip(dataType) {              ; fires when Flow drops the transcript on the clipboard
    global FlowState, FlowWin, FlowX, FlowY
    if (FlowState != "AWAITING_CLIP" || dataType != 1)   ; 1 = text
        return
    SetTimer _FlowTimeout, 0         ; cancel the safety timer
    FlowState := "IDLE"
    if (FlowWin && WinExist("ahk_id " FlowWin)) {
        WinActivate "ahk_id " FlowWin
        CoordMode "Mouse", "Screen"
        Click FlowX, FlowY
    }
    Send "^v"
    _FlowTip("")
}

OnClipboardChange _FlowOnClip
```

- [ ] **Step 2: Static validation**

```bash
cp opt/Desktop/Apps/scripts/macos.ahk "/mnt/c/Users/<user>/OneDrive/Desktop/Apps/scripts/macos.ahk"
powershell.exe -NoProfile -Command "\$p=Start-Process 'C:\Users\<user>\AppData\Local\Programs\AutoHotkey\v2\AutoHotkey64.exe' -ArgumentList '/validate','\"C:\Users\<user>\OneDrive\Desktop\Apps\scripts\macos.ahk\"' -Wait -PassThru -WindowStyle Hidden; 'validate exit: '+\$p.ExitCode"
```
Expected: `validate exit: 0`

- [ ] **Step 3: Reload AHK (manual, one UAC prompt)**

```
taskkill /F /IM AutoHotkey64.exe
"C:\Users\<user>\AppData\Local\Programs\AutoHotkey\v2\AutoHotkey64.exe" "C:\Users\<user>\OneDrive\Desktop\Apps\scripts\macos.ahk"
```

- [ ] **Step 4: Manual behavioral test**

  - Hover a text field (e.g. Notepad), hold the Copilot key, speak, release → tooltip shows Listening→Transcribing, then the transcript pastes at the caret.
  - Hover a different app's field and repeat → text lands there.
  - Hold, say nothing, release → no stray paste within ~15s (tooltip clears).
  - With IDLE state, press `Ctrl+C` elsewhere → nothing pastes (clipboard change ignored).
  - Confirm `Cmd+C`/`Cmd+V` and other macOS shortcuts still work.

- [ ] **Step 5: Commit**

```bash
git add opt/Desktop/Apps/scripts/macos.ahk
git commit -m "feat(wispr): hover-dictate — hold Copilot key to record, paste at mouse target"
```

---

## Task 2A: Fallback — toggle gesture (use only if R1 = MOMENTARY)

If the Copilot key can't hold, replace the **down/up handlers** from Task 2 Step 1 with the toggle version below (keep `_FlowTip`, the globals, `_FlowTimeout`, `_FlowOnClip`, and `OnClipboardChange` unchanged). In Flow, set the shortcut to a **toggle/hands-free** action on `Ctrl+Shift+F12`.

- [ ] **Step 1: Swap in the toggle handler**

```ahk
*F23::{           ; tap 1 = start (focus+caret, toggle Flow on); tap 2 = stop
    global FlowState, FlowWin, FlowX, FlowY
    if (FlowState = "IDLE") {
        CoordMode "Mouse", "Screen"
        MouseGetPos &FlowX, &FlowY, &FlowWin
        if FlowWin
            WinActivate "ahk_id " FlowWin
        Click FlowX, FlowY
        FlowState := "DICTATING"
        Send "{LWin up}{LShift up}^+{F12}"     ; toggle Flow ON
        _FlowTip("🎤  Listening… (tap again to stop)")
    } else if (FlowState = "DICTATING") {
        Send "^+{F12}"                          ; toggle Flow OFF
        FlowState := "AWAITING_CLIP"
        _FlowTip("⏳  Transcribing…")
        SetTimer _FlowTimeout, -15000
    }
}
```

- [ ] **Step 2:** Re-run Task 2 Steps 2–5 (validate, reload, manual test with tap-tap instead of hold, commit with message `feat(wispr): hover-dictate (toggle gesture) — Copilot key is momentary`).

---

## Task 2B: Fallback — Flow types (use only if R2 = TYPES-ONLY)

If Flow can't copy-to-clipboard, drop the clipboard paste and let Flow type into the field we focused at press. Use with either the hold (Task 2) or toggle (Task 2A) down/up handler, but make these edits:

- [ ] **Step 1: Remove the clipboard paste path**

  - Delete the `_FlowOnClip` function and the `OnClipboardChange _FlowOnClip` line.
  - In `*F23 up::` (or the toggle stop branch), replace the `SetTimer _FlowTimeout, -15000` line's purpose: keep the tooltip but clear it on the timer only:

```ahk
    FlowState := "IDLE"
    _FlowTip("⏳  Transcribing…")
    SetTimer () => _FlowTip(""), -4000     ; Flow types into the focused field itself
```
  (Keep `_FlowTimeout` removed in this variant; the inline timer handles tooltip cleanup.)

- [ ] **Step 2:** Re-run Task 2 Steps 2–5 (validate, reload, manual test — transcript should be typed by Flow into the field focused at press, no AHK paste; commit with message `feat(wispr): hover-dictate (Flow types) — clipboard mode unavailable`).

---

## Task 3: Document the new behavior

**Files:**
- Modify: `opt/Desktop/Apps/scripts/WISPR-FLOW.md`

- [ ] **Step 1: Update the AHK shim section**

In the "AutoHotkey shim" section, replace the single-line shim description with the hover-dictate behavior: hold (or tap-tap, per the spike) the Copilot key to dictate into the field under the mouse; Flow set to `Ctrl+Shift+F12` as push-to-talk (or toggle), and — for Model B — copy-to-clipboard enabled. Note the `~15s` safety timeout and that other macOS shortcuts are unaffected. Reference `docs/superpowers/specs/2026-05-30-wispr-flow-hover-dictate-design.md`.

- [ ] **Step 2: Commit**

```bash
git add opt/Desktop/Apps/scripts/WISPR-FLOW.md
git commit -m "docs(wispr): document hover-dictate behavior + required Flow settings"
```

---

## Self-Review notes

- **Spec coverage:** hold-to-talk (Task 2 / 2A), hover target (Task 2 Step 1 MouseGetPos+WinActivate+Click), clipboard-trigger paste (Task 2 `_FlowOnClip`), feedback tooltip (`_FlowTip`), safety timeout (`_FlowTimeout`), both spike fallbacks (2A, 2B), docs (Task 3). All spec sections map to a task.
- **Names consistent:** `FlowState`, `FlowWin/X/Y`, `_FlowTip`, `_FlowTimeout`, `_FlowOnClip` used identically across tasks.
- **No headless unit tests:** interactive AHK hotkeys can't be tested from WSL; gates are `/validate` + the explicit manual checklist. This is called out, not a placeholder.
