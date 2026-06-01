#Requires AutoHotkey v2.0
#SingleInstance Force
#Include flow-calib.ahk      ; overlay-offset calibration data layer (DEFAULT + ini load/save)
#Include flow-triggers.ahk   ; extra-trigger-key data + policy layer (load/save/normalize/compose/validate/manifest)

; ==============================================================================
;  macOS-style shortcuts for Windows  (AutoHotkey v2)
; ------------------------------------------------------------------------------
;  Left Windows key (LWin)  ->  acts like the macOS Command (Cmd) key
;  Left Alt           (!)   ->  acts like the macOS Option (Opt) key
;
;  This is the single canonical script. Old v1 files live in _archive_v1\.
; ==============================================================================

cmdTabActive := false

; --- Make a lone Cmd (LWin) tap do nothing, like macOS --------------------------
; Tilde keeps LWin working as a modifier; the unassigned vkE8 keystroke absorbs
; the press so Windows never shows the Start menu when Cmd is tapped on its own.
;
; CONFLICT NOTE: Wispr Flow's DEFAULT hotkey is Ctrl+Win, so its keyboard hook
; watches this same Win key and breaks the Cmd+* shortcuts below. Rebind ALL of
; Flow's shortcuts off the Win key (any non-Win combo); the Copilot key drives Flow
; via macos.ahk's overlay clicks, not a Flow hotkey. See WISPR-FLOW.md.
~LWin::Send "{Blind}{vkE8}"

~LWin Up::
{
    global cmdTabActive
    if cmdTabActive
    {
        Send "{Blind}{Alt up}"      ; release Alt -> commit the Cmd+Tab choice
        cmdTabActive := false
    }
}

; --- Core editing ---------------------------------------------------------------
<#c::Send "^c"                  ; Cmd+C        Copy
<#v::Send "^v"                  ; Cmd+V        Paste
<#+v::Send "^+v"                ; Cmd+Shift+V  Paste and match style
<#x::Send "^x"                  ; Cmd+X        Cut
<#a::Send "^a"                  ; Cmd+A        Select all
<#z::Send "^z"                  ; Cmd+Z        Undo
<#+z::Send "^y"                 ; Cmd+Shift+Z  Redo
<#s::Send "^s"                  ; Cmd+S        Save
<#+s::Send "^+s"                ; Cmd+Shift+S  Save as
<#f::Send "^f"                  ; Cmd+F        Find
<#g::Send "{F3}"                ; Cmd+G        Find next
<#+g::Send "+{F3}"              ; Cmd+Shift+G  Find previous
<#o::Send "^o"                  ; Cmd+O        Open
<#n::Send "^n"                  ; Cmd+N        New
<#p::Send "^p"                  ; Cmd+P        Print
<#/::Send "^/"                  ; Cmd+/        Toggle line comment (editors)

; --- Window & app management ----------------------------------------------------
<#w::Send "^w"                  ; Cmd+W        Close tab / window
<#m::WinMinimize "A"            ; Cmd+M        Minimize active window
<#h::WinMinimize "A"            ; Cmd+H        Hide (minimize) active window
<#!Esc::Send "^+{Esc}"          ; Cmd+Opt+Esc  Force quit (Task Manager)

<#q::                           ; Cmd+Q        Quit active app
{
    if InStr(WinGetTitle("A"), "World of Warcraft")
        return                  ; never Alt+F4 the game
    Send "!{F4}"
}

; --- Cmd+Tab app switcher (hold Cmd, tap Tab to cycle, release to choose) --------
<#Tab::
{
    global cmdTabActive
    cmdTabActive := true
    Send "{Blind}{Alt down}{Tab}"
}
<#+Tab::
{
    global cmdTabActive
    cmdTabActive := true
    Send "{Blind}{Alt down}{Shift down}{Tab}{Shift up}"
}

; --- Navigation & system --------------------------------------------------------
<#Space::Send "#s"              ; Cmd+Space    Spotlight (Windows Search)
<#l::Send "^l"                  ; Cmd+L        Focus address bar
<#r::Send "{F5}"                ; Cmd+R        Reload / refresh

; --- Line navigation (Cmd + arrows) ---------------------------------------------
<#Left::Send "{Home}"           ; Cmd+Left     Start of line
<#Right::Send "{End}"           ; Cmd+Right    End of line
<#Up::Send "^{Home}"            ; Cmd+Up       Top of document
<#Down::Send "^{End}"           ; Cmd+Down     Bottom of document

