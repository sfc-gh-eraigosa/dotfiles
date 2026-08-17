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
)

// Input is everything known about one host after the SSH probe.
type Input struct {
	Reachable, HaveStamp, IsAncestor bool
	Commit, Baseline                 string
	BehindCount                      int
}

type Result struct {
	Class  Class
	Behind int
}

// Classify decides a host's state. Order matters: an unreachable host is
// never reported as up-to-date, and a host with no stamp is Unknown rather
// than assumed current — silence is not success.
func Classify(in Input) Result {
	switch {
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
