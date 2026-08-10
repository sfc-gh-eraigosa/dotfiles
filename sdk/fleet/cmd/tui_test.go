package cmd

import (
	"strings"
	"testing"
)

func TestTUIModelRendersOneLinePerHost(t *testing.T) {
	m := newModel([]Row{
		{Alias: "alpha", Class: "up-to-date"},
		{Alias: "beta", Class: "behind", Behind: 3},
	}, testNow)
	view := m.View()
	for _, want := range []string{"alpha", "beta", "behind 3"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestTUISelectionMovesWithinBounds(t *testing.T) {
	m := newModel([]Row{{Alias: "a"}, {Alias: "b"}}, testNow)
	m = m.moveCursor(-1)
	if m.cursor != 0 {
		t.Fatalf("cursor went above 0: %d", m.cursor)
	}
	m = m.moveCursor(1)
	m = m.moveCursor(1)
	if m.cursor != 1 {
		t.Fatalf("cursor went past the last row: %d", m.cursor)
	}
}

func TestTUIEmptyFleetDoesNotPanic(t *testing.T) {
	m := newModel(nil, testNow)
	_ = m.View()
	m = m.moveCursor(1)
	if m.cursor != 0 {
		t.Fatalf("cursor on an empty list must stay 0, got %d", m.cursor)
	}
}

func TestTUIShowsWorstFirstLikeTheTable(t *testing.T) {
	m := newModel([]Row{
		{Alias: "good", Class: "up-to-date"},
		{Alias: "dead", Class: "unreachable"},
	}, testNow)
	v := m.View()
	if strings.Index(v, "dead") > strings.Index(v, "good") {
		t.Fatalf("TUI must order worst-first like the table:\n%s", v)
	}
}

// The update key must hand the terminal over via ssh -t, reusing the SAME
// remote script as the headless path — one definition of "update a host".
func TestInteractiveUpdateUsesTTYAndTheSharedRemoteScript(t *testing.T) {
	c := interactiveUpdate("alpha")
	args := strings.Join(c.Args, " ")
	if !strings.Contains(args, " -t ") {
		t.Fatalf("ssh must allocate a TTY so sudo can prompt: %v", c.Args)
	}
	if !strings.Contains(args, "install.sh") || !strings.Contains(args, "pull --ff-only") {
		t.Fatalf("must run the shared remote update script: %v", c.Args)
	}
}
