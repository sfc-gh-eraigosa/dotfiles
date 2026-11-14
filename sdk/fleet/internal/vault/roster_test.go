package vault

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

func sorted(s []string) []string { sort.Strings(s); return s }

func TestRosterLifecycle(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	r := Roster{Entries: []RosterEntry{{Workstation: "ws", Public: "pubA", Fingerprint: "fpA", State: KeyActive, Changed: t0}}}
	r.Add("ws", "pubB", "fpB", t1)
	if got := sorted(r.Managed()); !reflect.DeepEqual(got, []string{"pubA", "pubB"}) {
		t.Fatalf("managed %v", got)
	}
	r.Verified("fpB", "h1")
	r.Verified("fpB", "h1")
	if n := len(r.Entries[1].VerifiedOn); n != 1 {
		t.Fatalf("dedup failed: %d", n)
	}
	ret := r.Activate("fpB", t1)
	if len(ret) != 1 || ret[0].Fingerprint != "fpA" || ret[0].State != KeyRetired {
		t.Fatalf("retired %+v", ret)
	}
	if got := r.Managed(); !reflect.DeepEqual(got, []string{"pubB"}) {
		t.Fatalf("managed %v", got)
	}
	if !r.RetirableOn("fpA", "h1") || r.RetirableOn("fpA", "h2") {
		t.Fatal("RetirableOn wrong")
	}
	if r.Entries[1].State != KeyActive || !r.Entries[1].Changed.Equal(t1) {
		t.Fatal("B not active/changed")
	}
}

func TestManagedNeverIncludesRetired(t *testing.T) {
	r := Roster{Entries: []RosterEntry{
		{Workstation: "w", Public: "old", Fingerprint: "o", State: KeyRetired},
		{Workstation: "w", Public: "act", Fingerprint: "a", State: KeyActive},
		{Workstation: "w", Public: "pen", Fingerprint: "p", State: KeyPending},
	}}
	if got := r.Managed(); !reflect.DeepEqual(got, []string{"act", "pen"}) {
		t.Fatalf("%v", got)
	}
}

func TestRosterUnknownAndCrossWorkstation(t *testing.T) {
	now := time.Now()
	r := Roster{}
	r.Add("w1", "p1", "f1", now)
	r.Add("w2", "p2", "f2", now)
	r.Activate("f1", now)
	r.Activate("f2", now)
	if got := r.Activate("f1", now); len(got) != 0 {
		t.Fatalf("re-activate retired %v", got)
	}
	r.Add("w1", "p3", "f3", now)
	ret := r.Activate("f3", now)
	if len(ret) != 1 || ret[0].Fingerprint != "f1" {
		t.Fatalf("%v", ret)
	}
	if r.Entries[1].State != KeyActive {
		t.Fatal("w2 touched")
	}
	before := len(r.Entries)
	r.Verified("nope", "h")
	if r.Activate("nope", now) != nil || len(r.Entries) != before {
		t.Fatal("unknown not a no-op")
	}
	if r.RetirableOn("nope", "h") || r.RetirableOn("f2", "h") {
		t.Fatal("RetirableOn should be false")
	}
}
