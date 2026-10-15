package cmd

import (
	"os/exec"
	"strings"
	"testing"
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
