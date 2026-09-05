# gff TUI — vim navigation, `/` regex search, `:` command line — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The execution trio lives in [`./gff-tui-vim/`](./gff-tui-vim/).

- **Slug:** `gff-tui-vim`
- **Date:** 2026-09-05
- **Status:** Approved
- **Relates to:** spec [`../specs/gff-tui-vim.md`](../specs/gff-tui-vim.md) · parent `gff` plan [`./gff.md`](./gff.md) P3-T1 · feature `gss feature gff-tui-vim` · issue/PR: see `../index.md`

**Goal:** Make `gff tui` navigable with vim keys, searchable with `/` (incremental regex, `n`/`N`), and scriptable with a `:` command line (`:set`, `:unset`, `:q`, `:help`, `:/re`, Tab completion), so any flag can be found and toggled in a few keystrokes.

**Architecture:** Four small new files inside `sdk/gff/internal/tui` (`input.go` line editor, `search.go` search state + pure matchers, `command.go` parser/executor/completion, `keys.go` vim motions) plug into the existing bubbletea `Model` through two new screen modes (`modeSearch`, `modeCommand`). Writes keep flowing through `overrides.Write` / `overrides.Unset` and rows refresh through the existing `Explain` hook. No new dependencies.

**Tech Stack:** Go 1.26 (module `sdk/gff`), bubbletea v1.3.10, lipgloss v1.1.0, testify; tests drive `Model.Update` with real terminal key shapes (the `keyspace_test.go` pattern).

**Spec:** [`../specs/gff-tui-vim.md`](../specs/gff-tui-vim.md)

## Global constraints

- Coverage: `sdk/gff` module ≥ 90% overall (gate in `.github/workflows/gff-ci.yml`); `internal/tui` must not drop below its current 91.3%.
- Frozen contracts in [`./gff.md`](./gff.md) §3 (proto, file formats, CLI verbs, shell helper) are **not** touched. gff writes only the user override file; every test writes only under `t.TempDir()`.
- No new Go module dependency (`go.mod` unchanged).
- Real key shapes in tests: letters are `tea.KeyRunes`, the spacebar is `tea.KeySpace`, control keys are `tea.KeyCtrlD` etc., Esc is `tea.KeyEscape`.
- `h`/`H` must not open help anywhere after Task 2. `?` and `F1` do.
- Stage files by explicit name; one commit per task; commit messages exactly as written in each task; every commit ends with the session trailers required by the harness.
- Evidence: each task's gate output is `tee`'d into `docs/mbo/plans/gff-tui-vim/evidence/<task>/` and committed with the task.

---

## 1. Summary & verdict

Additive change to one package plus docs. Design approved in chat on 2026-09-05 with three decisions: `:` is an ex-style command line; vim `h`/`l` win over the old `h`=help binding; `/` auto-expands collapsed areas holding a match. Risk is low (no on-disk format changes); the only behavior a user might notice as a regression is `h` no longer opening help, which the footer, help overlay, README, and `--help` all announce.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `sdk/gff/internal/tui/input.go` | `lineInput` single-line editor (insert/backspace/delete/←→/home/end/ctrl+u/ctrl+w; `Render()` with a cursor block) | spec §3 unit 1, F3, F6 |
| `sdk/gff/internal/tui/input_test.go` (package `tui`) | table tests for every editor key | F3a, F6 |
| `sdk/gff/internal/tui/keys.go` | `motionState`, `(m *Model) handleMotion`, `moveCursor`, `turnPage`, `fullPage`, `halfPage` | F1 |
| `sdk/gff/internal/tui/search.go` | `searchState`, `compilePattern` (smartcase), `matchItem`, `rowKey`, `(m *Model) startSearch/applySearch/commitSearch/cancelSearch/collectMatches/nextMatch/clearHighlights` | F3, F4, F5 |
| `sdk/gff/internal/tui/search_internal_test.go` (package `tui`) | pure tests for `compilePattern`, `matchItem`, `rowKey` | F3e, F4b |
| `sdk/gff/internal/tui/command.go` | `command`, `parseCommand`, `parseValue`, `(m *Model) findKey/execCommand/cmdSet/cmdUnset/completeKey/completeCommand` | F6, F7 |
| `sdk/gff/internal/tui/command_internal_test.go` (package `tui`) | table tests for `parseCommand`, `parseValue` | F6g, F6a–c validation |
| `sdk/gff/internal/tui/model.go` | new modes + dispatch; `inScope`; motions wired into `updateList`/`updatePicker`; `?`/F1 help; `/`, `:`, `n`, `N`, Esc(`:noh`) | F1, F2, F3, F5, F6 |
| `sdk/gff/internal/tui/view.go` | prompt line replaces the footer hint; `[i/N]` badge; match gutter `*` + style; new footer hint; help overlay key table | F2, F5, F8, F9 |
| `sdk/gff/internal/tui/vim_test.go` (package `tui_test`) | model-driven tests: motions, help rebind, picker `j/k` | F1, F2 |
| `sdk/gff/internal/tui/search_test.go` (package `tui_test`) | model-driven tests: `/` flow, auto-expand, `n/N`, Esc, errors, NO_COLOR gutter, frame height | F3, F4, F5, F8 |
| `sdk/gff/internal/tui/command_test.go` (package `tui_test`) | model-driven tests: `:set`/`:unset`/`:q`/`:help`/`:/re`, Tab/Shift-Tab, file assertions | F6, F7 |
| `sdk/gff/internal/tui/round2_test.go` | rewire `TestHelpOverlayFromDetail` from `h` to `?` | F2b |
| `sdk/gff/cmd/tui.go` | `Long` text lists the key table | F9 |
| `sdk/gff/cmd/tui_keys_test.go` (package `cmd`) | asserts `tuiCmd.Long` names every key group | F9 |
| `sdk/gff/README.md` | new **TUI keys** section | F9 |
| `sdk/gff/AGENTS.md` | keymap line under `internal/tui` | F9 |
| `docs/mbo/plans/gff-tui-vim/evidence/**` | gate outputs + the real-terminal demo transcript | spec §6 |
| `docs/mbo/index.md` | state transitions | — |

## 3. Interface contracts

These are what neighboring tasks import from each other. Names are final.

```go
// input.go
type lineInput struct{ runes []rune; pos int }
func (l *lineInput) String() string
func (l *lineInput) Reset()
func (l *lineInput) SetText(s string)          // cursor to end
func (l *lineInput) Handle(msg tea.KeyMsg) bool // true = consumed (an editing key)
func (l *lineInput) Render() string             // text with "▌" at the cursor

// keys.go
type motionState struct{ pendingG bool }
func (m *Model) handleMotion(msg tea.KeyMsg) bool // true = consumed
func (m *Model) moveCursor(delta int)             // clamps, then rescope()
func (m *Model) turnPage(dir int)                 // wraps; resets cursor/scroll; buildRows()
func (m *Model) fullPage() int                    // lastInner, fallback 10
func (m *Model) halfPage() int                    // max(1, fullPage()/2)

// search.go
type searchState struct {
    input     lineInput
    pattern   string          // committed pattern for n/N
    re        *regexp.Regexp  // live (search mode) or committed
    err       string          // live compile error
    matches   []int           // row indices, ascending
    visible   bool            // highlights + badge shown
    anchorKey string          // rowKey of the cursor row when / was pressed
    anchorTop int             // scrollTop to restore on Esc
}
func compilePattern(p string) (*regexp.Regexp, error) // "" → nil,nil; smartcase
func matchItem(item resolve.Resolved, re *regexp.Regexp) bool // path OR description
func rowKey(r row) string
func (m *Model) inScope(item resolve.Resolved) bool // the buildRows visibility rule
func (m *Model) rowIndexOf(key string) int          // 0 when absent
func (m *Model) startSearch()                        // anchors + modeSearch
func (m *Model) applySearch()                        // compile live text; expand; collect; move cursor
func (m *Model) collectMatches()                     // rows → matches (no cursor move); called by buildRows
func (m *Model) commitSearch()                       // Enter
func (m *Model) cancelSearch()                       // Esc in search mode
func (m *Model) nextMatch(dir int)                   // n = +1, N = -1
func (m *Model) clearHighlights()                    // Esc in list (:noh)
func (m *Model) matchBadge() string                  // "/pat [i/N]" or ""

// command.go
type command struct{ name string; args []string }
type completion struct{ head string; candidates []string; idx int }
func parseCommand(line string) (command, error)
func parseValue(item resolve.Resolved, raw string) (*gffv1.Value, error)
func (m *Model) findKey(key string) (int, error)     // "ns:path" or "path"; scope-preferred
func (m *Model) execCommand(c command) tea.Cmd       // errors → m.errMsg
func (m *Model) completeKey(prefix string) []string  // in-scope paths, item order
func (m *Model) completeCommand(dir int)             // Tab=+1, Shift-Tab=-1

// model.go additions
const ( modeSearch screenMode = iota + 4 /* after modeHelp */; modeCommand )
type Model struct { …existing…; motion motionState; search searchState; cmd lineInput; comp completion }
func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd)
func (m *Model) updateCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd)
```

Footer hint (list, no prompt open), the single source for F9 — copy verbatim wherever it is shown:

```
j/k ↑/↓ move  h/l ←/→ page  gg/G top/end  ^d/^u half  / search  n/N next  : command  Enter open  Space toggle  u clear  ? help  q quit
```

## 4. TDD build order

Run every command from `sdk/gff/` (the module root). `EV=../../docs/mbo/plans/gff-tui-vim/evidence` is the evidence root; create the task folder before the `tee`.

### Task 1: `lineInput` line editor

**Files:**
- Create: `sdk/gff/internal/tui/input.go`
- Test: `sdk/gff/internal/tui/input_test.go` (package `tui`)

