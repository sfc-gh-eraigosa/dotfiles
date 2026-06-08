# Wispr Flow "Hover-Dictate" for the Copilot key — Design

**Date:** 2026-05-30
**Status:** Approved (pending spike verification of two unknowns)
**Area:** `opt/Desktop/Apps/scripts/macos.ahk` (Windows / AutoHotkey v2)

## Problem

The Copilot key (`LWin+LShift+F23`) is translated to Flow's hotkey by a one-line AHK
shim:

```ahk
*F23::Send "{LWin up}{LShift up}^+{F12}"
```

`Send` issues a **momentary tap** of `Ctrl+Shift+F12`. Flow's push-to-talk expects the
hotkey to be **held** for the duration of speech, so the key "doesn't stay depressed"
and Flow never records a full utterance. Separately, the user wants dictation to land
in **whatever window/field the mouse is hovering**, with visible processing feedback,
without manually clicking the field first.

## Goals

1. **Hold-to-talk works:** holding the Copilot key records in Flow for as long as it's
   held; releasing stops and transcribes.
2. **Hover targeting:** on press, focus the window under the mouse and place the caret
   there, so the transcript lands where the mouse was.
3. **Completion-driven paste + feedback:** show lightweight "listening/transcribing"
   feedback; when Flow finishes, place the text at the saved target.

## Non-goals (YAGNI)

- No swapping of the system cursor to an hourglass (a stuck cursor on script death is
  worse than the benefit; use a tooltip instead).
- No attempt to parse Flow's UI or transcript text; we only react to OS-level signals
  (key up/down, clipboard change, optional window close).
- No new standalone app — this lives in the existing `macos.ahk`.

## Approach (Model B — Flow → clipboard, AHK pastes)

Chosen because it gives full control over **where** the text lands (the user's core
ask), independent of focus drift during cloud processing.

### End-to-end flow

1. **Copilot key DOWN** (`*F23::`)
   - `MouseGetPos` → save target window (`ahk_id`) + screen x/y.
   - `WinActivate` the window under the mouse; `Click` at the saved point to set the caret.
   - Press-and-hold the Flow combo: `Send "{LWin up}{LShift up}{Ctrl down}{Shift down}{F12 down}"`.
   - Show tooltip near cursor: `🎤 Listening…`.
   - Set state `DICTATING`. Ignore auto-repeat of `*F23::` while in this state.
2. **Copilot key UP** (`*F23 up::`)
   - Release combo: `Send "{F12 up}{Shift up}{Ctrl up}"`.
   - Tooltip → `⏳ Transcribing…`. Set state `AWAITING_CLIP`. Start a ~15s safety timer.
3. **Clipboard changes** (`OnClipboardChange`)
   - Only act if state is `AWAITING_CLIP` (so unrelated copies are ignored).
   - `WinActivate` the saved window; `Click` the saved point (restore caret); `Send "^v"`.
   - Clear tooltip, reset state, cancel the safety timer.
   - Safety timer fallback: if no clipboard change arrives, clear tooltip + state and
     leave the clipboard untouched.

### State machine

`IDLE → (key down) DICTATING → (key up) AWAITING_CLIP → (clipboard change | timeout) IDLE`

A single script-global state variable plus the saved-target variables (`win_id`, `mx`,
`my`). Auto-repeat of the down-hotkey is guarded by checking the state is not already
`DICTATING`.

## Risks — verified by a short spike BEFORE final implementation

The plan's first step is a spike on the user's Windows machine; its results select the
real path. Both fallbacks keep a working feature.

- **R1 — Does the Copilot key physically *hold*?** Some Copilot keys are momentary
  (single HID event, no auto-repeat / no clean release), in which case no AHK can make
  it "hold". Verify by logging `*F23` down/up + `A_TimeSincePriorHotkey` while holding.
  - **If it holds:** hold-to-talk as designed.
  - **If momentary:** fall back to **toggle** — first tap starts (combo tap + save
    target + tooltip), second tap stops (combo tap → AWAITING_CLIP). Same automation,
    different gesture. (Flow must be set to a toggle/hands-free shortcut in that case.)
- **R2 — Can Flow copy-to-clipboard *without also typing*?** Check Flow → Settings for a
  "copy to clipboard" / "don't auto-insert" option.
  - **If yes:** Model B (clipboard trigger + AHK paste).
  - **If no (always types):** fall back to **Model A** — rely on Flow typing into the
    field we focused at press; drop the `OnClipboardChange` paste step; keep the
    tooltip feedback (cleared on a timeout).

## Components (all in `macos.ahk`)

| Unit | Responsibility |
|------|----------------|
| `*F23::` (down) | Save target, focus+caret, begin hold (or toggle-start), tooltip, set state |
| `*F23 up::` | End hold (or no-op in toggle mode), set AWAITING_CLIP, start safety timer |
| `OnClipboardChange` handler | If AWAITING_CLIP: restore target focus/caret, paste, reset |
| safety timer | Clear feedback/state if no transcript arrives within ~15s |
| tooltip helper | Show/clear a near-cursor `ToolTip` (no system-cursor swap) |

## Error handling

- **Mis-click on focus:** click-to-caret is best-effort; hovering a non-text control may
  click it. Accepted per user. (Mitigation option, not in v1: only `WinActivate` without
  click when the saved point isn't an editable control — deferred.)
- **Lost target window:** if the saved `ahk_id` no longer exists at paste time, skip the
  activate/click and paste into the current focus (best-effort), then reset.
- **Stuck modifiers:** the down/up handlers always pair their key events; the safety
  timer guarantees state resets even if a transcript never arrives.

## Testing

- **Spike (manual, on Windows):** confirm R1 (hold) and R2 (clipboard) before building.
- **Behavioral (manual):** hold over a text field → speak → release → transcript lands
  at the caret; hover a different window and repeat; release with no speech → no stray
  paste; unrelated `Ctrl+C` during IDLE → ignored.
- **Static:** `AutoHotkey64.exe /validate macos.ahk` must pass; the rest of the macOS
  shortcuts remain unaffected (only the `*F23` block changes).

## Rollback

Revert the `*F23` block to the single-line `Send` shim. No other part of `macos.ahk`
is touched.
