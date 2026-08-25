package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/linkstate"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/probe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/resolvconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/winhost"
)

// InterfaceLister is the slice of winhost the commands need — narrowed to an
// interface so tests supply recorded data instead of a Windows host.
type InterfaceLister interface {
	Interfaces(ctx context.Context) ([]winhost.Interface, error)
}

// Runtime holds everything a command needs. Every dependency is injectable,
// which is what lets the full pin/unpin path be tested against a temp directory
// with no Windows and no privileged write.
type Runtime struct {
	WSL    bool
	Host   InterfaceLister
	Lookup probe.Lookup

	// FleetHosts are the names to probe; ExcludedHosts are fleet names
	// deliberately not probed (served by /etc/hosts, or already addresses).
	// Excluded names are reported so a score below the host count is never a
	// mystery.
	FleetHosts    []string
	ExcludedHosts []string

	// ExtraFallbacks are appended after the resolvers already in resolv.conf.
	// Empty by default: wlink does not silently route a user's DNS to a
	// third-party resolver, and on WSL the NAT proxy is effectively always
	// present anyway (EC-22).
	ExtraFallbacks []string

	PublicSentinel    string
	AllowNonRecursive bool
	DryRun            bool
	JSON              bool

	// System resolves through the host path (nsswitch + resolv.conf ordering),
	// which is the only thing that proves what ssh will experience. Nil means
	// the real one.
	System SystemResolver
	// MaxFailSeconds overrides the budget derived from resolv.conf; 0 derives.
	MaxFailSeconds int

	Paths resolvconf.Paths
	Out   io.Writer
}

func (r *Runtime) sayf(format string, a ...any) {
	if r.Out != nil {
		fmt.Fprintf(r.Out, "wlink: "+format+"\n", a...)
	}
}

func (r *Runtime) sentinel() string {
	if r.PublicSentinel == "" {
		return "github.com"
	}
	return r.PublicSentinel
}

// candidates gathers every per-interface resolver Windows knows, in enumeration
// order, filtered to plausible ones.
//
// Order is preserved because it breaks scoring ties, so an unchanged machine
// keeps choosing the same resolver.
func (r *Runtime) candidates(ctx context.Context) ([]string, error) {
	ifaces, err := r.Host.Interfaces(ctx)
	if err != nil {
		return nil, err
	}
	var servers []string
	for _, i := range ifaces {
		servers = append(servers, i.DNSServers...)
	}
	return probe.FilterCandidates(servers), nil
}

// Pin selects and pins the resolver that knows the fleet.
//
// Every early return here is a SAFE DECLINE with exit 0, not a failure: not
// WSL, Windows unreachable, no fleet names, no candidates, no winner, the guard
// tripped, or no undo path available. install.sh must never fail because a
// tunnel happened to be down when it ran.
func (r *Runtime) Pin(ctx context.Context) (int, error) {
	if !r.WSL {
		r.sayf("not running under WSL; nothing to do.")
		return 0, nil
	}
	if len(r.ExcludedHosts) > 0 {
		r.sayf("not probing %v — already served by /etc/hosts or already an address (files precedes dns).", r.ExcludedHosts)
	}
	if len(r.FleetHosts) == 0 {
		r.sayf("no fleet hosts to probe; nothing to do.")
		return 0, nil
	}

	servers, err := r.candidates(ctx)
	if err != nil {
		r.sayf("WARNING — cannot reach Windows to enumerate resolvers (%v); declining.", err)
		return 0, nil
	}
	if len(servers) == 0 {
		r.sayf("WARNING — Windows reported no usable resolvers; declining.")
		return 0, nil
	}

	scores := probe.Score(ctx, r.Lookup, probe.ScoreInput{
		Servers:        servers,
		Fleet:          r.FleetHosts,
		PublicSentinel: r.sentinel(),
	})
	for _, c := range scores {
		switch {
		case c.FleetResolved > 0:
			r.sayf("candidate %s: resolved %d/%d fleet host(s).", c.Server, c.FleetResolved, len(r.FleetHosts))
		case c.Reachable:
			r.sayf("candidate %s: reachable, resolved 0/%d fleet host(s).", c.Server, len(r.FleetHosts))
		default:
			// Neutral wording on purpose: with a full-tunnel VPN up, the ISP's
			// resolvers go silent BECAUSE the tunnel is up. Calling that "tunnel
			// down" would be exactly backwards.
			r.sayf("candidate %s: NO RESPONSE — not reachable from this network.", c.Server)
		}
	}

	winner, ok := probe.Winner(scores)
	if !ok {
		r.declineNoWinner(scores)
		return 0, nil
	}
	r.sayf("selected %s (%d/%d fleet hosts).", winner.Server, winner.FleetResolved, len(r.FleetHosts))

	verdict := probe.Guard(winner, r.AllowNonRecursive)
	switch {
	case !verdict.OK:
		r.sayf("WARNING — %s", verdict.Reason)
		r.sayf("Refusing to change DNS. Override with --allow-nonrecursive if you know better.")
		return 0, nil
	case verdict.Overridden:
		r.sayf("WARNING — %s", verdict.Reason)
		r.sayf("WARNING — pinning anyway because --allow-nonrecursive was given.")
	default:
		r.sayf("%s", verdict.Reason)
	}

	fallbacks := append(resolvconf.Nameservers(readFileOrEmpty(r.Paths.ResolvConf)), r.ExtraFallbacks...)
	content := resolvconf.RenderResolvConf(resolvconf.Render{
		Winner:    winner.Server,
		Fallbacks: fallbacks,
	})

	if r.DryRun {
		r.sayf("--dry-run; would write:")
		fmt.Fprintf(r.Out, "--- %s ---\n%s", r.Paths.ResolvConf, content)
		return 0, nil
	}

	// Snapshot separately from the write so the two failures can be told apart:
	// no undo path is a safe decline (exit 0), a failed write is a real failure
	// (exit 1).
	if err := resolvconf.TakeSnapshot(r.Paths); err != nil {
		r.sayf("WARNING — cannot record an undo point (%v); refusing to change DNS without one.", err)
		return 0, nil
	}
	if err := resolvconf.Apply(r.Paths, content); err != nil {
		return 1, err
	}

	r.sayf("pinned %s in %s; %s keeps WSL from overwriting it.", winner.Server, r.Paths.ResolvConf, r.Paths.WslConf)
	r.sayf("undo any time: wlink unpin")
	return 0, nil
}