**Interfaces:**
- Consumes: bubbletea key types only.
- Produces: `lineInput` per §3 — used by Task 4 (search prompt) and Task 6 (command prompt).

- [ ] **Step 1: Write the failing test**

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func TestLineInputEditing(t *testing.T) {
	cases := []struct {
		name string
		keys []tea.KeyMsg
		want string
		pos  int
	}{
		{"insert", []tea.KeyMsg{runes("ab"), runes("c")}, "abc", 3},
		{"space is a rune", []tea.KeyMsg{runes("a"), key(tea.KeySpace), runes("b")}, "a b", 3},
		{"backspace", []tea.KeyMsg{runes("abc"), key(tea.KeyBackspace)}, "ab", 2},
		{"backspace at start is a no-op", []tea.KeyMsg{key(tea.KeyBackspace)}, "", 0},
		{"left then insert", []tea.KeyMsg{runes("ac"), key(tea.KeyLeft), runes("b")}, "abc", 2},
		{"delete under cursor", []tea.KeyMsg{runes("abc"), key(tea.KeyHome), key(tea.KeyDelete)}, "bc", 0},
		{"home/end", []tea.KeyMsg{runes("abc"), key(tea.KeyHome), runes("x"), key(tea.KeyEnd), runes("y")}, "xabcy", 5},
		{"ctrl+a / ctrl+e", []tea.KeyMsg{runes("abc"), key(tea.KeyCtrlA), runes("x"), key(tea.KeyCtrlE), runes("y")}, "xabcy", 5},
		{"ctrl+u kills to start", []tea.KeyMsg{runes("abc def"), key(tea.KeyLeft), key(tea.KeyCtrlU)}, "f", 0},
		{"ctrl+w kills a word", []tea.KeyMsg{runes("set install.ai "), key(tea.KeyCtrlW)}, "set ", 4},
		{"right clamps", []tea.KeyMsg{runes("a"), key(tea.KeyRight), key(tea.KeyRight), runes("b")}, "ab", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var l lineInput
			for _, k := range tc.keys {
				assert.True(t, l.Handle(k), "editing keys are consumed")
			}
			assert.Equal(t, tc.want, l.String())
			assert.Equal(t, tc.pos, l.pos)
		})
	}
}

func TestLineInputDoesNotConsumeModeKeys(t *testing.T) {
	var l lineInput
	for _, k := range []tea.KeyType{tea.KeyEscape, tea.KeyEnter, tea.KeyTab, tea.KeyShiftTab, tea.KeyUp, tea.KeyDown} {
		assert.False(t, l.Handle(key(k)), "%v belongs to the owning mode", k)
	}
}

