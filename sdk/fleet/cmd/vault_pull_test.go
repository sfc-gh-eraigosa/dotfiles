package cmd

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/agent"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/keystore"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
)

const pullPass = "correct horse battery"

// pullEnv is vaultEnv plus a saved vault.yaml, an ssh config path inside the
// temp HOME, a GOOS seam and an agent-loader spy.
type pullEnvT struct {
	vaultEnvT
	loads   []vault.Secret
	loadKey []string
	goos    string
}

func pullEnv(t *testing.T) *pullEnvT {
	t.Helper()
	e := &pullEnvT{vaultEnvT: vaultEnv(t, true), goos: "linux"}
	oc, og, ol := flagConfig, vaultGOOS, agentLoad
	t.Cleanup(func() { flagConfig, vaultGOOS, agentLoad = oc, og, ol })
	flagConfig = filepath.Join(e.home, ".ssh", "config")
	vaultGOOS = func() string { return e.goos }
	agentLoad = func(_ context.Context, keyPath string, pass vault.Secret) error {
		e.loads = append(e.loads, pass)
		e.loadKey = append(e.loadKey, keyPath)
		return nil
	}
	if err := saveVaultConfig(vaultConfigPath(), vault.Config{Provider: "lpass", Workstation: "ws1"}); err != nil {
		t.Fatal(err)
	}
	return e
}

// seedKey puts a real encrypted key under key/<ws> and returns its public text
// and fingerprint.
func (e *pullEnvT) seedKey(t *testing.T, ws string) (pub, fp string) {
	t.Helper()
	priv, pb, err := keystore.Generate([]byte(pullPass), ws)
	if err != nil {
		t.Fatal(err)
	}
	fp, err = keystore.Fingerprint(pb)
	if err != nil {
		t.Fatal(err)
	}
	b, err := vault.Encode(vault.KeyPair{Private: priv, Passphrase: vault.Secret(pullPass), Public: string(pb), Fingerprint: fp})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Put(context.Background(), "key/"+ws, b, ""); err != nil {
		t.Fatal(err)
	}
	return string(pb), fp
}

func (e *pullEnvT) seed(t *testing.T, name string, v any) {
	t.Helper()
	b, err := vault.Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Put(context.Background(), name, b, ""); err != nil {
		t.Fatal(err)
	}
}

func (e *pullEnvT) ssh(parts ...string) string {
	return filepath.Join(append([]string{e.home, ".ssh"}, parts...)...)
}

func backups(t *testing.T, dir, prefix string) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, prefix+".bak-*"))
	return m
}

const pullInv = "Host box  #fleet\n    HostName 10.0.0.9\n    User me\n"

