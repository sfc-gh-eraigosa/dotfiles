# Wispr Flow config: extra trigger keys + hold-F1 help

**Date:** 2026-05-31
**Feature:** `wispr-flow-config`
**Worker:** `wispr-flow-config/edward-raigosa/impl`
**Builds on:** PR #53 (Copilot-key hover-dictate driver in `opt/Desktop/Apps/scripts/macos.ahk`)

## Problem

The Wispr Flow dictation driver added in PR #53 binds exactly one trigger — the
Copilot key (remapped to `F24` by PowerToys KBM) — and its keymap (F9 manage was
absent; F10 toggle, F11 calibrate, Esc cancel) is only discoverable by reading
`WISPR-FLOW.md`. Two gaps:

1. **No way to add an alternate trigger.** When switching keyboards (e.g. one
   without a Copilot key, or one whose right Command key is more reachable) the
   user wants a *second* key to also start/stop dictation — without giving up the
   Copilot key. Today the trigger is a hardcoded `*F24::` binding.
2. **The keymap is invisible at the moment of use.** During a dictation the user
   sees `⏳ Transcribing…` with no hint that help exists.

## Goals

- **Extra trigger keys (press-to-add).** Add/remove dictation trigger keys at
  runtime via a capture mode. The Copilot key (`F24`) is the **permanent, locked
  default** — always bound, never removable. Added keys persist per-machine so
  they survive a script reload / repo re-deploy.
- **Hold-F1 help.** While dictation is enabled, holding `F1` shows a help overlay
  listing every live binding (including the current extra triggers); releasing
  `F1` dismisses it. The listening/transcribing/cancel tooltips advertise it with
  a `   F1 help` suffix.

## Non-goals

- Replacing or remapping the Copilot key (it stays the default).
- Multi-key *sequences* (chords like "g then d"). v1 supports a single key with
  optional modifiers (a key *combination*), plus the bare right-Command key.
- Cross-monitor/DPI trigger behavior — triggers are global keys, unaffected by
  the overlay-offset calibration that F11 already owns.

## Approach

Mirror the existing calibration pattern exactly (it is proven, tested, and
already understood):

- A **pure data layer** `flow-triggers.ahk` (load/save/validate — no GUI, no
  hotkeys, no side effects) with a **headless test** `flow-triggers-test.ahk`,
  paralleling `flow-calib.ahk` / `flow-calib-test.ahk`.
- The Copilot-key handler bodies in `macos.ahk` are refactored into shared
  `_FlowTriggerDown()` / `_FlowTriggerUp()` so `F24` and every dynamically-bound
  key reuse identical hold-to-talk logic.
- Two new HUDs (manage-triggers, help) reuse the rainbow toast / calibration-HUD
  styling already in `macos.ahk`.

