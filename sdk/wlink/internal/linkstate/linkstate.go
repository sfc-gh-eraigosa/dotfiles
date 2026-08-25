// Package linkstate is the composed answer to "can this WSL box reach its
// fleet by name, and if not, why?".
//
// State is a PUBLISHED CONTRACT: `wlink status --json` emits it, gsl renders it
// in the status line, and CI gates on it. Renaming a field or changing a wire
// value is a breaking change to those consumers, so the shape is pinned by test
// against the spec's §4.1 worked example rather than left to drift.
package linkstate

// TunnelState is the closed set of tunnel conditions.
//
// The distinction that matters most is not-ready vs down. Windows publishes a
// VPN adapter and its DNS server the moment you click connect — seconds before
// the handshake completes — and the previous network is already unroutable by
// then. Probing in that window finds nothing on every candidate, which looks
// identical to "no tunnel" unless the two are named separately.
type TunnelState string

const (
	TunnelUp       TunnelState = "up"
	TunnelNotReady TunnelState = "not-ready"
	TunnelDown     TunnelState = "down"
	TunnelUnknown  TunnelState = "unknown"
)

// Health is the one-word verdict, and drives the exit code (spec §3): ok → 0,
// degraded → 1.
type Health string

const (
	HealthOK       Health = "ok"
	HealthDegraded Health = "degraded"
)

// Tunnel describes the VPN/tunnel carrying the link.
type Tunnel struct {
	State     TunnelState `json:"state"`
	Interface string      `json:"interface"`
	// HandshakeAgeSeconds is nil when unobservable. A pointer rather than an
	// int because a plain 0 would read as "just handshaked" — precisely the
	// wrong impression — and absences are null everywhere else in this schema.
	HandshakeAgeSeconds *int `json:"handshake_age_seconds"`
}

// Pin describes the resolver wlink has pinned. Nil means nothing is pinned.
type Pin struct {
	Resolver string `json:"resolver"`
	Since    string `json:"since,omitempty"`
	// Managed is false when resolv.conf names this resolver but wlink did not
	// write it — someone else's pin, which wlink must not claim credit for or
	// silently take over.
	Managed bool `json:"managed"`
}

// Candidate is one resolver wlink considered.
//
// Reachable and FleetResolved are deliberately separate: a resolver that
// answers but does not know the fleet is a different situation from one that
// says nothing at all, and only the first distinguishes "wrong tunnel" from
// "tunnel not ready".
type Candidate struct {
	Server        string `json:"server"`
	Reachable     bool   `json:"reachable"`
	FleetResolved int    `json:"fleet_resolved"`
	Recursive     bool   `json:"recursive"`
}

// Fleet summarizes name resolution across the fleet.
type Fleet struct {
	Total    int `json:"total"`
	Resolved int `json:"resolved"`
	// ExcludedByHostsFile lists names /etc/hosts already answers. They are not
	// probed (nsswitch is "files dns", so no resolver is ever asked) and must
	// not count against the score.
	ExcludedByHostsFile []string `json:"excluded_by_hosts_file"`
}

// Drift reports a managed file changed since wlink wrote it.
type Drift struct {
	File   string `json:"file"`
	Detail string `json:"detail"`
}

// State is the whole answer. Link is computed by Health; it is carried as a
// field so a consumer reads one value instead of re-deriving the rules.
type State struct {
	WSL        bool        `json:"wsl"`
	Link       Health      `json:"link"`
	Tunnel     Tunnel      `json:"tunnel"`
	Pinned     *Pin        `json:"pinned"`
	Candidates []Candidate `json:"candidates"`
	Fleet      Fleet       `json:"fleet"`
	Drift      *Drift      `json:"drift"`
}

// Health derives the verdict.
//
// It asks ONE question: can this machine reach its fleet by name? Tunnel state
// and pin status are reported alongside, but are deliberately NOT inputs —
// a machine sitting directly on the fleet's LAN resolves everything with no
// tunnel and nothing pinned, and reporting that as degraded would be the tool
// crying wolf about a link that plainly works. (Found by running status on a
// real machine that was healthy in every way that matters and still reported
// DEGRADED because no tunnel was up.)
//
// Off WSL the tool does not apply, so it reports ok — a no-op, not a failure.
func (s State) Health() Health {
	if !s.WSL {
		return HealthOK
	}
	switch {
	case s.Drift != nil,
		// A zero-size fleet is not a shortfall — there is nothing to resolve.
		s.Fleet.Total > 0 && s.Fleet.Resolved < s.Fleet.Total:
		return HealthDegraded
	}
	return HealthOK
}

// Normalized returns a copy safe to marshal: Link computed, and nil slices
// replaced with empty ones so consumers never have to nil-check before ranging.
func (s State) Normalized() State {
	s.Link = s.Health()
	if s.Candidates == nil {
		s.Candidates = []Candidate{}
	}
	if s.Fleet.ExcludedByHostsFile == nil {
		s.Fleet.ExcludedByHostsFile = []string{}
	}
	return s
}
