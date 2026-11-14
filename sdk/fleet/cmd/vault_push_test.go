package cmd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

// pushTestPass is a throwaway passphrase for a key generated in the test; the
// marker makes a leak into output easy to spot.
const pushTestPass = "PASS-MARKER-push-test"

const pushInventory = "Host a  #fleet\n    HostName 10.0.0.1\n    User pi\n\nHost b  #fleet\n    HostName 10.0.0.2\n"

var pushKnownHosts = []string{"a ssh-ed25519 AAAAKEYA", "b ssh-ed25519 AAAAKEYB"}

type pushKeyT struct {
	path, pub, fp string
	private       []byte
}

// pushEnv is vaultEnv + a configured lpass vault for ws1, a local key and
// local inventory/known_hosts. pass "" writes an UNENCRYPTED key.
func pushEnv(t *testing.T, pass string) (vaultEnvT, pushKeyT) {
	t.Helper()
	e := vaultEnv(t, true)
	configure(t, "ws1")
	k := writePushKey(t, e.home, pass)
	fleetInventory = func() (vault.Inventory, error) { return vault.Inventory{Config: pushInventory}, nil }
	fleetKnownHosts = func() (vault.KnownHosts, []unresolvedHost, error) {
		return vault.KnownHosts{Lines: append([]string(nil), pushKnownHosts...)}, nil, nil
	}
	return e, k
}

func writePushKey(t *testing.T, home, pass string) pushKeyT {
	t.Helper()
	path := filepath.Join(home, ".ssh", "id_ed25519")
	var priv, pub []byte
	if pass != "" {
		var err error
		if priv, pub, err = keystore.Generate([]byte(pass), "ws1-test"); err != nil {
			t.Fatal(err)
		}
	} else {
		pk, sk, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		blk, err := ssh.MarshalPrivateKey(sk, "ws1-test")
		if err != nil {
			t.Fatal(err)
		}
		priv = pem.EncodeToMemory(blk)
		sp, err := ssh.NewPublicKey(pk)
		if err != nil {
			t.Fatal(err)
		}
		pub = append(ssh.MarshalAuthorizedKey(sp)[:len(ssh.MarshalAuthorizedKey(sp))-1], []byte(" ws1-test\n")...)
	}
	if _, err := keystore.Write(path, priv, pub, time.Now()); err != nil {
		t.Fatal(err)
	}
	fp, err := keystore.Fingerprint(pub)
	if err != nil {
		t.Fatal(err)
	}
	return pushKeyT{path: path, pub: strings.TrimSpace(string(pub)), fp: fp, private: priv}
}

func getItem[T vault.KeyPair | vault.Roster | vault.Inventory | vault.KnownHosts](t *testing.T, st vault.Store, name string) T {
	t.Helper()
	b, err := st.Get(context.Background(), name)
	if err != nil {
		t.Fatalf("get %s: %v", name, err)
	}
	v, err := vault.Decode[T](b.Data)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return v
}

func puts(calls []string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, "put ") {
			n++
		}
	}
	return n
}

// assertNoSecret fails if the output carries the passphrase or any line of
// the private key file.
func assertNoSecret(t *testing.T, out string, secrets ...string) {
	t.Helper()
	for _, s := range append(secrets, pushTestPass, "PRIVATE KEY", "PRIVATE-MATERIAL") {
		if s != "" && strings.Contains(out, s) {
			t.Fatalf("output leaks a secret (%d bytes matched):\n%s", len(s), strings.ReplaceAll(out, s, "<LEAK>"))
		}
	}
}

func keyLines(priv []byte) []string {
	var out []string
	for _, l := range strings.Split(string(priv), "\n") {
		if len(l) > 20 && !strings.HasPrefix(l, "-----") {
			out = append(out, l)
		}
	}
	return out
}

