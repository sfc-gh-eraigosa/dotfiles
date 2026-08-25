package cmd

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/resolvconf"
	"github.com/spf13/cobra"
)

// SystemResolver resolves through the HOST's normal path — nsswitch, then
// /etc/resolv.conf in order.
//
// This is deliberately NOT the direct-to-server probe used elsewhere. Querying
// a chosen server bypasses resolv.conf ordering entirely and would prove
// nothing about what ssh actually experiences, which is the only thing verify
// is trying to establish.
type SystemResolver interface {
	Resolve(ctx context.Context, name string) (found bool, elapsed time.Duration)
}

// hostResolver is the real one: Go's default resolver, which reads
// /etc/resolv.conf exactly as any other program does.
type hostResolver struct{}

func (hostResolver) Resolve(ctx context.Context, name string) (bool, time.Duration) {
	start := time.Now()
	addrs, err := net.DefaultResolver.LookupHost(ctx, name)
	return err == nil && len(addrs) > 0, time.Since(start)
}

// Verify runs the tunnel-up / tunnel-down matrix.
//
// The invariants, in the order they matter:
//
//  1. the public sentinel must resolve in BOTH tunnel states — this is what the
//     recursion guard protects, and its failure is the worst outcome wlink can
//     cause;
//  2. fleet names resolving only when the tunnel is up is EXPECTED, not a
//     failure;
//  3. a fleet MISS must complete inside the derived budget — a slow miss is the
//     20s stall this tool exists to remove, regressing.
func (r *Runtime) Verify(ctx context.Context) (int, error) {
	if !r.WSL {
		r.sayf("not running under WSL; nothing to verify.")
		return 0, nil
	}
	sys := r.System
	if sys == nil {
		sys = hostResolver{}
	}

	budget := r.MaxFailSeconds
	source := "explicit --max-fail-seconds"
	if budget <= 0 {
		budget = resolvconf.FailBudgetSeconds(readFileOrEmpty(r.Paths.ResolvConf))
		source = "derived from " + r.Paths.ResolvConf
	}
	limit := time.Duration(budget) * time.Second

	fmt.Fprintf(r.Out, "verify — a failed lookup must complete within %ds (%s)\n", budget, source)
	fmt.Fprintf(r.Out, "verify — NAME RESULT SECONDS\n")

	failed := false

	// 1. The public sentinel, in both states.
	sentinel := r.sentinel()
	ok, elapsed := sys.Resolve(ctx, sentinel)
	fmt.Fprintf(r.Out, "  %s %s %ds\n", sentinel, resultWord(ok), int(elapsed.Seconds()))
	if !ok {
		r.sayf("WARNING — %s did NOT resolve: public DNS is broken. That is a FAIL in either tunnel state.", sentinel)
		failed = true
	}

	// 2 & 3. Fleet names: a miss is fine, a SLOW miss is not.
	resolvedCount := 0
	for _, name := range r.FleetHosts {
		ok, elapsed := sys.Resolve(ctx, name)
		fmt.Fprintf(r.Out, "  %s %s %ds\n", name, resultWord(ok), int(elapsed.Seconds()))
		switch {
		case ok:
			resolvedCount++
		case elapsed > limit:
			r.sayf("WARNING — %s took %ds to fail (limit %ds) — the resolver timeout tuning regressed.",
				name, int(elapsed.Seconds()), budget)
			failed = true
		}
	}

	total := len(r.FleetHosts)
	switch {
	case total == 0:
		r.sayf("verify — no fleet names to check; public DNS is the only invariant, and it holds.")
	case resolvedCount == total:
		r.sayf("verify — fleet reachable: public OK, %d/%d fleet names resolve.", resolvedCount, total)
	case failed:
		r.sayf("verify — fleet unreachable: a miss exceeded %ds (see the warnings above).", budget)
		r.sayf("verify — that slow miss IS the bug this tool fixes; run `wlink pin`.")
	default:
		r.sayf("verify — fleet unreachable: public OK, %d/%d resolve, and every miss failed within %ds.",
			resolvedCount, total, budget)
	}

	if failed {
		fmt.Fprintln(r.Out, "verify — FAIL")
		return 1, nil
	}
	fmt.Fprintln(r.Out, "verify — PASS")
	return 0, nil
}

func resultWord(ok bool) string {
	if ok {
		return "OK"
	}
	return "MISS"
}

func newVerifyCmd() *cobra.Command {
	var maxFail int
	c := &cobra.Command{
		Use:   "verify",
		Short: "Check name resolution against the tunnel up/down matrix",
		Long: "Resolves through the host resolver — not a direct query to a chosen server,\n" +
			"which would bypass resolv.conf ordering and prove nothing about what ssh sees.\n\n" +
			"Run it once with the tunnel up and once with it down. The public sentinel must\n" +
			"resolve in BOTH states; fleet names only when the tunnel is up; and a fleet miss\n" +
			"must fail FAST — a slow miss is the stall this tool exists to remove.\n\n" +
			"Exit code: 0 expectations met, 1 an expectation failed.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cc *cobra.Command, _ []string) error {
			rt := newRuntime(cc.Context())
			rt.MaxFailSeconds = maxFail
			code, err := rt.Verify(cc.Context())
			if err != nil {
				return err
			}
			exitWith(code)
			return nil
		},
	}
	c.Flags().IntVar(&maxFail, "max-fail-seconds", 0,
		"longest acceptable failed lookup; 0 derives it from the resolv.conf in force")
	return c
}
