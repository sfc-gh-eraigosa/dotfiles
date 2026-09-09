package probe

import (
	"context"
	"fmt"
	"net"
)

// Lookup is the one thing scoring needs from a resolver, so scoring can be
// tested against a table instead of a network.
type Lookup interface {
	LookupA(ctx context.Context, server, name string) Result
}

// Candidate is one resolver's score.
//
// Reachable and FleetResolved are separate on purpose. A resolver that answers
// but knows none of the fleet is the "wrong tunnel" diagnosis; one that says
// nothing at all is the "not ready" diagnosis. Merging them loses the ability
// to tell a user which of the two they are looking at.
type Candidate struct {
	Server        string
	Reachable     bool
	FleetResolved int
	// Recursive means the server answered for a public sentinel — the
	// precondition for pinning it first. See Guard.
	Recursive bool
}

// ScoreInput describes one probe sweep.
type ScoreInput struct {
	// Servers in enumeration order. Order is load-bearing: it breaks ties, so
	// an unchanged machine keeps picking the same resolver.
	Servers []string
	// Fleet names to probe. Names served by /etc/hosts must already have been
	// excluded — no resolver is ever asked for those.
	Fleet []string
	// PublicSentinel proves recursion. Empty disables the recursion check.
	PublicSentinel string
}

// Score probes every candidate for every fleet name plus the sentinel.
//
// Every candidate is scored even after a clear winner appears: `wlink status`
// reports the whole picture, and "the ISP resolver answered but knew none of
// your hosts" is exactly the line that tells a user why their default route is
// not the answer.
func Score(ctx context.Context, lk Lookup, in ScoreInput) []Candidate {
	out := make([]Candidate, 0, len(in.Servers))
	for _, server := range in.Servers {
		c := Candidate{Server: server}

		// Ask the sentinel FIRST, and stop on silence.
		//
		// A dead candidate would otherwise cost one full timeout per fleet name:
		// on a machine with a stale Bluetooth or disconnected adapter, that is
		// (names+1) x timeout of pure waiting on every run — measured at 8s on a
		// three-host fleet, which made `wait --ready` take 11s to report a link
		// that was already up.
		//
		// Only SILENCE short-circuits. An NXDOMAIN or SERVFAIL proves the server
		// is there and talking, so its fleet names are still worth asking about;
		// a server that answers some names and is silent for others would be
		// pathological.
		if in.PublicSentinel != "" {
			r := lk.LookupA(ctx, server, in.PublicSentinel)
			c.Recursive = r.HasAddress()
			if !r.Reachable() {
				out = append(out, c) // silent: nothing more to learn
				continue
			}
			c.Reachable = true
		}

		for _, name := range in.Fleet {
			r := lk.LookupA(ctx, server, name)
			if r.Reachable() {
				c.Reachable = true
			}
			if r.HasAddress() {
				c.FleetResolved++
			}
		}
		out = append(out, c)
	}
	return out
}

// Winner returns the candidate resolving the most fleet names.
//
// Ties go to the first enumerated, so the choice is stable across runs on an
// unchanged machine — a pinned resolver that changed run to run would be
// baffling. Reports false when nothing resolved anything, which the caller must
// distinguish from a zero-valued candidate.
func Winner(scores []Candidate) (Candidate, bool) {
	best, found := Candidate{}, false
	for _, c := range scores {
		if c.FleetResolved > best.FleetResolved { // strictly greater: first wins ties
			best, found = c, true
		}
	}
	return best, found
}

// AllSilent reports whether no candidate responded at all.
//
// This is the signature of a tunnel attached but NOT READY: Windows publishes a
// VPN adapter and its DNS server the moment you click connect, seconds before
// the handshake completes, and the previous network is already unroutable by
// then. An empty list is not "all silent" — there was nothing to ask.
func AllSilent(scores []Candidate) bool {
	if len(scores) == 0 {
		return false
	}
	for _, c := range scores {
		if c.Reachable {
			return false
		}
	}
	return true
}

// FilterCandidates drops addresses that can never be a useful LAN/VPN resolver
// and collapses duplicates, preserving first-seen order (which Winner relies on
// for stable tie-breaking).
func FilterCandidates(servers []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		ip := net.ParseIP(s)
		switch {
		case ip == nil, // not an address at all
			ip.To4() == nil,         // IPv6: wlink pins IPv4 resolvers
			ip.IsLoopback(),         // 127.0.0.0/8 — the local stub, not a LAN resolver
			ip.IsUnspecified(),      // 0.0.0.0
			ip.IsLinkLocalUnicast(): // 169.254.0.0/16 — an interface with no DHCP
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Verdict is the recursion guard's decision.
type Verdict struct {
	OK bool
	// Overridden marks a pin permitted only because the caller insisted, so it
	// can be warned about loudly rather than passing silently.
	Overridden bool
	Reason     string
}

// Guard decides whether a candidate is safe to pin FIRST.
//
// This is the rule that protects public DNS, and it follows from how glibc
// actually works: /etc/resolv.conf is an ORDERED LIST, not a routing table.
// Nameserver #1 is asked for every name, and its NXDOMAIN is a FINAL answer —
// glibc falls through to the next nameserver only on timeout, never on a valid
// negative reply. So a resolver that serves only local names would answer
// NXDOMAIN for github.com, and the fallbacks would never be consulted: public
// DNS would be dead with no indication why.
//
// allowNonRecursive exists for split-horizon setups that genuinely want this,
// but the verdict is marked Overridden so the caller can say plainly what is
// being traded away.
func Guard(c Candidate, allowNonRecursive bool) Verdict {
	if c.Recursive {
		return Verdict{OK: true, Reason: fmt.Sprintf("%s also resolves public names; safe to pin first", c.Server)}
	}
	reason := fmt.Sprintf(
		"%s resolves fleet names but not public ones; pinning it first would break ALL public DNS, "+
			"because every query goes to nameserver #1 and its NXDOMAIN is final — the fallbacks are only tried on timeout",
		c.Server)
	if allowNonRecursive {
		return Verdict{OK: true, Overridden: true, Reason: reason}
	}
	return Verdict{OK: false, Reason: reason}
}
