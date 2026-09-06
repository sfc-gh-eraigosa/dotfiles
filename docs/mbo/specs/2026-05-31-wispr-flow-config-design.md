# Wispr Flow config: extra trigger keys + hold-F1 help

**Date:** 2026-05-31
**Feature:** `wispr-flow-config`
**Worker:** `wispr-flow-config/edward-raigosa/impl`
**Builds on:** PR #53 (Copilot-key hover-dictate driver in `opt/Desktop/Apps/scripts/macos.ahk`)
**Revision:** rev 2 — hardened after the multi-agent design review on PR #65 (precise
InputHook capture algorithm, allowlist bare-key policy, live-keymap collision check,
mode-guard + mutual-exclusion, canonical VK→name normalization, F10-off semantics).

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
  only, already in canonical form — see "Canonical key form" in Component 3;
  `F24`/Copilot is never stored). INI shape:

  ```ini
  [triggers]
  count=2
  k1=F13
  k2=^!d
  ```

  `count` is **authoritative**: load reads exactly `k1..k{count}` (IniRead has no
  section-enumerate primitive), each with an empty-string default, **skipping** any
  blank/whitespace entry rather than defaulting it — the opposite of the calib
  loader (which *wants* a per-key default). So a `count` that overstates the present
  keys (e.g. `count=3` with only `k1` set) silently yields the shorter list, never a
  placeholder. Save overwrites `k1..kN` and does not delete orphaned higher-index
  keys (they are inert because the new `count` ignores them).

  Defensive contract (must never throw at startup): missing file → `[]`;
  non-integer or absent `count` → `[]`; blank entries skipped; duplicates collapsed.
  A corrupt file degrades to "no extra triggers", never takes down `macos.ahk`.
- `_FlowTriggersSave(path, arr)` → creates the parent dir if absent, dedupes on the
  **normalized** form, then writes `count` + `k1..kN`.
- `_FlowTriggerValidate(hotkeyStr, boundChords)` → `{ok: bool, reason: string}` —
  **pure** reserved-key / shape / live-collision policy (see Component 3), unit-tested
  independently of any GUI or live binding. `boundChords` is the set of chords already
  bound by `macos.ahk` (passed in so the function stays pure and testable).

## Component 2 — handler refactor + dynamic binding (`macos.ahk`)

- Extract the current `*F24::` body → `_FlowTriggerDown()` and `*F24 up::` body →
  `_FlowTriggerUp()`. Bind `*F24::_FlowTriggerDown` and `*F24 up::_FlowTriggerUp`.
- **Entry guard (must be carried into the extracted handler).** `_FlowTriggerDown`
  must guard `if (!FlowEnabled || CalibActive || TriggerMgmtActive) return` — note the
  added `TriggerMgmtActive` term vs today's `*F24::` (which only checks
  `!FlowEnabled || CalibActive`). Without it a literal extraction would still fire
  triggers during F9 manage mode, contradicting "the Copilot key is inert while
  managing." A bare extraction is therefore **not** sufficient — this guard is part of
  the refactor.
- **Dynamic-bind context.** The startup and add/remove `Hotkey()` calls run under the
  **global** `#HotIf` context (no active `#HotIf`), so the on-bind and off-unbind
  calls target the same variant. After loading `_flowTriggers` (guarded `try/catch`
  → `[]`), loop and bind each: `Hotkey "*" t, _FlowTriggerDown` and
  `Hotkey "*" t " up", _FlowTriggerUp`. Each `Hotkey()` is individually guarded so one
  bad stored entry is skipped, not fatal. The entry guard inside the handler — not the
  binding context — is what makes triggers inert in the wrong mode.
- **Mode mutual-exclusion.** F9 (manage) enters only from `FlowState = "IDLE" &&
  !CalibActive`; F11 (calibrate) enters only from `FlowState = "IDLE" &&
  !TriggerMgmtActive`. This prevents two HUDs + an armed InputHook coexisting with
  calibration's F1–F5 capture.
- Hold-to-talk semantics are identical for every trigger: key-down → start
  (overlay click via the existing timer-driven path), key-up → stop. The existing
  auto-repeat swallow (`if FlowState != "IDLE": return`) already handles held keys.
