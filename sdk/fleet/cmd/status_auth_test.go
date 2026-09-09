package cmd

import (
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// sshErr reproduces what runner.Exec.Run really returns when ssh fails: a
// *exec.ExitError carrying the process's stderr. Hand-building the struct
// would leave ProcessState nil and test a shape production never emits.
func sshErr(t *testing.T, stderr string) error {
	t.Helper()
	_, err := exec.Command("sh", "-c", `printf '%s' "$1" >&2; exit 255`, "sh", stderr).Output()
	if err == nil {
		t.Fatal("setup: wanted a failing command")
	}
	return err
}

const hostKeyFail = "Host key verification failed.\r\n"

// The whole bug: BatchMode means an unknown host key fails instantly, which
// looked exactly like a dead machine and sent a real investigation to the
// network layer. The host was up the entire time.
func TestAuthFailureReportsAuthFailedNotUnreachable(t *testing.T) {
	cur := strings.Repeat("a", 40)
	r := runner.Fake{Err: map[string]error{"blocked": sshErr(t, hostKeyFail)}}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	row := probeHost(sshconf.Host{Alias: "blocked"}, r, base)

	if row.Class != string(drift.AuthFailed) {
		t.Fatalf("Class = %q, want %q", row.Class, drift.AuthFailed)
	}
	if !strings.Contains(row.Note, "host key unverified") {
		t.Fatalf("Note = %q, want it to name the fault", row.Note)
	}
}

// A failure with no stderr to read (every fake runner that predates this
// change) must classify exactly as it always did.
func TestFailureWithNoEvidenceStaysUnreachable(t *testing.T) {
	cur := strings.Repeat("a", 40)
	r := runner.Fake{Err: map[string]error{"dead": runner.ErrFake}}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	row := probeHost(sshconf.Host{Alias: "dead"}, r, base)

	if row.Class != string(drift.Unreachable) {
		t.Fatalf("Class = %q, want %q", row.Class, drift.Unreachable)
	}
}

// The ladder exists to rouse hosts asleep at layer 2. A host that answered SSH
// is not asleep, so waking it is pure waste — a full budget (~12s) of ICMP and
// relay hops per host, every run, to fix nothing.
func TestWakeLadderNeverFiresForAnAuthFailure(t *testing.T) {
	cur := strings.Repeat("a", 40)
	r := runner.Fake{Err: map[string]error{"blocked": sshErr(t, hostKeyFail)}}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	var mu sync.Mutex
	var woken []string
	w := func(h sshconf.Host, _ []reach.Peer) reach.Result {
		mu.Lock()
		defer mu.Unlock()
		woken = append(woken, h.Alias)
		return reach.Result{}
	}

	collectWake([]sshconf.Host{{Alias: "blocked"}}, r, base, testNow, w)

	if len(woken) != 0 {
		t.Fatalf("ladder fired for %v; an answering host must never be woken", woken)
	}
}

// We cannot run a command on a host that refuses us, so it must never be
// ranked as a live relay candidate. Marking it up would send a straggler's
// ladder through a hop that is guaranteed to fail.
func TestAuthFailedHostIsNeverOfferedAsALiveRelayPeer(t *testing.T) {
	cur := strings.Repeat("a", 40)
	r := runner.Fake{Err: map[string]error{
		"blocked": sshErr(t, hostKeyFail),
		"asleep":  runner.ErrFake,
	}}
	base := fakeBaseline{head: cur, ancestor: map[string]bool{cur: true}}

	var mu sync.Mutex
	var seen []reach.Peer
	w := func(h sshconf.Host, peers []reach.Peer) reach.Result {
		mu.Lock()
		defer mu.Unlock()
		if h.Alias == "asleep" {
			seen = peers
		}
		return reach.Result{}
	}

	collectWake([]sshconf.Host{{Alias: "blocked"}, {Alias: "asleep"}}, r, base, testNow, w)

	for _, p := range seen {
		if p.Alias == "blocked" && p.Reachable {
			t.Fatal("a host that refused us was offered as a reachable relay peer")
		}
	}
}
