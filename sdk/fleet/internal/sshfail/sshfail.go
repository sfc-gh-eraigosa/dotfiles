// Package sshfail tells apart the two ways an SSH probe fails, because they
// send an operator to different machines.
//
// A probe that never reached the host (refused, timed out, unresolvable) is a
// network fact. A probe that CONNECTED and was then refused on trust or
// credentials is not: the host is up, listening, and answering. Collapsing the
// second into "unreachable" is what sent a real investigation to the network
// layer for what was a one-line known_hosts gap — fleet runs every probe with
// BatchMode=yes, so an unknown host key cannot prompt and fails instantly,
// looking exactly like a dead machine.
//
// The evidence is ssh's stderr, which (*exec.Cmd).Output() already captures
// into *exec.ExitError. Nothing here opens a socket.
package sshfail

import (
	"errors"
	"os/exec"
	"strings"
)

// Kind is what the failure actually was.
type Kind string

const (
	// Unknown is the honest answer when there is no stderr to read — every
	// fake runner in the unit tests lands here, and callers must treat it
	// exactly as they treated a failure before this package existed.
	Unknown Kind = "unknown"
	// Network means the session never got established.
	Network Kind = "network"
	// Auth means ssh connected and we were turned away.
	Auth Kind = "auth"
)

// markers maps a stderr signature to its kind and operator-facing note.
//
// Order matters: a CHANGED host key prints the "Host key verification failed"
// line as well, so the more specific — and far more alarming — signature has
// to be tested first or a possible MITM would render as a routine unknown key.
var markers = []struct {
	sig, note string
	kind      Kind
}{
	{"REMOTE HOST IDENTIFICATION HAS CHANGED", "host key CHANGED", Auth},
	{"Host key verification failed", "host key unverified", Auth},
	{"Permission denied", "permission denied", Auth},
	{"Too many authentication failures", "permission denied", Auth},
	{"Connection refused", "", Network},
	{"Connection timed out", "", Network},
	{"Connection closed by", "", Network},
	{"No route to host", "", Network},
	{"Network is unreachable", "", Network},
	{"Could not resolve hostname", "", Network},
	{"Operation timed out", "", Network},
}

// match is the single scan both exported functions share, so a signature can
// never classify one way and annotate another.
func match(err error) (Kind, string) {
	s := stderrOf(err)
	if s == "" {
		return Unknown, ""
	}
	for _, m := range markers {
		if strings.Contains(s, m.sig) {
			return m.kind, m.note
		}
	}
	return Unknown, ""
}

// stderrOf recovers what the ssh process wrote. A plain error carries none.
func stderrOf(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return string(ee.Stderr)
	}
	return ""
}

// Classify reports which layer failed. A nil error is Unknown, never a verdict.
func Classify(err error) Kind { k, _ := match(err); return k }

// Note is the short qualifier for the row: which auth fault, since the fixes
// differ (ssh-keygen -R versus authorizing a key).
func Note(err error) string { _, n := match(err); return n }
