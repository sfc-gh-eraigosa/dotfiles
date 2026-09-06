// Package drift classifies how far a host's installed dotfiles commit has
// drifted from the baseline, and formats ages for display (spec F4).
//
// `now` is always injected — the package never calls time.Now() — so age
// formatting is deterministically testable.
package drift

import (
	"fmt"
	"time"
)

type Class string

const (
	UpToDate    Class = "up-to-date"
	Behind      Class = "behind"
	Divergent   Class = "ahead/divergent"
	Unknown     Class = "unknown"
	Unreachable Class = "unreachable"
	// AuthFailed is a host that ANSWERED and then refused us — an unknown or
	// changed host key, or no accepted credential. It is deliberately not
	// Unreachable: the machine is up and listening, and the fix is local
	// (known_hosts, a key) rather than on the network.
	AuthFailed Class = "auth-failed"
)

// Input is everything known about one host after the SSH probe.
type Input struct {
	Reachable, HaveStamp, IsAncestor bool
	// AuthFailed qualifies a failed probe: ssh got a session and was turned
	// away. Meaningless when Reachable is true.
	AuthFailed       bool
	Commit, Baseline string
	BehindCount      int
}

type Result struct {
	Class  Class
	Behind int
}

// Classify decides a host's state. Order matters: an unreachable host is
// never reported as up-to-date, and a host with no stamp is Unknown rather
// than assumed current — silence is not success.
//
// AuthFailed is tested before Unreachable because it is strictly more
// specific: both mean "no answer to work with", but only one tells the
// operator the machine is alive and the fix is local. It is deliberately
// checked only when the probe failed, so a stale flag can never mask a host
// that actually answered.
func Classify(in Input) Result {
	switch {
	case !in.Reachable && in.AuthFailed:
		return Result{Class: AuthFailed}
	case !in.Reachable:
		return Result{Class: Unreachable}
	case !in.HaveStamp:
		return Result{Class: Unknown}
	case in.Commit == in.Baseline:
		return Result{Class: UpToDate}
	case !in.IsAncestor:
		// The stamped commit is not in the baseline's history: a local build,
		// a rewritten branch, or a machine ahead of origin.
		return Result{Class: Divergent}
	default:
		return Result{Class: Behind, Behind: in.BehindCount}
	}
}

// FormatAge renders a coarse relative age. A zero `then` renders "-" so a
// host with no known install time never displays a fabricated duration.
func FormatAge(now, then time.Time) string {
	if then.IsZero() {
		return "-"
	}
	d := now.Sub(then)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	default:
		return fmt.Sprintf("%dw ago", int(d.Hours())/(24*7))
	}
}
