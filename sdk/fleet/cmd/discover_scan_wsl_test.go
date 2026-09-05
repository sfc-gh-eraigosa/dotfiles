package cmd

import (
	"errors"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// A fleet whose config holds DNS NAMES rather than addresses must classify as
// current when the name already resolves to the discovered address.
//
// This is the bug that made every scan a no-op-in-name-only: the comparison was
// `HostName == IP`, and "named-box" is never equal to "10.0.0.5", so
// every DNS-named host reported `moved` forever and each run rewrote a working
// name into a DHCP address that expires.
func TestClassifyScanTreatsAResolvedDNSNameAsCurrent(t *testing.T) {
	hosts := []sshconf.Host{{Alias: "named-box", HostName: "named-box", Fleet: true}}
	resolve := func(name string) []string {
		if name == "named-box" {
			return []string{"10.0.0.5"}
		}
		return nil
	}
	rows := classifyScan(hosts, []responder{
		{IP: "10.0.0.5", Hostname: "named-box", Identified: true},
	}, resolve)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Kind != scanCurrent {
		t.Fatalf("kind = %q, want current (a resolved name is not a move)", rows[0].Kind)
	}
}

// The fix must not blind the scan to a REAL move: if the configured name
// resolves somewhere else, the host genuinely moved and must be refreshed.
func TestClassifyScanStillReportsAGenuineMoveWhenTheNameResolvesElsewhere(t *testing.T) {
	hosts := []sshconf.Host{{Alias: "wanderer", HostName: "wanderer", Fleet: true}}
	resolve := func(string) []string { return []string{"10.0.0.9"} } // stale record
	rows := classifyScan(hosts, []responder{
		{IP: "10.0.0.7", Hostname: "wanderer", Identified: true},
	}, resolve)
	if rows[0].Kind != scanMoved {
		t.Fatalf("kind = %q, want moved", rows[0].Kind)
	}
}

// A name that does not resolve at all must not crash or silently pass; it is a
// move, because the address we found is the only thing that works.
func TestClassifyScanTreatsAnUnresolvableNameAsMoved(t *testing.T) {
	hosts := []sshconf.Host{{Alias: "ghost", HostName: "ghost", Fleet: true}}
	rows := classifyScan(hosts, []responder{
		{IP: "10.0.0.7", Hostname: "ghost", Identified: true},
	}, func(string) []string { return nil })
	if rows[0].Kind != scanMoved {
		t.Fatalf("kind = %q, want moved", rows[0].Kind)
	}
}

// Under WSL the default route leaves on the Hyper-V NAT interface, whose subnet
// is NOT the LAN the fleet lives on. Detection must prefer the LAN reported by
// the Windows host.
func TestDetectCIDRPrefersTheWindowsHostLANUnderWSL(t *testing.T) {
	got, err := detectCIDR(subnetDeps{
		underWSL:   func() bool { return true },
		hostLAN:    func() (string, error) { return "192.168.0.236/24", nil },
		localCIDRs: func() ([]string, error) { return []string{"172.21.70.21/20"}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "192.168.0.236/24" {
		t.Fatalf("cidr = %q, want the Windows host LAN, not the WSL NAT segment", got)
	}
}

// With interop unavailable (or mirrored networking, where the local interface
// already IS the LAN) detection must still produce an answer.
func TestDetectCIDRFallsBackToLocalInterfacesWhenHostLANUnavailable(t *testing.T) {
	got, err := detectCIDR(subnetDeps{
		underWSL:   func() bool { return true },
		hostLAN:    func() (string, error) { return "", errors.New("no interop") },
		localCIDRs: func() ([]string, error) { return []string{"192.168.0.236/24"}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "192.168.0.236/24" {
		t.Fatalf("cidr = %q, want the local interface fallback", got)
	}
}

// Off WSL the host query must never run: the local interface is the answer.
func TestDetectCIDRUsesLocalInterfacesWhenNotUnderWSL(t *testing.T) {
	called := false
	got, err := detectCIDR(subnetDeps{
		underWSL:   func() bool { return false },
		hostLAN:    func() (string, error) { called = true; return "192.168.0.236/24", nil },
		localCIDRs: func() ([]string, error) { return []string{"10.0.0.5/24"}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("queried the Windows host while not under WSL")
	}
	if got != "10.0.0.5/24" {
		t.Fatalf("cidr = %q, want 10.0.0.5/24", got)
	}
}

// The sweep probes by raw ADDRESS, so no per-alias Host block matches and the
// fleet key is never offered. The credentials the fleet already uses must be
// gathered from the config and presented explicitly.
func TestScanIdentitiesCollectsDistinctFleetCredentials(t *testing.T) {
	hosts := []sshconf.Host{
		{Alias: "a", User: "operator", Identity: "~/.ssh/id_fleet", Fleet: true},
		{Alias: "b", User: "operator", Identity: "~/.ssh/id_fleet", Fleet: true}, // duplicate
		{Alias: "c", User: "operator", Identity: "~/.ssh/id_other", Fleet: true},
		{Alias: "d", User: "nobody", Identity: "~/.ssh/id_skip"}, // not in fleet
	}
	ids := scanIdentities(hosts)
	// Two distinct fleet credentials, then the zero identity — a bare `ssh <ip>`,
	// which is the right answer when a wildcard Host block or the agent already
	// supplies the key. It must come LAST so an explicit credential wins.
	want := []scanIdentity{
		{User: "operator", Identity: "~/.ssh/id_fleet"},
		{User: "operator", Identity: "~/.ssh/id_other"},
		{},
	}
	if len(ids) != len(want) {
		t.Fatalf("identities = %+v, want %+v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("identities = %+v, want %+v", ids, want)
		}
	}
	for _, id := range ids {
		if id.Identity == "~/.ssh/id_skip" {
			t.Fatal("a non-fleet host's credential was used for probing")
		}
	}
}

// identify must try each fleet credential until one authenticates, rather than
// giving up after a bare connection that offers no key.
func TestIdentifyTriesEachFleetIdentityUntilOneAuthenticates(t *testing.T) {
	ids := []scanIdentity{
		{User: "operator", Identity: "~/.ssh/id_wrong"},
		{User: "operator", Identity: "~/.ssh/id_right"},
	}
	mk := func(id scanIdentity) runner.Runner {
		if id.Identity == "~/.ssh/id_right" {
			return runner.Fake{Out: map[string]string{"10.0.0.5": "named-box"}}
		}
		return runner.Fake{Err: map[string]error{"10.0.0.5": errors.New("permission denied")}}
	}
	got := identify(mk, ids, []string{"10.0.0.5"}, 4)
	if len(got) != 1 {
		t.Fatalf("responders = %d, want 1", len(got))
	}
	if !got[0].Identified || got[0].Hostname != "named-box" {
		t.Fatalf("responder = %+v, want identified as named-box", got[0])
	}
}

// A host that answers :22 but accepts no fleet credential stays unidentified —
// it must never be guessed at or written into the config.
func TestIdentifyLeavesAHostWithNoWorkingCredentialUnidentified(t *testing.T) {
	mk := func(scanIdentity) runner.Runner {
		return runner.Fake{Err: map[string]error{"10.0.0.11": errors.New("permission denied")}}
	}
	got := identify(mk, []scanIdentity{{Identity: "~/.ssh/id_fleet"}}, []string{"10.0.0.11"}, 4)
	if got[0].Identified {
		t.Fatalf("responder = %+v, want unidentified", got[0])
	}
}
