#Requires AutoHotkey v2.0
#Include flow-triggers.ahk
; Headless test for the Wispr Flow trigger DATA + POLICY layer.
; Results -> %TEMP%\flow-triggers-test.out   Exit 0 = all pass, 1 = a failure.
; Run with cwd = this dir so the relative #Include resolves:
;   AutoHotkey.exe flow-triggers-test.ahk ; echo EXIT=%ERRORLEVEL% ; type "%TEMP%\flow-triggers-test.out"

RESULT := A_Temp "\flow-triggers-test.out"
try FileDelete RESULT

_assert(cond, msg) {
    global RESULT
    if (!cond) {
        FileAppend "FAIL: " msg "`n", RESULT
        ExitApp 1
    }
}

; Helper: does an array (of canonical chords) contain a value?
_has(arr, val) {
    for v in arr
        if (v == val)
            return true
    return false
}

tmp := A_Temp "\flow-triggers-test-" A_TickCount ".ini"
try FileDelete tmp

; =================================================================================
;  Phase 1 — path / load / save (count-authoritative, blank-skip, inert-orphan)
; =================================================================================

; 1. Missing file -> []
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 0, "missing file -> empty list")

; 2. Round-trip
_FlowTriggersSave(tmp, ["F13", "F14"])
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 2, "round-trip length 2")
_assert(m[1] == "F13", "round-trip [1]=F13")
_assert(m[2] == "F14", "round-trip [2]=F14")

; 3. Count overstatement (count=3 but only k1,k2 set) -> shorter list, no placeholder
try FileDelete tmp
IniWrite 3, tmp, "triggers", "count"
IniWrite "F13", tmp, "triggers", "k1"
IniWrite "F14", tmp, "triggers", "k2"
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 2, "count overstatement -> length 2 not 3")
_assert(m[1] == "F13" && m[2] == "F14", "overstatement keeps real entries")

; 3b. count=3 with only k1 -> [k1]
try FileDelete tmp
IniWrite 3, tmp, "triggers", "count"
IniWrite "F13", tmp, "triggers", "k1"
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 1 && m[1] == "F13", "count=3 only k1 -> [F13]")

; 4. Absent count -> []
try FileDelete tmp
IniWrite "F13", tmp, "triggers", "k1"
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 0, "absent count -> []")
; 4b. Non-integer count -> []
try FileDelete tmp
IniWrite "abc", tmp, "triggers", "count"
IniWrite "F13", tmp, "triggers", "k1"
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 0, "non-integer count=abc -> []")

; 5. Blank / whitespace entries skipped (count=3, k2 blank) -> length 2
try FileDelete tmp
IniWrite 3, tmp, "triggers", "count"
IniWrite "F13", tmp, "triggers", "k1"
IniWrite "   ", tmp, "triggers", "k2"
IniWrite "F14", tmp, "triggers", "k3"
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 2, "blank entry skipped -> length 2")
_assert(m[1] == "F13" && m[2] == "F14", "blank-skip keeps F13,F14")

; 6. Inert-orphan no-delete: save 3, save 1 over it, reload -> [F13] length 1.
;    Pin BOTH halves of the contract: (a) load returns the short list, AND (b) the
;    orphaned k2/k3 are left physically on disk (count gates the read; save does NOT
;    scrub). Reading the raw orphan key locks out a future scrub-on-save regression
;    that would otherwise still pass the load-length assertion below.
try FileDelete tmp
_FlowTriggersSave(tmp, ["F13", "F14", "F15"])
_FlowTriggersSave(tmp, ["F13"])
_assert(IniRead(tmp, "triggers", "k2", "") != "",
    "inert-orphan: stale k2 left on disk (count gates the read, save does not scrub)")
_assert(IniRead(tmp, "triggers", "k3", "") != "",
    "inert-orphan: stale k3 left on disk (count gates the read, save does not scrub)")
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 1 && m[1] == "F13", "inert-orphan: load returns short list [F13]")

; 7. Nested-dir create on save
subdir := A_Temp "\ft-dir-" A_TickCount
nested := subdir "\sub\flow-triggers.ini"
_FlowTriggersSave(nested, ["F13"])
_assert(FileExist(nested), "save created nested ini (DirCreate branch)")
mN := _FlowTriggersLoad(nested)
_assert(mN.Length == 1 && mN[1] == "F13", "nested ini reloads [F13]")
try DirDelete subdir, true

; =================================================================================
;  Phase 2 — normalization + composition + dedupe
; =================================================================================