func TestPullRestoresKeyConfigAndKnownHosts(t *testing.T) {
	e := pullEnv(t)
	_, fp := e.seedKey(t, "ws1")
	e.seed(t, "inventory", vault.Inventory{Config: pullInv})
	e.seed(t, "known_hosts", vault.KnownHosts{Lines: []string{"box ssh-ed25519 AAAAC3Nza"}})
	if err := os.MkdirAll(e.ssh(), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "Host mine\n    HostName 1.2.3.4\n"
	if err := os.WriteFile(e.ssh("config"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.ssh("known_hosts"), []byte("old ssh-ed25519 OLD\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runVault(t, "", "pull")
	if err != nil {
		t.Fatalf("pull: %v\n%s", err, out)
	}
	key := e.ssh("id_ed25519")
	fi, err := os.Stat(key)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key stat %v %v", fi, err)
	}
	if enc, err := keystore.Encrypted(key); err != nil || !enc {
		t.Fatalf("key must be encrypted: %v %v", enc, err)
	}
	pub, _ := os.ReadFile(key + ".pub")
	if got, _ := keystore.Fingerprint(pub); got != fp {
		t.Fatalf("fp %s want %s", got, fp)
	}
	if di, _ := os.Stat(e.ssh()); di.Mode().Perm() != 0o700 {
		t.Fatalf("~/.ssh mode %o", di.Mode().Perm())
	}
	cfg, _ := os.ReadFile(e.ssh("config"))
	if !strings.Contains(string(cfg), "Host mine") || !strings.Contains(string(cfg), "Host box") || !strings.Contains(string(cfg), "10.0.0.9") {
		t.Fatalf("config not merged:\n%s", cfg)
	}
	if len(backups(t, e.ssh(), "config")) != 1 {
		t.Fatalf("config backup missing: %v", backups(t, e.ssh(), "config"))
	}
	kh, _ := os.ReadFile(e.ssh("known_hosts"))
	if !strings.Contains(string(kh), "old ssh-ed25519 OLD") || !strings.Contains(string(kh), "box ssh-ed25519 AAAAC3Nza") {
		t.Fatalf("known_hosts:\n%s", kh)
	}
	if len(backups(t, e.ssh(), "known_hosts")) != 1 {
		t.Fatal("known_hosts backup missing")
	}
	if strings.Contains(out, pullPass) {
		t.Fatalf("passphrase printed:\n%s", out)
	}
	// a second pull is a no-op: no new key backup, no duplicate known_hosts line
	if out, err := runVault(t, "", "pull"); err != nil || !strings.Contains(out, "unchanged") {
		t.Fatalf("second pull: %v\n%s", err, out)
	}
	kh, _ = os.ReadFile(e.ssh("known_hosts"))
	if strings.Count(string(kh), "box ssh-ed25519") != 1 {
		t.Fatalf("duplicate known_hosts line:\n%s", kh)
	}
	if len(backups(t, e.ssh(), "id_ed25519")) != 0 {
		t.Fatal("unchanged key must not be backed up")
	}
}

func TestPullRefusesToReplaceADifferentKeyWithoutTheFlag(t *testing.T) {
	e := pullEnv(t)
	_, vfp := e.seedKey(t, "ws1")
	e.seed(t, "inventory", vault.Inventory{Config: pullInv})
	priv, pb, _ := keystore.Generate([]byte("other"), "local")
	if _, err := keystore.Write(e.ssh("id_ed25519"), priv, pb, vaultNow()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(e.ssh("id_ed25519"))

	out, err := runVault(t, "", "pull")
	if err == nil || !strings.Contains(out+err.Error(), "--replace-key") {
		t.Fatalf("want refusal naming --replace-key: err=%v\n%s", err, out)
	}
	after, _ := os.ReadFile(e.ssh("id_ed25519"))
	if string(before) != string(after) {
		t.Fatal("local key was modified without --replace-key")
	}
	if _, err := os.Stat(e.ssh("config")); err == nil {
		t.Fatal("nothing may be written when the pull is refused")
	}

	out, err = runVault(t, "", "pull", "--replace-key")
	if err != nil {
		t.Fatalf("replace: %v\n%s", err, out)
	}
	pub, _ := os.ReadFile(e.ssh("id_ed25519.pub"))
	if got, _ := keystore.Fingerprint(pub); got != vfp {
		t.Fatalf("key not replaced: %s", got)
	}
	if len(backups(t, e.ssh(), "id_ed25519")) != 1 {
		t.Fatal("old key must be backed up")
	}
}

func TestPullAsRestoresAnotherName(t *testing.T) {
	e := pullEnv(t)
	_, fpOther := e.seedKey(t, "other")
	if out, err := runVault(t, "", "pull"); err != nil || !strings.Contains(out, "fleet keys rotate --new") {
		t.Fatalf("default name has no key: %v\n%s", err, out)
	}
	if _, err := os.Stat(e.ssh("id_ed25519")); err == nil {
		t.Fatal("no key should be restored for ws1")
	}
	if out, err := runVault(t, "", "pull", "--as", "other"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	pub, _ := os.ReadFile(e.ssh("id_ed25519.pub"))
	if got, _ := keystore.Fingerprint(pub); got != fpOther {
		t.Fatalf("fp %s want %s", got, fpOther)
	}
}

func TestPullWithNoKeyOffersRotateNew(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "elsewhere")
	e.seed(t, "inventory", vault.Inventory{Config: pullInv})
	out, err := runVault(t, "", "pull")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, w := range []string{"fleet keys rotate --new", "then run `fleet keys sync` on a trusted workstation", "elsewhere"} {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q:\n%s", w, out)
		}
	}
	if len(e.loads) != 0 {
		t.Fatal("no key, nothing to load")
	}
	if cfg, _ := os.ReadFile(e.ssh("config")); !strings.Contains(string(cfg), "Host box") {
		t.Fatal("inventory should still be restored")
	}
}

func TestPullOnDarwinOffersUseKeychain(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "ws1")
	e.goos = "darwin"

	out, err := runVault(t, "n\n", "pull")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if cfg, _ := os.ReadFile(e.ssh("config")); strings.Contains(string(cfg), "UseKeychain") {
		t.Fatalf("declined, yet added:\n%s", cfg)
	}
	if _, err := runVault(t, "y\n", "pull"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(e.ssh("config"))
	for _, w := range []string{"Host *", "UseKeychain yes", "AddKeysToAgent yes"} {
		if !strings.Contains(string(cfg), w) {
			t.Fatalf("missing %q:\n%s", w, cfg)
		}
	}
	// already present: no second prompt, no second block
	out, _ = runVault(t, "y\n", "pull")
	if strings.Contains(out, "UseKeychain") && strings.Contains(out, "[y/N]") {
		t.Fatalf("asked again:\n%s", out)
	}
	cfg, _ = os.ReadFile(e.ssh("config"))
	if strings.Count(string(cfg), "UseKeychain yes") != 1 {
		t.Fatalf("duplicated:\n%s", cfg)
	}

	// never on linux, even with an eager stdin
	e.goos = "linux"
	_ = os.Remove(e.ssh("config")) // may already be absent
	out, _ = runVault(t, "y\n", "pull")
	if cfg, _ := os.ReadFile(e.ssh("config")); strings.Contains(string(cfg), "UseKeychain") || strings.Contains(out, "UseKeychain") {
		t.Fatalf("linux got a keychain block:\n%s\n%s", cfg, out)
	}
}

func TestPullLoadsTheKeyIntoTheAgent(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "ws1")
	calls := 0
	agentLoaded = func(context.Context, string) (bool, error) { calls++; return calls > 1, nil }
	out, err := runVault(t, "", "pull")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(e.loads) != 1 || string(e.loads[0]) != pullPass {
		t.Fatalf("spy got %v", e.loads)
	}
	if e.loadKey[0] != e.ssh("id_ed25519") {
		t.Fatalf("key path %s", e.loadKey[0])
	}
	if !strings.Contains(out, "loaded in agent") {
		t.Fatalf("out:\n%s", out)
	}
	// already loaded: not loaded again
	e.loads = nil
	agentLoaded = func(context.Context, string) (bool, error) { return true, nil }
	if _, err := runVault(t, "", "pull"); err != nil || len(e.loads) != 0 {
		t.Fatalf("reloaded: %v %v", err, e.loads)
	}
	// no agent: skipped with a note, not an error
	agentLoaded = func(context.Context, string) (bool, error) { return false, agent.ErrNoAgent }
	out, err = runVault(t, "", "pull")
	if err != nil || len(e.loads) != 0 || !strings.Contains(out, "no agent") {
		t.Fatalf("no agent: %v %v\n%s", err, e.loads, out)
	}
}

// ---- PR #457 review fixes ----

func TestPullIsAllOrNothing(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "ws1")
	if _, err := e.store.Put(context.Background(), "inventory", vault.Secret("{not json"), ""); err != nil {
		t.Fatal(err)
	}
	priv, pb, _ := keystore.Generate([]byte("other"), "local")
	if _, err := keystore.Write(e.ssh("id_ed25519"), priv, pb, vaultNow()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(e.ssh("id_ed25519"))
	if out, err := runVault(t, "", "pull", "--replace-key"); err == nil {
		t.Fatalf("corrupt inventory must fail the pull\n%s", out)
	}
	if after, _ := os.ReadFile(e.ssh("id_ed25519")); string(after) != string(before) {
		t.Fatal("key was replaced although the inventory item was corrupt")
	}
	if len(backups(t, e.ssh(), "id_ed25519")) != 0 {
		t.Fatal("no backup/write may happen before every item is planned")
	}
}

func TestPullDarwinBlockIgnoresUnknownUseKeychainFirst(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "ws1")
	e.goos = "darwin"
	if _, err := runVault(t, "y\n", "pull"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(e.ssh("config"))
	s := string(cfg)
	iu, uk := strings.Index(s, "IgnoreUnknown UseKeychain"), strings.Index(s, "UseKeychain yes")
	if iu < 0 || uk < 0 || iu > uk {
		t.Fatalf("IgnoreUnknown must precede UseKeychain:\n%s", s)
	}
}

func TestPullAsBecomesThatWorkstation(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "laptop")
	out, err := runVault(t, "", "pull", "--as", "laptop")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	cfg, ok, err := loadVaultConfig(vaultConfigPath())
	if err != nil || !ok || cfg.Workstation != "laptop" {
		t.Fatalf("cfg=%+v ok=%v err=%v", cfg, ok, err)
	}
	if !strings.Contains(out, "laptop") || !strings.Contains(out, "workstation") {
		t.Fatalf("not announced:\n%s", out)
	}
}

func hashedLine(host string) string {
	salt := []byte("0123456789abcdefghij")
	m := hmac.New(sha1.New, salt)
	m.Write([]byte(host))
	return "|1|" + base64.StdEncoding.EncodeToString(salt) + "|" + base64.StdEncoding.EncodeToString(m.Sum(nil)) + " ssh-ed25519 LOCALKEY"
}

func TestPullKnownHostsKeepsRotatedAndHashedLocalEntries(t *testing.T) {
	e := pullEnv(t)
	e.seed(t, "known_hosts", vault.KnownHosts{Lines: []string{
		"rotated ssh-ed25519 OLDVAULT",
		"hashedbox ssh-ed25519 VAULTKEY",
		"[gone]:2222 ssh-ed25519 NEWHOST",
		"@revoked badhost ssh-ed25519 REVOKED",
	}})
	if err := os.MkdirAll(e.ssh(), 0o700); err != nil {
		t.Fatal(err)
	}
	local := "rotated ssh-ed25519 NEWLOCAL\n" + hashedLine("hashedbox") + "\n"
	if err := os.WriteFile(e.ssh("known_hosts"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runVault(t, "", "pull"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	b, _ := os.ReadFile(e.ssh("known_hosts"))
	s := string(b)
	if strings.Contains(s, "OLDVAULT") {
		t.Fatalf("rotated host's old vault key re-added:\n%s", s)
	}
	if strings.Contains(s, "VAULTKEY") {
		t.Fatalf("hashed local entry duplicated:\n%s", s)
	}
	if !strings.Contains(s, "[gone]:2222 ssh-ed25519 NEWHOST") || !strings.Contains(s, "@revoked badhost") {
		t.Fatalf("absent host / revoked line missing:\n%s", s)
	}
	if fi, _ := os.Stat(e.ssh("known_hosts")); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode not preserved: %o", fi.Mode().Perm())
	}
	if tmp, _ := filepath.Glob(e.ssh(".known_hosts.tmp-*")); len(tmp) != 0 {
		t.Fatalf("temp file left: %v", tmp)
	}
	if len(backups(t, e.ssh(), "known_hosts")) != 1 {
		t.Fatal("backup missing")
	}
}

func TestPullNoBackupNoticeWithoutPriorConfig(t *testing.T) {
	e := pullEnv(t)
	e.seed(t, "inventory", vault.Inventory{Config: pullInv})
	out, err := runVault(t, "", "pull")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "backup was written") {
		t.Fatalf("claims a backup that was never taken:\n%s", out)
	}
	if out, _ = runVault(t, "", "pull"); strings.Contains(out, "updated") {
		t.Fatalf("second pull should change nothing:\n%s", out)
	}
}

func TestPullDoesNotChmodASymlinkedSSHDir(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "ws1")
	target := filepath.Join(e.home, "real-ssh")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, e.ssh()); err != nil {
		t.Fatal(err)
	}
	out, err := runVault(t, "", "pull")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o755 {
		t.Fatalf("symlink target chmodded to %o", fi.Mode().Perm())
	}
	if !strings.Contains(out, "warning") {
		t.Fatalf("want a group/world-accessible warning:\n%s", out)
	}
}

