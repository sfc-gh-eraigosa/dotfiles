package configui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

func deckCfg() config.Config {
	c := config.Default()
	c.Middle = config.MiddleSplit
	c.Panes[0], c.Panes[1] = c.Panes[1], c.Panes[0] // htop, mid, gsl
	c.Panes[1] = config.Pane{Name: "mid", Size: "30%", Tools: []config.Tool{
		{Name: "gpu", Views: []config.View{{Name: "nvtop", Cmd: "nvtop"}}},
		{Name: "gdash", Zoom: true, Probe: "gdash doctor --quiet", Views: []config.View{{Name: "conductor", Cmd: "gdash conductor"}, {Name: "ci", Cmd: "gdash ci"}}},
	}}
	return c
}

// The config screen shows a deck as a row with its tools and views under it.
func TestDeckRowShowsToolsAndViews(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "c.json"), deckCfg(), env{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(m.Init()())
	v := m.View()
	for _, want := range []string{"mid", "deck: 2 tools", "gpu", "nvtop", "gdash (zoom)", "conductor", "ci", "middle: split"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

// enter on a deck edits the first view's command, through the same set.
func TestEnterOnDeckEditsAView(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "c.json"), deckCfg(), env{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(m.Init()())
	send(m, runes("j"), runes("j"), runes("j"), enter) // prefix, title, htop, mid
	if got := m.cmd.Input.String(); got != "set panes.mid.tools.gpu.views.nvtop.cmd nvtop" {
		t.Fatalf("prompt = %q", got)
	}
}

// Deck detail lines count against the screen: a tall deck on a short
// terminal scrolls instead of pushing the list off the screen.
func TestDeckDetailFitsTheScreen(t *testing.T) {
	c := deckCfg()
	for i := 0; i < 8; i++ {
		c.Panes[1].Tools = append(c.Panes[1].Tools, config.Tool{Name: "t" + string(rune('a'+i)), Views: []config.View{{Name: "v", Cmd: "true"}}})
	}
	m := New(filepath.Join(t.TempDir(), "c.json"), c, env{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m.Update(m.Init()())
	for _, at := range []int{0, 3, 4} {
		m.cur.Pos = 0
		for i := 0; i < at; i++ {
			send(m, runes("j"))
		}
		v := m.View()
		if n := strings.Count(v, "\n") + 1; n > 24 {
			t.Fatalf("cursor at %d: %d lines on a 24-row screen:\n%s", at, n, v)
		}
		if !strings.Contains(v, "\n> ") {
			t.Fatalf("cursor at %d: the selected row is off screen:\n%s", at, v)
		}
	}
}

// With middle: single the preview shows what mono runs — one merged "mid".
func TestPreviewFollowsMiddleSingle(t *testing.T) {
	c := deckCfg()
	c.Middle = config.MiddleSingle
	m := New(filepath.Join(t.TempDir(), "c.json"), c, env{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(m.Init()())
	v := m.View()
	pv := v[strings.LastIndex(v, "Preview"):] // last: the temp path names the test
	if strings.Contains(pv, "htop") || !strings.Contains(pv, "mid") {
		t.Fatalf("preview is not the merged middle:\n%s", pv)
	}
}
