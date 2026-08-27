package winhost

import (
	"context"
	"os"
	"testing"
)

// TestLive_AgainstRealWindows proves the parser handles this machine's actual
// PowerShell output, not just the recorded fixtures. Skipped unless
// WLINK_LIVE=1, so CI (which has no powershell.exe) stays green.
func TestLive_AgainstRealWindows(t *testing.T) {
	if os.Getenv("WLINK_LIVE") != "1" {
		t.Skip("set WLINK_LIVE=1 to run against the real Windows host")
	}
	ps, err := NewPowerShell()
	if err != nil {
		t.Skipf("no powershell.exe here: %v", err)
	}
	ifaces, err := New(ps).Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces() against real Windows: %v", err)
	}
	if len(ifaces) == 0 {
		t.Fatal("real Windows returned no interfaces — the parser is wrong")
	}
	for _, i := range ifaces {
		t.Logf("alias=%-40q index=%-3d tunnel=%-5v resolvers=%d",
			i.Alias, i.Index, i.IsTunnel, len(i.DNSServers))
	}
}
