package runner

import (
	"strings"
	"testing"
)

// F14 — RunVia relays through a peer with ssh -J. The peer is a transport hop
// only: authentication stays end-to-end workstation -> target, so the TARGET
// must be the ssh host and the peer must be the -J value. Swapping them would
// silently run the command on the wrong machine.
func TestViaArgsRelaysThroughPeerWithProxyJump(t *testing.T) {
	got := viaArgs("peer-a", "target-b", "6", []string{"echo hi"})

	// Asserted positionally rather than as an exact list: the option set grows
	// (BatchMode, timeout, multiplexing), but the RELATIONSHIP is the
	// invariant — peer is the -J value, target is the ssh host, and the
	// command is last. An exact-match assertion would break on every new
	// option while proving nothing extra.
	if !hasPair(got, "-J", "peer-a") {
		t.Fatalf("viaArgs = %q, want the PEER as the -J value", got)
	}
	if got[len(got)-1] != "echo hi" {
		t.Fatalf("viaArgs = %q, want the command last", got)
	}
	if got[len(got)-2] != "target-b" {
		t.Fatalf("viaArgs = %q, want the TARGET as the ssh host, not the peer", got)
	}
	if !hasPair(got, "-o", "ConnectTimeout=6") {
		t.Fatalf("viaArgs = %q, want the caller's timeout", got)
	}
}

// The relay must never hang waiting for input nobody can supply: a wake runs
// unattended inside a status poll, so BatchMode is not optional here.
func TestViaArgsIsAlwaysBatchMode(t *testing.T) {
	got := viaArgs("peer-a", "target-b", "6", []string{"cmd"})
	if !hasPair(got, "-o", "BatchMode=yes") {
		t.Fatalf("relay must be BatchMode, got %q", got)
	}
}

// Fake records which peer relayed, so a ladder test can assert the ranking
// picked the peer it was supposed to pick.
func TestFakeRecordsTheRelayHop(t *testing.T) {
	f := Fake{
		Out: map[string]string{"target-b": "ok"},
		Via: map[string]string{},
	}
	out, err := f.RunVia("peer-a", "target-b", "cmd")
	if err != nil {
		t.Fatalf("RunVia: %v", err)
	}
	if out != "ok" {
		t.Fatalf("out = %q, want %q", out, "ok")
	}
	if f.Via["target-b"] != "peer-a" {
		t.Fatalf("Via[target-b] = %q, want %q", f.Via["target-b"], "peer-a")
	}
}

// A relay to a host the peer cannot reach must surface as an error, not as
// empty output that a caller could mistake for success.
func TestFakeRunViaPropagatesTargetError(t *testing.T) {
	f := Fake{Err: map[string]error{"target-b": ErrFake}, Via: map[string]string{}}
	if _, err := f.RunVia("peer-a", "target-b", "cmd"); err == nil {
		t.Fatal("RunVia must propagate the target's error")
	}
}

func hasPair(argv []string, a, b string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == a && argv[i+1] == b {
			return true
		}
	}
	return false
}

// Multiplexing is the whole answer to "stop prompting me on every command":
// the first connection authenticates, later ones ride the same socket and skip
// authentication entirely — including under BatchMode, which is why fleet's
// unattended probes stop failing once a session exists.
func TestMuxArgsEnableConnectionReuse(t *testing.T) {
	got := muxArgs()
	if !hasPair(got, "-o", "ControlMaster=auto") {
		t.Fatalf("muxArgs = %q, want ControlMaster=auto", got)
	}
	if !hasPair(got, "-o", "ControlPersist="+controlPersist) {
		t.Fatalf("muxArgs = %q, want ControlPersist=%s", got, controlPersist)
	}
}

// %C is a fixed-length hash of the connection parameters. A literal
// %r@%h:%p path grows with the user and host name and can exceed the ~104-byte
// unix socket limit, at which point multiplexing fails silently and every
// command starts prompting again — the exact symptom this feature removes.
func TestControlPathIsShortEnoughToBeAUnixSocket(t *testing.T) {
	var path string
	got := muxArgs()
	for i := 0; i+1 < len(got); i++ {
		if v, ok := strings.CutPrefix(got[i+1], "ControlPath="); ok && got[i] == "-o" {
			path = v
		}
	}
	if path == "" {
		t.Fatal("muxArgs sets no ControlPath")
	}
	if !strings.Contains(path, "%C") {
		t.Fatalf("ControlPath = %q, want the fixed-length %%C token", path)
	}
	if len(path) > 80 {
		t.Fatalf("ControlPath %q is %d bytes; too close to the unix socket limit", path, len(path))
	}
}

// An escape hatch matters: multiplexing interacts badly with some jump hosts
// and stale sockets, and an operator who hits that needs a way out that does
// not involve editing the binary.
func TestMultiplexingCanBeDisabled(t *testing.T) {
	t.Setenv("FLEET_NO_MUX", "1")
	if got := muxArgs(); len(got) != 0 {
		t.Fatalf("muxArgs = %q, want none when FLEET_NO_MUX is set", got)
	}
}

// Every path that reaches a host must reuse the same socket, or the first
// interactive session authenticates and the later batch probe still prompts.
func TestEveryRemotePathCarriesTheMuxOptions(t *testing.T) {
	e := Exec{}
	for name, got := range map[string][]string{
		"Run":            e.baseArgs("h"),
		"RunVia":         viaArgs("peer", "h", "6", []string{"cmd"}),
		"RunInteractive": e.interactiveArgs("h"),
	} {
		if !hasPair(got, "-o", "ControlMaster=auto") {
			t.Errorf("%s args = %q, want ControlMaster=auto", name, got)
		}
	}
}
