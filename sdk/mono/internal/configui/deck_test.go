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
