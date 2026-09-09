// Package lanscan sweeps a local subnet for hosts listening on a TCP port.
//
// It exists so `fleet discover --scan` needs NO external prerequisite. The
// shell `ssh-find` it replaces shells out to nmap, and on a machine without
// nmap installed that script exits before scanning anything — a discovery tool
// that silently cannot discover. Go's standard dialer does the same job with
// nothing to install.
//
// Everything impure is injected: the dialer is a parameter, so the whole
// package is tested without opening a socket.
package lanscan

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"sync"
	"time"
)

// DialFunc reports whether addr accepted a connection. A nil error means open.
type DialFunc func(ctx context.Context, addr string) error

// maxHosts bounds a sweep. A /16 is 65k dials: refusing is far better than
// appearing to hang, and a fleet lives on a /24 in practice.
const maxHosts = 1 << 10 // a /22

// HostsIn enumerates the usable addresses of the subnet containing cidr,
// excluding the network and broadcast addresses — neither is a machine.
func HostsIn(cidr string) ([]string, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("lanscan: %w", err)
	}
	p = p.Masked()
	if !p.Addr().Is4() {
		return nil, fmt.Errorf("lanscan: only IPv4 subnets are swept, got %s", cidr)
	}
	total := 1 << (32 - p.Bits())
	if total > maxHosts {
		return nil, fmt.Errorf("lanscan: %s covers %d addresses; refusing to sweep more than %d",
			cidr, total, maxHosts)
	}
	var out []string
	for a := p.Addr().Next(); p.Contains(a); a = a.Next() {
		next := a.Next()
		if !p.Contains(next) {
			break // the broadcast address
		}
		out = append(out, a.String())
	}
	return out, nil
}

// Sweep dials port on every address concurrently and returns those that
// answered, ORDERED BY ADDRESS.
//
// Order is not cosmetic: results collected in completion order would render
// the same network differently on every run, which makes a diff of "what is on
// my network" useless. workers bounds concurrency so a sweep does not open a
// thousand sockets at once.
func Sweep(ctx context.Context, ips []string, port int, dial DialFunc, workers int) []string {
	if workers < 1 {
		workers = 1
	}
	open := make([]bool, len(ips))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, ip := range ips {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(i int, ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			if err := dial(ctx, net.JoinHostPort(ip, fmt.Sprint(port))); err == nil {
				open[i] = true
			}
		}(i, ip)
	}
	wg.Wait()

	var out []string
	for i, ok := range open {
		if ok {
			out = append(out, ips[i])
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		x, errX := netip.ParseAddr(out[a])
		y, errY := netip.ParseAddr(out[b])
		if errX != nil || errY != nil {
			return out[a] < out[b]
		}
		return x.Less(y)
	})
	return out
}

// TCPDial is the real dialer. The timeout is per address and deliberately
// short: a sweep waits on hundreds of silent addresses, and the ones that
// matter answer immediately on a LAN.
func TCPDial(timeout time.Duration) DialFunc {
	return func(ctx context.Context, addr string) error {
		d := net.Dialer{Timeout: timeout}
		c, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return err
		}
		return c.Close()
	}
}
