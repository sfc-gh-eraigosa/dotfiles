package cmd

import (
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
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

// bootstrapNeeded lists hosts a key sync cannot help.
//
// Authorizing a key means APPENDING to that host's authorized_keys, which
// requires the access we are trying to establish. A host that refuses us — or
// does not answer — must therefore be reported as needing manual bootstrap
// (ssh-copy-id with a password, or console access) rather than left to look
// like a sync that will fix itself.
func bootstrapNeeded(rows []Row) []string {
	var out []string
	for _, r := range rows {
		if r.Class == string(drift.AuthFailed) || r.Class == string(drift.Unreachable) {
			out = append(out, r.Alias)
		}
	}
	return out
}

// bootstrapHint turns bootstrapNeeded into the sentence an operator needs.
//
// A row reading auth-failed says the credential was refused; it does not say
// that fleet cannot repair that for you. Authorizing a key means appending to
// the host's authorized_keys, which requires the very access being
// established, so the fix is necessarily manual. Saying so is the honest
// answer, and saying nothing is how someone waits for a sync that can never
// come.
func bootstrapHint(rows []Row) string {
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