func TestPullValidatesTheVaultKeyAndComparesPrivateBytes(t *testing.T) {
	e := pullEnv(t)
	// vault key whose Private does not match its Public
	priv, _, _ := keystore.Generate([]byte(pullPass), "a")
	_, pubB, _ := keystore.Generate([]byte(pullPass), "b")
	fp, _ := keystore.Fingerprint(pubB)
	e.seed(t, "key/ws1", vault.KeyPair{Private: priv, Passphrase: vault.Secret(pullPass), Public: string(pubB), Fingerprint: fp})
	out, err := runVault(t, "", "pull")
	if err == nil || !strings.Contains(out+err.Error(), "does not match") {
		t.Fatalf("mismatched vault key must be refused: %v\n%s", err, out)
	}
	if _, serr := os.Stat(e.ssh("id_ed25519")); serr == nil {
		t.Fatal("nothing may be written")
	}

	// local .pub equals the vault's but the private half differs: not "unchanged"
	e2 := pullEnv(t)
	pub, _ := e2.seedKey(t, "ws1")
	lpriv, _, _ := keystore.Generate([]byte("zzz"), "local")
	if _, err := keystore.Write(e2.ssh("id_ed25519"), lpriv, []byte(pub), vaultNow()); err != nil {
		t.Fatal(err)
	}
	out, err = runVault(t, "", "pull")
	if err == nil || !strings.Contains(out+err.Error(), "--replace-key") {
		t.Fatalf("differing private key behind a matching .pub must need --replace-key: %v\n%s", err, out)
	}
}

