package tui_test

// Owner-reported (two registered sources, dotfiles + playground):
//  1. no clean way to switch sources — the breadcrumb only ever lists ONE
//     source's categories and the only way across was moving the cursor onto
//     another source's area row on the All page. Tab / Shift+Tab now cycle the
//     source; h/l stay inside it.
//  2. paging h/l onto a category past the terminal's right edge left the
//     active page invisible — the breadcrumb was one unbounded line the
//     renderer cut at the width. It now scrolls to keep the active page shown.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	keyTab      = tea.KeyMsg{Type: tea.KeyTab}
	keyShiftTab = tea.KeyMsg{Type: tea.KeyShiftTab}
)

// crumb is the breadcrumb line (the first line of the list view).
func crumb(m tea.Model) string {
	return strings.SplitN(m.View(), "\n", 2)[0]
}

func TestTabSwitchesSourceAndBack(t *testing.T) {
	m, _ := newTwoNSModel(t)
	require.Contains(t, crumb(m), "pkg", "launch scope is the repo namespace")

	m = press(m, keyTab)
	c := crumb(m)
	assert.Contains(t, c, "com.example.other", "Tab scopes the breadcrumb to the next source")
	assert.Contains(t, c, "web", "the other source's categories are listed")
	assert.NotContains(t, c, "pkg", "the first source's categories are out of scope")
	assert.Contains(t, cursorLine(m.View()), "com.example.other",
		"the cursor lands on the new source's rows so the breadcrumb and cursor agree")

	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	v := m.View()
	assert.Contains(t, v, "install.web.nginx", "l pages within the new source")
	assert.NotContains(t, v, "install.ai.claude")

	m = press(m, keyTab) // wraps back to the first source
	c = crumb(m)
	assert.Contains(t, c, "pkg")
	assert.NotContains(t, c, "web")
}

func TestShiftTabCyclesSourceBackwards(t *testing.T) {
	m, _ := newTwoNSModel(t)
	m = press(m, keyShiftTab) // two sources: backwards from the first wraps to the second
	assert.Contains(t, crumb(m), "web")
	m = press(m, keyShiftTab)
	assert.Contains(t, crumb(m), "pkg")
}

func TestTabFromCategoryPageLandsOnNewSourcesAllPage(t *testing.T) {
	m, _ := newTwoNSModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // tui-test: ai page
	m = press(m, keyTab)
	c := crumb(m)
	assert.Contains(t, c, "[install (All)]", "a source switch starts on that source's All page")
	assert.Contains(t, c, "web")
}

func TestTabWithOneSourceIsNoop(t *testing.T) {
	m := newPagerModel(t)
	before := m.View()
	m = press(m, keyTab)
	assert.Equal(t, before, m.View())
}

func TestBreadcrumbCountsSources(t *testing.T) {
	m, _ := newTwoNSModel(t)
	assert.Contains(t, crumb(m), "(1/2)", "multi-source worlds say there is more than one source")
	m = press(m, keyTab)
	assert.Contains(t, crumb(m), "(2/2)")

	single := newPagerModel(t)
	assert.NotContains(t, crumb(single), "(1/1)", "no source counter with a single source")
}

// Wrapping h/l back onto the All page must not leave the breadcrumb naming
// one source while the cursor sits on another source's row.
func TestPagingBackToAllKeepsCursorInScope(t *testing.T) {
	m, _ := newTwoNSModel(t)
	m = press(m, keyTab)                         // other
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // web
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // wraps to All
	require.Contains(t, crumb(m), "[install (All)]")
	assert.Contains(t, crumb(m), "com.example.other", "scope survives the wrap")
	assert.Contains(t, cursorLine(m.View()), "com.example.other",
		"the cursor sits on the scoped source's row, not row 0 of another source")
}