func TestPushShowsADiffAndWritesAfterConfirm(t *testing.T) {
	e, k := pushEnv(t, pushTestPass)
	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	for _, want := range []string{"key/ws1: new", "roster: add ws1 " + k.fp + " as active", "inventory:", "+ a", "+ b", "known_hosts: +2", "[y/N]"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	assertNoSecret(t, out, keyLines(k.private)...)

	kp := getItem[vault.KeyPair](t, e.store, "key/ws1")
	if string(kp.Private) != string(k.private) {
		t.Fatal("vault private key is not the local file as-is")
	}
	if string(kp.Passphrase) != pushTestPass || kp.Fingerprint != k.fp || kp.Public != k.pub {
		t.Fatalf("key pair fields wrong: fp=%s pub=%s", kp.Fingerprint, kp.Public)
	}
	r := getItem[vault.Roster](t, e.store, "roster")
	if len(r.Entries) != 1 || r.Entries[0].Fingerprint != k.fp || r.Entries[0].State != vault.KeyActive || r.Entries[0].Workstation != "ws1" {
		t.Fatalf("roster: %+v", r.Entries)
	}
	inv := getItem[vault.Inventory](t, e.store, "inventory")
	for _, want := range []string{"Host a", "HostName 10.0.0.1", "User pi", "Host b", "HostName 10.0.0.2"} {
		if !strings.Contains(inv.Config, want) {
			t.Errorf("inventory lacks %q:\n%s", want, inv.Config)
		}
	}
	kh := getItem[vault.KnownHosts](t, e.store, "known_hosts")
	if strings.Join(kh.Lines, "|") != strings.Join(pushKnownHosts, "|") {
		t.Fatalf("known_hosts: %v", kh.Lines)
	}
	// The local key is untouched.
	if b, _ := os.ReadFile(k.path); string(b) != string(k.private) {
		t.Fatal("local key file changed")
	}
}

func TestPushNothingChangedWritesNothing(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	if out, err := runVault(t, "y\n"+pushTestPass+"\n", "push"); err != nil {
		t.Fatalf("first push: %v\n%s", err, out)
	}
	before := puts(e.store.Calls)
	out, err := runVault(t, "", "push") // no confirm, no passphrase available
	if err != nil {
		t.Fatalf("second push: %v\n%s", err, out)
	}
	if !strings.Contains(out, "nothing to push") {
		t.Fatalf("want 'nothing to push':\n%s", out)
	}
	if got := puts(e.store.Calls); got != before {
		t.Fatalf("second push wrote %d item(s)", got-before)
	}
	if strings.Contains(out, "passphrase") {
		t.Fatalf("asked for a passphrase with nothing to push:\n%s", out)
	}
}

// conflictStore lets another workstation write the roster between our read
// and our write, once.
type conflictStore struct {
	*vault.Fake
	raced bool
}

func (s *conflictStore) Put(ctx context.Context, name string, data vault.Secret, version string) (string, error) {
	if name == "roster" && !s.raced {
		s.raced = true
		var r vault.Roster
		cur := ""
		if b, err := s.Fake.Get(ctx, "roster"); err == nil {
			cur = b.Version
			r, _ = vault.Decode[vault.Roster](b.Data)
		}
		r.Add("ws2", "ssh-ed25519 AAAAOTHER", "SHA256:other", time.Now())
		r.Activate("SHA256:other", time.Now())
		enc, _ := vault.Encode(r)
		if _, err := s.Fake.Put(ctx, "roster", enc, cur); err != nil {
			return "", err
		}
	}
	return s.Fake.Put(ctx, name, data, version)
}

func TestPushConflictRereadsAndAsksAgain(t *testing.T) {
	e, k := pushEnv(t, pushTestPass)
	st := &conflictStore{Fake: e.store}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	out, err := runVault(t, "y\n"+pushTestPass+"\ny\n", "push")
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	if n := strings.Count(out, "[y/N]"); n != 2 {
		t.Fatalf("asked %d times, want 2:\n%s", n, out)
	}
	if !strings.Contains(out, "roster changed in the vault") {
		t.Fatalf("conflict not reported:\n%s", out)
	}
	if strings.Count(out, "passphrase") > 1 {
		t.Fatalf("passphrase asked again after the conflict:\n%s", out)
	}
	r := getItem[vault.Roster](t, st, "roster")
	states := map[string]vault.KeyState{}
	for _, en := range r.Entries {
		states[en.Fingerprint] = en.State
	}
	if states[k.fp] != vault.KeyActive || states["SHA256:other"] != vault.KeyActive {
		t.Fatalf("lost an update: %+v", r.Entries)
	}
	assertNoSecret(t, out, keyLines(k.private)...)
}

// signedOutStore reads fine but its session expires at write number failAt
// (0 = the first); earlier writes land.
type signedOutStore struct {
	*vault.Fake
	failAt   int
	putTries int
}

func (s *signedOutStore) Put(ctx context.Context, name string, data vault.Secret, version string) (string, error) {
	s.putTries++
	if s.putTries >= max(s.failAt, 1) {
		return "", vault.ErrSignedOut
	}
	return s.Fake.Put(ctx, name, data, version)
}

func TestPushStopsCleanlyWhenSignedOutMidway(t *testing.T) {
	e, k := pushEnv(t, pushTestPass)
	st := &signedOutStore{Fake: e.store, failAt: 3} // key and roster land, inventory hits the expired session
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if !errors.Is(err, vault.ErrSignedOut) {
		t.Fatalf("err = %v, want ErrSignedOut\n%s", err, out)
	}
	if st.putTries != 3 {
		t.Fatalf("kept writing after the session expired: %d tries", st.putTries)
	}
	names, _ := e.store.List(context.Background())
	if strings.Join(names, ",") != "key/ws1,roster" {
		t.Fatalf("landed: %v", names)
	}
	msg := err.Error()
	for _, want := range []string{"wrote: key/ws1, roster;", "not written: inventory, known_hosts", "lpass login", "nothing more was written", "fleet vault push"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in: %v", want, msg)
		}
	}
	assertNoSecret(t, out+msg, keyLines(k.private)...)
}

