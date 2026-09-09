package cmd

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/wlink/internal/probe"
)

// becomesReady models a handshake completing: silent for the first n polls,
// then answering — exactly the window this command exists to wait out.
type becomesReady struct {
	afterPolls int32
	polls      atomic.Int32
}

func (b *becomesReady) LookupA(_ context.Context, server, name string) probe.Result {
	// Count one poll per sweep: the sentinel is queried once per candidate.
	if name == "github.com" {
		b.polls.Add(1)
	}
	if b.polls.Load() <= b.afterPolls {
		return probe.Result{Outcome: probe.Silent}
	}
	if server == "10.10.0.1" && name != "github.com" {
		return probe.Result{Outcome: probe.Resolved, Addrs: []string{"10.10.0.21"}}
	}
	return probe.Result{Outcome: probe.Resolved, Addrs: []string{"198.51.100.10"}}
}

func waitRuntime(t *testing.T, lookup probe.Lookup) (*Runtime, *strings.Builder) {
	t.Helper()
	rt, _ := healthyRuntime(t)
	out := &strings.Builder{}
	rt.Out = out
	rt.Lookup = lookup
	rt.PollInterval = time.Millisecond
	return rt, out
}

// EC-6: wait returns 0 once the tunnel starts answering. This is the whole
// point — a VPN adapter and its DNS server appear the moment you click connect,
// seconds before the handshake completes.
func TestWait_ReturnsZeroOnceTheTunnelAnswers(t *testing.T) {
	rt, out := waitRuntime(t, &becomesReady{afterPolls: 3})
	code, err := rt.Wait(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0 once the handshake completes:\n%s", code, out)
	}
	if !strings.Contains(out.String(), "ready") {
		t.Errorf("must say it became ready:\n%s", out)
	}
}

// EC-6: a tunnel that never comes up must time out with exit 1, not hang.
func TestWait_TimesOutWithExitOne(t *testing.T) {
	rt, out := waitRuntime(t, fakeZone{}) // nothing ever answers
	start := time.Now()
	code, err := rt.Wait(context.Background(), 80*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 on timeout", code)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("waited %s, want it bounded by the timeout", elapsed)
	}
	if !strings.Contains(strings.ToLower(out.String()), "timed out") {
		t.Errorf("must report the timeout plainly:\n%s", out)
	}
}

// Already reachable means return immediately — waiting on a link that already
// works would be a pointless delay in install.sh.
func TestWait_ReturnsImmediatelyWhenAlreadyReady(t *testing.T) {
	rt, out := waitRuntime(t, fakeZone{"10.10.0.1": {
		"lab-pi":     addr("10.10.0.21"),
		"github.com": addr("198.51.100.10"),
	}})
	start := time.Now()
	code, err := rt.Wait(context.Background(), 5*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0:\n%s", code, out)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %s on an already-ready link, want near-instant", elapsed)
	}
}

// A resolver that answers but knows nothing about the fleet is NOT ready:
// waiting exists to reach the fleet, not merely to see any DNS traffic.
func TestWait_ReachableButIgnorantIsNotReady(t *testing.T) {
	rt, _ := waitRuntime(t, fakeZone{
		"198.51.100.53": {"github.com": addr("198.51.100.10")},
		"10.10.0.1":     {"github.com": addr("198.51.100.10")},
	})
	code, err := rt.Wait(context.Background(), 60*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 — nothing resolves the fleet, so the link is not ready", code)
	}
}

// A cancelled context stops the wait promptly rather than running to timeout.
func TestWait_RespectsContextCancellation(t *testing.T) {
	rt, _ := waitRuntime(t, fakeZone{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	code, err := rt.Wait(ctx, 10*time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("ignored cancellation for %s", elapsed)
	}
}

// Off WSL there is no tunnel to wait for; exit 0 rather than blocking.
func TestWait_NonWSLIsANoOp(t *testing.T) {
	rt, out := waitRuntime(t, fakeZone{})
	rt.WSL = false
	code, err := rt.Wait(context.Background(), 5*time.Second)
	if err != nil || code != 0 {
		t.Errorf("Wait off WSL = (%d, %v), want (0, nil)", code, err)
	}
	if out.Len() == 0 {
		t.Error("must still say why it did nothing")
	}
}
