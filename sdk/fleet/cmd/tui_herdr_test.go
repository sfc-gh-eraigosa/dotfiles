package cmd

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// --- S: attach to the host's herdr session from the dashboard --------------

// The local client is what herdr --remote runs, so it is resolved HERE, never
// on the host. The order mirrors the fleet-connect design (§4.6): PATH, then
// the two per-user tool dirs the installer writes to.
func TestLocalHerdrPrefersPATHThenLocalBinThenOptBin(t *testing.T) {
	home := "/h"
	onPath := func(string) (string, error) { return "/usr/bin/herdr", nil }
	offPath := func(string) (string, error) { return "", exec.ErrNotFound }
	only := func(want string) func(string) bool { return func(p string) bool { return p == want } }

	if got := localHerdr(onPath, only("/h/.local/bin/herdr"), home); got != "/usr/bin/herdr" {
		t.Fatalf("PATH hit must win, got %q", got)
	}
	if got := localHerdr(offPath, only("/h/.local/bin/herdr"), home); got != "/h/.local/bin/herdr" {
		t.Fatalf("~/.local/bin fallback, got %q", got)
	}
	if got := localHerdr(offPath, only("/h/opt/bin/herdr"), home); got != "/h/opt/bin/herdr" {
		t.Fatalf("~/opt/bin fallback, got %q", got)
	}
	if got := localHerdr(offPath, func(string) bool { return false }, home); got != "" {
		t.Fatalf("nothing installed must resolve to empty, got %q", got)
	}
}

