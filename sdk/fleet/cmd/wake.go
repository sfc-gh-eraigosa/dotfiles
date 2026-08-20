package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/spf13/cobra"
)

var (
	flagNoWake      bool
	flagWakeTimeout time.Duration
)

// validateWakeTimeout rejects a budget that cannot do any work. Zero or
// negative would leave every ladder cancelled before its first rung while
// still costing a round of probes — a silent no-op is worse than an error.
func validateWakeTimeout(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("--wake-timeout must be positive, got %s", d)
	}
	return nil
}

// wakePolicy turns the flags into a policy. --no-wake disables every rung.
func wakePolicy() (reach.Policy, error) {
	if err := validateWakeTimeout(flagWakeTimeout); err != nil {
		return reach.Policy{}, err
	}
	return reach.Policy{
		Enabled: !flagNoWake,
		Budget:  flagWakeTimeout,
		Retries: 1,
	}, nil
}

// wakeProbeTimeout is the SSH connect timeout for a wake probe. Deliberately
// shorter than the default: inside a bounded ladder a host either answers
// promptly after a nudge or it is not going to, and spending the standard
// timeout per probe starved the relay rung outright when this was first
// measured against real hardware.
const wakeProbeTimeout = "4"

// prober derives a fast-failing runner for reachability probes. Non-Exec
// runners (the test fakes) pass through untouched.
func prober(r runner.Runner) runner.Runner {
	if e, ok := r.(runner.Exec); ok {
		e.ConnectTimeout = wakeProbeTimeout
		return e
	}
	return r
}

// newWaker builds the production waker: the ladder from internal/reach with
// its impure edges bound to the real network. Returns nil when wake is
// disabled, which is exactly how every caller turns the feature off.
// wakeEnv is the environment-facing half of the ladder, held in variables so a
// test can run it without touching the network.
//
// With the real functions wired in directly, a wake test's OUTCOME depended on
// the machine it ran on: localAddrs() reports this host's own interfaces, so a
// workstation inside the target's subnet takes the local-prime rung and the
// ladder never reaches peer-relay. TestRelayNudgeIsDetachedAndFlagPortable
// passed on a WSL2 box (172.x NAT, local-prime skipped) and failed on any host
// in the target's /24 — and it shelled out to a real `ping` either way.
var wakeEnv = struct {
	Resolve    func(host string) ([]net.IP, error)
	LocalAddrs func() ([]net.IPNet, error)
	PingLocal  func(ctx context.Context, ip string) error
	Sleep      func(time.Duration)
}{resolveHost, localAddrs, pingLocal, time.Sleep}

func newWaker(r runner.Runner, p reach.Policy) waker {
	if !p.Enabled {
		return nil
	}
	pr := prober(r)
	return func(target sshconf.Host, peers []reach.Peer) reach.Result {
		name := target.HostName
		if name == "" {
			name = target.Alias
		}
		return reach.Wake(context.Background(),
			reach.Peer{Alias: target.Alias, HostName: name},
			peers, p,
			reach.Deps{
				Probe:      func(alias string) error { _, err := pr.Run(alias, "true"); return err },
				Runner:     r,
				Resolve:    wakeEnv.Resolve,
				LocalAddrs: wakeEnv.LocalAddrs,
				PingLocal:  wakeEnv.PingLocal,
				Sleep:      wakeEnv.Sleep,
			})
	}
}

func resolveHost(host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	return net.LookupIP(host)
}

// localAddrs reports this machine's global unicast networks. Under WSL2 NAT
// these are a private 172.x link that contains no fleet host, which is
// precisely why the local-prime rung ends up skipped there.
func localAddrs() ([]net.IPNet, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var out []net.IPNet
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if ok && n.IP.IsGlobalUnicast() {
			out = append(out, *n)
		}
	}
	return out, nil
}

// pingLocal forces the local stack to ARP for ip. The process is bounded by
// the context rather than by ping's own flags: `-W` means seconds on GNU and
// milliseconds on BSD, so using it would behave differently on Linux and
// macOS. `-n` keeps ping off DNS.
func pingLocal(ctx context.Context, ip string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "ping", "-c", "2", "-n", ip).Run()
}

