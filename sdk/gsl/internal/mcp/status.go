package mcp

import (
	"strings"
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
	// StateNotConfigured — listed by the CLI but not set up (e.g. a plugin or
	// claude.ai connector the user never configured). Not a failure: it is
	// counted in Servers only, never in Connected/Failed/NeedsAuth. The CLI
	// added this after the plan was written (seen in the 2026-10 capture).
	StateNotConfigured
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
	case StateNotConfigured:
		return "not configured"
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
	ScopeLocal         // ~/.claude.json → .projects[cwd].mcpServers (Claude Code's "local")
	ScopeProject       // <cwd>/.mcp.json, once approved (Claude Code's "project")
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
	// RefreshDue is true when the entry is stale (or missing) AND the negative
	// cache's backoff allows another probe: the caller should start an
	// out-of-band refresh (SpawnRefresh). False while a failure backs off, so
	// a broken `claude` does not respawn on every render.
	RefreshDue bool
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

	// Fixed, when non-nil, is returned by Get as-is with no file access at
	// all: the hermetic preview and golden fixtures.
	Fixed *Status

	// Force makes Refresh probe even while a failure is backing off (the
	// explicit `gsl mcp refresh` verb); the single-flight lock still applies.
	Force bool
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
	var s Status
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		i := strings.LastIndex(line, " - ")
		if i < 0 {
			continue // "Checking MCP server health…", blanks, banners
		}
		left, field := line[:i], strings.TrimSpace(line[i+3:])
		name := left
		if j := strings.Index(left, ": "); j >= 0 {
			name = left[:j]
		}
		name = strings.TrimSpace(name)
		if name == "" || field == "" {
			continue
		}
		st := classify(field)
		s.Servers = append(s.Servers, ServerStatus{Name: name, State: st})
		switch st {
		case StateConnected:
			s.Connected++
		case StateFailed:
			s.Failed++
		case StateNeedsAuth:
			s.NeedsAuth++
		}
	}
	return s
}

// classify maps a status field to a State by keyword, case-insensitively.
// Order matters: "not configured" and "not connected" contain other keywords,
// and "failed to connect" must not read as connecting.
func classify(field string) State {
	f := strings.ToLower(field)
	switch {
	case strings.Contains(f, "not configured"):
		return StateNotConfigured
	case strings.Contains(f, "needs authentication"), strings.Contains(f, "needs auth"):
		return StateNeedsAuth
	case strings.Contains(f, "failed"), strings.Contains(f, "not connected"), strings.Contains(f, "error"):
		return StateFailed
	case strings.Contains(f, "connecting"):
		return StateConnecting
	case strings.Contains(f, "connected"):
		return StateConnected
	}
	return StateUnknown
}
