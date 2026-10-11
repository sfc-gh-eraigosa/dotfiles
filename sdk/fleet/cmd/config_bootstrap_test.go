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
// have on a host that REFUSED us. Claiming otherwise would be a lie. A host that
// never answered (unreachable) is a network fault, not a credential one, so it
// belongs to unreachableHosts — not here, where it used to misdirect the
// operator at `ssh-copy-id`.
func TestHostsThatRefuseUsAreReportedAsManualBootstrap(t *testing.T) {
	rows := []Row{
		{Alias: "ok", Class: string(drift.UpToDate)},
		{Alias: "blocked", Class: string(drift.AuthFailed)},
		{Alias: "dead", Class: string(drift.Unreachable)},
	}
	if got := bootstrapNeeded(rows); strings.Join(got, ",") != "blocked" {
		t.Fatalf("bootstrapNeeded = %v, want only the auth-failed host", got)
	}
	if got := unreachableHosts(rows); strings.Join(got, ",") != "dead" {
		t.Fatalf("unreachableHosts = %v, want only the unreachable host", got)
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
	got := bootstrapHint(rows, noNote, false)
	for _, want := range []string{"blocked", "dead", "ssh-copy-id"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hint %q missing %q", got, want)
		}
	}
	// The ssh-copy-id advice names ONLY the auth-failed host. Substring checks
	// on the whole hint passed even when the unreachable host was blamed on
	// the key, so check the key sentence on its own.
	key := keyBootstrapHint(rows)
	if !strings.Contains(key, "blocked") || strings.Contains(key, "dead") {
		t.Fatalf("key-bootstrap advice must name blocked and never dead:\n%s", key)
	}
}

// The regression that started this: an unreachable host opened no SSH session,
// so blaming the key and offering ssh-copy-id points the fix at the wrong
// layer. It must be named as a NETWORK fault instead, and never inside the
// `ssh-copy-id` sentence.
func TestUnreachableHostIsNotBlamedOnTheKey(t *testing.T) {
	rows := []Row{{Alias: "dead", Class: string(drift.Unreachable)}}
	got := bootstrapHint(rows, noNote, false)
	if strings.Contains(got, "ssh-copy-id") || strings.Contains(got, "with your key") {
		t.Fatalf("an unreachable host must not be offered ssh-copy-id:\n%s", got)
	}
	for _, want := range []string{"dead", "NETWORK fault", "fleet wake"} {
		if !strings.Contains(got, want) {
			t.Fatalf("unreachable hint missing %q:\n%s", want, got)
		}
	}
}

// When this workstation's own Wi-Fi is asleep, the unreachable hint carries the
// one-line cause and cure — but only when there is an unreachable host to
// explain, and never on a healthy fleet.
func TestUnreachableHintCarriesTheLocalPowerSaveCause(t *testing.T) {
	note := powerSaveNote("wlan0", true)
	if note == "" {
		t.Fatal("a radio with power save on must produce a note")
	}
	rows := []Row{{Alias: "dead", Class: string(drift.Unreachable)}}
	if got := bootstrapHint(rows, noteOf(note), false); !strings.Contains(got, "power_save ON") || !strings.Contains(got, "wlan0") {
		t.Fatalf("hint should carry the power-save note:\n%s", got)
	}
	// No unreachable host ⇒ no place for the note, even if the radio is asleep.
	healthy := []Row{{Alias: "ok", Class: string(drift.UpToDate)}}
	if got := bootstrapHint(healthy, noteOf(note), false); got != "" {
		t.Fatalf("a healthy fleet must stay silent, got:\n%s", got)
	}
}

// A healthy fleet must not be nagged.
func TestStatusHintIsSilentWhenEveryHostAnswers(t *testing.T) {
	if got := bootstrapHint([]Row{{Alias: "ok", Class: string(drift.UpToDate)}}, noNote, false); got != "" {
		t.Fatalf("hint = %q, want empty", got)
	}
}

// The hint covered only the KEY bootstrap. A password-auth host cannot be
// primed by any BatchMode probe — it needs one interactive session — so a
// CLI-only operator would see a permanent auth-failed with nothing telling
// them the way out.
func TestStatusHintExplainsHowToPrimeAPasswordAuthHost(t *testing.T) {
	got := bootstrapHint([]Row{{Alias: "blocked", Class: string(drift.AuthFailed)}}, noNote, false)
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
	got := bootstrapHint(rows, noNote, false)
	for _, want := range []string{"fleet trust rekeyed", "fleet trust fresh", "CHANGED", "T in `fleet tui`"} {
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
	}, noNote, false)
	if !strings.Contains(got, "fleet trust rekeyed") || !strings.Contains(got, "ssh-copy-id") ||
		strings.Contains(got, "key: blocked, rekeyed") {
		t.Fatalf("hint:\n%s", got)
	}
}

func noNote() string { return "" }

func noteOf(s string) func() string { return func() string { return s } }

// The power-save probe forks `iw` per wireless NIC, so it runs only when an
// unreachable host gives the note somewhere to go.
func TestPowerSaveProbeRunsOnlyForAnUnreachableHost(t *testing.T) {
	calls := 0
	probe := func() string { calls++; return "" }
	bootstrapHint([]Row{{Alias: "ok", Class: string(drift.UpToDate)}, {Alias: "b", Class: string(drift.AuthFailed)}}, probe, true)
	if calls != 0 {
		t.Fatalf("probe ran %d time(s) on a fleet with no unreachable host", calls)
	}
	bootstrapHint([]Row{{Alias: "dead", Class: string(drift.Unreachable)}}, probe, true)
	if calls != 1 {
		t.Fatalf("probe ran %d time(s) for one unreachable host, want 1", calls)
	}
}

// fleet status has ALREADY run the wake ladder unless --no-wake, so telling
// the operator to run `fleet wake` repeats the step that just failed.
func TestUnreachableHintDoesNotRepeatAWakeStatusAlreadyTried(t *testing.T) {
	rows := []Row{{Alias: "dead", Class: string(drift.Unreachable)}}
	tried := bootstrapHint(rows, noNote, true)
	if strings.Contains(tried, "fleet wake <alias>` nudges") {
		t.Fatalf("hint repeats the wake status already ran:\n%s", tried)
	}
	if !strings.Contains(tried, "already") {
		t.Fatalf("hint should say the wake ladder was already tried:\n%s", tried)
	}
	if notTried := bootstrapHint(rows, noNote, false); !strings.Contains(notTried, "fleet wake") {
		t.Fatalf("with --no-wake the hint should offer fleet wake:\n%s", notTried)
	}
}
