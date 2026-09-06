package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/probe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/resolvconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/winhost"
)

type fakeHost struct {
	ifaces []winhost.Interface
	err    error
}

func (f fakeHost) Interfaces(context.Context) ([]winhost.Interface, error) {
	return f.ifaces, f.err
}

type fakeZone map[string]map[string]probe.Result

func (z fakeZone) LookupA(_ context.Context, server, name string) probe.Result {
	names, ok := z[server]
	if !ok {
		return probe.Result{Outcome: probe.Silent}
	}
	if r, ok := names[name]; ok {
		return r
	}
	return probe.Result{Outcome: probe.NoAddress}
}

func addr(a string) probe.Result {
	return probe.Result{Outcome: probe.Resolved, Addrs: []string{a}}
}

// A machine with the fleet resolver on a tunnel and an ignorant ISP resolver on
// the default route — the shape this tool exists for.
func healthyRuntime(t *testing.T) (*Runtime, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	shared := filepath.Join(root, "shared-resolv.conf")
	if err := os.WriteFile(shared, []byte("nameserver 10.255.255.254\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolv := filepath.Join(root, "resolv.conf")
	if err := os.Symlink(shared, resolv); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wsl.conf"), []byte("[boot]\nsystemd=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	return &Runtime{
		WSL: true,
		Host: fakeHost{ifaces: []winhost.Interface{
			{Alias: "Wi-Fi", DNSServers: []string{"198.51.100.53"}},
			{Alias: "wg-lab", DNSServers: []string{"10.10.0.1"}, IsTunnel: true},
			{Alias: "Loopback Pseudo-Interface 1", DNSServers: []string{"127.0.0.53"}},
		}},
		Lookup: fakeZone{
			"198.51.100.53": {"github.com": addr("198.51.100.10")},
			"10.10.0.1": {
				"lab-pi":     addr("10.10.0.21"),
				"github.com": addr("198.51.100.10"),
			},
		},
		FleetHosts:     []string{"lab-pi"},
		PublicSentinel: "github.com",
		Paths: resolvconf.Paths{
			ResolvConf: resolv,
			WslConf:    filepath.Join(root, "wsl.conf"),
			BackupDir:  filepath.Join(root, "backup"),
		},
		Out: out,
	}, out
}

func TestPin_PinsTheFleetResolverAndReportsIt(t *testing.T) {
	rt, out := healthyRuntime(t)
	code, err := rt.Pin(context.Background())
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	content, _ := os.ReadFile(rt.Paths.ResolvConf)
	if !strings.Contains(string(content), "nameserver 10.10.0.1") {
		t.Errorf("resolv.conf did not pin the tunnel resolver:\n%s", content)
	}
	if !strings.Contains(out.String(), "10.10.0.1") {
		t.Error("output does not name the selected resolver")
	}
}

// A safe decline is exit 0. install.sh must never fail because a tunnel happens
// to be down when it runs.
func TestPin_DeclinesSafelyWhenNothingResolvesTheFleet(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.Lookup = fakeZone{} // nothing answers at all
	code, err := rt.Pin(context.Background())
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0 — a decline is not a failure", code)
	}
	if fi, _ := os.Lstat(rt.Paths.ResolvConf); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("resolv.conf was rewritten despite no winner")
	}
	if !strings.Contains(out.String(), "NOT READY") {
		t.Errorf("all-silent must be diagnosed as a tunnel not ready, got:\n%s", out)
	}
}

// Some resolvers answered, none knew the fleet: that is a wrong-tunnel
// diagnosis, NOT a handshake still completing.
func TestPin_DistinguishesWrongTunnelFromNotReady(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.Lookup = fakeZone{
		"198.51.100.53": {"github.com": addr("198.51.100.10")},
		"10.10.0.1":     {"github.com": addr("198.51.100.10")},
	}
	code, _ := rt.Pin(context.Background())
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if strings.Contains(out.String(), "NOT READY") {
		t.Errorf("resolvers answered, so this is not a readiness problem:\n%s", out)
	}
}

// EC-2 at the command level: the guard refuses and nothing is written.
func TestPin_RefusesToPinANonRecursiveResolver(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.Lookup = fakeZone{"10.10.0.1": {"lab-pi": addr("10.10.0.21")}} // no sentinel
	code, err := rt.Pin(context.Background())
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if fi, _ := os.Lstat(rt.Paths.ResolvConf); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("wrote a non-recursive resolver — that would break public DNS")
	}
	if !strings.Contains(out.String(), "public") {
		t.Errorf("refusal must explain the public-DNS consequence:\n%s", out)
	}
}

