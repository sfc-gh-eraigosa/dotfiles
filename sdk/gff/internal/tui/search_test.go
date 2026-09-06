package tui_test

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/tui"
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
// The cursor row keeps its "> " marker (cursor wins), so N matches with the
// cursor on one of them show N-1 stars. Tests run with NO_COLOR=1.
func gutterLines(v string) []string {
	var out []string
	for _, l := range strings.Split(v, "\n") {
		if strings.HasPrefix(l, "* ") {
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
	assert.Contains(t, cursorLine(v), "install.ai.claude", "cursor on the first match after the anchor")
	require.Len(t, gutterLines(v), 1)
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
	assert.Len(t, gutterLines(m.View()), 1)
	m = typeKeys(m, "[")
	v := m.View()
	assert.Contains(t, v, "missing closing ]", "inline error under the prompt")
	assert.Len(t, gutterLines(v), 1, "previous matches kept")
	assert.Contains(t, cursorLine(v), "install.ai.claude")
	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	assert.NotContains(t, m.View(), "missing closing", "fixing the pattern clears the error")
}

func TestSlashSearchEscRestoresCursorAndClears(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t)
	m = typeKeys(m, "/shell")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	v := m.View()
	assert.Contains(t, cursorLine(v), "▼ install", "cursor back on the header where / was pressed")
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
	assert.Contains(t, v, "/ai [1/2]")
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
	assert.Len(t, gutterLines(m.View()), 1, "only the page's two rows match (cursor on one)")
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

// F3b: Enter with an outstanding compile error must not commit — the lib
// refuses, the model surfaces the error, and the previous pattern survives.
func TestSlashSearchEnterOnInvalidPatternDoesNotCommit(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t)
	m = typeKeys(m, "/ai")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	assert.Contains(t, m.View(), "/ai [1/2]", "first pattern committed")
	m = typeKeys(m, "/[")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	v := m.View()
	assert.Contains(t, v, "invalid pattern: missing closing ]")
	assert.NotContains(t, v, "▌", "the prompt closed")
	m = typeKeys(m, "n")
	assert.Contains(t, cursorLine(m.View()), "install.ai.", "n still hops the previous pattern")
}

// F5a: n on a page where the committed pattern matches nothing reports it
// instead of moving the cursor.
func TestNOnAPageWithoutMatchesReportsNotFound(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newPagerModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // ai page
	m = typeKeys(m, "/claude")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // pkg page: no "claude" row
	before := cursorLine(m.View())
	m = typeKeys(m, "n")
	assert.Contains(t, m.View(), "pattern not found: claude")
	assert.Equal(t, before, cursorLine(m.View()), "cursor did not move")
}
