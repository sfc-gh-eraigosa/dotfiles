package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/agent"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/featflag"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// vaultEnv isolates HOME/XDG, swaps the seams, and returns the fake store.
type vaultEnvT struct {
	home  string
	store *vault.Fake
}

func vaultEnv(t *testing.T, enabled bool) vaultEnvT {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	st := vault.NewFake()
	oo, os_, oa := openVault, vaultSettings, agentLoaded
	oi, ok := fleetInventory, fleetKnownHosts
	t.Cleanup(func() { openVault, vaultSettings, agentLoaded = oo, os_, oa; fleetInventory, fleetKnownHosts = oi, ok })
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	vaultSettings = func() featflag.Settings { return featflag.Settings{VaultEnabled: enabled} }
	agentLoaded = func(context.Context, string) (bool, error) { return false, agent.ErrNoAgent }
	fleetInventory = func() (vault.Inventory, error) { return vault.Inventory{}, nil }
	fleetKnownHosts = func() (vault.KnownHosts, error) { return vault.KnownHosts{}, nil }
	return vaultEnvT{home: home, store: st}
}

func runVault(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	c := newVaultCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetIn(strings.NewReader(stdin))
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func TestSetupWritesConfigAfterSignIn(t *testing.T) {
	e := vaultEnv(t, true)
	out, err := runVault(t, "lpass\n", "setup", "--workstation", "ws1")
	if err != nil {
		t.Fatalf("setup: %v\n%s", err, out)
	}
	p := vaultConfigPath()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
	cfg, ok, err := loadVaultConfig(p)
	if err != nil || !ok || cfg.Provider != "lpass" || cfg.Workstation != "ws1" || cfg.Vault != "fleet" {
		t.Fatalf("cfg=%+v ok=%v err=%v", cfg, ok, err)
	}
	if !strings.Contains(out, "fleet vault push") {
		t.Fatalf("next step missing:\n%s", out)
	}
	_ = e
}

func TestSetupSignsInOnceWhenSignedOut(t *testing.T) {
	e := vaultEnv(t, true)
	st := &stubStore{Fake: e.store, readyErrs: []error{vault.ErrSignedOut, nil}}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	if _, err := runVault(t, "", "setup", "--provider", "lpass"); err != nil {
		t.Fatal(err)
	}
	if st.signIns != 1 {
		t.Fatalf("signIns=%d", st.signIns)
	}
	if _, err := os.Stat(vaultConfigPath()); err != nil {
		t.Fatal("config not written")
	}
}

func TestSetupDoesNotWriteConfigWhenNotReady(t *testing.T) {
	e := vaultEnv(t, true)
	st := &stubStore{Fake: e.store, readyErrs: []error{vault.ErrNotInstalled}}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	out, err := runVault(t, "", "setup", "--provider", "lpass")
	if err == nil || !strings.Contains(out, "install") {
		t.Fatalf("err=%v out=%s", err, out)
	}
	if _, err := os.Stat(vaultConfigPath()); err == nil {
		t.Fatal("config written despite not ready")
	}
}

func TestSetupOpIsNotInThisBuild(t *testing.T) {
	vaultEnv(t, true)
	openVault = func(vault.Config) (vault.Store, error) { return nil, vault.ErrNotInstalled }
	out, err := runVault(t, "op\n", "setup")
	if err == nil || !strings.Contains(out, "1Password support is not in this build") {
		t.Fatalf("err=%v out=%s", err, out)
	}
}

type stubStore struct {
	*vault.Fake
	readyErrs []error
	signIns   int
}

func (s *stubStore) Ready(context.Context) error {
	if len(s.readyErrs) == 0 {
		return nil
	}
	e := s.readyErrs[0]
	s.readyErrs = s.readyErrs[1:]
	return e
}

func (s *stubStore) SignIn(ctx context.Context, in io.Reader, out, errw io.Writer) error {
	s.signIns++
	return nil
}

func TestVaultVerbsRefuseWhenDisabled(t *testing.T) {
	e := vaultEnv(t, false)
	for _, args := range [][]string{{"setup", "--provider", "lpass"}, {"status"}} {
		out, err := runVault(t, "lpass\n", args...)
		if err == nil {
			t.Fatalf("%v: want error", args)
		}
		if !strings.Contains(err.Error()+out, "gff set fleet.vault.enabled true") {
			t.Fatalf("%v: message lacks gff command: %v %s", args, err, out)
		}
	}
	if len(e.store.Calls) != 0 {
		t.Fatalf("store called: %v", e.store.Calls)
	}
	if _, err := os.Stat(vaultConfigPath()); err == nil {
		t.Fatal("config written while disabled")
	}
}

func TestStatusNotConfigured(t *testing.T) {
	vaultEnv(t, true)
	out, err := runVault(t, "", "status")
	if err == nil || !strings.Contains(out, "not configured: run fleet vault setup") {
		t.Fatalf("err=%v out=%s", err, out)
	}
}

const testPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l8GKJl test"

func writeLocalKey(t *testing.T, home string) (path, fp string) {
	t.Helper()
	priv, pub, err := keystore.Generate([]byte("pw"), "t")
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(home, ".ssh", "id_ed25519")
	if _, err := keystore.Write(path, priv, pub, time.Now()); err != nil {
		t.Fatal(err)
	}
	fp, err = keystore.Fingerprint(pub)
	if err != nil {
		t.Fatal(err)
	}
	return path, fp
}

func putBlob(t *testing.T, st vault.Store, name string, v any) {
	t.Helper()
	b, err := vault.Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put(context.Background(), name, b, ""); err != nil {
		t.Fatal(err)
	}
}

func configure(t *testing.T, ws string) {
	t.Helper()
	cfg, err := applyVaultDefaults(vault.Config{Provider: "lpass", Workstation: ws})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveVaultConfig(vaultConfigPath(), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestStatusReportsEachFacet(t *testing.T) {
	e := vaultEnv(t, true)
	configure(t, "ws1")
	_, fp := writeLocalKey(t, e.home)
	var r vault.Roster
	r.Add("ws1", "ssh-ed25519 AAAA", fp, time.Now())
	r.Activate(fp, time.Now())
	putBlob(t, e.store, "roster", r)
	putBlob(t, e.store, "key/ws1", vault.KeyPair{Private: vault.Secret("x"), Public: "ssh-ed25519 AAAA", Fingerprint: fp})
	putBlob(t, e.store, "inventory", vault.Inventory{Config: "Host a\n  HostName 1.1.1.1\n\nHost b\n  HostName 2.2.2.2\n"})
	putBlob(t, e.store, "known_hosts", vault.KnownHosts{Lines: []string{"a ssh-ed25519 K1", "b ssh-ed25519 K2"}})
	fleetInventory = func() (vault.Inventory, error) {
		return vault.Inventory{Config: "Host a\n  HostName 1.1.1.1\n\nHost c\n  HostName 3.3.3.3\n"}, nil
	}
	fleetKnownHosts = func() (vault.KnownHosts, error) {
		return vault.KnownHosts{Lines: []string{"a ssh-ed25519 K1", "c ssh-ed25519 K3"}}, nil
	}
	agentLoaded = func(_ context.Context, got string) (bool, error) { return got == fp, nil }

	out, err := runVault(t, "", "status")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	for _, want := range []string{
		"provider: lpass signed in",
		"key: local file exists, encrypted, " + fp,
		"vault has key/ws1",
		"roster: active",
		"loaded in agent",
		"inventory: in vault, 2 hosts differ",
		"known_hosts: in vault, 2 lines differ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestStatusNamesTheExistingKeysWhenThisNameHasNone(t *testing.T) {
	e := vaultEnv(t, true)
	configure(t, "ws1")
	putBlob(t, e.store, "key/other", vault.KeyPair{Private: vault.Secret("x")})
	putBlob(t, e.store, "key/third", vault.KeyPair{Private: vault.Secret("x")})
	out, err := runVault(t, "", "status")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "no vault key for ws1; existing: other, third") {
		t.Fatalf("got:\n%s", out)
	}
	if !strings.Contains(out, "no local key file") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestStatusSignedOutContinuesLocally(t *testing.T) {
	e := vaultEnv(t, true)
	configure(t, "ws1")
	writeLocalKey(t, e.home)
	st := &stubStore{Fake: e.store, readyErrs: []error{vault.ErrSignedOut}}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	out, err := runVault(t, "", "status")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "signed out") || !strings.Contains(out, "local file exists") || !strings.Contains(out, "no agent running") || st.signIns != 0 {
		t.Fatalf("signIns=%d\n%s", st.signIns, out)
	}
	if len(e.store.Calls) != 0 {
		t.Fatalf("store used while signed out: %v", e.store.Calls)
	}
}

func TestFleetInventoryAndKnownHosts(t *testing.T) {
	e := vaultEnv(t, true)
	cfgPath := filepath.Join(e.home, "ssh_config")
	body := "Host a #fleet\n  HostName 1.1.1.1\n\nHost plain\n  HostName 9.9.9.9\n\nHost b #fleet\n  HostName 2.2.2.2\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	oc, om := flagConfig, flagMarker
	t.Cleanup(func() { flagConfig, flagMarker = oc, om })
	flagConfig, flagMarker = cfgPath, "#fleet"
	inv, err := inventoryFromConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inv.Config, "Host a") || !strings.Contains(inv.Config, "Host b") || strings.Contains(inv.Config, "plain") {
		t.Fatalf("inventory:\n%s", inv.Config)
	}
	kh := filepath.Join(e.home, "kh")
	if err := os.WriteFile(kh, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	x := func(name, _ string, args ...string) (string, error) {
		if name == "ssh" {
			return "hostname " + args[1] + "\nport 22\nuserknownhostsfile " + kh + "\n", nil
		}
		return "# Host " + args[1] + " found: line 1\n" + args[1] + " ssh-ed25519 KEY\n", nil
	}
	k, err := knownHostsFor(x, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Lines) != 2 || k.Lines[0] != "a ssh-ed25519 KEY" {
		t.Fatalf("lines: %v", k.Lines)
	}
}

func TestEveryVaultSubcommandIsGated(t *testing.T) {
	e := vaultEnv(t, false)
	subs := newVaultCmd().Commands()
	if len(subs) == 0 {
		t.Fatal("no subcommands")
	}
	for _, sc := range subs {
		out, err := runVault(t, "lpass\n", sc.Name())
		if err == nil || !strings.Contains(err.Error()+out, "gff set fleet.vault.enabled true") {
			t.Errorf("%s is not gated: err=%v out=%s", sc.Name(), err, out)
		}
	}
	if len(e.store.Calls) != 0 {
		t.Fatalf("store called: %v", e.store.Calls)
	}
}

type exitErr int

func (e exitErr) Error() string { return "exit status" }
func (e exitErr) ExitCode() int { return int(e) }

func khExec(t *testing.T, keygen func() (string, error)) func(string, string, ...string) (string, error) {
	kh := filepath.Join(t.TempDir(), "kh")
	if err := os.WriteFile(kh, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return func(name, _ string, args ...string) (string, error) {
		if name == "ssh" {
			return "hostname " + args[1] + "\nport 22\nuserknownhostsfile " + kh + "\n", nil
		}
		return keygen()
	}
}

func TestKnownHostsNoEntryIsNotAnError(t *testing.T) {
	x := khExec(t, func() (string, error) { return "", exitErr(1) })
	k, err := knownHostsFor(x, []string{"a"})
	if err != nil || len(k.Lines) != 0 {
		t.Fatalf("%v %v", k, err)
	}
}

func TestKnownHostsKeygenMissingIsAnError(t *testing.T) {
	x := khExec(t, func() (string, error) {
		return "", errors.New(`exec: "ssh-keygen": executable file not found in $PATH`)
	})
	if _, err := knownHostsFor(x, []string{"a"}); err == nil {
		t.Fatal("want error")
	}
}

func TestKnownHostsEveryAliasUnresolvableIsAnError(t *testing.T) {
	x := func(string, string, ...string) (string, error) { return "", errors.New("not found") }
	if _, err := knownHostsFor(x, []string{"a", "b"}); err == nil {
		t.Fatal("want error")
	}
	if k, err := knownHostsFor(x, nil); err != nil || len(k.Lines) != 0 {
		t.Fatalf("no aliases is fine: %v %v", k, err)
	}
}

func TestStatusShowsKnownHostsErrorAndKeepsGoing(t *testing.T) {
	e := vaultEnv(t, true)
	configure(t, "ws1")
	putBlob(t, e.store, "known_hosts", vault.KnownHosts{Lines: []string{"a k"}})
	putBlob(t, e.store, "inventory", vault.Inventory{})
	fleetKnownHosts = func() (vault.KnownHosts, error) { return vault.KnownHosts{}, errors.New("ssh-keygen: not found") }
	out, err := runVault(t, "", "status")
	if err != nil || !strings.Contains(out, "known_hosts: local fleet lines unreadable: ssh-keygen: not found") || !strings.Contains(out, "inventory:") {
		t.Fatalf("%v\n%s", err, out)
	}
}
