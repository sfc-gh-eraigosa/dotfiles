package reach

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

var errDown = errors.New("unreachable")

// probeScript answers Probe() from a queue — one entry per call, the last
// entry repeating — so a test can say "fail, fail, then succeed".
type probeScript struct {
	answers []error
	calls   int
}

func (p *probeScript) probe(string) error {
	i := p.calls
	p.calls++
	if i >= len(p.answers) {
		i = len(p.answers) - 1
	}
	return p.answers[i]
}

func alwaysDown() *probeScript { return &probeScript{answers: []error{errDown}} }

const wsConn = "192.168.0.249 49920 192.168.0.63 22"

// stub builds Deps with every impure edge defaulted to something harmless, so
// each test overrides only the edge it is actually about. Sleep is recorded
// rather than performed: the suite must not spend real time.
func stub(p *probeScript, r runner.Runner) (Deps, *[]time.Duration, *[]string) {
	slept := &[]time.Duration{}
	pinged := &[]string{}
	return Deps{
		Probe:      p.probe,
		Runner:     r,
		Resolve:    func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("192.168.0.128")}, nil },
		LocalAddrs: func() ([]net.IPNet, error) { return nil, nil }, // off-subnet by default (the WSL2 NAT case)
		PingLocal: func(_ context.Context, ip string) error {
			*pinged = append(*pinged, ip)
			return nil
		},
		Sleep: func(d time.Duration) { *slept = append(*slept, d) },
	}, slept, pinged
}

func pol() Policy { return Policy{Enabled: true, Budget: 8 * time.Second, Retries: 2} }

func onSubnet() func() ([]net.IPNet, error) {
	return func() ([]net.IPNet, error) {
		return []net.IPNet{{IP: net.ParseIP("192.168.0.249"), Mask: net.CIDRMask(24, 32)}}, nil
	}
}

// F14a — the ladder stops at the first rung whose direct re-probe succeeds.
// A later rung running after success would waste the budget and misreport Via.
func TestLadderStopsAtTheFirstRungThatWorks(t *testing.T) {
	p := &probeScript{answers: []error{nil}}
	d, _, _ := stub(p, runner.Fake{})
	res := Wake(context.Background(), Peer{Alias: "target"}, []Peer{{Alias: "peer", Reachable: true}}, pol(), d)

	if !res.Woke {
		t.Fatal("want Woke")
	}
	if res.Via != RungRetry {
		t.Fatalf("Via = %q, want %q", res.Via, RungRetry)
	}
	if len(res.Attempts) != 1 {
		t.Fatalf("want exactly 1 attempt, got %d: %+v", len(res.Attempts), res.Attempts)
	}
}

// F14b — THE false-green guard. A successful relay proves only that the PEER
// can route to the target; it says nothing about the workstation. Reporting
// Woke on that alone would convert a real network partition into a green row.
func TestRelaySuccessAloneNeverReportsWoke(t *testing.T) {
	f := runner.Fake{
		Out: map[string]string{"peer": wsConn},
		Via: map[string]string{},
	}
	d, _, _ := stub(alwaysDown(), f)
	res := Wake(context.Background(), Peer{Alias: "target"}, []Peer{{Alias: "peer", Reachable: true}}, pol(), d)

	if res.Woke {
		t.Fatal("relay success must never imply the workstation can reach the target")
	}
	if f.Via["target"] != "peer" {
		t.Fatalf("expected a relay attempt through peer, Via = %v", f.Via)
	}
}

// The happy path: the relay nudge lands and the DIRECT probe then succeeds.
// Via must name the peer, so the operator learns which machine rescued it.
func TestRelayWakeReportsThePeerItWokeThrough(t *testing.T) {
	// Down for both retries; up once the relay has nudged it. local-prime is
	// skipped here (off-subnet default) and so consumes no probe call.
	p := &probeScript{answers: []error{errDown, errDown, nil}}
	f := runner.Fake{Out: map[string]string{"peer": wsConn}, Via: map[string]string{}}
	d, _, _ := stub(p, f)

	res := Wake(context.Background(), Peer{Alias: "target"}, []Peer{{Alias: "peer", Reachable: true}}, pol(), d)
	if !res.Woke {
		t.Fatalf("want Woke, attempts: %+v", res.Attempts)
	}
	if res.Via != "peer" {
		t.Fatalf("Via = %q, want the peer alias", res.Via)
	}
}

