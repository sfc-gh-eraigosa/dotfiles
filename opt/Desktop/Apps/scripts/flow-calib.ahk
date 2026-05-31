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
