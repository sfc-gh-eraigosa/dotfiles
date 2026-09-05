# gff TUI — vim navigation, `/` regex search, `:` command line — spec

- **Slug:** `gff-tui-vim`
- **Date:** 2026-09-05
- **Status:** Approved (design approved in-chat 2026-09-05; spec self-reviewed)
- **Relates to:** parent objective `gff` (spec `./gff.md` F10, plan `../plans/gff.md` P3-T1) ·
  sibling keymap `./fleet-tui.md` F4–F7 (the vim/search contract this mirrors) ·
  issue #TBD-on-anchor · design PR via `gss feature gff-tui-vim`

## 1. Goal

`gff tui` becomes fast to drive from the keyboard the way vim users expect: `j/k/h/l`,
`gg/G`, `ctrl+d/u/f/b` move through the flag tree; `/` opens an incremental **regex** search
that finds a flag anywhere on the current page — expanding the collapsed area that holds it —
and `n/N` hop between hits; `:` opens an ex-style command line so `:set install.ai.claude
false` or `:unset <key>` work from muscle memory, with Tab completion of key paths. The
outcome: find any flag in a few keystrokes and toggle it without leaving the TUI or
reaching for the arrow keys. Every write still goes through the same override writer as
`gff set` / `gff unset`.

## 2. Use cases

**UC1 — find and flip.** User / runs `gff tui`, types `/wispr` / the area holding
`install.windows.wispr-flow` expands, the row highlights, the cursor lands on it; Space toggles
it / acceptance: three keystrokes plus Space, no arrow keys, the user-override file gains the
key, nothing else on disk changes.

**UC2 — regex across a family.** User / types `/ai\.(claude|teams)`, Enter, then `n`, `n` /
each `n` moves to the next matching row, wrapping at the end, the status shows `[2/3]` /
acceptance: the visited order is row order; wraparound is deterministic; no match leaves the
cursor where it was and says `pattern not found`.

**UC3 — typo mid-search.** User / types `/[ai` / an inline error (`missing closing ]`) shows
under the prompt, the previous matches stay highlighted, typing `]` recovers / acceptance: no
panic, no mode lockout, the prompt keeps every keystroke.

**UC4 — set from the command line.** User / types `:set install.pkg.manager brew`, Enter /
the choice flag's selection becomes `brew`, the row refreshes with `user-override`
provenance, mode returns to the list / acceptance: an unknown key, a bool value that is not
`true|false`, or a choice id not in the option list is rejected inline with the offending
token named, and **no file is written**.

**UC5 — complete a long key.** User / types `:unset install.ai.` then Tab, Tab / the command
line cycles `install.ai.claude`, `install.ai.teams` in item order / acceptance: completion is
scoped to the current page and namespace; Tab with no candidate is a no-op.

**UC6 — long list, no mouse.** User / on the (All) page with three areas expanded presses
`ctrl+d`, `G`, `gg` / the cursor moves half a page down, to the last row, to the first row;
the viewport follows / acceptance: motions clamp at both ends; `g` alone does nothing until
the second `g` (a different key cancels the pending `g`).

**UC7 — help is still one key away.** User / presses `?` in the list, the picker, the detail
view / the help overlay opens and lists the vim keys; `h` in the list now pages left /
acceptance: `h`/`H` never open help in any view; `?` and F1 do.

## 3. Architecture

All new code lives in `sdk/gff/internal/tui`. No new module dependency: the prompt is a
~60-line line editor, not `bubbles/textinput` (see §8).

