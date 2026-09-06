package tui_test

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Contains(t, m.View(), ":set install.ai.claude false▌")
	m, _ = enter(m)
	data, err := os.ReadFile(ovr)
	require.NoError(t, err)
	assert.Contains(t, string(data), "install.ai.claude: false")
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand install
	assert.Contains(t, m.View(), "user-override")
	assert.NotContains(t, m.View(), "▌")
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
	assert.Contains(t, m.View(), "KEYS")
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

func TestColonEscCancelsAndTypedLettersAreText(t *testing.T) {
	m, ovr := newCmdModel(t)
	m = typeKeys(m, ":set install.ai.claude false")
	m = press(m, tea.KeyMsg{Type: tea.KeyEscape})
	assert.NotContains(t, m.View(), "▌")
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
	m = press(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Contains(t, m.View(), ":set install.ai.claude▌")

	m, _ = newCmdModel(t)
	m = press(m, tea.KeyMsg{Type: tea.KeyRight})
	m = press(m, tea.KeyMsg{Type: tea.KeyRight}) // pkg page
	m = typeKeys(m, ":unset ")
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":unset install.pkg.manager▌", "only the page's keys complete")
	m = typeKeys(m, " ")
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	assert.Contains(t, m.View(), ":unset install.pkg.manager ▌", "the value position does not complete")
}
