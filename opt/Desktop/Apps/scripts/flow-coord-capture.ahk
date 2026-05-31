#Requires AutoHotkey v2.0
#SingleInstance Force
; flow-coord-capture.ahk — capture the overlay's START and STOP click points.
; Records the mouse position (screen + offset within Flow's "Status" overlay
; window) to a log file, WITHOUT clicking, so we get exact coordinates.
;
; Usage:
;   1. (idle) hover the START target — dead center of the overlay — press F1.
;   2. Start dictation manually (click the overlay) so the STOP icon appears.
;   3. hover the STOP icon, press F2.
;   4. (optional) F3 = click the last-captured START, F4 = click the last STOP,
;      to verify the coordinates actually hit the right buttons.
;   Esc = quit.   Log: C:\Users\edwar\AppData\Local\Temp\flow-coords.txt

SetTitleMatchMode 3                       ; exact title match
_overlay  := "Status ahk_exe Wispr Flow.exe"
_logfile  := "C:\Users\edwar\AppData\Local\Temp\flow-coords.txt"
_startXY  := ""
_stopXY   := ""
try FileDelete _logfile                   ; fresh log each run

_capture(label) {
    global _overlay, _logfile, _startXY, _stopXY
    CoordMode "Mouse", "Screen"
    MouseGetPos &mx, &my
    line := label " : screen=" mx "," my
    if WinExist(_overlay) {
        WinGetPos &wx, &wy, &ww, &wh, _overlay
        line .= "  overlay=(" wx "," wy ") size=" ww "x" wh "  offset=" (mx - wx) "," (my - wy)
    } else {
        line .= "  (overlay 'Status' window NOT found)"
    }
    FileAppend line "`n", _logfile
    if (label = "START")
        _startXY := mx "," my
    else
        _stopXY := mx "," my
    ToolTip "Captured " label " @ " mx "," my, mx + 18, my + 18
    SetTimer () => ToolTip(), -2500
}

_clickAt(xy) {                            ; verify: hover ~1.2s THEN click (matches macos.ahk's live behavior)
    if (xy = "")
        return
    parts := StrSplit(xy, ",")
    CoordMode "Mouse", "Screen"
    MouseMove parts[1], parts[2]
    Sleep 1200
    Click parts[1] " " parts[2]
}

F1::_capture("START")
F2::_capture("STOP")
F3::_clickAt(_startXY)                     ; verify START coords
F4::_clickAt(_stopXY)                      ; verify STOP coords
Esc::ExitApp