// The handoff is a local exec with no shell, so a hostile alias is an inert
// argv element; the test pins the exact argv the operator saw work by hand.
func TestHerdrAttachArgv(t *testing.T) {
	got := herdrAttachArgv("/h/opt/bin/herdr", "box; rm -rf /")
	want := []string{"/h/opt/bin/herdr", "--remote", "box; rm -rf /"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestShiftSHandsOffToHerdrWhenInstalled(t *testing.T) {
	m := testModel("a")
	m.cursor = "a"
	m.herdrBin = func() string { return "/h/opt/bin/herdr" }
	mm, cmd := send(m, "S")
	if cmd == nil {
		t.Fatal("S on an idle host with a local herdr must return the handoff command")
	}
	if mm.status != "" {
		t.Fatalf("no notice expected on a successful handoff, got %q", mm.status)
	}
}

func TestShiftSWithoutLocalHerdrExplainsInsteadOfRunning(t *testing.T) {
	m := testModel("a")
	m.cursor = "a"
	m.herdrBin = func() string { return "" }
	mm, cmd := send(m, "S")
	if cmd != nil {
		t.Fatal("S with no local herdr must not run anything")
	}
	if !strings.Contains(mm.status, "herdr") || !strings.Contains(mm.status, "not installed") {
		t.Fatalf("status must say herdr is not installed locally, got %q", mm.status)
	}
}

func TestShiftSIsANoOpWhileTheHostIsInFlight(t *testing.T) {
	m := testModel("a")
	m.cursor = "a"
	m.herdrBin = func() string { return "/h/opt/bin/herdr" }
	m.updating["a"] = updState{phase: updRunning}
	mm, cmd := send(m, "S")
	if cmd != nil {
		t.Fatal("S must not race an update in flight")
	}
	if mm.status != "" {
		t.Fatalf("unexpected status %q", mm.status)
	}
}

// S must be discoverable: one keyHelp row, in the SSH & access group beside s.
func TestShiftSIsInTheSSHHelpGroup(t *testing.T) {
	for _, k := range keyHelp {
		if k.keys == "S" {
			if k.group != grpSSH {
				t.Fatalf("S must sit in the %q group, got %q", grpSSH, k.group)
			}
			if !strings.Contains(k.what, "herdr") {
				t.Fatalf("S's help must name herdr, got %q", k.what)
			}
			return
		}
	}
	t.Fatal("keyHelp has no S row")
}

// --- S inside herdr: save the host as a herdr machine ----------------------
//
// herdr refuses to start a client inside one of its own panes ("nested herdr
// is disabled by default"), and fleet usually runs in one. Stripping HERDR_*
// from the child's env would dodge a guard that exists because two TUIs
// cannot share a prefix. So inside herdr S takes the fleet-herdr-machines
// route instead: save the host as a herdr machine and let prefix+w switch
// the herdr already open.

func TestHerdrMachineAddArgv(t *testing.T) {
	got := herdrMachineAddArgv("/h/opt/bin/herdr", "box")
	want := []string{"/h/opt/bin/herdr", "machine", "add", "box", "--label", "box"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

// `herdr machine list --json` is the source of truth for what is saved; the
// ssh target is what fleet keys on, never the label (which the operator may
// have renamed).
func TestSavedHerdrTargetsParsesMachineList(t *testing.T) {
	js := `[{"id":"382e","label":"big-box","target":"bigbox","session":"default","enabled":true,"selected":false},
	       {"id":"9f1c","label":"pi","target":"pi4","session":"default","enabled":false,"selected":false}]`
	got, err := savedHerdrTargets(js)
	if err != nil {
		t.Fatal(err)
	}
	if !got["bigbox"] || !got["pi4"] || len(got) != 2 {
		t.Fatalf("targets = %v", got)
	}
	if _, err := savedHerdrTargets("not json"); err == nil {
		t.Fatal("garbage must be an error, not an empty list")
	}
}

func insideHerdr(saved map[string]bool, listErr error) tuiModel {
	m := testModel("a")
	m.cursor = "a"
	m.inHerdr = true
	m.herdrBin = func() string { return "/h/opt/bin/herdr" }
	m.herdrMachines = func(string) (map[string]bool, error) { return saved, listErr }
	return m
}

func TestShiftSInsideHerdrSavedHostJustPointsAtPrefixW(t *testing.T) {
	mm, cmd := send(insideHerdr(map[string]bool{"a": true}, nil), "S")
	if cmd != nil {
		t.Fatal("a saved host needs no handoff")
	}
	for _, want := range []string{"a", "saved", "prefix+w"} {
		if !strings.Contains(mm.status, want) {
			t.Errorf("status must mention %q, got %q", want, mm.status)
		}
	}
}

func TestShiftSInsideHerdrHandsOffMachineAddForUnsavedHost(t *testing.T) {
	mm, cmd := send(insideHerdr(map[string]bool{"other": true}, nil), "S")
	if cmd == nil {
		t.Fatal("an unsaved host must hand the terminal to herdr machine add")
	}
	if mm.status != "" {
		t.Fatalf("no notice until the handoff returns, got %q", mm.status)
	}
}

func TestShiftSInsideHerdrListFailureIsReportedNotIgnored(t *testing.T) {
	mm, cmd := send(insideHerdr(nil, errors.New("exit status 7")), "S")
	if cmd != nil {
		t.Fatal("a failed list must not guess and add anyway")
	}
	if !strings.Contains(mm.status, "machine list") || !strings.Contains(mm.status, "exit status 7") {
		t.Fatalf("status must name the failed command and herdr's error, got %q", mm.status)
	}
}

// The handoff's return carries the note the operator reads next: success says
// prefix+w, failure says herdr's error (its own output is still on screen).
func TestMachineAddReturnSetsTheStatusLine(t *testing.T) {
	m := insideHerdr(nil, nil)
	var mm tea.Model
	mm, _ = m.Update(machineAddDone("a", nil))
	if st := mm.(tuiModel).status; !strings.Contains(st, "a") || !strings.Contains(st, "prefix+w") {
		t.Fatalf("success note must name the host and prefix+w, got %q", st)
	}
	mm, _ = m.Update(machineAddDone("a", errors.New("exit status 1")))
	if st := mm.(tuiModel).status; !strings.Contains(st, "machine add a") || !strings.Contains(st, "exit status 1") {
		t.Fatalf("failure note must name the command and the error, got %q", st)
	}
}

// Outside herdr the inside-herdr machinery must stay out of the way: S still
// attaches with herdr --remote and never consults the machine list.
func TestShiftSOutsideHerdrNeverListsMachines(t *testing.T) {
	m := testModel("a")
	m.cursor = "a"
	m.herdrBin = func() string { return "/h/opt/bin/herdr" }
	m.herdrMachines = func(string) (map[string]bool, error) {
		t.Fatal("machine list consulted outside herdr")
		return nil, nil
	}
	if _, cmd := send(m, "S"); cmd == nil {
		t.Fatal("expected the --remote handoff")
	}
}
