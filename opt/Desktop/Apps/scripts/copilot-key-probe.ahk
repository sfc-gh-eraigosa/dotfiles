#Requires AutoHotkey v2.0
#SingleInstance Force
; copilot-key-probe.ahk — one-off diagnostic for the hover-dictate spike (R1).
; Logs every F23 event so we can see whether the Copilot key AUTO-REPEATS while
; held (R1 = holds) or fires once (R1 = momentary).
;
; IMPORTANT: stop the running macos.ahk first (it also binds *F23 and would both
; pollute this probe and trigger Flow). Then run this, watch the tooltip while you
; press and HOLD the Copilot key for ~2 seconds, then release. Press Esc to exit.
_n := 0
*F23::{                  ; Copilot key down (fires repeatedly if the key auto-repeats)
    global _n
    _n += 1
    ToolTip "F23 DOWN #" _n "   (t=" A_TickCount ")"
}
*F23 up::{               ; Copilot key released
    global _n
    ToolTip "F23 UP after " _n " DOWN event(s)  ->  " (_n > 1 ? "HOLDS (auto-repeat)" : "looks MOMENTARY")
    SetTimer () => ToolTip(), -3000
    _n := 0
}
Esc::ExitApp