- **Held-key + F10-off.** `_FlowTriggerUp` returns early unless
  `FlowState = "DICTATING"`, and F10-off sets `FlowState := "IDLE"` — so holding a
  trigger across an F10 tap would otherwise send no STOP click and leave Flow
  recording. The F10-off branch must therefore issue the overlay STOP (or cancel the
  in-flight dictation) **before** going IDLE when `FlowState != "IDLE"`. (This latent
  gap already exists in PR #53 for the single F24 trigger; rev 2 closes it rather than
  inheriting it.)

### Right Command (RWin) — the named goal

`macos.ahk`'s Cmd layer is built entirely on **LWin** (`~LWin`, `<#…`). **RWin is
unused**, so it is a valid trigger. Specifics:

- Bound as `*RWin::_FlowTriggerDown` / `*RWin up::_FlowTriggerUp` **without** `~`,
  so the lone-Win "open Start menu on release" is suppressed.
- While RWin is a trigger it cannot also act as a Windows modifier — acceptable,
  the layout does not use it.
- **F10-off does NOT restore RWin's OS role.** Toggling dictation off makes the
  handler early-return, but AHK has already swallowed the key — so a captured RWin
  stays "dead" (no Start menu, no Win-modifier) for as long as `macos.ahk` runs. This
  is a deliberate, documented decision: F10 gates whether dictation *fires*, not
  whether trigger keys are *captured*. To return RWin (or any added key) to its OS
  role, **remove it via F9** (or quit `macos.ahk`). The runbook calls this out so the
  dead-key behavior is never surprising. (Non-modifier triggers like `F13`/`^!d` are
  swallowed too, but harmlessly.)
- Capturing a bare modifier needs the modifier-aware capture path in Component 3
  (it resolves on key-**up**).

## Component 3 — manage-triggers capture mode (F9) + reserved-key policy

A new mode toggle parallel to F11 calibration. `F9` is bound globally (no global
Windows action claims it).

- **`F9`** toggles manage mode: rainbow ON / grey OFF toast (same as F10/F11),
  shows/destroys the manage HUD, and arms/disarms the capture `InputHook`. The
  Copilot key and the F1 help are inert while managing.
- **Manage HUD** lists `🔒 Copilot key (F24)  — locked default` then each added
  trigger by friendly label. Keymap line: *"Press a key to add it · press it
  again to remove · Esc to exit"*.
- **Capture = toggle.** A captured key composes to a canonical chord string; then:
  - if **already present** → remove it (drop from the list, save, unbind — see below)
    and tip `✓ removed  <label>`;
  - else if **valid** → bind it live, append, save, and tip `✓ added  <label>`;
  - else (**reserved/invalid**) → tip `✗ <label> — <reason>`, change nothing.
  After each action the HUD refreshes so the new state is visible immediately.
- **Esc / F9** exits manage mode (scoped `#HotIf TriggerMgmtActive`).

#### Capture algorithm (InputHook) — the precise mechanism

A bare modifier (RWin) cannot be read as an InputHook *EndKey* — AHK v2 treats
Win/Ctrl/Alt/Shift as modifiers, never surfaces a lone one via `OnChar`/`Input`, and
KeyOpt end-keys are non-modifier only. The only in-hook signal is `OnKeyDown`/
`OnKeyUp`, which fire for every key. So a lone modifier-down is indistinguishable from
the first modifier of an intended combo **until release**. The capture therefore
resolves on **key-up**:

1. On manage-mode entry, arm one suppressing `InputHook`:
   `ih.VisibleText := false`, `ih.VisibleNonText := false` (so the captured key/combo
   is **consumed**, not typed into the focused app — critical: the HUD is `NoActivate`,
   so without this, pressing `F13`/`^!d` to add it would also fire it into the editor).
2. Register `OnKeyDown` for the right-modifier VKs we allow bare (RWin = `vk5C`) and a
   broad `OnKeyDown`/end-key for everything else. Track the set of modifiers currently
   held.
3. Resolution:
   - If a **non-modifier** key is seen → compose `<held-modifiers><that key>` and
     resolve immediately (e.g. Ctrl+Alt+D → `^!d`; bare F13 → `F13`).
   - If a **modifier is released** with **no** non-modifier seen during its press →
     resolve it as a **bare modifier** (only RWin is whitelisted; any other bare
     modifier → reject with "add a key, not a lone modifier", except RWin).
