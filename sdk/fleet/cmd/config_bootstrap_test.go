package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/drift"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// A pull that adds hosts should be able to authorize just those, not re-push
// keys to the entire fleet.
func TestFilterHostsRestrictsToNamedAliases(t *testing.T) {
	all := []sshconf.Host{{Alias: "a"}, {Alias: "b"}, {Alias: "c"}}
	got := filterHosts(all, []string{"a", "c"})
	if len(got) != 2 || got[0].Alias != "a" || got[1].Alias != "c" {
		t.Fatalf("filterHosts = %+v", got)
	}
	if all := filterHosts(all, nil); len(all) != 3 {
		t.Fatalf("no filter must mean every host, got %+v", all)
	}
}

// Silently ignoring a typo'd alias would look like a successful sync that
// authorized nothing.
func TestCheckHostsRejectsAnUnknownAlias(t *testing.T) {
	if _, err := checkHosts([]sshconf.Host{{Alias: "a"}}, []string{"nope"}); err == nil {
		t.Fatal("want an error naming the unknown alias")
	}
	if _, err := checkHosts([]sshconf.Host{{Alias: "a"}}, []string{"a"}); err != nil {
		t.Fatalf("a known alias must be accepted, got %v", err)
	}
}

// keys sync APPENDS to a remote authorized_keys, so it needs access it does not
// have on a host that refuses us. Claiming otherwise would be a lie.
func TestHostsThatRefuseUsAreReportedAsManualBootstrap(t *testing.T) {
	rows := []Row{
		{Alias: "ok", Class: string(drift.UpToDate)},
		{Alias: "blocked", Class: string(drift.AuthFailed)},
		{Alias: "dead", Class: string(drift.Unreachable)},
	}
	got := bootstrapNeeded(rows)
	if len(got) != 2 {
		t.Fatalf("got %v, want blocked and dead both listed", got)
	}
	if strings.Join(got, ",") != "blocked,dead" {
		t.Fatalf("got %v, want stable order", got)
	}
}

// bootstrapNeeded existed with a test but was never CALLED — spec F10 was
// written and never delivered. The hint is where it earns its place: a row
// reading auth-failed tells you the key is wrong, but not that fleet cannot
// fix it for you, because authorizing a key needs the access being established.
func TestStatusHintNamesHostsThatNeedManualBootstrap(t *testing.T) {
	rows := []Row{
		{Alias: "ok", Class: string(drift.UpToDate)},
		{Alias: "blocked", Class: string(drift.AuthFailed)},
		{Alias: "dead", Class: string(drift.Unreachable)},
	}
	got := bootstrapHint(rows)
	for _, want := range []string{"blocked", "dead", "ssh-copy-id"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hint %q missing %q", got, want)
		}
	}
}

// A healthy fleet must not be nagged.
func TestStatusHintIsSilentWhenEveryHostAnswers(t *testing.T) {
	if got := bootstrapHint([]Row{{Alias: "ok", Class: string(drift.UpToDate)}}); got != "" {
		t.Fatalf("hint = %q, want empty", got)
	}
}

// The hint covered only the KEY bootstrap. A password-auth host cannot be
// primed by any BatchMode probe — it needs one interactive session — so a
// CLI-only operator would see a permanent auth-failed with nothing telling
// them the way out.
func TestStatusHintExplainsHowToPrimeAPasswordAuthHost(t *testing.T) {
	got := bootstrapHint([]Row{{Alias: "blocked", Class: string(drift.AuthFailed)}})
	for _, want := range []string{"password", "fleet tui", "s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hint missing %q:\n%s", want, got)
		}
	}
}

// ssh-copy-id cannot fix a host-key fault: it connects with the same
// known_hosts and fails the same way, printing ssh's MITM banner twice. A
// host-key row is routed to `fleet trust` and kept out of the key-bootstrap list.
func TestHostKeyRowsAreSentToFleetTrustNotSshCopyID(t *testing.T) {
	rows := []Row{
		{Alias: "rekeyed", Class: string(drift.AuthFailed), Note: "host key CHANGED"},
		{Alias: "fresh", Class: string(drift.AuthFailed), Note: "host key unverified"},
	}
	if got := bootstrapNeeded(rows); len(got) != 0 {
		t.Fatalf("host-key rows need no key bootstrap, got %v", got)
	}
	got := bootstrapHint(rows)
	for _, want := range []string{"fleet trust rekeyed", "fleet trust fresh", "CHANGED"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hint missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ssh-copy-id") {
		t.Fatalf("hint offers ssh-copy-id for a host-key fault:\n%s", got)
	}
}

func TestMixedFaultsGetBothHints(t *testing.T) {
	got := bootstrapHint([]Row{
		{Alias: "rekeyed", Class: string(drift.AuthFailed), Note: "host key CHANGED"},
		{Alias: "blocked", Class: string(drift.AuthFailed), Note: "permission denied"},
	})
	if !strings.Contains(got, "fleet trust rekeyed") || !strings.Contains(got, "ssh-copy-id") ||
		strings.Contains(got, "key: blocked, rekeyed") {
		t.Fatalf("hint:\n%s", got)
	}
}
