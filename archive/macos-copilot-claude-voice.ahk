#Requires AutoHotkey v2.0
#SingleInstance Force

; ##############################################################################
; ARCHIVED 2026-05-29 — RETIRED, not part of the active setup.
;
; This was the Copilot-key voice-dictation macro, lifted verbatim out of
; opt/Desktop/Apps/scripts/macos.ahk when dictation moved to the Wispr Flow app.
; Kept here in case we ever want the AHK-driven Claude Desktop + Windows Voice
; Typing gesture back. To restore: paste the block below back into macos.ahk
; (it expects the macos.ahk runtime) or run this file on its own with AHK v2.
;
; Why retired: see ../opt/Desktop/Apps/scripts/WISPR-FLOW.md
; ##############################################################################

; ==============================================================================
;  Copilot key  ->  Claude Desktop voice assistant
; ------------------------------------------------------------------------------
;  The Copilot key emits LWin+LShift+F23 (vk86/sc6e) -- confirmed with
;  copilot-key-detect.ahk. AHK's hook sees F23, so *F23 (any modifiers) catches
;  AND suppresses it (no ~), stopping Windows' own Copilot-key action (which,
;  with the Copilot app uninstalled, just opens the "assign Copilot" Settings).
;
;  Claude Desktop's Windows dictation is push-to-talk (hold) with no keyboard
;  shortcut, and its Mac-style "Quick Entry" voice is macOS-only -- so we drive
;  voice with Windows Voice Typing (Win+H): a toggle that types into whatever
;  field has focus (here, Claude's composer). No mouse, no holding.
;
;  Gesture model:
;    * Idle, single tap        -> focus Claude, CONTINUE chat, start listening
;    * Idle, double tap (fast) -> focus Claude, NEW chat (Ctrl+N), start listening
;    * While listening, a tap  -> stop dictation, submit (Enter), confirm
;
;  Audio cues:  1 rising beep  = listening (continue)   2 beeps = listening (new)
;               two-tone chirp = submitted ("working")  low buzz = error/timeout
;
;  (The official Settings/registry remap can't target Claude on a personal MS
;  account -- its picker only lists registered "Copilot key provider" apps, and
;  the enterprise CopilotKey policy is ignored. Hence this AHK interception.)
; ==============================================================================
ClaudeAUMID       := "Claude_pzs8sxrjxfjjc!Claude"
ClaudeListening   := false      ; are we mid-dictation?
ClaudeBusy        := false      ; mid-transition guard (ignore taps)
ClaudeTapCount    := 0          ; taps seen inside the double-tap window
ClaudeDoubleGap   := 320        ; ms window that counts as a double tap
ClaudeCommitDelay := 800        ; ms to let final dictation commit before Enter
ClaudeListenLimit := 90000      ; ms before an unfinished session auto-resets

*F23::ClaudeKey()

ClaudeKey()
{
    global ClaudeListening, ClaudeBusy, ClaudeTapCount, ClaudeDoubleGap
    if ClaudeBusy                       ; ignore taps during a transition
        return
    if ClaudeListening                  ; second tap -> stop + submit
    {
        ClaudeSubmit()
        return
    }
    ClaudeTapCount += 1                 ; idle -> count taps, decide on timeout
    if (ClaudeTapCount = 1)
        SetTimer ClaudeResolveTaps, -ClaudeDoubleGap
}

ClaudeResolveTaps()
{
    global ClaudeTapCount
    newChat := (ClaudeTapCount >= 2)
    ClaudeTapCount := 0
    ClaudeStartListening(newChat)
}

; --- open/focus Claude, optional new chat, then start Windows Voice Typing -------
ClaudeStartListening(newChat)
{
    global ClaudeAUMID, ClaudeListening, ClaudeBusy, ClaudeListenLimit
    ClaudeBusy := true
    try
    {
        if !WinExist("ahk_exe Claude.exe")
        {
            Run 'explorer.exe shell:AppsFolder\' ClaudeAUMID
            if !WinWait("ahk_exe Claude.exe", , 12)
            {
                ClaudeFail("Couldn't reach Claude Desktop")
                return
            }
        }
        WinActivate "ahk_exe Claude.exe"
        if !WinWaitActive("ahk_exe Claude.exe", , 5)
        {
            ClaudeFail("Couldn't focus Claude Desktop")
            return
        }
        if newChat
        {
            Send "^n"                   ; Ctrl+N = new chat in Claude Desktop
            Sleep 350
        }
        Sleep 250                       ; let the composer take focus
        Send "#h"                       ; Win+H -> start Windows Voice Typing
        ClaudeListening := true
        if newChat
        {
            SoundBeep 660, 110
            SoundBeep 990, 130
            ClaudeTip("New chat - listening...  (tap again to send)")
        }
        else
        {
            SoundBeep 880, 130
            ClaudeTip("Listening...  (tap again to send)")
        }
        SetTimer ClaudeListenTimeout, -ClaudeListenLimit
    }
    finally
        ClaudeBusy := false
}

; --- stop dictation, submit the prompt, confirm receipt -------------------------
ClaudeSubmit()
{
    global ClaudeListening, ClaudeBusy, ClaudeCommitDelay
    ClaudeBusy := true
    SetTimer ClaudeListenTimeout, 0     ; cancel the auto-reset safety net
    try
    {
        Send "#h"                       ; Win+H -> stop voice typing
        Sleep ClaudeCommitDelay         ; let the final dictation land in the box
        if WinExist("ahk_exe Claude.exe")
            WinActivate "ahk_exe Claude.exe"
        Send "{Enter}"                  ; submit the prompt
        ClaudeListening := false
        SoundBeep 1175, 90              ; "sent / working" two-tone chirp
        SoundBeep 1568, 110
        ClaudeTip("Sent - Claude is working...")
    }
    finally
        ClaudeBusy := false
}

ClaudeListenTimeout()
{
    global ClaudeListening
    if !ClaudeListening
        return
    Send "#h"                           ; close the voice-typing bar
    ClaudeListening := false
    SoundBeep 440, 220
    ClaudeTip("Listening timed out - tap to start again")
}

ClaudeFail(msg)
{
    global ClaudeListening
    ClaudeListening := false
    SoundBeep 300, 250
    ClaudeTip(msg)
}

ClaudeTip(text)
{
    ToolTip text
    SetTimer () => ToolTip(), -2200
}