4. The composed string is passed through **canonical normalization** (below) before
   validate/bind/compare.

Acceptance criteria for the algorithm: *"RWin alone yields exactly `RWin`; Ctrl+Alt+D
yields `^!d` (never a bare modifier); F13 alone yields `F13`; capturing any key in
manage mode does NOT type or trigger it in the focused app."*

#### Canonical key form

Capture, validate, bind, dedup, and presence-compare **all** operate on one canonical
string so a VK/SC form can never bypass a name-based reserved check (a leaked `LWin`
would re-bind `*LWin::` and break the whole `~LWin` Cmd layer). Normalization rules:

- Resolve every key to its **named** form (`LWin`, `RWin`, `F24`, `Left`, …) — never a
  raw `vk5B`/`sc…`.
- Emit modifiers in a **fixed order** (`^ ! + #`) so `^!d` and `!^d` can't both exist.
- Apply normalization at capture time, *before* validate/bind, and again on load (so a
  hand-edited ini mixing orders is de-duplicated to the canonical form).

#### Unbind shape (remove path)

A trigger is two hotkeys, so removal is **two** `Hotkey()` calls —
`Hotkey "*" chord, , "Off"` and `Hotkey "*" chord " up", , "Off"` — issued under the
same **global** `#HotIf` context the add used (so On/Off variants match). Turning the
hotkey Off releases AHK's hook on that key, so a removed RWin returns to opening the
Start menu automatically.

### Reserved-key policy (`_FlowTriggerValidate`, pure + tested)

The policy is an **allowlist** (default-deny), checked on the canonical chord. A
chord is added only if it passes **all three** gates; otherwise it's rejected with the
reason shown.

**Gate 1 — not a reserved control key.** Hard-blocked, the driver owns these:

| Key(s) | Why |
|---|---|
| `Esc` | cancel dictation / exit modes |
| `F1` | hold-for-help |
| `F2` `F3` `F4` `F5` | calibration-mode capture / revert / defaults / save |
| `F9` | manage-triggers toggle |
| `F10` | dictation on/off toggle |
| `F11` | calibration toggle |
| `F23` | the raw Copilot scancode (`Win+Shift+F23` pre-KBM-remap); a `*F23` trigger would collide with the native Copilot emission if KBM is ever removed |
| `F24` | the Copilot key — the permanent default, can't be re-added |
| `LWin` | the Cmd-layer base — binding it re-binds `*LWin::` and breaks `~LWin` + every `<#…` shortcut |

**Gate 2 — shape allowlist (this is the typing-safety rule).** A chord is admitted
only if it is one of:

- a **function key** in the non-reserved range — `F6`, `F7`, `F8`, `F12`–`F22`
  (F23/F24 reserved above), **bare or modified**;
- a **media / browser** key (Volume\*, Media\*, Browser\*, Launch\*), bare or modified;
- **any** key **with ≥1 modifier** (`^ ! + #`) — e.g. `^!d`, `+F6` (this is "key
  combination" support);
- **`RWin`** alone — the one whitelisted bare modifier (the named goal).

Everything else **bare** is rejected — not just printable/whitespace keys but also
navigation/editing keys (`Left` `Right` `Up` `Down` `Home` `End` `PgUp` `PgDn`
`Delete` `Insert`), `Tab` `Enter` `Space` `Backspace`, and toggles (`CapsLock`
`NumLock` `ScrollLock` `Apps` `PrintScreen`). Reason: a trigger binds `*Key::` with no
`~`, so a bare `*Left::` would swallow the Left arrow **everywhere**. The earlier
denylist wording admitted exactly these keys; the allowlist closes that hole. Bare
`LCtrl/LAlt/LShift` and bare `RCtrl/RAlt/RShift` are rejected here too — RWin is the
sole bare-modifier exception (the capture path can extend to other right-modifiers
later if wanted).

**Gate 2b — OS-shortcut denylist (same typing-safety rationale).** Modifier+key
combos are allowed by the shape rule, but a trigger binds `*chord::` with no `~`, so a
ubiquitous OS/editor combo would be swallowed globally — `*^c::` eats Copy
everywhere, even though `^c` is *not* one of `macos.ahk`'s own bindings (the Cmd layer
*Sends* `^c`, it doesn't *bind* it, so Gate 3 wouldn't catch it). Reject a curated set
of well-known combos: `^c ^v ^x ^z ^y ^a ^s ^f ^p ^n ^o ^w ^t`, `!Tab !F4`,
`#Tab #d #l #e #r`, `^+Esc`, and the like. The runbook recommends a function key
(F6–F8 / F12–F22) or RWin for the most predictable, conflict-free trigger.

