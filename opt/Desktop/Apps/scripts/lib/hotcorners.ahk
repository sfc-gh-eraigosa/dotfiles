#Requires AutoHotkey v2.0
; hotcorners.ahk — outer-corner-only hot-corner GEOMETRY layer (issue #93).
; Pure functions (monitors in, target/boolean out): no GUI, no hotkeys, no
; SetTimer, no auto-execute side effects, so they can be exercised headlessly by
; hotcorners.test.ahk. #Include'd by macos.ahk.
;
; Why this layer exists: the old HotCorners() used A_ScreenWidth (the PRIMARY
; monitor width) for the X test. With CoordMode Screen, a monitor to the right of
; primary has mx >= primaryWidth, so the X test was always true across the ENTIRE
; right monitor and the whole top edge fired Task View. We instead derive the true
; top-right corner from the rightmost monitor's REAL bounds.

; Enumerate every monitor's FULL bounds (NOT the work area) via live WinAPI queries.
; Recomputed every poll by the caller, so disconnect/reconnect/resolution/arrangement
; changes self-heal within one tick (MonitorGet* are live, never cached here).
_EnumMonitors() {
    mons := []
    count := MonitorGetCount()
    loop count {
        MonitorGet A_Index, &left, &top, &right, &bottom
        mons.Push({left: left, top: top, right: right, bottom: bottom})
    }
    return mons
}

; The single true outer corner: the {right, top} of the monitor with the greatest
; .right edge (the rightmost monitor). Returns "" when there are no monitors.
_HotCornerTarget(monitors) {
    if (monitors.Length = 0)
        return ""
    best := monitors[1]
    for mon in monitors {
        if (mon.right > best.right)
            best := mon
    }
    return {right: best.right, top: best.top}
}

; True only inside the T-sized box hugging the rightmost monitor's top-right corner.
; The seam between monitors and the top-center of any monitor must NOT match.
_InHotCorner(mx, my, target, T) {
    return (mx >= target.right - T) && (my >= target.top) && (my <= target.top + T)
}
