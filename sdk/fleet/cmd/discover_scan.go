package cmd

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/lanscan"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

// responder is one address that answered on :22, plus what it said its name is
// when we could authenticate. Identified is false when the port answered but
// no usable session followed — that is a real and common state on a home LAN.
type responder struct {
	IP, Hostname string
	Identified   bool
}

type scanKind string

const (
	scanCurrent      scanKind = "current"      // known alias, address unchanged
	scanMoved        scanKind = "moved"        // known alias at a NEW address
	scanNew          scanKind = "new"          // identified, not in the config
	scanUnidentified scanKind = "unidentified" // answered :22, would not authenticate
)

type scanRow struct {
	IP, Hostname, Alias, WasIP string
	Kind                       scanKind
}

// classifyScan decides what the sweep found relative to the config we already
// have. It is pure, so the whole decision is testable without a network.
//
// The moved case is the reason this exists: DHCP reassigns a box, the config
// keeps pointing at the old address, and every fleet command reports it dead.
// Matching on the host's OWN reported name — not on the address — is what
// turns that into a one-line refresh instead of a duplicate Host block.
//
// resolve maps a configured HostName to its addresses, and is what keeps a
// NAME-based config stable. A block may legitimately say `HostName named-box`
// rather than an address; comparing that string to "192.168.0.63" is never
// equal, so without resolution every named host reported `moved` on every run
// and each apply rewrote a working name into a DHCP address that expires. A nil
// resolve degrades to the literal comparison.
func classifyScan(hosts []sshconf.Host, found []responder, resolve func(string) []string) []scanRow {
	byName := make(map[string]sshconf.Host, len(hosts))
	for _, h := range hosts {
		byName[h.Alias] = h
	}
	out := make([]scanRow, 0, len(found))
	for _, r := range found {
		row := scanRow{IP: r.IP, Hostname: r.Hostname}
		switch {
		case !r.Identified:
			row.Kind = scanUnidentified
		default:
			if h, ok := byName[r.Hostname]; ok {
				row.Alias = h.Alias
				if configPointsAt(h, r.IP, resolve) {
					row.Kind = scanCurrent
				} else {
					row.Kind, row.WasIP = scanMoved, h.HostName
				}
			} else {
				row.Alias, row.Kind = r.Hostname, scanNew
			}
		}
		out = append(out, row)
	}
	// Numerically, not lexically: string order puts .128 before .16 and .201
	// before .61, so a column of addresses reads as noise.
	sort.SliceStable(out, func(i, j int) bool { return lessAddr(out[i].IP, out[j].IP) })
	return out
}

// configPointsAt reports whether the block already directs ssh at ip, either
// literally or by a name that resolves there. An empty HostName means ssh falls
// back to the alias, so that is what gets resolved.
func configPointsAt(h sshconf.Host, ip string, resolve func(string) []string) bool {
	name := h.HostName
	if name == "" {
		name = h.Alias
	}
	if name == ip {
		return true
	}
	if resolve == nil {
		return false
	}
	for _, got := range resolve(name) {
		if got == ip {
			return true
		}
	}
	return false
}

// lessAddr orders two IPv4 strings by value. Unparseable input falls back to a
// string compare so ordering stays total rather than panicking.
func lessAddr(a, b string) bool {
	x, errX := netip.ParseAddr(a)
	y, errY := netip.ParseAddr(b)
	if errX != nil || errY != nil {
		return a < b
	}
	return x.Less(y)
}

// applyScan renders the config with moved hosts refreshed and new hosts
// adopted, and reports how many blocks it changed.
//
// An unidentified responder is NEVER written: we do not know what it is or
// which user it wants, and guessing would put a broken block in the file every
// command depends on. It is reported instead.
func applyScan(cfg string, rows []scanRow, marker string) (string, int, error) {
	out, changed := cfg, 0
	var err error
	for _, r := range rows {
		switch r.Kind {
		case scanMoved:
			// A refresh, not a re-add: Update preserves everything unmodelled
			// in the block, so a ProxyCommand survives a DHCP move.
			out, err = sshconf.Update(out, sshconf.Host{Alias: r.Alias, HostName: r.IP})
		case scanNew:
			out, err = sshconf.Add(out, sshconf.Host{Alias: r.Alias, HostName: r.IP}, marker)
		default:
			continue
		}
		if err != nil {
			return "", 0, fmt.Errorf("scan: %s %s: %w", r.Kind, r.Alias, err)
		}
		changed++
	}
	return out, changed, nil
}

