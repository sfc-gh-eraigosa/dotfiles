package winhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeRunner replays recorded PowerShell output. Every Windows interop path in
// this module goes through Runner precisely so the rest can be tested in CI,
// where no powershell.exe exists.
type fakeRunner struct {
	byKind map[scriptKind]string
	err    error
	calls  int
}

func (f *fakeRunner) Run(_ context.Context, s script) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out, ok := f.byKind[s.kind]
	if !ok {
		return nil, errors.New("fakeRunner: no fixture for " + string(s.kind))
	}
	return []byte(out), nil
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	// PowerShell emits CRLF; the parser must tolerate it.
	return string(b) + "\r\n"
}

func newFake(t *testing.T) *fakeRunner {
	t.Helper()
	return &fakeRunner{byKind: map[scriptKind]string{
		kindDNSServers: fixture(t, "dns_servers.json"),
		kindAdapters:   fixture(t, "adapters.json"),
	}}
}

func byAlias(ifaces []Interface, alias string) (Interface, bool) {
	for _, i := range ifaces {
		if i.Alias == alias {
			return i, true
		}
	}
	return Interface{}, false
}

// EC-1 depends on seeing EVERY interface's resolver, not just the default
// route's — so enumeration must not drop or merge rows.
func TestInterfaces_EnumeratesEveryInterfaceWithItsResolvers(t *testing.T) {
	got, err := New(newFake(t)).Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces() error = %v", err)
	}
	if len(got) != 6 {
		t.Fatalf("got %d interfaces, want 6 (every row, including the empty ones)", len(got))
	}

	wifi, ok := byAlias(got, "Wi-Fi")
	if !ok {
		t.Fatal("Wi-Fi missing from enumeration")
	}
	if len(wifi.DNSServers) != 2 || wifi.DNSServers[0] != "198.51.100.53" {
		t.Errorf("Wi-Fi DNSServers = %v, want both ISP resolvers in order", wifi.DNSServers)
	}

	tun, ok := byAlias(got, "wg-lab")
	if !ok {
		t.Fatal("wg-lab missing from enumeration")
	}
	if len(tun.DNSServers) != 1 || tun.DNSServers[0] != "10.10.0.1" {
		t.Errorf("wg-lab DNSServers = %v, want [10.10.0.1]", tun.DNSServers)
	}
}

// The tunnel's resolver is the one that knows the fleet, and it is NOT on the
// default route — identifying it is what makes the readiness diagnosis possible.
func TestInterfaces_IdentifiesTheTunnel(t *testing.T) {
	got, err := New(newFake(t)).Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces() error = %v", err)
	}
	for _, tc := range []struct {
		alias string
		want  bool
	}{
		{"wg-lab", true},
		{"Wi-Fi", false},
		{"Bluetooth Network Connection", false},
		{"vEthernet (WSL (Hyper-V firewall))", false},
	} {
		i, ok := byAlias(got, tc.alias)
		if !ok {
			t.Fatalf("%s missing", tc.alias)
		}
		if i.IsTunnel != tc.want {
			t.Errorf("%s IsTunnel = %v, want %v", tc.alias, i.IsTunnel, tc.want)
		}
	}
}

// An interface that is present but has no resolver must survive enumeration as
// an empty entry rather than vanishing: "present with none" and "absent" are
// different facts, and only the caller can decide what they mean.
func TestInterfaces_KeepsInterfacesWithNoResolvers(t *testing.T) {
	got, err := New(newFake(t)).Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces() error = %v", err)
	}
	lo, ok := byAlias(got, "Loopback Pseudo-Interface 1")
	if !ok {
		t.Fatal("loopback dropped from enumeration")
	}
	if len(lo.DNSServers) != 0 {
		t.Errorf("loopback DNSServers = %v, want empty", lo.DNSServers)
	}
}

// PowerShell's ConvertTo-Json emits a BARE OBJECT when exactly one row matches,
// and an array otherwise. A parser that assumes an array silently returns
// nothing on a single-interface machine — the classic trap.
func TestInterfaces_HandlesSingleObjectJSON(t *testing.T) {
	f := &fakeRunner{byKind: map[scriptKind]string{
		kindDNSServers: fixture(t, "dns_servers_single.json"),
		kindAdapters:   fixture(t, "adapters.json"),
	}}
	got, err := New(f).Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces() error = %v", err)
	}
	if len(got) != 1 || got[0].Alias != "wg-lab" {
		t.Fatalf("got %+v, want a single wg-lab interface", got)
	}
	if !got[0].IsTunnel {
		t.Error("single-object path lost the tunnel classification")
	}
}

// Interop failing is normal (no powershell.exe, interop PATH broken). It must
// surface as an error the caller can decline on, never a panic or silent zero.
func TestInterfaces_PropagatesRunnerFailure(t *testing.T) {
	f := &fakeRunner{err: errors.New("boom")}
	if _, err := New(f).Interfaces(context.Background()); err == nil {
		t.Fatal("Interfaces() error = nil, want the runner's failure surfaced")
	}
}

