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
	reason    string // what a row may say about a NETWORK failure; see Reason
}{
	{"REMOTE HOST IDENTIFICATION HAS CHANGED", "host key CHANGED", Auth, ""},
	{"Host key verification failed", "host key unverified", Auth, ""},
	{"Permission denied", "permission denied", Auth, ""},
	{"Too many authentication failures", "permission denied", Auth, ""},
	{"Connection refused", "", Network, "refused the connection"},
	{"Connection timed out", "", Network, "timed out"},
	{"Connection closed by", "", Network, "closed the connection"},
	{"Connection reset", "", Network, "reset the connection"},
	{"No route to host", "", Network, "no route to host"},
	{"Network is unreachable", "", Network, "network unreachable"},
	{"Could not resolve hostname", "", Network, "name did not resolve"},
	{"Operation timed out", "", Network, "timed out"},
}

// match is the single scan both exported functions share, so a signature can
// never classify one way and annotate another.
func match(err error) (Kind, string) {
	k, n, _ := matchAll(err)
	return k, n
}

func matchAll(err error) (Kind, string, string) {
	s := stderrOf(err)
	if s == "" {
		return Unknown, "", ""
	}
	for _, m := range markers {
		if strings.Contains(s, m.sig) {
			return m.kind, m.note, m.reason
		}
	}
	return Unknown, "", ""
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

// Reason names WHICH network failure it was — a refusal, a timeout and a
// closed session point at different causes (sshd down, a slow handshake past
// ConnectTimeout, MaxStartups or a ban). Empty for anything that is not a
// recognised network failure; Note stays the qualifier for auth faults.
func Reason(err error) string { _, _, r := matchAll(err); return r }

// IsHostKey reports whether a Note is a host-key fault. Those are repaired on
// THIS machine (known_hosts, via `fleet trust`); every other auth note needs a
// change on the remote one (authorized_keys), so callers route on this split.
func IsHostKey(note string) bool { return strings.HasPrefix(note, "host key ") }

// AuthMethods is the list ssh prints after a refused login —
// `Permission denied (publickey,password).` yields [publickey password]. It is
// how a scan tells "this host only takes keys" from "this host would take a
// password", which decides whether ssh-copy-id can bootstrap it at all. Nil
// when the stderr carries no such line.
func AuthMethods(err error) []string {
	s := stderrOf(err)
	_, rest, ok := strings.Cut(s, "Permission denied (")
	if !ok {
		return nil
	}
	list, _, ok := strings.Cut(rest, ")")
	if !ok || list == "" {
		return nil
	}
	return strings.Split(list, ",")
}