// scanIdentity is one credential pair the fleet already uses. The sweep probes
// by ADDRESS, so ssh_config cannot supply either half: a Host block matches the
// alias, never the address behind it.
type scanIdentity struct{ User, Identity string }

// scanIdentities collects the distinct credentials of the fleet-marked hosts,
// so a probe offers exactly what the operator already trusts and nothing else.
//
// The zero identity is always tried LAST: it is what a bare `ssh <ip>` does, and
// it is the right answer when ssh_config carries a wildcard block or the agent
// already holds the key. Ordering is deterministic so a scan of an unchanged
// network reads the same on every run.
func scanIdentities(hosts []sshconf.Host) []scanIdentity {
	seen := map[scanIdentity]bool{}
	var out []scanIdentity
	for _, h := range hosts {
		if !h.Fleet {
			continue
		}
		id := scanIdentity{User: h.User, Identity: h.Identity}
		if id == (scanIdentity{}) || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].User != out[j].User {
			return out[i].User < out[j].User
		}
		return out[i].Identity < out[j].Identity
	})
	return append(out, scanIdentity{})
}

// identify asks each responder who it is, trying the fleet credentials in turn
// and stopping at the first that authenticates. Concurrent because a silent
// host costs a full connect timeout and a sweep finds many.
//
// A host that accepts none of them stays unidentified: that is a real and
// common state on a home LAN, and guessing would write a broken block into the
// file every other command depends on.
func identify(mk func(scanIdentity) runner.Runner, ids []scanIdentity, ips []string, workers int) []responder {
	if len(ids) == 0 {
		ids = []scanIdentity{{}}
	}
	out := make([]responder, len(ips))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, ip := range ips {
		wg.Add(1)
		go func(i int, ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = responder{IP: ip}
			for _, id := range ids {
				name, err := mk(id).Run(ip, "hostname")
				if err == nil && strings.TrimSpace(name) != "" {
					out[i].Hostname, out[i].Identified = strings.TrimSpace(name), true
					return
				}
			}
		}(i, ip)
	}
	wg.Wait()
	return out
}

// subnetDeps are the impure edges of subnet detection, injected so the decision
// itself is unit-tested without an interface or a subprocess.
type subnetDeps struct {
	localCIDRs func() ([]string, error) // this kernel's own IPv4 interfaces
	hostLAN    func() (string, error)   // the LAN of the machine hosting this kernel
	underWSL   func() bool
}

// detectCIDR picks the subnet to sweep.
//
// Under WSL the default route leaves on the Hyper-V NAT interface — 172.x.y.z/20
// here — which is a private segment shared with nothing. The fleet lives on the
// LAN of the WINDOWS host, and WSL holds no interface on it: it can route there
// but cannot enumerate it, so no amount of looking at local interfaces finds it.
// Asking Windows is the only way. A /20 is also over lanscan's sweep ceiling, so
// the old behaviour did not merely scan the wrong network, it refused to scan at
// all.
//
// The fallback still matters: with interop unavailable, or under WSL's mirrored
// networking mode where the local interface already IS the LAN, the local answer
// is the right one.
func detectCIDR(d subnetDeps) (string, error) {
	if d.underWSL() {
		if cidr, err := d.hostLAN(); err == nil && cidr != "" {
			return cidr, nil
		}
	}
	cidrs, err := d.localCIDRs()
	if err != nil {
		return "", err
	}
	if len(cidrs) == 0 {
		return "", fmt.Errorf("no usable IPv4 interface found")
	}
	return cidrs[0], nil
}

// realSubnetDeps wires detectCIDR to the machine it is running on.
func realSubnetDeps() subnetDeps {
	return subnetDeps{localCIDRs: localCIDRs, hostLAN: windowsHostLAN, underWSL: underWSL}
}

// localCIDRs reports the subnets of every up, non-loopback IPv4 interface, the
// default-route one first by virtue of interface order.
func localCIDRs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil || n.IP.IsLinkLocalUnicast() {
				continue
			}
			ones, _ := n.Mask.Size()
			out = append(out, fmt.Sprintf("%s/%d", n.IP.String(), ones))
		}
	}
	return out, nil
}

