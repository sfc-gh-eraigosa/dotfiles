package cmd

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/hostkey"
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

// An unidentified row must carry ssh's own reason: a host-key gap, a refused key
// and a dropped connection are different fixes, and "would not authenticate"
// sent the operator after the wrong one.
func TestIdentifyRecordsWhyAResponderWasNotIdentified(t *testing.T) {
	fail := func(stderr string) error {
		_, err := exec.Command("sh", "-c", `printf '%s' "$1" >&2; exit 255`, "sh", stderr).Output()
		return err
	}
	errs := map[string]error{
		"10.0.0.1": fail("Host key verification failed."),
		"10.0.0.2": fail("op@10.0.0.2: Permission denied (publickey,password)."),
		"10.0.0.3": fail("op@10.0.0.3: Permission denied (publickey)."),
		"10.0.0.4": fail("Connection closed by 10.0.0.4 port 22"),
		"10.0.0.5": errors.New("no stderr to read"),
		"10.0.0.6": fail("ssh: connect to host 10.0.0.6 port 22: Connection timed out"),
	}
	out := map[string]string{"10.0.0.7": "", "10.0.0.8": "x\n    ProxyCommand sh -c 'touch /tmp/pwned'"}
	mk := func(scanIdentity) runner.Runner { return runner.Fake{Err: errs, Out: out} }
	got := map[string]responder{}
	for _, r := range identify(mk, nil, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5", "10.0.0.6", "10.0.0.7", "10.0.0.8"}, 4) {
		got[r.IP] = r
	}
	want := map[string]string{
		"10.0.0.1": "host key unverified",
		"10.0.0.2": "key not accepted; host takes a password",
		"10.0.0.3": "key not accepted",
		"10.0.0.4": "closed the connection",
		"10.0.0.5": "would not authenticate",
		"10.0.0.6": "timed out",
		"10.0.0.7": "logged in, but reported no hostname",
		"10.0.0.8": "logged in, but reported an unusable hostname",
	}
	for ip, w := range want {
		if got[ip].Identified || got[ip].Cause != w {
			t.Errorf("%s = %+v, want unidentified with cause %q", ip, got[ip], w)
		}
	}
	rows := classifyScan(nil, []responder{{IP: "10.0.0.2", Cause: want["10.0.0.2"]}}, nil)
	if r := renderScan(rows); !strings.Contains(r, "key not accepted; host takes a password — not written") {
		t.Fatalf("render dropped the cause:\n%s", r)
	}
}

// Probes accept unknown host keys, so any LAN device can answer `hostname` with
// an existing alias. Only a responder presenting that alias's own key is a move.
func TestAClaimedMoveIsWrittenOnlyWhenTheHostKeyProvesIt(t *testing.T) {
	cfg := "Host nas  #fleet\n    HostName 10.0.0.9\n"
	hosts, _ := sshconf.Parse(cfg, "#fleet")
	for _, tc := range []struct {
		verdict moveVerdict
		kind    scanKind
	}{
		{moveVerified, scanMoved},
		{moveMismatch, scanUnverified},
		{moveUnverifiable, scanUnverified},
	} {
		rows := classifyScan(hosts, []responder{{IP: "10.0.0.66", Hostname: "nas", Identified: true}}, nil)
		rows = verifyMoves(rows, func(string, string) moveVerdict { return tc.verdict })
		if rows[0].Kind != tc.kind {
			t.Fatalf("verdict %d: kind = %q, want %q", tc.verdict, rows[0].Kind, tc.kind)
		}
		next, _, err := applyScan(cfg, rows, "#fleet")
		if err != nil {
			t.Fatal(err)
		}
		if moved := strings.Contains(next, "10.0.0.66"); moved != (tc.kind == scanMoved) {
			t.Fatalf("verdict %d: rewrote=%v, config:\n%s", tc.verdict, moved, next)
		}
	}
}

func TestJudgeMove(t *testing.T) {
	ed := func(fp string) hostkey.Key { return hostkey.Key{Type: "ED25519", Fingerprint: fp} }
	rsa := hostkey.Key{Type: "RSA", Fingerprint: "SHA256:rsa"}
	for _, tc := range []struct {
		name              string
		stored, presented []hostkey.Key
		want              moveVerdict
	}{
		{"same key", []hostkey.Key{ed("a"), rsa}, []hostkey.Key{ed("a")}, moveVerified},
		{"swapped key", []hostkey.Key{ed("a")}, []hostkey.Key{ed("b")}, moveMismatch},
		{"nothing stored", nil, []hostkey.Key{ed("a")}, moveUnverifiable},
		{"only another type stored", []hostkey.Key{rsa}, []hostkey.Key{ed("a")}, moveUnverifiable},
	} {
		if got := judgeMove(tc.stored, tc.presented); got != tc.want {
			t.Errorf("%s: judgeMove = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// The bootstrap hint is about a refused KEY. It must stay quiet when nothing
// was refused for that reason (timeouts, a changed host key), and once any
// host was identified.
func TestScanHintOnlyWhenARefusedKeyIsTheProblem(t *testing.T) {
	refused := []scanRow{{IP: "10.0.0.1", Kind: scanUnidentified, Cause: "key not accepted"}}
	if !strings.Contains(scanHint(refused), "ssh-copy-id") {
		t.Fatalf("hint = %q, want the bootstrap path", scanHint(refused))
	}
	for name, rows := range map[string][]scanRow{
		"identified": append(refused, scanRow{IP: "10.0.0.2", Kind: scanCurrent}),
		"timeouts":   {{IP: "10.0.0.1", Kind: scanUnidentified, Cause: "timed out"}},
		"changed":    {{IP: "10.0.0.1", Kind: scanUnidentified, Cause: "host key CHANGED"}},
		"empty":      nil,
	} {
		if h := scanHint(rows); strings.Contains(h, "ssh-copy-id") {
			t.Errorf("%s: hint = %q, want no ssh-copy-id advice", name, h)
		}
	}
	claimed := []scanRow{{IP: "10.0.0.66", Alias: "nas", Kind: scanUnverified}}
	if h := scanHint(claimed); !strings.Contains(h, "fleet trust nas") {
		t.Fatalf("hint = %q, want the fleet trust recovery for an unverified move", h)
	}
}

// The probe must read every known_hosts file the operator's config names, not a
// hardcoded one, with the scratch file first.
func TestScanKnownHostsKeepsTheOperatorsFiles(t *testing.T) {
	got := scanKnownHosts("/tmp/s/kh", "10.0.0.5", func(string) (hostkey.Target, error) {
		return hostkey.Target{KnownHosts: []string{"/h/.ssh/custom_hosts", "/h/.ssh/known_hosts2"}}, nil
	})
	want := []string{"/tmp/s/kh", "/h/.ssh/custom_hosts", "/h/.ssh/known_hosts2"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("scanKnownHosts = %q, want %q", got, want)
	}
	fallback := scanKnownHosts("/tmp/s/kh", "10.0.0.5", func(string) (hostkey.Target, error) { return hostkey.Target{}, errors.New("no ssh") })
	if fallback[0] != "/tmp/s/kh" || len(fallback) < 2 {
		t.Fatalf("fallback = %q, want scratch first then ssh's defaults", fallback)
	}
}
