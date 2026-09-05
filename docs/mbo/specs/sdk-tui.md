# sdk-tui — shared TUI behaviors for sdk tools — spec

- **Slug:** `sdk-tui`
- **Date:** 2026-09-05
- **Status:** Approved
- **Relates to:** design `../designs/sdk-tui.md` · guide `sdk/libs/tui/GUIDE.md` · consumer `gff-tui-vim` (#281 / #280) · fleet TUI code (`sdk/fleet/cmd/tui_*.go`)

## 1. Goal

A tool author wires five small packages from `sdk/libs/tui` and gets, without writing them:
a data-driven keymap that renders its own footer and help, vim cursor/viewport navigation,
a prompt editor, incremental smartcase `/` search with `n`/`N`, an ex-style `:` command line
with completion, and help/confirm overlays. A user who learned `fleet tui` already knows
`gff tui`. gff adopts it first; the guide makes the UX the default for every TUI after.

## 2. Use cases

**UC1 — gff adopts the lib.** gff's model / replaces its planned `lineInput`, `searchState`,
`motionState`, and help table with `prompt.Line`, `search.State`, `nav.Cursor`,
`keymap.Vim.Merge(...)`, `overlay.Help` / acceptance: gff's `gff-tui-vim` spec F1–F9 pass with
gff owning only glue (auto-expand, `inScope`, row rendering, `:set` validation and writers).

**UC2 — a tool without a lateral axis.** fleet (later) / `keymap.Vim.Without(PageLeft,
PageRight).Merge(Binding{Action: "log", Keys: ["l"], …})` / acceptance: `l` opens the log
pane, `h` does nothing, the help overlay shows `l` with fleet's text and no `h/l` row.

**UC3 — the same search everywhere.** Any tool / user types `/nano|pi`, then `[`, then `]`,
Enter, `n`, `n`, Esc, `n` / acceptance: live matches, inline `bad pattern` with the previous
matches kept, commit with `[1/2]`, wrap, `:noh`, re-arm — identical in every consumer
because the state machine is the lib's.

**UC4 — commands mirror the CLI.** gff / registers `set` and `unset` with `Run` and
`Complete` closures; user types `:set install.ai.` Tab Tab Enter / acceptance: completion
cycles gff's in-scope keys, `Run` receives `["install.ai.teams"]`, gff's own validation error
lands in gff's error line; the lib never saw a flag.

**UC5 — confirm before a wave.** fleet (later) / `overlay.Confirm{Title: "update 3 hosts →
main", Lines: targets}` / acceptance: `enter`/`y` → `Yes`, `esc`/`n`/anything → `No`; the
render lists the targets and the two choices.

**UC6 — one key table, four renderings.** Any tool / a test iterates `keymap.Map` and checks
the footer, the help overlay, the cobra `Long`, and the README table / acceptance: adding a
binding to the map is the only edit needed to make it discoverable everywhere.

## 3. Architecture

Module `sdk/libs` (existing). New tree:

```
sdk/libs/tui/
  GUIDE.md          the design guide (already written in the design PR)
  doc.go            package tui: overview + links (no code)
  keymap/           Action, Binding, Map, Vim, Lookup/Merge/Without/HeaderHint/HelpRows, Handlers, Dispatch
  nav/              Cursor (Pos, Len, Top, Height, pendingG), Move/To/Clamp/Visible/Half/Page, Apply, Key
  prompt/           Line editor
  search/           State (Input prompt.Line, Pattern, Re, Err, Matches, Visible, Anchor*), Compile, Type, Collect, First, Next, Commit, Cancel, Hide, Rearm, Badge, IsMatch
  cmdline/          Command, Parse, Spec, Registry, Standard, State (Input, completion), Key → Event, Complete
  overlay/          Palette, Plain, Help, Section, Confirm, Decision
  example/          build-tagged `example` program wiring all five (evidence only, not installed)
```

Dependency direction: `overlay → keymap`; `nav → keymap`; `search → prompt`; `cmdline → prompt`;
everything → bubbletea (`tea.KeyMsg` only). No lipgloss, no tool imports, no I/O.

**Key naming contract:** every key is the bubbletea `KeyMsg.String()` name; the lib canonicalizes
`" "` to `"space"`. `gg` is a chord owned by `nav.Cursor`.

**Composition contract (how a tool uses it):**

```go
// in Update:
switch m.mode {
case modeSearch:
    ev := m.search.Key(msg)                     // Typed | Submitted | Cancelled | Ignored
    …tool reveals matches, collects via m.search.Collect(len(rows), hit), moves cursor…
case modeCommand:
    ev := m.cmd.Key(msg, &m.registry)           // Typed | Submitted{Command} | Cancelled | Ignored
    …on Submitted: cmd, err := m.registry.Run(ev.Command); m.errMsg = err…
default:
    if m.cur.Key(msg, m.keys) { … }             // vim motions incl. gg
    if handled, cmd := keymap.Dispatch(m.keys, msg, m.handlers); handled { return m, cmd }
}
```

## 4. Behavior / features

| # | Feature |
| :-- | :-- |
| F1 | `keymap`: `Binding{Action, Keys, Help, Icon, Header}`; `Map` is ordered; `Lookup(msg)` matches on `msg.String()` (space canonicalized); `Merge` replaces by action or appends; `Without` removes; `HeaderHint(sep)` joins `Header` bindings as `keys help`; `HelpRows()`; `Vim` is the canonical map from GUIDE §3; `Dispatch(map, msg, Handlers)` calls the handler for the looked-up action and reports `handled`. |
| F2 | `nav.Cursor`: `SetLen`, `SetHeight`, `Move(delta)`, `To(i)` clamp to `[0, Len-1]` (or 0 when empty) and call `Clamp` so `Top ≤ Pos < Top+Height`; `Half()` = max(1, Height/2), fallback 5 when Height ≤ 0; `Page()` = Height, fallback 10; `Visible()` = `[Top, min(Top+Height, Len))`; `Apply(action)` handles Up/Down/Top/Bottom/HalfUp/HalfDown/PageUp/PageDown; `Key(msg, map)` handles the `gg` chord: `g` sets pending, a second `g` = Top, any other key clears pending and is **not** consumed (the caller handles it). |
| F3 | `prompt.Line`: insert runes / space, backspace, delete, ←/→, home/end, ctrl+a/e, ctrl+u (kill to start), ctrl+w (kill word); `Handle` reports consumed; Esc/Enter/Tab/Shift-Tab/Up/Down are **not** consumed; `Render(cursor)` inserts the cursor glyph; `String`, `Reset`, `SetText`. |
| F4 | `search.Compile`: `""` → `nil, nil`; smartcase (`(?i)` when the pattern has no uppercase letter); errors drop Go's `error parsing regexp: ` prefix (`missing closing ]: `[ai``). |
| F5 | `search.State` live loop: `Start(cursor, top)` records the anchor and clears; `Key(msg)` → `Typed` (recompiled; on error `Err` set and `Re` **unchanged**), `Submitted`, `Cancelled`, `Ignored`; `Collect(n, hit)` fills `Matches` (ascending) when `Re != nil && Visible`; `First(from)` = first match ≥ from, wrap, `ok=false` when none; `Next(cursor, dir)` = strictly after/before with wrap; `Commit()` keeps `Pattern`, clears `Input`, returns `(committed bool, notFound bool)` — an outstanding `Err` refuses to commit and returns it; `Cancel()` returns the anchor `(cursor, top)` and clears everything but `Pattern`; `Hide()` = `:noh`; `Rearm()` recompiles `Pattern`; `Badge(cursor)` = `"/pat [i/N]"` (`i`=`-` off a match, empty when hidden); `IsMatch(i)`. |
| F6 | `cmdline.Parse`: trims; empty → `ErrEmpty`; `/…` → `Command{Name: "search", Args: [rest]}` (rest keeps spaces); otherwise `Fields`. `Registry.Register(Spec)`, `Run(Command)` resolves name or alias → `Spec.Run(args)`; unknown → `unknown command: <name>`. `Standard(onHelp func(), onSearch func(pattern string))` returns the `q`/`quit`, `h`/`help`, `search` specs. |
| F7 | `cmdline.State`: `Key(msg, reg)` → `Typed` (resets completion), `Submitted{Command}` (parsed; `ErrEmpty` → `Cancelled`), `Cancelled` (Esc), `Ignored`; Tab / Shift-Tab call `Complete(±1, reg)`: finds the spec by the first token, asks `Spec.Complete(argIdx, prefix)` for the token under the cursor **only when the cursor is at the end of the line**, cycles with wrap, a trailing space means "next argument, empty prefix"; no candidates → no change. |
| F8 | `overlay.Help(p, title, m, sections...)`: title line, one row per binding `  <icon> <keys padded 18> <help>` (icon omitted when empty), then each `Section{Title, Lines}`, then a closing hint line passed by the caller (default `esc/?/q close`). `overlay.Plain` returns text unchanged. |
| F9 | `overlay.Confirm{Title, Lines, YesKeys, NoKeys}`: `Key(msg)` → `Yes` for a yes key, `No` for a no key **or any other key** (declining is the safe default), `Pending` never (single-key dialog); defaults `enter`,`y` / `esc`,`n`; `Render(p, width)` = title, lines, `enter/y <yes label> · esc/n cancel`. |
| F10 | `example/`: a build-tagged program with 30 rows wiring all five packages; its test drives real keys through it (`/`, `n`, `:`, `?`, confirm) and is the API's composition proof. |
| F11 | Docs: `sdk/libs/AGENTS.md` package table row, `sdk/libs/tui/doc.go`, GUIDE §11 roadmap; `sdk/AGENTS.md` "Conventions" gains one line pointing TUIs at the guide. |

## 5. Evaluation criteria (per feature)

| Feature | Trigger | Fires | Must not fire | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- | :-- |
| F1a | `Lookup` on `ctrl+d`, `j`, `space`, `f1`, `pgdown` | the bound action | an unbound key | `KeySpace` and `KeyRunes{' '}` both → `space` | table test |
| F1b | `Merge` with an existing action | binding replaced in place (order kept) | duplicates | merge a new action → appended | test |
| F1c | `Without(PageLeft, PageRight)` | rows gone; `Lookup("h")` → false | other rows shifting | remove an absent action → no-op | test |
| F1d | `HeaderHint` | only `Header` rows, table order, `keys help` joined by sep | non-header rows | empty map → `""` | test |
| F1e | `Dispatch` | handler called once, `handled=true`, its `tea.Cmd` returned | handler for an unbound key | action bound but no handler → `handled=false` | test |
| F2a | `Move(+1)` at the end, `Move(-1)` at 0 | clamped | wrap | `Len=0` → `Pos=0`, no panic | table test |
| F2b | `Clamp` after `To(i)` | `Top ≤ Pos < Top+Height`; `Top ≤ max(0, Len-Height)` | header scrolled away | `Height ≤ 0` → `Top=0` | fleet's clampViewport cases |
| F2c | `Half/Page` with `Height=0` | 5 / 10 | 0 stride | `Height=1` → 1 / 1 | test |
| F2d | `Key` with `g`,`g` / `g`,`j` / `G` | Top / pending cleared and `j` **not consumed** / Bottom | a lone `g` moving | `g`,`g` on an empty list | test |
| F3 | every editor key | per GUIDE; `Handle=false` for mode keys | — | unicode runes | table test (moved from the gff plan Task 1) |
| F4 | `claude` vs `Claude CLI`; `Claude` vs `claude`; `[ai`; `""` | smartcase; error `missing closing ]`; nil | — | symbols-only pattern → case-insensitive | table test |
| F5a | `Key` with runes | `Typed`, `Matches` recomputed by the caller's next `Collect` | motion keys acting | invalid pattern keeps old `Re` | test |
| F5b | `Commit` with `Err != ""` | `committed=false`, error kept for the caller | pattern overwritten | `Commit` on empty input → hides | test |
| F5c | `First(from)` / `Next(cur, ±1)` | wrap both ways; `ok=false` on no matches | moving on no matches | single match → `Next` stays | test |
| F5d | `Cancel` | anchor returned; `Input`, `Re`, `Matches`, `Visible` cleared; `Pattern` kept | — | Cancel before Start → zeros | test |
| F5e | `Hide` then `Rearm` | `Visible` false → true with `Re` from `Pattern` | `Pattern` lost | `Rearm` with empty pattern → no-op | test |
| F5f | `Badge(cursor)` | `/ai [2/3]`; `[-/3]` off a match; `""` when hidden | — | `[0/0]` never (notFound reported instead) | test |
| F6a | `Parse` table | per F6 | — | `"/"` → `search` with `""` | table test (moved from gff plan Task 5) |
| F6b | `Run` on alias `quit` | the `q` spec's `Run` | — | unknown → error names it | test |
| F6c | `Standard` | `q` returns `tea.Quit`; `h` calls `onHelp`; `search` calls `onSearch(pattern)` with an empty pattern **not** forwarded | — | — | test |
| F7a | Tab on `set install.ai.` with `Complete` returning `[claude, teams]` | cycles, wraps; Shift-Tab reverses | value position completing | cursor not at the end → no completion | test |
| F7b | typing after Tab | cycle reset (next Tab recomputes) | stale candidates | — | test |
| F7c | Enter on empty / `:` only | `Cancelled` | `Submitted` with empty name | whitespace-only | test |
| F8 | `Help` with icons and without; two sections | rows padded; sections in order; closing hint last | color codes (Plain) | empty map → title + hint only | golden string test |
| F9 | `Confirm.Key` with `y`,`enter`,`n`,`esc`,`x` | Yes, Yes, No, No, No | Yes on anything not in `YesKeys` | custom `YesKeys` | table test |
| F10 | example test | `/` finds a row, `n` wraps, `:q` quits, `?` shows every binding, confirm declines on `x` | — | — | `example_test.go` (build tag `example`) |
| F11 | docs | AGENTS row present; `go vet` + `make lint-go` clean; `scripts/test.sh unit` with `COVERAGE_ENFORCE=1` passes for `libs` | — | — | CI + evidence |

## 6. Verification harness

- Package unit tests (all pure; real key shapes; no lipgloss) — each package ≥ 90%.
- `cd sdk/libs && go test ./... -cover`; `COVERAGE_ENFORCE=1 ./scripts/test.sh unit` from the
  repo root (libs floor 80%); `make lint-go`.
- The `example` composition test (build tag) is run in Task 7 and its transcript captured
  from a real terminal via tmux under `evidence/demo/`.
- Consumer proof is the `gff-tui-vim` build, which stacks on this branch.

## 7. Prerequisites / dependencies

- `sdk/libs/go.mod`: add `github.com/charmbracelet/bubbletea v1.3.10` (the version gff and
  fleet already pin); `go 1.24` directive stays (nothing newer than `strings.ContainsFunc`, 1.21).
- None on other objectives. `gff-tui-vim` depends on this one.

## 8. Out of scope (and why)

- Forms (fleet's answers form), pickers (gff's), log panes, tables, layouts, themes — one
  consumer each or still in flux (gsl-ultra's style engine).
- Porting fleet; building gsl's studio — phase 3, separate objectives so this one stays small.
- Mouse support, key rebinding via config files — no demand.

## 9. Rollback

Remove `sdk/libs/tui` and the bubbletea requirement; no consumer until `gff-tui-vim` lands.

> Produced via `superpowers:brainstorming`. Plan: `../plans/sdk-tui.md`. Register / update `../index.md`.
