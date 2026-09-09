// Package sshconf reads and edits ~/.ssh/config, which is the fleet's only
// inventory (spec F2, F9, F10). Everything here is a pure function over the
// config text: no filesystem, no network, no clock — so the whole surface is
// unit-testable, which matters because a bad write costs SSH access to every
// machine.
package sshconf

import "strings"

// Host is one concrete (non-pattern) Host block.
type Host struct {
	Alias, HostName, User, Port, Identity string
	Fleet                                 bool // block carries the fleet marker
}

// hasMarker reports whether a comment carries the marker as a whole token.
//
// Both "# fleet" and "#fleet" must count — the docs write it one way and the
// --marker default the other — so the leading '#' is stripped from both sides
// before comparing. Comparison stays token-based, never substring: plain
// Contains would make "# fleetwood" match "#fleet".
func hasMarker(comment, marker string) bool {
	want := strings.TrimLeft(marker, "#")
	if want == "" {
		return false
	}
	for _, f := range strings.Fields(strings.TrimLeft(strings.TrimSpace(comment), "#")) {
		if strings.TrimRight(strings.TrimLeft(f, "#"), ",;") == want {
			return true
		}
	}
	return false
}

// splitComment separates a config line into its directive and trailing comment.
func splitComment(line string) (code, comment string) {
	if i := strings.Index(line, "#"); i >= 0 {
		return strings.TrimSpace(line[:i]), line[i:]
	}
	return strings.TrimSpace(line), ""
}

// isHostLine reports whether the trimmed line opens a Host block.
func isHostLine(t string) bool {
	f := strings.Fields(t)
	return len(f) >= 2 && strings.EqualFold(f[0], "Host")
}

// Parse returns every concrete Host block in cfg. Pattern blocks (containing
// * or ?) are omitted entirely — they configure defaults, not machines.
// Fleet is true when the marker appears on the Host line or on any comment
// line inside the block.
func Parse(cfg, marker string) ([]Host, error) {
	var out []Host
	var cur *Host

	flush := func() {
		if cur != nil && !strings.ContainsAny(cur.Alias, "*?") {
			out = append(out, *cur)
		}
		cur = nil
	}

	for _, line := range strings.Split(cfg, "\n") {
		t := strings.TrimSpace(line)
		code, comment := splitComment(t)

		if isHostLine(code) {
			flush()
			alias := strings.Fields(code)[1]
			cur = &Host{Alias: alias, Fleet: hasMarker(comment, marker)}
			continue
		}
		if cur == nil {
			continue
		}
		if code == "" {
			if hasMarker(comment, marker) {
				cur.Fleet = true
			}
			continue
		}
		f := strings.Fields(code)
		if len(f) < 2 {
			continue
		}
		switch strings.ToLower(f[0]) {
		case "hostname":
			cur.HostName = f[1]
		case "user":
			cur.User = f[1]
		case "port":
			cur.Port = f[1]
		case "identityfile":
			cur.Identity = f[1]
		}
	}
	flush()
	return out, nil
}