// Tunnel classification drives the readiness diagnosis, so its inputs are
// pinned here rather than left to the adapter fixture alone.
func TestIsTunnelAdapter(t *testing.T) {
	for _, tc := range []struct {
		name, alias, desc string
		want              bool
	}{
		{"wireguard by description", "wg-lab", "WireGuard Tunnel", true},
		{"wintun by description", "tun0", "Wintun Userspace Tunnel", true},
		{"tap by description", "tap-a", "TAP-Windows Adapter V9", true},
		{"wg-prefixed alias", "wg65abcdef", "", true},
		{"tailscale by alias", "Tailscale", "", true},
		{"physical wifi", "Wi-Fi", "Intel(R) Wi-Fi 6E AX211 160MHz", false},
		{"hyper-v vswitch is not a tunnel", "vEthernet (WSL)", "Hyper-V Virtual Ethernet Adapter", false},
		{"bluetooth pan is not a tunnel", "Bluetooth Network Connection", "Bluetooth Device (Personal Area Network)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTunnelAdapter(tc.alias, tc.desc); got != tc.want {
				t.Errorf("isTunnelAdapter(%q, %q) = %v, want %v", tc.alias, tc.desc, got, tc.want)
			}
		})
	}
}

// PowerShell emits nothing at all when no rows match. That is a legitimate
// empty result, not a parse failure — treating it as an error would turn a
// quiet machine into a hard stop.
func TestDecodeRows_EmptyOutputIsNotAnError(t *testing.T) {
	for _, in := range []string{"", "   ", "\r\n", "\xef\xbb\xbf\r\n"} {
		got, err := decodeRows[dnsRow]([]byte(in))
		if err != nil {
			t.Errorf("decodeRows(%q) error = %v, want nil", in, err)
		}
		if len(got) != 0 {
			t.Errorf("decodeRows(%q) = %v, want empty", in, got)
		}
	}
}

// Windows tooling frequently prefixes UTF-8 output with a BOM, which is not
// valid JSON to a strict decoder.
func TestDecodeRows_StripsUTF8BOM(t *testing.T) {
	raw := "\xef\xbb\xbf[{\"InterfaceAlias\":\"wg-lab\",\"InterfaceIndex\":42,\"ServerAddresses\":[\"10.10.0.1\"]}]\r\n"
	got, err := decodeRows[dnsRow]([]byte(raw))
	if err != nil {
		t.Fatalf("decodeRows with BOM error = %v", err)
	}
	if len(got) != 1 || got[0].InterfaceAlias != "wg-lab" {
		t.Fatalf("decodeRows with BOM = %+v, want one wg-lab row", got)
	}
}

// Garbage must surface as an error rather than an empty result: silently
// returning "no interfaces" would make wlink decline for the wrong reason.
func TestDecodeRows_MalformedIsAnError(t *testing.T) {
	if _, err := decodeRows[dnsRow]([]byte("not json at all")); err == nil {
		t.Fatal("decodeRows(garbage) error = nil, want a parse error")
	}
}

// The adapter query is a refinement, not a requirement: a machine where
// Get-NetAdapter fails must still get its resolver list, just without tunnel
// classification from the description.
func TestInterfaces_SurvivesAdapterQueryFailure(t *testing.T) {
	f := &fakeRunner{byKind: map[scriptKind]string{
		kindDNSServers: fixture(t, "dns_servers.json"),
		// no kindAdapters entry: the adapter query errors
	}}
	got, err := New(f).Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces() error = %v, want the DNS list despite adapter failure", err)
	}
	if len(got) != 6 {
		t.Fatalf("got %d interfaces, want 6", len(got))
	}
	// Alias-based detection is the fallback when descriptions are unavailable.
	tun, _ := byAlias(got, "wg-lab")
	if !tun.IsTunnel {
		t.Error("wg-lab not classified as a tunnel via its alias when adapter data was missing")
	}
}

// Tunnel aliases come in two flavours: client-generated (wg65fffd80) and
// user-named (wg0, wg-lab, wg_home). Both must classify without adapter data,
// because Get-NetAdapter is best-effort.
func TestIsTunnelAdapter_AliasShapesWithoutDescription(t *testing.T) {
	for _, tc := range []struct {
		alias string
		want  bool
	}{
		{"wg65fffd80", true}, {"wg0", true}, {"wg-lab", true},
		{"wg_home", true}, {"WG-Lab", true}, {"wg", true},
		{"wgetmirror", true}, // acceptable false positive; see wgAlias comment
		{"Wi-Fi", false}, {"Ethernet", false}, {"vEthernet (WSL)", false},
	} {
		if got := isTunnelAdapter(tc.alias, ""); got != tc.want {
			t.Errorf("isTunnelAdapter(%q, \"\") = %v, want %v", tc.alias, got, tc.want)
		}
	}
}