// F14c — local-prime is for the case where fleet runs ON the LAN. Under WSL2
// NAT the workstation has no layer-2 presence on the fleet subnet at all, so
// pinging from here is a no-op and the rung must be skipped, not attempted.
func TestLocalPrimeOnlyRunsWhenTheWorkstationSharesTheTargetSubnet(t *testing.T) {
	t.Run("on-subnet fires the prime", func(t *testing.T) {
		d, _, pinged := stub(alwaysDown(), runner.Fake{})
		d.LocalAddrs = onSubnet()
		Wake(context.Background(), Peer{Alias: "target", HostName: "target"}, nil, pol(), d)
		if len(*pinged) == 0 {
			t.Fatal("on-subnet workstation must prime the local ARP cache")
		}
		if (*pinged)[0] != "192.168.0.128" {
			t.Fatalf("primed %q, want the resolved target address", (*pinged)[0])
		}
	})

	t.Run("off-subnet skips it", func(t *testing.T) {
		d, _, pinged := stub(alwaysDown(), runner.Fake{}) // LocalAddrs defaults to none
		res := Wake(context.Background(), Peer{Alias: "target", HostName: "target"}, nil, pol(), d)
		if len(*pinged) != 0 {
			t.Fatalf("off-subnet workstation must not ping; pinged %v", *pinged)
		}
		if !hasSkippedRung(res, RungLocalPrime) {
			t.Fatalf("the skipped rung must still be reported: %+v", res.Attempts)
		}
	})
}

// F14d — a peer that is itself asleep must not be tried ahead of one known to
// be answering; otherwise wake turns one slow host into two.
func TestPeerRankingPrefersReachableThenSameSubnet(t *testing.T) {
	target := Peer{Alias: "target", HostName: "192.168.0.128"}
	peers := []Peer{
		{Alias: "far-down", HostName: "10.0.0.5"},
		{Alias: "far-up", HostName: "10.0.0.6", Reachable: true},
		{Alias: "near-up", HostName: "192.168.0.63", Reachable: true},
	}
	got := rankPeers(peers, target)

	var order []string
	for _, p := range got {
		order = append(order, p.Alias)
	}
	want := []string{"near-up", "far-up", "far-down"}
	for i := range want {
		if i >= len(order) || order[i] != want[i] {
			t.Fatalf("ranking = %v, want %v", order, want)
		}
	}
}

// F14f — a single-host fleet has no peer. The target must never be offered as
// its own relay: ssh -J target target is nonsense and would burn the budget.
func TestTargetIsNeverItsOwnPeer(t *testing.T) {
	target := Peer{Alias: "target"}
	if got := rankPeers([]Peer{{Alias: "target", Reachable: true}}, target); len(got) != 0 {
		t.Fatalf("target must be excluded from its own peer list, got %+v", got)
	}
}

func TestSingleHostFleetDegradesGracefully(t *testing.T) {
	d, _, _ := stub(alwaysDown(), runner.Fake{})
	res := Wake(context.Background(), Peer{Alias: "target"}, nil, pol(), d)
	if res.Woke {
		t.Fatal("no peer, host down: must not report Woke")
	}
	if len(res.Attempts) == 0 {
		t.Fatal("the exhausted ladder must still be reported for diagnosis")
	}
}

// Regression, found by live testing against real hardware: with a 20s budget
// and two retries the ladder spent the WHOLE budget probing a dead host and
// never reached peer-relay — starving the only rung that fixes the motivating
// failure. The cheapest rung must never be able to consume the budget the
// effective rung needs.
func TestRetryRungCannotStarveThePeerRelay(t *testing.T) {
	const budget = 300 * time.Millisecond

	// A probe that costs real time, as a real SSH to a dead host does.
	slowProbe := &probeScript{answers: []error{errDown}}
	f := runner.Fake{Out: map[string]string{"peer": wsConn}, Via: map[string]string{}}
	d, _, _ := stub(slowProbe, f)
	d.Probe = func(string) error {
		time.Sleep(30 * time.Millisecond)
		return errDown
	}

	res := Wake(context.Background(), Peer{Alias: "target"},
		[]Peer{{Alias: "peer", Reachable: true}},
		Policy{Enabled: true, Budget: budget, Retries: 100}, d)

	var sawRelay bool
	for _, a := range res.Attempts {
		if a.Rung == RungPeerRelay {
			sawRelay = true
		}
	}
	if !sawRelay {
		t.Fatalf("retry starved the relay rung; attempts: %+v", res.Attempts)
	}
	if len(f.Via) == 0 {
		t.Fatal("the relay must actually have been attempted")
	}
}