Rejected alternatives: folding triggers into `flow-calib.ini` (overloads the
calibration module's single responsibility); session-only binding with no
persistence (added keys vanish on reload — defeats "switch keyboards and keep my
key").

## Component 1 — `flow-triggers.ahk` (data layer)

Pure functions, `#Include`d by `macos.ahk`, exercised headlessly by the test.

- `_FlowTriggersPath()` → `%LOCALAPPDATA%\dotfiles\flow-triggers.ini` (per-machine
  runtime, **not** tracked in git — same location class as `flow-calib.ini`).
- `_FlowTriggersLoad(path)` → array of AHK hotkey strings (the **added** keys
  only; `F24`/Copilot is never stored). INI shape:

  ```ini
  [triggers]
  count=2
  k1=F13
  k2=^!d
  ```

  Defensive contract (same as the calib loader — must never throw at startup):
  missing file → `[]`; non-integer or absent `count` → `[]`; blank entries
  skipped; duplicates collapsed. A corrupt file degrades to "no extra triggers",
  never takes down `macos.ahk`.
- `_FlowTriggersSave(path, arr)` → creates the parent dir if absent, dedupes, then
  writes `count` + `k1..kN`.
- `_FlowTriggerValidate(hotkeyStr)` → `{ok: bool, reason: string}` — **pure**
  reserved-key / shape policy (see Component 3), unit-tested independently of any
  GUI or live binding.

## Component 2 — handler refactor + dynamic binding (`macos.ahk`)

- Extract the current `*F24::` body → `_FlowTriggerDown()` and `*F24 up::` body →
  `_FlowTriggerUp()`. Bind `*F24::_FlowTriggerDown` and `*F24 up::_FlowTriggerUp`.
- At startup, after loading `_flowTriggers` (guarded `try/catch` → `[]`), loop the
  list and bind each: `Hotkey "*" t, _FlowTriggerDown` and
  `Hotkey "*" t " up", _FlowTriggerUp`. Each `Hotkey()` call is individually
  guarded so one bad stored entry is skipped, not fatal.
- Hold-to-talk semantics are identical for every trigger: key-down → start
  (overlay click via the existing timer-driven path), key-up → stop. The existing
  auto-repeat swallow (`if FlowState != "IDLE": return`) already handles held keys.

### Right Command (RWin) — the named goal

`macos.ahk`'s Cmd layer is built entirely on **LWin** (`~LWin`, `<#…`). **RWin is
unused**, so it is a valid trigger. Specifics:

- Bound as `*RWin::_FlowTriggerDown` / `*RWin up::_FlowTriggerUp` **without** `~`,
  so the lone-Win "open Start menu on release" is suppressed.
- While RWin is a trigger it cannot also act as a Windows modifier — acceptable,
  the layout does not use it.
- Capturing a bare modifier needs the modifier-aware capture path in Component 3.

## Component 3 — manage-triggers capture mode (F9) + reserved-key policy

A new mode toggle parallel to F11 calibration. `F9` is bound globally (no global
Windows action claims it).

- **`F9`** toggles manage mode: rainbow ON / grey OFF toast (same as F10/F11),
  shows/destroys the manage HUD, and arms/disarms the capture `InputHook`. The
  Copilot key and the F1 help are inert while managing.
- **Manage HUD** lists `🔒 Copilot key (F24)  — locked default` then each added
  trigger by friendly label. Keymap line: *"Press a key to add it · press it
  again to remove · Esc to exit"*.
- **Capture = toggle.** The armed `InputHook` captures the next physical key plus
  any held modifiers and composes an AHK hotkey string. Then:
  - if **already present** → remove it (unbind both up/down via `Hotkey …,, "Off"`,
    drop from the list, save) and tip `✓ removed  <label>`;
  - else if **valid** → bind it live, append, save, and tip `✓ added  <label>`;
  - else (**reserved/invalid**) → tip `✗ <label> — <reason>`, change nothing.
  After each action the HUD refreshes so the new state is visible immediately.
- **Esc / F9** exits manage mode (scoped `#HotIf TriggerMgmtActive`).

### Reserved-key policy (`_FlowTriggerValidate`, pure + tested)

Hard-blocked — rejected with an explanatory tip, never added:

| Key(s) | Why |
|---|---|
| `Esc` | cancel dictation / exit modes |
| `F1` | hold-for-help |
| `F2` `F3` `F4` `F5` | calibration-mode capture / revert / defaults / save |
| `F9` | manage-triggers toggle |
| `F10` | dictation on/off toggle |
| `F11` | calibration toggle |
| `F24` | the Copilot key — the permanent default, can't be re-added |
| `LWin` | the Cmd-layer base — binding it breaks every `Cmd+*` shortcut |
| bare printable / whitespace key (letter, digit, punctuation, Space, Tab, Enter, Backspace) **with no modifier** | binding `*c::` would swallow that character everywhere you type — only allowed **with** a modifier |

Allowed:

- Non-text keys outside the reserved set — e.g. `F6`–`F8`, `F12`–`F23`, media /
  browser keys.
- Any key **with ≥1 modifier** (Ctrl / Alt / Shift / Win) — e.g. `^!d`, `+F6`,
  `^#k`. (This is "key combination" support.)
- **`RWin`** as a bare push-to-talk modifier (the named goal). v1 restricts bare
  *modifier* triggers to RWin only; the capture path generalizes to other
  right-hand modifiers later if wanted. LWin is excluded (above); bare LCtrl /
  LAlt / LShift are rejected as "add a modifier-plus-key" since they're needed for
  normal shortcuts.

A re-press of an **already-added** key is *not* an error — it is the remove path.

### Friendly labels

A small formatter maps hotkey strings to readable names for the HUD/tips:
`RWin` → `Right Cmd`, `^!d` → `Ctrl+Alt+D`, `F13` → `F13`. Raw AHK strings are what
get persisted and bound; labels are presentation only.

## Component 4 — hold-F1 help overlay (`macos.ahk`)

- **Tooltip hint.** A `_FLOW_HINT := "   F1 help"` suffix is appended to the
  Listening / Transcribing / cancelled tips, e.g. `⏳ Transcribing…   F1 help`.
- **Help HUD.** A `FLOW HELP`-titled HUD (rainbow title + keymap, same builder
  style as the calibration HUD) listing every live binding, built fresh on each
  open so it reflects the **current** extra triggers:

  ```
  Copilot key    hold to dictate    🔒
  Right Cmd      hold to dictate           ← (only if added)
  Ctrl+Alt+D     hold to dictate           ← (only if added)
  Esc            cancel dictation
  F10            dictation on / off
  F11            calibrate overlay
  F9             manage trigger keys
  F1 (hold)      this help
  ```

- **Scope & lifecycle.** Bound `#HotIf FlowEnabled && !CalibActive &&
  !TriggerMgmtActive`: `*F1::` shows the HUD, `*F1 up::` destroys it. So it's live
  whenever dictation is ON (overriding app-Help F1 while on; toggling the tool off
  with F10 restores normal F1), but yields F1 to calibration mode (where F1 = set
  START) and to manage mode. The F10-off branch also destroys the help HUD if it
  happens to be showing.

## Files

| File | Change |
|---|---|
| `opt/Desktop/Apps/scripts/flow-triggers.ahk` | **new** — data layer (path/load/save/validate) |
| `opt/Desktop/Apps/scripts/flow-triggers-test.ahk` | **new** — headless test |
| `opt/Desktop/Apps/scripts/macos.ahk` | handler refactor, startup dynamic-bind, F9 manage mode + InputHook + reserved-key guard, F1 help HUD + tooltip hints, F10-off cleanup, `#Include flow-triggers.ahk` |
| `opt/Desktop/Apps/scripts/WISPR-FLOW.md` | document F9 manage-triggers, F1 help, the new `flow-triggers.ini`, RWin guidance |
| `docs/superpowers/specs/2026-05-31-wispr-flow-config-design.md` | this spec |
| `docs/superpowers/plans/2026-05-31-wispr-flow-config.md` | implementation plan (next step) |

No `.gitignore` change: `flow-triggers.ini` lives under `%LOCALAPPDATA%`, outside
the repo, exactly like `flow-calib.ini`.

## Testing

- **`flow-triggers-test.ahk`** (headless, exit 0/1 like `flow-calib-test.ahk`):
  round-trip save→load; missing file → `[]`; non-integer/absent `count` → `[]`;
  blank-entry skip; dedupe on save and load; nested-dir create on save.
- **`_FlowTriggerValidate` cases** (same harness): rejects each reserved key
  (`Esc`, `F1`–`F5`, `F9`–`F11`, `F24`, `LWin`); rejects bare `c` / `5` / `Space`;
  accepts `F13`, `^!d`, `+F6`, `RWin`; treats an already-present key as
  remove-not-error (validate returns ok; manage layer routes to removal).
- **Manual desktop pass:** F9 opens the manage HUD; pressing `F13` adds it (tip +
  HUD row), holding F13 dictates; pressing F13 again removes it; pressing the
  right Command key adds `Right Cmd` and it dictates; pressing `Esc`/`F1`/`F10`
  during capture is refused with a reason; F9/Esc exits. Holding F1 while enabled
  shows the help listing the current triggers; release dismisses; F10-off restores
  normal F1.
- **Regression:** `macos.ahk` `/validate` exits 0; Copilot key still
  starts/stops/pastes; Esc still cancels; F10/F11 unchanged.

## Risks

- **`InputHook` capturing a bare modifier (RWin).** Win is treated as a modifier,
  not an end key; the capture path must notify on the modifier VK. Mitigated by a
  modifier-aware capture; falls back to "press a non-modifier key" messaging if a
  lone modifier can't be resolved.
- **Global F1 override while enabled.** Intentional and user-approved; F10-off is
  the documented escape hatch. Scoped off during calibration/manage so it never
  shadows those modes' own F1 use.
