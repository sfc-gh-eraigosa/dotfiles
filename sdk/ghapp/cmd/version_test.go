package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestVersionCommand: `ghapp version` prints the version block; in a test
// binary (no ldflags) the version is the default "dev".
func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "ghapp vdev") {
		t.Fatalf("want default dev version in output, got %q", got)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("version output is empty")
	}
}

// TestRootHelpNonTTY: bare `ghapp` prints help (there is no TUI), exit 0.
func TestRootHelpNonTTY(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("want help output, got %q", out.String())
	}
}
