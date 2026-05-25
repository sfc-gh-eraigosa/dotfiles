#Requires AutoHotkey v2.0
#SingleInstance Force

; ============================================================================
;  Copilot-key detector. Run this, press your Copilot key 3 times, then press
;  the letter 'a' once. It logs every key it sees (without blocking it) to
;  copilot-key-detect.log next to this script, then closes after ~30s.
;
;  If the Copilot key shows up in the log -> AHK can see it; we read the real
;  vk/sc and bind it. If only 'a' shows up -> the key bypasses AHK and can't
;  be redirected.
; ============================================================================

logFile := A_ScriptDir "\copilot-key-detect.log"
try FileDelete(logFile)
FileAppend("=== Copilot key detector ===`nStarted: " A_Now "`n"
    . "Press your Copilot key 3 times, then press 'a' once.`n"
    . "Closes automatically after ~30s (writes '=== done ===').`n`n", logFile)

ih := InputHook("V")            ; V = don't block keys, just observe
ih.KeyOpt("{All}", "N")         ; notify on every key
ih.OnKeyDown := LogDown
ih.Start()

LogDown(hook, vk, sc) {
    global logFile
    FileAppend(Format("DOWN  vk={:#04x}  sc={:#04x}  name={}`n",
        vk, sc, GetKeyName(Format("vk{:x}", vk))), logFile)
}

TrayTip("Copilot key detector running", "Press the Copilot key 3 times, then 'a'.")
SetTimer(Finish, -30000)
Finish() {
    global logFile
    FileAppend("`n=== done ===`n", logFile)
    ExitApp()
}
