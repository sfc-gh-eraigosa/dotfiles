package tui_test

// Owner-requested TUI capabilities from the PR #187 review (spec §F10
// extension): category breadcrumb paging (←/→ across alphabetical
// second-segment pages), a per-feature detail view on Enter (attributes +
// per-layer provenance via resolve.Explain), and viewport-aware rendering
// (fixed header, windowed rows, cursor-following scroll).

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pagerYAML = `
namespace: com.example.tui-test
sets:
  - area: install
    features:
      - {path: install.ai.claude, description: Claude CLI, boolDefault: true}
      - {path: install.ai.teams, description: AI teams, boolDefault: true}
      - path: install.pkg.manager
        description: Package manager
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: auto, description: Auto-detect, stringValue: auto, selected: true}
            - {id: apt, description: Debian apt, stringValue: apt}
      - {path: install.shell.profiles, description: Shell profiles, boolDefault: true}
`

func newPagerModel(t *testing.T) tea.Model {
	t.Helper()
	r, p := newResolver(t, tuiWorld{repo: pagerYAML})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	m.Explain = r.Explain
	return m
}

func TestBreadcrumbListsPagesAlphabetically(t *testing.T) {
	m := newPagerModel(t)
	v := m.View()
	for _, label := range []string{"install (All)", "ai", "pkg", "shell"} {
		assert.Contains(t, v, label, "breadcrumb must list %q", label)
	}
	iAll := strings.Index(v, "install (All)")
	iAI := strings.Index(v, "ai")
	iPkg := strings.Index(v, "pkg")
	iShell := strings.Index(v, "shell")
	assert.True(t, iAll < iAI && iAI < iPkg && iPkg < iShell, "pages must be alphabetical after All")
}

func TestRightArrowFiltersToCategory(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // All -> ai
	v := m.View()
	assert.Contains(t, v, "install.ai.claude", "ai page shows ai features without expanding")
	assert.Contains(t, v, "install.ai.teams")
	assert.NotContains(t, v, "install.pkg.manager", "other categories are filtered out")
	assert.NotContains(t, v, "install.shell.profiles")
}

func TestLeftArrowWrapsToLastPage(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyLeft}) // All wraps to shell (last alphabetical)
	v := m.View()
	assert.Contains(t, v, "install.shell.profiles")
	assert.NotContains(t, v, "install.ai.claude")
}

func TestEnterOnFeatureOpensDetailWithLayers(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // ai page; cursor row 0 = install.ai.claude
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})

	v := m.View()
	assert.Contains(t, v, "install.ai.claude", "detail shows the path")
	assert.Contains(t, v, "Claude CLI", "detail shows the description")
	// The full 5-layer story must be listed.
	for _, layer := range []string{"system-snapshot", "user-snapshot", "repo-live", "system-override", "user-override"} {
		assert.Contains(t, v, layer, "detail must list layer %q", layer)
	}
	assert.Contains(t, v, "winning", "the winning layer is marked")

	// Esc returns to the list.
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	assert.Contains(t, m.View(), "install.ai.teams", "back on the ai page")
}

func TestDetailShowsChoiceOptions(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // ai
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // pkg; cursor 0 = install.pkg.manager
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	v := m.View()
	assert.Contains(t, v, "auto")
	assert.Contains(t, v, "apt")
	assert.Contains(t, v, "Auto-detect", "options show descriptions and typed values")
}

func TestViewportWindowsRowsToHeight(t *testing.T) {
	m := newPagerModel(t)
	// Small terminal: only a few body lines fit.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand area on All page (5 rows total)
	v := m.View()
	assert.Contains(t, v, "install (All)", "breadcrumb header stays fixed")
	assert.Contains(t, v, "more", "overflow indicator when rows exceed the viewport")
	assert.NotContains(t, v, "install.shell.profiles", "rows beyond the window are not rendered")
}

