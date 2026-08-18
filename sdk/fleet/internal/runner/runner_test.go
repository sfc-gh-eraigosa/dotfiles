package runner

import (
	"reflect"
	"testing"
)

// F14 — RunVia relays through a peer with ssh -J. The peer is a transport hop
// only: authentication stays end-to-end workstation -> target, so the TARGET
// must be the ssh host and the peer must be the -J value. Swapping them would
// silently run the command on the wrong machine.
func TestViaArgsRelaysThroughPeerWithProxyJump(t *testing.T) {
	got := viaArgs("peer-a", "target-b", "6", []string{"echo hi"})
	want := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=6",
		"-J", "peer-a",
		"target-b",
		"echo hi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("viaArgs =\n  %q\nwant\n  %q", got, want)
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