// F14e — the budget is a hard stop. A cancelled context must prevent the next
// rung from starting, or "bounded" means nothing.
func TestExhaustedBudgetStopsTheLadder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := alwaysDown()
	f := runner.Fake{Out: map[string]string{"peer": wsConn}, Via: map[string]string{}}
	d, _, _ := stub(p, f)

	res := Wake(ctx, Peer{Alias: "target"}, []Peer{{Alias: "peer", Reachable: true}}, pol(), d)
	if res.Woke {
		t.Fatal("a cancelled ladder cannot have woken anything")
	}
	if len(f.Via) != 0 {
		t.Fatalf("no relay may start after the deadline, got %v", f.Via)
	}
}

// F14g — every wait goes through the injected clock, so the suite spends no
// real time and the ladder stays deterministic.
func TestLadderSleepsOnlyThroughTheInjectedClock(t *testing.T) {
	start := time.Now()
	d, slept, _ := stub(alwaysDown(), runner.Fake{})
	Wake(context.Background(), Peer{Alias: "target"}, nil, pol(), d)

	if len(*slept) == 0 {
		t.Fatal("the retry rung must back off through Deps.Sleep")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("ladder spent %v of real time; waits must be injected", elapsed)
	}
}

// A disabled policy is a hard off switch: --no-wake must reach every rung.
func TestDisabledPolicyRunsNothing(t *testing.T) {
	p := alwaysDown()
	d, _, pinged := stub(p, runner.Fake{})
	d.LocalAddrs = onSubnet()

	res := Wake(context.Background(), Peer{Alias: "target"}, []Peer{{Alias: "peer", Reachable: true}},
		Policy{Enabled: false, Budget: time.Second, Retries: 2}, d)

	if res.Woke || len(res.Attempts) != 0 {
		t.Fatalf("disabled wake must do nothing, got %+v", res)
	}
	if p.calls != 0 || len(*pinged) != 0 {
		t.Fatalf("disabled wake probed %d times and pinged %v", p.calls, *pinged)
	}
}

// F18b + the 11s measurement. On real hardware a blocking no-reply `ping -c 2`
// cost 11.0s — more than the whole default budget — because GNU ping waits out
// the final reply. The replies never come: the workstation's firewall drops
// inbound ICMP. Only the ARP request matters, and it leaves in the first
// milliseconds, so the nudge is detached and never waited on. `-W` is banned
// outright: it means seconds on GNU and milliseconds on BSD.
func TestNudgeIsFireAndForgetAndNeverUsesDashW(t *testing.T) {
	cmd := nudgeCmd("192.168.0.249")

	if !strings.Contains(cmd, "&") {
		t.Fatalf("nudge must be detached, got %q", cmd)
	}
	for _, banned := range []string{" -W", " -w"} {
		if strings.Contains(cmd, banned) {
			t.Fatalf("nudge must not use %q (GNU seconds vs BSD milliseconds): %q", banned, cmd)
		}
	}
	if !strings.Contains(cmd, "192.168.0.249") {
		t.Fatalf("nudge must target the workstation address, got %q", cmd)
	}
}

// The workstation address comes from the peer's view of our connection, which
// is what sees through the NAT. Garbage must not reach a shell command.
func TestWorkstationIPRejectsUnparseableSSHConnection(t *testing.T) {
	f := runner.Fake{Out: map[string]string{"peer": "not-an-ip 1 2 3"}}
	if _, err := workstationIP(f, "peer"); err == nil {
		t.Fatal("a non-IP $SSH_CONNECTION must be rejected, not interpolated into a command")
	}
}

func TestWorkstationIPParsesTheClientAddress(t *testing.T) {
	f := runner.Fake{Out: map[string]string{"peer": wsConn}}
	got, err := workstationIP(f, "peer")
	if err != nil {
		t.Fatalf("workstationIP: %v", err)
	}
	if got != "192.168.0.249" {
		t.Fatalf("got %q, want the first field of $SSH_CONNECTION", got)
	}
}

func hasSkippedRung(r Result, rung string) bool {
	for _, a := range r.Attempts {
		if a.Rung == rung && strings.HasPrefix(a.Err, "skipped") {
			return true
		}
	}
	return false
}
