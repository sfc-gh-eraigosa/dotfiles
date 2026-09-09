package fleetsrc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeCmd struct {
	out  string
	err  error
	call string
}

func (f *fakeCmd) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.call = name + " " + args[0]
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.out), nil
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const fleetJSON = `[
  {"alias":"lab-pi","hostname":"lab-pi","in_fleet":true},
  {"alias":"lab-nas","hostname":"lab-nas","in_fleet":true},
  {"alias":"not-adopted","hostname":"not-adopted","in_fleet":false}
]`

// fleet owns the #fleet marker (fleet add/discover/remove manage those blocks),
// so wlink consumes its contract instead of maintaining a second parser that
// would drift from it.
func TestResolve_PrefersFleetDiscoverJSON(t *testing.T) {
	dir := t.TempDir()
	cmd := &fakeCmd{out: fleetJSON}
	s := Source{
		Cmd:       cmd,
		SSHConfig: writeFile(t, dir, "ssh_config", "Host should-not-be-read  #fleet\n    Hostname should-not-be-read\n"),
		HostsFile: writeFile(t, dir, "hosts", "127.0.0.1 localhost\n"),
	}
	got, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Origin != OriginFleet {
		t.Errorf("Origin = %q, want %q", got.Origin, OriginFleet)
	}
	if !reflect.DeepEqual(got.Probe, []string{"lab-pi", "lab-nas"}) {
		t.Errorf("Probe = %v, want the two in-fleet hosts", got.Probe)
	}
	if cmd.call != "fleet discover" {
		t.Errorf("invoked %q, want `fleet discover`", cmd.call)
	}
}

// A host present in the ssh config but not adopted into the fleet is not ours
// to probe — scoring against it would understate every resolver.
func TestResolve_IgnoresHostsNotInTheFleet(t *testing.T) {
	dir := t.TempDir()
	s := Source{
		Cmd:       &fakeCmd{out: fleetJSON},
		SSHConfig: writeFile(t, dir, "ssh_config", ""),
		HostsFile: writeFile(t, dir, "hosts", ""),
	}
	got, _ := s.Resolve(context.Background())
	for _, h := range got.Probe {
		if h == "not-adopted" {
			t.Error("probed a host with in_fleet=false")
		}
	}
}

// fleet may not be installed. Falling back to a READ-ONLY scan of the ssh
// config keeps wlink usable, without wlink ever writing those blocks — that
// remains fleet's job.
func TestResolve_FallsBackToSSHConfigWhenFleetIsAbsent(t *testing.T) {
	dir := t.TempDir()
	cfg := `Host lab-pi  #fleet
    Hostname lab-pi

Host lab-nas  #fleet
    Hostname lab-nas

Host not-fleet
    Hostname not-fleet
`
	s := Source{
		Cmd:       &fakeCmd{err: errors.New("exec: fleet: not found")},
		SSHConfig: writeFile(t, dir, "ssh_config", cfg),
		HostsFile: writeFile(t, dir, "hosts", ""),
	}
	got, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Origin != OriginSSHConfig {
		t.Errorf("Origin = %q, want %q", got.Origin, OriginSSHConfig)
	}
	if !reflect.DeepEqual(got.Probe, []string{"lab-pi", "lab-nas"}) {
		t.Errorf("Probe = %v, want only the #fleet-marked hosts", got.Probe)
	}
}

// EC-13: a wildcard Host block cannot be resolved, so probing it would score
// every resolver as failing on a name that never had an address.
func TestResolve_NeverProbesWildcardHostPatterns(t *testing.T) {
	dir := t.TempDir()
	cfg := `Host lab-pi  #fleet
    Hostname lab-pi

Host *  #fleet
    ServerAliveInterval 30

Host lab-?  #fleet
    User someone
`
	s := Source{
		Cmd:       &fakeCmd{err: errors.New("no fleet")},
		SSHConfig: writeFile(t, dir, "ssh_config", cfg),
		HostsFile: writeFile(t, dir, "hosts", ""),
	}
	got, _ := s.Resolve(context.Background())
	if !reflect.DeepEqual(got.Probe, []string{"lab-pi"}) {
		t.Errorf("Probe = %v, want only lab-pi (patterns are not resolvable)", got.Probe)
	}
}

