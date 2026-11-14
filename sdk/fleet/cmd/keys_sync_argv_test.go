package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/runner"
)

// UC8 guard: with no vault configured, `keys sync` must send exactly today's
// argv. keysSyncCmd builds a real runner.Exec, so the fixture drives the same
// managedKeys() + syncKeyToHost() loop against runner.Fake. Regenerate the
// golden only deliberately: FLEET_UPDATE_GOLDEN=1 go test ./cmd -run KeysSyncArgv.
func TestNoVaultMeansTodaysKeysSyncArgv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFIXTUREFIXTUREFIXTUREFIXTUREFIXTUREFIXTUR me@box\n"
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519.pub"), []byte(pub), 0o644); err != nil {
		t.Fatal(err)
	}

	mine, err := managedKeys()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(mine))
	for n := range mine {
		names = append(names, n)
	}
	sort.Strings(names)

	var sent []string
	r := recordingRunner{fake: runner.Fake{Out: map[string]string{"alpha": "", "beta": ""}}, log: &sent}
	for _, h := range []string{"alpha", "beta"} {
		for _, n := range names {
			if err := syncKeyToHost(r, h, mine[n]); err != nil {
				t.Fatal(err)
			}
		}
	}
	got := strings.Join(sent, "\n") + "\n"

	golden := filepath.Join("testdata", "keys_sync_argv.golden")
	if os.Getenv("FLEET_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("keys sync argv changed with no vault configured.\n got: %q\nwant: %q", got, want)
	}
}
