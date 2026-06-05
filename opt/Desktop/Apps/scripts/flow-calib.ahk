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

; Read each key, falling back to its default when the key is missing OR holds a
; non-numeric value, so a missing or corrupt/hand-edited file degrades gracefully
; to defaults per key and never throws (Integer() would throw on garbage like
; "startX=abc", which at startup would take down all of macos.ahk).
_FlowCalibLoad(path) {
    d := _FlowCalibDefaults()
    m := Map()
    for key, dflt in d {
        raw := IniRead(path, "overlay", key, dflt)
        m[key] := IsInteger(raw) ? Integer(raw) : dflt
    }
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
