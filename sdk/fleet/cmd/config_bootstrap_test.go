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