func TestPin_OverrideAllowsTheNonRecursivePinButWarns(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.Lookup = fakeZone{"10.10.0.1": {"lab-pi": addr("10.10.0.21")}}
	rt.AllowNonRecursive = true
	if _, err := rt.Pin(context.Background()); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	content, _ := os.ReadFile(rt.Paths.ResolvConf)
	if !strings.Contains(string(content), "nameserver 10.10.0.1") {
		t.Error("override did not pin")
	}
	if !strings.Contains(strings.ToUpper(out.String()), "WARNING") {
		t.Errorf("override must warn loudly:\n%s", out)
	}
}

// EC-12: off WSL, every command is a no-op that exits 0. Reporting failure
// would make install.sh look broken on machines the feature never applied to.
func TestPin_NonWSLIsANoOp(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.WSL = false
	code, err := rt.Pin(context.Background())
	if err != nil || code != 0 {
		t.Errorf("Pin off WSL = (%d, %v), want (0, nil)", code, err)
	}
	if fi, _ := os.Lstat(rt.Paths.ResolvConf); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("wrote files on a non-WSL host")
	}
	if out.Len() == 0 {
		t.Error("a no-op must still say why it did nothing")
	}
}

// EC-4 at the command level: no undo path means no write, and it is still a
// safe decline rather than a hard failure.
func TestPin_DeclinesWhenTheSnapshotCannotBeTaken(t *testing.T) {
	rt, out := healthyRuntime(t)
	root := filepath.Dir(rt.Paths.BackupDir)
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	code, err := rt.Pin(context.Background())
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if fi, _ := os.Lstat(rt.Paths.ResolvConf); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("wrote without an undo path")
	}
	if !strings.Contains(strings.ToLower(out.String()), "undo") {
		t.Errorf("must say why it refused:\n%s", out)
	}
}

// Dry run reports the whole decision and writes nothing.
func TestPin_DryRunWritesNothing(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.DryRun = true
	if _, err := rt.Pin(context.Background()); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if fi, _ := os.Lstat(rt.Paths.ResolvConf); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("--dry-run wrote to resolv.conf")
	}
	if resolvconf.HasSnapshot(rt.Paths) {
		t.Error("--dry-run took a snapshot")
	}
	if !strings.Contains(out.String(), "10.10.0.1") {
		t.Error("--dry-run must still report what it would do")
	}
}

// EC-5 reporting: an excluded name must be announced, so a score below the host
// count is never a mystery.
func TestPin_AnnouncesHostsFileExclusions(t *testing.T) {
	rt, out := healthyRuntime(t)
	rt.ExcludedHosts = []string{"selfhost"}
	if _, err := rt.Pin(context.Background()); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if !strings.Contains(out.String(), "selfhost") {
		t.Errorf("excluded host not announced:\n%s", out)
	}
}

// Interop failing is not a crash: it means Windows is unreachable from here, so
// wlink declines.
func TestPin_DeclinesWhenWindowsIsUnreachable(t *testing.T) {
	rt, _ := healthyRuntime(t)
	rt.Host = fakeHost{err: errors.New("no powershell.exe")}
	code, err := rt.Pin(context.Background())
	if err != nil {
		t.Fatalf("Pin returned a hard error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

func TestUnpin_RestoresAndReports(t *testing.T) {
	rt, out := healthyRuntime(t)
	if _, err := rt.Pin(context.Background()); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	code, err := rt.Unpin(context.Background())
	if err != nil {
		t.Fatalf("Unpin: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if _, err := os.Readlink(rt.Paths.ResolvConf); err != nil {
		t.Errorf("resolv.conf not restored to a symlink: %v", err)
	}
	if out.Len() == 0 {
		t.Error("unpin must report what it restored")
	}
}

// Unpinning a machine that was never pinned is a no-op, not an error.
func TestUnpin_WithNothingPinnedIsSafe(t *testing.T) {
	rt, _ := healthyRuntime(t)
	rt.Paths.StockResolvTarget = filepath.Join(filepath.Dir(rt.Paths.ResolvConf), "stock")
	code, err := rt.Unpin(context.Background())
	if err != nil {
		t.Fatalf("Unpin: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}

// EC-22: fallbacks come from the resolvers already present, and a public one is
// opt-in rather than silently hardcoded.
func TestPin_ExtraFallbacksAreOptIn(t *testing.T) {
	rt, _ := healthyRuntime(t)
	if _, err := rt.Pin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, rt.Paths.ResolvConf)); strings.Contains(got, "1.1.1.1") {
		t.Errorf("appended a third-party resolver nobody asked for:\n%s", got)
	}

	rt2, _ := healthyRuntime(t)
	rt2.ExtraFallbacks = []string{"1.1.1.1"}
	if _, err := rt2.Pin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, rt2.Paths.ResolvConf)); !strings.Contains(got, "1.1.1.1") {
		t.Errorf("WLINK_FALLBACKS not honoured:\n%s", got)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