; 8. Fixed modifier order ^ ! + #
_assert(_FlowTriggerNormalize("!^d") == "^!d", "fixed order !^d -> ^!d")
_assert(_FlowTriggerNormalize("+^F6") == "^+F6", "fixed order +^F6 -> ^+F6")

; 9. VK/SC -> named
_assert(_FlowTriggerNormalize("vk87") == "F24", "vk87 -> F24")
_assert(_FlowTriggerNormalize("vk5B") == "LWin", "vk5B -> LWin")
_assert(_FlowTriggerNormalize("vk5C") == "RWin", "vk5C -> RWin")

; 10. Handedness rule: Win preserved, non-Win folded; #c distinct from <#c
_assert(_FlowTriggerNormalize("<#c") == "<#c", "<#c preserved (LWin Cmd)")
_assert(_FlowTriggerNormalize("#c") == "#c", "#c stays #c")
_assert(_FlowTriggerNormalize("#c") != _FlowTriggerNormalize("<#c"), "#c distinct from <#c")
_assert(_FlowTriggerNormalize("<^d") == "^d", "non-Win handedness folded <^d -> ^d")

; 11. Compose (the de-risked capture step)
_assert(_FlowComposeChord(["Ctrl", "Alt"], "d") == "^!d", "compose Ctrl,Alt,d -> ^!d")
_assert(_FlowComposeChord(["Alt", "Ctrl"], "d") == "^!d", "compose order-independent -> ^!d")
_assert(_FlowComposeChord(["RWin"], "") == "RWin", "compose bare RWin -> RWin")
_assert(_FlowComposeChord(["LWin"], "") == "LWin", "compose bare LWin -> LWin")
_assert(_FlowComposeChord(["LCtrl"], "") == "", "compose lone non-RWin modifier -> sentinel ''")

; 12. Load dedupe/normalize: count=2, k1=!^d, k2=^!d -> ["^!d"] length 1
try FileDelete tmp
IniWrite 2, tmp, "triggers", "count"
IniWrite "!^d", tmp, "triggers", "k1"
IniWrite "^!d", tmp, "triggers", "k2"
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 1, "load dedupe mixed-order -> length 1")
_assert(m[1] == "^!d", "load dedupe canonical -> ^!d")

; 13. Save dedupe: ["^!d","!^d","F13"] -> reload ["^!d","F13"] length 2
try FileDelete tmp
_FlowTriggersSave(tmp, ["^!d", "!^d", "F13"])
m := _FlowTriggersLoad(tmp)
_assert(m.Length == 2, "save dedupe -> length 2")
_assert(m[1] == "^!d" && m[2] == "F13", "save dedupe -> [^!d, F13]")

; =================================================================================
;  Phase 3 — Gate 1 (reserved, normalize-first)
; =================================================================================

; 14. Reserved rejects (canonical forms)
for k in ["Esc", "F1", "F2", "F3", "F4", "F5", "F9", "F10", "F11", "F23", "F24", "LWin"]
    _assert(_FlowTriggerValidate(k, []).Has("ok") && !_FlowTriggerValidate(k, [])["ok"],
        "reserved reject " k)

; 15. Normalize-before-validate invariant: vk forms of F24 / LWin reject too
_assert(!_FlowTriggerValidate("vk87", [])["ok"], "vk87 (F24) reserved via normalize")
_assert(!_FlowTriggerValidate("vk5B", [])["ok"], "vk5B (LWin) reserved via normalize")

; 16. A non-reserved chord passes for now (tightened by later gates)
_assert(_FlowTriggerValidate("F13", [])["ok"], "F13 ok (non-reserved)")

; =================================================================================
;  Phase 4 — Gate 2 (shape allowlist) + Gate 2b (OS-shortcut denylist)
; =================================================================================

; 17. Shape rejects (bare nav/edit/toggle/printable/whitespace/other bare modifiers)
for k in ["c", "5", "Space", "Enter", "Tab", "Backspace", "Delete", "Left", "Right",
          "Home", "End", "PgUp", "Insert", "CapsLock", "Apps", "PrintScreen",
          "LCtrl", "RAlt"]
    _assert(!_FlowTriggerValidate(k, [])["ok"], "shape reject bare " k)

; 18. Shape accepts: F6, F12, F13, ^!d, +F6, RWin
for k in ["F6", "F12", "F13", "^!d", "+F6", "RWin"]
    _assert(_FlowTriggerValidate(k, [])["ok"], "shape accept " k)