func TestLineInputRenderAndReset(t *testing.T) {
	var l lineInput
	l.SetText("abc")
	l.Handle(key(tea.KeyLeft))
	assert.Equal(t, "ab▌c", l.Render())
	l.Reset()
	assert.Equal(t, "", l.String())
	assert.Equal(t, "▌", l.Render())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestLineInput' 2>&1 | head -5`
Expected: FAIL — `undefined: lineInput`

- [ ] **Step 3: Write minimal implementation**

```go
package tui

import tea "github.com/charmbracelet/bubbletea"

// lineInput is the minimal single-line editor behind the / and : prompts.
// It owns only editing keys; Esc/Enter/Tab are the owning mode's business.
type lineInput struct {
	runes []rune
	pos   int
}

func (l *lineInput) String() string { return string(l.runes) }

func (l *lineInput) Reset() { l.runes, l.pos = l.runes[:0], 0 }

// SetText replaces the buffer and parks the cursor at the end.
func (l *lineInput) SetText(s string) { l.runes = []rune(s); l.pos = len(l.runes) }

// Render is the prompt text with a block cursor at the insertion point.
func (l *lineInput) Render() string {
	return string(l.runes[:l.pos]) + "▌" + string(l.runes[l.pos:])
}

// Handle applies one editing key and reports whether it consumed it.
func (l *lineInput) Handle(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes:
		l.insert(msg.Runes)
	case tea.KeySpace:
		l.insert([]rune{' '})
	case tea.KeyBackspace:
		if l.pos > 0 {
			l.runes = append(l.runes[:l.pos-1], l.runes[l.pos:]...)
			l.pos--
		}
	case tea.KeyDelete:
		if l.pos < len(l.runes) {
			l.runes = append(l.runes[:l.pos], l.runes[l.pos+1:]...)
		}
	case tea.KeyLeft:
		if l.pos > 0 {
			l.pos--
		}
	case tea.KeyRight:
		if l.pos < len(l.runes) {
			l.pos++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		l.pos = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		l.pos = len(l.runes)
	case tea.KeyCtrlU:
		l.runes = append([]rune{}, l.runes[l.pos:]...)
		l.pos = 0
	case tea.KeyCtrlW:
		start := l.pos
		for start > 0 && l.runes[start-1] == ' ' {
			start--
		}
		for start > 0 && l.runes[start-1] != ' ' {
			start--
		}
		l.runes = append(l.runes[:start], l.runes[l.pos:]...)
		l.pos = start
	default:
		return false
	}
	return true
}

func (l *lineInput) insert(rs []rune) {
	out := make([]rune, 0, len(l.runes)+len(rs))
	out = append(out, l.runes[:l.pos]...)
	out = append(out, rs...)
	out = append(out, l.runes[l.pos:]...)
	l.runes = out
	l.pos += len(rs)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `mkdir -p $EV/task1 && go test ./internal/tui/ -run 'TestLineInput' -v 2>&1 | tee $EV/task1/go-test.txt | tail -5`
Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/input.go internal/tui/input_test.go ../../docs/mbo/plans/gff-tui-vim/evidence/task1
git commit -m "feat(gff/tui): lineInput single-line editor for the / and : prompts"
```

---

### Task 2: vim motions + help rebind

**Files:**
- Create: `sdk/gff/internal/tui/keys.go`
- Modify: `sdk/gff/internal/tui/model.go` (`Model` struct; `updateList` arrows/PgUp/PgDn/`?`/`h`; `updatePicker` `j/k`/`h`; `updateDetail` `h`)
- Modify: `sdk/gff/internal/tui/view.go` (footer hint; help overlay key lines)
- Modify: `sdk/gff/internal/tui/round2_test.go:68` (`'h'` → `'?'`)
- Test: `sdk/gff/internal/tui/vim_test.go` (package `tui_test`)

**Interfaces:**
- Consumes: existing `Model.cursor/rows/pages/pageIdx/scrollTop/lastInner`, `rescope()`, `buildRows()`.
- Produces: `motionState`, `handleMotion`, `moveCursor`, `turnPage`, `fullPage`, `halfPage` per §3 (Task 4 reuses `moveCursor`; Task 7 documents the keys).

- [ ] **Step 1: Write the failing tests**

```go
package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func rn(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// cursorLine returns the rendered row carrying the "> " cursor marker.
func cursorLine(v string) string {
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "> ") {
			return l
		}
	}
	return ""
}

func TestVimJKMoveLikeArrows(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand install
	m = press(m, rn("j"))
	assert.Contains(t, cursorLine(m.View()), "install.ai.claude")
	m = press(m, rn("j"))
	assert.Contains(t, cursorLine(m.View()), "install.ai.teams")
	m = press(m, rn("k"))
	assert.Contains(t, cursorLine(m.View()), "install.ai.claude")
}

func TestVimKClampsAtTop(t *testing.T) {
	m := newPagerModel(t)
	before := m.View()
	m = press(m, rn("k"))
	assert.Equal(t, before, m.View(), "k on the first row is a no-op")
}

func TestVimHLTurnPages(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, rn("l"))
	assert.Contains(t, m.View(), "[ai]", "l = next category page")
	m = press(m, rn("h"))
	assert.Contains(t, m.View(), "(All)]", "h = previous page")
	m = press(m, rn("h"))
	assert.Contains(t, m.View(), "[shell]", "h wraps to the last page")
}

func TestVimGGAndG(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, rn("G"))
	assert.Contains(t, cursorLine(m.View()), "install.shell.profiles", "G = last row")
	m = press(m, rn("g"))
	assert.Contains(t, cursorLine(m.View()), "install.shell.profiles", "a lone g moves nothing")
	m = press(m, rn("g"))
	assert.Contains(t, cursorLine(m.View()), "install", "gg = first row")
	assert.NotContains(t, cursorLine(m.View()), "install.", "first row is the area header")
}

func TestVimPendingGIsCancelledByAnotherKey(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, rn("G"))
	m = press(m, rn("g"))
	m = press(m, rn("k")) // cancels the pending g AND moves up once
	assert.Contains(t, cursorLine(m.View()), "install.pkg.manager")
	m = press(m, rn("g")) // a fresh single g: still pending, nothing moves
	assert.Contains(t, cursorLine(m.View()), "install.pkg.manager")
}

func TestVimCtrlDUHalfPage(t *testing.T) {
	m := newPagerModel(t)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m.View() // establishes lastInner
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	after := cursorLine(m.View())
	assert.NotContains(t, after, "▼ install", "ctrl+d moved off the header")
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	assert.Contains(t, cursorLine(m.View()), "install", "ctrl+u returns to the top")
}

func TestVimCtrlFBFullPage(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlF})
	assert.Contains(t, cursorLine(m.View()), "install.shell.profiles", "ctrl+f clamps to the last row on a short list")
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlB})
	assert.NotContains(t, cursorLine(m.View()), "install.", "ctrl+b clamps to the first row")
}

func TestHelpOpensOnQuestionAndF1NotH(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, rn("h"))
	assert.NotContains(t, m.View(), "KEYS", "h no longer opens help in the list")
	m = press(m, tea.KeyMsg{Type: tea.KeyF1})
	assert.Contains(t, m.View(), "KEYS", "F1 opens help")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	m = press(m, rn("?"))
	assert.Contains(t, m.View(), "KEYS", "? opens help")
	assert.Contains(t, m.View(), "j/k", "help lists the vim keys")
}

func TestHDoesNotOpenHelpInPickerOrDetail(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // pkg page
	m = press(m, tea.KeyMsg{Type: tea.KeySpace})  // picker
	m = press(m, rn("h"))
	assert.Contains(t, m.View(), "Pick option", "h in the picker is inert")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // detail
	m = press(m, rn("h"))
	assert.Contains(t, m.View(), "LAYERS", "h in the detail view is inert")
}

func TestPickerJKMoveCursor(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(m, rn("j"))
	assert.Contains(t, cursorLine(m.View()), "apt", "j moves the picker cursor")
	m = press(m, rn("k"))
	assert.Contains(t, cursorLine(m.View()), "auto", "k moves it back")
}
```

Also edit `round2_test.go` line 68: replace `Runes: []rune{'h'}` with `Runes: []rune{'?'}` in `TestHelpOverlayFromDetail`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestVim|TestHelpOpensOn|TestHDoesNot|TestPickerJK' 2>&1 | grep -E '^(--- FAIL|FAIL|ok)' | head`
Expected: FAIL for every new test (`j` is ignored, `h` opens help, `G` does nothing).

- [ ] **Step 3: Write minimal implementation**

`keys.go`:

```go
package tui

import tea "github.com/charmbracelet/bubbletea"

// motionState carries the multi-key vim motion in flight (only "gg" today).
type motionState struct {
	pendingG bool
}

// handleMotion applies a vim motion in list mode and reports whether it
// consumed the key. A pending 'g' is completed only by a second 'g'; any
// other key cancels it and is then handled normally.
func (m *Model) handleMotion(msg tea.KeyMsg) bool {
	if m.motion.pendingG {
		m.motion.pendingG = false
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'g' {
			m.cursor, m.scrollTop = 0, 0
			m.rescope()
			return true
		}
	}
	switch msg.Type {
	case tea.KeyCtrlD:
		m.moveCursor(m.halfPage())
	case tea.KeyCtrlU:
		m.moveCursor(-m.halfPage())
	case tea.KeyCtrlF:
		m.moveCursor(m.fullPage())
	case tea.KeyCtrlB:
		m.moveCursor(-m.fullPage())
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return false
		}
		switch msg.Runes[0] {
		case 'j':
			m.moveCursor(1)
		case 'k':
			m.moveCursor(-1)
		case 'h':
			m.turnPage(-1)
		case 'l':
			m.turnPage(1)
		case 'g':
			m.motion.pendingG = true
		case 'G':
			m.moveCursor(len(m.rows))
		default:
			return false
		}
	default:
		return false
	}
	return true
}

// moveCursor moves by delta, clamps to the row list, and re-derives the
// breadcrumb scope exactly as the arrow keys do.
func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor > len(m.rows)-1 {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.rescope()
}

// turnPage moves dir pages through the breadcrumb with wraparound.
func (m *Model) turnPage(dir int) {
	n := len(m.pages)
	if n <= 1 {
		return
	}
	m.pageIdx = ((m.pageIdx+dir)%n + n) % n
	m.cursor, m.scrollTop = 0, 0
	m.buildRows()
}

// fullPage is the PgUp/PgDn stride: the body rows shown in the last render.
func (m *Model) fullPage() int {
	if m.lastInner < 1 {
		return 10
	}
	return m.lastInner
}

// halfPage is the ctrl+d / ctrl+u stride.
func (m *Model) halfPage() int {
	if n := m.fullPage() / 2; n > 0 {
		return n
	}
	return 1
}
```

`model.go` edits:

1. Add `motion motionState` to `Model`.
2. In `updateList`, first line of the function body: `if m.handleMotion(msg) { return m, nil }`.
3. Replace the `KeyUp`/`KeyDown` cases with `m.moveCursor(-1)` / `m.moveCursor(1)`; replace the `KeyLeft, KeyRight` body with `m.turnPage(-1)` / `m.turnPage(1)`; replace the `KeyPgUp, KeyPgDown` body with `m.moveCursor(-m.fullPage())` / `m.moveCursor(m.fullPage())`.
4. Add `case tea.KeyF1:` (next to the `'?'` rune case) in `updateList`, `updatePicker`, `updateDetail` doing what `'?'` does there.
5. In the rune switches of `updateList`, `updateDetail`, `updatePicker`: change `case '?', 'h', 'H':` to `case '?':`.
6. In `updatePicker`'s rune switch add:

```go
		case 'j':
			if m.pickerCursor < len(m.pickerEntries)-1 {
				m.pickerCursor++
			}
		case 'k':
			if m.pickerCursor > 0 {
				m.pickerCursor--
			}
```

`view.go` edits:

1. The list footer hint becomes exactly the §3 string.
2. Help overlay, `default:` (flag list) branch, replace the three key lines with:

```go
		sb.WriteString(dim.Render("  j/k ↑/↓ move · h/l ←/→ category pages · gg/G first/last · ^d/^u half page · ^f/^b (PgUp/PgDn) page"))
		sb.WriteString("\n")
		sb.WriteString(dim.Render("  /  regex search (smartcase; Enter commit, Esc cancel) · n/N next/prev match · Esc clear highlights"))
		sb.WriteString("\n")
		sb.WriteString(dim.Render("  :  command line — :set <key> <value> · :unset <key> · :/re · :help · :q  (Tab completes keys)"))
		sb.WriteString("\n")
		sb.WriteString(dim.Render("  Enter  expand an area / open feature details (attributes + layers)"))
		sb.WriteString("\n")
		sb.WriteString(dim.Render("  Space  toggle a bool / pick choice options · u clear the user override · q quit"))
		sb.WriteString("\n")
```

3. Picker help line: `"  j/k ↑/↓ move · Space toggle an option (multi) · Enter select/confirm · Esc cancel"`.
4. Every `?/h` mention in help/detail footers becomes `?`; the final help line becomes `"Esc/?/q close"` (unchanged) and the detail footer `"Space toggle/pick  u clear user override  Esc/Enter back  ? help"` (unchanged).

- [ ] **Step 4: Run the whole package**

Run: `mkdir -p $EV/task2 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task2/go-test.txt | tail -3`
Expected: `ok … coverage: ≥ 91.3%`. If `TestHelpOverlayFromDetail` still fails, the `'h'`→`'?'` rewire was missed.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/keys.go internal/tui/model.go internal/tui/view.go internal/tui/vim_test.go internal/tui/round2_test.go ../../docs/mbo/plans/gff-tui-vim/evidence/task2
git commit -m "feat(gff/tui): vim motions (j/k/h/l, gg/G, ^d/^u/^f/^b); help moves to ? and F1"
```

---

### Task 3: search engine (pure helpers)

**Files:**
- Create: `sdk/gff/internal/tui/search.go` (this task: `searchState`, `compilePattern`, `matchItem`, `rowKey`, `inScope`, `rowIndexOf`)
- Modify: `sdk/gff/internal/tui/model.go` (`buildRows` uses `inScope`)
- Test: `sdk/gff/internal/tui/search_internal_test.go` (package `tui`)

**Interfaces:**
- Consumes: `resolve.Resolved`, `row`, `Model.pageIdx/pages/scopeNS`.
- Produces: the pure half of §3 `search.go`; Task 4 adds the mode methods.

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilePatternSmartcase(t *testing.T) {
	re, err := compilePattern("claude")
	require.NoError(t, err)
	assert.True(t, re.MatchString("Claude CLI"), "all-lowercase pattern is case-insensitive")

	re, err = compilePattern("Claude")
	require.NoError(t, err)
	assert.False(t, re.MatchString("claude"), "an uppercase letter makes it case-sensitive")

	re, err = compilePattern("ai\\.(claude|teams)")
	require.NoError(t, err)
	assert.True(t, re.MatchString("install.ai.teams"))

	re, err = compilePattern("")
	require.NoError(t, err)
	assert.Nil(t, re, "empty pattern compiles to nil (no matches, no error)")

	_, err = compilePattern("[ai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing closing ]")
}

func item(path, desc string) resolve.Resolved {
	return resolve.Resolved{Feature: &gffv1.Feature{Path: path, Description: desc}}
}

func TestMatchItemPathOrDescription(t *testing.T) {
	re, _ := compilePattern("teams")
	assert.True(t, matchItem(item("install.ai.teams", ""), re), "path matches")
	assert.True(t, matchItem(item("install.ai.x", "AI teams"), re), "description matches")
	assert.False(t, matchItem(item("install.ai.claude", "Claude CLI"), re))
	assert.False(t, matchItem(item("install.ai.claude", "Claude CLI"), nil), "nil regex never matches")
}

func TestRowKeyDistinguishesAreasAndItems(t *testing.T) {
	assert.Equal(t, "area:ns\x00install", rowKey(row{isArea: true, ns: "ns", area: "install"}))
	assert.Equal(t, "item:3", rowKey(row{itemIdx: 3}))
	assert.NotEqual(t, rowKey(row{itemIdx: 0}), rowKey(row{isArea: true}))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestCompilePattern|TestMatchItem|TestRowKey' 2>&1 | head -5`
Expected: FAIL — `undefined: compilePattern`.

- [ ] **Step 3: Write minimal implementation**

`search.go`:

```go
package tui

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
)

// searchState is the / prompt plus the committed pattern that n/N replay.
type searchState struct {
	input     lineInput
	pattern   string         // committed pattern (n/N)
	re        *regexp.Regexp // live (search mode) or committed pattern
	err       string         // live compile error shown under the prompt
	matches   []int          // matching row indices, ascending
	visible   bool           // highlights + badge shown (Esc in the list = :noh)
	anchorKey string         // rowKey of the cursor row when / was pressed
	anchorTop int            // scrollTop when / was pressed
}

// compilePattern compiles a vim-style search: an empty pattern is nil (no
// matches, no error); a pattern with no uppercase letter is case-insensitive
// (smartcase).
func compilePattern(p string) (*regexp.Regexp, error) {
	if p == "" {
		return nil, nil
	}
	if !strings.ContainsFunc(p, unicode.IsUpper) {
		p = "(?i)" + p
	}
	return regexp.Compile(p)
}

// matchItem reports whether the pattern hits the key path or the description.
func matchItem(item resolve.Resolved, re *regexp.Regexp) bool {
	if re == nil {
		return false
	}
	return re.MatchString(item.Feature.GetPath()) || re.MatchString(item.Feature.GetDescription())
}

// rowKey identifies a row across buildRows rebuilds (row indices shift when
// an area expands; keys do not).
func rowKey(r row) string {
	if r.isArea {
		return "area:" + r.ns + "\x00" + r.area
	}
	return "item:" + strconv.Itoa(r.itemIdx)
}

// inScope is the single visibility rule shared by buildRows and search: the
// All page shows every namespace; a category page shows its component within
// the breadcrumb's namespace.
func (m *Model) inScope(item resolve.Resolved) bool {
	if m.pageIdx <= 0 || m.pageIdx >= len(m.pages) {
		return true
	}
	if componentOf(item.Feature.GetPath()) != m.pages[m.pageIdx].component {
		return false
	}
	return m.scopeNS == "" || item.Namespace() == m.scopeNS
}

// rowIndexOf finds a row by key; 0 when it is no longer rendered.
func (m *Model) rowIndexOf(key string) int {
	for i, r := range m.rows {
		if rowKey(r) == key {
			return i
		}
	}
	return 0
}
```

`model.go`, `buildRows` category branch: replace the two `continue` checks with

```go
			if !m.inScope(item) {
				continue
			}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `mkdir -p $EV/task3 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task3/go-test.txt | tail -3`
Expected: `ok`, coverage ≥ 91.3% (the whole package still passes after the `inScope` refactor).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/search.go internal/tui/search_internal_test.go internal/tui/model.go ../../docs/mbo/plans/gff-tui-vim/evidence/task3
git commit -m "feat(gff/tui): search primitives — smartcase compile, path/description match, row keys, shared inScope"
```

---

### Task 4: `/` search mode, `n`/`N`, `:noh`, highlight rendering

**Files:**
- Modify: `sdk/gff/internal/tui/search.go` (mode methods)
- Modify: `sdk/gff/internal/tui/model.go` (`modeSearch`; `search searchState`; dispatch; `/`, `n`, `N`, Esc in `updateList`; `buildRows` → `collectMatches`)
- Modify: `sdk/gff/internal/tui/view.go` (`View` routes `modeSearch` to `viewList`; prompt/error lines; badge; `*` gutter + style; height budget)
- Test: `sdk/gff/internal/tui/search_test.go` (package `tui_test`)

**Interfaces:**
- Consumes: Task 1 `lineInput`, Task 2 `moveCursor`, Task 3 helpers.
- Produces: `startSearch/applySearch/commitSearch/cancelSearch/collectMatches/nextMatch/clearHighlights/matchBadge` per §3; Task 6 reuses `startSearch`+`applySearch`+`commitSearch` for `:/re`.

- [ ] **Step 1: Write the failing tests**

```go
package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func typeKeys(m tea.Model, s string) tea.Model {
	for _, c := range s {
		m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
	}
	return m
}

// gutterLines returns the rendered lines carrying the "* " match marker.
// The cursor row keeps its "> " marker (cursor wins), so a search with N
// matches and the cursor parked on one of them shows N-1 gutter stars.
func gutterLines(v string) []string {
	var out []string
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "* ") || strings.HasPrefix(l, "*") {
			out = append(out, l)
		}
	}
	return out
}

func TestSlashSearchExpandsAreaAndJumpsToFirstMatch(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t) // install collapsed
	m = typeKeys(m, "/ai")
	v := m.View()
	assert.Contains(t, v, "/ai▌", "prompt shows the live pattern")
	assert.Contains(t, v, "▼ install", "the area holding the matches auto-expanded")
	assert.Contains(t, cursorLine(v), "install.ai.claude", "cursor landed on the first match after the anchor")
	assert.Len(t, gutterLines(v), 1, "the other match (teams) carries the * marker; the cursor row keeps >")
	assert.Contains(t, gutterLines(v)[0], "install.ai.teams")
}

func TestSlashSearchTypedLettersNeverFireNormalKeys(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: pagerYAML})
	items, err := r.All()
	require.NoError(t, err)
	var m tea.Model = tui.NewModel(items, p)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, c := range "qujk" {
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
		assert.Nil(t, cmd, "%q inside the prompt must not quit", c)
	}
	assert.Contains(t, m.View(), "/qujk▌")
	_, statErr := os.Stat(p.UserOverride)
	assert.True(t, os.IsNotExist(statErr), "u inside the prompt must not unset anything")
}

func TestSlashSearchInvalidRegexKeepsPreviousMatches(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t)
	m = typeKeys(m, "/ai")
	assert.Len(t, gutterLines(m.View()), 1) // teams; the cursor sits on claude
	m = typeKeys(m, "[")
	v := m.View()
	assert.Contains(t, v, "missing closing ]", "inline error under the prompt")
	assert.Len(t, gutterLines(v), 1, "previous matches kept")
	assert.Contains(t, cursorLine(v), "install.ai.claude", "cursor did not move")
	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	assert.NotContains(t, m.View(), "missing closing", "fixing the pattern clears the error")
}

func TestSlashSearchEscRestoresCursorAndClears(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t)
	m = typeKeys(m, "/shell")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	v := m.View()
	assert.Contains(t, cursorLine(v), "▼ install", "cursor back on the area header where / was pressed")
	assert.Empty(t, gutterLines(v), "matches cleared")
	assert.Contains(t, v, "▼ install", "the expanded area stays expanded")
	assert.NotContains(t, v, "▌", "prompt closed")
}

func TestSlashSearchEnterCommitsAndNNHop(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t)
	m = typeKeys(m, "/ai")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	v := m.View()
	assert.Contains(t, v, "/ai [1/2]", "badge shows position and count")
	assert.Contains(t, cursorLine(v), "install.ai.claude")
	m = typeKeys(m, "n")
	assert.Contains(t, cursorLine(m.View()), "install.ai.teams")
	assert.Contains(t, m.View(), "[2/2]")
	m = typeKeys(m, "n")
	assert.Contains(t, cursorLine(m.View()), "install.ai.claude", "n wraps")
	m = typeKeys(m, "N")
	assert.Contains(t, cursorLine(m.View()), "install.ai.teams", "N wraps backwards")
}

func TestSlashSearchNoMatchReportsNotFound(t *testing.T) {
	m := newPagerModel(t)
	before := cursorLine(m.View())
	m = typeKeys(m, "/zzz")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	assert.Contains(t, m.View(), "pattern not found: zzz")
	assert.Equal(t, before, cursorLine(m.View()), "cursor did not move")
}

func TestEscInListClearsHighlightsButKeepsPatternForN(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t)
	m = typeKeys(m, "/ai")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	v := m.View()
	assert.Empty(t, gutterLines(v), ":noh clears the markers")
	assert.NotContains(t, v, "[1/2]")
	m = typeKeys(m, "n")
	assert.NotEmpty(t, gutterLines(m.View()), "n re-arms the last pattern")
}

func TestNWithoutPatternIsNoop(t *testing.T) {
	m := newPagerModel(t)
	before := m.View()
	m = typeKeys(m, "n")
	assert.Equal(t, before, m.View())
}

func TestSearchScopeIsTheCurrentPage(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // ai page: claude, teams
	m = typeKeys(m, "/install")
	assert.Len(t, gutterLines(m.View()), 1, "only the page's two rows match (cursor on one, star on the other)")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = typeKeys(m, "l") // pkg page: the badge recomputes for the new page
	assert.Contains(t, m.View(), "[1/1]")
}

func TestSearchPromptKeepsFrameWithinHeight(t *testing.T) {
	m := newPagerModel(t)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = typeKeys(m, "/[")
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	assert.LessOrEqual(t, len(lines), 8, "prompt + error line fit the height budget")
}
```

Add `"os"` and the `tui` import to the file's import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestSlash|TestEscInList|TestNWithout|TestSearch' 2>&1 | grep -E '^(--- FAIL|ok|FAIL)' | head`
Expected: every new test FAILs (`/` is ignored today).

- [ ] **Step 3: Write minimal implementation**

`search.go` additions:

```go
// startSearch anchors the cursor and opens the / prompt.
func (m *Model) startSearch() {
	s := &m.search
	s.input.Reset()
	s.err = ""
	s.re = nil
	s.matches = s.matches[:0]
	s.visible = true
	s.anchorTop = m.scrollTop
	s.anchorKey = ""
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		s.anchorKey = rowKey(m.rows[m.cursor])
	}
	m.mode = modeSearch
}

