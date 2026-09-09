# sdk TUI design guide — `sdk/libs/tui`

> **Read this before writing or changing any bubbletea TUI under `sdk/`.** It is the UX
> contract every sdk tool shares, and `sdk/libs/tui` is the code that makes following it
> cheaper than not. Design record: [`docs/mbo/designs/sdk-tui.md`](../../../docs/mbo/designs/sdk-tui.md).

## 1. Why this exists

fleet, gff, and gsl each grew a bubbletea model. fleet implemented vim motions, `/`
smartcase search, `n`/`N`, a help overlay rendered from a key table, and a confirm dialog;
gff was about to implement the same set a second time; gsl's config studio is next. Same
behaviors, three keymaps, three sets of edge-case bugs. The rule in
[`../AGENTS.md`](../AGENTS.md) is "a package belongs in libs once a **second** tool needs
it" — for TUI behaviors that line was crossed the day gff planned a `/` key.

The goal is not a framework. It is that a user who learned one sdk TUI already knows the
next one, and that a tool author gets navigation, search, prompts, help, and dialogs by
wiring five small packages instead of writing them.

## 2. Principles

1. **Vim is the default grammar.** Motions, `/` search, `n`/`N`, `:` commands, Esc to back
   out. Arrow keys and PgUp/PgDn always work too; they are synonyms, never the only way.
2. **A keymap is data, not a switch statement.** Every key a tool handles is a `keymap.Binding`
   with an action, its keys, its help text, and whether it belongs in the always-visible
   footer. Help overlays, footers, `--help` text, and README key tables **render from that
   one table**; a test pins that they agree.
3. **Mode owns the key.** While a prompt (search, command, form, confirm) is open, every
   typed character is text. Normal-mode letters (`q`, `u`, `j`…) must never fire from inside
   a prompt. Route on mode first, then on key.
4. **Behavior is pure; the tool supplies the edges.** The lib packages do not import the
   tool's model. They own state machines over integers and strings (cursor indices, row
   counts, patterns, match indices) and take closures for "is row `i` a hit", "run this
   command". The tool renders.
5. **Writes stay with the tool.** Nothing in the lib touches disk, network, or the tool's
   domain types. A `:set` in gff calls gff's override writer; a confirm in fleet starts
   fleet's update. The lib only decides *that* the user confirmed.
6. **Total construction, no panics.** An invalid regex is an inline error, an empty list
   clamps to nothing, a zero height falls back to a sane stride. A TUI helper must never be
   the reason a tool dies.

## 3. The canonical keymap (`keymap.Vim`)

| Action | Keys | Footer? | Meaning |
| :-- | :-- | :-- | :-- |
| `Up` / `Down` | `k` `↑` / `j` `↓` | yes | move the cursor |
| `PageLeft` / `PageRight` | `h` `←` / `l` `→` | yes | lateral axis: category pages, tabs, panes. A tool with no lateral axis **rebinds or removes** these via `Merge`/`Without` and says so in its help |
| `Top` / `Bottom` | `gg` / `G` | no | first / last row |
| `HalfUp` / `HalfDown` | `ctrl+u` / `ctrl+d` | no | half a page |
| `PageUp` / `PageDown` | `ctrl+b` `pgup` / `ctrl+f` `pgdown` | no | a full page |
| `Search` | `/` | yes | open the search prompt |
| `NextMatch` / `PrevMatch` | `n` / `N` | yes | hop between matches, wrapping |
| `ClearHighlight` | `esc` | no | vim `:noh`: hide highlights, keep the pattern |
| `Command` | `:` | yes | open the command line |
| `Help` | `?` `f1` | yes | help overlay (**never** `h` — it is a motion) |
| `Quit` | `q` `ctrl+c` | yes | quit |
| `Select` | `space` | tool | the row's primary action (toggle, select, pick) |
| `Confirm` / `Back` | `enter` / `esc` | tool | open / drill in; back out |

Keys are bubbletea `KeyMsg.String()` names (`"ctrl+d"`, `"pgdown"`, `"f1"`, `" "` for
space is written `"space"`). `gg` is the one two-key chord; `nav.Cursor` owns its pending
state — never a package-level variable.

Tools add their own actions (`Toggle`, `Refresh`, `SSH`, `Unset`…) with `Merge`. Prefer
lowercase single letters for frequent actions, uppercase for the destructive or rare
variant (`u` unset / `U` unset all), and keep `q`, `?`, `/`, `:`, `n`, `N`, `j`, `k`, `g`,
`G` untouched.

## 4. Modes and routing

```
normal ──/──▶ search ──enter/esc──▶ normal
normal ──:──▶ command ─enter/esc──▶ normal
normal ──?──▶ help ────any key────▶ normal   (or the view it came from)
normal ─act─▶ confirm ─y/enter|n/esc▶ normal
```

- The model keeps **one** `mode` field and dispatches on it first (`switch m.mode`).
- Prompts (`search.State`, `cmdline.State`) expose `Key(msg)` that consumes editing keys
  and reports `Submitted`/`Cancelled`; the tool never looks at runes itself.
- Help returns to the view it was opened from (`helpReturn`), so `?` from a picker goes
  back to the picker.
- Esc is layered: in a prompt it cancels the prompt; in a dialog it declines; in normal mode
  it clears highlights/selection and nothing else. Esc **never quits**.

## 5. Search semantics (`search.State`)

- Incremental: the pattern recompiles on every keystroke; matches update live.
- **Smartcase**: an all-lowercase pattern is case-insensitive; one uppercase letter makes
  it exact-case. Empty pattern = no matches, no error.
