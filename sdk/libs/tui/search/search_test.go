package search

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func r(s string) tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func k(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

var rows = []string{"install.ai.claude Claude CLI", "install.ai.teams AI teams", "install.pkg.manager Package manager", "shell.zsh Zsh"}

func hit(s *State) func(int) bool {
	return func(i int) bool { return s.Re != nil && s.Re.MatchString(rows[i]) }
}

func typeInto(s *State, text string) {
	for _, c := range text {
		s.Key(r(string(c)))
	}
}

func TestCompileSmartcaseAndErrors(t *testing.T) {
	re, err := Compile("claude")
	require.NoError(t, err)
	assert.True(t, re.MatchString("Claude CLI"))
	re, err = Compile("Claude")
	require.NoError(t, err)
	assert.False(t, re.MatchString("claude"))
	re, err = Compile("")
	require.NoError(t, err)
	assert.Nil(t, re)
	_, err = Compile("[ai")
	require.Error(t, err)
	assert.Equal(t, "missing closing ]: `[ai`", err.Error(), "parser prefix stripped, fragment kept")
	re, err = Compile("\\.\\(")
	require.NoError(t, err)
	assert.True(t, re.MatchString("A.("), "symbols-only pattern is case-insensitive and still valid")
}

func TestTypingRecomputesAndInvalidKeepsPreviousRe(t *testing.T) {
	var s State
	s.Start(0, 0)
	typeInto(&s, "ai")
	s.Collect(len(rows), hit(&s))
	assert.Equal(t, []int{0, 1}, s.Matches)
	assert.Equal(t, Typed, s.Key(r("[")))
	assert.Contains(t, s.Err, "missing closing ]")
	s.Collect(len(rows), hit(&s))
	assert.Equal(t, []int{0, 1}, s.Matches, "previous Re kept")
	s.Key(k(tea.KeyBackspace))
	assert.Equal(t, "", s.Err)
}

func TestModeKeysAreIgnoredNotTyped(t *testing.T) {
	var s State
	s.Start(0, 0)
	assert.Equal(t, Ignored, s.Key(k(tea.KeyTab)))
	assert.Equal(t, Typed, s.Key(r("q")))
	assert.Equal(t, "q", s.Input.String())
}

func TestFirstAndNextWrap(t *testing.T) {
	var s State
	s.Start(2, 0)
	typeInto(&s, "install")
	s.Collect(len(rows), hit(&s))
	i, ok := s.First(2)
	assert.True(t, ok)
	assert.Equal(t, 2, i, "first match at or after the anchor")
	i, _ = s.First(3)
	assert.Equal(t, 0, i, "wraps to the top")
	i, _ = s.Next(2, 1)
	assert.Equal(t, 0, i)
	i, _ = s.Next(0, -1)
	assert.Equal(t, 2, i)
	var empty State
	_, ok = empty.Next(0, 1)
	assert.False(t, ok)
}

func TestCommitCancelHideRearmBadge(t *testing.T) {
	var s State
	s.Start(3, 1)
	typeInto(&s, "ai")
	s.Collect(len(rows), hit(&s))
	assert.Equal(t, Submitted, s.Key(k(tea.KeyEnter)))
	committed, notFound := s.Commit()
	assert.True(t, committed)
	assert.False(t, notFound)
	assert.Equal(t, "ai", s.Pattern)
	assert.Equal(t, "", s.Input.String())
	assert.Equal(t, "/ai [1/2]", s.Badge(0))
	assert.Equal(t, "/ai [-/2]", s.Badge(3))
	assert.True(t, s.IsMatch(1))

	s.Hide()
	assert.False(t, s.Visible)
	assert.Equal(t, "", s.Badge(0))
	assert.Equal(t, "ai", s.Pattern, "pattern survives :noh")
	assert.True(t, s.Rearm())
	assert.True(t, s.Visible)
	s.Collect(len(rows), hit(&s))
	assert.Equal(t, []int{0, 1}, s.Matches)

	s.Start(2, 0)
	typeInto(&s, "zzz")
	s.Collect(len(rows), hit(&s))
	committed, notFound = s.Commit()
	assert.True(t, committed)
	assert.True(t, notFound)

	s.Start(2, 5)
	typeInto(&s, "[")
	committed, _ = s.Commit()
	assert.False(t, committed, "an outstanding error refuses to commit")
	assert.Equal(t, "zzz", s.Pattern)
	assert.Equal(t, Cancelled, s.Key(k(tea.KeyEscape)))
	pos, top := s.Cancel()
	assert.Equal(t, 2, pos)
	assert.Equal(t, 5, top)
	assert.Nil(t, s.Re)
	assert.Equal(t, "zzz", s.Pattern)

	var fresh State
	assert.False(t, fresh.Rearm(), "nothing to re-arm")
	s.Start(0, 0)
	committed, _ = s.Commit()
	assert.True(t, committed, "empty commit hides and clears the pattern")
	assert.Equal(t, "", s.Pattern)
}
