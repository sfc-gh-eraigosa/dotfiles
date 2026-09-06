package ghapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func testPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func TestFileStoreSaveCreatesPrivateDirAndFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ghapp")
	s := FileStore{Dir: dir}

	pemPath, err := s.SavePEM("my-app", testPEM(t))
	if err != nil {
		t.Fatal(err)
	}
	apps := Apps{"my-app": {ID: 42, Slug: "my-app", PEMPath: pemPath, Installations: map[string]int64{"o/r": 7}}}
	if err := s.Save(apps); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o700 {
		t.Errorf("dir mode = %o, want 0700", got)
	}
	for _, p := range []string{pemPath, filepath.Join(dir, "apps.json")} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := st.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %o, want 0600", filepath.Base(p), got)
		}
	}
	if pemPath != filepath.Join(dir, "my-app.pem") {
		t.Errorf("pem path = %s, want <dir>/my-app.pem", pemPath)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	s := FileStore{Dir: filepath.Join(t.TempDir(), "ghapp")}
	pemPath, err := s.SavePEM("a", testPEM(t))
	if err != nil {
		t.Fatal(err)
	}
	want := Apps{"a": {ID: 1, Slug: "a", PEMPath: pemPath, Installations: map[string]int64{"o": 2, "o/r": 3}}}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["a"].ID != 1 || got["a"].Slug != "a" || got["a"].PEMPath != pemPath ||
		got["a"].Installations["o"] != 2 || got["a"].Installations["o/r"] != 3 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestFileStoreLoadMissingIsEmpty(t *testing.T) {
	s := FileStore{Dir: filepath.Join(t.TempDir(), "nope")}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty apps, got %+v", got)
	}
}

func TestFileStoreLoadRefusesWorldReadablePEM(t *testing.T) {
	s := FileStore{Dir: filepath.Join(t.TempDir(), "ghapp")}
	pemPath, err := s.SavePEM("a", testPEM(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Apps{"a": {ID: 1, Slug: "a", PEMPath: pemPath}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(pemPath, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = s.Load()
	if err == nil {
		t.Fatal("want error for 0644 PEM, got nil")
	}
	if !contains(err.Error(), "a.pem") || !contains(err.Error(), "0644") {
		t.Fatalf("error should name the file and mode, got: %v", err)
	}
}

func TestFileStoreSaveRejectsBadSlug(t *testing.T) {
	s := FileStore{Dir: filepath.Join(t.TempDir(), "ghapp")}
	for _, bad := range []string{"", "../x", "a/b", "a b"} {
		if _, err := s.SavePEM(bad, testPEM(t)); err == nil {
			t.Errorf("SavePEM(%q): want error, got nil", bad)
		}
	}
}

func TestFileStoreLoadRejectsCorruptJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ghapp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "apps.json"), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (FileStore{Dir: dir}).Load(); err == nil {
		t.Fatal("want parse error, got nil")
	}
}

func TestDefaultStoreDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	if got := DefaultDir(); got != "/tmp/fakehome/.config/ghapp" {
		t.Fatalf("DefaultDir() = %s", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got := DefaultDir(); got != "/tmp/xdg/ghapp" {
		t.Fatalf("DefaultDir() with XDG = %s", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestFileStoreLoadErrorsWhenPEMMissing(t *testing.T) {
	s := FileStore{Dir: filepath.Join(t.TempDir(), "ghapp")}
	if err := s.Save(Apps{"a": {ID: 1, Slug: "a", PEMPath: filepath.Join(s.Dir, "gone.pem")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil || !contains(err.Error(), "gone.pem") {
		t.Fatalf("want error naming the missing PEM, got %v", err)
	}
}

func TestFileStoreSaveFailsWhenDirIsAFile(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "ghapp")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := FileStore{Dir: blocker}
	if err := s.Save(Apps{}); err == nil {
		t.Fatal("want error when the store dir is a regular file")
	}
	if _, err := s.SavePEM("a", []byte("k")); err == nil {
		t.Fatal("want SavePEM error when the store dir is a regular file")
	}
}