// applySearch recompiles the live text, reveals matches, and parks the
// cursor on the first hit at or after the anchor (wrapping to the top).
func (m *Model) applySearch() {
	s := &m.search
	re, err := compilePattern(s.input.String())
	if err != nil {
		s.err = err.Error() // keep the previous matches on screen
		return
	}
	s.err = ""
	s.re = re
	if m.pageIdx == 0 { // category pages are flat: nothing to expand
		for _, it := range m.items {
			if matchItem(it, re) {
				m.expanded[it.Namespace()+"\x00"+areaOf(it.Feature.GetPath())] = true
			}
		}
	}
	m.buildRows() // → collectMatches
	if len(s.matches) == 0 {
		m.cursor = m.rowIndexOf(s.anchorKey)
		return
	}
	anchor := m.rowIndexOf(s.anchorKey)
	target := s.matches[0]
	for _, ri := range s.matches {
		if ri >= anchor {
			target = ri
			break
		}
	}
	m.cursor = target
	m.rescope()
}

// collectMatches maps the current pattern onto the rendered rows. buildRows
// calls it so page turns and expands keep the match set honest.
func (m *Model) collectMatches() {
	s := &m.search
	s.matches = s.matches[:0]
	if s.re == nil || !s.visible {
		return
	}
	for i, r := range m.rows {
		if !r.isArea && matchItem(r.item, s.re) {
			s.matches = append(s.matches, i)
		}
	}
}

