package tui_test

// Real-terminal key semantics: bubbletea delivers the spacebar as
// tea.KeyMsg{Type: tea.KeySpace} (key.go maps the ' ' rune to KeySpace), NOT as
// KeyRunes{' '}. The original teatest suite injected KeyRunes and therefore
// passed while the compiled binary ignored the spacebar entirely — caught by
// the p3+p4 tmux integration demo. These tests drive the Model with the exact
// KeyMsg shapes a real terminal produces.

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/tui"
	"github.com/stretchr/testify/require"
)

// press drives one key through Update, following the returned model.
func press(m tea.Model, msg tea.KeyMsg) tea.Model {
	next, _ := m.Update(msg)
	return next
}

func TestRealTerminalSpaceTogglesBool(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: minimalBoolYAML})
	items, err := r.All()
	require.NoError(t, err)

	var m tea.Model = tui.NewModel(items, p)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand area
	m = press(m, tea.KeyMsg{Type: tea.KeyDown})  // onto install.ai.claude
	m = press(m, tea.KeyMsg{Type: tea.KeySpace}) // REAL spacebar

	data, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err, "real-terminal KeySpace must write the user override")
	require.Contains(t, string(data), "install.ai.claude: false")
}

func TestRealTerminalSpaceDrivesPicker(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: multiChoiceYAML})
	items, err := r.All()
	require.NoError(t, err)

	var m tea.Model = tui.NewModel(items, p)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // expand area
	// Find the multi-choice row: walk down until the picker opens on KeySpace.
	opened := false
	for i := 0; i < len(items)+1 && !opened; i++ {
		m = press(m, tea.KeyMsg{Type: tea.KeyDown})
		m = press(m, tea.KeyMsg{Type: tea.KeySpace})
		if v := m.View(); v != "" && containsPickerHint(v) {
			opened = true
		}
	}
	require.True(t, opened, "KeySpace on a choice row must open the picker")

	m = press(m, tea.KeyMsg{Type: tea.KeyDown})  // second option
	m = press(m, tea.KeyMsg{Type: tea.KeySpace}) // REAL spacebar toggles entry
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // confirm

	data, err := os.ReadFile(p.UserOverride)
	require.NoError(t, err, "picker confirm after KeySpace toggle must write the override")
	require.NotEmpty(t, data)
}

// containsPickerHint reports whether the view is showing the picker overlay.
func containsPickerHint(v string) bool {
	for _, hint := range []string{"Space select", "Enter confirm", "esc cancel", "Esc cancel"} {
		if len(v) > 0 && contains(v, hint) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestToggleWriteErrorIsSurfacedNotSwallowed(t *testing.T) {
	r, p := newResolver(t, tuiWorld{repo: minimalBoolYAML})
	items, err := r.All()
	require.NoError(t, err)

	// Make the override destination unwritable.
	roDir := filepath.Join(t.TempDir(), "ro")
	require.NoError(t, os.Mkdir(roDir, 0o500))
	p.UserOverride = filepath.Join(roDir, "sub", "config.yaml")

	var m tea.Model = tui.NewModel(items, p)
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(m, tea.KeyMsg{Type: tea.KeySpace})

	view := m.View()
	require.Contains(t, view, "write failed", "a failed override write must be surfaced in the view")
	// The row must NOT optimistically flip to user-override on a failed write.
	require.NotContains(t, view, "user-override")
}
