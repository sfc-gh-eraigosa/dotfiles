package probe

import (
	"context"
	"reflect"
	"testing"
)

// fakeZone answers from a table: zone[server][name] = outcome. A server absent
// from the table says nothing at all (Silent), which is how an unreachable
// resolver behaves.
type fakeZone map[string]map[string]Result

func (z fakeZone) LookupA(_ context.Context, server, name string) Result {
	names, ok := z[server]
	if !ok {
		return Result{Outcome: Silent}
	}
	if r, ok := names[name]; ok {
		return r
	}
	return Result{Outcome: NoAddress} // reachable, does not know the name
}

func resolved(addr string) Result { return Result{Outcome: Resolved, Addrs: []string{addr}} }

// EC-1, the trap this whole tool exists for: the resolver that knows the fleet
// is frequently NOT on the default route. Selection must follow which server
// actually answers for the fleet, never which one is "primary".
func TestScore_PrefersTheResolverThatAnswersForTheFleet(t *testing.T) {
	zone := fakeZone{
		// The default route's resolver: reachable, recursive, knows nothing local.
		"192.0.2.1": {"github.com": resolved("198.51.100.10")},
		// The tunnel's resolver: knows the fleet AND recurses.
		"10.10.0.1": {
			"lab-pi":     resolved("10.10.0.21"),
			"lab-nas":    resolved("10.10.0.22"),
			"github.com": resolved("198.51.100.10"),
		},
	}
	scores := Score(context.Background(), zone, ScoreInput{
		Servers:        []string{"192.0.2.1", "10.10.0.1"}, // gateway listed FIRST
		Fleet:          []string{"lab-pi", "lab-nas"},
		PublicSentinel: "github.com",
	})
	win, ok := Winner(scores)
	if !ok {
		t.Fatal("Winner() found none, want the tunnel resolver")
	}
	if win.Server != "10.10.0.1" {
		t.Errorf("winner = %s, want 10.10.0.1 — selection followed the default route instead of the fleet", win.Server)
	}
	if win.FleetResolved != 2 {
		t.Errorf("winner FleetResolved = %d, want 2", win.FleetResolved)
	}
}

// Reachable-but-ignorant and silent are different diagnoses: the first means
// "wrong resolver", the second means "nothing is answering", and only the
// second implies a tunnel that is still handshaking.
func TestScore_SeparatesSilentFromReachableButIgnorant(t *testing.T) {
	zone := fakeZone{
		"192.0.2.1": {"github.com": resolved("198.51.100.10")}, // answers, ignorant
		"10.10.0.1": {"lab-pi": resolved("10.10.0.21")},
		// 203.0.113.1 absent => silent
	}
	scores := Score(context.Background(), zone, ScoreInput{
		Servers:        []string{"192.0.2.1", "203.0.113.1", "10.10.0.1"},
		Fleet:          []string{"lab-pi"},
		PublicSentinel: "github.com",
	})
	byServer := map[string]Candidate{}
	for _, s := range scores {
		byServer[s.Server] = s
	}
	if got := byServer["192.0.2.1"]; !got.Reachable || got.FleetResolved != 0 {
		t.Errorf("gateway = %+v, want reachable with 0 fleet hits", got)
	}
	if got := byServer["203.0.113.1"]; got.Reachable {
		t.Errorf("blackhole = %+v, want unreachable", got)
	}
}

// A SERVFAIL came FROM the server, so the server is reachable. Treating it as
// silent would misdiagnose a working tunnel as not-ready.
func TestScore_ServfailCountsAsReachable(t *testing.T) {
	zone := fakeZone{"192.0.2.1": {
		"lab-pi":     {Outcome: Unhelpful},
		"github.com": {Outcome: Unhelpful},
	}}
	scores := Score(context.Background(), zone, ScoreInput{
		Servers: []string{"192.0.2.1"}, Fleet: []string{"lab-pi"}, PublicSentinel: "github.com",
	})
	if !scores[0].Reachable {
		t.Error("SERVFAIL must count as reachable — the server spoke")
	}
	if scores[0].Recursive {
		t.Error("SERVFAIL on the sentinel is not proof of recursion")
	}
}

