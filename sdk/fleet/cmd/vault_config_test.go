package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

func writeVaultYAML(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "vault.yaml")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVaultConfigMissingIsUnconfigured(t *testing.T) {
	c, ok, err := loadVaultConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || ok || c != (vault.Config{}) {
		t.Fatalf("got (%+v, %v, %v), want zero, false, nil", c, ok, err)
	}
}

func TestVaultConfigRefusesAGroupWritableFile(t *testing.T) {
	p := writeVaultYAML(t, "provider: op\n", 0o664)
	_, _, err := loadVaultConfig(p)
	if err == nil || !strings.Contains(err.Error(), "0664") {
		t.Fatalf("want an error naming mode 0664, got %v", err)
	}
}

func TestVaultConfigRefusesAFileNotOwnedByUs(t *testing.T) {
	p := writeVaultYAML(t, "provider: op\n", 0o600)
	old := checkOwner
	checkOwner = func(os.FileInfo) (int, bool) { return os.Getuid() + 1, true }
	defer func() { checkOwner = old }()
	if _, _, err := loadVaultConfig(p); err == nil || !strings.Contains(err.Error(), "not owned") {
		t.Fatalf("want a not-owned error, got %v", err)
	}
}

func TestVaultConfigDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	host, _ := os.Hostname()
	host, _, _ = strings.Cut(host, ".")

	c, ok, err := loadVaultConfig(writeVaultYAML(t, "provider: op\n", 0o600))
	if err != nil || !ok {
		t.Fatalf("load: %v ok=%v", err, ok)
	}
	if c.Workstation != host {
		t.Fatalf("Workstation = %q, want short hostname %q", c.Workstation, host)
	}
	if c.Key != filepath.Join(home, ".ssh", "id_ed25519") {
		t.Fatalf("Key = %q", c.Key)
	}
	if c.Vault != "Private" {
		t.Fatalf("op Vault = %q, want Private", c.Vault)
	}
	l, _, err := loadVaultConfig(writeVaultYAML(t, "provider: lpass\n", 0o600))
	if err != nil || l.Vault != "fleet" {
		t.Fatalf("lpass: %+v %v", l, err)
	}
	e, _, err := loadVaultConfig(writeVaultYAML(t, "provider: op\nvault: Work\nworkstation: w1\nkey: /k/id\n", 0o600))
	if err != nil || e.Vault != "Work" || e.Workstation != "w1" || e.Key != "/k/id" {
		t.Fatalf("explicit values lost: %+v %v", e, err)
	}
}

func TestVaultConfigUnknownProviderListsKnown(t *testing.T) {
	_, _, err := loadVaultConfig(writeVaultYAML(t, "provider: keepass\n", 0o600))
	if err == nil || !strings.Contains(err.Error(), `"op"`) || !strings.Contains(err.Error(), `"lpass"`) {
		t.Fatalf("want an error listing op and lpass, got %v", err)
	}
}

func TestVaultConfigSaveRoundTrips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := filepath.Join(t.TempDir(), "a", "b", "vault.yaml")
	want := vault.Config{Provider: "lpass", Vault: "fleet", Workstation: "w1", Key: "/k/id"}
	if err := saveVaultConfig(p, want); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v, want 0600", fi.Mode().Perm())
	}
	di, _ := os.Stat(filepath.Dir(p))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v, want 0700", di.Mode().Perm())
	}
	got, ok, err := loadVaultConfig(p)
	if err != nil || !ok || got != want {
		t.Fatalf("round trip: %+v %v %v", got, ok, err)
	}
}

func TestVaultConfigPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x/cfg")
	if got := vaultConfigPath(); got != filepath.Join("/x/cfg", "fleet", "vault.yaml") {
		t.Fatalf("got %q", got)
	}
}

func TestVaultActiveNeedsBothGateAndConfig(t *testing.T) {
	on, off := featflag.Settings{VaultEnabled: true}, featflag.Settings{}
	for _, c := range []struct {
		s         featflag.Settings
		cfg, want bool
	}{
		{on, true, true}, {on, false, false}, {off, true, false}, {off, false, false},
	} {
		if got := vaultActive(c.s, c.cfg); got != c.want {
			t.Fatalf("vaultActive(%v,%v) = %v", c.s.VaultEnabled, c.cfg, got)
		}
	}
}

func TestVaultConfigExpandsTildeInKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	c, _, err := loadVaultConfig(writeVaultYAML(t, "provider: op\nkey: ~/.ssh/foo\n", 0o600))
	if err != nil || c.Key != filepath.Join(home, ".ssh", "foo") {
		t.Fatalf("Key = %q, err %v; want ~ expanded", c.Key, err)
	}
	c, _, _ = loadVaultConfig(writeVaultYAML(t, "provider: op\nkey: /abs/k\n", 0o600))
	if c.Key != "/abs/k" {
		t.Fatalf("absolute key changed: %q", c.Key)
	}
}

func TestVaultConfigRejectsUnknownKeys(t *testing.T) {
	_, _, err := loadVaultConfig(writeVaultYAML(t, "provider: op\nvualt: Work\n", 0o600))
	if err == nil || !strings.Contains(err.Error(), "vualt") {
		t.Fatalf("want an error naming vualt, got %v", err)
	}
}

// An empty file decodes to a zero config, so it is rejected for its missing
// provider exactly as before (not treated as unconfigured, no panic on EOF).
func TestVaultConfigEmptyFileIsAMissingProviderError(t *testing.T) {
	_, ok, err := loadVaultConfig(writeVaultYAML(t, "", 0o600))
	if ok || err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("got ok=%v err=%v, want the unknown-provider error", ok, err)
	}
}