func TestPushStopsAtTheFirstWriteWhenSignedOut(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	st := &signedOutStore{Fake: e.store}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	_, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if !errors.Is(err, vault.ErrSignedOut) || st.putTries != 1 || !strings.Contains(err.Error(), "wrote: none;") {
		t.Fatalf("err=%v tries=%d", err, st.putTries)
	}
	if names, _ := e.store.List(context.Background()); len(names) != 0 {
		t.Fatalf("partial items: %v", names)
	}
}

func TestPushNeverIncludesAnExecDirective(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	fleetInventory = func() (vault.Inventory, error) {
		return vault.Inventory{Config: "Host a  #fleet\n    HostName 10.0.0.1\n    ProxyCommand nc -X 5 %h %p\n    LocalCommand touch /tmp/x\n"}, nil
	}
	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	inv := getItem[vault.Inventory](t, e.store, "inventory")
	low := strings.ToLower(inv.Config)
	if strings.Contains(low, "proxycommand") || strings.Contains(low, "localcommand") || strings.Contains(low, "nc -x") {
		t.Fatalf("exec directive reached the vault:\n%s", inv.Config)
	}
	if !strings.Contains(inv.Config, "HostName 10.0.0.1") {
		t.Fatalf("inert field lost:\n%s", inv.Config)
	}
	if !strings.Contains(out, "not imported: LocalCommand, ProxyCommand") {
		t.Fatalf("withheld directives not named:\n%s", out)
	}
}

