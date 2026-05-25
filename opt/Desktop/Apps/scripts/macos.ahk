#Requires AutoHotkey v2.0
#SingleInstance Force

; ==============================================================================
;  macOS-style shortcuts for Windows  (AutoHotkey v2)
; ------------------------------------------------------------------------------
;  Left Windows key (LWin)  ->  acts like the macOS Command (Cmd) key
;  Left Alt           (!)   ->  acts like the macOS Option (Opt) key
;
;  This is the single canonical script. Old v1 files live in _archive_v1\.
; ==============================================================================

cmdTabActive := false

; --- Make a lone Cmd (LWin) tap do nothing, like macOS --------------------------
; Tilde keeps LWin working as a modifier; the unassigned vkE8 keystroke absorbs
; the press so Windows never shows the Start menu when Cmd is tapped on its own.
~LWin::Send "{Blind}{vkE8}"

~LWin Up::
{
    global cmdTabActive
    if cmdTabActive
    {
        Send "{Blind}{Alt up}"      ; release Alt -> commit the Cmd+Tab choice
        cmdTabActive := false
    }
}

; --- Core editing ---------------------------------------------------------------
<#c::Send "^c"                  ; Cmd+C        Copy
<#v::Send "^v"                  ; Cmd+V        Paste
<#+v::Send "^+v"                ; Cmd+Shift+V  Paste and match style
<#x::Send "^x"                  ; Cmd+X        Cut
<#a::Send "^a"                  ; Cmd+A        Select all
<#z::Send "^z"                  ; Cmd+Z        Undo
<#+z::Send "^y"                 ; Cmd+Shift+Z  Redo
<#s::Send "^s"                  ; Cmd+S        Save
<#+s::Send "^+s"                ; Cmd+Shift+S  Save as
<#f::Send "^f"                  ; Cmd+F        Find
<#g::Send "{F3}"                ; Cmd+G        Find next
<#+g::Send "+{F3}"              ; Cmd+Shift+G  Find previous
<#o::Send "^o"                  ; Cmd+O        Open
<#n::Send "^n"                  ; Cmd+N        New
<#p::Send "^p"                  ; Cmd+P        Print
<#/::Send "^/"                  ; Cmd+/        Toggle line comment (editors)

; --- Window & app management ----------------------------------------------------
<#w::Send "^w"                  ; Cmd+W        Close tab / window
<#m::WinMinimize "A"            ; Cmd+M        Minimize active window
<#h::WinMinimize "A"            ; Cmd+H        Hide (minimize) active window
<#!Esc::Send "^+{Esc}"          ; Cmd+Opt+Esc  Force quit (Task Manager)

<#q::                           ; Cmd+Q        Quit active app
{
    if InStr(WinGetTitle("A"), "World of Warcraft")
        return                  ; never Alt+F4 the game
    Send "!{F4}"
}

; --- Cmd+Tab app switcher (hold Cmd, tap Tab to cycle, release to choose) --------
<#Tab::
{
    global cmdTabActive
    cmdTabActive := true
    Send "{Blind}{Alt down}{Tab}"
}
<#+Tab::
{
    global cmdTabActive
    cmdTabActive := true
    Send "{Blind}{Alt down}{Shift down}{Tab}{Shift up}"
}

; --- Navigation & system --------------------------------------------------------
<#Space::Send "#s"              ; Cmd+Space    Spotlight (Windows Search)
<#l::Send "^l"                  ; Cmd+L        Focus address bar
<#r::Send "{F5}"                ; Cmd+R        Reload / refresh

; --- Line navigation (Cmd + arrows) ---------------------------------------------
<#Left::Send "{Home}"           ; Cmd+Left     Start of line
<#Right::Send "{End}"           ; Cmd+Right    End of line
<#Up::Send "^{Home}"            ; Cmd+Up       Top of document
<#Down::Send "^{End}"           ; Cmd+Down     Bottom of document

; --- Line selection (Cmd + Shift + arrows) --------------------------------------
<#+Left::Send "+{Home}"         ; select to start of line
<#+Right::Send "+{End}"         ; select to end of line
<#+Up::Send "+^{Home}"          ; select to top of document
<#+Down::Send "+^{End}"         ; select to bottom of document

; --- Delete behaviour -----------------------------------------------------------
<#Backspace::Send "+{Home}{Del}"   ; Cmd+Delete   Delete to start of line
!Backspace::Send "^{Backspace}"    ; Opt+Delete   Delete previous word

; --- Word navigation (Option / Alt + arrows) ------------------------------------
!Left::Send "^{Left}"           ; Opt+Left          Previous word
!Right::Send "^{Right}"         ; Opt+Right         Next word
!+Left::Send "+^{Left}"         ; Opt+Shift+Left    Select previous word
!+Right::Send "+^{Right}"       ; Opt+Shift+Right   Select next word

; --- Browser tabs ---------------------------------------------------------------
<#t::Send "^t"                  ; Cmd+T        New tab
<#+t::Send "^+t"                ; Cmd+Shift+T  Reopen closed tab
<#[::Send "!{Left}"             ; Cmd+[        Back
<#]::Send "!{Right}"            ; Cmd+]        Forward
<#1::Send "^1"                  ; Cmd+1..8     Jump to tab N
<#2::Send "^2"
<#3::Send "^3"
<#4::Send "^4"
<#5::Send "^5"
<#6::Send "^6"
<#7::Send "^7"
<#8::Send "^8"
<#9::Send "^9"                  ; Cmd+9        Jump to last tab

; --- Screenshots ----------------------------------------------------------------
<#+3::Send "#{PrintScreen}"     ; Cmd+Shift+3  Full screen -> Pictures\Screenshots
<#+4::Send "#+s"                ; Cmd+Shift+4  Region capture (Snip & Sketch)
^+c::CaptureActiveWindow()      ; Ctrl+Shift+C Capture active window -> Pictures\Screenshots

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

; ==============================================================================
;  Hot corners  (move the pointer to the top-right corner -> Task View)
; ==============================================================================
SetTimer HotCorners, 200

HotCorners()
{
    static armed := true
    CoordMode "Mouse", "Screen"
    MouseGetPos &mx, &my
    T := 3
    inTopRight := (my <= T) && (mx >= A_ScreenWidth - T)
    if (inTopRight && armed)
    {
        armed := false
        Send "#{Tab}"               ; Task View (show all open windows)
    }
    else if (!inTopRight)
    {
        armed := true               ; re-arm once the pointer leaves the corner
    }
}

; ==============================================================================
;  Capture the active window to Pictures\Screenshots (DPI-aware helper)
; ==============================================================================
CaptureActiveWindow()
{
    hwnd := WinGetID("A")
    if !hwnd
        return
    ps     := A_WinDir "\System32\WindowsPowerShell\v1.0\powershell.exe"
    helper := A_ScriptDir "\screenshot-window.ps1"
    Run('"' ps '" -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "' helper '" -Hwnd ' hwnd, , "Hide")
    SoundBeep(1000, 90)                                   ; audible shutter cue
    ToolTip("Screenshot saved to Pictures\Screenshots")
    SetTimer(() => ToolTip(), -1600)                      ; auto-hide the tip
}