// underWSL reports whether this kernel is running under WSL. The environment
// variables are the cheap answer; osrelease is the one that survives a login
// shell that scrubbed them.
func underWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// windowsHostLAN asks Windows for the address and prefix of the adapter
// carrying ITS default route — the LAN the fleet is actually on.
//
// The lowest-metric default route is the one Windows would use, which is what
// makes this pick the Wi-Fi adapter over a Bluetooth PAN or the vEthernet
// switch that faces this VM.
func windowsHostLAN() (string, error) {
	const ps = `$i = Get-NetRoute -DestinationPrefix '0.0.0.0/0' | ` +
		`Sort-Object RouteMetric | Select-Object -First 1 -ExpandProperty ifIndex; ` +
		`Get-NetIPAddress -AddressFamily IPv4 -InterfaceIndex $i | ` +
		`ForEach-Object { "$($_.IPAddress)/$($_.PrefixLength)" }`

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil {
		return "", fmt.Errorf("asking Windows for its LAN: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		// Windows writes CRLF; a bare TrimSpace on the whole blob would still
		// leave the \r on every line but the last.
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, err := netip.ParsePrefix(line); err != nil {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("no IPv4 subnet reported by the Windows host")
}

func renderScan(rows []scanRow) string {
	var b strings.Builder
	for _, r := range rows {
		switch r.Kind {
		case scanMoved:
			fmt.Fprintf(&b, "  ~ %-20s %s → %s  (moved)\n", r.Alias, r.WasIP, r.IP)
		case scanNew:
			fmt.Fprintf(&b, "  + %-20s %s  (new)\n", r.Alias, r.IP)
		case scanCurrent:
			fmt.Fprintf(&b, "    %-20s %s  (current)\n", r.Alias, r.IP)
		case scanUnidentified:
			fmt.Fprintf(&b, "  ? %-20s %s  (answers :22, would not authenticate — not written)\n", "", r.IP)
		}
	}
	return b.String()
}

// lookupHost resolves a configured HostName to its addresses. A failure is not
// an error here: an unresolvable name simply cannot vouch for the address we
// found, which classifyScan reads as a move.
func lookupHost(name string) []string {
	addrs, err := net.LookupHost(name)
	if err != nil {
		return nil
	}
	return addrs
}

var (
	scanSubnet  string
	scanWorkers int
	scanTimeout time.Duration
)

func runScan(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	cidr := scanSubnet
	if cidr == "" {
		var err error
		if cidr, err = detectCIDR(realSubnetDeps()); err != nil {
			return err
		}
	}
	ips, err := lanscan.HostsIn(cidr)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "sweeping %s (%d addresses) for :22...\n", cidr, len(ips))

	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
	defer cancel()
	open := lanscan.Sweep(ctx, ips, 22, lanscan.TCPDial(scanTimeout), scanWorkers)
	fmt.Fprintf(out, "%d host(s) answered; identifying...\n\n", len(open))

	cfg, err := readConfig(flagConfig)
	if err != nil {
		return err
	}
	hosts, err := sshconf.Parse(cfg, flagMarker)
	if err != nil {
		return err
	}
	// Probing is by address, so the credentials must be handed over explicitly;
	// see scanIdentities. Resolution is what keeps a NAME-based config stable.
	mk := func(id scanIdentity) runner.Runner {
		e := runner.Exec{ConnectTimeout: "3", User: id.User}
		if id.Identity != "" {
			e.Identities = []string{id.Identity}
		}
		return e
	}
	found := identify(mk, scanIdentities(hosts), open, scanWorkers)
	rows := classifyScan(hosts, found, lookupHost)
	fmt.Fprint(out, renderScan(rows))

	next, changed, err := applyScan(cfg, rows, flagMarker)
	if err != nil {
		return err
	}
	if changed == 0 {
		fmt.Fprintln(out, "\nnothing to change — every identified host is already current")
		return nil
	}
	fmt.Fprintf(out, "\n%d block(s) would change\n", changed)
	if discoverDryRun {
		return applyConfig(out, flagConfig, next, true)
	}
	if !discoverYes && !askYesNo(cmd, "apply these changes?") {
		fmt.Fprintln(out, "nothing changed")
		return nil
	}
	if err := applyConfig(out, flagConfig, next, false); err != nil {
		return err
	}
	fmt.Fprintf(out, "updated %s (a timestamped backup was written alongside it)\n", flagConfig)
	return nil
}
