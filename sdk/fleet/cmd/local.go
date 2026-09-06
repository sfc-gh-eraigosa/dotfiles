package cmd

import (
	"net"
	"os"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// localGlyph prefixes the header badge. Colour alone must not be the only
// cue that says "you are here"; the glyph carries it on a mono terminal and
// for anyone who cannot separate the palette.
const localGlyph = "⌂"

// localColor is the "this is the machine you are typing on" colour. It is
// deliberately NOT one of the drift classes' colours (green/yellow/magenta/
// red/orange) — location is a different question from status, and reusing a
// status colour for it would make "up to date" and "this is me" look alike.
const localColor = "213"

// localHost is who THIS machine is, in the only two terms an ssh config can
// be matched against: the name it calls itself, and the addresses it answers
// on. Both are captured once at startup and then treated as data, so every
// matching decision below is a pure function.
type localHost struct {
	Name string   // os.Hostname(), FQDN or short as the machine reports it
	IPs  []string // every non-loopback interface address, v4 and v6
}

// detectLocal is the single impure edge: it asks the OS who we are. Callers
// hold the result and pass it to isLocalHost, which stays testable.
func detectLocal() localHost {
	var me localHost
	if n, err := os.Hostname(); err == nil {
		me.Name = strings.TrimSpace(n)
	}
	me.IPs = localInterfaceIPs()
	return me
}

// localInterfaceIPs lists this machine's own addresses. Loopback interfaces are
// skipped on purpose: 127.0.0.1 is matched by isLocalHost's own loopback
// rule, which is broader (it also covers `localhost` and 127.0.1.1) and does
// not depend on which loopback aliases happen to be configured.
func localInterfaceIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.IsLinkLocalUnicast() || n.IP.IsLinkLocalMulticast() {
				continue
			}
			out = append(out, n.IP.String())
		}
	}
	return out
}

// isLocalHost reports whether this fleet entry IS the machine fleet is running
// on. It answers by ADDRESS and by NAME, and either is enough, because each
// covers the other's blind spot: a DHCP move makes a configured HostName stale
// while the alias still names the box, and an alias that is a nickname says
// nothing about the box while the address still resolves to it.
//
// Deliberately no DNS: this runs on every frame's worth of state at startup,
// and a resolver hang would stall the dashboard for a cosmetic highlight. A
// name that needs resolving simply does not match.
func isLocalHost(h sshconf.Host, me localHost) bool {
	// ssh dials the alias when the block sets no HostName, so with none the
	// alias IS the address.
	target := strings.TrimSpace(h.HostName)
	if target == "" {
		target = h.Alias
	}
	if isLoopbackTarget(target) {
		return true
	}
	if ip := net.ParseIP(target); ip != nil {
		for _, a := range me.IPs {
			if other := net.ParseIP(a); other != nil && other.Equal(ip) {
				return true
			}
		}
	} else if sameMachineName(target, me.Name) {
		return true
	}
	// The ALIAS is asked separately, and last. A literal HostName that did not
	// match our addresses is not a verdict: that is exactly the stale-DHCP case
	// — `Host <our-name>` with a HostName the router reassigned last week — and
	// returning false there left the operator's own machine unmarked.
	return sameMachineName(h.Alias, me.Name)
}

// isLoopbackTarget covers both spellings of "back at this machine": a literal
// loopback address (127.0.0.0/8, ::1) and the `localhost` name, which is the
// form an ssh config usually carries and which ParseIP alone never catches.
func isLoopbackTarget(s string) bool {
	if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(shortName(s), "localhost")
}

// sameMachineName compares two host names as machines, not as strings: case
// is insignificant to DNS, and a short name and its FQDN are one host.
func sameMachineName(a, b string) bool {
	a, b = shortName(a), shortName(b)
	return a != "" && strings.EqualFold(a, b)
}

// shortName is the label before the first dot — the machine, without its
// domain.
func shortName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i > 0 {
		s = s[:i]
	}
	return s
}