; --- Line selection (Cmd + Shift + arrows) --------------------------------------
<#+Left::Send "+{Home}"         ; select to start of line
<#+Right::Send "+{End}"         ; select to end of line
<#+Up::Send "+^{Home}"          ; select to top of document
<#+Down::Send "+^{End}"         ; select to bottom of document

; --- Delete behaviour -----------------------------------------------------------
<#Backspace::Send "+{Home}{Del}"   ; Cmd+Delete   Delete to start of line
!Backspace::Send "^{Backspace}"    ; Opt+Delete   Delete previous word

; --- Word navigation (Option / Alt + arrows) ------------------------------------
!Left::Send "^{Left}"           ; Opt+Left          Previous word
!Right::Send "^{Right}"         ; Opt+Right         Next word
!+Left::Send "+^{Left}"         ; Opt+Shift+Left    Select previous word
!+Right::Send "+^{Right}"       ; Opt+Shift+Right   Select next word

; --- Browser tabs ---------------------------------------------------------------
<#t::Send "^t"                  ; Cmd+T        New tab
<#+t::Send "^+t"                ; Cmd+Shift+T  Reopen closed tab
<#[::Send "!{Left}"             ; Cmd+[        Back
<#]::Send "!{Right}"            ; Cmd+]        Forward
<#1::Send "^1"                  ; Cmd+1..8     Jump to tab N
<#2::Send "^2"
<#3::Send "^3"
<#4::Send "^4"
<#5::Send "^5"
<#6::Send "^6"
<#7::Send "^7"
<#8::Send "^8"
<#9::Send "^9"                  ; Cmd+9        Jump to last tab

; --- Screenshots ----------------------------------------------------------------
<#+3::Send "#{PrintScreen}"     ; Cmd+Shift+3  Full screen -> Pictures\Screenshots
<#+4::Send "#+s"                ; Cmd+Shift+4  Region capture (Snip & Sketch)
^+c::CaptureActiveWindow()      ; Ctrl+Shift+C Capture active window -> Pictures\Screenshots

; ==============================================================================
;  Copilot key  ->  Wispr Flow "hover-dictate" (overlay-click driver)
; ------------------------------------------------------------------------------
;  The Copilot key emits LWin+LShift+F23. Trying to suppress that chord in AHK is a
;  RACE against Windows' own Copilot handler (it intermittently opens the "Customize
;  Copilot key" Settings page). So we take Windows out of the race: PowerToys
;  Keyboard Manager remaps Win+Shift+F23 -> F24 (a clean unused key) BEFORE Windows
;  sees it. We therefore bind F24 here, not F23. (KBM-only is enabled; FancyZones /
;  PowerToys Run / Shortcut Guide are disabled so KBM doesn't fight the Cmd=Win
;  mappings above. See WISPR-FLOW.md. The overlay click offsets are calibratable
;  live via F11 — see the calibration block below.)
;
;  Wispr Flow IGNORES injected keystrokes, but it DOES accept injected mouse clicks
;  on its always-visible "Status" overlay. So we drive Flow by clicking the overlay:
;    * key down -> remember the window/point under the mouse (the paste target),
;      then click the overlay START spot TWICE (1st click focuses the overlay
;      window, 2nd actually starts dictation).
;    * key up   -> click the overlay STOP icon ONCE.
;    * Flow drops the transcript on the CLIPBOARD; OnClipboardChange re-activates
;      the saved target, clicks to set the caret, and pastes. A 15s timer resets
;      state if nothing arrives.
;  The real mouse position is saved/restored around each overlay click, so the
;  cursor just flicks to the widget and back.
;
;  Click points are OFFSETS within the "Status" overlay window (anchored via
;  WinGetPos so they survive the widget moving). Re-capture live with the F11
;  calibration mode if the overlay layout changes. See WISPR-FLOW.md and
;  docs/superpowers/specs/2026-05-31-flow-calibration-mode-design.md.
; ==============================================================================
FlowState := "IDLE"          ; IDLE | DICTATING | AWAITING_CLIP
FlowWin   := 0
FlowX     := 0
FlowY     := 0
FlowAutoPaste := false       ; Flow auto-pastes the transcript itself, so we do NOT Ctrl+V
                             ; (that would double-paste). Set true only if a Flow update
                             ; ever stops auto-pasting and we need to paste ourselves.
