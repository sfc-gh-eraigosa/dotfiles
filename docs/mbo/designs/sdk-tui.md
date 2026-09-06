# sdk-tui — shared TUI behaviors for sdk tools — design

- **Slug:** `sdk-tui`
- **Date:** 2026-09-05
- **Status:** Approved (in-chat 2026-09-05)
- **Relates to:** issue #283 · consumer objective `gff-tui-vim` (#281, PR #280) · fleet TUI (spec `../specs/fleet-tui.md`, code in `sdk/fleet/cmd/tui_*.go`) · gsl-ultra config studio (`../specs/gsl-ultra.md` F16) · `sdk/libs` contract `sdk/libs/AGENTS.md`
- **Author(s):** Edward Raigosa (owner) with Claude

## 1. Problem / context

Verified in the repo on 2026-09-05:

- **fleet** ships a full vim TUI on `main` (`sdk/fleet/cmd/tui_keys.go`, `tui_model.go`,
  `tui_view.go`; merged via #232, #237, #254): `j/k`, `gg/G`, `ctrl+d/u/f/b`, `/` smartcase
  regex with live highlight and `n/N`, a `keyHelp` table that renders both the header strip
  and the `?` overlay, an answers form, and a confirm strip. Its tests
  (`tui_model_test.go`) cover smartcase, prompt swallowing motion keys, invalid regex,
  esc/enter, wraparound. Two warts: the `gg` pending state is a **package-level variable**,
  and `routeNormal` is a 300-line string switch.
- **gff**'s TUI (`sdk/gff/internal/tui`) has none of it; the `gff-tui-vim` plan (#280) was
  about to implement the same set a second time, with its own `lineInput`, `searchState`,
  `motionState`, and help table.
- **gsl** has a third bubbletea model (`internal/preview`) and the approved gsl-ultra spec
  adds a config studio TUI.
- `sdk/libs` exists for exactly this ("belongs here once a second tool needs it"), is its own
  module consumed by `replace`, is tested by `make unit-test` with an **80% coverage floor**
  (`scripts/test.sh`), and linted by `make lint-go`. It currently holds only `log`.
- No sdk module depends on `charmbracelet/bubbles`; all three TUIs use bubbletea + lipgloss.

The cost is not just duplicated code: three keymaps drift (fleet binds `l` to the log pane,
gff wants `l` for pages; fleet closes help on any key, gff on Esc), and each copy re-finds
the same bugs (the fleet global, the gff `h`=help collision).

## 2. Goals & non-goals

**Goals**
- One place for the behaviors every sdk TUI needs: keymap (data + dispatch), cursor/viewport
  navigation, a prompt line editor, incremental smartcase search, an ex-style command line
  with completion, and help/confirm overlays.
- A **design guide** (`sdk/libs/tui/GUIDE.md`) that fixes the shared UX so the next TUI is
  consistent by construction.
- gff adopts it first (`gff-tui-vim`), proving the API against a real consumer before fleet
  is ported.

**Non-goals (this objective)**
- A layout/table/theme framework, forms (fleet's answers form), pickers, log panes.
- Porting fleet or building gsl's studio — those are phase 3 objectives.
- Adding `bubbles`; the lib is small enough to own its editor.

## 3. Options considered

1. **Extract-and-adopt now (chosen).** New `sdk/libs/tui` extracted from fleet's proven code
   plus gff's planned pieces, tested in isolation; gff consumes it; fleet and gsl follow.
   Cost: one blocking objective before the gff build. Benefit: the API is shaped by two real
   consumers (fleet's existing behavior, gff's needs) and never a third copy exists.
2. **Build gff as planned, extract later.** Faster for gff alone; creates the third copy, and
   the fleet-tui follow-ups keep adding hand-rolled routing in the meantime. Rejected.
3. **Full TUI framework.** Screens, tables, themes, forms. Nobody needs the rest yet; it
   would freeze decisions (theme engine) that gsl-ultra is still making. Rejected (YAGNI).

## 4. Decision

`sdk/libs/tui` — five packages plus the guide, all in the existing `libs` module (adds one
dependency, bubbletea, to `libs/go.mod`; no lipgloss — rendering goes through a palette
interface).

| Package | Owns | Interface (frozen in the plan §3) | Depends on |
| :-- | :-- | :-- | :-- |
| `keymap` | actions, bindings as bubbletea key strings, ordered table with help text, icon, footer flag; `Vim` default map; `Lookup`, `Merge`, `Without`, `HeaderHint`, `HelpRows`, `Dispatch(handlers)` | the **key mapper** and **function mapper** the owner asked for | bubbletea |
| `nav` | `Cursor{Pos, Len, Top, Height}`: `Move/To/Clamp/Visible`, half/page strides, the `gg` pending state as a field; `Apply(action)` / `Key(msg, map)` | pure cursor + viewport engine (fleet's `clampViewport`/`move`/`moveTo`, gff's `moveCursor`) | keymap |
| `prompt` | `Line`: single-line editor (insert, backspace, delete, ←/→, home/end, ctrl+a/e/u/w), `Render(cursor)` | shared by search, cmdline, and any future form | bubbletea |
| `search` | `State`: incremental smartcase compile (fleet's `compileInto` + `cleanReErr`), match collection over a `hit` closure, first/next/prev with wrap, commit/cancel/hide/re-arm, `[i/N]` badge, anchor for Esc-restore | the `/` behavior | prompt |
| `cmdline` | `Parse`, `Registry` of `Spec{Name, Aliases, Help, Run, Complete}`, `Standard(onHelp, onSearch)` built-ins (`q`/`quit`, `h`/`help`, `/re`), `State` with Tab/Shift-Tab completion | the `:` behavior; tools register domain verbs | prompt |
| `overlay` | `Palette` interface + `Plain`; `Help(palette, title, map, sections...)`; `Confirm{Title, Lines, YesKeys, NoKeys}` with `Key(msg) Decision` and `Render` | help + dialog | keymap |

**Boundaries.** The lib never imports a tool, never touches disk or network, never chooses
colors. Tools keep: their model, their rows/domain types, all writers, the render loop, and
their extra actions. Search's haystack and cmdline's verbs are closures the tool passes in.

**Adoption order.** gff first (the `gff-tui-vim` build worker stacks on this objective's lib
branch, `--base` = the dependency edge). fleet's port and gsl's studio are named follow-ups
(GUIDE §11), each its own objective so this one stays small.

## 5. Risks & blast radius

- **API shaped by one consumer.** Mitigated: the extraction starts from fleet's shipped
  behavior and tests, so two consumers constrain it before fleet is even ported.
- **libs module gains a runtime dependency (bubbletea)** — every libs consumer (gss, tmux-mgr,
  wlink) pulls it into `go.sum`, but Go only links packages actually imported; binaries that
  import only `libs/log` are unchanged. Verified assumption for the plan: `go build` sizes
  before/after recorded in evidence.
- **Coverage floor.** libs is gated at 80%; pure state machines target ≥ 90%. The
  `COVERAGE_ENFORCE=0` default means a regression warns rather than fails — the plan runs the
  check with `COVERAGE_ENFORCE=1` locally.
- **fleet drift.** Until fleet is ported, fleet and the lib can diverge. The guide's canonical
  keymap is copied from fleet's today, and the fleet port is the first phase-3 item.
- Blast radius of this objective itself: additive (new packages, no consumer changes).

## 6. Rollback

Delete `sdk/libs/tui` and the bubbletea requirement from `libs/go.mod`; no consumer exists
until `gff-tui-vim` lands, and that objective's rollback is its own PR revert.

## 7. Evidence expectations

- Per package: `go test -cover` output captured under `docs/mbo/plans/sdk-tui/evidence/<pkg>/`,
  ≥ 90% each; the module run through `scripts/test.sh` with `COVERAGE_ENFORCE=1`.
- `make lint-go` clean for the libs module.
- A **consumer smoke**: a tiny example program under `sdk/libs/tui/example/` (build-tagged
  `example`, not installed) that wires all five packages into a 30-row list, driven by a
  test with real key shapes, and a real-terminal tmux transcript of it under
  `evidence/demo/`. This is the proof the API composes before gff depends on it.
- Binary-size delta of a libs consumer that does not import `tui` (e.g. `gss`) before/after,
  recorded in `evidence/deps/`, to close risk §5 bullet 2.

> Produced via `superpowers:brainstorming` (mbo-plan routed: Go module under `sdk/`, go-team
> interface work). Registered in `../index.md`. Spec: `../specs/sdk-tui.md`.
