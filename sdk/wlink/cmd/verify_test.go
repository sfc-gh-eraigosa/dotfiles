package cmd

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeSystem stands in for the host resolver path. Names in `ok` resolve
// instantly; everything else misses after `missDelay`, which is how a dead
// nameserver behaves.
type fakeSystem struct {
	ok        map[string]bool
	missDelay time.Duration
}

func (f fakeSystem) Resolve(_ context.Context, name string) (bool, time.Duration) {
	if f.ok[name] {
		return true, 0
	}
	return false, f.missDelay
}

func verifyRuntime(t *testing.T, sys SystemResolver) (*Runtime, *strings.Builder) {
	t.Helper()
	rt, _ := healthyRuntime(t)
	out := &strings.Builder{}
	rt.Out = out
	rt.System = sys
	rt.FleetHosts = []string{"lab-pi", "lab-nas"}
	return rt, out
}

// EC-7, tunnel UP: the public sentinel and every fleet name resolve.
func TestVerify_PassesWhenPublicAndFleetBothResolve(t *testing.T) {
	rt, out := verifyRuntime(t, fakeSystem{ok: map[string]bool{
		"github.com": true, "lab-pi": true, "lab-nas": true,
	}})
	code, err := rt.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0:\n%s", code, out)
	}
	if !strings.Contains(out.String(), "PASS") {
		t.Errorf("expected a PASS verdict:\n%s", out)
	}
}

// EC-7, tunnel DOWN: fleet names missing is EXPECTED, not a failure — what
// matters is that public DNS survived and the misses were fast.
func TestVerify_PassesOffTunnelWhenMissesAreFast(t *testing.T) {
	rt, out := verifyRuntime(t, fakeSystem{
		ok:        map[string]bool{"github.com": true},
		missDelay: 1 * time.Second,
	})
	code, err := rt.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0 — a fleet miss off-tunnel is expected:\n%s", code, out)
	}
	if !strings.Contains(out.String(), "PASS") {
		t.Errorf("expected PASS:\n%s", out)
	}
}

// EC-7: public DNS failing is a hard failure in EITHER tunnel state. This is
// the outcome the recursion guard exists to prevent, so verify must catch it.
func TestVerify_FailsWhenPublicDNSIsBroken(t *testing.T) {
	rt, out := verifyRuntime(t, fakeSystem{ok: map[string]bool{"lab-pi": true, "lab-nas": true}})
	code, err := rt.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 — public DNS is broken:\n%s", code, out)
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Errorf("expected FAIL:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out.String()), "public") {
		t.Errorf("failure must name the public-DNS problem:\n%s", out)
	}
}

// EC-7: a SLOW miss is the original 20s stall regressing — the whole reason
// this tool exists — so it fails even though the miss itself is expected.
func TestVerify_FailsWhenAMissExceedsTheBudget(t *testing.T) {
	rt, out := verifyRuntime(t, fakeSystem{
		ok:        map[string]bool{"github.com": true},
		missDelay: 30 * time.Second,
	})
	code, err := rt.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 — a slow miss is the stall regressing:\n%s", code, out)
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Errorf("expected FAIL:\n%s", out)
	}
}

// The budget must be derived from the resolv.conf in force and REPORTED, so a
// verdict is never a bare number the reader has to trust.
func TestVerify_ReportsTheDerivedBudget(t *testing.T) {
	rt, out := verifyRuntime(t, fakeSystem{ok: map[string]bool{
		"github.com": true, "lab-pi": true, "lab-nas": true,
	}})
	if _, err := rt.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "within") {
		t.Errorf("verify must state the budget it is holding lookups to:\n%s", out)
	}
}

// An explicit override wins over the derived value, for a user who knows their
// network is slower than the arithmetic assumes.
func TestVerify_HonoursAnExplicitBudget(t *testing.T) {
	rt, out := verifyRuntime(t, fakeSystem{
		ok:        map[string]bool{"github.com": true},
		missDelay: 8 * time.Second,
	})
	rt.MaxFailSeconds = 20
	code, err := rt.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0 — 8s is inside the 20s override:\n%s", code, out)
	}
}

// Off WSL, verify is a no-op that exits 0.
func TestVerify_NonWSLIsANoOp(t *testing.T) {
	rt, out := verifyRuntime(t, fakeSystem{})
	rt.WSL = false
	code, err := rt.Verify(context.Background())
	if err != nil || code != 0 {
		t.Errorf("Verify off WSL = (%d, %v), want (0, nil)", code, err)
	}
	if out.Len() == 0 {
		t.Error("must still say why it did nothing")
	}
}

// With no fleet names there is nothing to verify, which is a clean pass rather
// than a vacuous failure.
func TestVerify_NoFleetHostsIsACleanPass(t *testing.T) {
	rt, _ := verifyRuntime(t, fakeSystem{ok: map[string]bool{"github.com": true}})
	rt.FleetHosts = nil
	code, err := rt.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
}
