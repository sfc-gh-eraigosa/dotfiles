package cmd

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/cfgplan"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// A fleet member's own alias commonly resolves to 127.0.0.1 via /etc/hosts, so
// this guard is load-bearing, not theoretical.
func TestLoopbackHostNameIsDetected(t *testing.T) {
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true}, {"::1", true}, {"127.1.2.3", true},
		{"192.168.0.5", false}, {"example.internal", false}, {"", false},
	} {
		if got := isLoopbackHostName(tc.host); got != tc.want {
			t.Errorf("isLoopbackHostName(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// An imported IdentityFile is only a PATH. If the key is absent the alias looks
// configured and fails at connect time, so the miss must be named.
func TestMissingIdentitiesNamesAbsentKeys(t *testing.T) {
	p := cfgplan.Plan{Changes: []cfgplan.Change{
		{Alias: "a", Kind: cfgplan.Add, Host: sshconf.Host{Alias: "a", Identity: "~/.ssh/id_here"}},
		{Alias: "b", Kind: cfgplan.Add, Host: sshconf.Host{Alias: "b", Identity: "~/.ssh/id_gone"}},
		{Alias: "c", Kind: cfgplan.Add, Host: sshconf.Host{Alias: "c"}},
		{Alias: "d", Kind: cfgplan.Unchanged, Host: sshconf.Host{Alias: "d", Identity: "~/.ssh/id_gone"}},
	}}
	got := missingIdentities(p, func(path string) bool { return strings.HasSuffix(path, "id_here") })
	if len(got) != 1 || !strings.Contains(got[0], "id_gone") || !strings.Contains(got[0], "b") {
		t.Fatalf("got %v, want only host b's absent key named", got)
	}
}

// Key readiness is a presence check, never a read of key material.
func TestKeyReadinessOnlyEverStatsAPath(t *testing.T) {
	var probed []string
	_ = missingIdentities(
		cfgplan.Plan{Changes: []cfgplan.Change{{Alias: "a", Kind: cfgplan.Add,
			Host: sshconf.Host{Alias: "a", Identity: "~/.ssh/id_x"}}}},
		func(p string) bool { probed = append(probed, p); return true },
	)
	if len(probed) != 1 || probed[0] != "~/.ssh/id_x" {
		t.Fatalf("probed %v, want exactly one path existence check", probed)
	}
}

// The plan renders what it withheld; an invisible exclusion is the failure mode
// this reporting exists to prevent.
func TestRenderPlanNamesWithheldDirectivesAndIncludes(t *testing.T) {
	p := cfgplan.Plan{
		Changes:     []cfgplan.Change{{Alias: "a", Kind: cfgplan.Add, Host: sshconf.Host{Alias: "a", HostName: "10.0.0.1"}}},
		NotImported: []string{"ProxyCommand"},
		Includes:    2,
	}
	out := renderPlan(p)
	for _, want := range []string{"+ a", "HostName 10.0.0.1", "not imported: ProxyCommand", "2 Include"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
}

func TestRenderPlanSaysNothingToDoWhenEmpty(t *testing.T) {
	if out := renderPlan(cfgplan.Plan{}); !strings.Contains(out, "already current") {
		t.Fatalf("empty plan rendered as %q", out)
	}
}