// commitSearch is Enter in the prompt: freeze the pattern for n/N.
func (m *Model) commitSearch() {
	s := &m.search
	m.mode = modeList
	if s.err != "" {
		m.errMsg = "invalid pattern: " + s.err
		s.err = ""
		return
	}
	s.pattern = s.input.String()
	s.input.Reset()
	if s.pattern == "" {
		s.visible = false
		s.re = nil
		s.matches = s.matches[:0]
		return
	}
	if len(s.matches) == 0 {
		m.errMsg = "pattern not found: " + s.pattern
	}
}

// cancelSearch is Esc in the prompt: back to where / was pressed.
func (m *Model) cancelSearch() {
	s := &m.search
	m.mode = modeList
	s.input.Reset()
	s.err = ""
	s.re = nil
	s.visible = false
	s.matches = s.matches[:0]
	m.cursor = m.rowIndexOf(s.anchorKey)
	m.scrollTop = s.anchorTop
	m.rescope()
}

// nextMatch is n (+1) / N (-1): hop with wraparound; re-arm after :noh.
func (m *Model) nextMatch(dir int) {
	s := &m.search
	if s.pattern == "" {
		return
	}
	if !s.visible {
		re, err := compilePattern(s.pattern)
		if err != nil {
			return
		}
		s.re, s.visible = re, true
		m.buildRows()
	}
	if len(s.matches) == 0 {
		m.errMsg = "pattern not found: " + s.pattern
		return
	}
	target := -1
	if dir > 0 {
		for _, ri := range s.matches {
			if ri > m.cursor {
				target = ri
				break
			}
		}
		if target < 0 {
			target = s.matches[0]
		}
	} else {
		for i := len(s.matches) - 1; i >= 0; i-- {
			if s.matches[i] < m.cursor {
				target = s.matches[i]
				break
			}
		}
		if target < 0 {
			target = s.matches[len(s.matches)-1]
		}
	}
	m.cursor = target
	m.errMsg = ""
	m.rescope()
}

// clearHighlights is Esc in the list (vim :noh): hide, keep the pattern.
func (m *Model) clearHighlights() {
	s := &m.search
	s.visible = false
	s.matches = s.matches[:0]
	m.errMsg = ""
}

// matchBadge is the footer's "/pattern [i/N]" (i = "-" off a match).
func (m *Model) matchBadge() string {
	s := &m.search
	if !s.visible || s.pattern == "" {
		return ""
	}
	pos := "-"
	for i, ri := range s.matches {
		if ri == m.cursor {
			pos = strconv.Itoa(i + 1)
			break
		}
	}
	return "/" + s.pattern + " [" + pos + "/" + strconv.Itoa(len(s.matches)) + "]"
}

// updateSearch handles keys while the / prompt is open.
func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.cancelSearch()
	case tea.KeyEnter:
		m.commitSearch()
	default:
		if m.search.input.Handle(msg) {
			m.applySearch()
		}
	}
	return m, nil
}
```

Add `tea "github.com/charmbracelet/bubbletea"` to `search.go`'s imports.

`model.go` edits:

1. Constants: append `modeSearch` and `modeCommand` after `modeHelp` (`modeCommand` is wired in Task 6 but declared now).
2. `Model` struct: add `search searchState` and `cmd lineInput` (the `:` buffer; Task 6 wires it).
3. `Update` key dispatch: add `case modeSearch: return m.updateSearch(msg)`.
4. `buildRows`: call `m.collectMatches()` immediately before the category branch's early `return` **and** as the last statement of the function.
5. `updateList` rune switch, add:

```go
		case '/':
			m.startSearch()
		case 'n':
			m.nextMatch(1)
		case 'N':
			m.nextMatch(-1)
```

   and a new key case `case tea.KeyEscape: m.clearHighlights()` in `updateList`'s type switch.

`view.go` edits:

1. `View()`: `case modeSearch, modeCommand: return m.viewList()`.
2. `viewList` overhead: after `overhead := 4`, add `if m.mode == modeSearch && m.search.err != "" { overhead++ }`.
3. Match set + gutter: before the row loop build `matchSet := map[int]bool{}` from `m.search.matches` when `m.search.visible`. In the loop, `cursor := "  "`; `if matchSet[i] { cursor = "* " }`; `if i == m.cursor { cursor = "> " }`. For a matching, non-cursor feature row render the `path` cell with `matchStyle` (`lipgloss.NewStyle().Bold(true).Foreground(pal.Orange)`, plain under `noColor()`) instead of `dimStyle`.
4. Footer: replace the single hint `WriteString` with (the `modeCommand` branch compiles now and goes live in Task 6)

```go
	switch m.mode {
	case modeSearch:
		sb.WriteString("/" + m.search.input.Render())
		if m.search.err != "" {
			sb.WriteString("\n")
			sb.WriteString(errStyleFor(pal).Render(m.search.err))
		}
	case modeCommand:
		sb.WriteString(":" + m.cmd.Render())
	default:
		hint := listHint
		if b := m.matchBadge(); b != "" {
			hint = b + "  " + hint
		}
		sb.WriteString(dimStyle.Render(hint))
	}
```

   with `const listHint = "j/k ↑/↓ move  h/l ←/→ page  gg/G top/end  ^d/^u half  / search  n/N next  : command  Enter open  Space toggle  u clear  ? help  q quit"` and a small helper `func errStyleFor(pal style.Colors) lipgloss.Style` extracted from the existing errMsg rendering (red unless `noColor()`).

- [ ] **Step 4: Run the package**

Run: `mkdir -p $EV/task4 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task4/go-test.txt | tail -3`
Expected: `ok`, coverage ≥ 91.3%. Also `go vet ./...` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/search.go internal/tui/model.go internal/tui/view.go internal/tui/search_test.go ../../docs/mbo/plans/gff-tui-vim/evidence/task4
git commit -m "feat(gff/tui): / incremental regex search with auto-expand, n/N, :noh, match gutter"
```

---

### Task 5: command parser + value validation (pure)

**Files:**
- Create: `sdk/gff/internal/tui/command.go` (this task: `command`, `parseCommand`, `parseValue`, `findKey`)
- Test: `sdk/gff/internal/tui/command_internal_test.go` (package `tui`)

**Interfaces:**
- Consumes: `resolve.Resolved`, `gffv1` value types, `Model.items/scopeNS`.
- Produces: `parseCommand`, `parseValue`, `findKey` per §3 for Task 6.

- [ ] **Step 1: Write the failing tests**

```go
package tui

import (
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCommand(t *testing.T) {
	cases := []struct {
		line string
		want command
		err  string
	}{
		{"set a.b.c true", command{name: "set", args: []string{"a.b.c", "true"}}, ""},
		{"  unset   a.b.c  ", command{name: "unset", args: []string{"a.b.c"}}, ""},
		{"q", command{name: "q"}, ""},
		{"/ai\\.(x|y) z", command{name: "search", args: []string{"ai\\.(x|y) z"}}, ""},
		{"/", command{name: "search", args: []string{""}}, ""},
		{"", command{}, "empty command"},
		{"   ", command{}, "empty command"},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			got, err := parseCommand(tc.line)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want.name, got.name)
			assert.Equal(t, tc.want.args, got.args)
		})
	}
}

func boolItem(path string) resolve.Resolved {
	return resolve.Resolved{Feature: &gffv1.Feature{Path: path, Default: &gffv1.Feature_BoolDefault{BoolDefault: true}}}
}

func choiceItem(path string, mode gffv1.ChoiceMode, ids ...string) resolve.Resolved {
	cd := &gffv1.ChoiceDefault{Mode: mode}
	for _, id := range ids {
		cd.Options = append(cd.Options, &gffv1.ChoiceOption{Id: id})
	}
	return resolve.Resolved{Feature: &gffv1.Feature{Path: path, Default: &gffv1.Feature_ChoiceDefault{ChoiceDefault: cd}}}
}

func TestParseValueBool(t *testing.T) {
	v, err := parseValue(boolItem("a.b"), "false")
	require.NoError(t, err)
	assert.False(t, v.GetBoolValue())
	_, err = parseValue(boolItem("a.b"), "yes")
	require.EqualError(t, err, `value for a.b must be true or false, got "yes"`)
}

func TestParseValueChoice(t *testing.T) {
	single := choiceItem("p.m", gffv1.ChoiceMode_CHOICE_MODE_SINGLE, "auto", "apt")
	multi := choiceItem("p.n", gffv1.ChoiceMode_CHOICE_MODE_MULTI, "a", "b", "c")

	v, err := parseValue(single, "apt")
	require.NoError(t, err)
	assert.Equal(t, []string{"apt"}, v.GetChoiceValue().GetSelected())

	v, err = parseValue(multi, "a,c")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "c"}, v.GetChoiceValue().GetSelected())

	_, err = parseValue(single, "auto,apt")
	require.EqualError(t, err, "p.m is a single-choice flag: give exactly one id")
	_, err = parseValue(single, "brew")
	require.EqualError(t, err, `unknown option "brew" for p.m`)
	_, err = parseValue(multi, "a,a")
	require.EqualError(t, err, `duplicate option "a" for p.n`)
}

func TestFindKeyScopedAndQualified(t *testing.T) {
	m := &Model{items: []resolve.Resolved{
		boolItem("x.y.z").WithNamespace("one"),
		boolItem("x.y.z").WithNamespace("two"),
		boolItem("only.here").WithNamespace("two"),
	}, scopeNS: "two"}
	idx, err := m.findKey("only.here")
	require.NoError(t, err)
	assert.Equal(t, 2, idx)
	idx, err = m.findKey("x.y.z")
	require.NoError(t, err, "ambiguous bare path resolves to the breadcrumb namespace")
	assert.Equal(t, 1, idx)
	idx, err = m.findKey("one:x.y.z")
	require.NoError(t, err)
	assert.Equal(t, 0, idx)
	_, err = m.findKey("nope")
	require.EqualError(t, err, "unknown key: nope")
	m.scopeNS = ""
	_, err = m.findKey("x.y.z")
	require.EqualError(t, err, `ambiguous key "x.y.z": qualify it as <namespace>:x.y.z`)
}
```