FlowEnabled := true          ; F10 toggles this; when false the Copilot key does nothing.
_flowToastGui := ""          ; current toggle popup (so a rapid re-toggle replaces it)

; Pastel-rainbow palette shared by the F10/F11 toasts and the calibration HUD title
; (same hues as the install.sh end banner). Single source so they can't drift.
_FLOW_RAINBOW := ["FF8787","FFAF87","FFD787","AFFFAF","87D7FF","AFAFFF","D7AFFF","FFAFFF"]

; Overlay click offsets — WORKING set, used by the click functions below.
; Loaded from the calibration ini at startup (falls back to _FlowCalibDefaults()).
; _flowCalib holds the SAVED snapshot for the dirty check + F3 revert (F11 mode).
; Guard the load: a calibration-ini problem must never prevent the rest of this
; daily-driver script (Cmd shortcuts, screenshots, dictation) from loading.
try
    _flowCalib := _FlowCalibLoad(_FlowCalibPath())
catch
    _flowCalib := _FlowCalibDefaults()
FlowStartX := _flowCalib["startX"]
FlowStartY := _flowCalib["startY"]
FlowStopX  := _flowCalib["stopX"]
FlowStopY  := _flowCalib["stopY"]

; Extra trigger keys (the Copilot key F24 is the locked default and is NOT stored).
; Loaded at startup from the per-machine flow-triggers.ini, beside the calib load so
; it inherits the same proven startup-execution path. Guarded identically: a corrupt
; or hand-edited ini must degrade to "no extra triggers" ([]), never crash startup.
try
    _flowTriggers := _FlowTriggersLoad(_FlowTriggersPath())
catch
    _flowTriggers := []

; Bind each saved trigger to the same hold-to-talk handlers as the Copilot key, under
; the GLOBAL #HotIf context (no #HotIf is open here). Each Hotkey() call is individually
; try-guarded so one bad entry is skipped, not fatal. _FlowTriggerDown/Up are defined
; later in the file but referenced by name (AHK v2 resolves the reference at load).
for t in _flowTriggers {
    try Hotkey "*" t, _FlowTriggerDown, "On"
    try Hotkey "*" t " up", _FlowTriggerUp, "On"
}

CalibActive   := false       ; F11 calibration mode flag
_flowCalibGui := ""          ; persistent calibration HUD handle
TriggerMgmtActive := false   ; F9 manage-triggers mode flag (mutual-exclusive with calib)
_flowTriggerGui   := ""      ; persistent manage-triggers HUD handle
_flowHelpGui      := ""      ; persistent hold-F1 help HUD handle
_flowInputHook    := ""      ; live InputHook while managing (F9)
_FLOW_HINT        := "   F1 help"   ; suffix appended to the listening/transcribing tips

_FlowTip(msg) {
    if (msg = "") {
        ToolTip
        return
    }
    CoordMode "ToolTip", "Screen"
    CoordMode "Mouse", "Screen"
    MouseGetPos &tx, &ty
    ToolTip msg, tx + 18, ty + 18
}

; Show a transient tip and auto-clear it after `ms` milliseconds (one-shot timer).
_FlowTipFor(msg, ms) {
    _FlowTip(msg)
    SetTimer () => _FlowTip(""), -ms
}

; Centered, color-coded popup shown for ~1s (green=ON, grey=OFF). NoActivate so it
; never steals focus; replaces any previous popup if toggled rapidly.
_FlowToast(msg, isOn) {
    global _flowToastGui, _FLOW_RAINBOW
    if IsObject(_flowToastGui)
        try _flowToastGui.Destroy()
    ; pastel rainbow when ON, muted grey when OFF.
    static grey := ["8A8A8A","A8A8A8"]
    hues := isOn ? _FLOW_RAINBOW : grey
    g := Gui("-Caption +AlwaysOnTop +ToolWindow +Disabled", "")
    g.MarginX := 30, g.MarginY := 18
    g.BackColor := "0B0E14"                              ; dark backdrop so the pastels pop
    g.SetFont("s22 Bold", "Consolas")                    ; monospace -> clean per-character row
    Loop Parse msg {                                     ; one Text control per char, rainbow-cycled
        opt := "c" hues[Mod(A_Index - 1, hues.Length) + 1]
        if (A_Index > 1)
            opt := "x+0 " opt
        g.AddText(opt, A_LoopField)
    }
    g.Show("NoActivate Center AutoSize")
    _flowToastGui := g
    SetTimer(() => _FlowToastDestroy(g), -1100)
}
_FlowToastDestroy(g) {
    global _flowToastGui
    try g.Destroy()
    if (_flowToastGui == g)
        _flowToastGui := ""
}

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

