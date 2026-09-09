package lanscan

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestHostsInEnumeratesAUsableRange(t *testing.T) {
	got, err := HostsIn("192.168.0.201/24")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 254 {
		t.Fatalf("got %d addresses, want 254 (network and broadcast excluded)", len(got))
	}
	if got[0] != "192.168.0.1" || got[253] != "192.168.0.254" {
		t.Fatalf("range = %s..%s", got[0], got[253])
	}
	for _, ip := range got {
		if ip == "192.168.0.0" || ip == "192.168.0.255" {
			t.Fatalf("%s must not be probed", ip)
		}
	}
}

// A /16 would be 65k dials. Refusing is better than appearing to hang.
func TestHostsInRefusesARangeTooLargeToSweep(t *testing.T) {
	if _, err := HostsIn("10.0.0.1/16"); err == nil {
		t.Fatal("want an error for a range larger than a /22")
	}
}

func TestHostsInRejectsGarbage(t *testing.T) {
	if _, err := HostsIn("not-a-cidr"); err == nil {
		t.Fatal("want an error")
	}
}

// The sweep reports only what answered, and must not be fooled into reporting
// an address that refused.
func TestSweepReportsOnlyOpenPorts(t *testing.T) {
	open := map[string]bool{"10.0.0.2": true, "10.0.0.5": true}
	dial := func(_ context.Context, addr string) error {
		host, _, _ := net.SplitHostPort(addr)
		if open[host] {
			return nil
		}
		return errors.New("refused")
	}
	got := Sweep(context.Background(), []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.5"}, 22, dial, 4)
	if len(got) != 2 || got[0] != "10.0.0.2" || got[1] != "10.0.0.5" {
		t.Fatalf("got %v, want the two open addresses in order", got)
	}
}

// Results must be ordered by address, not by whichever goroutine finished
// first, or the same network renders differently on every run.
func TestSweepIsOrderStableRegardlessOfCompletionOrder(t *testing.T) {
	var mu sync.Mutex
	seen := 0
	dial := func(_ context.Context, addr string) error {
		mu.Lock()
		n := seen
		seen++
		mu.Unlock()
		// Finish in reverse-ish order.
		time.Sleep(time.Duration(10-n%10) * time.Millisecond)
		return nil
	}
	ips := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"}
	got := Sweep(context.Background(), ips, 22, dial, 5)
	for i := range got {
		if got[i] != ips[i] {
			t.Fatalf("got %v, want %v — results must be address-ordered", got, ips)
		}
	}
}

// A cancelled sweep stops rather than dialing every remaining address.
func TestSweepHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := Sweep(ctx, []string{"10.0.0.1", "10.0.0.2"}, 22, func(context.Context, string) error { return nil }, 2)
	if len(got) != 0 {
		t.Fatalf("got %v, want nothing from a cancelled sweep", got)
	}
}
