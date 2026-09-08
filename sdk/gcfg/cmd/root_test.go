package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func run(args ...string) (string, string, error) {
	var out, errb bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

func TestVersionCommand(t *testing.T) {
	out, _, err := run("version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "gcfg vdev") {
		t.Fatalf("want default dev version, got %q", out)
	}
}

func TestBareCommandPrintsHelp(t *testing.T) {
	out, _, err := run()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("want help output, got %q", out)
	}
}

// The repo target defaults to -R and is parsed as owner/repo.
func TestTargetFlagParsing(t *testing.T) {
	for _, tc := range []struct{ in, owner, repo string }{
		{"sfc-gh-eraigosa/dotfiles", "sfc-gh-eraigosa", "dotfiles"},
	} {
		o, r, err := parseTarget(tc.in)
		if err != nil || o != tc.owner || r != tc.repo {
			t.Fatalf("parseTarget(%q) = %q %q %v", tc.in, o, r, err)
		}
	}
	for _, bad := range []string{"", "noslash", "a/", "/b", "a/b/c"} {
		if _, _, err := parseTarget(bad); err == nil {
			t.Errorf("parseTarget(%q): want error", bad)
		}
	}
}
