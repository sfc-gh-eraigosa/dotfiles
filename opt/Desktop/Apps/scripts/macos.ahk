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
;  Copilot key  ->  Claude Desktop  (then start voice typing)
; ------------------------------------------------------------------------------
;  The dedicated Copilot key sends Shift+Win+F23, but the modifiers don't always
;  register cleanly, so we fire on F23 with ANY modifiers (*) -- nothing else on
;  a keyboard emits F23. Intercepting it (no ~) also stops Windows launching
;  Copilot. We open/focus Claude for Windows -- a packaged app launched by its
;  AppID -- then fire Win+H so Windows voice typing dictates into Claude's input.
; ==============================================================================
*F23::LaunchClaude()

LaunchClaude()
{
    if !WinExist("ahk_exe Claude.exe")
    {
        Run 'explorer.exe shell:AppsFolder\Claude_pzs8sxrjxfjjc!Claude'
        if !WinWait("ahk_exe Claude.exe", , 12)   ; bail if it never appears
            return
    }
    WinActivate "ahk_exe Claude.exe"
    WinWaitActive "ahk_exe Claude.exe", , 5
    Sleep 400                       ; let the input box take focus
    Send "#h"                       ; Win+H -> Windows voice typing
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