`WithNamespace` does not exist on `resolve.Resolved` today (the namespace is unexported). Check `internal/resolve/resolve.go` for a test-visible constructor; if none exists, add to `resolve.go`:

```go
// WithNamespace returns a copy bound to ns. Used by tests that build items
// without a resolver.
func (r Resolved) WithNamespace(ns string) Resolved { r.namespace = ns; return r }
```

and cover it in `internal/resolve/resolve_test.go` with a one-line assertion (`assert.Equal(t, "ns", Resolved{}.WithNamespace("ns").Namespace())`) so `internal/resolve` stays ≥ 95%.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestParseCommand|TestParseValue|TestFindKey' 2>&1 | head -5`
Expected: FAIL — `undefined: parseCommand`.

- [ ] **Step 3: Write minimal implementation**

```go
package tui

import (
	"errors"
	"fmt"
	"strings"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
)

// command is one parsed :-line.
type command struct {
	name string
	args []string
}

// parseCommand tokenizes a :-line. ":/re" is the search alias and keeps the
// whole remainder (spaces included) as its single argument.
func parseCommand(line string) (command, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return command{}, errors.New("empty command")
	}
	if strings.HasPrefix(line, "/") {
		return command{name: "search", args: []string{line[1:]}}, nil
	}
	f := strings.Fields(line)
	return command{name: f[0], args: f[1:]}, nil
}

// parseValue turns the :set value token into a typed Value for item,
// rejecting anything the picker would not let you choose.
func parseValue(item resolve.Resolved, raw string) (*gffv1.Value, error) {
	path := item.Feature.GetPath()
	switch item.Feature.Default.(type) {
	case *gffv1.Feature_BoolDefault:
		switch raw {
		case "true":
			return &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}, nil
		case "false":
			return &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: false}}, nil
		}
		return nil, fmt.Errorf("value for %s must be true or false, got %q", path, raw)
	case *gffv1.Feature_ChoiceDefault:
		cd := item.Feature.GetChoiceDefault()
		known := map[string]bool{}
		for _, opt := range cd.GetOptions() {
			known[opt.GetId()] = true
		}
		ids := strings.Split(raw, ",")
		seen := map[string]bool{}
		for _, id := range ids {
			if !known[id] {
				return nil, fmt.Errorf("unknown option %q for %s", id, path)
			}
			if seen[id] {
				return nil, fmt.Errorf("duplicate option %q for %s", id, path)
			}
			seen[id] = true
		}
		if cd.GetMode() != gffv1.ChoiceMode_CHOICE_MODE_MULTI && len(ids) != 1 {
			return nil, fmt.Errorf("%s is a single-choice flag: give exactly one id", path)
		}
		return &gffv1.Value{Kind: &gffv1.Value_ChoiceValue{ChoiceValue: &gffv1.ChoiceSelection{Selected: ids}}}, nil
	}
	return nil, fmt.Errorf("unsupported flag type for %s", path)
}

// findKey resolves "<ns>:<path>" or a bare "<path>" to an item index. A bare
// path that exists in several namespaces resolves to the breadcrumb's
// namespace when it is one of them, otherwise it is an error.
func (m *Model) findKey(key string) (int, error) {
	ns, path := "", key
	if i := strings.IndexByte(key, ':'); i >= 0 {
		ns, path = key[:i], key[i+1:]
	}
	var hits []int
	for i, it := range m.items {
		if it.Feature.GetPath() != path || (ns != "" && it.Namespace() != ns) {
			continue
		}
		hits = append(hits, i)
	}
	switch len(hits) {
	case 0:
		return -1, fmt.Errorf("unknown key: %s", key)
	case 1:
		return hits[0], nil
	}
	for _, i := range hits {
		if m.scopeNS != "" && m.items[i].Namespace() == m.scopeNS {
			return i, nil
		}
	}
	return -1, fmt.Errorf("ambiguous key %q: qualify it as <namespace>:%s", key, path)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `mkdir -p $EV/task5 && go test ./internal/tui/ ./internal/resolve/ -cover 2>&1 | tee $EV/task5/go-test.txt | tail -3`
Expected: both `ok`; `internal/resolve` ≥ 95%.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/command.go internal/tui/command_internal_test.go internal/resolve/resolve.go internal/resolve/resolve_test.go ../../docs/mbo/plans/gff-tui-vim/evidence/task5
git commit -m "feat(gff/tui): :-line parser, typed value validation, scoped key lookup"
```

---

### Task 6: `:` command mode, execution, Tab completion

**Files:**
- Modify: `sdk/gff/internal/tui/command.go` (`execCommand`, `cmdSet`, `cmdUnset`, `completion`, `completeKey`, `completeCommand`, `updateCommand`)
- Modify: `sdk/gff/internal/tui/model.go` (`cmd lineInput`, `comp completion`; `:` in `updateList`; dispatch `modeCommand`)
- Modify: `sdk/gff/internal/tui/view.go` (the `modeCommand` footer branch from Task 4 now renders `m.cmd`)
- Test: `sdk/gff/internal/tui/command_test.go` (package `tui_test`)

**Interfaces:**
- Consumes: Task 1 `lineInput`, Task 4 `startSearch/applySearch/commitSearch`, Task 5 parser/validation, existing `overrides.Write/Unset`, `refreshItem`.
- Produces: `execCommand`, `completeKey`, `completeCommand`, `updateCommand` per §3.

- [ ] **Step 1: Write the failing tests**

```go
package tui_test

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/tui"
)

func newCmdModel(t *testing.T) (tea.Model, string) {
	t.Helper()
	r, p := newResolver(t, tuiWorld{repo: pagerYAML})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	m.Explain = r.Explain
	return m, p.UserOverride
}

func enter(m tea.Model) (tea.Model, tea.Cmd) { return m.Update(tea.KeyMsg{Type: tea.KeyEnter}) }

func TestColonSetBoolWritesOverrideAndRefreshesRow(t *testing.T) {
	m, ovr := newCmdModel(t)
	m = typeKeys(m, ":set install.ai.claude false")
	assert.Contains(t, m.View(), ":set install.ai.claude false▌", "command prompt shows the line")
	m, _ = enter(m)
	data, err := os.ReadFile(ovr)
	require.NoError(t, err)
	assert.Contains(t, string(data), "install.ai.claude: false")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand install
	assert.Contains(t, m.View(), "user-override", "row shows the new provenance")
	assert.NotContains(t, m.View(), "▌", "prompt closed")
}

func TestColonSetRejectsBadValueWithoutWriting(t *testing.T) {
	m, ovr := newCmdModel(t)
	m = typeKeys(m, ":set install.ai.claude maybe")
	m, _ = enter(m)
	assert.Contains(t, m.View(), `value for install.ai.claude must be true or false, got "maybe"`)
	_, err := os.Stat(ovr)
	assert.True(t, os.IsNotExist(err), "nothing written")

	m = typeKeys(m, ":set install.ai.claude")
	m, _ = enter(m)
	assert.Contains(t, m.View(), "missing value for install.ai.claude")

	m = typeKeys(m, ":set nope true")
	m, _ = enter(m)
	assert.Contains(t, m.View(), "unknown key: nope")
}

func TestColonSetChoiceSingleAndInvalid(t *testing.T) {
	m, ovr := newCmdModel(t)
	m = typeKeys(m, ":set install.pkg.manager apt")
	m, _ = enter(m)
	data, err := os.ReadFile(ovr)
	require.NoError(t, err)
	assert.Contains(t, string(data), "apt")
	m = typeKeys(m, ":set install.pkg.manager auto,apt")
	m, _ = enter(m)
	assert.Contains(t, m.View(), "single-choice flag")
}

func TestColonUnsetClearsOverride(t *testing.T) {
	m, ovr := newCmdModel(t)
	m = typeKeys(m, ":set install.ai.claude false")
	m, _ = enter(m)
	m = typeKeys(m, ":unset install.ai.claude")
	m, _ = enter(m)
	data, _ := os.ReadFile(ovr)
	assert.NotContains(t, string(data), "install.ai.claude")
	m = typeKeys(m, ":unset nope")
	m, _ = enter(m)
	assert.Contains(t, m.View(), "unknown key: nope")
}

func TestColonQuitHelpAndUnknown(t *testing.T) {
	m, _ := newCmdModel(t)
	m = typeKeys(m, ":q")
	_, cmd := enter(m)
	require.NotNil(t, cmd, ":q quits")

	m, _ = newCmdModel(t)
	m = typeKeys(m, ":help")
	m, _ = enter(m)
	assert.Contains(t, m.View(), "KEYS", ":help opens the overlay")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})

	m = typeKeys(m, ":frobnicate")
	m, _ = enter(m)
	assert.Contains(t, m.View(), "unknown command: frobnicate")
}

func TestColonSlashIsSearchAlias(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	a, _ := newCmdModel(t)
	a = typeKeys(a, "/ai")
	a, _ = enter(a)
	b, _ := newCmdModel(t)
	b = typeKeys(b, ":/ai")
	b, _ = enter(b)
	assert.Equal(t, a.View(), b.View(), ":/re and /re end in the same state")
}

func TestColonEscCancels(t *testing.T) {
	m, ovr := newCmdModel(t)
	m = typeKeys(m, ":set install.ai.claude false")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	assert.NotContains(t, m.View(), "▌")
	_, err := os.Stat(ovr)
	assert.True(t, os.IsNotExist(err))
}

func TestColonTypedLettersNeverFireNormalKeys(t *testing.T) {
	m, ovr := newCmdModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	var cmd tea.Cmd
	for _, c := range "qujk" {
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
		assert.Nil(t, cmd)
	}
	_, err := os.Stat(ovr)
	assert.True(t, os.IsNotExist(err))
}

func TestColonTabCompletesKeysInScope(t *testing.T) {
	m, _ := newCmdModel(t)
	m = typeKeys(m, ":set install.ai.")
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":set install.ai.claude▌")
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":set install.ai.teams▌")
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":set install.ai.claude▌", "Tab wraps")
	m = press(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Contains(t, m.View(), ":set install.ai.teams▌", "Shift-Tab goes back")
	m = typeKeys(m, "x") // typing resets the cycle
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":set install.ai.teamsx▌", "no candidate → unchanged")
}

func TestColonTabIsScopedToThePage(t *testing.T) {
	m, _ := newCmdModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // pkg page
	m = typeKeys(m, ":unset ")
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":unset install.pkg.manager▌", "only the page's keys complete")
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":unset install.pkg.manager▌", "single candidate cycles onto itself")
}

func TestColonTabDoesNotCompleteTheValue(t *testing.T) {
	m, _ := newCmdModel(t)
	m = typeKeys(m, ":set install.ai.claude ")
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":set install.ai.claude ▌")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TestColon' 2>&1 | grep -E '^(--- FAIL|ok|FAIL)' | head`
Expected: every `TestColon*` FAILs.

