package cmd

import (
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshfail"
)

// filterHosts narrows a fleet to the named aliases. An empty filter means every
// host, so existing callers keep their behaviour exactly.
func filterHosts(all []sshconf.Host, only []string) []sshconf.Host {
	if len(only) == 0 {
		return all
	}
	want := make(map[string]bool, len(only))
	for _, a := range only {
		want[a] = true
	}
	out := make([]sshconf.Host, 0, len(only))
	for _, h := range all {
		if want[h.Alias] {
			out = append(out, h)
		}
	}
	return out
}

// checkHosts is filterHosts that refuses an alias the fleet does not have.
// Silently ignoring a typo would render as a successful sync that in fact
// authorized nothing.
func checkHosts(all []sshconf.Host, only []string) ([]sshconf.Host, error) {
	have := make(map[string]bool, len(all))
	for _, h := range all {
		have[h.Alias] = true
	}
	for _, a := range only {
		if !have[a] {
			return nil, fmt.Errorf("--host %q is not a fleet host", a)
		}
	}
	return filterHosts(all, only), nil
}

// bootstrapNeeded lists hosts a key sync cannot help because they ANSWERED and
// refused our credential.
//
// Authorizing a key means APPENDING to that host's authorized_keys, which
// requires the access we are trying to establish. An auth-failed host must
// therefore be reported as needing manual bootstrap (ssh-copy-id with a
// password, or console access) rather than left to look like a sync that will
// fix itself.
//
// An UNREACHABLE host is deliberately NOT here. It opened no SSH session at
// all, so the fault is the network, not the key — ssh-copy-id cannot help a
// host you cannot connect to, and offering it sent a real investigation to the
// wrong layer. Those hosts are named by unreachableHint instead.
func bootstrapNeeded(rows []Row) []string {
	var out []string
	for _, r := range rows {
		if sshfail.IsHostKey(r.Note) {
			continue // a known_hosts fault, not a missing key: see hostKeyNeeded
		}
		if r.Class == string(drift.AuthFailed) {
			out = append(out, r.Alias)
		}
	}
	return out
}

// unreachableHosts lists hosts that opened no SSH session — a network fact,
// repaired by waking/powering/wiring the host, never by a key.
func unreachableHosts(rows []Row) []string {
	var out []string
	for _, r := range rows {
		if r.Class == string(drift.Unreachable) {
			out = append(out, r.Alias)
		}
	}
	return out
}

// bootstrapHint is the advice printed under the status table, one paragraph
// per fault kind, each naming only the hosts it can actually help:
//   - host-key faults → `fleet trust` (hostKeyHint);
//   - auth-failed hosts → manual key bootstrap, ssh-copy-id or one interactive
//     session for a password host (keyBootstrapHint) — authorizing a key needs
//     the access being established, so fleet cannot do it for you;
//   - unreachable hosts → a NETWORK fault, never a key (unreachableHint).
//
// powerSave is the lazy local Wi-Fi power-save advisory; it is only invoked
// when an unreachable host gives it somewhere to go, because it forks `iw`.
// wakeTried says whether `fleet status` already ran the wake ladder, so the
// advice never repeats a step that just failed.
func bootstrapHint(rows []Row, powerSave func() string, wakeTried bool) string {
	return hostKeyHint(rows) + keyBootstrapHint(rows) + unreachableHint(rows, powerSave, wakeTried)
}

// unreachableHint names the hosts no SSH session reached and says plainly that
// this is a network fault, not a key one — the opposite of what the old shared
// bootstrap sentence told an operator. The power-save note (empty unless THIS
// workstation's own Wi-Fi is asleep) is appended because a sleeping local radio
// is a common cause: a cold peer's broadcast ARP to us is dropped, so the host
// is live yet unreachable from here.
func unreachableHint(rows []Row, powerSave func() string, wakeTried bool) string {
	hosts := unreachableHosts(rows)
	if len(hosts) == 0 {
		return ""
	}
	wake := "or asleep at layer 2 — `fleet wake <alias>` nudges the last case."
	if wakeTried {
		wake = "or asleep — this run already tried the wake ladder, so check power and\n" +
			"  cabling (or rerun with a longer --wake-timeout)."
	}
	note := ""
	if powerSave != nil {
		note = powerSave()
	}
	return fmt.Sprintf("\n%d host(s) unreachable — no SSH session opened: %s\n"+
		"  this is a NETWORK fault, not a key — no credential fix applies to a host\n"+
		"  you cannot open a connection to. The box may be powered off, off the\n"+
		"  network, %s\n",
		len(hosts), strings.Join(hosts, ", "), wake) + note
}

// hostKeyHint routes host-key faults to `fleet trust`. ssh-copy-id cannot help
// them: it connects through the same known_hosts and is refused the same way.
func hostKeyHint(rows []Row) string {
	var b strings.Builder
	for _, r := range rows {
		if r.Class != string(drift.AuthFailed) || !sshfail.IsHostKey(r.Note) {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("\nhost key not trusted — compare fingerprints, then accept (backed up + logged; or T in `fleet tui`):\n")
		}
		fmt.Fprintf(&b, "  fleet trust %-20s # %s\n", r.Alias, r.Note)
	}
	return b.String()
}

func keyBootstrapHint(rows []Row) string {
	need := bootstrapNeeded(rows)
	if len(need) == 0 {
		return ""
	}
	return fmt.Sprintf("\n%d host(s) cannot be reached with your key: %s\n"+
		"  authorize it with `ssh-copy-id <alias>` (or press A in `fleet tui`) —\n"+
		"  `fleet keys sync` cannot help: appending to a remote authorized_keys\n"+
		"  needs the access it is trying to establish.\n"+
		"  if the host wants a password rather than a key, open `fleet tui` and\n"+
		"  press `s` once: no BatchMode probe can ever answer a password prompt,\n"+
		"  so that session is the only thing that can prime the shared connection.\n",
		len(need), strings.Join(need, ", "))
}
