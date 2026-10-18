package configui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

type env struct{}

func (env) LookPath(b string) (string, error) {
	if b == "nvtop" || b == "nvitop" {
		return "", context.Canceled
	}
	return "/bin/" + b, nil
}
func (env) Run(context.Context, string) bool { return false }
func (env) DRMDrivers() []string             { return nil }

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	enter = tea.KeyMsg{Type: tea.KeyEnter}
	esc   = tea.KeyMsg{Type: tea.KeyEscape}
)

func newModel(t *testing.T) (*Model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	m := New(path, config.Default(), env{})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.Update(m.Init()()) // run the tool check synchronously
	return m, path
}

// send feeds keys, running any returned command once (probes, quit).
func send(m *Model, keys ...tea.KeyMsg) tea.Msg {
	var last tea.Msg
	for _, k := range keys {
		_, cmd := m.Update(k)
		if cmd != nil {
			last = cmd()
			if _, ok := last.(probedMsg); ok {
				m.Update(last)
			}
		}
	}
	return last
}

func typeLine(m *Model, s string) tea.Msg {
	var keys []tea.KeyMsg
	for _, r := range s {
		if r == ' ' {
			keys = append(keys, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
		} else {
			keys = append(keys, runes(string(r)))
		}
	}
	keys = append(keys, enter)
	return send(m, keys...)
}

func TestViewListsPanesWithToolCheckAndPreview(t *testing.T) {
	m, _ := newModel(t)
	v := m.View()
	for _, want := range []string{"prefix", "ctrl+b", "htop", "60%", "✓ htop", "✗ not shown: nvtop not installed", "every 1s", "Preview", "title      2 rows", "htop      35 rows", "gsl        3 rows"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestEnterEditsThePaneCommand(t *testing.T) {
	m, _ := newModel(t)
	send(m, runes("j"), runes("j"), runes("j"), enter) // rows: prefix, title, htop, gpu
	if got := m.cmd.Input.String(); got != "set panes.gpu.cmd nvtop" {
		t.Fatalf("prompt = %q", got)
	}
	// replace the value: clear with ctrl+u then type
	send(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	typeLine(m, "set panes.gpu.cmd btop")
	if m.Config().Panes[1].Cmd != "btop" || !m.Dirty() {
		t.Fatalf("edit not applied: %+v dirty=%v", m.Config().Panes[1], m.Dirty())
	}
	if !strings.Contains(m.View(), "✓ btop") {
		t.Fatal("tool check not re-run after the edit")
	}
}

func TestBadEditNamesFieldAndChangesNothing(t *testing.T) {
	m, _ := newModel(t)
	send(m, runes(":"))
	typeLine(m, "set panes.htop.size 0%")
	if !strings.Contains(m.View(), "panes.htop.size") || m.Dirty() {
		t.Fatalf("bad edit: dirty=%v view tail %q", m.Dirty(), m.View()[len(m.View())-80:])
	}
}

func TestReorderAddRemove(t *testing.T) {
	m, _ := newModel(t)
	send(m, runes("j"), runes("j"), runes("J")) // htop down
	if order(m) != "gpu,htop,gsl" || m.cur.Pos != 3 {
		t.Fatalf("J: %s cursor %d", order(m), m.cur.Pos)
	}
	send(m, runes("K"))
	if order(m) != "htop,gpu,gsl" {
		t.Fatalf("K: %s", order(m))
	}
	send(m, runes(":"))
	typeLine(m, "add logs 10% journalctl -f")
	if order(m) != "htop,logs,gpu,gsl" {
		t.Fatalf("add after cursor: %s", order(m))
	}
	send(m, runes("j"), runes("d")) // cursor on logs
	if !strings.Contains(m.View(), "Remove pane logs?") {
		t.Fatalf("no confirm: %s", m.View())
	}
	send(m, runes("x")) // anything but yes declines
	if order(m) != "htop,logs,gpu,gsl" {
		t.Fatal("declined remove still removed")
	}
	send(m, runes("d"), runes("y"))
	if order(m) != "htop,gpu,gsl" {
		t.Fatalf("confirmed remove: %s", order(m))
	}
}

func TestQuitGuardsUnsavedChanges(t *testing.T) {
	m, path := newModel(t)
	if _, ok := send(m, runes("q")).(tea.QuitMsg); !ok {
		t.Fatal("clean q did not quit")
	}
	m, path = newModel(t)
	send(m, runes(":"))
	typeLine(m, "set panes.htop.size 50%")
	if send(m, runes("q")) != nil {
		t.Fatal("q quit with unsaved changes")
	}
	send(m, esc) // decline
	send(m, runes(":"))
	if _, ok := typeLine(m, "q").(tea.QuitMsg); ok {
		t.Fatal(":q quit with unsaved changes")
	}
	send(m, runes(":"))
	if _, ok := typeLine(m, "wq").(tea.QuitMsg); !ok {
		t.Fatal(":wq did not quit")
	}
	got, err := config.Load(path)
	if err != nil || got.Panes[0].Size != "50%" {
		t.Fatalf("saved file: %+v %v", got.Panes[0], err)
	}
}

func TestTypingInPromptFiresNoKeys(t *testing.T) {
	m, _ := newModel(t)
	send(m, runes(":"), runes("q"), runes("J"), runes("d"))
	if m.mode != modeCommand || order(m) != "htop,gpu,gsl" {
		t.Fatalf("keys fired inside the prompt: mode=%v order=%s", m.mode, order(m))
	}
}

func TestHelpRendersEveryBinding(t *testing.T) {
	m, _ := newModel(t)
	send(m, runes("?"))
	v := m.View()
	for _, b := range Keys() {
		if !strings.Contains(v, b.Help) {
			t.Errorf("help missing %q", b.Help)
		}
	}
	if !strings.Contains(v, ":set") || !strings.Contains(v, ":wq") {
		t.Error("help missing the command list")
	}
	send(m, runes("z"))
	if m.mode != modeList {
		t.Fatal("any key did not close help")
	}
}

func order(m *Model) string {
	var n []string
	for _, p := range m.Config().Panes {
		n = append(n, p.Name)
	}
	return strings.Join(n, ",")
}

func TestFooterHintFitsAndGroupsKeys(t *testing.T) {
	hint := Keys().HeaderHint("  ")
	for _, want := range []string{"j/k move", "J/K reorder", "q quit", "enter edit", "s size", "a add", "d remove"} {
		if !strings.Contains(hint, want) {
			t.Errorf("footer %q missing %q", hint, want)
		}
	}
	if len(hint) > 110 {
		t.Errorf("footer is %d columns; keep it on one line of a 110-column terminal: %q", len(hint), hint)
	}
}