- [ ] **Step 3: Write minimal implementation**

`command.go` additions:

```go
// completion is the Tab cycle over key paths for the current :-line.
type completion struct {
	head       string   // "set " / "unset " — everything before the key token
	candidates []string // in-scope paths matching the typed prefix, item order
	idx        int
}

// execCommand runs one parsed :-line. Errors land in m.errMsg; the mode is
// already back to the list.
func (m *Model) execCommand(c command) tea.Cmd {
	var err error
	switch c.name {
	case "q", "quit":
		return tea.Quit
	case "h", "help":
		m.helpReturn = modeList
		m.mode = modeHelp
	case "search":
		if c.args[0] != "" {
			m.startSearch()
			m.search.input.SetText(c.args[0])
			m.applySearch()
			m.commitSearch()
		}
	case "set":
		err = m.cmdSet(c.args)
	case "unset":
		err = m.cmdUnset(c.args)
	default:
		err = fmt.Errorf("unknown command: %s", c.name)
	}
	if err != nil {
		m.errMsg = err.Error()
	}
	return nil
}

func (m *Model) cmdSet(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: :set <key> <value>")
	}
	if len(args) < 2 {
		return fmt.Errorf("missing value for %s", args[0])
	}
	idx, err := m.findKey(args[0])
	if err != nil {
		return err
	}
	item := m.items[idx]
	val, err := parseValue(item, args[1])
	if err != nil {
		return err
	}
	if err := overrides.Write(m.p, item.Feature.GetPath(), val); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	m.items[idx] = item.WithValue(val, resolve.LayerUserOverride)
	m.refreshItem(idx) // re-resolves when Explain is wired
	m.buildRows()
	return nil
}

func (m *Model) cmdUnset(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: :unset <key>")
	}
	idx, err := m.findKey(args[0])
	if err != nil {
		return err
	}
	if err := overrides.Unset(m.p, m.items[idx].Feature.GetPath()); err != nil {
		return fmt.Errorf("unset failed: %w", err)
	}
	m.refreshItem(idx)
	m.buildRows()
	return nil
}

// completeKey lists in-scope key paths with the prefix, in item order.
func (m *Model) completeKey(prefix string) []string {
	var out []string
	for _, it := range m.items {
		if m.inScope(it) && strings.HasPrefix(it.Feature.GetPath(), prefix) {
			out = append(out, it.Feature.GetPath())
		}
	}
	return out
}

// completeCommand is Tab (+1) / Shift-Tab (-1) on the :-line. Only the key
// token of set/unset completes; the value position is left alone.
func (m *Model) completeCommand(dir int) {
	c := &m.comp
	if c.candidates == nil {
		text := m.cmd.String()
		f := strings.Fields(text)
		if len(f) == 0 || (f[0] != "set" && f[0] != "unset") || len(f) > 2 {
			return
		}
		if len(f) == 2 && strings.HasSuffix(text, " ") {
			return // cursor is at the value position
		}
		prefix := ""
		if len(f) == 2 {
			prefix = f[1]
		}
		cands := m.completeKey(prefix)
		if len(cands) == 0 {
			return
		}
		c.head, c.candidates = f[0]+" ", cands
		if dir > 0 {
			c.idx = -1
		} else {
			c.idx = 0
		}
	}
	n := len(c.candidates)
	c.idx = ((c.idx+dir)%n + n) % n
	m.cmd.SetText(c.head + c.candidates[c.idx])
}

// updateCommand handles keys while the : prompt is open.
func (m *Model) updateCommand(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.mode = modeList
		m.cmd.Reset()
		m.comp = completion{}
	case tea.KeyEnter:
		line := m.cmd.String()
		m.cmd.Reset()
		m.comp = completion{}
		m.mode = modeList
		c, err := parseCommand(line)
		if err != nil {
			return m, nil // empty :-line — nothing to do
		}
		m.errMsg = ""
		return m, m.execCommand(c)
	case tea.KeyTab:
		m.completeCommand(1)
	case tea.KeyShiftTab:
		m.completeCommand(-1)
	default:
		if m.cmd.Handle(msg) {
			m.comp = completion{} // typing resets the cycle
		}
	}
	return m, nil
}
```

Add `tea`, `overrides` imports to `command.go`.

`model.go` edits: add `comp completion` to `Model` (`cmd lineInput` exists since Task 4); `case modeCommand: return m.updateCommand(msg)` in `Update`; in `updateList`'s rune switch:

```go
		case ':':
			m.cmd.Reset()
			m.comp = completion{}
			m.mode = modeCommand
```

`view.go`: the `modeCommand` footer branch renders `":" + m.cmd.Render()` (finish the Task 4 stub).

- [ ] **Step 4: Run the package**

Run: `mkdir -p $EV/task6 && go test ./internal/tui/ -cover 2>&1 | tee $EV/task6/go-test.txt | tail -3 && go vet ./...`
Expected: `ok`, coverage ≥ 91.3%, vet clean.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/command.go internal/tui/model.go internal/tui/view.go internal/tui/command_test.go ../../docs/mbo/plans/gff-tui-vim/evidence/task6
git commit -m "feat(gff/tui): : command line — set/unset/q/help/:/re with Tab key completion"
```

---

### Task 7: docs, key-table consistency test, coverage gate, real-terminal demo

**Files:**
- Modify: `sdk/gff/cmd/tui.go` (`Long`)
- Create: `sdk/gff/cmd/tui_keys_test.go` (package `cmd`)
- Modify: `sdk/gff/README.md` (new section **TUI keys** after the existing `gff tui` mention, or at the end of the CLI tour if there is none)
- Modify: `sdk/gff/AGENTS.md` (`internal/tui` bullet)
- Create: `docs/mbo/plans/gff-tui-vim/evidence/demo/README.md` + transcript

**Interfaces:** none new.

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The four places that list TUI keys (help overlay, footer hint, README,
// --help) must agree; this pins the --help side.
func TestTUIHelpListsVimSearchAndCommandKeys(t *testing.T) {
	for _, want := range []string{"j/k", "h/l", "gg/G", "ctrl+d", "/ ", "n/N", ":set", ":unset", ":q", "? help"} {
		assert.Contains(t, tuiCmd.Long, want, "gff tui --help must mention %q", want)
	}
	assert.NotContains(t, tuiCmd.Long, "h help", "h no longer opens help")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ -run TestTUIHelpListsVimSearchAndCommandKeys 2>&1 | tail -3`
Expected: FAIL on `j/k`.

- [ ] **Step 3: Write the docs**

`cmd/tui.go` `Long`:

```go
	Long: `Launch an interactive bubbletea TUI that shows all resolved feature flags
in a collapsible area tree with layer provenance.

Keys (vim style):
  j/k ↑/↓ move   h/l ←/→ category page   gg/G first/last   ctrl+d/ctrl+u half page   ctrl+f/ctrl+b page
  / regex search (smartcase; Enter commit, Esc cancel)   n/N next/prev match   Esc clear highlights
  : command line — :set <key> <value>  :unset <key>  :/re  :help  :q   (Tab completes key paths)
  Enter expand area / open details   Space toggle bool or pick choice   u clear override   ? help   q quit

Writes go only to the user override file (~/.config/gff/config.yaml, mode 0600).
Quit without any change leaves the file untouched.`,
```

`README.md` — add:

