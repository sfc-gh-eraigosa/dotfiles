package cmd

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/reach"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// F16a — the ladder IS the diagnostic. A bare "ok" would tell the operator
// nothing about which rung did the work, which is the whole reason to run the
// command by hand.
func TestRenderWakeShowsEveryRungAndTheVerdict(t *testing.T) {
	out := renderWake("sleeper", reach.Result{
		Woke: true,
		Via:  "peer-a",
		Attempts: []reach.Attempt{
			{Rung: reach.RungRetry, Via: reach.RungRetry, Err: "still unreachable after 2 retries"},
			{Rung: reach.RungLocalPrime, Via: reach.RungLocalPrime, Err: "skipped: workstation is not on the target's subnet"},
			{Rung: reach.RungPeerRelay, Via: "peer-a", OK: true},
		},
	})

	for _, want := range []string{"sleeper", reach.RungRetry, reach.RungLocalPrime, reach.RungPeerRelay, "peer-a", "woke via peer-a"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "skipped") {
		t.Fatalf("a skipped rung must still be shown — knowing what was NOT tried matters:\n%s", out)
	}
}

// F16b — a host that stayed down must read as a failure, not a quiet success.
func TestRenderWakeReportsAnExhaustedLadder(t *testing.T) {
	out := renderWake("dead", reach.Result{
		Attempts: []reach.Attempt{{Rung: reach.RungRetry, Err: "still unreachable after 2 retries"}},
	})
	if !strings.Contains(out, "still unreachable") {
		t.Fatalf("an exhausted ladder must say so:\n%s", out)
	}
	if strings.Contains(out, "woke via") {
		t.Fatalf("a host that never woke must not claim provenance:\n%s", out)
	}
}

// F16c — the JSON surface carries the full ladder so it can be diagnosed from
// a script or a dashboard.
func TestWakeJSONCarriesTheWholeLadder(t *testing.T) {
	raw := renderWakeJSON([]wakeJSON{{
		Alias: "sleeper",
		Woke:  true,
		Via:   "peer-a",
		Attempts: []reach.Attempt{
			{Rung: reach.RungRetry, Err: "no"},
			{Rung: reach.RungPeerRelay, Via: "peer-a", OK: true},
		},
	}})

	var got []wakeJSON
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("wake --json must round-trip: %v\n%s", err, raw)
	}
	if len(got) != 1 || len(got[0].Attempts) != 2 {
		t.Fatalf("attempts lost in serialisation: %+v", got)
	}
	if !got[0].Woke || got[0].Via != "peer-a" {
		t.Fatalf("verdict lost in serialisation: %+v", got[0])
	}
}

// F16d — THE non-mutation invariant. Wake runs automatically inside a read
// path (`fleet status`), so it must never change anything on a target. The
// hermeticWake pins the environment-facing deps so the ladder behaves the same
// everywhere: no real interfaces, no real ping, no real sleeping. Without it
// these tests pass or fail according to whether the machine running them
// happens to share a subnet with the hardcoded target.
func hermeticWake(t *testing.T) {
	t.Helper()
	saved := wakeEnv
	t.Cleanup(func() { wakeEnv = saved })
	wakeEnv.LocalAddrs = func() ([]net.IPNet, error) { return nil, nil } // no local subnet => local-prime skipped
	wakeEnv.PingLocal = func(context.Context, string) error { return nil }
	wakeEnv.Sleep = func(time.Duration) {}
	wakeEnv.Resolve = func(h string) ([]net.IP, error) {
		if ip := net.ParseIP(h); ip != nil {
			return []net.IP{ip}, nil
		}
		return nil, nil
	}
}

// only traffic allowed is ICMP, a read of $SSH_CONNECTION, and the probe.
func TestWakeNeverSendsAnythingThatWritesToATarget(t *testing.T) {
	hermeticWake(t)
	var sent []string
	r := recordingRunner{
		fake: runner.Fake{Out: map[string]string{"peer-a": "192.168.0.249 49920 192.168.0.63 22"}, Via: map[string]string{}},
		log:  &sent,
	}

	w := newWaker(r, reach.Policy{Enabled: true, Budget: 2 * time.Second, Retries: 1})
	w(sshconf.Host{Alias: "sleeper", HostName: "192.168.0.128"},
		[]reach.Peer{{Alias: "peer-a", HostName: "192.168.0.63", Reachable: true}})

	if len(sent) == 0 {
		t.Fatal("expected the ladder to have sent something")
	}
	for _, cmd := range sent {
		switch {
		case strings.Contains(cmd, "SSH_CONNECTION"):
		case strings.HasPrefix(cmd, "(ping "):
		case cmd == "true": // the reachability probe
		default:
			t.Fatalf("wake sent a command that is not ICMP, $SSH_CONNECTION, or the probe: %q", cmd)
		}
		for _, banned := range []string{"rm ", "install.sh", "sudo", "systemctl", "tee ", ">>", "git "} {
			if strings.Contains(cmd, banned) {
				t.Fatalf("wake must not mutate a target; command %q contains %q", cmd, banned)
			}
		}
	}
}

// F18b — `-W` is seconds on GNU and milliseconds on BSD. Using it would make
// the nudge behave 1000x differently between Linux and macOS targets. The
// nudge is also detached: a blocking no-reply ping measured 11.0s on real
// hardware, more than the entire default budget.
func TestRelayNudgeIsDetachedAndFlagPortable(t *testing.T) {
	hermeticWake(t)
	var sent []string
	r := recordingRunner{
		fake: runner.Fake{Out: map[string]string{"peer-a": "192.168.0.249 49920 192.168.0.63 22"}, Via: map[string]string{}},
		log:  &sent,
	}

	w := newWaker(r, reach.Policy{Enabled: true, Budget: 2 * time.Second, Retries: 0})
	w(sshconf.Host{Alias: "sleeper", HostName: "192.168.0.128"},
		[]reach.Peer{{Alias: "peer-a", HostName: "192.168.0.63", Reachable: true}})

	var nudge string
	for _, c := range sent {
		if strings.HasPrefix(c, "(ping ") {
			nudge = c
		}
	}
	if nudge == "" {
		t.Fatalf("no relay nudge was sent; commands: %v", sent)
	}
	if !strings.Contains(nudge, "&") {
		t.Fatalf("nudge must be detached, got %q", nudge)
	}
	for _, banned := range []string{" -W", " -w"} {
		if strings.Contains(nudge, banned) {
			t.Fatalf("nudge must not use %q: %q", banned, nudge)
		}
	}
}

// --no-wake produces a nil waker, which is how every call site disables the
// feature without a second code path to keep in sync.
func TestDisabledPolicyProducesNoWaker(t *testing.T) {
	if w := newWaker(runner.Fake{}, reach.Policy{Enabled: false}); w != nil {
		t.Fatal("a disabled policy must yield a nil waker")
	}
}

// fleetPeers is the explicit-verb path's candidate list: everything but the
// target, so `fleet wake <host>` can never try to relay through the very host
// it is trying to reach.
func TestFleetPeersExcludesTheTarget(t *testing.T) {
	all := []sshconf.Host{{Alias: "a"}, {Alias: "b", HostName: "b.local"}, {Alias: "c"}}
	got := fleetPeers(all, "b")

	if len(got) != 2 {
		t.Fatalf("want 2 peers, got %+v", got)
	}
	for _, p := range got {
		if p.Alias == "b" {
			t.Fatalf("target must be excluded: %+v", got)
		}
		if p.HostName == "" {
			t.Fatalf("peer %q must carry a resolvable name for subnet ranking", p.Alias)
		}
	}
}