; Shared HUD builder for the calibration / manage / help overlays — pins the EXACT
; calibration geometry & fonts so the three HUDs can't drift. Builds a pinned-top-
; center, NoActivate (never steals focus / never covers the bottom overlay) window:
;   title       -> rainbow per-char heading (s15 Bold)
;   rows         -> body lines (s12 Norm cD0D0D0); first row y+14, rest y+4
;   keymapLines  -> dimmer keymap block (s11 c8A8A8A); first line y+12, rest y+4
; `winTitle` is the OS-level (hidden, -Caption) window title; it defaults to `title`
; but is overridable so a HUD can keep its PR-#53 window-title string distinct from
; the visible heading (e.g. calibration's "Flow Calibration" vs heading "FLOW CALIBRATION").
; Returns the shown Gui handle (caller stores + destroys it).
_FlowHud(title, rows, keymapLines, winTitle := title) {
    global _FLOW_RAINBOW
    g := Gui("+AlwaysOnTop -Caption +ToolWindow +Border", winTitle)
    g.BackColor := "0B0E14"
    g.MarginX := 16, g.MarginY := 12

    ; rainbow per-char title
    g.SetFont "s15 Bold", "Consolas"
    Loop Parse title {
        g.SetFont "c" _FLOW_RAINBOW[Mod(A_Index - 1, _FLOW_RAINBOW.Length) + 1]
        g.Add("Text", (A_Index = 1 ? "xm ym" : "x+0 yp"), A_LoopField)
    }

    ; body rows
    g.SetFont "s12 Norm cD0D0D0", "Consolas"
    first := true
    for row in rows {
        g.Add("Text", (first ? "xm y+14" : "xm y+4"), row)
        first := false
    }

    ; keymap
    g.SetFont "s11 c8A8A8A", "Consolas"
    first := true
    for line in keymapLines {
        g.Add("Text", (first ? "xm y+12" : "xm y+4"), line)
        first := false
    }

    ; Center on screen via AHK's built-in (handles DPI scaling correctly; manual
    ; A_ScreenWidth math mismatches units at >100% scaling and lands off-screen).
    g.Show("NoActivate AutoSize Center")
    return g
}

; Build (or rebuild) the calibration HUD from the current WORKING values, via the
; shared _FlowHud builder so its geometry/fonts stay pinned to the manage/help HUDs.
_FlowCalibShow() {
    global _flowCalibGui, FlowStartX, FlowStartY, FlowStopX, FlowStopY
    _FlowCalibDestroy()
    startMark := _FlowCalibStartDirty() ? "● unsaved" : "✓ saved"
    stopMark  := _FlowCalibStopDirty()  ? "● unsaved" : "✓ saved"
    rows := [Format("START   {1}, {2}     {3}", FlowStartX, FlowStartY, startMark),
             Format("STOP    {1}, {2}     {3}", FlowStopX,  FlowStopY,  stopMark)]
    keymap := ["F1 set START      F2 set STOP",
               "F3 revert         F4 save",
               "F5 defaults       F10 test",
               "F11 / Esc   end calibration"]
    _flowCalibGui := _FlowHud("FLOW CALIBRATION", rows, keymap, "Flow Calibration")
}

