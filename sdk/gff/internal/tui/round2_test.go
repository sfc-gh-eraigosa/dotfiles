package tui_test

// Second owner-review round on PR #187:
//  - the launch frame is clean; help/about/sources live behind ? or h in EVERY view
//  - the detail view can act: Space toggles/picks via the existing writers,
//    'u' clears the user override via overrides.Unset — no new write paths
//  - area rows are namespace-separated (one row per namespace+area) and the
//    breadcrumb rescopes to the cursor row's namespace

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const otherNSYAML = `
namespace: com.example.other
sets:
  - area: install
    features:
      - {path: install.web.nginx, description: Web server, boolDefault: true}
`

func newTwoNSModel(t *testing.T) (tea.Model, string) {
	t.Helper()
	r, p := newResolver(t, tuiWorld{repo: pagerYAML, userSnap: otherNSYAML})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	m.Explain = r.Explain
	m.Sources = []tui.SourceInfo{{Namespace: "com.example.other", URL: "https://example.com/other.git", Commit: "abc123"}}
	return m, p.UserOverride
}

func TestLaunchFrameIsCleanWithHelpHint(t *testing.T) {
	m := newPagerModel(t)
	v := m.View()
	assert.NotContains(t, v, "SOURCES", "sources live behind the help overlay now")
	assert.NotContains(t, v, "layered flags persisted", "no always-on about panel")
	assert.Contains(t, v, "? help", "the footer advertises the help overlay")
}

func TestHelpOverlayShowsAboutVersionAndSources(t *testing.T) {
	m, _ := newTwoNSModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	v := m.View()
	assert.Contains(t, v, "git fast features", "help shows the tool name/about")
	assert.Contains(t, v, "gff v", "help shows version info")
	assert.Contains(t, v, "SOURCES")
	assert.Contains(t, v, "https://example.com/other.git", "registry entries listed")
	assert.Contains(t, v, "com.example.tui-test", "discovered namespaces listed too")
	assert.Contains(t, v, "not registered", "discovered-but-unregistered origins are labeled")
	assert.Contains(t, v, "category", "view-specific key legend present")

	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	assert.Contains(t, m.View(), "[", "Esc closes help back to the list breadcrumb")
}

func TestHelpOverlayFromDetail(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // detail
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	v := m.View()
	assert.Contains(t, v, "clear", "detail help explains the u/clear action")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	assert.Contains(t, m.View(), "LAYERS", "Esc returns to the detail view")
}

func TestNamespaceSeparatedAreaRows(t *testing.T) {
	m, _ := newTwoNSModel(t)
	v := m.View()
	assert.Contains(t, v, "com.example.tui-test")
	assert.Contains(t, v, "com.example.other", "each namespace gets its own area row")

	// Expanding the first (repo/focus) namespace shows only its features.
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	v = m.View()
	assert.Contains(t, v, "install.ai.claude")
	assert.NotContains(t, v, "install.web.nginx", "the other namespace's features stay collapsed")
}

func TestBreadcrumbRescopesToCursorNamespace(t *testing.T) {
	m, _ := newTwoNSModel(t)
	// Move the cursor onto the second namespace's area row.
	m = press(m, tea.KeyMsg{Type: tea.KeyDown})
	v := m.View()
	assert.Contains(t, v, "web", "breadcrumb now lists the other namespace's categories")
	assert.NotContains(t, v, "pkg", "first namespace's categories are out of scope")
}

func TestDetailSpaceTogglesBoolViaExistingWriter(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // ai page
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // detail of install.ai.claude (true)
	m = press(m, tea.KeyMsg{Type: tea.KeySpace}) // toggle from the detail view

	v := m.View()
	assert.Contains(t, v, "LAYERS", "still in the detail view after acting")
	assert.Contains(t, v, "override: false", "the user-override layer row appeared")
	idx := strings.Index(v, "user-override")
	require.GreaterOrEqual(t, idx, 0)
	assert.Contains(t, v[idx:], "winning", "user-override is now the winner")
}

func TestDetailUClearsUserOverride(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: pagerYAML, usrOvr: "install.ai.claude: false\n"})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	m.Explain = r.Explain
	var tm tea.Model = m
	tm = press(tm, tea.KeyMsg{Type: tea.KeyRight}) // ai
	tm = press(tm, tea.KeyMsg{Type: tea.KeyEnter}) // detail: user-override false winning
	require.Contains(t, tm.View(), "override: false")

	tm = press(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}) // clear
	v := tm.View()
	idx := strings.Index(v, "repo-live")
	require.GreaterOrEqual(t, idx, 0)
	assert.Contains(t, v[idx:], "winning", "default restored — repo-live wins again")

	data, _ := os.ReadFile(p.UserOverride)
	assert.NotContains(t, string(data), "install.ai.claude", "the override entry is gone from the file")
}

