# Implementation Plan — Wispr Flow extra trigger keys + hold-F1 help (PR #65, spec rev 2)

## Overview

Build the approved design **bottom-up and test-first**, but open with a **risk-first throwaway spike** for the two mechanisms that cannot be unit-tested. The plan's spine is: a tiny SPIKE (de-risk InputHook capture + dynamic `Hotkey()` On/Off), then a pure, headless-testable layer built in strict dependency order — **normalizer first, then chord-composition, then the validator gates that depend on them** — so the `vk87`/`vk5B` normalize-before-validate invariant is a real test from the first gate phase (not deferred behind an identity seam) and the most error-prone capture step (`{Ctrl,Alt}+D → ^!d`, `RWin → RWin`) is a **pure, headless-tested** function with only the InputHook event-plumbing left manual. Then `macos.ahk` wiring in dependency order (`#Include` → globals → guarded load → handler refactor + new guard → dynamic bind → **donor-first `_FlowHud` extraction** → F9 manage → F1 help → F10-off cleanup), then the runbook.

**Environment reality (explicit).** The dev host is **WSL/Linux**. AutoHotkey v2 runs only on the **Windows** side; `AutoHotkey.exe` lives at `/mnt/c/Users/edwar/AppData/Local/Microsoft/WindowsApps/AutoHotkey.exe` (Windows path `C:\Users\edwar\AppData\Local\Microsoft\WindowsApps\AutoHotkey.exe`). There is **no CI runner** for `.ahk` (the repo's `scripts/test.sh` does not reference AutoHotkey); `flow-calib-test.ahk` is run manually on the Windows host, signalling pass/fail via **exit code 0/1** plus an `.out` file in `%TEMP%`. Consequently:

- **Headless-tested (auto-verifiable):** everything in `flow-triggers.ahk` — `_FlowTriggersPath/Load/Save`, `_FlowTriggerNormalize`, `_FlowComposeChord`, `_FlowTriggerValidate`, `_FlowTriggerLabel`, and `_FlowTriggerManifest`. These pure functions are the ONLY auto-verifiable units; they come first and drive the design (TDD). **Critically, `_FlowComposeChord` (the modifier-set ordering + bare-RWin-on-key-up decision) is factored out of the InputHook so it is headless-tested**, shrinking the manual surface to event plumbing only.
- **Manual-only (GUI / InputHook event wiring / live `Hotkey()`):** the spike and every `macos.ahk` HUD/InputHook-event/dynamic-bind behavior. Each such phase lists concrete steps + expected observation, because AHK GUI/InputHook event delivery/`Hotkey()` cannot be exercised headlessly on a WSL/Linux host.

**Headless verification command** (run on the Windows host, **with cwd = the directory containing `flow-triggers.ahk` and `flow-triggers-test.ahk`** so the `#Include` resolves):

```
cd <dir-with-flow-triggers.ahk>
"C:\Users\edwar\AppData\Local\Microsoft\WindowsApps\AutoHotkey.exe" flow-triggers-test.ahk ; echo EXIT=%ERRORLEVEL% ; type "%TEMP%\flow-triggers-test.out"
```

Pass = `EXIT=0` **and** `.out` contains `OK: all trigger config tests passed`. Fail = `EXIT=1` and the first `FAIL: …` line names the broken assertion. (Substitute your host's `AutoHotkey.exe` path if it differs.)

**`/validate` note.** `macos.ahk /validate` is the AutoHotkey syntax check: `AutoHotkey.exe /validate macos.ahk`, **exit 0 = parses**. It **must be run with cwd = the directory containing BOTH `macos.ahk` and `flow-triggers.ahk`** (the worktree scripts dir or the deployed dir), because line 3's `#Include flow-calib.ahk` and Phase 7's `#Include flow-triggers.ahk` are **relative includes resolved against the script's own directory**; running `/validate` from elsewhere fails to find the includes. Every `macos.ahk` phase ends by re-running it; syntax-only `/validate` is necessary but not sufficient, so each `macos.ahk` phase also carries a manual smoke + regression matrix.

**Convention notes honored.** `flow-triggers.ini` lives under `%LOCALAPPDATA%\dotfiles\` (mirrors `flow-calib.ini`; **no `.gitignore` change** — runtime path, not in git). No new documented dirs, so **no GEMINI.md/CLAUDE.md** additions. Commits are grouped one per coherent increment with conventional scopes `feat/test/fix/refactor/docs(wispr-flow)` (precedent: `a8b0143`). The `<#` Cmd layer (LWin-only) and the `!` Opt word-nav layer are a **regression gate after every `macos.ahk` phase**. The spike file is **never committed** (see Phase 0 guard).

---

## Phase 0 — SPIKE (throwaway): de-risk InputHook capture + dynamic `Hotkey()` On/Off. NOT COMMITTED.

**Goal.** Prove on the real Windows host, before any production code, that the two un-unit-testable mechanisms work: (1) a **suppressing** `InputHook` captures a bare **RWin resolved on key-up**, captures `^!d`/`F13` on the non-modifier key-down, and does **not leak** the key into the focused app; (2) `Hotkey("*RWin", fn, "On")` then `Hotkey("*RWin", , "Off")` under the **global** `#HotIf` context binds, then **releases RWin back to the OS** (Start menu returns). These are spec Risks §1–2 — the single most uncertain part of the feature.

**Files.** `_spike-capture.ahk`, created **only under a scratch path that is NOT swept into git** — write it to the Windows-visible scratch dir `%TEMP%\_spike-capture.ahk` (WSL: `/mnt/c/Users/edwar/AppData/Local/Temp/_spike-capture.ahk`), **never** under `opt/Desktop/Apps/scripts/` (a tracked, install.sh-globbed deploy dir). It is throwaway, deleted at end of phase, never committed, never `#Include`d by `macos.ahk`. **Hard guard:** before any `git add` in Phases 1–13, `git status` MUST show no `_spike-*` file anywhere under the worktree; the spike is never `git add`-ed and never lives in a tracked dir.

**Change.** Minimal standalone script (no dependence on `macos.ahk`): an F9-style toggle that arms one `InputHook` with `ih.VisibleText := false`, `ih.VisibleNonText := false`; an `OnKeyDown` recording held modifiers and starting a "modifier held, no non-mod seen" watch for `vk5C` (RWin); an `OnKeyUp` that, when a tracked modifier releases with no non-modifier seen during its press, emits the bare modifier (RWin → `RWin`); on a non-modifier key-down it composes `<held-mods><key>` and emits immediately. Each resolved string shows in a ToolTip. Separately, an F8 that toggles `Hotkey("*RWin", (*)=>ToolTip("RWin fired"), "On"/"Off")` under global context. Record the exact VK for RWin and any `KeyOpt`/`-Win` flags learned.

**Test.** manual-only (this mechanism is *why* it is a spike). With Notepad focused:
1. Arm capture; tap bare **RWin** → ToolTip shows `RWin`; **no Start menu; nothing typed in Notepad**.
2. Press **Ctrl+Alt+D** → ToolTip shows `^!d` on the D key-down (before release); **no `d` typed**.
3. Tap **F13** → ToolTip `F13`; **no F13 effect in Notepad**.
4. Disarm. F8 to bind `*RWin`; tap RWin → `RWin fired` ToolTip, **no Start menu**. F8 again to `Off`; tap RWin → **Start menu opens** (proves Off releases the hook).

**Verify.** All four observations match. **GO/NO-GO:** if steps 1–3 leak the key into Notepad, or RWin can't be resolved, or step 4 fails to restore the Start menu → **NO-GO on bare-RWin**: record it, fall back to the spec's documented residual ("press a non-modifier key, or a key with a modifier"), and **drop `RWin` from Gate 2(d)** in Phase 4 (F-keys/combos still ship). All else proceeds unchanged. All pass → **GO**: Component-3 algorithm is sound as written.

**Checkpoint.** Riskiest assumptions settled empirically. `_spike-capture.ahk` is **deleted** from `%TEMP%`. A one-line GO/NO-GO note (plus tuning: exact RWin VK, `KeyOpt` flags) is captured in the PR description to feed Phases 4 & 10. No production code written; nothing committed; `git status` confirmed clean of any spike file.

---

## Phase 1 — `flow-triggers.ahk` path/load/save (pure data layer, headless TDD)

**Goal.** Stand up the persistence layer mirroring `flow-calib.ahk` exactly: `_FlowTriggersPath`, `_FlowTriggersLoad`, `_FlowTriggersSave` with **count-authoritative / overstatement-shrinks / blank-skip / inert-orphan** semantics (spec Component 1). Normalization/dedupe land in Phase 2 via an explicit identity seam (so this phase's tests are real now and tighten later).

**Files.** `opt/Desktop/Apps/scripts/flow-triggers.ahk` (new), `opt/Desktop/Apps/scripts/flow-triggers-test.ahk` (new).

**Change.** New file, header `#Requires AutoHotkey v2.0`. Mirror the four `flow-calib.ahk` shapes:
- `_FlowTriggersPath()` → `EnvGet("LOCALAPPDATA") . "\dotfiles\flow-triggers.ini"` (parallels `_FlowCalibPath`, flow-calib.ahk:13–15).
- `_FlowTriggersLoad(path)` → `count := IniRead(path,"triggers","count","")`; if not `IsInteger(count)` → return `[]`; loop `i := 1..Integer(count)`: `t := Trim(IniRead(path,"triggers","k" i,""))`, **skip if `t=""`** (no placeholder pushed), else `_FlowTriggerNormalize(t)` (Phase-1 **identity seam**: `_FlowTriggerNormalize(x) => x`), then dedupe-preserving-order push. Missing file → `count` reads `""` → `[]`. The `IsInteger` guard is the defensive contract and must never throw (no `try` needed). This is the inverse of the per-key-default `_FlowCalibLoad` (flow-calib.ahk:21–29): there blanks default; here blanks are **skipped**.
- `_FlowTriggersSave(path, arr)` → `SplitPath`+`DirCreate` parent if absent (mirror `_FlowCalibSave`, flow-calib.ahk:32–38); dedupe (identity seam) then `IniWrite arr.Length` as `count`, then `k1..kN`. **Do not** delete orphaned higher-index keys (inert: the new `count` ignores them — explicitly tested below).

**Test (write FIRST, watch it FAIL).** `flow-triggers-test.ahk` mirroring `flow-calib-test.ahk` structure exactly: `#Requires AutoHotkey v2.0`, `#Include flow-triggers.ahk`, `RESULT := A_Temp "\flow-triggers-test.out"`, `_assert(cond,msg)` helper (flow-calib-test.ahk:9–15), unique `tmp := A_Temp "\flow-triggers-test-" A_TickCount ".ini"`. Groups for THIS phase:
1. Missing file → `_FlowTriggersLoad(tmp)` is `[]` (length 0).
2. Round-trip: `_FlowTriggersSave(tmp,["F13","F14"])` → Load length 2, `[1]="F13"`, `[2]="F14"`.
3. **Count overstatement (critical):** write `count=3`, only `k1=F13`,`k2=F14` → Load length 2 `["F13","F14"]`, **not** length 3, no `""` placeholder at index 3.
4. Absent `count` (write only `k1`) → `[]`; non-integer `count=abc` → `[]`.
5. Blank/whitespace skip: `count=3`,`k1=F13`,`k2=`(blank),`k3=F14` → `["F13","F14"]` length 2.
6. **Inert-orphan no-delete:** save `["F13","F14","F15"]`, then save `["F13"]` over the same `tmp`; reload → `["F13"]` length 1 (orphaned `k2`/`k3` ignored because `count=1`), AND assert the **contract is "load returns the short list", not "disk is scrubbed"** (a stale `k2` may remain on disk; that's allowed because `count` gates the read).
7. Nested-dir create: save to `A_Temp "\ft-dir-" A_TickCount "\sub\flow-triggers.ini"`, assert `FileExist`. Cleanup `try FileDelete`/`try DirDelete …,true`.
Footer `FileAppend "OK: all trigger config tests passed\n"`, `ExitApp(0)`. First run must be **EXIT=1** (no impl); implement to **EXIT=0**.

**Verify.** Headless command above (cwd = scripts dir) → `EXIT=0`, `.out` = OK line. `macos.ahk` untouched.

**Checkpoint.** Persistence contract (count-authoritative, blank-skip, overstatement→shorter list, inert-orphan, corrupt→`[]`, nested-dir create) proven headlessly. Dedupe/normalize assertions deferred to Phase 2 (need the real normalizer).

---

## Phase 2 — Canonical normalization + chord composition + dedupe (pure, headless TDD)

**Goal.** Implement `_FlowTriggerNormalize(chord)` (VK/SC→named, **fixed modifier order `^ ! + #`**, handedness fold) and `_FlowComposeChord(heldMods, baseKey)` (the pure capture-composition step factored out of the InputHook), then replace the Phase-1 identity seam in load/save, so a leaked `vk5B` can't bypass a name-based `LWin` check downstream (spec Component 3 "Canonical key form" + gotcha). These are the safety primitives every later gate depends on — they land **before** any validator gate, and the most error-prone capture logic becomes headless-testable here.

**Files.** `flow-triggers.ahk`, `flow-triggers-test.ahk`.

**Change.**
- `_FlowTriggerNormalize(chord)` (pure): split leading modifier symbols (`^ ! + # < >`) from the base token; **handedness fold** per this table — `vk5B`(LWin)→`LWin`, `vk5C`(RWin)→`RWin`, `vk87`→`F24`, plus the F-key/nav VKs the validator must catch; re-emit modifiers in fixed order `^ ! + #`; lowercase single-letter bases for stable compare. Document the table in a header comment.
  - **HANDEDNESS-FOLD-vs-COLLISION RESOLUTION (resolves the prior plan/spec contradiction — see Open Question 5).** The `<`/`>` handedness *prefixes* are **preserved on the Win modifier specifically**, NOT folded, because the Cmd layer is `<#…` (LWin) and a candidate `#c`/`>#c` (RWin) trigger must be a **distinct** chord per spec lines 277–280 ("`#c` does not cleanly shadow `<#c`"). Rule, pinned by test:
    - `<#c` normalizes to `<#c` (LWin-Cmd preserved) — it is **the Cmd layer**.
    - bare `#` / `>#` (RWin) normalizes to `#`/`>#` respectively and is treated as a **different** chord from `<#`.
    - a **bare** `vk5B` (no base, no other mod) → base key `LWin`; a **bare** `vk5C` → base key `RWin`. (Bare-modifier *base-key* resolution is for the hard-reserve `LWin` check and the RWin trigger; it is distinct from the `<#`/`>#` *modifier-prefix* case which always has a base or is the RWin-trigger handled as base `RWin`.)
    - For **non-Win** modifiers (`^ ! +`) handedness IS folded (`<^` → `^`) — those layers never bind handed.
  - Net effect: the manifest can store the Cmd layer in its real handed form `<#c`, and a user adding `#c`/`>#c` (RWin) is **NOT** falsely rejected as colliding with `<#c`. This is the deliberate, tested rule (no redesign — it pins the spec's stated intent).
- `_FlowComposeChord(heldModsArray, baseKeyOrEmpty)` (pure) → raw chord string, **order-independent in, canonical-modifier-order out**: maps a set of held-modifier names to symbols and concatenates with the base. Contract (pinned by test): `compose(["Ctrl","Alt"],"d") == "^!d"` AND `compose(["Alt","Ctrl"],"d") == "^!d"` (input order irrelevant); `compose(["RWin"],"") == "RWin"`; `compose(["LWin"],"") == "LWin"`; `compose(["LCtrl"],"") == ""` (sentinel empty = "reject a lone non-RWin modifier" — the InputHook turns this into the "add a key, not a lone modifier" tip). The InputHook in Phase 10 only wires events to this tested function.
- Replace the identity seam in `_FlowTriggersLoad` (normalize each kept entry, dedupe preserving first-seen order) and `_FlowTriggersSave` (normalize+dedupe `arr` before writing).

**Test (write FIRST).** Add groups covering normalization + composition explicitly:
8. Fixed order: `_FlowTriggerNormalize("!^d")=="^!d"`; `_FlowTriggerNormalize("+^F6")=="^+F6"`.
9. VK/SC→named: `"vk87"=="F24"`, bare `"vk5B"=="LWin"`, bare `"vk5C"=="RWin"`.
10. **Handedness rule (pins the Gate-1/Gate-3 surface):** `_FlowTriggerNormalize("<#c")=="<#c"` (LWin-Cmd PRESERVED, not folded); `_FlowTriggerNormalize("#c")=="#c"` and `"#c" != "<#c"` (RWin-combo is a DISTINCT chord); `_FlowTriggerNormalize("<^d")=="^d"` (non-Win handedness folded).
11. **Compose (the de-risked capture step):** `_FlowComposeChord(["Ctrl","Alt"],"d")=="^!d"`; `_FlowComposeChord(["Alt","Ctrl"],"d")=="^!d"` (order-independent); `_FlowComposeChord(["RWin"],"")=="RWin"`; `_FlowComposeChord(["LCtrl"],"")==""` (lone-non-RWin sentinel).
12. **Load dedupe/normalize:** ini `count=2`,`k1=!^d`,`k2=^!d` → Load `["^!d"]` length 1.
13. **Save dedupe:** `_FlowTriggersSave(tmp,["^!d","!^d","F13"])` → reload `["^!d","F13"]` length 2.
First run FAILs on the new groups; implement to green.

**Verify.** Headless → `EXIT=0`.

**Checkpoint.** Every persisted/compared chord is canonical; the `LWin`/`RWin` bypass surface and the `<#c` vs `#c` distinction are pinned by test; the error-prone capture-composition is headless-tested. Still no `macos.ahk` change.

---

## Phase 3 — `_FlowTriggerValidate` Gate 1 (reserved, normalize-first) + remove-path contract

**Goal.** Introduce `_FlowTriggerValidate(chord, boundChords)` → `{ok, reason}` with **Gate 1 only** (hard-reserved control keys), **normalizing input first** so VK/SC forms also reject. Establish that "already-present → remove, not validate-error" is the **caller's** responsibility (the validator only gates *additions*).

**Files.** `flow-triggers.ahk`, `flow-triggers-test.ahk`.

**Change.** `_FlowTriggerValidate(chord, boundChords)`: `c := _FlowTriggerNormalize(chord)` **first**. Gate 1 reserved set (canonical): `Esc, F1, F2, F3, F4, F5, F9, F10, F11, F23, F24, LWin`. If `c` ∈ set → `{ok:false, reason:"reserved (driver key)"}`. Else (for now) `{ok:true}`. `boundChords` accepted but unused until Phase 5.

**Test (write FIRST).**
14. Reserved rejects: each of `Esc`,`F1`,`F2`,`F3`,`F4`,`F5`,`F9`,`F10`,`F11`,`F23`,`F24`,`LWin` → `.ok=false`.
15. **Normalize-before-validate invariant (lands HERE, the first validator commit):** `_FlowTriggerValidate("vk87",[]).ok=false` (F24) and `_FlowTriggerValidate("vk5B",[]).ok=false` (LWin) — proves Gate 1 runs on the normalized form, the invariant is **not** deferred.
16. A clearly non-reserved chord that will pass later gates (`F13`) → `.ok=true` for now.
First run FAILs on 14–15; implement to green.

**Verify.** Headless → `EXIT=0`.

**Checkpoint.** Reserved-key hard block proven *including* VK/SC normalization, from the very first validator phase. (Group 16 is provisional, tightened by Phases 4–5 fixtures.)

---

## Phase 4 — Gate 2 (shape allowlist) + Gate 2b (OS-shortcut denylist)

**Goal.** Add the default-deny shape allowlist and the curated OS-combo denylist to `_FlowTriggerValidate`, on the canonical chord (spec Component 3).

**Files.** `flow-triggers.ahk`, `flow-triggers-test.ahk`.

**Change.** In `_FlowTriggerValidate`, after Gate 1:
- **Gate 2 allowlist** — admit `c` only if: (a) base is `F6,F7,F8,F12..F22` (bare or modified); (b) base is media/browser (`Volume*`,`Media*`,`Browser*`,`Launch*`), bare or modified; (c) `c` has ≥1 modifier (`^ ! + #`); or (d) `c == "RWin"` (sole bare-modifier exception — **omit (d) if Phase 0 was NO-GO**). Else `{ok:false, reason:"add an F6–F22 / media key, a modifier+key combo, or Right Cmd"}`. This explicitly rejects bare nav/edit/toggle/printable/whitespace keys and bare `LCtrl/LAlt/LShift/RCtrl/RAlt/RShift`.
- **Gate 2b denylist** (applies to modified chords passing shape) — reject the curated set `^c ^v ^x ^z ^y ^a ^s ^f ^p ^n ^o ^w ^t`, `!Tab !F4`, `#Tab #d #l #e #r`, `^+Esc` → `{ok:false, reason:"that's a common OS/editor shortcut"}`. Store as a normalized Map for O(1) lookup. **Note:** the Cmd layer *Sends* `^c/^v/...` but does NOT *bind* them, so Gate 3 (live keymap) cannot catch them — Gate 2b is exactly why these are blocked here, not in Gate 3.

**Test (write FIRST).**
17. Shape rejects (bare): `c`,`5`,`Space`,`Enter`,`Tab`,`Backspace`,`Delete`,`Left`,`Right`,`Home`,`End`,`PgUp`,`Insert`,`CapsLock`,`Apps`,`PrintScreen`,`LCtrl`,`RAlt` → all `.ok=false`.
18. Shape accepts: `F6`,`F12`,`F13`,`^!d`,`+F6`,`RWin` → all `.ok=true` (empty `boundChords`). *(Drop `RWin` from this assertion if Phase 0 NO-GO.)*
19. Gate 2b: `^c`,`^v`,`!Tab`,`#d` → `.ok=false` even though shape-valid and absent from `boundChords`.
First run FAILs; implement to green.

**Verify.** Headless → `EXIT=0`.

**Checkpoint.** Typing-safety gates proven: no bare typing/nav key, no ubiquitous OS combo addable. F-keys/combos/`RWin` still admitted.

---

## Phase 5 — Gate 3 (live-keymap collision) + `boundChords` fixture + scoped-chord exclusion

**Goal.** Reject any chord already in the live keymap by consulting `boundChords`; prove `!Left`/`^+c` reject and `F13` accepts. **Critically: exclude calibration-scoped chords** (F1–F5/F10/Esc bound only under `#HotIf CalibActive`) from Gate 3 so they don't over-reject globally valid triggers.

**Files.** `flow-triggers.ahk`, `flow-triggers-test.ahk`.

**Change.** Add Gate 3 (last gate): build a normalized set from `boundChords` once (normalize each entry — preserving `<#` handedness per Phase 2), and if `c` ∈ it → `{ok:false, reason:"already bound by macos.ahk"}`. **Scoped-chord rule:** the `boundChords` the caller passes must contain **only globally-live chords** — the Cmd layer (`<#…`, in handed form so `#c`/`>#c` do NOT collide with `<#c`), the `!` Opt layer (`!Left`,`!Right`,`!+Left`,`!+Right`,`!Backspace`), `^+c`, `~LWin`, and the global F10/F11. It must **exclude** chords that are live only under a transient `#HotIf` (calibration F1–F5/F10/Esc, the manage-mode F9/Esc, the `FlowState!=IDLE` Esc), because those keys are legitimately reusable as triggers outside that mode. (Gate 1 already hard-reserves the user-facing driver keys F1–F5/F9/F10/F11/Esc, so excluding their *scoped* duplicates from Gate 3 is safe and non-overlapping.)

**Test (write FIRST).** Fixture `bc := ["<#c","!Left","!Right","!+Left","!+Right","!Backspace","^+c"]` (Cmd-layer sample in HANDED form + the `!`-layer + screenshot) — note it deliberately contains **no** calib-scoped F-keys:
20. `_FlowTriggerValidate("!Left",bc).ok=false`; `_FlowTriggerValidate("^+c",bc).ok=false`.
21. `_FlowTriggerValidate("F13",bc).ok=true` (not in set).
22. **Handedness non-collision (resolves the contradiction):** `_FlowTriggerValidate("#c",bc).ok=true` — an RWin-`#c` candidate is **NOT** rejected by the LWin Cmd-layer `<#c` in `bc`, because Phase 2 keeps them distinct. (If Phase 0 NO-GO, `#c` still requires the RWin host to exist; the assertion stays valid because Gate 3 only checks collision, not host availability.)
23. **Scoped-exclusion proof:** with `bc` (which omits calib F-keys), `_FlowTriggerValidate("F6",bc).ok=true` — a globally-free F-key is **not** over-rejected by a calib-scoped binding it never collides with at runtime.
First run FAILs on 20; implement to green.

**Verify.** Headless → `EXIT=0`.

**Checkpoint.** All three gates green headlessly; Gate 3 fed only globally-live chords (Cmd layer in handed form), so it can't over-reject `#c`/`>#c` or calib-scoped F-keys.

---

## Phase 6 — `_FlowTriggerManifest()` + manifest-staleness parity guard

**Goal.** Provide the manifest Gate 3 is fed at runtime, and add a **parity guard** so the static list can't silently rot against `macos.ahk`'s live hotkeys. Per spec a generated manifest is preferred, a static blocklist acceptable as v1.

**Files.** `flow-triggers.ahk`, `flow-triggers-test.ahk`.

**Change.**
- `_FlowTriggerManifest()` → static array of `macos.ahk`'s **globally-live** chords (the GROUND MAP `boundChords` list, **stripping scope annotations** like `(scoped …)` and the leading `*` on `*F24`/`*Esc`, and **excluding** the calib/manage-scoped entries per Phase 5). **Cmd-layer entries are stored in their HANDED form (`<#c`, `<#v`, …)** so the Phase 2 handedness rule keeps `#c`/`>#c` addable. Header comment lists the `macos.ahk` layers it mirrors (Cmd `<#…` lines 37–98, Opt `!` lines 100–106, `^+c` screenshot, `~LWin`) and a TODO to auto-generate on deploy.
- **Parity guard test fixture:** embed the expected sorted, normalized list inline in the test (a dump of the global chords currently in `macos.ahk`). The parity test asserts `_FlowTriggerManifest()` (normalized+sorted) equals the expected dump. When a future PR adds/removes a global hotkey in `macos.ahk`, this test forces a deliberate manifest update.

**Test (write FIRST).**
24. `_FlowTriggerManifest()` contains normalized `!Left` and `^+c`; `_FlowTriggerValidate("!Left",_FlowTriggerManifest()).ok=false`; `_FlowTriggerValidate("F13",_FlowTriggerManifest()).ok=true`.
25. **Manifest never contains a calib-scoped F-key:** assert `F6`/`F7`/`F8` (globally free) are **absent** from `_FlowTriggerManifest()`, so they remain addable.
26. **Handedness in manifest:** `_FlowTriggerManifest()` contains `<#c` (handed), and `_FlowTriggerValidate("#c",_FlowTriggerManifest()).ok=true` — pins that the manifest's Cmd layer doesn't block the RWin combo.
27. **Staleness parity:** `_FlowTriggerManifest()` normalized+sorted == the expected inline dump (drift fails this test, forcing a deliberate update).
First run FAILs; implement to green.

**Verify.** Headless → `EXIT=0`. **The entire pure data + composition + validator + manifest surface is now complete and auto-verified before any `macos.ahk` change.**

**Checkpoint.** Validator is self-sufficient (`_FlowTriggerValidate(c, _FlowTriggerManifest())`), with a staleness guard, and the handedness rule is locked end-to-end. **Commit** Phases 1–6 as two grouped commits: `test(wispr-flow): flow-triggers data layer + normalize/compose + validator + manifest (headless)` and `feat(wispr-flow): flow-triggers.ahk load/save/normalize/compose/validate/manifest`. All headless work done.

---

## Phase 7 — `macos.ahk`: `#Include` + globals + guarded startup load (no behavior change)

**Goal.** Make the data layer reachable inside `macos.ahk` and declare new globals, with a guarded startup load placed **immediately beside the proven-working calib load** (so it executes at startup), and **no** binding yet. Purely additive; `/validate` exits 0; all existing shortcuts intact.

**Files.** `opt/Desktop/Apps/scripts/macos.ahk`.

**Change.**
- After line 3 (`#Include flow-calib.ahk`): add `#Include flow-triggers.ahk` (insertion point #1) so load/save/validate/compose exist before globals init.
- Globals block (after `_flowCalibGui := ""`, macos.ahk:185): add `TriggerMgmtActive := false`, `_flowTriggers := []`, `_flowTriggerGui := ""`, `_flowHelpGui := ""`, `_flowInputHook := ""`, `_FLOW_HINT := "   F1 help"` (insertion point #2).
- **Guarded load placement (BLOCKER addressed).** Place the guarded trigger load **directly adjacent to the existing calib load at macos.ahk:176–183** (i.e., right after line 183, before line 184's `CalibActive := false`), mirroring its `try/catch`:
  ```
  try
      _flowTriggers := _FlowTriggersLoad(_FlowTriggersPath())
  catch
      _flowTriggers := []
  ```
  Rationale: the existing `_flowCalib := _FlowCalibLoad(...)` at line 177 **demonstrably runs at startup in shipped PR #53** (calibration loads its ini and the WORKING offsets are populated). Placing the new load at the identical position inherits that proven execution path — this sidesteps any ambiguity about where the AHK v2 auto-execute section ends relative to the first hotkey `~LWin::` at line 24. **No** dynamic-bind loop yet (Phase 8).

**Test.** manual-only. Steps:
1. **Execution-proof FIRST (the blocker check):** add a temporary `_FlowTipFor("triggers loaded: " _flowTriggers.Length, 2000)` line immediately after the new load, launch `macos.ahk`, and confirm the tip appears at startup — this empirically proves the load line at ~184 actually executes (matching the calib load). Remove the temp line before commit. *(If the tip does NOT appear, the load is dead code: move both the calib load and this load into a `_FlowStartup()` function called from the top-of-file auto-exec region above line 24, and re-test. Do not proceed until the tip shows.)*
2. Run `/validate` with cwd = scripts dir (exit 0).
3. Launch `macos.ahk`; confirm `<#c` copies, `<#Left` line-jumps, `!Left` word-nav, Copilot-key dictation all still work (regression — nothing should change).
4. Corrupt-file check: set `count=abc` in the ini, relaunch → loads, no error dialog, zero extra triggers.

**Verify.** Startup-execution tip confirms the load runs; `/validate` exit 0; corrupt/missing ini does not crash startup; existing bindings unchanged.

**Checkpoint.** Module included, globals present, triggers loaded into `_flowTriggers` **with empirical proof the load executes**, not yet bound. **Commit** `feat(wispr-flow): include flow-triggers, add manage/help globals + guarded startup load`.

---

## Phase 8 — Refactor `*F24` → `_FlowTriggerDown`/`_FlowTriggerUp` + new guard + dynamic bind + concurrency rule

**Goal.** Extract the two Copilot-key handler bodies into shared named functions carrying the **new 3-term guard** (`!FlowEnabled || CalibActive || TriggerMgmtActive`), rebind `*F24` to them, add the startup dynamic-bind loop **at the proven-executing load region** under **global** `#HotIf`, and define explicit **concurrent-held-trigger** behavior against the single-flight `FlowState` machine (spec Component 2). A key hand-written into the ini binds on launch.

**Files.** `macos.ahk`.

**Change.**
- Add `_FlowTriggerDown()` = body of `*F24::` (macos.ahk:380–391) with the guard line changed to `if (!FlowEnabled || CalibActive || TriggerMgmtActive) return` (the `TriggerMgmtActive` term is the spec-mandated addition, **not** a literal extraction). Keep the `FlowState != "IDLE"` auto-repeat swallow, `MouseGetPos`, `FlowState := "DICTATING"`, tip, `SetTimer _FlowStartClicks,-1`.
- **Concurrency rule (single-flight, "FIRST RELEASE WINS" — corrected wording).** Because `FlowState` is single-flight and `_FlowTriggerUp()` keys on `FlowState`, NOT on which key owns the session, the real, accepted behavior is:
  - A **second** trigger pressed while `FlowState != "IDLE"` hits the existing early-return swallow → **no-op on down** (no double-start).
  - On release, **whichever held trigger is released FIRST while `FlowState == "DICTATING"` drives the STOP** — there is **no per-key ownership**. Releasing the other (already-past-DICTATING) key is then inert.
  - Document this verbatim in a one-line code comment: `; single-flight: FIRST trigger released while DICTATING drives STOP (no per-key ownership)`. This matches the single-flight intent; per-key ownership is explicitly **out of scope** per spec.
- Add `_FlowTriggerUp()` = body of `*F24 up::` (macos.ahk:393–401), both functions with `global` declarations (insertion point #5).
- Replace `*F24::{…}` with `*F24::_FlowTriggerDown` and `*F24 up::{…}` with `*F24 up::_FlowTriggerUp` (insertion point #4).
- **Dynamic-bind loop placement.** Append it **immediately after the Phase-7 guarded trigger load** (the proven-executing region at ~line 184, before `CalibActive := false` if practical, or right after the load block), under **global** `#HotIf` (no active `#HotIf` context is open at that point):
  ```
  for t in _flowTriggers {
      try Hotkey "*" t, _FlowTriggerDown, "On"
      try Hotkey "*" t " up", _FlowTriggerUp, "On"
  }
  ```
  Each call individually `try`-guarded so one bad entry is skipped, not fatal. Placing it beside the proven-executing load (Phase 7 step 1) guarantees it runs at startup; the functions `_FlowTriggerDown/Up` are defined later in the file but referenced by name, which AHK v2 resolves at load — confirm via the manual test below that an ini-listed `F13` actually binds.
- Append `_FLOW_HINT` to the Listening / Transcribing tips inside the extracted functions (`_FlowTip("🎤  Listening…" _FLOW_HINT)`, `_FlowTip("⏳  Transcribing…" _FLOW_HINT)`). The Esc-cancel tip is intentionally **left unhinted** (see Open Question 3, resolved "no").

**Test.** manual-only. Steps:
1. `/validate` (cwd = scripts dir) exit 0.
2. Copilot key dictates start→stop→paste (refactor parity), now with ` F1 help` suffix on the Listening/Transcribing tips.
3. **Clipboard/state-machine regression (full cycle):** start dictation, release → STOP fires → `AWAITING_CLIP`; let a real transcript arrive → `_FlowOnClip` (macos.ahk:341) re-activates the target and the `✓ (Flow auto-paste)` tip shows; state returns to IDLE. **Also: timeout path** — start, release, then let 15s elapse with NO clipboard event → `_FlowTimeout` (macos.ahk:333) resets to IDLE and the tip clears. Both prove the extraction didn't break the clipboard half of the machine.
4. Esc-cancel mid-dictation still works (its `#HotIf FlowState != "IDLE"` scope, macos.ahk:407–416, untouched).
5. Hand-write `flow-triggers.ini` `count=1`/`k1=F13`; reload; **hold F13 → dictates** (proves dynamic bind + name-resolution of the later-defined functions). Remove the line; reload; F13 inert.
6. **Concurrency (first-release-wins):** start dictation with F13 (held), then tap the Copilot key → second press is a no-op (no double-start). Then release **either** key while DICTATING → exactly one STOP fires (assert: releasing EITHER key stops dictation — NOT owner-tracked).
7. Corrupt entry: `count=abc`, relaunch → loads, Copilot works, zero triggers, no dialog.

**Verify.** `/validate` exit 0; Copilot parity incl. full clipboard cycle + 15s timeout; F13-from-ini dictates; concurrent press is inert and first-release stops; `<#…` Cmd layer + `!` word-nav + `^+c` screenshot all still fire (regression).

**Checkpoint.** All triggers route through one hold-to-talk path with the 3-term guard; persistence-driven binding works; concurrent-held behavior is defined as first-release-wins and commented. **Commit** `feat(wispr-flow): shared trigger handlers + global dynamic-bind + first-release concurrency rule`.

---

## Phase 9 — Extract shared `_FlowHud(title, rows, keymapLines)` (donor-first, before consumers)

**Goal.** Extract the HUD scaffolding from `_FlowCalibShow` (macos.ahk:256–288) **now**, before the manage/help consumers exist, so they're built on the real builder (no stub-then-extract churn) and the three HUDs can't drift (spec Component 4). Calibration HUD must look **pixel-identical** after the refactor.

**Files.** `macos.ahk`.

**Change.** Add `_FlowHud(title, rows, keymapLines)` returning a shown `Gui` handle, pinning the **exact** calib geometry (NOT the `_FlowToast` loop at lines 217–222 — that is a different, `s22`, control):
- `g := Gui("+AlwaysOnTop -Caption +ToolWindow +Border", title)` — the title arg is required by AHK even with `-Caption` (macos.ahk:259).
- `g.BackColor := "0B0E14"`; `g.MarginX := 16`, `g.MarginY := 12` (macos.ahk:260–261).
- **Title:** `g.SetFont "s15 Bold", "Consolas"` ONCE (macos.ahk:264), then the rainbow per-char loop from macos.ahk:264–268 — `Loop Parse title { g.SetFont "c" _FLOW_RAINBOW[...]; g.Add("Text", (A_Index=1 ? "xm ym" : "x+0 yp"), A_LoopField) }` — using the exact `"xm ym"` / `"x+0 yp"` geometry (NOT the toast's `x+0` pattern).
- **Body rows:** `g.SetFont "s12 Norm cD0D0D0", "Consolas"` (macos.ahk:273), then one `g.Add("Text", (first ? "xm y+14" : "xm y+4"), row)` per `rows` entry (mirrors macos.ahk:274–275 spacing: first body row `y+14`, subsequent `y+4`).
- **Keymap:** `g.SetFont "s11 c8A8A8A", "Consolas"` (macos.ahk:278), then one `g.Add("Text", (first ? "xm y+12" : "xm y+4"), line)` per `keymapLines` entry (mirrors macos.ahk:279–282).
- `g.Show("NoActivate AutoSize Center")` (macos.ahk:286); `return g`.
Refactor `_FlowCalibShow` to compute its `startMark`/`stopMark` dirty markers (macos.ahk:271–272), build `rows := [Format("START ...", ...), Format("STOP ...", ...)]` and `keymap := ["F1 set START      F2 set STOP", "F3 revert         F4 save", "F5 defaults       F10 test", "F11 / Esc   end calibration"]`, then `_flowCalibGui := _FlowHud("FLOW CALIBRATION", rows, keymap)`.

**Test.** manual-only. Steps: `/validate` (cwd = scripts dir) exit 0; launch, press F11 → calibration HUD appears; **pixel-diff check** — screenshot the HUD before this phase (from PR #53) and after; verify identical rainbow title geometry, START/STOP rows with `✓ saved`/`● unsaved` markers, keymap block render (since `/validate` cannot catch a visual regression); F1/F2 capture, F4 save, F3 revert, F5 defaults, F10 dry-run, Esc/F11 exit all still work.

**Verify.** `/validate` exit 0; calibration HUD **visually unchanged (pixel-diff)** and fully functional (capture/revert/save/defaults/dry-run parity with PR #53).

**Checkpoint.** One HUD builder pinning the exact calib fonts/geometry; calibration proven still correct on top of it; manage/help can consume it directly next. **Commit** `refactor(wispr-flow): shared _FlowHud builder extracted from _FlowCalibShow`.

---

## Phase 10 — F9 manage mode: toggle + manage HUD + InputHook capture (add/remove) + mutual exclusion

**Goal.** F9 manage mode: rainbow/grey toast, manage HUD via `_FlowHud`, suppressing `InputHook` whose event handlers feed the **already-headless-tested** `_FlowComposeChord`, resolving a bare modifier on key-**up**; capture=toggle add/remove with live bind/unbind + save; friendly labels; mutual exclusion with F11 (spec Component 3). Carries the Phase-0 GO/NO-GO outcome.

**Files.** `macos.ahk` (consumes `_FlowTriggerLabel`/`_FlowComposeChord`/`_FlowTriggerNormalize`/`_FlowTriggerValidate`/`_FlowTriggerManifest` from `flow-triggers.ahk`).

**Change.**
- `_FlowTriggerLabel(chord)` lives in `flow-triggers.ahk` (add it in the Phase 2 region if not already present, pure: `RWin`→`Right Cmd`, `^!d`→`Ctrl+Alt+D`, `F13`→`F13`, `F24`→`Copilot key`).
- `_FlowManageShow()`: rows = `"🔒 Copilot key (F24)   locked default"` then one row per `_flowTriggers` via `_FlowTriggerLabel`; keymap = `["Press a key to add it · press again to remove","Esc / F9 to exit"]`; `_flowTriggerGui := _FlowHud("MANAGE TRIGGERS", rows, keymap)`.
- `_FlowManageArm()`: `ih := InputHook("V")`; `ih.VisibleText := false`, `ih.VisibleNonText := false` (suppress so captured keys don't type into the focused app); register `OnKeyDown`/`OnKeyUp` that **track held modifiers + base key and delegate the composition to `_FlowComposeChord(heldMods, baseKey)`** (the pure, Phase-2-tested function) — bare RWin resolves on key-**up** (held modifier released, no non-modifier seen, VK `vk5C` → `_FlowComposeChord(["RWin"],"")`); a non-modifier composes on key-down and resolves immediately; a lone non-RWin modifier yields `_FlowComposeChord`'s empty sentinel → tip "add a key, not a lone modifier". Start hook; store `_flowInputHook := ih`. *(If Phase 0 NO-GO: omit bare-RWin resolution; lone modifiers all reject.)* The InputHook here contains **only event plumbing**; the chord logic is the headless-tested `_FlowComposeChord`.
- `_FlowManageResolve(raw)`: `c := _FlowTriggerNormalize(raw)`; if `c == ""` (sentinel) → tip "add a key, not a lone modifier", return; if `c` ∈ `_flowTriggers` (normalized compare) → **remove path:** drop, `_FlowTriggersSave`, unbind **both** `Hotkey "*" c, , "Off"` and `Hotkey "*" c " up", , "Off"` **under global context**, tip `✓ removed <label>`; else `v := _FlowTriggerValidate(c, _FlowTriggerManifest())`: if `v.ok` → bind both `"On"`, push, save, tip `✓ added <label>`; else tip `✗ <label> — <v.reason>`. Refresh `_FlowManageShow()` after any action.
- `_FlowManageDestroy()`: idempotent teardown of `_flowTriggerGui` (identity-guard like `_FlowCalibDestroy`, macos.ahk:247–253) AND `_flowInputHook` (`try _flowInputHook.Stop()`, clear).
- After the F11 handler (macos.ahk:446): add global `F9::` — enter only from `FlowState="IDLE" && !CalibActive` (mutual exclusion); flips `TriggerMgmtActive`, on→`_FlowManageShow()`+`_FlowManageArm()`+rainbow toast, off→`_FlowManageDestroy()`+grey toast. Extend the F11 entry guard (macos.ahk:438) to also require `!TriggerMgmtActive`.
- After the calibration `#HotIf` block (macos.ahk:491): add `#HotIf TriggerMgmtActive` … `F9::` (toggle off) and `Esc::` (exit) both calling `_FlowManageDestroy()` + clear flag + grey toast … bare `#HotIf` (insertion point #10).

**Test.** manual-only. Steps (Notepad/editor focused):
1. F9 → manage HUD + ON toast, `🔒 Copilot key (F24)` locked row.
2. Press **F13** → `✓ added F13`, HUD gains row, **and F13 does NOT type/trigger into the editor** (suppression). Hold F13 → dictates. Press F13 again → `✓ removed`, row gone.
3. **RWin global-bind regression proof:** press **Right Cmd** → `✓ added Right Cmd`; exit F9; **lone RWin no longer opens Start menu** (proves global-context bind); re-enter F9, press RWin → removed; **lone RWin opens Start menu again** (proves global-context `Off` unbind — same-variant match). *(Skip if Phase 0 NO-GO.)*
4. **Handedness non-collision live:** press **Ctrl+Alt+D** (`^!d`) → added; **#c (RWin+c)** if RWin available → added and does NOT collide with the `<#c` Cmd-layer copy.
5. Press `Esc`/`F1`/`F10`/`F23`/bare `Left` during capture → each refused with a reason; HUD unchanged.
6. F9 or Esc exits; grey toast.
7. Mutual exclusion: F11 refuses to enter while managing; F9 refuses while calibrating.
8. Persistence: add F13, reload `macos.ahk`, F13 still dictates (closes the loop with Phase 8).

**Verify.** `/validate` exit 0; full matrix passes — especially no-leak suppression and the RWin add→remove→Start-menu-returns sequence (doubles as the global-`#HotIf` bind/unbind same-variant proof). Copilot key inert while managing.

**Checkpoint.** Runtime add/remove with persistence, suppression, RWin bare-modifier capture (via headless-tested compose), mode mutual-exclusion all work. **Commit** `feat(wispr-flow): F9 manage mode (InputHook capture + add/remove + HUD)`.

---

## Phase 11 — F1 hold-for-help HUD + tooltip hints + context-free teardown

**Goal.** Hold-F1 help overlay scoped to dictation-ON (yielding to calib/manage), with a **context-free** `*F1 up::` teardown also called from **F9 and F11** so the HUD can't strand across a mode flip; `_FLOW_HINT` on the listening/transcribing tips (spec Component 4). **F10-off help teardown is OWNED BY PHASE 12** — do NOT edit the F10 handler in this phase.

**Files.** `macos.ahk`.

**Change.**
- `_FlowHelpShow()` (via `_FlowHud`): rows built fresh each open — `"Copilot key   hold to dictate   🔒"`, one `"<label>   hold to dictate"` per `_flowTriggers` entry (via `_FlowTriggerLabel`), then `"Esc   cancel dictation"`, `"F10   dictation on / off"`, `"F11   calibrate overlay"`, `"F9   manage trigger keys"`, `"F1 (hold)   this help"`; `_flowHelpGui := _FlowHud("FLOW HELP", rows, [])`.
- `_FlowHelpDestroy()`: idempotent identity-guarded teardown of `_flowHelpGui` (mirror `_FlowCalibDestroy`, macos.ahk:247–253).
- Scoped `#HotIf FlowEnabled && !CalibActive && !TriggerMgmtActive`: `*F1::_FlowHelpShow()`.
- **Context-free** `*F1 up::_FlowHelpDestroy()` (no `#HotIf`, global scope) — re-evaluated `#HotIf` could otherwise fire the up-variant in a different context, so the teardown must be unconditional.
- Also call `_FlowHelpDestroy()` from **only the F9 handler, the F11 handler, and the `#HotIf FlowState != "IDLE"` Esc handler** (so a mode flip while holding F1 tears the HUD down). **Explicitly defer the F10-off help-teardown to Phase 12** (Phase 12 replaces the F10 off-branch wholesale with a unified idempotent teardown whose first step is `_FlowHelpDestroy()`); adding it to F10 here would be immediately overwritten by Phase 12 — do not touch F10 in this phase.
- Append `_FLOW_HINT` only to the Listening/Transcribing tips (already done in Phase 8). The Esc-cancel `✗ cancelled` tip (macos.ahk:414, set via `_FlowTipFor`) is **left unhinted** — F1 help is irrelevant after a cancel and the toast is transient (Open Question 3, resolved "no").

**Test.** manual-only. Steps:
1. Dictation ON: hold F1 → FLOW HELP HUD lists Copilot + current triggers + keymap; release → dismissed.
2. Add `F13` via F9, hold F1 → HUD shows an `F13   hold to dictate` row (built-fresh proof).
3. **Strand test (F9/F11 only here):** hold F1, then press F9 (or F11) while still holding F1 → HUD tears down (not stranded); release F1 → no stray HUD. *(The F10-while-holding-F1 strand case is verified in Phase 12.)*
4. F10-off → hold F1 → normal app F1 (no HUD, because the scope requires `FlowEnabled`); F10-on restores help.
5. Tips read `🎤 Listening…   F1 help` / `⏳ Transcribing…   F1 help`; the `✗ cancelled` tip has **no** ` F1 help` suffix.

**Verify.** `/validate` exit 0; help-overlay matrix incl. F9/F11 strand test passes; F1 yields to calibration (F1=set START) and manage modes; F10 handler untouched.

**Checkpoint.** F1 help complete and strand-proof against F9/F11; F10 strand deferred to Phase 12 (intentional, single-owner). **Commit** `feat(wispr-flow): hold-F1 help HUD + tooltip hints + context-free teardown`.

---

## Phase 12 — F10-off cleanup: one idempotent teardown handler (close the latent recording gap)

**Goal.** When F10 turns dictation off mid-dictation, run **one idempotent teardown** that sequences help-destroy → cancel-pending-start → STOP → IDLE, so a held trigger never leaves Flow recording — and so the worst case (F1 held + trigger held + F10 tapped, including a tap *during the start dwell before START fires*) resolves cleanly through a single handler (spec Component 2 "Held-key + F10-off" + gotcha; closes the PR #53 latent gap). **This phase OWNS the F10 handler's off-branch wholesale and is the single place the F10-off help teardown lands** (Phase 11 deferred it here).

**Files.** `macos.ahk`.

**Change.** In `F10::` (macos.ahk:421–429), replace the existing off-branch (macos.ahk:424–427) with a single idempotent teardown when `!FlowEnabled && FlowState != "IDLE"`:
1. `_FlowHelpDestroy()` (Phase 11 dependency — guarantees no stranded help HUD even if F1 was held; this is the F10-off help teardown Phase 11 deferred here).
2. Cancel the in-flight start: `SetTimer _FlowStartClicks, 0` — covers the case where F10 is tapped **during the 1.2s hover-dwell before START fires** (no STOP needed because START never landed).
3. Cancel timeout: `SetTimer _FlowTimeout, 0`.
4. If `FlowState = "DICTATING"` (START already fired): issue STOP via `SetTimer _FlowStopClicks, -1` (mirror the `*Esc` teardown, macos.ahk:410–414).
5. `FlowState := "IDLE"`.
Keep the existing OFF toast (macos.ahk:428). The ordering (cancel start *before* deciding STOP) makes the in-dwell tap and the post-START tap both correct, and re-entry is harmless because the branch only runs while `FlowState != "IDLE"`.

**Test.** manual-only. Steps:
1. Hold a trigger (Copilot or added F13) to start dictating; **while holding**, tap F10 (off); release → Flow **not** left recording; state IDLE; OFF toast.
2. **In-dwell timing:** hold the trigger and tap F10 **within the 1.2s before START fires** → no START click lands, no STOP needed, state IDLE, OFF toast; Flow never armed.
3. **Worst case (the F10 strand from Phase 11):** hold F1 *and* a trigger, tap F10 → help HUD gone, dictation stopped, IDLE — all via the one handler.
4. F10 back on → normal dictation resumes.

**Verify.** `/validate` exit 0; Flow not stranded recording in either the post-START or in-dwell case; no stranded help HUD on F10-off; toast unchanged.

**Checkpoint.** Latent PR-#53 gap closed by a single idempotent teardown covering help-destroy + cancel-start + STOP + IDLE across all timing windows; F10-off help teardown lives here only (no double-edit with Phase 11). **Commit** `fix(wispr-flow): F10-off idempotent teardown stops a held-trigger dictation`.

---

## Phase 13 — Runbook updates (`WISPR-FLOW.md`)

**Goal.** Document F9 manage mode, F1 hold-help, `flow-triggers.ini`, and the RWin dead-key-while-added caveat (spec Files row + Component 2 RWin section).

**Files.** `opt/Desktop/Apps/scripts/WISPR-FLOW.md`.

**Change.**
- Controls table (WISPR-FLOW.md:35–41): add `F9` (manage trigger keys — add/remove via capture) and `F1 (hold, dictation ON)` (help overlay listing live bindings); note the listening/transcribing tips now show ` F1 help` (the cancelled tip does not).
- New "Adding extra trigger keys (F9)" section: press-to-add / press-again-to-remove; Copilot key (F24) is the **locked default**; allowed shapes (F6–F8 / F12–F22, modifier+key, **Right Cmd**); the reserved/OS-denylist policy in plain language; persistence to `%LOCALAPPDATA%\dotfiles\flow-triggers.ini` (same class as `flow-calib.ini`, **not in git**, survives re-deploy). Note the **first-release-wins** behavior when two triggers are held (releasing either stops dictation).
- **RWin dead-key callout** (Caveats, WISPR-FLOW.md:141): while Right Cmd is an added trigger, a lone RWin no longer opens the Start menu; **F10-off does NOT restore it** — F10 gates whether dictation *fires*, not whether keys are *captured*; to return RWin to its OS role, **remove it via F9** (or quit `macos.ahk`). State this is deliberate. Non-Win triggers (F13/`^!d`) are also swallowed but harmlessly.
- Note hold-F1 help is suppressed during calibration/manage modes and restored by F10-on.

**Test.** manual-only (doc review): re-read the controls table + new sections against implemented behavior from Phases 8–12; confirm every documented key/path/caveat matches.

**Verify.** Markdown renders; every new control and the RWin caveat present and accurate; cross-check each spec "Files → runbook" requirement covered.

**Checkpoint.** Runbook matches shipped behavior. **Final commit** `docs(wispr-flow): document F9 manage, F1 help, flow-triggers.ini, RWin dead-key note`; full manual regression pass (Copilot dictation + full clipboard cycle + 15s timeout, Esc cancel, F10/F11, `<#…` Cmd shortcuts, `!`-layer word-nav, `^+c`) + `/validate` exit 0.

---

## Traceability — Phase → Spec component

| Phase | Spec component |
|---|---|
| 0 (spike) | Component 3 capture algorithm + Risks §1–2 (de-risk only; not a deliverable; not committed) |
| 1 | Component 1 — `_FlowTriggersPath/Load/Save`, count-authoritative load, blank-skip, count-overstatement, inert-orphan |
| 2 | Component 3 "Canonical key form" (normalize + handedness rule) + `_FlowComposeChord` (pure capture step); Component 1 dedupe-on-save/load |
| 3 | Component 3 Gate 1 (reserved set + normalize-first `vk87`/`vk5B` reject) |
| 4 | Component 3 Gate 2 (shape allowlist) + Gate 2b (OS denylist; covers Cmd-layer *Sent* combos Gate 3 can't see) |
| 5 | Component 3 Gate 3 (live collision via `boundChords`) + scoped-chord exclusion + handedness non-collision |
| 6 | Component 3 "generated manifest (preferred) / blocklist (v1)" + staleness parity guard + handed Cmd layer |
| 7 | Files row `#Include flow-triggers.ahk`; Component 2 globals + guarded load (with startup-execution proof) |
| 8 | Component 2 handler refactor + hardened `TriggerMgmtActive` guard + dynamic-bind + first-release concurrency rule + full clipboard/timeout regression; Component 4 `_FLOW_HINT` |
| 9 | Component 4 shared `_FlowHud` builder (donor-first extraction from `_FlowCalibShow`, exact calib geometry pinned) |
| 10 | Component 3 F9 manage mode, InputHook event plumbing → headless-tested `_FlowComposeChord`, toggle add/remove, unbind shape, friendly labels, mutual exclusion; "Right Command" goal |
| 11 | Component 4 hold-F1 help HUD, scope, context-free teardown (F9/F11), tooltip hints; F10 teardown deferred to Phase 12 |
| 12 | Component 2 "Held-key + F10-off" idempotent help-destroy + STOP-before-IDLE (incl. in-dwell timing); single F10 owner |
| 13 | Files row `WISPR-FLOW.md`; RWin dead-key documentation |

## Traceability — Phase → resolved review item (rev 2 hardening + this critique)

| Resolved review item | Phase(s) |
|---|---|
| Precise InputHook capture, bare-RWin resolves on key-**up** | 0, 10 |
| InputHook suppression (`VisibleText/VisibleNonText := false`) — no leak into focused app | 0, 10 |
| **Capture composition factored into pure headless-tested `_FlowComposeChord`** (manual surface shrunk to event plumbing) | 2, 10 |
| Allowlist (default-deny) bare-key policy incl. nav/edit/toggle reject | 4 |
| Gate 2b OS-shortcut denylist (`^c ^v !Tab #d …`) — covers Cmd-layer *Sent* (not bound) combos | 4 |
| Gate 3 live-keymap collision via injected `boundChords` + manifest | 5, 6 |
| Manifest staleness guard (parity test vs live `macos.ahk` chords) | 6 |
| Scoped-only chords (calib F1–F5/F10/Esc) excluded from Gate 3 — no over-reject | 5, 6 |
| **Handedness-fold-vs-collision contradiction resolved: `<#c` preserved, `#c`/`>#c` distinct, pinned by test** | 2, 5, 6 |
| Canonical VK→name normalization before every check (`vk87`/`vk5B` reject) — tested at first validator commit | 2, 3 |
| Normalizer handedness rule (`<#`/`>#`/`vk5B`/`vk5C`) pinned by test | 2 |
| New mode-guard term `TriggerMgmtActive` in `_FlowTriggerDown` (not a literal extraction) | 8 |
| **Concurrent held triggers: "FIRST RELEASE WINS" — corrected (no owner-tracking), commented + asserted** | 8 |
| **Full clipboard cycle + 15s timeout regression after `*F24` refactor** | 8 |
| F9/F11 mode mutual-exclusion | 10 |
| Dynamic bind/unbind under global `#HotIf`, On/Off same-variant (proven by RWin add→remove→Start-menu) | 8, 10 |
| **Startup load/bind placed at proven-executing calib-load region + empirical execution proof (auto-exec blocker)** | 7, 8 |
| Context-free F1 teardown (no stranded help HUD on mode-flip) | 11, 12 |
| **F10-off help-teardown owned by Phase 12 only (no double-edit with Phase 11)** | 11, 12 |
| Shared `_FlowHud` so the three HUDs can't drift — **exact calib fonts/geometry pinned + pixel-diff verify (not the toast loop)** | 9 |
| F10-off semantics — one idempotent handler STOP/cancel a held trigger before IDLE, incl. in-dwell timing | 12 |
| Count overstatement → shorter list, not placeholder; inert-orphan no-delete asserted | 1 |
| Corrupt ini degrades to "no extra triggers", never fatal at startup | 1, 7 |
| Idempotent identity-guarded HUD teardown (manage/help) | 10, 11 |
| RWin dead-key-while-added documented; F10 doesn't restore it; first-release-wins documented | 13 |
| `flow-triggers.ini` under `%LOCALAPPDATA%`, no `.gitignore` change | 1, 13 |
| **Spike file under ignored scratch path (`%TEMP%`), never in tracked deploy dir + pre-commit `git status` guard** | 0 |
| **`/validate` + headless test cwd = dir with the includes (relative `#Include` resolution)** | 1–13 |
| **`_FLOW_HINT` NOT on the transient cancelled tip (Open Question 3 resolved "no")** | 8, 11 |

## Open Questions

1. **`/validate` host invocation.** The repo has no committed `/validate` wrapper; the plan assumes `AutoHotkey.exe /validate macos.ahk` run Windows-side **with cwd = the scripts dir** (so the relative `#Include`s resolve), or via the `/mnt/c/...WindowsApps/AutoHotkey.exe` path from WSL. If the team expects a named runner script, that changes only the literal command, not the plan.
2. **Manifest auto-generation vs static (Phase 6).** The plan ships the static `_FlowTriggerManifest()` + a staleness parity test (the spec's accepted "v1 minimum"), with a TODO for deploy-time generation. Confirm a generated manifest is **not required** for this PR to land.
3. **`_FLOW_HINT` on the "cancelled" tip — RESOLVED "no" in this plan.** Component 4 lists "Listening / Transcribing / cancelled" as the hinted tips, but the cancelled tip is a transient `_FlowTipFor` toast and F1 help is meaningless after a cancel. The plan applies `_FLOW_HINT` only to Listening/Transcribing. Flag if the team wants it on the cancelled toast too (trivial one-line add in the `*Esc` handler).
4. **No `.ahk` CI runner.** There is no automated runner for `flow-calib-test.ahk` today (`scripts/test.sh` ignores `.ahk`), so `flow-triggers-test.ahk` is run manually on the Windows host like its predecessor. If automated execution is desired, that is a separate infra task — out of scope here unless directed.
5. **Handedness-fold-vs-collision — RESOLVED in Phase 2.** The prior plan's normalize fold (`<#c → #c`) contradicted spec lines 277–280 (`#c` must NOT shadow `<#c`). This plan pins the rule: **Win-modifier handedness is preserved** (`<#c` stays `<#c`, `#c`/`>#c` are distinct chords), non-Win handedness folds. Confirm this matches the spec author's intent for the RWin-`#c` trigger case; if instead `#c` should collide with `<#c` by design, only Phase 2 group 10 + Phase 5 group 22 + Phase 6 group 26 assertions flip (no structural change).
6. **AHK v2 auto-execute placement — DE-RISKED, not assumed.** The trigger load + bind loop sit at the **same position as the shipped, working calib load** (macos.ahk:177), and Phase 7 step 1 adds an **empirical startup-execution tip** that must be observed before proceeding (with a documented fallback: hoist both loads into a top-of-file `_FlowStartup()` if the tip never shows). This converts the critique's blocker into a verified checkpoint rather than an assumption.
