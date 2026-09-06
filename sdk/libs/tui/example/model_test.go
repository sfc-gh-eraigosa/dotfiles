//go:build example

package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func r(s string) tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func k(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func drive(m tea.Model, keys ...tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, key := range keys {
		m, cmd = m.Update(key)
	}
	return m, cmd
}

func TestExampleComposesAllPackages(t *testing.T) {
	var m tea.Model = newModel(30)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})

	m, _ = drive(m, r("j"), r("j"), k(tea.KeyCtrlD)) // 0 → 2 → 6 (half of the 8-row body)
	assert.Contains(t, cursorLine(m.View()), "row 07")

	m, _ = drive(m, r("/"), r("2"), r("5"))
	assert.Contains(t, m.View(), "/25▌")
	m, _ = drive(m, k(tea.KeyEnter), r("n"))
	assert.Contains(t, cursorLine(m.View()), "row 25", "search commits; n wraps onto the only match")
	assert.Contains(t, m.View(), "/25 [1/1]")

	m, _ = drive(m, r("?"))
	for _, want := range []string{"j/down", "gg", "ctrl+d", "/", ":", "q/ctrl+c"} {
		assert.Contains(t, m.View(), want, "help lists %q", want)
	}
	m, _ = drive(m, k(tea.KeyEscape))

	m, _ = drive(m, r(":"))
	for _, c := range "mark row 25" {
		m, _ = drive(m, r(string(c)))
	}
	m, _ = drive(m, k(tea.KeyTab)) // single candidate cycles onto itself
	assert.Contains(t, m.View(), ":mark row 25▌")
	m, _ = drive(m, k(tea.KeyEnter))
	assert.Contains(t, m.View(), "marked row 25", ":mark <row> completed and ran")

	m, _ = drive(m, r("d"))
	assert.Contains(t, m.View(), "delete row 25?")
	m, _ = drive(m, r("x"))
	assert.NotContains(t, m.View(), "delete row 25?", "x declines")
	assert.Contains(t, m.View(), "row 25")

	_, cmd := drive(m, r(":"), r("q"), k(tea.KeyEnter))
	assert.NotNil(t, cmd, ":q quits")
}

func cursorLine(v string) string {
	for _, l := range strings.Split(v, "\n") {
		if strings.HasPrefix(l, "> ") {
			return l
		}
	}
	return ""
}