; 19. Gate 2b denylist: ^c, ^v, !Tab, #d rejected (shape-valid, not in boundChords)
for k in ["^c", "^v", "!Tab", "#d"]
    _assert(!_FlowTriggerValidate(k, [])["ok"], "Gate 2b reject " k)

; =================================================================================
;  Phase 5 — Gate 3 (live collision) + handedness non-collision + scoped exclusion
; =================================================================================

bc := ["<#c", "!Left", "!Right", "!+Left", "!+Right", "!Backspace", "^+c"]

; 20. Collision rejects
_assert(!_FlowTriggerValidate("!Left", bc)["ok"], "Gate 3 reject !Left (in boundChords)")
_assert(!_FlowTriggerValidate("^+c", bc)["ok"], "Gate 3 reject ^+c (in boundChords)")

; 21. F13 not in set -> accept
_assert(_FlowTriggerValidate("F13", bc)["ok"], "Gate 3 accept F13 (absent)")

; 22. Handedness non-collision: #c NOT rejected by <#c in bc
_assert(_FlowTriggerValidate("#c", bc)["ok"], "Gate 3: #c not blocked by <#c")

; 23. Scoped-exclusion proof: a globally-free F-key not over-rejected
_assert(_FlowTriggerValidate("F6", bc)["ok"], "Gate 3: F6 not over-rejected (calib-scoped excluded)")

; =================================================================================
;  Phase 6 — _FlowTriggerManifest + staleness parity guard
; =================================================================================

man := _FlowTriggerManifest()

; 24. Manifest contains normalized !Left and ^+c; validator uses it
_assert(_has(man, "!Left"), "manifest contains !Left")
_assert(_has(man, "^+c"), "manifest contains ^+c")
_assert(!_FlowTriggerValidate("!Left", man)["ok"], "validate(!Left, manifest) reject")
_assert(_FlowTriggerValidate("F13", man)["ok"], "validate(F13, manifest) accept")

; 25. Manifest never contains a calib-scoped F-key (F6/F7/F8 stay addable)
_assert(!_has(man, "F6"), "manifest excludes F6 (calib-scoped)")
_assert(!_has(man, "F7"), "manifest excludes F7 (calib-scoped)")
_assert(!_has(man, "F8"), "manifest excludes F8 (calib-scoped)")

; 26. Handedness in manifest: contains <#c (handed), #c still addable
_assert(_has(man, "<#c"), "manifest contains handed <#c")
_assert(_FlowTriggerValidate("#c", man)["ok"], "manifest's <#c does not block #c")

; 27. Staleness parity: manifest (normalized) == the expected inline dump.
;     Drift in macos.ahk's global hotkeys fails this, forcing a deliberate update.
;     Order-independent set+length comparison (collation-safe).
expected := [
    "!+Left", "!+Right", "!<#Esc", "!Backspace", "!Left", "!Right",
    "+<#3", "+<#4", "+<#Down", "+<#Left", "+<#Right", "+<#Tab", "+<#Up",
    "+<#g", "+<#s", "+<#t", "+<#v", "+<#z",
    "<#/", "<#1", "<#2", "<#3", "<#4", "<#5", "<#6", "<#7", "<#8", "<#9",
    "<#Backspace", "<#Down", "<#Left", "<#Right", "<#Space", "<#Tab", "<#Up",
    "<#[", "<#]", "<#a", "<#c", "<#f", "<#g", "<#h", "<#l", "<#m", "<#n",
    "<#o", "<#p", "<#q", "<#r", "<#s", "<#t", "<#v", "<#w", "<#x", "<#z",
    "LWin", "^+c"]
_assert(man.Length == expected.Length,
    "manifest parity: length " man.Length " == expected " expected.Length)
manSet := Map()
for v in man
    manSet[v] := true
for e in expected
    _assert(manSet.Has(e), "manifest parity: missing expected chord " e)
expSet := Map()
for e in expected
    expSet[e] := true
for v in man
    _assert(expSet.Has(v), "manifest parity: unexpected chord drifted in: " v)

; 28. Label spot-checks (presentation layer)
_assert(_FlowTriggerLabel("RWin") == "Right Cmd", "label RWin -> Right Cmd")
_assert(_FlowTriggerLabel("^!d") == "Ctrl+Alt+D", "label ^!d -> Ctrl+Alt+D")
_assert(_FlowTriggerLabel("F13") == "F13", "label F13 -> F13")
_assert(_FlowTriggerLabel("F24") == "Copilot key", "label F24 -> Copilot key")

try FileDelete tmp
FileAppend "OK: all trigger config tests passed`n", RESULT
ExitApp 0
