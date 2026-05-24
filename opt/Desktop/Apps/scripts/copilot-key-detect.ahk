#Requires AutoHotkey v2.0
#SingleInstance Force

; ============================================================================
;  Copilot-key detector. Run this, press your Copilot key 2-3 times, then
;  press the letter 'a' once. It logs every key it sees (without blocking it)
;  to copilot-key-detect.log next to this script, then closes after ~30s.
;
;  If the Copilot key shows up in the log -> AHK can see it; we read the real
;  vk/sc and fix the hotkey. If only 'a' shows up -> the key bypasses AHK and
;  we remap it through Windows Settings instead.
; ============================================================================

log := A_ScriptDir "\copilot-key-detect.log"
try FileDelete(log)
FileAppend("=== Copilot key detector ===`nStarted: " A_Now "`n"
    . "Press your Copilot key 2-3 times, then press the 'a' key once.`n"
    . "Closes automatically after ~30s (writes '=== done ===').`n`n", log)

ih := InputHook("V")            ; V = don't block keys, just observe
ih.KeyOpt("{All}", "N")         ; notify on every key
ih.OnKeyDown := LogDown
ih.Start()

LogDown(hook, vk, sc) {
    global log
    FileAppend(Format("DOWN  vk={:#04x}  sc={:#04x}  name={}`n",
        vk, sc, GetKeyName(Format("vk{:x}", vk))), log)
}

TrayTip("Copilot key detector running", "Press the Copilot key a few times, then 'a'.", 1)
SetTimer(End, -30000)
End() {
    global log
    FileAppend("`n=== done ===`n", log)
    ExitApp()
}