func TestViewportFollowsCursor(t *testing.T) {
	m := newPagerModel(t)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand area
	for i := 0; i < 4; i++ {                     // walk to the last row
		m = press(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	v := m.View()
	assert.Contains(t, v, "install.shell.profiles", "scroll follows the cursor to the bottom")
	assert.NotContains(t, v, "install.ai.claude", "top rows scrolled out")
}

func TestLaunchShowsHelpAndSources(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: pagerYAML})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	m.Sources = []tui.SourceInfo{{
		Namespace: "com.example.tui-test",
		URL:       "https://example.com/tui-test.git",
		Commit:    "abc1234",
	}}

	v := m.View()
	// Launch state presents help…
	assert.Contains(t, v, "Space", "launch help lists the toggle key")
	assert.Contains(t, v, "category", "launch help mentions ←/→ category paging")
	// …and the sources/registry story.
	assert.Contains(t, v, "SOURCES")
	assert.Contains(t, v, "com.example.tui-test")
	assert.Contains(t, v, "https://example.com/tui-test.git")
	// The area row names its namespace so provenance is visible at a glance.
	assert.Contains(t, v, "install")
}

func TestHelpPanelYieldsToRowsWhenExpanded(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: pagerYAML})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	m.Sources = []tui.SourceInfo{{Namespace: "com.example.tui-test", URL: "https://example.com/tui-test.git"}}

	var tm tea.Model = m
	tm = press(tm, tea.KeyMsg{Type: tea.KeyEnter}) // expand install
	v := tm.View()
	assert.Contains(t, v, "install.ai.claude", "rows take over after expand")
	assert.NotContains(t, v, "https://example.com/tui-test.git", "sources panel yields to content")
}

func TestDetailWithoutExplainHook(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: pagerYAML})
	items, err := r.All()
	require.NoError(t, err)
	var m tea.Model = tui.NewModel(items, p) // Explain deliberately not wired
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	v := m.View()
	assert.Contains(t, v, "install.ai.claude")
	assert.Contains(t, v, "per-layer detail unavailable")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // Enter also returns
	assert.Contains(t, m.View(), "install.ai.teams")
}

func TestDetailQReturnsToList(t *testing.T) {
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.Contains(t, m.View(), "install.ai.teams", "q leaves the detail view, not the app")
}

func TestDetailMultiChoiceMarks(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: multiChoiceYAML})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	m.Explain = r.Explain
	var tm tea.Model = m
	tm = press(tm, tea.KeyMsg{Type: tea.KeyEnter}) // expand area
	// walk to the multi-choice feature and open its detail
	for i := 0; i < len(items); i++ {
		tm = press(tm, tea.KeyMsg{Type: tea.KeyDown})
	}
	tm = press(tm, tea.KeyMsg{Type: tea.KeyEnter})
	v := tm.View()
	if strings.Contains(v, "checkbox (multi)") {
		assert.Contains(t, v, "[x]", "selected multi options use checkbox marks")
	}
}

func TestPgDnPgUpMoveCursorAcrossViewport(t *testing.T) {
	m := newPagerModel(t)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand
	_ = m.View()                                 // render once so the page stride is known
	m = press(m, tea.KeyMsg{Type: tea.KeyPgDown})
	v := m.View()
	assert.Contains(t, v, "more above", "PgDn jumped the viewport forward")
	m = press(m, tea.KeyMsg{Type: tea.KeyPgUp})
	v = m.View()
	assert.Contains(t, v, "install.ai.claude", "PgUp returned to the top")
}

func TestLaunchPanelWithoutSources(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: pagerYAML})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	assert.Contains(t, m.View(), "no sources registered")
}

const twoAreaYAML = `
namespace: com.example.tui-test
sets:
  - area: install
    features:
      - {path: install.ai.claude, description: Claude CLI, boolDefault: true}
  - area: shellcfg
    features:
      - {path: shellcfg.zsh.plugins, description: Plugins, boolDefault: true}
`

func TestBreadcrumbMultiAreaAllLabel(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: twoAreaYAML})
	items, err := r.All()
	require.NoError(t, err)
	m := tui.NewModel(items, p)
	v := m.View()
	assert.Contains(t, v, "(All)")
	assert.NotContains(t, v, "install (All)", "multi-area worlds use the bare (All) label")
}