func TestPushRefusesPartialKnownHostsWithoutFlag(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	fleetKnownHosts = func() (vault.KnownHosts, []unresolvedHost, error) {
		return vault.KnownHosts{Lines: []string{"a ssh-ed25519 AAAAKEYA"}}, []unresolvedHost{{Alias: "b", Reason: "no known_hosts entry", Names: []string{"b"}}, {Alias: "c", Reason: "ssh -G failed", Names: []string{"c"}}}, nil
	}
	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if err == nil {
		t.Fatalf("want non-zero exit with unresolved hosts:\n%s", out)
	}
	for _, want := range []string{"b: no known_hosts entry", "c: ssh -G failed", "ssh-keyscan", "--allow-partial"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	names, _ := e.store.List(context.Background())
	if slices.Contains(names, "known_hosts") {
		t.Fatal("partial known_hosts written without --allow-partial")
	}
	for _, n := range []string{"key/ws1", "roster", "inventory"} {
		if !slices.Contains(names, n) {
			t.Errorf("%s not written: %v", n, names)
		}
	}

	out, err = runVault(t, "y\n", "push", "--allow-partial")
	if err != nil {
		t.Fatalf("--allow-partial: %v\n%s", err, out)
	}
	kh := getItem[vault.KnownHosts](t, e.store, "known_hosts")
	if len(kh.Lines) != 1 {
		t.Fatalf("known_hosts: %v", kh.Lines)
	}
}

func TestPushCapturesAPlainKeyEncryptedInTheVaultOnly(t *testing.T) {
	e, k := pushEnv(t, "")
	out, err := runVault(t, "y\n", "push")
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	kp := getItem[vault.KeyPair](t, e.store, "key/ws1")
	if _, err := ssh.ParseRawPrivateKey(kp.Private); err == nil {
		t.Fatal("vault copy of the key is not encrypted")
	}
	if len(kp.Passphrase) < 43 {
		t.Fatalf("generated passphrase too short: %d", len(kp.Passphrase))
	}
	s, err := ssh.ParsePrivateKeyWithPassphrase(kp.Private, kp.Passphrase)
	if err != nil {
		t.Fatalf("vault copy does not open with its passphrase: %v", err)
	}
	if ssh.FingerprintSHA256(s.PublicKey()) != k.fp || kp.Fingerprint != k.fp {
		t.Fatal("vault holds a different key")
	}
	if b, _ := os.ReadFile(k.path); string(b) != string(k.private) {
		t.Fatal("local key changed without --encrypt-local")
	}
	if !strings.Contains(out, "--encrypt-local") {
		t.Fatalf("does not mention --encrypt-local:\n%s", out)
	}
	assertNoSecret(t, out, append(keyLines(k.private), string(kp.Passphrase))...)
}

func TestPushEncryptLocalRewritesTheKeyAfterTheVaultWrite(t *testing.T) {
	e, k := pushEnv(t, "")
	out, err := runVault(t, "y\n", "push", "--encrypt-local")
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	kp := getItem[vault.KeyPair](t, e.store, "key/ws1")
	enc, err := keystore.Encrypted(k.path)
	if err != nil || !enc {
		t.Fatalf("local key not encrypted: %v %v", enc, err)
	}
	local, _ := os.ReadFile(k.path)
	if _, err := ssh.ParsePrivateKeyWithPassphrase(local, kp.Passphrase); err != nil {
		t.Fatalf("local key does not open with the vault passphrase: %v", err)
	}
	// the plaintext backup is verified-then-removed (TestPushEncryptLocalDropsThePlaintextBackupOnceVerified)
	if !strings.Contains(out, "fleet keys unlock") {
		t.Fatalf("does not say the agent now needs unlock:\n%s", out)
	}
	assertNoSecret(t, out, append(keyLines(k.private), string(kp.Passphrase))...)
}

func TestPushEncryptLocalLeavesThePlainKeyWhenTheVaultWriteFails(t *testing.T) {
	e, k := pushEnv(t, "")
	st := &signedOutStore{Fake: e.store}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	if _, err := runVault(t, "y\n", "push", "--encrypt-local"); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(k.path); string(b) != string(k.private) {
		t.Fatal("local key encrypted although its passphrase never reached the vault")
	}
}

func TestPushWrongPassphraseWritesNothing(t *testing.T) {
	e, k := pushEnv(t, pushTestPass)
	out, err := runVault(t, "y\nnot-the-passphrase\n", "push")
	if err == nil {
		t.Fatalf("want error:\n%s", out)
	}
	if n := puts(e.store.Calls); n != 0 {
		t.Fatalf("wrote %d item(s) after a wrong passphrase", n)
	}
	assertNoSecret(t, out+err.Error(), append(keyLines(k.private), "not-the-passphrase")...)
}

func TestPushDeclinedWritesNothing(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	out, err := runVault(t, "n\n", "push")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if n := puts(e.store.Calls); n != 0 {
		t.Fatalf("wrote %d item(s) after 'n'", n)
	}
	if !strings.Contains(out, "nothing written") {
		t.Fatalf("missing 'nothing written':\n%s", out)
	}
}

func TestPushRefusesAPubThatDoesNotMatchTheKey(t *testing.T) {
	e, k := pushEnv(t, pushTestPass)
	other := writePushKey(t, t.TempDir(), "x")
	if err := os.WriteFile(k.path+".pub", []byte(other.pub+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	if n := puts(e.store.Calls); n != 0 {
		t.Fatalf("wrote %d item(s)", n)
	}
}

func TestPushShowsAFingerprintChangeAndRetiresTheOldKey(t *testing.T) {
	e, k := pushEnv(t, pushTestPass)
	var r vault.Roster
	r.Add("ws1", "ssh-ed25519 AAAAOLD", "SHA256:old", time.Now())
	r.Activate("SHA256:old", time.Now())
	putBlob(t, e.store, "roster", r)
	putBlob(t, e.store, "key/ws1", vault.KeyPair{Private: vault.Secret("PRIVATE-MATERIAL-old"), Fingerprint: "SHA256:old"})
	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"key/ws1: fingerprint SHA256:old → " + k.fp, "roster: retire ws1 SHA256:old"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	assertNoSecret(t, out)
	if kp := getItem[vault.KeyPair](t, e.store, "key/ws1"); kp.Fingerprint != k.fp {
		t.Fatalf("key not replaced: %s", kp.Fingerprint)
	}
}

func TestPushRefusesARetiredKey(t *testing.T) {
	e, k := pushEnv(t, pushTestPass)
	var r vault.Roster
	r.Add("ws1", k.pub, k.fp, time.Now())
	r.Activate(k.fp, time.Now())
	r.Add("ws1", "ssh-ed25519 AAAANEW", "SHA256:new", time.Now())
	r.Activate("SHA256:new", time.Now())
	putBlob(t, e.store, "roster", r)
	before := puts(e.store.Calls)
	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	if puts(e.store.Calls) != before {
		t.Fatal("wrote after finding a retired key")
	}
}

func TestPushSignsInFirstWhenSignedOut(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	st := &stubStore{Fake: e.store, readyErrs: []error{vault.ErrSignedOut, nil}}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	if out, err := runVault(t, "y\n"+pushTestPass+"\n", "push"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if st.signIns != 1 {
		t.Fatalf("signIns=%d", st.signIns)
	}
}

func TestPushNotConfigured(t *testing.T) {
	vaultEnv(t, true)
	out, err := runVault(t, "", "push")
	if err == nil || !strings.Contains(out, "not configured: run fleet vault setup") {
		t.Fatalf("err=%v out=%s", err, out)
	}
}

func TestPushAllowPartialKeepsTheVaultLinesOfUnresolvedHosts(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	hashedB := knownhosts.HashHostname("10.0.0.2") + " ssh-ed25519 AAAAHASHEDB"
	hashedZ := knownhosts.HashHostname("10.0.0.9") + " ssh-ed25519 AAAAHASHEDZ"
	putBlob(t, e.store, "known_hosts", vault.KnownHosts{Lines: []string{
		"a ssh-ed25519 AAAAOLDA",
		"b,10.0.0.2 ssh-ed25519 AAAAKEYB",
		"[b]:2222 ssh-ed25519 AAAAPORTB",
		hashedB,
		hashedZ,
		"z ssh-ed25519 AAAASTALE",
	}})
	fleetKnownHosts = func() (vault.KnownHosts, []unresolvedHost, error) {
		return vault.KnownHosts{Lines: []string{"a ssh-ed25519 AAAANEWA"}},
			[]unresolvedHost{{Alias: "b", Reason: "no known_hosts entry", Names: []string{"b", "10.0.0.2", "[b]:2222"}}}, nil
	}
	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push", "--allow-partial")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	kh := getItem[vault.KnownHosts](t, e.store, "known_hosts")
	got := map[string]bool{}
	for _, l := range kh.Lines {
		got[l] = true
	}
	for _, want := range []string{"a ssh-ed25519 AAAANEWA", "b,10.0.0.2 ssh-ed25519 AAAAKEYB", "[b]:2222 ssh-ed25519 AAAAPORTB", hashedB} {
		if !got[want] {
			t.Errorf("missing %q in %v", want, kh.Lines)
		}
	}
	for _, gone := range []string{"a ssh-ed25519 AAAAOLDA", hashedZ, "z ssh-ed25519 AAAASTALE"} {
		if got[gone] {
			t.Errorf("kept %q", gone)
		}
	}
	if !strings.Contains(out, "kept 3 existing vault line(s) for: b") {
		t.Fatalf("diff does not say b's lines are kept:\n%s", out)
	}
}

func TestPushEncryptLocalDropsThePlaintextBackupOnceVerified(t *testing.T) {
	_, k := pushEnv(t, "")
	out, err := runVault(t, "y\n", "push", "--encrypt-local")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if m, _ := filepath.Glob(k.path + ".bak-*"); len(m) != 0 {
		t.Fatalf("plaintext backup left on disk: %v", m)
	}
	if m, _ := filepath.Glob(k.path + ".pub.bak-*"); len(m) != 0 { // PR #452 item 6
		t.Fatalf(".pub backup left on disk: %v", m)
	}
	if !strings.Contains(out, "the vault holds the recovery copy") {
		t.Fatalf("missing recovery note:\n%s", out)
	}
}

func TestPushEncryptLocalKeepsTheBackupWhenVerificationFails(t *testing.T) {
	_, k := pushEnv(t, "")
	orig := verifyLocalEncrypted
	t.Cleanup(func() { verifyLocalEncrypted = orig })
	verifyLocalEncrypted = func(string, []byte, string) error { return errors.New("re-read mismatch") }
	out, err := runVault(t, "y\n", "push", "--encrypt-local")
	m, _ := filepath.Glob(k.path + ".bak-*")
	if len(m) != 1 {
		t.Fatalf("backup not kept: %v (err=%v)\n%s", m, err, out)
	}
	if b, _ := os.ReadFile(m[0]); string(b) != string(k.private) {
		t.Fatal("backup is not the plain key")
	}
	if !strings.Contains(out+fmt.Sprint(err), m[0]) || !strings.Contains(out+fmt.Sprint(err), "plaintext — delete it after `fleet keys unlock` works") {
		t.Fatalf("backup path/warning missing: %v\n%s", err, out)
	}
}

func TestPushEncryptLocalConflictRetryRewritesOnce(t *testing.T) {
	e, k := pushEnv(t, "")
	st := &conflictStore{Fake: e.store}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	out, err := runVault(t, "y\ny\n", "push", "--encrypt-local")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "re-stored under a new passphrase") {
		t.Fatalf("retry re-stored the key:\n%s", out)
	}
	if n := strings.Count(out, "encrypted "+k.path); n != 1 {
		t.Fatalf("local key rewritten %d times:\n%s", n, out)
	}
	kp := getItem[vault.KeyPair](t, st, "key/ws1")
	local, _ := os.ReadFile(k.path)
	if _, err := ssh.ParsePrivateKeyWithPassphrase(local, kp.Passphrase); err != nil {
		t.Fatalf("local key does not open with the vault passphrase: %v", err)
	}
}

func TestPushDeclinedWithWithheldKnownHostsExitsNonZero(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	fleetKnownHosts = func() (vault.KnownHosts, []unresolvedHost, error) {
		return vault.KnownHosts{Lines: []string{"a ssh-ed25519 AAAAKEYA"}}, []unresolvedHost{{Alias: "b", Reason: "no known_hosts entry", Names: []string{"b"}}}, nil
	}
	out, err := runVault(t, "n\n", "push")
	if err == nil {
		t.Fatalf("want non-zero exit:\n%s", out)
	}
	if puts(e.store.Calls) != 0 {
		t.Fatal("wrote after 'n'")
	}
}

// PR #452 item 1.
func TestPushHandsSignInTheRealStdinFile(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	st := &stubStore{Fake: e.store, readyErrs: []error{vault.ErrSignedOut, nil}}
	openVault = func(vault.Config) (vault.Store, error) { return st, nil }
	f := stdinFile(t, "y\n"+pushTestPass+"\n")
	c := newVaultCmd()
	var out strings.Builder
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetIn(f)
	c.SetArgs([]string{"push"})
	if err := c.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got, ok := st.signInStdin.(*os.File); !ok || got != f {
		t.Fatalf("SignIn got %T, want the *os.File itself", st.signInStdin)
	}
	// the prompt reader built after sign-in still read the answers
	if n := puts(e.store.Calls); n != 4 {
		t.Fatalf("wrote %d item(s), want 4\n%s", n, out.String())
	}
}

// PR #452 item 2: right after a push, status agrees there is nothing to push.
func TestStatusAgreesWithPushRightAfterAPush(t *testing.T) {
	_, _ = pushEnv(t, pushTestPass)
	fleetInventory = func() (vault.Inventory, error) {
		return vault.Inventory{Config: "Host a  #fleet\n    HostName 10.0.0.1\n    User pi\n    ProxyCommand nc %h %p\n\nHost b  #fleet\n    HostName 10.0.0.2\n"}, nil
	}
	if out, err := runVault(t, "y\n"+pushTestPass+"\n", "push"); err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	out, err := runVault(t, "", "status")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	for _, want := range []string{"inventory: in vault, in sync", "known_hosts: in vault, 0 lines differ"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if out, _ := runVault(t, "", "push"); !strings.Contains(out, "nothing to push") {
		t.Fatalf("push disagrees:\n%s", out)
	}
}

// PR #452 item 3: a fresh machine (no fleet alias resolves) can still push
// with --allow-partial; every vault known_hosts line is kept.
func TestPushAllowPartialOnAFreshHostKeepsEveryVaultLine(t *testing.T) {
	e, _ := pushEnv(t, pushTestPass)
	vaultLines := []string{"a ssh-ed25519 AAAAKEYA", "b ssh-ed25519 AAAAKEYB", "old ssh-ed25519 AAAAOLD"}
	putBlob(t, e.store, "known_hosts", vault.KnownHosts{Lines: vaultLines})
	unresolved := []unresolvedHost{{Alias: "a", Reason: "no known_hosts entry", Names: []string{"a"}}, {Alias: "b", Reason: "no known_hosts entry", Names: []string{"b"}}}
	fleetKnownHosts = func() (vault.KnownHosts, []unresolvedHost, error) {
		return vault.KnownHosts{}, unresolved, fmt.Errorf("%w (first: a: no known_hosts entry)", errNoAliasResolved)
	}

	out, err := runVault(t, "y\n"+pushTestPass+"\n", "push")
	if err == nil || !strings.Contains(out, "known_hosts: NOT pushed") || !strings.Contains(out, "--allow-partial") {
		t.Fatalf("without the flag: want withheld + non-zero: %v\n%s", err, out)
	}

	out, err = runVault(t, "", "push", "--allow-partial")
	if err != nil {
		t.Fatalf("--allow-partial: %v\n%s", err, out)
	}
	if !strings.Contains(out, "keeps all 3 vault line(s)") {
		t.Fatalf("does not report the kept lines:\n%s", out)
	}
	kh := getItem[vault.KnownHosts](t, e.store, "known_hosts")
	if strings.Join(kh.Lines, "|") != strings.Join(vaultLines, "|") {
		t.Fatalf("vault lines changed: %v", kh.Lines)
	}
}

// PR #452 item 8: push says it never removes hosts.
func TestPushSaysItNeverRemovesHosts(t *testing.T) {
	_, _ = pushEnv(t, pushTestPass)
	const note = "push adds and updates hosts; it never removes them from the vault"
	out, err := runVault(t, "n\n", "push")
	if err != nil || !strings.Contains(out, note) {
		t.Fatalf("diff lacks the note: %v\n%s", err, out)
	}
	if !strings.Contains(newVaultPushCmd().Long, note) {
		t.Fatal("help lacks the note")
	}
}

// PR #452 item 7: one symmetric-difference helper.
func TestLineSetDiff(t *testing.T) {
	add, del := lineSetDiff([]string{"a", "b", "b", "c"}, []string{"c", "d"})
	if add != 2 || del != 1 {
		t.Fatalf("add=%d del=%d", add, del)
	}
}