; Capture the mouse position (relative to the Flow overlay) into a WORKING offset.
_FlowCalibCapture(which) {
    global FlowStartX, FlowStartY, FlowStopX, FlowStopY
    overlay := "Status ahk_exe Wispr Flow.exe"
    if !WinExist(overlay) {
        _FlowTipFor("✗  Flow overlay not found", 1500)
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

; Move at natural speed (fires the Electron overlay's hover/mouseenter), DWELL, then
; click. The hover+dwell is REQUIRED: a teleport-then-click lands on the button but
; the overlay ignores it (it only "arms" a button that's been hovered).
_FlowHoverClick(x, y, dwell := 150) {
    CoordMode "Mouse", "Screen"
    MouseMove x, y
    Sleep dwell
    Click x " " y
}

; Click the Flow "Status" overlay at (offX,offY) within the overlay window. We hover
; ~1.2s FIRST so Flow expands its overlay and wins the focus competition (e.g. with
; side-by-side WSL windows whose divider crosses the overlay) before we click — a
; single dwell-then-click replaces the old focus-click + activate-click pair.
; Returns false if the overlay window isn't found.
_FlowClickOverlay(offX, offY) {
    SetTitleMatchMode 3                                   ; exact title "Status"
    if !WinExist("Status ahk_exe Wispr Flow.exe")
        return false
    WinGetPos &wx, &wy, &ww, &wh, "Status ahk_exe Wispr Flow.exe"
    _FlowHoverClick(wx + offX, wy + offY, 1200)           ; ~1.2s dwell for expand+focus, then click
    return true
}

_FlowTimeout() {
    global FlowState
    if (FlowState = "AWAITING_CLIP") {
        FlowState := "IDLE"
        _FlowTip("")
    }
}

_FlowOnClip(dataType) {       ; fires when Flow puts the transcript on the clipboard
    global FlowState, FlowWin, FlowX, FlowY, FlowAutoPaste
    if (FlowState != "AWAITING_CLIP" || dataType != 1)   ; 1 = text
        return
    SetTimer _FlowTimeout, 0                              ; cancel safety timer
    FlowState := "IDLE"
    if (FlowWin && WinExist("ahk_id " FlowWin)) {
        WinActivate "ahk_id " FlowWin
        WinWaitActive "ahk_id " FlowWin, , 1
    }
    _FlowHoverClick(FlowX, FlowY)                        ; re-focus the target / set caret where you pressed
    if (FlowAutoPaste)
        Send "^v"                                        ; our paste — disabled by default (Flow auto-pastes); see FlowAutoPaste
    _FlowTipFor(FlowAutoPaste ? "✓  pasted" : "✓  (Flow auto-paste)", 1200)
}

OnClipboardChange _FlowOnClip

; The F24 handlers MUST return instantly. While the Copilot key is held it auto-
; repeats (F24, via the KBM remap), and the overlay START click does a ~1.2s
; hover-dwell — doing that work inline would block the hotkey thread and stack up
; repeats. So the hotkeys only flip state + show the tip, and the slow dwell-click
; runs on a separate timer thread.
_FlowStartClicks() {
    global FlowState, FlowX, FlowY, FlowStartX, FlowStartY
    if !_FlowClickOverlay(FlowStartX, FlowStartY) {       ; START: WORKING offset (calibratable via F11)
        FlowState := "IDLE"
        _FlowTipFor("Flow overlay not found", 1500)
        return
    }
    MouseMove FlowX, FlowY, 0                             ; flick the cursor back to your target
}

_FlowStopClicks() {
    global FlowX, FlowY, FlowStopX, FlowStopY
    _FlowClickOverlay(FlowStopX, FlowStopY)               ; STOP: WORKING offset (calibratable via F11)
    MouseMove FlowX, FlowY, 0
}

; Shared hold-to-talk handlers. The Copilot key (*F24) AND every dynamically-bound
; extra trigger (see the startup bind loop) route through these so all triggers share
; one path. single-flight: FIRST trigger released while DICTATING drives STOP (no per-key
; ownership) — a 2nd trigger pressed mid-session hits the auto-repeat swallow (no-op).
_FlowTriggerDown() {
    global FlowState, FlowWin, FlowX, FlowY, FlowEnabled, CalibActive, TriggerMgmtActive, _FLOW_HINT
    if (!FlowEnabled || CalibActive || TriggerMgmtActive)   ; ignore while toggled off / calibrating / managing
        return                                           ; dictation toggled off (F10) or another mode owns the key
    if (FlowState != "IDLE")
        return                                           ; swallow auto-repeat while held (and 2nd concurrent trigger)
    CoordMode "Mouse", "Screen"
    MouseGetPos &FlowX, &FlowY, &FlowWin                 ; remember where to drop the text
    FlowState := "DICTATING"
    _FlowTip("🎤  Listening…" _FLOW_HINT)
    SetTimer _FlowStartClicks, -1                        ; slow clicking off the hotkey thread
}

_FlowTriggerUp() {
    global FlowState, _FLOW_HINT
    if (FlowState != "DICTATING")
        return
    FlowState := "AWAITING_CLIP"
    _FlowTip("⏳  Transcribing…" _FLOW_HINT)
    SetTimer _FlowStopClicks, -1
    SetTimer _FlowTimeout, -15000
}

*F24::_FlowTriggerDown            ; Copilot key (remapped to F24 by PowerToys KBM) -> start
*F24 up::_FlowTriggerUp           ; Copilot key released

; Esc cancels an in-progress dictation. The #HotIf scopes this hotkey to ONLY when
; we're mid-flow (DICTATING / AWAITING_CLIP), so normal Esc is untouched otherwise.
; It resets our state machine, cancels the safety timer, and (because state is no
; longer AWAITING_CLIP) suppresses the pending paste.
#HotIf FlowState != "IDLE"
*Esc::{
    global FlowState
    SetTimer _FlowTimeout, 0
    SetTimer _FlowStartClicks, 0                          ; cancel any pending start clicks
    SetTimer _FlowStopClicks, 0
    FlowState := "IDLE"
    _FlowTipFor("✗  cancelled", 1000)
}
#HotIf

; F10 toggles the whole Copilot-key dictation flow on/off. We bind F10 (no '~') so
; it also suppresses Windows' own F10 dictation/menu action, freeing F11 for normal
; use (e.g. browser fullscreen).
F10::{
    global FlowEnabled, FlowState
    FlowEnabled := !FlowEnabled
    if (!FlowEnabled && FlowState != "IDLE") {           ; turning off mid-flow: stop a held-trigger dictation
        SetTimer _FlowTimeout, 0
        SetTimer _FlowStartClicks, 0                      ; cancel any pending start (F10 tapped during the dwell)
        if (FlowState = "DICTATING")
            SetTimer _FlowStopClicks, -1                  ; START already fired -> issue STOP so Flow isn't left recording
        FlowState := "IDLE"
    }
    _FlowToast(FlowEnabled ? "  Dictation  ON  " : "  Dictation  OFF  ", FlowEnabled)
}

; F11 toggles calibration mode. Only enters from IDLE; the Copilot key is ignored
; while active (see *F24). Rainbow ON / grey OFF toast, same as the F10 toggle.
; Exiting (F11/Esc) intentionally keeps the WORKING offsets live for the session so
; you can test them with the real Copilot key; only F4 persists them, and a script
; reload reverts to the saved ini.
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
    try {
        _FlowCalibSave(_FlowCalibPath(), m)
    } catch as e {                       ; surface a real write failure, don't fake "saved"
        _FlowTipFor("✗  save failed: " e.Message, 3000)
        return                           ; keep the ● unsaved markers
    }
    _flowCalib := m.Clone()             ; SAVED snapshot now matches WORKING (clears dirty)
    _FlowCalibShow()
    _FlowTipFor("✓  saved", 1200)
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

; ==============================================================================
;  Hot corners  (move the pointer to the top-right corner -> Task View)
; ==============================================================================
SetTimer HotCorners, 200

HotCorners()
{
    static armed := true
    CoordMode "Mouse", "Screen"
    MouseGetPos &mx, &my
    T := 3
    inTopRight := (my <= T) && (mx >= A_ScreenWidth - T)
    if (inTopRight && armed)
    {
        armed := false
        Send "#{Tab}"               ; Task View (show all open windows)
    }
    else if (!inTopRight)
    {
        armed := true               ; re-arm once the pointer leaves the corner
    }
}

; ==============================================================================
;  Capture the active window to Pictures\Screenshots (DPI-aware helper)
; ==============================================================================
CaptureActiveWindow()
{
    hwnd := WinGetID("A")
    if !hwnd
        return
    ps     := A_WinDir "\System32\WindowsPowerShell\v1.0\powershell.exe"
    helper := A_ScriptDir "\screenshot-window.ps1"
    Run('"' ps '" -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "' helper '" -Hwnd ' hwnd, , "Hide")
    SoundBeep(1000, 90)                                   ; audible shutter cue
    ToolTip("Screenshot saved to Pictures\Screenshots")
    SetTimer(() => ToolTip(), -1600)                      ; auto-hide the tip
}
