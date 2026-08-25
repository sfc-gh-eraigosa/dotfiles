package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestEnvOr(t *testing.T) {
	t.Setenv("WLINK_TEST_KEY", "set")
	if got := envOr("WLINK_TEST_KEY", "fallback"); got != "set" {
		t.Errorf("envOr = %q, want the env value", got)
	}
	if got := envOr("WLINK_TEST_UNSET_KEY", "fallback"); got != "fallback" {
		t.Errorf("envOr = %q, want the fallback", got)
	}
}

// IsWSL gates every command, so it must key off the documented marker rather
// than something incidental.
func TestIsWSL_MatchesProcVersion(t *testing.T) {
	b, err := os.ReadFile("/proc/version")
	if err != nil {
		t.Skip("no /proc/version here")
	}
	want := strings.Contains(strings.ToLower(string(b)), "microsoft")
	if got := IsWSL(); got != want {
		t.Errorf("IsWSL() = %v, want %v for %q", got, want, string(b))
	}
}

// The commands must exist and carry the flags the docs promise.
func TestRootCommand_HasThePromisedSurface(t *testing.T) {
	root := newRootCmd()
	byName := map[string]bool{}
	for _, c := range root.Commands() {
		byName[c.Name()] = true
	}
	for _, want := range []string{"pin", "unpin"} {
		if !byName[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
	pin, _, err := root.Find([]string{"pin"})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"dry-run", "allow-nonrecursive"} {
		if pin.Flags().Lookup(flag) == nil {
			t.Errorf("pin is missing --%s", flag)
		}
	}
}

// EC-19: an unknown flag is a usage error (exit 2), distinct from a safe
// decline (0) and a real failure (1).
func TestUnknownFlag_IsAUsageError(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"--definitely-not-a-flag"})
	root.SetOut(&strings.Builder{})
	root.SetErr(&strings.Builder{})
	if err := root.Execute(); err == nil {
		t.Fatal("unknown flag accepted; want a usage error")
	}
}