// Ties must not depend on map iteration order, or the pinned resolver would
// change between runs on an unchanged machine.
func TestWinner_TiesResolveToFirstEnumerated(t *testing.T) {
	zone := fakeZone{
		"10.10.0.1": {"lab-pi": resolved("10.10.0.21"), "github.com": resolved("198.51.100.10")},
		"10.10.0.2": {"lab-pi": resolved("10.10.0.21"), "github.com": resolved("198.51.100.10")},
	}
	for i := range 20 {
		scores := Score(context.Background(), zone, ScoreInput{
			Servers: []string{"10.10.0.1", "10.10.0.2"}, Fleet: []string{"lab-pi"}, PublicSentinel: "github.com",
		})
		win, _ := Winner(scores)
		if win.Server != "10.10.0.1" {
			t.Fatalf("run %d: winner = %s, want the first enumerated on a tie", i, win.Server)
		}
	}
}

// Nothing resolves anything: there is no winner, and the caller must be able to
// tell that apart from a zero-value candidate.
func TestWinner_NoneWhenNothingResolvesTheFleet(t *testing.T) {
	zone := fakeZone{"192.0.2.1": {"github.com": resolved("198.51.100.10")}}
	scores := Score(context.Background(), zone, ScoreInput{
		Servers: []string{"192.0.2.1"}, Fleet: []string{"lab-pi"}, PublicSentinel: "github.com",
	})
	if _, ok := Winner(scores); ok {
		t.Error("Winner() reported one when no candidate resolved a fleet name")
	}
}

// AllSilent is what separates "attached but not ready" from "wrong tunnel".
func TestAllSilent(t *testing.T) {
	silent := []Candidate{{Server: "a"}, {Server: "b"}}
	if !AllSilent(silent) {
		t.Error("AllSilent = false, want true when nothing is reachable")
	}
	mixed := []Candidate{{Server: "a"}, {Server: "b", Reachable: true}}
	if AllSilent(mixed) {
		t.Error("AllSilent = true, want false when something answered")
	}
	if AllSilent(nil) {
		t.Error("AllSilent(nil) = true; no candidates is not the same as all silent")
	}
}

// EC-14: loopback and link-local are never useful as a LAN/VPN resolver, and
// duplicates across interfaces must collapse without reordering.
func TestFilterCandidates(t *testing.T) {
	got := FilterCandidates([]string{
		"127.0.0.53", "10.10.0.1", "169.254.1.1", "0.0.0.0",
		"10.10.0.1", "", "198.51.100.53", "not-an-ip", "::1",
	})
	want := []string{"10.10.0.1", "198.51.100.53"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterCandidates() = %v, want %v (first-seen order preserved)", got, want)
	}
}

// EC-2, the rule that protects public DNS. resolv.conf is an ordered list, not
// a routing table: nameserver #1 answers EVERY query, and its NXDOMAIN is a
// FINAL answer that never falls through. Pinning a local-only resolver first
// would take out public DNS with no fallback.
func TestGuard_RefusesAResolverThatCannotRecurse(t *testing.T) {
	localOnly := Candidate{Server: "10.10.0.1", Reachable: true, FleetResolved: 3, Recursive: false}

	v := Guard(localOnly, false)
	if v.OK {
		t.Fatal("Guard allowed a non-recursive resolver — pinning it first would kill public DNS")
	}
	if v.Reason == "" {
		t.Error("Guard refusal must carry a reason; a silent refusal is unexplainable to the user")
	}

	recursive := Candidate{Server: "10.10.0.1", Reachable: true, FleetResolved: 3, Recursive: true}
	if v := Guard(recursive, false); !v.OK {
		t.Errorf("Guard refused a recursive resolver: %s", v.Reason)
	}
}

// The override exists for split-horizon setups, but must be loud: it trades
// public DNS away.
func TestGuard_OverrideAllowsButStillExplains(t *testing.T) {
	localOnly := Candidate{Server: "10.10.0.1", Reachable: true, FleetResolved: 3, Recursive: false}
	v := Guard(localOnly, true)
	if !v.OK {
		t.Fatal("--allow-nonrecursive must permit the pin")
	}
	if v.Reason == "" {
		t.Error("the override must still explain what is being traded away")
	}
	if !v.Overridden {
		t.Error("an overridden verdict must be marked as such so the caller can warn loudly")
	}
}