| Unit | Does | Used by | Depends on |
| :-- | :-- | :-- | :-- |
| `input.go` — `lineInput` | An editable rune buffer with a cursor: insert, backspace, delete, ←/→, home/end, `ctrl+u` (kill to start), `ctrl+w` (kill word). `Handle(tea.KeyMsg) bool` reports whether it consumed the key. | search mode, command mode | bubbletea key types only |
| `search.go` — `searchState` + pure helpers | `compilePattern` (smartcase: an all-lowercase pattern compiles with `(?i)`), `matchItem` (does `path` or `description` match), `rowKey` (row identity across rebuilds), `inScope` (the one visibility rule `buildRows` and search share), plus the mode methods `startSearch/applySearch/commitSearch/cancelSearch/collectMatches/nextMatch/clearHighlights`. | `Model` search mode, `n`/`N` | `regexp`, `resolve.Resolved` |
| `command.go` — `parseCommand`, `completeKey`, `(m *Model) execCommand` | Tokenizes `:`-lines, validates against the resolved items, executes through `overrides.Write` / `overrides.Unset`, refreshes rows via the `Explain` hook (or `WithValue` when `Explain` is nil — the same fallback `activateFeature` uses). | `Model` command mode | `overrides`, `resolve`, `gffv1` |
| `keys.go` — `motionState` + `(m *Model) handleMotion` | The vim motion table for the list (and `j/k` for the picker): `j k h l gg G ctrl+d ctrl+u ctrl+f ctrl+b`, with the pending-`g` state. Returns `handled` so `updateList` falls through to its existing switch otherwise. | `updateList`, `updatePicker` | `Model` cursor/pages/viewport fields |
| `model.go` | Two new modes, `modeSearch` and `modeCommand`, dispatched from `Update`; `applySearch` (expand → rebuild rows → row-index matches → cursor); `nextMatch`; the `:noh` Esc. | — | all of the above |
| `view.go` | The prompt line (`/…` or `:…` plus inline error) replaces the footer hint while a prompt is open; committed searches show `/pattern [i/N]` in the footer; matching rows carry a `*` gutter marker (and a highlight style when color is on). Help overlay lists the vim keys. | — | `style` |

**Data flow, `/`:** keystroke → `lineInput` → `compilePattern` → `matchItem` over `m.items`
in scope sets `m.expanded[...]` for each hit's area → `buildRows` → `collectMatches` maps hits to **row** indices
→ cursor := first match ≥ anchor (wrap) → `View` highlights. Enter freezes `pattern`,
`matches`; Esc restores `cursor`, `scrollTop` to the anchor and clears `matches` (expanded
areas stay expanded — the user can see where hits were).

**Data flow, `:`:** Enter → `parseCommand` → validate against `m.items` → writer → refresh →
mode back to list; any error → `m.errMsg` (the existing red footer line), mode back to list
with the file untouched.

**Mode routing invariant:** while `modeSearch` or `modeCommand` is active, *every*
`tea.KeyRunes` goes to the `lineInput`; normal-mode letters (`q`, `u`, `j`, `k`, …) must not
fire. Only Esc, Enter, Tab (command mode), and the editor keys are interpreted.

## 4. Behavior / features