// EC-5, the rule that came out of a real capped score. nsswitch is "files dns",
// so a name in /etc/hosts is answered BEFORE any resolver is consulted. Probing
// it would cap the score below 100% forever, and would make verify count a
// /etc/hosts hit as evidence the RESOLVER works — a false positive about the
// very thing being verified.
func TestResolve_ExcludesNamesServedByHostsFile(t *testing.T) {
	dir := t.TempDir()
	hosts := `127.0.0.1	localhost
127.0.1.1	selfhost.localdomain	selfhost
# 203.0.113.9 commented-out
`
	s := Source{
		Cmd:       &fakeCmd{out: `[{"alias":"lab-pi","hostname":"lab-pi","in_fleet":true},{"alias":"selfhost","hostname":"selfhost","in_fleet":true},{"alias":"commented-out","hostname":"commented-out","in_fleet":true}]`},
		SSHConfig: writeFile(t, dir, "ssh_config", ""),
		HostsFile: writeFile(t, dir, "hosts", hosts),
	}
	got, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !reflect.DeepEqual(got.Probe, []string{"lab-pi", "commented-out"}) {
		t.Errorf("Probe = %v, want selfhost excluded and the commented name kept", got.Probe)
	}
	if !reflect.DeepEqual(got.Excluded, []string{"selfhost"}) {
		t.Errorf("Excluded = %v, want [selfhost] — it must be reported, not silently dropped", got.Excluded)
	}
}

// A Hostname that is already an IP has nothing for DNS to resolve; probing it
// would penalise every candidate for a name no resolver was ever asked about.
func TestResolve_SkipsHostnamesThatAreAlreadyAddresses(t *testing.T) {
	dir := t.TempDir()
	s := Source{
		Cmd:       &fakeCmd{out: `[{"alias":"pinned","hostname":"10.10.0.21","in_fleet":true},{"alias":"lab-pi","hostname":"lab-pi","in_fleet":true}]`},
		SSHConfig: writeFile(t, dir, "ssh_config", ""),
		HostsFile: writeFile(t, dir, "hosts", ""),
	}
	got, _ := s.Resolve(context.Background())
	if !reflect.DeepEqual(got.Probe, []string{"lab-pi"}) {
		t.Errorf("Probe = %v, want the IP-hostname entry skipped", got.Probe)
	}
	if !reflect.DeepEqual(got.Excluded, []string{"pinned"}) {
		t.Errorf("Excluded = %v, want the IP-hostname entry reported as excluded", got.Excluded)
	}
}

// The name ssh will actually resolve is Hostname, not the alias.
func TestResolve_ProbesTheHostnameNotTheAlias(t *testing.T) {
	dir := t.TempDir()
	s := Source{
		Cmd:       &fakeCmd{out: `[{"alias":"pi","hostname":"lab-pi.internal","in_fleet":true}]`},
		SSHConfig: writeFile(t, dir, "ssh_config", ""),
		HostsFile: writeFile(t, dir, "hosts", ""),
	}
	got, _ := s.Resolve(context.Background())
	if !reflect.DeepEqual(got.Probe, []string{"lab-pi.internal"}) {
		t.Errorf("Probe = %v, want the Hostname (what ssh resolves), not the alias", got.Probe)
	}
}

// EC-15: no fleet hosts is a clean, explainable no-op — not an error.
func TestResolve_NoFleetHostsIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	s := Source{
		Cmd:       &fakeCmd{out: `[]`},
		SSHConfig: writeFile(t, dir, "ssh_config", ""),
		HostsFile: writeFile(t, dir, "hosts", ""),
	}
	got, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v, want a clean empty result", err)
	}
	if len(got.Probe) != 0 {
		t.Errorf("Probe = %v, want empty", got.Probe)
	}
}

// An explicit override skips discovery entirely — the escape hatch for a
// machine whose fleet is not in ssh config at all.
func TestResolve_ExplicitOverrideWins(t *testing.T) {
	dir := t.TempDir()
	s := Source{
		Cmd:       &fakeCmd{out: fleetJSON},
		SSHConfig: writeFile(t, dir, "ssh_config", ""),
		HostsFile: writeFile(t, dir, "hosts", ""),
		Override:  []string{"one", "two"},
	}
	got, _ := s.Resolve(context.Background())
	if got.Origin != OriginOverride {
		t.Errorf("Origin = %q, want %q", got.Origin, OriginOverride)
	}
	if !reflect.DeepEqual(got.Probe, []string{"one", "two"}) {
		t.Errorf("Probe = %v, want the override", got.Probe)
	}
}

func TestHostsFileNames(t *testing.T) {
	got := hostsFileNames("127.0.0.1\tlocalhost\n127.0.1.1  box.localdomain box\n\n# 1.2.3.4 nope\n::1 ip6-localhost\n")
	want := map[string]bool{"localhost": true, "box.localdomain": true, "box": true, "ip6-localhost": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hostsFileNames() = %v, want %v", got, want)
	}
}