func TestDetailSpaceOnChoiceOpensPickerAndReturnsToDetail(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // ai
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // pkg
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // detail of install.pkg.manager
	m = press(m, tea.KeyMsg{Type: tea.KeySpace}) // open picker FROM detail
	assert.Contains(t, m.View(), "Pick option", "picker overlay opened from detail")

	m = press(m, tea.KeyMsg{Type: tea.KeyDown})  // apt
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // radio select + confirm
	v := m.View()
	assert.Contains(t, v, "LAYERS", "picker returns to the detail view")
	assert.Contains(t, v, "override: apt", "the selection is visible in the layer table")
}

func TestPickerEscReturnsToDetail(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // ai
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // pkg
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // detail
	m = press(m, tea.KeyMsg{Type: tea.KeySpace}) // picker from detail
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	assert.Contains(t, m.View(), "LAYERS", "Esc from a detail-opened picker returns to detail")
}

func TestHelpFromPickerDescribesPickerKeys(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})                     // pkg page
	m = press(m, tea.KeyMsg{Type: tea.KeySpace})                     // picker from list
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}) // help overlay
	v := m.View()
	assert.Contains(t, v, "option picker", "help overlay describes the picker view")
	assert.Contains(t, v, "Enter select")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	assert.Contains(t, m.View(), "Pick option", "Esc returns to the picker")
}

// Owner-reported bug: toggling a feature with Space on a CATEGORY page made
// the row disappear. Cause: the list toggle rebuilt the item as a bare
// Resolved literal, silently dropping the unexported namespace — the category
// filter (Namespace() == scopeNS) then excluded it from the rebuilt rows.
func TestSpaceToggleOnCategoryPageKeepsTheRow(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // ai page
	require.Contains(t, m.View(), "install.ai.claude")

	m = press(m, tea.KeyMsg{Type: tea.KeySpace}) // toggle the first ai feature
	v := m.View()
	assert.Contains(t, v, "install.ai.claude", "the toggled row must NOT disappear")
	assert.Contains(t, v, "false", "the toggle itself must have applied")
	assert.Contains(t, v, "user-override", "provenance updates in place")
}

func TestSpaceToggleOnAllPageKeepsRowInItsNamespaceGroup(t *testing.T) {
	m, _ := newTwoNSModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand the first (tui-test) group
	m = press(m, tea.KeyMsg{Type: tea.KeyDown})  // onto install.ai.claude
	m = press(m, tea.KeyMsg{Type: tea.KeySpace}) // toggle
	v := m.View()
	assert.Contains(t, v, "install.ai.claude", "row stays inside its namespace group")
	assert.NotContains(t, v, "▶ install  · \n", "no phantom empty-namespace group appears")
}

// Owner-reported: help opened from inside a namespace scope must SAY which
// source that scope belongs to — the SOURCES list marks the current one.
func helpLineWith(v, needle string) string {
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func TestHelpMarksCurrentScopeSource(t *testing.T) {
	m, _ := newTwoNSModel(t)

	// Initial scope: the repo namespace (com.example.tui-test).
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	line := helpLineWith(m.View(), "current scope")
	require.NotEmpty(t, line, "help must mark the current scope's source")
	assert.Contains(t, line, "com.example.tui-test")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})

	// Rescope to the second namespace, reopen help: the marker must move.
	m = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	line = helpLineWith(m.View(), "current scope")
	require.NotEmpty(t, line)
	assert.Contains(t, line, "com.example.other", "marker follows the breadcrumb scope")
	assert.NotContains(t, line, "com.example.tui-test")
}

func TestHelpFromDetailMarksTheItemsSource(t *testing.T) {
	m, _ := newTwoNSModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand tui-test group
	m = press(m, tea.KeyMsg{Type: tea.KeyDown})  // install.ai.claude
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // detail
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	line := helpLineWith(m.View(), "current scope")
	require.NotEmpty(t, line)
	assert.Contains(t, line, "com.example.tui-test", "detail help marks the item's own source")
}

// Owner ask: 'u' clears the user override from the LIST view too, mirroring
// the detail view — same overrides.Unset path.
func TestListUClearsUserOverrideOnCursorRow(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: pagerYAML, usrOvr: "install.ai.claude: false\n"})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	m.Explain = r.Explain
	var tm tea.Model = m
	tm = press(tm, tea.KeyMsg{Type: tea.KeyRight}) // ai page; cursor 0 = claude
	require.Contains(t, tm.View(), "user-override")

	tm = press(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	v := tm.View()
	assert.Contains(t, v, "install.ai.claude", "row stays after clearing")
	assert.Contains(t, v, "repo-live", "provenance reverts to the definition layer")
	assert.NotContains(t, v, "user-override", "override cleared from the list view")

	data, _ := os.ReadFile(p.UserOverride)
	assert.NotContains(t, string(data), "install.ai.claude")
}

func TestListUOnAreaRowIsHarmless(t *testing.T) {
	m, _ := newTwoNSModel(t)
	before := m.View()
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}) // cursor on an area row
	assert.Equal(t, before, m.View(), "u on an area row is a no-op")
}
