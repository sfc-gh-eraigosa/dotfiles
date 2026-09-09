// Package reach rescues hosts that are asleep rather than dead.
//
// A Wi-Fi power-saving host (a Raspberry Pi with `power_save on` is the
// motivating case) sleeps through the BROADCAST ARP requests that a cold
// neighbour cache must send. The workstation therefore fails to resolve the
// host's MAC and gives up at layer 2 — before a single SSH packet is sent —
// and fleet reports a live machine as `unreachable`. A peer with a warm cache
// is unaffected, because Linux refreshes a warm entry with a UNICAST probe,
// which the sleeper does wake for.
//
// Wake escalates a bounded ladder to give such a host a chance to answer. Two
// rules shape everything here:
//
//   - The workstation may have NO layer-2 presence on the fleet's subnet.
//     Under WSL2's NAT mode the ARP table that matters belongs to Windows, so
//     "just send an ARP request" is a no-op in exactly the environment that
//     exhibits the bug. The peer relay exists for that case.
//   - Only a DIRECT re-probe may set Woke. A successful relay proves the PEER
//     can route to the target, which is strictly weaker; treating it as
//     success would turn a real partition into a green row.
//
// Everything impure is injected through Deps, so the escalation policy is
// unit-tested without opening a socket or spending real time — the same
// discipline the rest of fleet follows.
package reach

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// Rung names, also used as Result.Via when a non-relay rung succeeds.
const (
	RungRetry      = "retry"
	RungLocalPrime = "local-prime"
	RungPeerRelay  = "peer-relay"
)

// DefaultBudget bounds the whole ladder for one target, sized from measured
// costs rather than guessed: a relay round-trip is ~1.1s and a probe to a host
// that is not answering costs a full SSH connect timeout (~4s, see
// wakeProbeTimeout). One retry probe plus a full relay is therefore ~6s, and
// 12s leaves headroom without making a genuinely dead host feel like a hang.
// Every sleeping host in a fleet pays this concurrently, not serially.
const DefaultBudget = 12 * time.Second

// settle gives the nudged host a moment to emit its ARP traffic and the local
// stack a moment to record it, before the direct re-probe decides the verdict.
const settle = 400 * time.Millisecond

// Peer is a fleet host considered as a relay candidate. Reachable is what the
// caller already learned in this run — the concurrent probe has usually
// answered for several hosts by the time a straggler fails.
type Peer struct {
	Alias, HostName string
	Reachable       bool
}

// Policy bounds the ladder. A zero Budget falls back to DefaultBudget.
type Policy struct {
	Enabled bool
	Budget  time.Duration
	Retries int
}

// Attempt records one rung, including the ones that were skipped: an operator
// diagnosing a stubborn host needs to see what was NOT tried as much as what
// was.
type Attempt struct {
	Rung string `json:"rung"`
	Via  string `json:"via,omitempty"`
	OK   bool   `json:"ok"`
	Err  string `json:"err,omitempty"`
}

// Result is the ladder's verdict. Via names the rung — or, for a relay, the
// peer alias — that earned it, and becomes the row's "woke via …" note.
type Result struct {
	Woke     bool      `json:"woke"`
	Via      string    `json:"via,omitempty"`
	Attempts []Attempt `json:"attempts"`
}

// Deps is every impure edge the ladder touches.
type Deps struct {
	// Probe answers "is the target reachable RIGHT NOW, directly?" — the only
	// thing allowed to set Result.Woke.
	Probe      func(alias string) error
	Runner     runner.Runner
	Resolve    func(host string) ([]net.IP, error)
	LocalAddrs func() ([]net.IPNet, error)
	PingLocal  func(ctx context.Context, ip string) error
	Sleep      func(time.Duration)
}

// Wake runs the ladder and returns what it achieved. It never mutates the
// target: the only things sent are ICMP, a read of $SSH_CONNECTION, and the
// probe itself.
func Wake(ctx context.Context, target Peer, peers []Peer, p Policy, d Deps) Result {
	var res Result
	if !p.Enabled {
		return res
	}

	if p.Budget <= 0 {
		p.Budget = DefaultBudget
	}
	ctx, cancel := context.WithTimeout(ctx, p.Budget)
	defer cancel()

	for _, rung := range []func(context.Context, Peer, []Peer, Policy, Deps) (Attempt, bool){
		rungRetry, rungLocalPrime, rungPeerRelay,
	} {
		if ctx.Err() != nil {
			return res
		}
		a, woke := rung(ctx, target, peers, p, d)
		res.Attempts = append(res.Attempts, a)
		if woke {
			res.Woke = true
			res.Via = a.Via
			return res
		}
	}
	return res
}

