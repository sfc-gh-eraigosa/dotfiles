#Requires AutoHotkey v2.0
#SingleInstance Off
#NoTrayIcon
#Include lib\hotcorners.ahk
; Headless test for the hot-corner geometry layer (issue #93). Results ->
; %TEMP%\hotcorners_test_out.txt. First line "RESULT GREEN" (no fails) or
; "RESULT RED", then "TOTAL n  FAILS m", then one line per case.
; Exit code: 0 = all pass, 1 = a failure, 2 = an uncaught error.
;
; All asserted coordinates are REACHABLE on-screen points: the OS clamps the
; cursor, so a point off every monitor is meaningless and is never asserted.

OUT := A_Temp "\hotcorners_test_out.txt"

; Surface any uncaught error as a distinct RED result so the runner never hangs.
OnError(_OnTestError)
_OnTestError(err, mode) {
    global OUT
    FileAppend "RESULT ERROR " err.Message "`n", OUT
    ExitApp(2)
}

lines := ""           ; accumulated PASS/FAIL lines
total := 0
fails := 0

Assert(cond, name) {
    global lines, total, fails
    total += 1
    if (cond) {
        lines .= "PASS " name "`n"
    } else {
        lines .= "FAIL " name "`n"
        fails += 1
    }
}

T := 3

; --- A) Dual: primary left + secondary right ----------------------------------
dualA := [{left: 0, top: 0, right: 1920, bottom: 1080}, {left: 1920, top: 0, right: 4480, bottom: 1440}]
tA := _HotCornerTarget(dualA)
Assert(tA.right = 4480, "A: target right=4480")
Assert(tA.top = 0, "A: target top=0")
Assert(!!_InHotCorner(4479, 1, tA, T), "A: true outer corner fires")
Assert(!_InHotCorner(3000, 1, tA, T), "A: top-center of right monitor does NOT fire (#93 bug)")
Assert(!_InHotCorner(1919, 1, tA, T), "A: seam (top-right of left monitor) does NOT fire")

; --- B) Single monitor --------------------------------------------------------
single := [{left: 0, top: 0, right: 1920, bottom: 1080}]
tB := _HotCornerTarget(single)
Assert(tB.right = 1920, "B: target right=1920")
Assert(tB.top = 0, "B: target top=0")
Assert(!!_InHotCorner(1919, 1, tB, T), "B: outer corner fires")
Assert(!_InHotCorner(960, 1, tB, T), "B: top-center does NOT fire")

; --- C) Monitor extended to the LEFT (negative coords) ------------------------
leftExt := [{left: -2560, top: 0, right: 0, bottom: 1440}, {left: 0, top: 0, right: 1920, bottom: 1080}]
tC := _HotCornerTarget(leftExt)
Assert(tC.right = 1920, "C: target right=1920 (rightmost wins over negative-coord mon)")
Assert(tC.top = 0, "C: target top=0")
Assert(!!_InHotCorner(1919, 1, tC, T), "C: outer corner fires")
Assert(!_InHotCorner(-100, 1, tC, T), "C: point on left monitor does NOT fire")

; --- D) Vertically-offset right monitor (corner at real top, not y=0) ---------
offset := [{left: 0, top: 0, right: 1920, bottom: 1080}, {left: 1920, top: 300, right: 4480, bottom: 1740}]
tD := _HotCornerTarget(offset)
Assert(tD.right = 4480, "D: target right=4480")
Assert(tD.top = 300, "D: target top=300 (real monitor top)")
Assert(!!_InHotCorner(4479, 301, tD, T), "D: corner at monitor's real top fires")
Assert(!_InHotCorner(4479, 1, tD, T), "D: y=1 (above real top) does NOT fire")

; --- E) Hotplug: corner relocates on geometry change --------------------------
targetA := _HotCornerTarget(dualA)
targetB := _HotCornerTarget([{left: 0, top: 0, right: 1920, bottom: 1080}])
Assert(targetA.right = 4480, "E: dual target right=4480")
Assert(targetB.right = 1920, "E: single target right=1920 (corner RELOCATES)")
Assert(!!_InHotCorner(1919, 1, targetB, T), "E: under single target, outer corner fires")
Assert(!_InHotCorner(960, 1, targetB, T), "E: under single target, top-center does NOT fire")

; --- Emit the report ----------------------------------------------------------
try FileDelete OUT
header := (fails = 0 ? "RESULT GREEN" : "RESULT RED") "`n"
header .= "TOTAL " total "  FAILS " fails "`n"
FileAppend header . lines, OUT
ExitApp(fails = 0 ? 0 : 1)
