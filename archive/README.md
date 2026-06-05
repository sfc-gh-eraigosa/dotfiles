# archive/

Retired-but-kept artifacts. Nothing here is wired into the install workflow —
it is parked so we can restore it later without digging through git history.

| File | Retired | Replaced by | Restore |
|------|---------|-------------|---------|
| `macos-copilot-claude-voice.ahk` | 2026-05-29 | [Wispr Flow](../opt/Desktop/Apps/scripts/WISPR-FLOW.md) | see below |

## macos-copilot-claude-voice.ahk

The original AutoHotkey v2 "Copilot key → voice dictation" macro, lifted verbatim
out of `opt/Desktop/Apps/scripts/macos.ahk`. It remapped the Copilot key
(`LWin+LShift+F23`) into a tap-to-dictate gesture: one tap focused Claude Desktop
and started Windows Voice Typing (`Win+H`); a second tap stopped dictation and
pressed Enter; a fast double-tap opened a new chat first.

**Why it was retired:** dictation moved to the [Wispr Flow](https://wisprflow.ai)
app, which is the engine now and types transcribed text into whatever field has
focus. The Copilot key is left unbound in `macos.ahk` so Flow can capture it
directly. Full rationale, setup, caveats, and the fallback shim live in
[`opt/Desktop/Apps/scripts/WISPR-FLOW.md`](../opt/Desktop/Apps/scripts/WISPR-FLOW.md).

**To bring it back:** paste the body of this file (everything below the archive
header) back into `opt/Desktop/Apps/scripts/macos.ahk`, then re-deploy
(`install.sh` re-runs the Windows desktop deploy) and restart AutoHotkey. You
would also want to unbind the Copilot key inside Wispr Flow so the two don't both
fire. The file is self-contained (`#Requires AutoHotkey v2.0`) and can also be
run on its own for testing.
