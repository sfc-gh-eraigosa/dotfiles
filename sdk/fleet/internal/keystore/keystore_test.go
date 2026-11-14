package keystore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

var testPass = []byte("test-passphrase-not-secret")

func TestGenerateIsEncryptedWithThePassphrase(t *testing.T) {
	priv, pub, err := Generate(testPass, "fleet-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(pub), "ssh-ed25519 ") || !strings.HasSuffix(string(pub), " fleet-test\n") {
		t.Fatalf("unexpected public key shape")
	}
	p := filepath.Join(t.TempDir(), "id")
	if err := os.WriteFile(p, priv, 0o600); err != nil {
		t.Fatal(err)
	}
	if enc, err := Encrypted(p); err != nil || !enc {
		t.Fatalf("Encrypted = %v, %v; want true", enc, err)
	}
	if _, err := ssh.ParseRawPrivateKeyWithPassphrase(priv, testPass); err != nil {
		t.Fatalf("right passphrase failed: %v", err)
	}
	if _, err := ssh.ParseRawPrivateKeyWithPassphrase(priv, []byte("wrong")); err == nil {
		t.Fatal("wrong passphrase unexpectedly parsed")
	}
}

func TestGenerateRejectsEmptyPassphrase(t *testing.T) {
	if _, _, err := Generate(nil, "x"); err == nil {
		t.Fatal("want error for empty passphrase")
	}
}

func TestWriteBacksUpAnExistingKeyAndSetsModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ssh")
	path := filepath.Join(dir, "id_ed25519")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	priv1, pub1, _ := Generate(testPass, "one")
	bak, err := Write(path, priv1, pub1, now)
	if err != nil || bak != "" {
		t.Fatalf("first write: bak=%q err=%v", bak, err)
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", fi.Mode().Perm())
	}

	priv2, pub2, _ := Generate(testPass, "two")
	bak, err = Write(path, priv2, pub2, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := path + ".bak-20260102T030405"; bak != want {
		t.Fatalf("bak = %q want %q", bak, want)
	}
	check := func(p string, mode os.FileMode, content []byte) {
		t.Helper()
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != mode {
			t.Fatalf("%s mode %v want %v", filepath.Base(p), fi.Mode().Perm(), mode)
		}
		got, _ := os.ReadFile(p)
		if string(got) != string(content) {
			t.Fatalf("%s content mismatch", filepath.Base(p))
		}
	}
	check(path, 0o600, priv2)
	check(path+".pub", 0o644, pub2)
	check(bak, 0o600, priv1)
	check(path+".pub.bak-20260102T030405", 0o600, pub1)
}

func TestWriteLeavesExistingDirModeAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	priv, pub, _ := Generate(testPass, "c")
	if _, err := Write(filepath.Join(dir, "k"), priv, pub, time.Now()); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o755 {
		t.Fatalf("dir mode changed to %v", fi.Mode().Perm())
	}
}

func TestEncryptedFalseForAPlainKey(t *testing.T) {
	// A throwaway unencrypted key generated at test time, in-process.
	_, _, err := Generate(testPass, "x")
	if err != nil {
		t.Fatal(err)
	}
	plain := plainTestKey(t)
	p := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(p, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	if enc, err := Encrypted(p); err != nil || enc {
		t.Fatalf("Encrypted = %v, %v; want false", enc, err)
	}
}

func TestEncryptedErrorsOnGarbageAndMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "junk")
	_ = os.WriteFile(p, []byte("not a key"), 0o600)
	if _, err := Encrypted(p); err == nil {
		t.Fatal("want error for garbage")
	}
	if _, err := Encrypted(p + "-missing"); err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestFingerprintRejectsGarbage(t *testing.T) {
	if _, err := Fingerprint([]byte("nope")); err == nil {
		t.Fatal("want error")
	}
}

func TestFingerprintMatchesSshKeygen(t *testing.T) {
	bin, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
	_, pub, _ := Generate(testPass, "fp")
	fp, err := Fingerprint(pub)
	if err != nil || !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("fp=%q err=%v", fp, err)
	}
	pubFile := filepath.Join(t.TempDir(), "k.pub")
	_ = os.WriteFile(pubFile, pub, 0o644)
	out, err := exec.Command(bin, "-lf", pubFile).Output() // public file only
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 || fields[1] != fp {
		t.Fatalf("ssh-keygen fingerprint differs from ours")
	}
}

func TestWriteSameSecondKeepsEveryEarlierKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var privs [][]byte
	var baks []string
	for i := 0; i < 3; i++ {
		priv, pub, _ := Generate(testPass, "c")
		bak, err := Write(path, priv, pub, now)
		if err != nil {
			t.Fatal(err)
		}
		privs = append(privs, priv)
		baks = append(baks, bak)
	}
	if baks[0] != "" || baks[1] != path+".bak-20260102T030405" || baks[2] != path+".bak-20260102T030405-1" {
		t.Fatalf("backup names: %q", baks)
	}
	for i := 1; i < 3; i++ {
		got, err := os.ReadFile(baks[i])
		if err != nil || string(got) != string(privs[i-1]) {
			t.Fatalf("backup %d lost the earlier key (err=%v)", i, err)
		}
	}
}

func TestWriteDoesNotFollowAPlantedBackupSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "id")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	priv1, pub1, _ := Generate(testPass, "c")
	if _, err := Write(path, priv1, pub1, now); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(dir, "victim")
	_ = os.WriteFile(victim, []byte("untouched"), 0o644)
	if err := os.Symlink(victim, path+".bak-20260102T030405"); err != nil {
		t.Fatal(err)
	}
	priv2, pub2, _ := Generate(testPass, "c")
	bak, err := Write(path, priv2, pub2, now)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(victim); string(got) != "untouched" {
		t.Fatal("symlink target was written through")
	}
	if got, _ := os.ReadFile(bak); string(got) != string(priv1) {
		t.Fatal("backup does not hold the old key")
	}
}

func TestWriteReportsBackupWhenPubFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	priv1, pub1, _ := Generate(testPass, "c")
	if _, err := Write(path, priv1, pub1, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path + ".pub"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".pub", 0o700); err != nil { // makes the .pub step fail
		t.Fatal(err)
	}
	priv2, pub2, _ := Generate(testPass, "c")
	bak, err := Write(path, priv2, pub2, now)
	if err == nil || !strings.Contains(err.Error(), ".pub") {
		t.Fatalf("want an error mentioning .pub, got %v", err)
	}
	if bak == "" {
		t.Fatal("backup path dropped on error")
	}
	if got, _ := os.ReadFile(bak); string(got) != string(priv1) {
		t.Fatal("reported backup does not hold the old key")
	}
}