**Gate 3 — no live-keymap collision.** Reject any chord already bound in `macos.ahk`
(passed to the validator as `boundChords`). Concrete collisions a modifier+key chord
could otherwise shadow: the Opt/`!`-layer — `!Left` `!Right` `!+Left` `!+Right`
(word-nav), `!Backspace` (delete-word) — and `^+c` (screenshot). Adding `!Left` would
register `*!Left::_FlowTriggerDown` over the existing `!Left::Send "^{Left}"`;
last-registration-wins breaks one or the other unpredictably. The validator consults a
**generated manifest** of bound chords (preferred) or, failing that, an explicit
blocklist of the known combos.

> *Cmd-layer scope note:* the Cmd shortcuts are `<#`-specific (left-Win only) and
> `LWin` is already hard-reserved, so a `#c` trigger does **not** cleanly shadow
> `<#c`. The live-collision risk is real for the `!`-layer; it is not a justification
> for blanket-blocking the Win modifier.

A re-press of an **already-added** key is *not* an error — it is the remove path
(checked before the gates, on the canonical form).

### Friendly labels

A small formatter maps hotkey strings to readable names for the HUD/tips:
`RWin` → `Right Cmd`, `^!d` → `Ctrl+Alt+D`, `F13` → `F13`. Raw AHK strings are what
get persisted and bound; labels are presentation only.

## Component 4 — hold-F1 help overlay (`macos.ahk`)

- **Tooltip hint.** A `_FLOW_HINT := "   F1 help"` suffix is appended to the
  Listening / Transcribing / cancelled tips, e.g. `⏳ Transcribing…   F1 help`.
- **Help HUD.** A `FLOW HELP`-titled HUD listing every live binding, built fresh on
  each open so it reflects the **current** extra triggers. To stop the three overlays
  (calibration, manage, help) drifting apart, extract a shared
  `_FlowHud(title, rows, keymapLines)` builder from the current inline
  `_FlowCalibShow` (which hand-rolls font sizes `s15/s12/s11`, margins, the
  `+ToolWindow +Border` options and `0B0E14` backdrop) and have all three call it,
  reusing `_FLOW_RAINBOW`. Layout:

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
  !TriggerMgmtActive`: `*F1::` shows the HUD. So it's live whenever dictation is ON
  (overriding app-Help F1 while on; toggling the tool off with F10 restores normal
  F1), but yields F1 to calibration mode (where F1 = set START) and to manage mode.
- **Teardown must not rely on the scoped up-variant.** `#HotIf` is re-evaluated per
  key event and never pairs an up with its down: pressing F9/F11 (or F10-off) *while
  holding* F1 flips the context, so a scoped `*F1 up::` would evaluate false and never
  fire — stranding the help HUD. Mitigation: a single idempotent `_FlowHelpDestroy()`
  (mirroring `_FlowCalibDestroy`) called from a **context-free** `*F1 up::` **and**
  from the F9, F11, and F10 handlers. The HUD always tears down regardless of which
  mode flip or key event ends the F1 press.

## Files

| File | Change |
|---|---|
| `opt/Desktop/Apps/scripts/flow-triggers.ahk` | **new** — data layer (path/load/save/validate) |
| `opt/Desktop/Apps/scripts/flow-triggers-test.ahk` | **new** — headless test |
| `opt/Desktop/Apps/scripts/macos.ahk` | handler refactor, startup dynamic-bind, F9 manage mode + InputHook + reserved-key guard, F1 help HUD + tooltip hints, F10-off cleanup, `#Include flow-triggers.ahk` |
| `opt/Desktop/Apps/scripts/WISPR-FLOW.md` | document F9 manage-triggers, F1 help, the new `flow-triggers.ini`, RWin guidance |
| `docs/mbo/specs/2026-05-31-wispr-flow-config-design.md` | this spec |
| `docs/mbo/plans/2026-05-31-wispr-flow-config.md` | implementation plan (next step) |

