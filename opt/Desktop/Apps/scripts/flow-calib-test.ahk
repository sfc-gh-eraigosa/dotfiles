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

; 3. Partial file (missing key) falls back per-key to default
IniDelete tmp, "overlay", "stopY"
m3 := _FlowCalibLoad(tmp)
_assert(m3["startX"] = 111, "partial file keeps startX=111")
_assert(m3["stopY"] = d["stopY"], "partial file falls back stopY to default")

; 4. Non-numeric (corrupt/hand-edited) value falls back to default and does NOT throw
IniWrite "garbage", tmp, "overlay", "startX"
m4 := _FlowCalibLoad(tmp)
_assert(m4["startX"] = d["startX"], "non-numeric startX falls back to default")
_assert(m4["stopX"] = 333, "corrupt startX does not disturb other keys")

try FileDelete tmp
FileAppend "OK: all calibration config tests passed`n", RESULT
ExitApp 0
