package cmd

import (
	"context"
	"fmt"
	"net"
	"net/netip"
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
func classifyScan(hosts []sshconf.Host, found []responder) []scanRow {
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
				if h.HostName == r.IP {
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

// identify asks each responder who it is, over the fleet key. Concurrent
// because a silent host costs a full connect timeout and a sweep finds many.
func identify(r runner.Runner, ips []string, workers int) []responder {
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
			if name, err := r.Run(ip, "hostname"); err == nil && strings.TrimSpace(name) != "" {
				out[i].Hostname, out[i].Identified = strings.TrimSpace(name), true
			}
		}(i, ip)
	}
	wg.Wait()
	return out
}

// localCIDR reports the subnet of the interface carrying the default route.
func localCIDR() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
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
			return fmt.Sprintf("%s/%d", n.IP.String(), ones), nil
		}
	}
	return "", fmt.Errorf("no usable IPv4 interface found")
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
		if cidr, err = localCIDR(); err != nil {
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
	rows := classifyScan(hosts, identify(runner.Exec{ConnectTimeout: "3"}, open, scanWorkers))
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
