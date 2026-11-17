package cmd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/agent"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
)

// preflightHome gives a temp HOME holding an ed25519 pair; pass a passphrase
// for an encrypted key, nil for a plain one. Nothing here touches the real
// ~/.ssh or agent.
func preflightHome(t *testing.T, pass []byte) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	var priv, pub []byte
	var err error
	if pass != nil {
		priv, pub, err = keystore.Generate(pass, "t@box")
	} else { // Generate refuses an empty passphrase, so build a plain key by hand
		priv, pub, err = plainKey()
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), priv, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519.pub"), pub, 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func stubAgent(t *testing.T, loaded bool, err error) {
	t.Helper()
	old := agentLoaded
	agentLoaded = func(context.Context, string) (bool, error) { return loaded, err }
	t.Cleanup(func() { agentLoaded = old })
}

func stubVaultOff(t *testing.T) {
	t.Helper()
	old := vaultSettings
	vaultSettings = func() featflag.Settings { return featflag.Settings{} }
	t.Cleanup(func() { vaultSettings = old })
}

func TestKeyLockedTruthTable(t *testing.T) {
	yes := func(string) (bool, error) { return true, nil }
	no := func(string) (bool, error) { return false, nil }
	boom := func(string) (bool, error) { return false, errors.New("boom") }
	noAgent := func(string) (bool, error) { return false, agent.ErrNoAgent }
	for _, c := range []struct {
		name        string
		enc, loaded func(string) (bool, error)
		want        bool
	}{
		{"encrypted+unloaded", yes, no, true},
		{"encrypted+loaded", yes, yes, false},
		{"plain", no, no, false},
		{"unreadable key", boom, no, false},
		{"fingerprint/agent failure", yes, boom, false},
		{"encrypted+no agent", yes, noAgent, true},
	} {
		got, hint := keyLocked("/k", "SHA256:x", c.enc, c.loaded)
		if got != c.want || (got && hint == "") || (!got && hint != "") {
			t.Errorf("%s: locked=%v hint=%q, want locked=%v", c.name, got, hint, c.want)
		}
	}
}

func TestPreflightStopsBeforeAnyDial(t *testing.T) {
	preflightHome(t, []byte("pw"))
	stubVaultOff(t)
	stubAgent(t, false, nil)
	// A host is configured: if preflight did not stop first, status would
	// dial it. Without a host the "no fleet hosts" error would show instead.
	cfg := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(cfg, []byte("Host ghost #fleet\n  HostName 192.0.2.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := flagConfig
	flagConfig = cfg
	t.Cleanup(func() { flagConfig = old })

	err := statusCmd.RunE(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "ssh-add ") || !strings.Contains(err.Error(), "id_ed25519") {
		t.Fatalf("want the ssh-add hint, got %v", err)
	}
	var ee exitError
	if errors.As(err, &ee) {
		t.Fatalf("a locked key must not look like drift (exitError): %v", err)
	}
}

func TestPreflightWithAVaultHintsKeysUnlock(t *testing.T) {
	home := preflightHome(t, []byte("pw"))
	stubAgent(t, false, nil)
	old := vaultSettings
	vaultSettings = func() featflag.Settings { return featflag.Settings{VaultEnabled: true} }
	t.Cleanup(func() { vaultSettings = old })
	cfgDir := filepath.Join(home, ".config", "fleet")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "vault.yaml"), []byte("provider: lpass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := preflight(context.Background())
	if err == nil || !strings.Contains(err.Error(), lockedHint) {
		t.Fatalf("want %q, got %v", lockedHint, err)
	}
}

func TestPreflightIgnoresAPlainKey(t *testing.T) {
	preflightHome(t, nil)
	stubVaultOff(t)
	stubAgent(t, false, nil)
	if err := preflight(context.Background()); err != nil {
		t.Fatalf("plain key must pass through: %v", err)
	}
}

func TestPreflightIgnoresALoadedKey(t *testing.T) {
	preflightHome(t, []byte("pw"))
	stubVaultOff(t)
	stubAgent(t, true, nil)
	if err := preflight(context.Background()); err != nil {
		t.Fatalf("loaded key must pass through: %v", err)
	}
}

func TestPreflightIgnoresAMissingKeyAndAgentErrors(t *testing.T) {
	home := preflightHome(t, []byte("pw"))
	stubVaultOff(t)
	stubAgent(t, false, errors.New("agent exploded"))
	if err := preflight(context.Background()); err != nil {
		t.Fatalf("an agent failure must not block: %v", err)
	}
	if err := os.Remove(filepath.Join(home, ".ssh", "id_ed25519")); err != nil {
		t.Fatal(err)
	}
	stubAgent(t, false, nil)
	if err := preflight(context.Background()); err != nil {
		t.Fatalf("an unreadable key must not block: %v", err)
	}
}

func TestPreflightWithNoAgentNamesHowToStartOne(t *testing.T) {
	preflightHome(t, []byte("pw"))
	stubVaultOff(t)
	stubAgent(t, false, agent.ErrNoAgent)
	for _, goos := range []string{"linux", "darwin"} {
		old := preflightGOOS
		preflightGOOS = goos
		err := preflight(context.Background())
		preflightGOOS = old
		if err == nil || !strings.Contains(err.Error(), `eval "$(ssh-agent -s)"`) {
			t.Fatalf("%s: want how to start an agent, got %v", goos, err)
		}
		if has := strings.Contains(err.Error(), "launchd"); has != (goos == "darwin") {
			t.Errorf("%s: launchd mention = %v", goos, has)
		}
	}
}

func TestPreflightNeverMarksAHostAuthFailed(t *testing.T) {
	preflightHome(t, []byte("pw"))
	stubVaultOff(t)
	stubAgent(t, false, nil)
	cfg := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(cfg, []byte("Host ghost #fleet\n  HostName 192.0.2.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := flagConfig
	flagConfig = cfg
	t.Cleanup(func() { flagConfig = old })
	var out strings.Builder
	c := &cobra.Command{}
	c.SetOut(&out)
	err := statusCmd.RunE(c, nil)
	if err == nil {
		t.Fatal("want the preflight error")
	}
	if s := out.String(); s != "" || strings.Contains(err.Error(), "auth-failed") {
		t.Fatalf("no host row may be produced: out=%q err=%v", s, err)
	}
}

func plainKey() (priv, pub []byte, err error) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	blk, err := ssh.MarshalPrivateKey(sk, "t@box")
	if err != nil {
		return nil, nil, err
	}
	sp, err := ssh.NewPublicKey(pk)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(blk), ssh.MarshalAuthorizedKey(sp), nil
}
