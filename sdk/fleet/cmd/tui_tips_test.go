package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

func tipModel(r Row) tuiModel {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1, updplan.Default())
	m.setRow(r)
	m.cursor = r.Alias
	return m
}

// Every fault row tells the operator which key fixes it. Without the tip the
// key lives only in the help overlay, which is exactly where nobody looks
// while staring at a red row.
func TestRowTipNamesTheKeyThatFixesTheRow(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  Row
		want []string
	}{
		{"host key changed", Row{Alias: "h", Class: "auth-failed", Note: "host key CHANGED"}, []string{"host key CHANGED", "T"}},
		{"host key unverified", Row{Alias: "h", Class: "auth-failed", Note: "host key unverified"}, []string{"T"}},
		{"credential refused", Row{Alias: "h", Class: "auth-failed", Note: "permission denied"}, []string{"A", "s"}},
		{"unreachable", Row{Alias: "h", Class: "unreachable"}, []string{"w"}},
		{"behind", Row{Alias: "h", Class: "behind", Behind: 3}, []string{"u"}},
		{"unknown", Row{Alias: "h", Class: "unknown"}, []string{"u"}},
		{"divergent", Row{Alias: "h", Class: "ahead/divergent"}, []string{"u"}},
	} {
		got := tipModel(tc.row).rowTip()
		for _, w := range tc.want {
			if !strings.Contains(got, "press "+w) && !strings.Contains(got, w+" ") {
				t.Errorf("%s: tip %q does not name %q", tc.name, got, w)
			}
		}
	}
}

func TestAFailedUpdatePointsAtHistoryAndStderr(t *testing.T) {
	m := tipModel(Row{Alias: "h", Class: "behind"})
	m.updating["h"] = updState{phase: updFail}
	got := m.rowTip()
	if !strings.Contains(got, "H") || !strings.Contains(got, "e") || !strings.Contains(got, "failed") {
		t.Fatalf("tip %q", got)
	}
}

func TestAHealthyRowHasNoTip(t *testing.T) {
	if got := tipModel(Row{Alias: "h", Class: "up-to-date"}).rowTip(); got != "" {
		t.Fatalf("tip %q", got)
	}
}

// The status line already speaks for the last action; a tip must not talk
// over it.
func TestTheLatestStatusOutranksTheTip(t *testing.T) {
	m := tipModel(Row{Alias: "h", Class: "unreachable"})
	m.status = "selection cleared"
	if got := m.statusView(); strings.Contains(got, "press w") {
		t.Fatalf("tip shown over a status message: %q", got)
	}
	m.status = ""
	if got := m.statusView(); !strings.Contains(got, "press w") {
		t.Fatalf("tip missing: %q", got)
	}
}

// A tip only names a key that would act: a host another path owns (waking,
// updating) gets no "press u" that the key would then ignore.
func TestNoTipForAHostAnotherPathOwns(t *testing.T) {
	m := tipModel(Row{Alias: "h", Class: "unreachable"})
	m.waking["h"] = true
	if got := m.rowTip(); got != "" {
		t.Fatalf("tip %q for a waking host", got)
	}
}

func TestEveryKeyBelongsToADeclaredHelpGroup(t *testing.T) {
	known := map[string]bool{}
	for _, g := range helpGroups {
		known[g] = true
	}
	for _, k := range keyHelp {
		if !known[k.group] {
			t.Errorf("key %q has group %q, not one of %v", k.keys, k.group, helpGroups)
		}
	}
}

// Everything about getting INTO a host lives in one section, so the operator
// facing an auth fault reads one block instead of hunting the whole list.
func TestSSHKeysShareOneSection(t *testing.T) {
	want := map[string]bool{"s": true, "T": true, "A": true, "p": true, "P": true}
	for _, k := range keyHelp {
		if want[k.keys] && k.group != grpSSH {
			t.Errorf("key %q is in %q, want %q", k.keys, k.group, grpSSH)
		}
		delete(want, k.keys)
	}
	if len(want) != 0 {
		t.Fatalf("keys missing from keyHelp: %v", want)
	}
}

// Reading order is down the left column, then down the right: the sections
// must come out in helpGroups order that way, every one of them, when the
// terminal is tall enough.
func TestHelpRendersSectionsInDeclaredOrder(t *testing.T) {
	cols, shown := layoutHelp(helpSections(46), 2, 60)
	if shown != len(keyHelp) {
		t.Fatalf("a tall terminal shows %d of %d keys", shown, len(keyHelp))
	}
	var order []string
	for _, c := range cols {
		for _, l := range c {
			for _, g := range helpGroups {
				if strings.Contains(l, g) {
					order = append(order, g)
				}
			}
		}
	}
	if strings.Join(order, "|") != strings.Join(helpGroups, "|") {
		t.Fatalf("sections read in order %v, want %v", order, helpGroups)
	}
}

// Wide enough for two columns, the descriptions must fit: a help line cut
// mid-word explains nothing.
func TestHelpDescriptionsFitTheirColumn(t *testing.T) {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1, updplan.Default())
	m.vp = viewport{height: 60, width: 100}
	if got := m.helpView(); strings.Contains(got, "…") {
		t.Fatalf("a help line was truncated at width 100:\n%s", got)
	}
}

// A short terminal drops whole sections from the END, so the access keys —
// the ones you need when something is broken — are the last to go.
func TestShortTerminalHelpKeepsTheSSHSection(t *testing.T) {
	m := newTUIModel(nil, nil, nil, time.Time{}, "", 1, updplan.Default())
	m.vp = viewport{height: bannerHeight + 6 + 9, width: 100}
	got := m.helpView()
	for _, want := range []string{grpSSH, "trust", "more"} {
		if !strings.Contains(got, want) {
			t.Fatalf("short help missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, grpNav) {
		t.Fatalf("navigation should be dropped before access keys:\n%s", got)
	}
}