- Invalid regex: inline error under the prompt (`bad pattern: missing closing ]`), the
  **previous** match set stays on screen, editing continues. Never a panic, never a lockout.
- The haystack is the tool's choice, given as `hit func(i int) bool` over the rendered rows.
  Default recommendation: the row's visible text (path/alias **and** description) — what
  the user can see is what they can search.
- Cursor: on typing, park on the first match at or after where `/` was pressed, wrapping;
  Enter commits (pattern kept for `n`/`N`, `[i/N]` badge in the footer); Esc cancels and
  restores cursor + scroll. Zero matches on commit says `pattern not found: <pattern>`.
- `n`/`N` wrap in both directions. `n` after `:noh` re-arms the last pattern.
- Tools with collapsible groups **reveal** matches (expand the group holding a hit) rather
  than hiding them; a match the user cannot reach is a bug.

## 6. Command line (`cmdline`)

- `:` opens it; Enter runs, Esc cancels, Tab / Shift-Tab cycle completions for the argument
  the cursor is on; typing resets the cycle.
- Standard commands every tool registers via `cmdline.Standard`: `q`/`quit`, `h`/`help`,
  and `/<regex>` (search alias — identical end state to pressing `/`).
- Tool commands mirror the CLI verbs (`:set <key> <value>` ≙ `gff set`). Validation names
  the offending token (`value for install.ai.claude must be true or false, got "maybe"`);
  a rejected command changes nothing.
- Errors go to the tool's existing error line; the mode is already back to normal.

## 7. Help, footer, dialogs (`overlay`)

- **Footer hint** (normal mode): `keymap.Map.HeaderHint` — only `Header: true` bindings, in
  table order, e.g. `j/k move  h/l page  / search  n/N next  : command  ? help  q quit`.
  While a prompt is open the footer **is** the prompt line (plus an error line beneath).
- **Help overlay** (`overlay.Help`): title, the full keymap (every binding, icon optional),
  then tool sections (gff adds SOURCES; fleet adds nothing). Closes on `esc`/`?`/`q`/enter,
  or any key if the tool prefers (fleet does) — say which in the footer line of the overlay.
- **Confirm** (`overlay.Confirm`): a titled panel listing the consequential facts (targets,
  refs, values), then `enter/y = do it · esc/n = cancel`; anything else declines. Declining
  must change nothing. Use it for every irreversible or multi-target action.
- Palette: the lib renders through a tiny `overlay.Palette` interface (`Dim`, `Bold`,
  `Accent`, `Err`); tools adapt their lipgloss theme. `overlay.Plain` is the `NO_COLOR` /
  test palette.

## 8. Rendering rules

- The cursor row is marked `> `; matching rows `* `; the cursor wins on its own row. These
  markers exist **without color**, so `NO_COLOR=1` and tests see the same structure.
- The viewport follows the cursor (`nav.Cursor.Clamp`); overflow shows `… N more above/below`.
- Fixed chrome (title, breadcrumb, footer, error line) is counted in the height budget so a
  prompt never pushes the list off screen. Minimum budget is one row.
- Provenance / status colors come from the tool's theme; the lib does not choose colors.

## 9. Testing rules

- Drive `Model.Update` with the **real** key shapes a terminal produces: letters are
  `tea.KeyRunes`, the spacebar is `tea.KeySpace`, control keys are `tea.KeyCtrlD`…, Esc is
  `tea.KeyEscape`. `KeyRunes{' '}` never happens in a real terminal.
- Pin the lipgloss profile to ASCII in tests (`lipgloss.SetColorProfile(termenv.Ascii)`) or
  set `NO_COLOR=1` so assertions read plain text.
- Every lib package: table tests over its state machine, ≥ 90% line coverage (the libs
  module floor is 80% in `scripts/test.sh`; TUI state machines are pure and can do better).
- Every tool: one test per keymap entry that the key does what the help text says, one
  mode-routing test per prompt ("typing `qujk` inside the prompt quits nothing"), one test
  that the footer/overlay/`--help`/README key tables agree.
- Evidence: a real-terminal transcript (tmux `capture-pane`, never backgrounded) for any
  change to keys or prompts, committed under the objective's `evidence/demo/`.

## 10. Adopting the lib in a tool (checklist)

1. `require github.com/sfc-gh-eraigosa/dotfiles/sdk/libs v0.0.0` + `replace … => ../libs`.
2. Build your keymap: `keymap.Vim.Merge(yourBindings...)`, `.Without(...)` for actions you
   truly do not have. Keep `keymap.Vim`'s keys for the actions you keep.
3. Replace hand-rolled cursor/viewport math with `nav.Cursor`; call `SetLen`/`SetHeight`
   whenever rows or the window change.
4. Replace the search state with `search.State`; supply the `hit` closure; reveal matches.
5. Register commands with `cmdline.Registry` (`cmdline.Standard(...)` first).
6. Render help with `overlay.Help(palette, title, keymap, sections...)`; footer with
   `HeaderHint`; confirmations with `overlay.Confirm`.
7. Add the key-table agreement test and the mode-routing tests (§9).
8. Update the tool's README key table and its cobra `Long` from the same keymap.

## 11. Roadmap (owned by `docs/mbo/index.md`)

| Phase | Objective | Status |
| :-- | :-- | :-- |
| 1 | `sdk-tui` — this lib + guide | see index |
| 2 | `gff-tui-vim` — first consumer (vim nav, `/`, `:`) | see index |
| 3 | `fleet` TUI port onto the lib (delete `cmd/tui_keys.go` routing, keep fleet's actions) · gsl-ultra config studio built on it from day one | idea |
