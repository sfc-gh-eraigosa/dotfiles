#Requires AutoHotkey v2.0
; flow-triggers.ahk — Wispr Flow extra-trigger-key DATA + POLICY layer.
; Pure functions only: no GUI, no hotkeys, no auto-exec side effects, so they can
; be exercised headlessly by flow-triggers-test.ahk and #Include'd by macos.ahk.
;
; Mirrors flow-calib.ahk's defensive contract exactly: never throw at load; a
; missing or corrupt/hand-edited file degrades to "no extra triggers" ([]), never
; takes down macos.ahk at startup.
;
; ---------------------------------------------------------------------------------
;  Canonical key form (the single string every gate / compare / dedupe runs on)
; ---------------------------------------------------------------------------------
;  - Every key is resolved to its NAMED form (LWin, RWin, F24, Left, ...), never a
;    raw vkNN / scNNN, so a leaked vk5B can't bypass a name-based LWin reserve check.
;  - Modifiers are re-emitted in a FIXED order ^ ! + #  (Ctrl Alt Shift Win) so
;    ^!d and !^d collapse to one canonical entry.
;  - Handedness: NON-Win modifiers fold (<^ -> ^). The WIN modifier's handedness is
;    PRESERVED (<#c stays <#c — the LWin Cmd layer; #c / >#c are DISTINCT chords)
;    so a user adding an RWin #c trigger is not falsely rejected as colliding with
;    the Cmd-layer <#c. A *bare* vk5B -> base key LWin; a *bare* vk5C -> base RWin.
;
;  VK/SC -> named fold table (the forms the validator must catch):
;    vk5B -> LWin   vk5C -> RWin   vk87 -> F24   vk1B -> Esc
;    vk25 -> Left   vk27 -> Right  vk26 -> Up     vk28 -> Down
;    vk70..vk7B -> F1..F12  vk7C..vk87 -> F13..F24

; --- Per-machine runtime store (NOT tracked in git; same class as flow-calib.ini).
_FlowTriggersPath() {
    return EnvGet("LOCALAPPDATA") . "\dotfiles\flow-triggers.ini"
}

; ---------------------------------------------------------------------------------
;  Normalization helpers
; ---------------------------------------------------------------------------------

; Map a raw base token (possibly vkNN / scNNN, any case) to its canonical NAMED
; form. Unknown tokens pass through with a stable case (single letters lowercased).
_FlowNormBase(base) {
    if (base == "")
        return ""
    low := StrLower(base)
    ; vk / sc fold table -> named keys the policy must recognise.
    static vkmap := Map(
        "vk5b", "LWin", "vk5c", "RWin", "vk87", "F24", "vk1b", "Esc",
        "vk25", "Left", "vk27", "Right", "vk26", "Up", "vk28", "Down",
        "vk2e", "Delete", "vk2d", "Insert", "vk24", "Home", "vk23", "End",
        "vk21", "PgUp", "vk22", "PgDn", "vk09", "Tab", "vk0d", "Enter",
        "vk20", "Space", "vk08", "Backspace", "vk14", "CapsLock",
        "vk5d", "Apps", "vk2c", "PrintScreen",
        "vk70", "F1", "vk71", "F2", "vk72", "F3", "vk73", "F4",
        "vk74", "F5", "vk75", "F6", "vk76", "F7", "vk77", "F8",
        "vk78", "F9", "vk79", "F10", "vk7a", "F11", "vk7b", "F12",
        "vk7c", "F13", "vk7d", "F14", "vk7e", "F15", "vk7f", "F16",
        "vk80", "F17", "vk81", "F18", "vk82", "F19", "vk83", "F20",
        "vk84", "F21", "vk85", "F22", "vk86", "F23",
        "vka2", "LCtrl", "vka3", "RCtrl", "vka4", "LAlt", "vka5", "RAlt",
        "vka0", "LShift", "vka1", "RShift")
    if (vkmap.Has(low))
        return vkmap[low]
    ; Function keys & multi-char named keys: keep as-is (canonical case below).
    static named := Map(
        "lwin", "LWin", "rwin", "RWin", "esc", "Esc", "escape", "Esc",
        "left", "Left", "right", "Right", "up", "Up", "down", "Down",
        "home", "Home", "end", "End", "pgup", "PgUp", "pgdn", "PgDn",
        "delete", "Delete", "del", "Delete", "insert", "Insert", "ins", "Insert",
        "tab", "Tab", "enter", "Enter", "return", "Enter", "space", "Space",
        "backspace", "Backspace", "bs", "Backspace", "capslock", "CapsLock",
        "numlock", "NumLock", "scrolllock", "ScrollLock", "apps", "Apps",
        "appskey", "Apps", "printscreen", "PrintScreen", "lctrl", "LCtrl",
        "rctrl", "RCtrl", "lalt", "LAlt", "ralt", "RAlt", "lshift", "LShift",
        "rshift", "RShift")
    if (named.Has(low))
        return named[low]
    ; F-keys (f1..f24) -> canonical "F" + number.
    if (RegExMatch(low, "^f(\d{1,2})$", &mm)) {
        n := Integer(mm[1])
        if (n >= 1 && n <= 24)
            return "F" . n
    }
    ; Media / browser / launch keys -> canonical Pascal-ish (keep author form).
    static special := Map(
        "volume_up", "Volume_Up", "volume_down", "Volume_Down",
        "volume_mute", "Volume_Mute", "media_next", "Media_Next",
        "media_prev", "Media_Prev", "media_play_pause", "Media_Play_Pause",
        "media_stop", "Media_Stop", "browser_back", "Browser_Back",
        "browser_forward", "Browser_Forward", "browser_refresh", "Browser_Refresh",
        "browser_stop", "Browser_Stop", "browser_search", "Browser_Search",
        "browser_favorites", "Browser_Favorites", "browser_home", "Browser_Home",
        "launch_mail", "Launch_Mail", "launch_media", "Launch_Media",
        "launch_app1", "Launch_App1", "launch_app2", "Launch_App2")
    if (special.Has(low))
        return special[low]
    ; Single printable letter -> lowercase for a stable compare.
    if (StrLen(base) == 1)
        return low
    ; Unknown: return the original token unchanged.
    return base
}

; Parse a raw chord ("<#c", "!^d", "+F6", "vk87", "RWin", ...) into its modifier
; symbols and base token. Returns Map("ctrl", bool, "alt", bool, "shift", bool,
; "winSym", "" | "#" | "<#" | ">#", "base", canonical-base).
_FlowParseChord(chord) {
    s := chord
    ctrl := false, alt := false, shift := false
    winSym := ""          ; "", "#", "<#", ">#"
    handPending := ""     ; "<" or ">" seen, awaiting the modifier it qualifies
    loop {
        if (s == "")
            break
        ch := SubStr(s, 1, 1)
        if (ch == "<") {
            handPending := "<"
            s := SubStr(s, 2)
            continue
        }
        if (ch == ">") {
            handPending := ">"
            s := SubStr(s, 2)
            continue
        }
        if (ch == "^") {
            ctrl := true, handPending := "", s := SubStr(s, 2)
            continue
        }
        if (ch == "!") {
            alt := true, handPending := "", s := SubStr(s, 2)
            continue
        }
        if (ch == "+") {
            shift := true, handPending := "", s := SubStr(s, 2)
            continue
        }
        if (ch == "#") {
            ; Win modifier — PRESERVE handedness prefix if one was pending.
            winSym := (handPending == "<") ? "<#" : (handPending == ">") ? ">#" : "#"
            handPending := ""
            s := SubStr(s, 2)
            continue
        }
        ; Not a modifier symbol -> the rest is the base token.
        break
    }
    base := _FlowNormBase(s)
    ; A bare Win symbol with no base resolves to a base key (LWin/RWin) for the
    ; hard-reserve LWin check and the RWin trigger — distinct from the <#/>#
    ; modifier-prefix case (which always carries a base).
    return Map("ctrl", ctrl, "alt", alt, "shift", shift, "winSym", winSym, "base", base)
}

; Canonical form: fixed modifier order ^ ! + #, Win handedness preserved, non-Win
; handedness folded, named base. Never throws.
_FlowTriggerNormalize(chord) {
    if (!IsSet(chord) || chord == "")
        return ""
    p := _FlowParseChord(chord)
    base := p["base"]
    winSym := p["winSym"]
    ; Bare Win modifier (no base, no other mod) -> resolve to its base key.
    if (base == "" && winSym != "" && !p["ctrl"] && !p["alt"] && !p["shift"]) {
        return (winSym == ">#" || winSym == "#") ? "RWin" : "LWin"
    }
    out := ""
    if (p["ctrl"])
        out .= "^"
    if (p["alt"])
        out .= "!"
    if (p["shift"])
        out .= "+"
    out .= winSym          ; "", "#", "<#", or ">#"
    out .= base
    return out
}

; Pure capture-composition (factored OUT of the InputHook so it is headless-tested).
; heldMods: array of modifier NAMES (Ctrl/Alt/Shift/LWin/RWin/LCtrl/...). baseKey:
; the resolved non-modifier key, or "" for a bare-modifier resolution.
;   compose(["Ctrl","Alt"],"d")  -> "^!d"   (order-independent in)
;   compose(["Alt","Ctrl"],"d")  -> "^!d"
;   compose(["RWin"],"")         -> "RWin"  (the one whitelisted bare modifier)
;   compose(["LWin"],"")         -> "LWin"
;   compose(["LCtrl"],"")        -> ""      (sentinel: reject a lone non-RWin modifier)
_FlowComposeChord(heldMods, baseKey) {
    ctrl := false, alt := false, shift := false, win := false
    for m in heldMods {
        ml := StrLower(m)
        if (ml == "ctrl" || ml == "lctrl" || ml == "rctrl" || ml == "control")
            ctrl := true
        else if (ml == "alt" || ml == "lalt" || ml == "ralt")
            alt := true
        else if (ml == "shift" || ml == "lshift" || ml == "rshift")
            shift := true
        else if (ml == "win" || ml == "lwin" || ml == "rwin")
            win := true
    }
    if (!IsSet(baseKey) || baseKey == "") {
        ; Bare-modifier resolution. Only RWin and LWin resolve to a base key;
        ; every other lone modifier yields the empty sentinel ("add a key").
        for m in heldMods {
            ml := StrLower(m)
            if (ml == "rwin")
                return "RWin"
            if (ml == "lwin")
                return "LWin"
        }
        return ""          ; lone non-Win modifier -> sentinel
    }
    ; Non-modifier base present: build canonical "<mods><base>".
    mods := ""
    if (ctrl)
        mods .= "^"
    if (alt)
        mods .= "!"
    if (shift)
        mods .= "+"
    if (win)
        mods .= "#"
    return _FlowTriggerNormalize(mods . baseKey)
}

; ---------------------------------------------------------------------------------
;  Persistence (count-authoritative / blank-skip / dedupe-on-normalized / no-throw)
; ---------------------------------------------------------------------------------

; Dedupe an array on the normalized form, preserving first-seen order. Returns the
; surviving RAW entries (already-normalized callers pass normalized in).
_FlowDedupeNormalized(arr) {
    out := []
    seen := Map()
    for v in arr {
        n := _FlowTriggerNormalize(v)
        if (n == "")
            continue
        if (seen.Has(n))
            continue
        seen[n] := true
        out.Push(n)
    }
    return out
}

; Load the ADDED trigger chords (F24/Copilot is never stored). count is
; AUTHORITATIVE: read exactly k1..k{count}, empty-string default, SKIP blank /
; whitespace entries (never push a placeholder), normalize + dedupe. Missing file /
; non-integer / absent count -> []. Never throws (IsInteger guard is the contract).
_FlowTriggersLoad(path) {
    count := IniRead(path, "triggers", "count", "")
    if (!IsInteger(count))
        return []
    raw := []
    n := Integer(count)
    i := 1
    while (i <= n) {
        t := Trim(IniRead(path, "triggers", "k" . i, ""))
        if (t != "")
            raw.Push(t)
        i += 1
    }
    return _FlowDedupeNormalized(raw)
}

; Persist: create the parent dir if absent (fresh machine), dedupe on the
; normalized form, write count + k1..kN. Orphaned higher-index keys are left inert
; (the new count ignores them) — load gates on count, not on disk contents.
_FlowTriggersSave(path, arr) {
    SplitPath path, , &dir
    if (dir && !DirExist(dir))
        DirCreate dir
    clean := _FlowDedupeNormalized(arr)
    IniWrite clean.Length, path, "triggers", "count"
    i := 1
    for v in clean {
        IniWrite v, path, "triggers", "k" . i
        i += 1
    }
}

; ---------------------------------------------------------------------------------
;  Friendly label (presentation only — raw chords are what get persisted/bound)
; ---------------------------------------------------------------------------------
_FlowTriggerLabel(chord) {
    c := _FlowTriggerNormalize(chord)
    if (c == "F24")
        return "Copilot key"
    if (c == "RWin")
        return "Right Cmd"
    if (c == "LWin")
        return "Left Cmd"
    p := _FlowParseChord(c)
    parts := []
    if (p["ctrl"])
        parts.Push("Ctrl")
    if (p["alt"])
        parts.Push("Alt")
    if (p["shift"])
        parts.Push("Shift")
    if (p["winSym"] == "<#")
        parts.Push("Left Cmd")
    else if (p["winSym"] == ">#")
        parts.Push("Right Cmd")
    else if (p["winSym"] == "#")
        parts.Push("Cmd")
    base := p["base"]
    if (base != "") {
        ; Single letter -> uppercase for display (d -> D).
        disp := (StrLen(base) == 1) ? StrUpper(base) : base
        parts.Push(disp)
    }
    label := ""
    for idx, part in parts {
        label .= (idx == 1) ? part : ("+" . part)
    }
    return label
}

; ---------------------------------------------------------------------------------
;  Reserved set (Gate 1) and OS-shortcut denylist (Gate 2b) — canonical forms
; ---------------------------------------------------------------------------------

; Hard-reserved control keys the driver owns. Compared on the NORMALIZED chord, so
; vk87 (F24) and vk5B (LWin) also reject (normalize-first invariant).
_FlowReservedSet() {
    ; Built once (static) — the reserved set is constant, so avoid per-call churn
    ; when the validator runs in a tight loop. Still side-effect free at load.
    static m := Map(
        "Esc", true, "F1", true, "F2", true, "F3", true, "F4", true,
        "F5", true, "F9", true, "F10", true, "F11", true,
        "F23", true, "F24", true, "LWin", true)
    return m
}

; Curated OS / editor combos that Gate 3 cannot see (the Cmd layer *Sends* ^c etc.
; but does not *bind* them), so they are blocked here. Stored normalized for O(1).
_FlowOsShortcutDenylist() {
    ; Built once (static): the denylist is constant, so we normalize the ~21 raw
    ; entries a single time instead of on every validate() call. Side-effect free.
    static m := ""
    if (m != "")
        return m
    raw := ["^c", "^v", "^x", "^z", "^y", "^a", "^s", "^f", "^p", "^n", "^o",
            "^w", "^t", "!Tab", "!F4", "#Tab", "#d", "#l", "#e", "#r", "^+Esc"]
    m := Map()
    for r in raw
        m[_FlowTriggerNormalize(r)] := true
    return m
}

; ---------------------------------------------------------------------------------
;  Gate 2 shape helpers
; ---------------------------------------------------------------------------------

; Is the canonical BASE an allowed bare/modified function key (F6-F8, F12-F22)?
_FlowIsAllowedFKey(base) {
    if (!RegExMatch(base, "^F(\d{1,2})$", &mm))
        return false
    n := Integer(mm[1])
    if (n == 6 || n == 7 || n == 8)
        return true
    if (n >= 12 && n <= 22)
        return true
    return false
}

; Is the canonical BASE a media / browser / launch key (allowed bare or modified)?
_FlowIsMediaKey(base) {
    return RegExMatch(base, "i)^(Volume_|Media_|Browser_|Launch_)")
}

; ---------------------------------------------------------------------------------
;  _FlowTriggerValidate — three gates on the CANONICAL chord. {ok, reason}.
;  Already-present-key -> remove is the CALLER's job (validator gates additions only).
; ---------------------------------------------------------------------------------
_FlowTriggerValidate(chord, boundChords) {
    c := _FlowTriggerNormalize(chord)          ; normalize FIRST (Gate-1 invariant)
    if (c == "")
        return Map("ok", false, "reason", "add a key, not a lone modifier")

    ; --- Gate 1: reserved control keys ---------------------------------------
    if (_FlowReservedSet().Has(c))
        return Map("ok", false, "reason", "reserved (driver key)")

    p := _FlowParseChord(c)
    base := p["base"]
    hasMod := (p["ctrl"] || p["alt"] || p["shift"] || p["winSym"] != "")

    ; --- Gate 2: shape allowlist (default-deny) ------------------------------
    allowed := false
    if (_FlowIsAllowedFKey(base))               ; (a) F6-F8 / F12-F22, bare or mod
        allowed := true
    else if (_FlowIsMediaKey(base))             ; (b) media / browser, bare or mod
        allowed := true
    else if (hasMod)                            ; (c) any key with >= 1 modifier
        allowed := true
    else if (c == "RWin")                       ; (d) the one whitelisted bare mod
        allowed := true
    if (!allowed)
        return Map("ok", false,
            "reason", "add an F6-F22 / media key, a modifier+key combo, or Right Cmd")

    ; --- Gate 2b: OS-shortcut denylist (applies to shape-valid modified chords)
    if (_FlowOsShortcutDenylist().Has(c))
        return Map("ok", false, "reason", "that's a common OS/editor shortcut")

    ; --- Gate 3: live-keymap collision ---------------------------------------
    bound := Map()
    for b in boundChords
        bound[_FlowTriggerNormalize(b)] := true
    if (bound.Has(c))
        return Map("ok", false, "reason", "already bound by macos.ahk")

    return Map("ok", true, "reason", "")
}

; ---------------------------------------------------------------------------------
;  _FlowTriggerManifest — the GLOBALLY-LIVE chords macos.ahk binds today (Gate 3
;  ground truth). Stored CANONICAL, with the Cmd layer in its HANDED form (<#...) so
;  the Phase-2 handedness rule keeps #c / >#c addable.
;
;  Mirrors these macos.ahk layers (parity-guarded by flow-triggers-test.ahk):
;    - Cmd layer  <#...        (macos.ahk Core editing / Window / Nav / Browser)
;    - Opt layer  !Left/!Right/!+Left/!+Right/!Backspace  (word-nav)
;    - ^+c        Ctrl+Shift+C screenshot (CaptureActiveWindow)
;    - ~LWin      the Cmd-layer base passthrough
;  EXCLUDED on purpose:
;    - Calibration-scoped F1-F5/F10/Esc (live only under #HotIf CalibActive) — they
;      are reusable triggers outside that mode (Gate 1 already hard-reserves the
;      user-facing driver keys, so omitting their scoped duplicates is safe).
;    - The global driver keys F10/F11 — already hard-reserved by Gate 1.
;  TODO: auto-generate this manifest at deploy time from macos.ahk's live hotkeys.
; ---------------------------------------------------------------------------------
_FlowTriggerManifest() {
    raw := [
        ; --- Cmd layer (LWin / <#) — stored HANDED so #c / >#c stay addable ---
        "<#c", "<#v", "<#+v", "<#x", "<#a", "<#z", "<#+z", "<#s", "<#+s",
        "<#f", "<#g", "<#+g", "<#o", "<#n", "<#p", "<#/",
        "<#w", "<#m", "<#h", "<#!Esc", "<#q",
        "<#Tab", "<#+Tab",
        "<#Space", "<#l", "<#r",
        "<#Left", "<#Right", "<#Up", "<#Down",
        "<#+Left", "<#+Right", "<#+Up", "<#+Down",
        "<#Backspace",
        "<#t", "<#+t", "<#[", "<#]",
        "<#1", "<#2", "<#3", "<#4", "<#5", "<#6", "<#7", "<#8", "<#9",
        "<#+3", "<#+4",
        ; --- Opt (!) word-nav layer ---
        "!Backspace", "!Left", "!Right", "!+Left", "!+Right",
        ; --- screenshot + Cmd-layer base passthrough ---
        "^+c", "~LWin"]
    out := []
    for r in raw {
        ; ~LWin has a passthrough tilde the normalizer ignores; strip it so the
        ; manifest is canonical. (Normalize folds <#... handedness preservation.)
        n := _FlowTriggerNormalize(StrReplace(r, "~"))
        if (n != "")
            out.Push(n)
    }
    return out
}
