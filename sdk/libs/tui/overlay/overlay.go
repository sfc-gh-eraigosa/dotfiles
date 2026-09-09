// Package overlay renders the help overlay from a keymap and drives a
// yes/no confirm dialog. Colors come from the caller's Palette; the lib
// never picks them.
package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/tui/keymap"
)

// Palette is the tool's theme adapter (four styles are enough for chrome).
type Palette interface {
	Dim(string) string
	Bold(string) string
	Accent(string) string
	Err(string) string
}

// Plain is the NO_COLOR / test palette: text unchanged.
type Plain struct{}

func (Plain) Dim(s string) string    { return s }
func (Plain) Bold(s string) string   { return s }
func (Plain) Accent(s string) string { return s }
func (Plain) Err(s string) string    { return s }

// Section is an extra block under the key table (gff's SOURCES, for example).
type Section struct {
	Title string
	Lines []string
}

// Help renders: title, one row per binding ("  <icon> <keys> <help>"), each
// section, then the close hint. Icons are optional; columns stay aligned.
func Help(p Palette, title string, m keymap.Map, closeHint string, sections ...Section) string {
	var b strings.Builder
	b.WriteString(p.Bold(title))
	b.WriteString("\n")
	rows := m.HelpRows()
	if len(rows) > 0 {
		b.WriteString("\n")
	}
	for _, r := range rows {
		icon := r.Icon
		if icon == "" {
			icon = " "
		}
		fmt.Fprintf(&b, "  %s %-18s %s\n", icon, r.Keys, p.Dim(r.Help))
	}
	for _, s := range sections {
		b.WriteString("\n")
		b.WriteString(p.Bold(s.Title))
		b.WriteString("\n")
		for _, l := range s.Lines {
			b.WriteString("  " + l + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(p.Dim(closeHint))
	return b.String()
}

// Decision is the outcome of a confirm key.
type Decision int

const (
	No Decision = iota
	Yes
)

// Confirm is a single-key yes/no dialog. Anything not a yes key declines —
// declining must be the safe default.
type Confirm struct {
	Title    string
	Lines    []string
	YesLabel string   // default "confirm"
	YesKeys  []string // default enter, y
	NoKeys   []string // default esc, n (documented; any other key also declines)
}

func (c Confirm) yes() []string {
	if len(c.YesKeys) == 0 {
		return []string{"enter", "y"}
	}
	return c.YesKeys
}

func (c Confirm) no() []string {
	if len(c.NoKeys) == 0 {
		return []string{"esc", "n"}
	}
	return c.NoKeys
}

// Key decides.
func (c Confirm) Key(msg tea.KeyMsg) Decision {
	name := keymap.KeyName(msg)
	for _, y := range c.yes() {
		if y == name {
			return Yes
		}
	}
	return No
}

// Render is the dialog body: title, indented lines, the two choices.
func (c Confirm) Render(p Palette) string {
	label := c.YesLabel
	if label == "" {
		label = "confirm"
	}
	var b strings.Builder
	b.WriteString(p.Bold(c.Title))
	b.WriteString("\n")
	for _, l := range c.Lines {
		b.WriteString("  " + p.Accent(l) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(p.Dim(strings.Join(c.yes(), "/") + " " + label + " · " + strings.Join(c.no(), "/") + " cancel"))
	return b.String()
}
