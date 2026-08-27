package linkstate

import (
	"encoding/json"
	"testing"
)

// The JSON shape is a published contract (gsl consumes it, CI gates on it), so
// it is pinned against the spec's §4.1 worked example rather than left to
// whatever the struct happens to marshal to.
func TestState_JSONMatchesTheSpecShape(t *testing.T) {
	s := State{
		WSL:    true,
		Link:   HealthOK,
		Tunnel: Tunnel{State: TunnelUp, Interface: "wg-lab", HandshakeAgeSeconds: func() *int { n := 34; return &n }()},
		Pinned: &Pin{Resolver: "10.10.0.1", Since: "2026-08-24T21:40:11Z", Managed: true},
		Candidates: []Candidate{
			{Server: "10.10.0.1", Reachable: true, FleetResolved: 3, Recursive: true},
		},
		Fleet: Fleet{Total: 3, Resolved: 3, ExcludedByHostsFile: []string{"selfhost"}},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"wsl", "link", "tunnel", "pinned", "candidates", "fleet", "drift"} {
		if _, ok := got[key]; !ok {
			t.Errorf("top-level key %q missing from the JSON contract", key)
		}
	}
	tun, _ := got["tunnel"].(map[string]any)
	for _, key := range []string{"state", "interface", "handshake_age_seconds"} {
		if _, ok := tun[key]; !ok {
			t.Errorf("tunnel.%s missing", key)
		}
	}
	fleet, _ := got["fleet"].(map[string]any)
	for _, key := range []string{"total", "resolved", "excluded_by_hosts_file"} {
		if _, ok := fleet[key]; !ok {
			t.Errorf("fleet.%s missing", key)
		}
	}
}

// "Not pinned" and "no drift" are absences, and must serialize as null rather
// than as a zero-valued object — a consumer reading pinned.resolver == "" as
// "pinned to nothing" would be wrong in a way that is hard to see.
func TestState_AbsencesAreNullNotZeroObjects(t *testing.T) {
	b, err := json.Marshal(State{WSL: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["pinned"] != nil {
		t.Errorf("pinned = %v, want null when nothing is pinned", got["pinned"])
	}
	if got["drift"] != nil {
		t.Errorf("drift = %v, want null when there is no drift", got["drift"])
	}
}

// A nil slice marshals to null, which forces every consumer to nil-check before
// ranging. Empty collections are emitted as [] so `.candidates.length` is
// always meaningful.
func TestState_EmptyCollectionsAreArraysNotNull(t *testing.T) {
	b, _ := json.Marshal(State{WSL: true}.Normalized())
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	for _, key := range []string{"candidates"} {
		if got[key] == nil {
			t.Errorf("%s = null, want [] so consumers need no nil check", key)
		}
	}
	fleet, _ := got["fleet"].(map[string]any)
	if fleet["excluded_by_hosts_file"] == nil {
		t.Error("fleet.excluded_by_hosts_file = null, want []")
	}
}

// Health drives the process exit code (spec §3: 0 healthy, 1 degraded), so the
// rules that make a link degraded are pinned individually.
func TestState_Health(t *testing.T) {
	healthy := func() State {
		return State{
			WSL:    true,
			Tunnel: Tunnel{State: TunnelUp, Interface: "wg-lab"},
			Pinned: &Pin{Resolver: "10.10.0.1", Managed: true},
			Fleet:  Fleet{Total: 3, Resolved: 3},
		}
	}
	if got := healthy().Health(); got != HealthOK {
		t.Errorf("healthy state Health() = %q, want %q", got, HealthOK)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*State)
	}{
		{"drift detected", func(s *State) { s.Drift = &Drift{File: "/etc/resolv.conf", Detail: "edited"} }},
		{"fleet partially resolvable", func(s *State) { s.Fleet.Resolved = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := healthy()
			tc.mutate(&s)
			if got := s.Health(); got != HealthDegraded {
				t.Errorf("Health() = %q, want %q", got, HealthDegraded)
			}
		})
	}
}

// A machine on the fleet's LAN has no tunnel and nothing pinned, and resolves
// everything. That is healthy: tunnel state and pin status are context, not
// health inputs. Reporting degraded here is the tool crying wolf.
func TestState_NoTunnelAndNoPinIsHealthyWhenTheFleetResolves(t *testing.T) {
	s := State{
		WSL:    true,
		Tunnel: Tunnel{State: TunnelDown},
		Pinned: nil,
		Fleet:  Fleet{Total: 3, Resolved: 3},
	}
	if got := s.Health(); got != HealthOK {
		t.Errorf("Health() = %q, want %q — the fleet resolves, so the link works", got, HealthOK)
	}
}

// Off WSL the tool is a no-op, not a failure — reporting it degraded would make
// install.sh look broken on a machine the feature simply does not apply to.
func TestState_NonWSLIsNotDegraded(t *testing.T) {
	if got := (State{WSL: false}).Health(); got != HealthOK {
		t.Errorf("non-WSL Health() = %q, want %q (a no-op, not a failure)", got, HealthOK)
	}
}

// A zero fleet must not read as "0 of 0 resolved, therefore degraded" — there
// is simply nothing to resolve.
func TestState_EmptyFleetIsNotDegraded(t *testing.T) {
	s := State{
		WSL:    true,
		Tunnel: Tunnel{State: TunnelUp},
		Pinned: &Pin{Resolver: "10.10.0.1"},
		Fleet:  Fleet{Total: 0, Resolved: 0},
	}
	if got := s.Health(); got != HealthOK {
		t.Errorf("empty-fleet Health() = %q, want %q", got, HealthOK)
	}
}

// The four tunnel states are a closed set; a typo'd string in a consumer's
// switch is a silent bug, so the constants are asserted verbatim.
func TestTunnelStateWireValues(t *testing.T) {
	for got, want := range map[TunnelState]string{
		TunnelUp: "up", TunnelNotReady: "not-ready",
		TunnelDown: "down", TunnelUnknown: "unknown",
	} {
		if string(got) != want {
			t.Errorf("tunnel state = %q, want %q", got, want)
		}
	}
}
