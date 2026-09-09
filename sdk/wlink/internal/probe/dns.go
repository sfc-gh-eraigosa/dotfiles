// Package probe asks a specific DNS server a specific question and classifies
// what came back.
//
// It resolves natively rather than shelling out to dig. That is not tidiness:
// dig comes from a package (dnsutils) that never installs on a nix-managed
// host, where the shell prototype silently no-opped as a result. Resolving in
// process removes the dependency entirely.
//
// The classification is the point. Four outcomes must stay distinct, because
// two different decisions read them:
//
//   - the RECURSION GUARD needs "answered, but gave no address" — a resolver
//     that NXDOMAINs a public name would, if pinned first, take out public DNS
//     entirely, since resolv.conf is an ordered list and an NXDOMAIN from
//     nameserver #1 is a FINAL answer that never falls through.
//   - the READINESS DIAGNOSIS needs "said nothing at all" — when no candidate
//     responds even for a public name, a tunnel is attached but still
//     handshaking, which is a different situation from a wrong resolver.
//
// Collapsing any two of the four breaks one of those.
package probe

import (
	"context"
	"errors"
	"net"
	"time"
)

// Outcome is what a single lookup produced.
type Outcome string

const (
	// Resolved: the server returned at least one address.
	Resolved Outcome = "resolved"
	// NoAddress: the server answered, but with no address — NXDOMAIN (the name
	// does not exist) or NODATA (it exists with no A record). Both prove the
	// server is reachable, and both mean "do not pin this for that name".
	NoAddress Outcome = "no-address"
	// Unhelpful: the server answered with a failure (SERVFAIL, REFUSED). Still
	// reachable — reporting it as silent would misdiagnose a working tunnel as
	// not-ready.
	Unhelpful Outcome = "unhelpful"
	// Silent: nothing came back — timeout, network unreachable, cancelled.
	Silent Outcome = "silent"
)

// DefaultTimeout bounds one lookup. Short on purpose: a probe sweep asks
// several servers several names, and a dead candidate must not stall an
// install.
const DefaultTimeout = 2 * time.Second

// Result is one lookup's outcome.
type Result struct {
	Outcome Outcome
	Addrs   []string
	// Err is the underlying error, kept for diagnostics. Outcome is what
	// callers branch on.
	Err error
}

// Reachable reports whether the server responded at all — the signal that
// separates "wrong resolver" from "tunnel not ready".
func (r Result) Reachable() bool { return r.Outcome != Silent }

// HasAddress reports whether an address came back. This is what the recursion
// guard checks: anything else means the resolver cannot serve that name.
func (r Result) HasAddress() bool { return r.Outcome == Resolved }

// Resolver probes specific DNS servers.
type Resolver struct {
	// Timeout per lookup; zero means DefaultTimeout.
	Timeout time.Duration
	// Port to append when a server is given without one; zero means "53".
	Port string
}

func (p *Resolver) timeout() time.Duration {
	if p.Timeout == 0 {
		return DefaultTimeout
	}
	return p.Timeout
}

func (p *Resolver) port() string {
	if p.Port == "" {
		return "53"
	}
	return p.Port
}

// LookupA asks one server for one name's IPv4 addresses.
//
// PreferGo forces Go's own resolver so the query actually goes to the server
// named here. Without it, cgo's resolver would consult /etc/resolv.conf and
// silently answer from the system's configuration — which is precisely what
// wlink is trying to evaluate, so the probe would be measuring the wrong thing.
func (p *Resolver) LookupA(ctx context.Context, server, name string) Result {
	addr := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		addr = net.JoinHostPort(server, p.port())
	}

	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: p.timeout()}).DialContext(ctx, network, addr)
		},
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()

	addrs, err := r.LookupIP(ctx, "ip4", name)
	if err == nil && len(addrs) > 0 {
		out := make([]string, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, a.String())
		}
		return Result{Outcome: Resolved, Addrs: out}
	}
	if err == nil {
		// No error and no addresses: NODATA.
		return Result{Outcome: NoAddress}
	}

	// Cancellation/deadline means nothing came back — never "the server
	// answered". Checked before the DNSError switch because a cancelled dial
	// surfaces as a generic DNSError that would otherwise read as Unhelpful,
	// making a status-line timeout look like a reachable resolver.
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Result{Outcome: Silent, Err: err}
	}

	if dnsErr, ok := err.(*net.DNSError); ok {
		switch {
		case dnsErr.IsTimeout:
			return Result{Outcome: Silent, Err: err}
		case dnsErr.IsNotFound:
			// NXDOMAIN, and NODATA as reported by the Go resolver.
			return Result{Outcome: NoAddress, Err: err}
		default:
			// SERVFAIL/REFUSED surface as "server misbehaving": the server
			// spoke, so it is reachable.
			return Result{Outcome: Unhelpful, Err: err}
		}
	}
	// Context cancelled, or a non-DNS transport failure: nothing came back.
	return Result{Outcome: Silent, Err: err}
}
