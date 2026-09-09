// Package winhost is the single place in wlink that talks to Windows.
//
// WSL cannot see Windows' per-interface network configuration, but that is
// exactly where the answer lives: the resolver that knows your fleet is
// frequently on a VPN/tunnel interface rather than the default route, so
// reasoning from WSL's own routing table gets it wrong every time.
//
// Every query goes through Runner. That is deliberate and load-bearing: it is
// what lets the rest of the module be tested in CI, where no powershell.exe
// exists. Nothing outside this package may shell out to Windows.
package winhost

import "context"

// scriptKind names a query so a test double can answer it without matching on
// PowerShell source text.
type scriptKind string

const (
	kindDNSServers scriptKind = "dns-servers"
	kindAdapters   scriptKind = "adapters"
)

// script is one PowerShell query: its kind (what a fake keys on) and the source
// (what the real runner executes).
type script struct {
	kind   scriptKind
	source string
}

// Runner executes a PowerShell query and returns its raw stdout.
//
// This is the frozen seam of the module — see the plan's §3. Implementations:
// PowerShell (real) and the test doubles.
type Runner interface {
	Run(ctx context.Context, s script) ([]byte, error)
}

// Interface is one Windows network interface as wlink cares about it.
//
// Note that an interface with no resolvers is still reported. "Present with no
// resolver" and "absent" are different facts, and only the caller can decide
// what they mean.
type Interface struct {
	Alias      string   `json:"alias"`
	Index      int      `json:"index"`
	DNSServers []string `json:"dns_servers"`
	IsTunnel   bool     `json:"is_tunnel"`
}

// Host queries the Windows side through a Runner.
type Host struct{ r Runner }

// New returns a Host backed by the given Runner.
func New(r Runner) *Host { return &Host{r: r} }