| # | Feature |
| :-- | :-- |
| F1 | Vim motions in the list: `j/k` (= ↓/↑), `h/l` (= ←/→ category pages), `gg`/`G` (first/last row), `ctrl+d`/`ctrl+u` (half of the last rendered body height, min 1), `ctrl+f`/`ctrl+b` (= PgDn/PgUp). Arrows and PgUp/PgDn keep working. `j/k` also move the picker cursor. Every motion re-runs `rescope()` exactly as the arrow keys do. |
| F2 | Help rebind: `?` and F1 open help from the list, picker, and detail views; `h`/`H` no longer open help anywhere. Footer hints, the help overlay, and `gff tui --help` list the new keys. |
| F3 | `/` opens the search prompt: incremental regex compiled per keystroke, smartcase, live highlight; invalid pattern → inline error under the prompt, previous matches kept, input still editable; Esc cancels (cursor/scroll restored, matches cleared); Enter commits (pattern kept for `n/N`). Typed characters never trigger normal-mode keys. |
| F4 | Search haystack and reveal: `path` + `description` of every feature the current page renders — every namespace on the (All) page, the page's component within the breadcrumb namespace on a category page (the same rule `buildRows` uses); areas holding a match are auto-expanded; the cursor jumps to the first match at or after the position where `/` was pressed, wrapping to the top (crossing into another namespace's rows rescopes the breadcrumb exactly as the arrow keys do). |
| F5 | `n`/`N` hop to the next/previous committed match with wraparound; the footer shows `/pattern [i/N]`; a committed pattern with zero matches says `pattern not found`; Esc in the list clears highlights and the `[i/N]` badge but keeps the pattern so `n` re-arms it (vim `:noh`). |
| F6 | `:` opens the command line. Commands: `:set <key> <value>` (bool: `true`/`false`; choice: comma-separated ids), `:unset <key>`, `:q`/`:quit`, `:help`/`:h`, `:/<regex>` (= commit a search). Validation names the bad token; a rejected command writes nothing. Writes use `overrides.Write`/`overrides.Unset`; the row refreshes with `user-override` (or the resolved layer after unset). Esc cancels. |
| F7 | Tab completion in command mode: for the `<key>` argument of `set`/`unset`, Tab cycles the in-scope key paths whose prefix matches, in item order; Shift-Tab cycles backwards; no candidate → no-op; typing again resets the cycle. |
| F8 | Rendering: while a prompt is open it replaces the footer hint line (`/re▌` / `:cmd▌`, error on the next line); the height budget accounts for it so the viewport never overflows; matching rows carry a `*` in the gutter (always — so `NO_COLOR` and tests see it; the cursor's `>` wins on the cursor row) and, with color, a highlight style on the path. |
| F9 | Docs: `cmd/tui.go` Long text, a **Keys** section in `sdk/gff/README.md`, the keymap line in `sdk/gff/AGENTS.md`, and the help overlay all agree on one key table. |

## 5. Evaluation criteria (per feature)

Each rule is a test; the pass column names the proof class.

| Feature | Trigger | Fires | Must not fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1a | `j`/`k` on the list | cursor ±1, viewport follows, `rescope()` runs | on an area row nothing expands | clamp at 0 and `len(rows)-1` | model test with `KeyRunes` |
| F1b | `h`/`l` | page index wraps, cursor/scroll reset, rows rebuilt | when only one page exists → no-op | wrap both directions | model test |
| F1c | `g` then `g` / `G` | first / last row | `g` then any other key → no motion, that key handled normally | `g` then `j` = single down | pending-key test |
| F1d | `ctrl+d/u/f/b` | half/full page stride from `lastInner` (fallback 10/5) | — | list shorter than a page → clamp | model test with `KeyCtrlD` etc. |
| F1e | `j/k` in the picker | picker cursor moves | list cursor unchanged | — | model test |
| F2a | `?` / F1 in list, picker, detail | help overlay opens, `helpReturn` set | — | — | model test per view |
| F2b | `h` / `H` in list, picker, detail | list: page left; picker/detail: nothing | help opens | — | negative test per view (rewires the existing `h` test) |
| F3a | `/` then runes | prompt shows the text; matches recomputed each key | `q`/`u`/`j`/`k` typed in the prompt never quit/unset/move | empty pattern → no matches, no error | mode-routing test |
| F3b | invalid regex (`[ai`) | inline error text, previous matches retained, editing continues | panic / lockout | fixing the pattern clears the error | error-state test |
| F3c | Esc during search | cursor + scrollTop restored to the anchor; matches cleared; mode list | expanded areas collapsing | Esc with an empty prompt | state test |
| F3d | Enter during search | pattern committed; matches + cursor kept; mode list | — | Enter on zero matches → `pattern not found` | state test |
| F3e | smartcase | `claude` matches `Claude CLI`; `Claude` does not match `claude` | — | pattern with only symbols → case-insensitive | table test on `compilePattern` |
| F4a | match inside a collapsed area | that `(ns, area)` expands; others untouched | areas without matches expanding | match on a category page (flat) → no expand needed | model test on the (All) page |
| F4b | haystack | path OR description matches | value/layer/namespace text matching | description empty | table test on `matchItems` |
| F4c | scope | only the rows the current page renders are searched; a page turn recomputes the match set | a category page surfacing another component's or namespace's flags | single-page world | category-page model test |
| F4d | first-match rule | first match at/after the anchor; wraps to top if none after | — | anchor is itself a match → stays | model test |
| F5a | `n`/`N` | next/prev match with wrap; footer `[i/N]` | `n` with no committed pattern → no-op, no error | one match → `n` stays | model test |
| F5b | Esc in the list after a commit | highlights + badge cleared; pattern kept | pattern lost (`n` must re-arm) | — | state test |
| F6a | `:set <bool-key> false` Enter | override written; row shows `false` + `user-override`; mode list | write on a rejected value | `:set k` (missing value) → error names `value` | model test asserting the override file |
| F6b | `:set <choice-key> a,b` on multi / `:set <choice-key> a` on single | selection written | `a,b` on single → rejected naming the key's mode; unknown id → rejected naming the id | duplicate ids | model test |
| F6c | `:unset <key>` | `overrides.Unset` path; row refreshes to the resolved layer | unset of an unknown key → error names the key, no write | key with no override → no-op success | model test |
| F6d | `:q`/`:quit` | `tea.Quit` | — | trailing spaces | model test |
| F6e | `:help`/`:h` | help overlay, `helpReturn = modeList` | — | — | model test |
| F6f | `:/re` Enter | identical end-state to `/re` Enter | — | `:/` empty → no-op | equivalence test |
| F6g | unknown command / bad tokens | `errMsg` = `unknown command: <name>` etc.; mode list; nothing written | partial application | quoted args not supported (documented) | parser table test + fs assertion |
| F7a | Tab on `:set install.ai.` | text becomes `:set install.ai.claude`, Tab again → `.teams`, wraps | completion of the value argument | no candidate → unchanged | completion test |
| F7b | Shift-Tab | previous candidate | — | — | completion test |
| F7c | scope | candidates limited to current page + namespace | — | — | multi-namespace test |
| F8a | prompt open | footer hint replaced by the prompt line; frame height ≤ `m.height` | viewport overflow | tiny height (8) | frame-height test |
| F8b | any match set | matching rows carry `*` in the gutter; the cursor row keeps `>` | `*` on non-matches | `NO_COLOR=1` renders the same markers | View string test under `NO_COLOR` |
| F9 | docs | the four key tables (help overlay, footer, README, `--help`) list the same keys | drift | — | a test that greps the help overlay and the cobra Long text for every F1/F3/F6 key |

## 6. Verification harness

- **Unit (pure):** table tests for `lineInput` (every key), `compilePattern` (smartcase +
  error text), `matchItems` (haystack, scope), `parseCommand` (every command × good/bad
  tokens), `completeKey` (prefix, order, scope).
- **Model-driven (real key shapes):** extend the `keyspace_test.go` style — drive `Model.Update`
  with the exact `tea.KeyMsg` a terminal produces (`KeyRunes` for letters, `KeyCtrlD` …,
  `KeyEsc`, `KeyEnter`, `KeyTab`, `KeyShiftTab`, `KeyBackspace`) and assert on state, the
  rendered `View()` string, and the override file under `t.TempDir()`.
- **Coverage:** `internal/tui` stays ≥ 90% (today 91.3%); the `gff-ci.yml` gate keeps the
  module ≥ 90% overall.
- **Human-evidenced gate:** one real-terminal transcript (tmux capture, as the P3+P4 demo
  did) showing `/`, `n`, Space, `:set`, Tab, `gg`/`G` on the live dotfiles flag inventory,
  committed under `docs/mbo/plans/gff-tui-vim/evidence/demo/`.
- **Lint:** `go vet ./...` (CI); no shell changes.

## 7. Prerequisites / dependencies

- `gff` objective merged (it is — `docs/mbo/index.md` row `gff` = done).
- bubbletea v1.3.10 / lipgloss v1.1.0 already in `sdk/gff/go.mod`; no additions.
- The frozen contracts in `../plans/gff.md` §3 are untouched: no proto, file-format, CLI, or
  shell contract changes. Writes still go only to the user override file.

## 8. Out of scope (and why)

- **`bubbles/textinput`** — a dependency for ~60 lines; the hand-rolled editor is fully tested.
- **Visual selection / batch toggle (`v`, `space` on many rows)** — fleet-tui F7 territory;
  gff toggles are per-key writes and a batch needs a confirm strip. Backlog.
- **Search across pages / namespaces** — chosen against in design (rescoping the breadcrumb
  mid-search is disorienting); the page filter is the scope.
- **Completion of `:set` values, command history, `:g/re/`, `:%s`** — no demand yet.
- **Vim keys inside the detail view** — it has no rows; `?`/F1 and the existing keys suffice.
- **`zo`/`zc` fold keys** — Enter already toggles an area.

## 9. Rollback

Pure additive change inside one package plus docs; revert the PR. No file formats, override
files, or CLI verbs change, so nothing on disk needs migration.

> Produced via `superpowers:brainstorming` (mbo-plan routed: Go CLI under `sdk/`). The matching
> plan is `../plans/gff-tui-vim.md`; registered in `../index.md`.