// declineNoWinner explains WHICH failure this is.
//
// All candidates silent means a tunnel is attached but NOT READY — Windows
// publishes a VPN adapter and its DNS server the moment you click connect,
// seconds before the handshake completes, and the previous network is already
// unroutable by then. If something answered, the network plainly works and the
// problem is a different one.
func (r *Runtime) declineNoWinner(scores []probe.Candidate) {
	r.sayf("WARNING — no candidate resolver answered for any of: %v", r.FleetHosts)
	if probe.AllSilent(scores) {
		r.sayf("WARNING — NOT ONE of the %d configured resolvers responded, even for a public name.", len(scores))
		r.sayf("WARNING — TUNNEL LIKELY NOT READY: a VPN adapter and its DNS server appear the moment")
		r.sayf("WARNING — you click connect, seconds before the handshake finishes, and the old network")
		r.sayf("WARNING — is already unroutable by then. Wait for it to finish connecting, then re-run.")
		return
	}
	r.sayf("WARNING — some resolvers answered but none knows these names.")
	r.sayf("WARNING — Is the tunnel that serves these names the one that is up?")
}

// Unpin restores the pre-pin state.
func (r *Runtime) Unpin(ctx context.Context) (int, error) {
	if !r.WSL {
		r.sayf("not running under WSL; nothing to do.")
		return 0, nil
	}
	report, err := resolvconf.Restore(r.Paths)
	if err != nil {
		return 1, err
	}
	r.sayf("%s", report.Detail)
	if report.Repaired {
		r.sayf("run `wsl.exe --shutdown` from Windows for a fully clean slate.")
	}
	return 0, nil
}

// State composes the current picture for status/verify.
func (r *Runtime) State(ctx context.Context, scores []probe.Candidate, tunnel linkstate.Tunnel) linkstate.State {
	s := linkstate.State{WSL: r.WSL, Tunnel: tunnel}
	for _, c := range scores {
		s.Candidates = append(s.Candidates, linkstate.Candidate{
			Server:        c.Server,
			Reachable:     c.Reachable,
			FleetResolved: c.FleetResolved,
			Recursive:     c.Recursive,
		})
	}
	s.Fleet = linkstate.Fleet{Total: len(r.FleetHosts), ExcludedByHostsFile: r.ExcludedHosts}
	if content := readFileOrEmpty(r.Paths.ResolvConf); content != "" {
		if ns := resolvconf.Nameservers(content); len(ns) > 0 {
			s.Pinned = &linkstate.Pin{Resolver: ns[0], Managed: resolvconf.IsManaged(content)}
		}
	}
	if d, err := resolvconf.DetectDrift(r.Paths); err == nil && d != nil {
		s.Drift = &linkstate.Drift{File: d.File, Detail: d.Detail}
	}
	return s.Normalized()
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
