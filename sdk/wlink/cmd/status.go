package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/linkstate"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/probe"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/winhost"
	"github.com/spf13/cobra"
)

// tunnelStateFor decides which of the four tunnel conditions this machine is in.
//
// The distinction that earns its keep is not-ready vs down. Windows publishes a
// VPN adapter and its DNS server the moment you click connect — seconds before
// the handshake completes — and the previous network is already unroutable by
// then, so every candidate goes silent. Reporting that as "down" sends a user
// looking for a disconnected tunnel that is in fact seconds from working.
//
// A tunnel that is up but serves a DIFFERENT network is still up: the fleet
// shortfall is reported on its own line rather than blamed on the tunnel.
func tunnelStateFor(ifaces []winhost.Interface, scores []probe.Candidate) linkstate.TunnelState {
	if len(ifaces) == 0 {
		// We could not look. Claiming "down" would assert something unobserved.
		return linkstate.TunnelUnknown
	}
	hasTunnel := false
	for _, i := range ifaces {
		if i.IsTunnel {
			hasTunnel = true
			break
		}
	}
	if !hasTunnel {
		return linkstate.TunnelDown
	}
	if len(scores) == 0 {
		// A tunnel adapter exists but nothing was probed (no fleet names), so
		// there is no evidence either way. Reporting "up" would assert a state
		// nothing was observed for.
		return linkstate.TunnelUnknown
	}
	if probe.AllSilent(scores) {
		return linkstate.TunnelNotReady
	}
	return linkstate.TunnelUp
}

func tunnelIfaceName(ifaces []winhost.Interface) string {
	for _, i := range ifaces {
		if i.IsTunnel {
			return i.Alias
		}
	}
	return ""
}

// Status reports the whole picture.
//
// Exit code is the contract a script reads (spec §3, EC-20): 0 healthy,
// 1 degraded. It is derived from the same State a --json consumer receives, so
// the two can never disagree.
func (r *Runtime) Status(ctx context.Context) (int, error) {
	if !r.WSL {
		r.sayf("not running under WSL; nothing to report.")
		return 0, nil
	}

	ifaces, herr := r.Host.Interfaces(ctx)
	var scores []probe.Candidate
	if herr == nil && len(r.FleetHosts) > 0 {
		var servers []string
		for _, i := range ifaces {
			servers = append(servers, i.DNSServers...)
		}
		scores = probe.Score(ctx, r.Lookup, probe.ScoreInput{
			Servers:        probe.FilterCandidates(servers),
			Fleet:          r.FleetHosts,
			PublicSentinel: r.sentinel(),
		})
	}

	tunnel := linkstate.Tunnel{
		State:     tunnelStateFor(ifaces, scores),
		Interface: tunnelIfaceName(ifaces),
	}
	state := r.State(ctx, scores, tunnel)
	// fleet.resolved is what this machine can resolve RIGHT NOW — through the
	// resolver actually in force — not the best score any candidate could
	// achieve. Reporting the best candidate would call a machine healthy
	// because some resolver COULD serve it, which is precisely the state the
	// user is asking about when they run status.
	state.Fleet.Resolved = r.currentlyResolvable(ctx, state.Pinned, scores)
	state = state.Normalized()

	if r.JSON {
		enc := json.NewEncoder(r.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(state); err != nil {
			return 1, err
		}
		return exitForHealth(state.Link), nil
	}

	r.renderStatus(state, scores)
	return exitForHealth(state.Link), nil
}

// currentlyResolvable counts fleet names the resolver in force can answer.
func (r *Runtime) currentlyResolvable(ctx context.Context, pinned *linkstate.Pin, scores []probe.Candidate) int {
	if pinned == nil {
		return 0
	}
	for _, c := range scores {
		if c.Server == pinned.Resolver {
			return c.FleetResolved // already probed in the sweep
		}
	}
	// The resolver in force is not among Windows' per-interface servers — the
	// WSL NAT proxy is the usual case. Ask it directly rather than assuming.
	n := 0
	for _, name := range r.FleetHosts {
		if r.Lookup.LookupA(ctx, pinned.Resolver, name).HasAddress() {
			n++
		}
	}
	return n
}

func exitForHealth(h linkstate.Health) int {
	if h == linkstate.HealthOK {
		return 0
	}
	return 1
}

func (r *Runtime) renderStatus(s linkstate.State, scores []probe.Candidate) {
	w := r.Out
	fmt.Fprintf(w, "link:      %s\n", strings.ToUpper(string(s.Link)))

	tun := string(s.Tunnel.State)
	if s.Tunnel.Interface != "" {
		tun += "        " + s.Tunnel.Interface
	}
	fmt.Fprintf(w, "tunnel:    %s\n", tun)

	if s.Pinned != nil {
		managed := "pinned by wlink"
		if !s.Pinned.Managed {
			// Someone else's resolver sits in resolv.conf. wlink will neither
			// claim credit for it nor silently take it over.
			managed = "present, but NOT written by wlink"
		}
		fmt.Fprintf(w, "resolver:  %s   (%s)\n", s.Pinned.Resolver, managed)
	} else {
		fmt.Fprintf(w, "resolver:  unpinned\n")
	}

	fmt.Fprintf(w, "fleet:     %d/%d resolvable\n", s.Fleet.Resolved, s.Fleet.Total)
	if len(s.Fleet.ExcludedByHostsFile) > 0 {
		fmt.Fprintf(w, "excluded:  %s   (served by /etc/hosts — files precedes dns)\n",
			strings.Join(s.Fleet.ExcludedByHostsFile, ", "))
	}
	if s.Drift != nil {
		fmt.Fprintf(w, "drift:     %s — %s\n", s.Drift.File, s.Drift.Detail)
	}

	// A degraded machine with an obvious remedy should say what it is, rather
	// than leaving the reader to work it out from the candidate list.
	//
	// The condition is "a candidate can do better than the resolver in force" —
	// not "nothing is pinned". The interesting case is precisely a resolver that
	// IS present and simply does not know this fleet, which is what happens when
	// the default route's resolver wins by being first rather than by being
	// right.
	if s.Link != linkstate.HealthOK {
		if best, ok := probe.Winner(scores); ok && best.FleetResolved > s.Fleet.Resolved {
			fmt.Fprintf(w, "hint:      run `wlink pin` — %s answers for %d of %d\n",
				best.Server, best.FleetResolved, s.Fleet.Total)
		}
	}
}

func newStatusCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "status",
		Short: "Report whether this box can reach its fleet by name",
		Long: "Reports tunnel state, the pinned resolver, and fleet reachability.\n\n" +
			"Exit code: 0 healthy, 1 degraded, 2 usage error.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cc *cobra.Command, _ []string) error {
			rt := newRuntime(cc.Context())
			rt.JSON = asJSON
			code, err := rt.Status(cc.Context())
			if err != nil {
				return err
			}
			exitWith(code)
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit the machine-readable contract instead of the human report")
	return c
}
