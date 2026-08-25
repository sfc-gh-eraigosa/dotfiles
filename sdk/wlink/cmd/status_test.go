package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/linkstate"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/probe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/winhost"
)

// Tunnel state is what tells a user which problem they have, so each of the
// four is pinned to the situation that produces it.
func TestTunnelState(t *testing.T) {
	tunnelIface := []winhost.Interface{
		{Alias: "Wi-Fi", DNSServers: []string{"198.51.100.53"}},
		{Alias: "wg-lab", DNSServers: []string{"10.10.0.1"}, IsTunnel: true},
	}
	noTunnel := []winhost.Interface{{Alias: "Wi-Fi", DNSServers: []string{"198.51.100.53"}}}

	for _, tc := range []struct {
		name   string
		ifaces []winhost.Interface
		scores []probe.Candidate
		want   linkstate.TunnelState
	}{
		{
			name:   "tunnel present and serving the fleet",
			ifaces: tunnelIface,
			scores: []probe.Candidate{{Server: "10.10.0.1", Reachable: true, FleetResolved: 2}},
			want:   linkstate.TunnelUp,
		},
		{
			// A VPN adapter and its DNS server appear the moment you click
			// connect, seconds before the handshake completes — and the old
			// network is already unroutable by then, so NOTHING answers.
			name:   "tunnel attached but nothing answers",
			ifaces: tunnelIface,
			scores: []probe.Candidate{{Server: "10.10.0.1"}, {Server: "198.51.100.53"}},
			want:   linkstate.TunnelNotReady,
		},
		{
			name:   "no tunnel interface at all",
			ifaces: noTunnel,
			scores: []probe.Candidate{{Server: "198.51.100.53", Reachable: true}},
			want:   linkstate.TunnelDown,
		},
		{
			// A tunnel that is up but serves a different network is still UP —
			// the fleet shortfall is reported separately rather than blamed on
			// the tunnel being absent.
			name:   "tunnel up but serving a different fleet",
			ifaces: tunnelIface,
			scores: []probe.Candidate{{Server: "10.10.0.1", Reachable: true, FleetResolved: 0}},
			want:   linkstate.TunnelUp,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tunnelStateFor(tc.ifaces, tc.scores); got != tc.want {
				t.Errorf("tunnelStateFor = %q, want %q", got, tc.want)
			}
		})
	}
}

// Windows unreachable is "unknown", not "down": claiming the tunnel is down
// when we simply could not look would be an assertion we have no basis for.
func TestTunnelState_UnknownWhenWindowsUnreachable(t *testing.T) {
	if got := tunnelStateFor(nil, nil); got != linkstate.TunnelUnknown {
		t.Errorf("tunnelStateFor(nil, nil) = %q, want %q", got, linkstate.TunnelUnknown)
	}
}

// EC-9: the JSON is a published contract, so it is validated as a document
// rather than eyeballed as a string.
func TestStatus_JSONIsTheDocumentedContract(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.JSON = true
	code, err := rt.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("status --json did not emit valid JSON: %v\n%s", err, out)
	}
	for _, key := range []string{"wsl", "link", "tunnel", "pinned", "candidates", "fleet", "drift"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("missing contract key %q", key)
		}
	}
	if doc["candidates"] == nil {
		t.Error("candidates = null; empty collections must be [] so .length always works")
	}
	if code != 0 && code != 1 {
		t.Errorf("exit = %d, want 0 or 1 (health-derived)", code)
	}
}

// A machine whose EXISTING resolver already reaches the fleet is healthy, even
// though wlink did not write it. This is the common case on a machine sitting
// directly on the fleet's LAN, and demanding a pin there would be the tool
// crying wolf about a link that plainly works. `managed:false` records who
// wrote it without pretending the machine is broken.
func TestStatus_ExistingWorkingResolverIsHealthyWithoutAPin(t *testing.T) {
	rt, out := healthyRuntime(t)
	// The stock resolv.conf already names a resolver, and it resolves the fleet.
	rt.Lookup = fakeZone{"10.255.255.254": {
		"lab-pi":     addr("10.10.0.21"),
		"github.com": addr("198.51.100.10"),
	}}
	rt.Host = fakeHost{ifaces: []winhost.Interface{
		{Alias: "wg-lab", DNSServers: []string{"10.255.255.254"}, IsTunnel: true},
	}}
	code, err := rt.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0 — the fleet is reachable, so nothing needs pinning:\n%s", code, out)
	}
	if strings.Contains(out.String(), "wlink pin") {
		t.Errorf("suggested a pin on a machine that already works:\n%s", out)
	}
	if !strings.Contains(out.String(), "NOT written by wlink") {
		t.Errorf("should record that the working resolver is not wlink's:\n%s", out)
	}
}

// EC-20 at the command level: the exit code is the contract a script reads.
func TestStatus_ExitCodeFollowsHealth(t *testing.T) {
	rt, _ := healthyRuntime(t)
	if _, err := rt.Pin(context.Background()); err != nil {
		t.Fatal(err)
	}
	code, err := rt.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0 after a successful pin", code)
	}
}

// Off WSL, status is a no-op that exits 0 and says so.
func TestStatus_NonWSLIsANoOp(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.WSL = false
	code, err := rt.Status(context.Background())
	if err != nil || code != 0 {
		t.Errorf("Status off WSL = (%d, %v), want (0, nil)", code, err)
	}
	if out.Len() == 0 {
		t.Error("must still explain why it did nothing")
	}
}

// The human rendering has to answer the question a person actually asked, so
// the lines that carry the answer are asserted.
func TestStatus_HumanOutputAnswersTheQuestion(t *testing.T) {
	rt, out := healthyRuntime(t)
	if _, err := rt.Status(context.Background()); err != nil {
		t.Fatalf("Status: %v", err)
	}
	s := out.String()
	for _, want := range []string{"link:", "tunnel:", "resolver:", "fleet:"} {
		if !strings.Contains(s, want) {
			t.Errorf("status output missing %q line:\n%s", want, s)
		}
	}
}

// A machine that CANNOT reach its fleet, where a candidate can, should say so
// rather than leaving the reader to infer it from the candidate list.
func TestStatus_HintsAtTheRemedyWhenDegraded(t *testing.T) {
	rt, out := healthyRuntime(t)
	// Nothing is pinned yet: resolv.conf points at a resolver that knows nothing.
	rt.Lookup = fakeZone{
		"10.10.0.1":      {"lab-pi": addr("10.10.0.21"), "github.com": addr("198.51.100.10")},
		"10.255.255.254": {"github.com": addr("198.51.100.10")},
	}
	rt.Host = fakeHost{ifaces: []winhost.Interface{
		{Alias: "Wi-Fi", DNSServers: []string{"10.255.255.254"}},
		{Alias: "wg-lab", DNSServers: []string{"10.10.0.1"}, IsTunnel: true},
	}}
	code, err := rt.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 (degraded)", code)
	}
	if !strings.Contains(out.String(), "wlink pin") {
		t.Errorf("degraded machine with an available fix should name it:\n%s", out)
	}
}

// Handshake age is not something wlink can observe yet. It must be absent from
// the JSON rather than reported as 0, which would read as "just handshaked".
func TestStatus_UnknownHandshakeAgeIsAbsentNotZero(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.JSON = true
	if _, err := rt.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tunnel map[string]any `json:"tunnel"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if v, present := doc.Tunnel["handshake_age_seconds"]; present && v != nil {
		t.Errorf("handshake_age_seconds = %v, want absent/null while unobservable", v)
	}
}
