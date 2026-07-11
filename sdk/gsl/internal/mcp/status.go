package mcp

import (
	"context"
	"time"
)

// status.go — the FROZEN MCP status contract (gsl-ultra, plan §3).
//
// Everything downstream (internal/render, internal/tui, cmd) compiles against
// the types and signatures in this file. The IMPLEMENTATIONS land in the `mcp`
// leaf; this file exists so the dependent leaves can be built in parallel
// against a contract that will not move under them (Interface-First).
//
// # Why this replaces ActiveCount/ConfiguredCount
//
// The shipped design collapsed MCP to a single integer via ActiveCount, which
// was unconditionally 0 in production for two independent reasons:
//
//  1. parseConnectedCount matched the literal "✓ Connected" (U+2713 CHECK
//     MARK), but `claude mcp list` emits U+2714 HEAVY CHECK MARK ("✔"). A
//     codepoint census of real output: 6× U+2714, 5× U+2718, ZERO U+2713.
//  2. The probe ran inline under a 500 ms deadline against a command measured
//     at 3.45–4.10 s (it dials every server), so it was SIGKILLed every render
//     and the cache was never even seeded.
//
// The contract below fixes the CLASS of bug, not just the instance:
//
//   - ParseStatus keys on STATUS WORDS, never on a decorated sigil, so a glyph
//     change in a future CLI release cannot silently zero the count.
//   - Get is CACHE-ONLY and must never spawn a subprocess, so a render can
//     never stall on a multi-second probe.
//   - Refresh is the only thing that shells out, and it runs OUT OF BAND.
type State int

const (
	// StateUnknown is the zero value: the server's state could not be
	// determined (an unparseable line, or a server we know of from config but
	// have never probed).
	StateUnknown State = iota
	// StateConnected — the server responded to a health check.
	StateConnected
	// StateFailed — the server is configured but did not connect.
	StateFailed
	// StateNeedsAuth — the server requires authentication the user has not
	// completed. Distinct from Failed: it is actionable by the user.
	StateNeedsAuth
	// StateConnecting — the health check was still in flight.
	StateConnecting
)

// String renders the state as the lowercase keyword the CLI itself uses.
func (s State) String() string {
	switch s {
	case StateConnected:
		return "connected"
	case StateFailed:
		return "failed"
	case StateNeedsAuth:
		return "needs authentication"
	case StateConnecting:
		return "connecting"
	default:
		return "unknown"
	}
}

// Scope is where a server definition came from. Precedence for de-duplication
// is Local > Project > User > Plugin: a server defined in more than one scope
// is counted ONCE, at its highest-precedence scope.
//
// The shipped ConfiguredCount SUMMED the scopes with no de-duplication (its own
// doc comment conceded the result was "an upper bound"), so a server present
// both globally and in .mcp.json was counted twice.
type Scope int

const (
	ScopeUnknown Scope = iota
	ScopeLocal         // <cwd>/.mcp.json
	ScopeProject       // ~/.claude.json → .projects[cwd].mcpServers
	ScopeUser          // ~/.claude.json → .mcpServers
	ScopePlugin        // ~/.claude/plugins/cache/**/.mcp.json (gated by enabledPlugins)
)

// ServerStatus is one MCP server's identity and health.
type ServerStatus struct {
	Name  string
	State State
	Scope Scope
}

// Status is the whole MCP picture for one working directory.
//
// Connected/Failed/NeedsAuth are derived from a probe (Refresh); Configured is
// the DE-DUPLICATED denominator derived from the config files. They can
// legitimately disagree — a server can be configured but never probed — which
// is why Configured is not simply len(Servers).
type Status struct {
	Connected  int
	Failed     int
	NeedsAuth  int
	Configured int

	Servers []ServerStatus

	// FetchedAt is when the underlying probe ran. Zero when the status was
	// synthesized from config alone (never probed).
	FetchedAt time.Time
	// Stale is true when this Status was served from an expired cache entry.
	// Callers SHOULD still render it — a stale count beats no count — and MAY
	// signal staleness in the UI. Renderers must not block to refresh it.
	Stale bool
}

