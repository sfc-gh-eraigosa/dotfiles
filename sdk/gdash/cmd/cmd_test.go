package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/version"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func TestVersionPrintsStampedVersion(t *testing.T) {
	old := version.Version
	version.Version = "9.8.7"
	t.Cleanup(func() { version.Version = old })
	out, err := run(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "gdash v9.8.7\n") {
		t.Fatalf("version output = %q", out)
	}
	out, err = run(t, "version", "--json")
	if err != nil || !strings.Contains(out, `"version":"9.8.7"`) {
		t.Fatalf("version --json = %q %v", out, err)
	}
}