```markdown
## TUI keys

`gff tui` is vim-flavored. Search finds a flag anywhere on the current page (collapsed
areas holding a hit expand themselves); the `:` line is the CLI's `set`/`unset` from
inside the TUI.

| Keys | Action |
| :-- | :-- |
| `j`/`k`, `↑`/`↓` | move |
| `h`/`l`, `←`/`→` | previous / next category page |
| `gg` / `G` | first / last row |
| `ctrl+d` / `ctrl+u`, `ctrl+f` / `ctrl+b` (PgUp/PgDn) | half page / full page |
| `/` then a regex | incremental search, smartcase (`claude` matches `Claude CLI`; `Claude` is exact-case); Enter commits, Esc cancels |
| `n` / `N` | next / previous match (wraps); Esc in the list clears highlights |
| `:set <key> <value>` · `:unset <key>` | write / clear a user override — same writer as the CLI. Bool: `true`/`false`; choice: comma-separated ids. Tab completes key paths |
| `:/re` · `:help` · `:q` | search alias · help · quit |
| Enter | expand an area / open a flag's detail (layers) |
| Space | toggle a bool / open the choice picker |
| `u` | clear the user override on the cursor row |
| `?` / F1 | help |
| `q` | quit |
```

`AGENTS.md` — extend the `internal/tui` bullet with: "vim keymap (`keys.go`), `/` regex search (`search.go`), `:` command line (`command.go`), prompt editor (`input.go`); the footer hint string in `view.go` is the key table's single source — README §TUI keys and `cmd/tui.go` Long mirror it (pinned by `cmd/tui_keys_test.go`)."

- [ ] **Step 4: Run the gates**

Run: `mkdir -p $EV/task7 && go test ./... -cover 2>&1 | tee $EV/task7/go-test-all.txt | grep -E 'coverage|FAIL' && go vet ./... && (cd ../.. && make gff-proto-check 2>&1 | tail -2)`
Expected: every package `ok`; `internal/tui` ≥ 91.3%; module gate (`gff-ci.yml` recipe) ≥ 90% — run its exact `go test -coverpkg` line from the workflow and `tee` it to `$EV/task7/coverage-gate.txt`.

- [ ] **Step 5: Real-terminal demo (human-evidenced gate, spec §6)**

In a real terminal (never backgrounded/piped), from `~/git/dotfiles` on this branch:

```bash
./sdk/gff/build.sh && tmux new-session -d -s gffdemo -x 140 -y 40 'gff tui'
# drive: /wispr  Enter  Space  n  gg  :set install.ai.teams false  Enter  :unset install.ai.teams  Enter  q
tmux send-keys -t gffdemo '/wispr' ; sleep 1; tmux capture-pane -pt gffdemo | tee -a $EV/demo/transcript.txt
# … one capture per step above, then:
tmux send-keys -t gffdemo Enter; sleep 1; tmux send-keys -t gffdemo Space; sleep 1; tmux capture-pane -pt gffdemo | tee -a $EV/demo/transcript.txt
tmux send-keys -t gffdemo q
gff get install.windows.wispr-flow   # shows the toggled value; then restore:
gff unset install.windows.wispr-flow
```

`$EV/demo/README.md` names the date, binary version (`gff version`), and each step with the line of the transcript proving it. Restore any flag you flipped.

- [ ] **Step 6: Commit**

```bash
git add cmd/tui.go cmd/tui_keys_test.go README.md AGENTS.md ../../docs/mbo/plans/gff-tui-vim/evidence/task7 ../../docs/mbo/plans/gff-tui-vim/evidence/demo
git commit -m "docs(gff/tui): key table in --help, README, AGENTS; pin with a test; demo evidence"
```

Then update `docs/mbo/index.md` (state `building → in-review`) and `docs/mbo/plans/gff-tui-vim/TRACKING.md`, commit as `docs(mbo): gff-tui-vim → in-review`, and `gss feature checkpoint` (confirm first).

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| F1a | `TestVimJKMoveLikeArrows`, `TestVimKClampsAtTop` |
| F1b | `TestVimHLTurnPages` |
| F1c | `TestVimGGAndG`, `TestVimPendingGIsCancelledByAnotherKey` |
| F1d | `TestVimCtrlDUHalfPage`, `TestVimCtrlFBFullPage` |
| F1e | `TestPickerJKMoveCursor` |
| F2a | `TestHelpOpensOnQuestionAndF1NotH`, `TestHelpOverlayFromDetail` (rewired) |
| F2b | `TestHelpOpensOnQuestionAndF1NotH`, `TestHDoesNotOpenHelpInPickerOrDetail` |
| F3a | `TestSlashSearchExpandsAreaAndJumpsToFirstMatch`, `TestSlashSearchTypedLettersNeverFireNormalKeys` |
| F3b | `TestSlashSearchInvalidRegexKeepsPreviousMatches` |
| F3c | `TestSlashSearchEscRestoresCursorAndClears` |
| F3d | `TestSlashSearchEnterCommitsAndNNHop`, `TestSlashSearchNoMatchReportsNotFound` |
| F3e | `TestCompilePatternSmartcase` |
| F4a | `TestSlashSearchExpandsAreaAndJumpsToFirstMatch` |
| F4b | `TestMatchItemPathOrDescription` |
| F4c | `TestSearchScopeIsTheCurrentPage` |
| F4d | `TestSlashSearchExpandsAreaAndJumpsToFirstMatch` (anchor = header row 0 → first match after it) |
| F5a | `TestSlashSearchEnterCommitsAndNNHop`, `TestNWithoutPatternIsNoop` |
| F5b | `TestEscInListClearsHighlightsButKeepsPatternForN` |
| F6a | `TestColonSetBoolWritesOverrideAndRefreshesRow`, `TestColonSetRejectsBadValueWithoutWriting` |
| F6b | `TestColonSetChoiceSingleAndInvalid`, `TestParseValueChoice` |
| F6c | `TestColonUnsetClearsOverride` |
| F6d, F6e, F6g | `TestColonQuitHelpAndUnknown`, `TestParseCommand` |
| F6f | `TestColonSlashIsSearchAlias` |
| F6 (routing/Esc) | `TestColonEscCancels`, `TestColonTypedLettersNeverFireNormalKeys` |
| F7a, F7b | `TestColonTabCompletesKeysInScope` |
| F7c | `TestColonTabIsScopedToThePage`, `TestColonTabDoesNotCompleteTheValue` |
| F8a | `TestSearchPromptKeepsFrameWithinHeight` |
| F8b | `gutterLines` assertions in every `NO_COLOR` search test |
| F9 | `TestTUIHelpListsVimSearchAndCommandKeys`, `TestHelpOpensOnQuestionAndF1NotH` (overlay lists `j/k`) |
| Editor | `TestLineInputEditing`, `TestLineInputDoesNotConsumeModeKeys`, `TestLineInputRenderAndReset` |
| Key lookup | `TestFindKeyScopedAndQualified` |

## 6. Integration & rollout

- No build/CI wiring changes: `gff-ci.yml` already runs `go test ./...` with the coverage gate for the module, and the sdk `scripts/test.sh` discovers the package by directory.
- Docs touched: `sdk/gff/README.md`, `sdk/gff/AGENTS.md`, `cmd/tui.go` Long. `docs/mbo/index.md` row `gff-tui-vim` moves `planning → building → in-review → merged`.
- Rollout is the PR merge; users get the keys on the next `sdk/gff/build.sh` (install.sh runs it).
- Manual acceptance: the Task 7 Step 5 demo transcript.

### 6.1 Build leaves / DAG

Single leaf — not broken out. Tasks 1→7 are strictly sequential inside one `gss feature` worker (`gff-tui-vim/<user>/build`, `--base` = the design worker's branch so the docs and the build stack).

| Leaf | Owns (paths) | Consumes | `done-when` gate | Blocking? |
| :-- | :-- | :-- | :-- | :-- |
| build | `sdk/gff/internal/tui/**`, `sdk/gff/cmd/tui.go`, `sdk/gff/cmd/tui_keys_test.go`, `sdk/gff/README.md`, `sdk/gff/AGENTS.md`, `docs/mbo/plans/gff-tui-vim/evidence/**` | design worker (this plan) | `go test ./... -cover` ≥ 90% module / ≥ 91.3% tui · `go vet` · demo transcript committed | — |

## 7. Validation & evidence (show the work)

- **Coverage bars:** module ≥ 90% (CI gate); `internal/tui` ≥ 91.3% (no regression — checked in every task's Step 4); `internal/resolve` ≥ 95% after the `WithNamespace` addition.
- **Adversarial cases baked into tests:** invalid regex mid-typing; normal-mode letters typed into both prompts; `:set` with a missing value, unknown key, wrong bool, unknown/duplicate/over-count choice ids; ambiguous keys across namespaces; Tab at the value position; pending `g` cancelled by another key; motions on a list shorter than a page.
- **Evidence protocol:** `docs/mbo/plans/gff-tui-vim/evidence/task{1..7}/go-test.txt` (each task's Step 4 output, dated header, append-only), `evidence/task7/coverage-gate.txt`, `evidence/demo/{README.md,transcript.txt}`. A task without its evidence file is not done; the `TRACKING.md` row stays `in-progress`.
- **Demo plan:** the Task 7 Step 5 tmux script against the live dotfiles inventory (`.github/gff/features.yaml`) — `/wispr` finds `install.windows.wispr-flow` inside the collapsed `install` area; `:set`/`:unset` round-trip a bool; the flipped flag is restored before the transcript is committed.

> Produced via `superpowers:writing-plans`. Execute with `superpowers:executing-plans` /
> `subagent-driven-development`, TDD throughout, using the trio in [`./gff-tui-vim/`](./gff-tui-vim/).
> Update `../index.md` state as it moves.