// rungRetry re-probes after a short backoff. It costs only time already
// budgeted and catches the common case: a neighbour entry that expired
// moments ago and needs one more broadcast to land.
func rungRetry(ctx context.Context, target Peer, _ []Peer, p Policy, d Deps) (Attempt, bool) {
	a := Attempt{Rung: RungRetry, Via: RungRetry}

	// Reserve most of the budget for the rungs that actually fix things.
	// Found by live testing: each probe is an SSH to a host that is not
	// answering, so it costs a full connect timeout. Left unchecked, retrying
	// consumed the ENTIRE budget and peer-relay — the only rung that resolves
	// the motivating failure — never ran at all.
	reserve := p.Budget - p.Budget/retryShare

	for i := 0; i < p.Retries; i++ {
		d.Sleep(backoff(i))
		if ctx.Err() != nil {
			a.Err = "deadline exceeded"
			return a, false
		}
		if remaining(ctx) < reserve {
			a.Err = fmt.Sprintf("stopped after %d retries to leave budget for the relay", i)
			return a, false
		}
		if err := d.Probe(target.Alias); err == nil {
			a.OK = true
			return a, true
		}
	}
	a.Err = fmt.Sprintf("still unreachable after %d retries", p.Retries)
	return a, false
}

// retryShare is the fraction of the budget the cheap retry rung may spend.
const retryShare = 3

// remaining reports how much of the ladder's budget is left. A context with no
// deadline never limits a rung.
func remaining(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 1<<63 - 1
	}
	return time.Until(deadline)
}

// rungLocalPrime pings the target from the workstation, forcing the local
// stack to ARP for it. This only means anything when the workstation is
// genuinely on the target's subnet — i.e. when fleet runs on a fleet member
// rather than behind WSL2's NAT.
func rungLocalPrime(ctx context.Context, target Peer, _ []Peer, _ Policy, d Deps) (Attempt, bool) {
	a := Attempt{Rung: RungLocalPrime, Via: RungLocalPrime}

	ip, ok := localSubnetTarget(target, d)
	if !ok {
		a.Err = "skipped: workstation is not on the target's subnet"
		return a, false
	}
	if err := d.PingLocal(ctx, ip); err != nil {
		a.Err = err.Error()
	}
	d.Sleep(settle)
	if ctx.Err() != nil {
		a.Err = "deadline exceeded"
		return a, false
	}
	if err := d.Probe(target.Alias); err == nil {
		a.OK = true
		return a, true
	}
	if a.Err == "" {
		a.Err = "primed but still unreachable"
	}
	return a, false
}

// rungPeerRelay is the rung that fixes the motivating incident, and the only
// one that works when the workstation is NAT'd off the fleet's subnet.
//
// It asks a reachable peer what address our SSH connection appears to come
// from — which sees straight through the NAT — then reaches the target THROUGH
// that peer and tells it to send traffic to us. The target's own ARP request
// is what populates the workstation's cache; the ping's replies are irrelevant
// and never waited for.
func rungPeerRelay(ctx context.Context, target Peer, peers []Peer, _ Policy, d Deps) (Attempt, bool) {
	a := Attempt{Rung: RungPeerRelay}

	ranked := rankPeers(peers, target)
	if len(ranked) == 0 {
		a.Err = "skipped: no peer available to relay through"
		return a, false
	}

	for _, peer := range ranked {
		if ctx.Err() != nil {
			a.Err = "deadline exceeded"
			return a, false
		}
		a.Via = peer.Alias

		ws, err := workstationIP(d.Runner, peer.Alias)
		if err != nil {
			a.Err = fmt.Sprintf("peer %s: %v", peer.Alias, err)
			continue
		}
		if _, err := d.Runner.RunVia(peer.Alias, target.Alias, nudgeCmd(ws)); err != nil {
			a.Err = fmt.Sprintf("relay via %s: %v", peer.Alias, err)
			continue
		}

		d.Sleep(settle)
		if ctx.Err() != nil {
			a.Err = "deadline exceeded"
			return a, false
		}
		// The verdict is the DIRECT probe, never the relay's success.
		if err := d.Probe(target.Alias); err == nil {
			a.OK = true
			return a, true
		}
		a.Err = fmt.Sprintf("nudged via %s but still unreachable directly", peer.Alias)
	}
	return a, false
}

