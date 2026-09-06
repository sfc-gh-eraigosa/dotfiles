package overlay

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
	"github.com/stretchr/testify/assert"
)

func r(s string) tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func k(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

type tagged struct{}

func (tagged) Dim(s string) string    { return "<d>" + s + "</d>" }
func (tagged) Bold(s string) string   { return "<b>" + s + "</b>" }
func (tagged) Accent(s string) string { return "<a>" + s + "</a>" }
func (tagged) Err(s string) string    { return "<e>" + s + "</e>" }

func TestHelpRendersEveryBindingSectionsAndHint(t *testing.T) {
	m := keymap.Map{
		{Action: keymap.Down, Keys: []string{"j", "down"}, Help: "move down", Icon: "⬍"},
		{Action: keymap.Quit, Keys: []string{"q"}, Help: "quit"},
	}
	out := Help(Plain{}, "gff — keys", m, "esc/?/q close",
		Section{Title: "SOURCES", Lines: []string{"● com.example  @abc123"}})
	lines := strings.Split(out, "\n")
	assert.Equal(t, "gff — keys", lines[0])
	assert.Contains(t, out, fmt.Sprintf("  ⬍ %-18s move down", "j/down"))
	assert.Contains(t, out, fmt.Sprintf("    %-18s quit", "q"), "missing icon keeps the columns aligned")
	assert.Contains(t, out, "SOURCES\n  ● com.example  @abc123")
	assert.True(t, strings.HasSuffix(out, "esc/?/q close"))
	assert.Equal(t, "gff — keys\n\nesc/?/q close", Help(Plain{}, "gff — keys", keymap.Map{}, "esc/?/q close"))
}

func TestHelpUsesThePalette(t *testing.T) {
	m := keymap.Map{{Action: keymap.Quit, Keys: []string{"q"}, Help: "quit"}}
	out := Help(tagged{}, "T", m, "close")
	assert.Contains(t, out, "<b>T</b>")
	assert.Contains(t, out, "<d>quit</d>")
	assert.Contains(t, out, "<d>close</d>")
}

func TestConfirmKeys(t *testing.T) {
	c := Confirm{Title: "update 2 hosts", Lines: []string{"nano", "pi"}}
	assert.Equal(t, Yes, c.Key(k(tea.KeyEnter)))
	assert.Equal(t, Yes, c.Key(r("y")))
	assert.Equal(t, No, c.Key(r("n")))
	assert.Equal(t, No, c.Key(k(tea.KeyEscape)))
	assert.Equal(t, No, c.Key(r("x")), "anything else declines")
	custom := Confirm{YesKeys: []string{"Y"}, NoKeys: []string{"esc"}}
	assert.Equal(t, No, custom.Key(r("y")))
	assert.Equal(t, Yes, custom.Key(r("Y")))
}

func TestConfirmRender(t *testing.T) {
	c := Confirm{Title: "update 2 hosts → main", Lines: []string{"● nano", "● pi"}, YesLabel: "update"}
	out := c.Render(Plain{})
	assert.Equal(t, "update 2 hosts → main\n  ● nano\n  ● pi\n\nenter/y update · esc/n cancel", out)
	assert.Contains(t, Confirm{Title: "t"}.Render(Plain{}), "enter/y confirm · esc/n cancel")
}