func TestTabIsInTheFooterAndHelp(t *testing.T) {
	m, _ := newTwoNSModel(t)
	assert.Contains(t, m.View(), "tab source", "the footer advertises the source key")
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	assert.Contains(t, m.View(), "shift+tab", "the help overlay lists both directions")
}

// manyCatsYAML has ten categories — far wider than a 40-column breadcrumb.
const manyCatsYAML = `
namespace: com.example.tui-test
sets:
  - area: install
    features:
      - {path: install.alpha.x, description: a, boolDefault: true}
      - {path: install.bravo.x, description: b, boolDefault: true}
      - {path: install.charlie.x, description: c, boolDefault: true}
      - {path: install.delta.x, description: d, boolDefault: true}
      - {path: install.echo.x, description: e, boolDefault: true}
      - {path: install.foxtrot.x, description: f, boolDefault: true}
      - {path: install.golf.x, description: g, boolDefault: true}
      - {path: install.hotel.x, description: h, boolDefault: true}
      - {path: install.india.x, description: i, boolDefault: true}
      - {path: install.juliet.x, description: j, boolDefault: true}
`

func newManyCatsModel(t *testing.T, width int) tea.Model {
	t.Helper()
	r, p := newResolver(t, tuiWorld{repo: manyCatsYAML})
	items, err := r.All()
	require.NoError(t, err)
	var m tea.Model = tui.NewModel(items, p)
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	return m
}

func TestBreadcrumbScrollsToKeepActivePageVisible(t *testing.T) {
	const width = 40
	m := newManyCatsModel(t, width)
	c := crumb(m)
	require.Contains(t, c, "[install (All)]")
	assert.NotContains(t, c, "juliet", "the far categories do not fit yet")
	assert.Contains(t, c, "›", "a right marker says more pages are off-screen")
	assert.LessOrEqual(t, lipgloss.Width(c), width, "the breadcrumb fits the terminal")

	cats := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet"}
	for _, cat := range cats {
		m = press(m, tea.KeyMsg{Type: tea.KeyRight})
		c = crumb(m)
		assert.Contains(t, c, "["+cat+"]", "l onto %q keeps it on screen", cat)
		assert.LessOrEqual(t, lipgloss.Width(c), width, "breadcrumb on %q fits the terminal", cat)
	}
	assert.NotContains(t, c, "install (All)", "the leftmost pages scrolled off")
	assert.NotContains(t, c, "alpha")
	assert.Contains(t, c, "‹", "a left marker says pages are off-screen to the left")
	assert.NotContains(t, c, "›", "nothing is left to the right of the last page")

	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // wrap to All
	c = crumb(m)
	assert.Contains(t, c, "[install (All)]", "wrapping scrolls back to the start")
	assert.NotContains(t, c, "‹")

	m = press(m, tea.KeyMsg{Type: tea.KeyLeft}) // wrap to the last page
	assert.Contains(t, crumb(m), "[juliet]")
}

// The window only moves when the active page would leave it (like the row
// viewport), so stepping back left inside the window does not scroll.
func TestBreadcrumbScrollIsSticky(t *testing.T) {
	m := newManyCatsModel(t, 40)
	for range 10 {
		m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // to juliet
	}
	atEnd := crumb(m)
	m = press(m, tea.KeyMsg{Type: tea.KeyLeft}) // india — still inside the window
	c := crumb(m)
	assert.Contains(t, c, "[india]")
	assert.Contains(t, c, "juliet", "stepping left inside the window does not scroll it")
	assert.Equal(t, lipgloss.Width(atEnd), lipgloss.Width(c), "same window, only the brackets moved")
}

func TestBreadcrumbUnboundedWithoutWindowSize(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: manyCatsYAML})
	items, err := r.All()
	require.NoError(t, err)
	c := crumb(tui.NewModel(items, p))
	assert.Contains(t, c, "alpha")
	assert.Contains(t, c, "juliet", "no width known yet: every page renders")
	assert.NotContains(t, c, "›")
}