// nudgeCmd tells the target to transmit toward the workstation.
//
// It is detached on purpose. Measured on real hardware, a blocking
// `ping -c 2` with no replies cost 11.0s — more than the entire default
// budget — because GNU ping waits out the final reply. Those replies never
// arrive: a Windows workstation drops inbound ICMP by default. Only the ARP
// request matters, and it leaves in the first milliseconds, so the command
// returns immediately (measured ~1.1s end to end including two SSH hops).
//
// `-W`/`-w` are banned: `-W` is seconds on GNU and milliseconds on BSD, a
// silent 1000x difference across the platforms fleet targets.
func nudgeCmd(wsIP string) string {
	return fmt.Sprintf("(ping -c 2 -n %s >/dev/null 2>&1 &) >/dev/null 2>&1; exit 0", wsIP)
}

// workstationIP asks a peer what address our connection comes from. This is
// the one reliable way to learn the workstation's address AS SEEN FROM the
// fleet's subnet: under WSL2 NAT the local interfaces show a private 172.x
// link, while the peer sees the Windows host's real LAN address.
func workstationIP(r runner.Runner, peer string) (string, error) {
	out, err := r.Run(peer, "echo $SSH_CONNECTION")
	if err != nil {
		return "", err
	}
	// "<client-ip> <client-port> <server-ip> <server-port>"
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("empty $SSH_CONNECTION")
	}
	if net.ParseIP(fields[0]) == nil {
		return "", fmt.Errorf("unparseable client address %q in $SSH_CONNECTION", fields[0])
	}
	return fields[0], nil
}

// rankPeers orders relay candidates: peers already known to answer come first
// (an asleep peer must never turn one slow host into two), then peers sharing
// the target's subnet. The target itself is never a candidate.
//
// Subnet affinity is a /24 heuristic on the configured HostName. It only
// affects ORDERING — any peer that can reach the target relays correctly — so
// a wrong guess costs a little time, never correctness.
func rankPeers(peers []Peer, target Peer) []Peer {
	var reachableNear, reachableFar, rest []Peer
	for _, p := range peers {
		if p.Alias == target.Alias {
			continue
		}
		switch {
		case p.Reachable && sameSubnet(p.HostName, target.HostName):
			reachableNear = append(reachableNear, p)
		case p.Reachable:
			reachableFar = append(reachableFar, p)
		default:
			rest = append(rest, p)
		}
	}
	return append(append(reachableNear, reachableFar...), rest...)
}

// sameSubnet is a /24 comparison over literal IPv4 host names. Names that are
// not literal addresses simply do not match, which downgrades them one rank.
func sameSubnet(a, b string) bool {
	ipA, ipB := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ipA == nil || ipB == nil {
		return false
	}
	return ipA.Mask(net.CIDRMask(24, 32)).Equal(ipB.Mask(net.CIDRMask(24, 32)))
}

// localSubnetTarget resolves the target and reports its address if any local
// interface shares a subnet with it.
func localSubnetTarget(target Peer, d Deps) (string, bool) {
	name := target.HostName
	if name == "" {
		name = target.Alias
	}
	ips, err := d.Resolve(name)
	if err != nil || len(ips) == 0 {
		return "", false
	}
	locals, err := d.LocalAddrs()
	if err != nil {
		return "", false
	}
	for _, ip := range ips {
		for _, n := range locals {
			if n.Contains(ip) {
				return ip.String(), true
			}
		}
	}
	return "", false
}

// backoff spaces the retry rung: immediate, then a short pause. Long enough
// for a neighbour resolution to complete, short enough to leave budget for the
// rungs that actually fix things.
func backoff(i int) time.Duration {
	if i == 0 {
		return 0
	}
	return time.Duration(i) * 750 * time.Millisecond
}
