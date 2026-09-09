package cmd

import (
	"context"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/probe"
	"github.com/spf13/cobra"
)

// DefaultWaitTimeout bounds `wait --ready`. A WireGuard handshake normally
// completes in a couple of seconds; a minute is generous without being a hang.
const DefaultWaitTimeout = 60 * time.Second

// DefaultPollInterval is how often readiness is re-checked.
const DefaultPollInterval = 2 * time.Second

// Wait blocks until this box can resolve a fleet name, or the timeout expires.
//
// It exists for a race that is easy to hit and confusing to diagnose: Windows
// publishes a VPN adapter AND its DNS server the moment you click connect,
// seconds before the handshake completes — and the previous network is already
// unroutable by then. Probing in that window finds nothing on every candidate,
// which looks exactly like "no tunnel". Rather than making a user guess how long
// to wait, this waits for them.
//
// Readiness is "some resolver answers for a fleet name", not "some resolver
// answers at all". A reachable-but-ignorant resolver means the link is up to
// somewhere, just not to the fleet — returning ready there would hand back
// control before the thing being waited for had happened.
func (r *Runtime) Wait(ctx context.Context, timeout time.Duration) (int, error) {
	if !r.WSL {
		r.sayf("not running under WSL; nothing to wait for.")
		return 0, nil
	}
	if timeout <= 0 {
		timeout = DefaultWaitTimeout
	}
	interval := r.PollInterval
	if interval <= 0 {
		interval = DefaultPollInterval
	}

	deadline := time.Now().Add(timeout)
	attempt := 0
	for {
		attempt++
		if ready, server, n := r.fleetReachable(ctx); ready {
			r.sayf("ready — %s resolves %d/%d fleet name(s) after %d check(s).",
				server, n, len(r.FleetHosts), attempt)
			return 0, nil
		}
		if ctx.Err() != nil {
			r.sayf("cancelled while waiting for the link to become ready.")
			return 1, nil
		}
		if time.Now().After(deadline) {
			r.sayf("timed out after %s waiting for a resolver that knows the fleet.", timeout)
			r.sayf("If a tunnel is connecting, it may need longer; if it is already up, the")
			r.sayf("resolver serving these names may not be the one that came up.")
			return 1, nil
		}
		select {
		case <-ctx.Done():
			r.sayf("cancelled while waiting for the link to become ready.")
			return 1, nil
		case <-time.After(interval):
		}
	}
}

// fleetReachable reports whether any candidate resolves a fleet name.
func (r *Runtime) fleetReachable(ctx context.Context) (bool, string, int) {
	if len(r.FleetHosts) == 0 {
		return false, "", 0
	}
	servers, err := r.candidates(ctx)
	if err != nil || len(servers) == 0 {
		return false, "", 0
	}
	scores := probe.Score(ctx, r.Lookup, probe.ScoreInput{
		Servers:        servers,
		Fleet:          r.FleetHosts,
		PublicSentinel: r.sentinel(),
	})
	if best, ok := probe.Winner(scores); ok {
		return true, best.Server, best.FleetResolved
	}
	return false, "", 0
}

func newWaitCmd() *cobra.Command {
	var ready bool
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "wait",
		Short: "Block until this box can resolve its fleet",
		Long: "Waits out the window where a VPN adapter and its DNS server exist but the\n" +
			"handshake has not finished — during which every resolver looks silent and the\n" +
			"link looks, wrongly, like it is down.\n\n" +
			"Exit code: 0 became ready, 1 timed out.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cc *cobra.Command, _ []string) error {
			if !ready {
				return errNeedsReadyFlag
			}
			rt := newRuntime(cc.Context())
			code, err := rt.Wait(cc.Context(), timeout)
			if err != nil {
				return err
			}
			exitWith(code)
			return nil
		},
	}
	c.Flags().BoolVar(&ready, "ready", false, "wait until the fleet is resolvable (required)")
	c.Flags().DurationVar(&timeout, "timeout", DefaultWaitTimeout, "give up after this long")
	return c
}