// ---- PR #457 fix round 2 ----

func hashedWith(host, salt, key string) string {
	m := hmac.New(sha1.New, []byte(salt))
	m.Write([]byte(host))
	return "|1|" + base64.StdEncoding.EncodeToString([]byte(salt)) + "|" + base64.StdEncoding.EncodeToString(m.Sum(nil)) + " ssh-ed25519 " + key
}

const piInv = "Host pi  #fleet\n    HostName 10.0.0.5\n    User me\n"

func readKH(e *pullEnvT) string { b, _ := os.ReadFile(e.ssh("known_hosts")); return string(b) }

func TestPullHashedVaultLineForRotatedHostIsNotReAdded(t *testing.T) {
	e := pullEnv(t)
	e.seed(t, "inventory", vault.Inventory{Config: piInv})
	e.seed(t, "known_hosts", vault.KnownHosts{Lines: []string{hashedWith("pi", "saltsaltsaltsaltsalt", "OLD")}})
	if err := os.MkdirAll(e.ssh(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.ssh("known_hosts"), []byte("pi ssh-ed25519 NEW\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runVault(t, "", "pull"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if s := readKH(e); strings.Contains(s, "OLD") || !strings.Contains(s, "NEW") {
		t.Fatalf("old hashed key re-trusted:\n%s", s)
	}
}

func TestPullHashedVaultLineForAbsentHostIsAddedOnceAndSecondPullIsANoOp(t *testing.T) {
	e := pullEnv(t)
	e.seed(t, "inventory", vault.Inventory{Config: piInv})
	line := hashedWith("pi", "saltsaltsaltsaltsalt", "VKEY")
	e.seed(t, "known_hosts", vault.KnownHosts{Lines: []string{line}})
	if out, err := runVault(t, "", "pull"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(readKH(e), line) {
		t.Fatalf("hashed line for an absent fleet host not added:\n%s", readKH(e))
	}
	before := readKH(e)
	// same host+key under a different local salt must not be re-added either
	if err := os.WriteFile(e.ssh("known_hosts"), []byte(hashedWith("pi", "othersaltothersalt!", "VKEY")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runVault(t, "", "pull")
	if err != nil || !strings.Contains(out, "known_hosts: unchanged") {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Count(readKH(e), "VKEY") != 1 {
		t.Fatalf("re-added under a new salt:\n%s", readKH(e))
	}
	_ = before
	if n := len(backups(t, e.ssh(), "known_hosts")); n != 0 && n != 1 {
		t.Fatalf("unexpected backups %d", n)
	}
}

func TestPullSkipsHashedLineMatchingNoFleetHost(t *testing.T) {
	e := pullEnv(t)
	e.seed(t, "inventory", vault.Inventory{Config: piInv})
	e.seed(t, "known_hosts", vault.KnownHosts{Lines: []string{hashedWith("stranger", "saltsaltsaltsaltsalt", "XKEY")}})
	out, err := runVault(t, "", "pull")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readKH(e), "XKEY") {
		t.Fatal("unmatched hashed line appended blindly")
	}
	if !strings.Contains(out, "skipped 1 hashed") {
		t.Fatalf("no notice:\n%s", out)
	}
}

func TestPullAsWithATypoDoesNotRenameThisMachine(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "ws1")
	if _, err := runVault(t, "", "pull", "--as", "nosuch"); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := loadVaultConfig(vaultConfigPath())
	if cfg.Workstation != "ws1" {
		t.Fatalf("renamed to %q", cfg.Workstation)
	}
}

func TestPullWritesThroughSymlinkedKnownHostsAndConfig(t *testing.T) {
	e := pullEnv(t)
	e.seed(t, "inventory", vault.Inventory{Config: pullInv})
	e.seed(t, "known_hosts", vault.KnownHosts{Lines: []string{"box ssh-ed25519 K"}})
	real := filepath.Join(e.home, "dots")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(e.ssh(), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"known_hosts", "config"} {
		if err := os.WriteFile(filepath.Join(real, n), []byte("# x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(real, n), e.ssh(n)); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := runVault(t, "", "pull"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, n := range []string{"known_hosts", "config"} {
		if fi, err := os.Lstat(e.ssh(n)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s symlink replaced", n)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(real, "known_hosts")); !strings.Contains(string(b), "box ssh-ed25519 K") {
		t.Fatal("target not updated")
	}
	if b, _ := os.ReadFile(filepath.Join(real, "config")); !strings.Contains(string(b), "Host box") {
		t.Fatal("config target not updated")
	}
}

func TestPullDoesNotChmodHome(t *testing.T) {
	e := pullEnv(t)
	e.seedKey(t, "ws1")
	if err := os.Chmod(e.home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveVaultConfig(vaultConfigPath(), vault.Config{Provider: "lpass", Workstation: "ws1", Key: filepath.Join(e.home, "id_x")}); err != nil {
		t.Fatal(err)
	}
	if out, err := runVault(t, "", "pull"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if fi, _ := os.Stat(e.home); fi.Mode().Perm() != 0o755 {
		t.Fatalf("$HOME chmodded to %o", fi.Mode().Perm())
	}
}