// fleetPeers converts the inventory into relay candidates. Reachability is
// unknown here (no probe has run), so the ladder falls back to subnet
// affinity for ordering.
func fleetPeers(all []sshconf.Host, target string) []reach.Peer {
	out := make([]reach.Peer, 0, len(all))
	for _, h := range all {
		if h.Alias == target {
			continue
		}
		name := h.HostName
		if name == "" {
			name = h.Alias
		}
		out = append(out, reach.Peer{Alias: h.Alias, HostName: name})
	}
	return out
}

// renderWake prints the ladder rung by rung. Silence on success would be a
// mistake: the ladder IS the diagnostic, and an operator running `fleet wake`
// explicitly wants to see which rung did the work.
func renderWake(alias string, res reach.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", alias)
	for _, a := range res.Attempts {
		mark := "x"
		if a.OK {
			mark = "OK"
		}
		line := fmt.Sprintf("  %-12s %-2s", a.Rung, mark)
		if a.Via != "" && a.Via != a.Rung {
			line += " via " + a.Via
		}
		if a.Err != "" {
			line += "  " + a.Err
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	if res.Woke {
		fmt.Fprintf(&b, "  => reachable (woke via %s)\n", res.Via)
	} else {
		b.WriteString("  => still unreachable\n")
	}
	return b.String()
}

type wakeJSON struct {
	Alias    string          `json:"alias"`
	Woke     bool            `json:"woke"`
	Via      string          `json:"via,omitempty"`
	Attempts []reach.Attempt `json:"attempts"`
}

func renderWakeJSON(out []wakeJSON) string {
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b)
}

var wakeCmd = &cobra.Command{
	Use:   "wake [host...]",
	Short: "Rouse hosts that are asleep at layer 2 rather than genuinely down",
	Long: `Escalate a bounded reachability ladder against each host: retry, prime the
local ARP cache, then relay through a reachable peer and ask the target to
send traffic back to this workstation.

Wake never modifies a target. It sends ICMP, reads $SSH_CONNECTION on a peer,
and probes. The permanent cure for a host that needs waking every run is to
disable Wi-Fi power save on that host.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := wakePolicy()
		if err != nil {
			return err
		}
		// An explicit `fleet wake` means the operator wants the ladder, so
		// --no-wake is not allowed to quietly turn it into a no-op.
		p.Enabled = true

		raw, err := os.ReadFile(flagConfig)
		if err != nil {
			return fmt.Errorf("reading %s: %w", flagConfig, err)
		}
		all, err := sshconf.Parse(string(raw), flagMarker)
		if err != nil {
			return err
		}
		targets := selectHosts(all, args)
		if len(targets) == 0 {
			return fmt.Errorf("no fleet hosts found — mark hosts with %q in %s, or pass aliases explicitly", flagMarker, flagConfig)
		}

		r := runner.Exec{}
		w := newWaker(r, p)

		var jsonOut []wakeJSON
		stayedDown := 0
		for _, h := range targets {
			res := w(h, fleetPeers(all, h.Alias))
			if !res.Woke {
				stayedDown++
			}
			if flagJSON {
				jsonOut = append(jsonOut, wakeJSON{Alias: h.Alias, Woke: res.Woke, Via: res.Via, Attempts: res.Attempts})
			} else {
				fmt.Fprint(cmd.OutOrStdout(), renderWake(h.Alias, res))
			}
		}
		if flagJSON {
			fmt.Fprintln(cmd.OutOrStdout(), renderWakeJSON(jsonOut))
		}
		if stayedDown > 0 {
			return exitError{code: 1}
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagNoWake, "no-wake", false,
		"do not try to rouse unreachable hosts (faster, literal answer)")
	rootCmd.PersistentFlags().DurationVar(&flagWakeTimeout, "wake-timeout", reach.DefaultBudget,
		"per-host budget for the reachability ladder")
	rootCmd.AddCommand(wakeCmd)
}
