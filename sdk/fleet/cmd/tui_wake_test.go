package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

func wakeTestModel(aliases ...string) tuiModel {
	var hosts []sshconf.Host
	for _, a := range aliases {
		hosts = append(hosts, sshconf.Host{Alias: a})
	}
	m := newTUIModel(hosts, runner.Fake{}, fakeBaseline{}, testNow, "main", 4, updplan.Default())
	for _, a := range aliases {
		delete(m.pending, a)
		m.setRow(Row{Alias: a, Class: "unreachable"})
	}
	m.resort()
	return m
}

// F17a — wake runs in the BACKGROUND lane. tea.ExecProcess suspends the whole
// TUI, so routing wake through it would freeze the dashboard for every
// sleeping host — exactly the hang this feature exists to remove.
func TestWakeKeyClaimsTheCursorHostInTheBackgroundLane(t *testing.T) {
	m := wakeTestModel("alpha", "beta")
	m.cursor = "alpha"

	got, cmd := route(m, key("w"))
	mm := got.(tuiModel)

	if !mm.waking["alpha"] {
		t.Fatalf("`w` must claim the cursor host, waking = %v", mm.waking)
	}
	if mm.waking["beta"] {
		t.Fatal("`w` with no selection must claim only the cursor host")
	}
	if cmd == nil {
		t.Fatal("`w` must return a command that actually wakes")
	}
}

// `w` follows `u`'s rule: act on the selection when there is one.
func TestWakeKeyActsOnTheWholeSelection(t *testing.T) {
	m := wakeTestModel("alpha", "beta", "gamma")
	m.selected = map[string]bool{"alpha": true, "gamma": true}

	got, _ := route(m, key("w"))
	mm := got.(tuiModel)

	if !mm.waking["alpha"] || !mm.waking["gamma"] {
		t.Fatalf("selection must be claimed, waking = %v", mm.waking)
	}
	if mm.waking["beta"] {
		t.Fatalf("unselected host must not be woken, waking = %v", mm.waking)
	}
}

// F17b — the in-flight ownership invariant now spans three states. A host
// being woken must not be re-polled underneath the ladder, or two async paths
// own one row and the last writer wins at random.
func TestRefreshSkipsHostsBeingWoken(t *testing.T) {
	m := wakeTestModel("alpha", "beta")
	m.waking = map[string]bool{"alpha": true}

	m.refresh()

	if m.pending["alpha"] {
		t.Fatal("refresh must not re-poll a host the wake ladder owns")
	}
	if !m.pending["beta"] {
		t.Fatal("refresh must still re-poll hosts nobody owns")
	}
}

func TestWakingHostIsReportedInFlight(t *testing.T) {
	m := wakeTestModel("alpha")
	m.waking = map[string]bool{"alpha": true}
	if !m.inFlight("alpha") {
		t.Fatal("a waking host must count as in-flight so `u` and `s` cannot claim it")
	}
}

// Double-claiming would run two ladders against one host and leak a slot.
func TestWakeIgnoresHostsAlreadyInFlight(t *testing.T) {
	m := wakeTestModel("alpha")
	m.cursor = "alpha"
	m.updating = map[string]updState{"alpha": {phase: updRunning}}

	got, _ := route(m, key("w"))
	if got.(tuiModel).waking["alpha"] {
		t.Fatal("a host the update engine owns must not be claimed by wake")
	}
}

// F17c — a completion must release ownership and re-poll, or the row keeps a
// stale verdict forever and the queue wedges.
func TestWakeCompletionReleasesOwnershipAndRepolls(t *testing.T) {
	m := wakeTestModel("alpha")
	m.waking = map[string]bool{"alpha": true}

	got, cmd := m.Update(wakeDoneMsg{alias: "alpha", woke: true, via: "peer-a"})
	mm := got.(tuiModel)

	if mm.waking["alpha"] {
		t.Fatal("completion must release the wake claim")
	}
	if !mm.pending["alpha"] {
		t.Fatal("completion must re-poll the host so the row shows its real class")
	}
	if cmd == nil {
		t.Fatal("completion must issue the re-poll command")
	}
	if !strings.Contains(mm.status, "peer-a") {
		t.Fatalf("status should name the rescuer, got %q", mm.status)
	}
}

// A failed wake must still release the claim — otherwise the row is stuck
// in-flight and refresh will skip it for the rest of the session.
func TestFailedWakeStillReleasesOwnership(t *testing.T) {
	m := wakeTestModel("alpha")
	m.waking = map[string]bool{"alpha": true}

	got, _ := m.Update(wakeDoneMsg{alias: "alpha", woke: false})
	if got.(tuiModel).waking["alpha"] {
		t.Fatal("a failed wake must not leave the host owned forever")
	}
}

// F17d — keyHelp is the single source of truth for the overlay, so a key that
// is not listed there is invisible to the operator.
func TestHelpListsTheWakeKey(t *testing.T) {
	for _, h := range keyHelp {
		if strings.TrimSpace(h.keys) == "w" {
			return
		}
	}
	t.Fatalf("`w` missing from keyHelp: %+v", keyHelp)
}

// The view must show that something is happening; a silently-stalled row is
// indistinguishable from a hung TUI.
func TestWakingRowRendersAsWaking(t *testing.T) {
	m := wakeTestModel("alpha")
	m.waking = map[string]bool{"alpha": true}

	if !strings.Contains(m.View(), "waking") {
		t.Fatalf("a waking row must say so:\n%s", m.View())
	}
}

// The quit guard exists so work in flight is never orphaned; waking is work.
func TestQuitGuardCountsWaking(t *testing.T) {
	m := wakeTestModel("alpha")
	m.waking = map[string]bool{"alpha": true}
	if !m.busy() {
		t.Fatal("a waking host must keep the quit guard armed")
	}
}

// The TUI must inherit auto-wake through the same probe path the headless
// command uses, so the two can never diverge on what `unreachable` means.
func TestPollHostCarriesTheWakerIntoTheTUI(t *testing.T) {
	s := newSleeper(stampFor(strings.Repeat("a", 40)), "sleeper")
	base := fakeBaseline{head: strings.Repeat("a", 40), ancestor: map[string]bool{strings.Repeat("a", 40): true}}

	w := func(h sshconf.Host, _ []reach.Peer) reach.Result {
		s.rouse(h.Alias)
		return reach.Result{Woke: true, Via: "peer-a"}
	}

	cmd := pollHostWake(sshconf.Host{Alias: "sleeper"}, nil, s, base, w)
	msg := cmd().(hostRowMsg)

	if msg.row.Class != "up-to-date" {
		t.Fatalf("class = %q, want the woken class", msg.row.Class)
	}
	if msg.row.Note != "woke via peer-a" {
		t.Fatalf("Note = %q, want wake provenance in the TUI too", msg.row.Note)
	}
}