// Empty reports whether there is nothing worth rendering: no servers
// configured and none in any probed state.
func (s Status) Empty() bool {
	return s.Configured == 0 && s.Connected == 0 && s.Failed == 0 && s.NeedsAuth == 0
}

// Options configures Get and Refresh. The zero value uses system defaults.
type Options struct {
	// CacheDir overrides the default cache directory
	// (${XDG_CACHE_HOME:-$HOME/.cache}/gsl/mcp).
	//
	// The cache is keyed BY CWD (sha256(cwd)[:16]). The shipped cache used a
	// single global path (~/.cache/gsl/mcp.json) to hold a cwd-DEPENDENT value,
	// so a count computed in repo A was served to repo B.
	CacheDir string

	// TTL overrides how long a cache entry is considered fresh.
	TTL time.Duration

	// Now, if non-nil, replaces time.Now (tests).
	Now func() time.Time
}

// ParseStatus extracts the health picture from `claude mcp list` output.
//
// PURE: no I/O, no clock, no environment. Given bytes, it returns a Status.
//
// It MUST classify by STATUS KEYWORD, matched case-insensitively against the
// field after the LAST " - " on each line — "connected", "failed",
// "needs authentication", "connecting". Decorated sigils (U+2713 ✓, U+2714 ✔,
// U+2705 ✅, U+2718 ✘, U+2717 ✗) may be used only as a SECONDARY signal, never
// as the primary discriminator. Matching a sigil is precisely what broke the
// shipped parser.
//
// Lines that are not status lines contribute nothing. Configured is NOT set by
// ParseStatus (it comes from the config scan) — only Connected/Failed/
// NeedsAuth/Servers are.
func ParseStatus(out []byte) Status {
	// Implemented in the `mcp` leaf.
	return Status{}
}

// Get returns the MCP status for cwd from the CACHE ONLY.
//
// CONTRACT — this is the load-bearing invariant of the whole design:
//
//	Get MUST NEVER spawn a subprocess, and MUST NEVER block on the network.
//
// It is called on the render hot path, once per assistant turn. `claude mcp
// list` takes 3.4–4.1 s because it dials every server; there is no timeout at
// which running it inline is acceptable.
//
// A fresh entry is returned with Stale=false. An EXPIRED entry is still
// returned, with Stale=true — a slightly-old count is strictly better than no
// count — and the caller SHOULD trigger an out-of-band Refresh. A cold cache
// returns a Status carrying only the config-derived Configured count, with
// Stale=true.
func Get(ctx context.Context, cwd string, o Options) (Status, error) {
	// Implemented in the `mcp` leaf.
	return Status{}, nil
}

// Refresh runs the probe OUT OF BAND and rewrites the cwd-keyed cache entry.
//
// This is the ONLY function in the package that shells out. It is invoked by
// `gsl mcp refresh` (which the render path forks in a DETACHED process when it
// observes a stale entry) — never synchronously from a render.
//
// Refresh MUST be single-flight (a lockfile in the cache dir): concurrent gsl
// processes across panes must not stampede N copies of a 4-second probe.
// Failures MUST be cached NEGATIVELY with exponential backoff, so a broken
// `claude` binary does not cause a respawn on every keystroke.
func Refresh(ctx context.Context, r Runner, cwd string, o Options) error {
	// Implemented in the `mcp` leaf.
	return nil
}

// ConfiguredServers returns the DE-DUPLICATED set of servers configured for
// cwd, resolved across every scope.
//
// Rules (plan F6):
//   - De-duplicate by server NAME; a name in several scopes resolves to its
//     highest-precedence scope (Local > Project > User > Plugin).
//   - Include plugin servers from ~/.claude/plugins/cache/**/.mcp.json, gated
//     by `enabledPlugins` in ~/.claude/settings.json.
//   - SUBTRACT names listed in `disabledMcpjsonServers`.
//   - Honour `enableAllProjectMcpServers`.
//
// A missing config file contributes nothing and is NOT an error.
func ConfiguredServers(cwd string) ([]ServerStatus, error) {
	// Implemented in the `mcp` leaf.
	return nil, nil
}