No `.gitignore` change: `flow-triggers.ini` lives under `%LOCALAPPDATA%`, outside
the repo, exactly like `flow-calib.ini`.

## Testing

- **`flow-triggers-test.ahk` — data layer** (headless, exit 0/1 like
  `flow-calib-test.ahk`): round-trip save→load; missing file → `[]`;
  non-integer `count` → `[]`; absent `count` → `[]`; blank-entry skip; **`count=3`
  with only `k1` present → `[k1]`** (count overstates → shorter list, not a
  placeholder); dedupe on save and load; **mixed modifier order (`!^d` + `^!d`) loads
  to one canonical entry**; nested-dir create on save.
- **`_FlowTriggerValidate` — policy** (same harness, three gates):
  - *reserved*: rejects `Esc`, `F1`–`F5`, `F9`–`F11`, `F23`, `F24`, `LWin`; **and the
    VK/SC forms of `F24` and `LWin` (`vk87`, `vk5B`) reject too** (proves canonical
    normalization runs before the check).
  - *shape allowlist*: rejects bare `c` / `5` / `Space` / `Enter` / `Tab` /
    `Backspace` / `Delete` / `Left` / `Right` / `Home` / `End` / `PgUp` / `Insert` /
    `CapsLock` / `Apps` / `PrintScreen`, and bare `LCtrl`/`RAlt`; **accepts bare `F6`
    and `F12`**, `F13`, `^!d`, `+F6`, `RWin`.
  - *OS-shortcut denylist*: rejects `^c`, `^v`, `!Tab`, `#d` even though they pass the
    shape rule and aren't in `boundChords`.
  - *live collision*: with a `boundChords` fixture containing `!Left`/`^+c`, rejects
    `!Left` and `^+c`; accepts `F13` (not in the set).
  - already-present key → routed to removal (not a validate error).
- **Manual desktop pass:**
  - F9 opens the manage HUD; pressing `F13` adds it (tip + HUD row) **and does not
    type/trigger F13 in the focused app**; holding F13 dictates; pressing F13 again
    removes it (tip + HUD row gone).
  - Pressing the right Command key adds `Right Cmd` and it dictates; **a lone RWin no
    longer opens the Start menu while added; after removing it via F9, a lone RWin
    opens the Start menu again.**
  - Pressing `Esc`/`F1`/`F10`/`F23` or a bare arrow during capture is refused with a
    reason; F9/Esc exits.
  - **Persistence:** add `F13`, reload `macos.ahk`, confirm `F13` still dictates.
  - **Corrupt file:** hand-set `count=abc` in `flow-triggers.ini`, reload, confirm
    `macos.ahk` loads and the Copilot key still works with zero extra triggers.
  - **Held-key + F10:** hold a trigger mid-dictation, tap F10, release — confirm Flow
    is **not** left recording.
  - Holding F1 while enabled shows the help listing the current triggers; release
    dismisses; **pressing F9 while still holding F1 does not strand the help HUD**;
    F10-off restores normal F1.
- **Regression:** `macos.ahk` `/validate` exits 0; Copilot key still
  starts/stops/pastes; Esc still cancels; F10/F11 unchanged; the `<#…` Cmd shortcuts
  and `!`-layer word-nav still work.

## Risks

- **`InputHook` capturing a bare modifier (RWin).** Win is a modifier, never an
  EndKey; resolved by the key-**up** algorithm in Component 3 (a modifier released with
  no non-modifier suffix → bare RWin). Residual risk: if RWin can't be resolved on a
  given machine, capture falls back to "press a non-modifier key, or a key with a
  modifier" — and the user can still use any allowed F-key/combo trigger.
- **Captured key leaking into the focused app.** Closed by the suppressing InputHook
  (`VisibleText/VisibleNonText := false`); covered by a manual-pass bullet.
- **Modifier+key shadowing the live keymap.** Closed by Gate 3 (live-collision check)
  fed a generated manifest of bound chords; covered by validator tests.
- **VK/SC bypassing a name-based reserved check.** Closed by canonical VK→name
  normalization applied before validate/bind/compare; covered by the `vk87`/`vk5B`
  reject tests.
- **Global F1 override while enabled.** Intentional and user-approved; F10-off is
  the documented escape hatch. Scoped off during calibration/manage so it never
  shadows those modes' own F1 use; teardown is context-free so the HUD can't strand.
