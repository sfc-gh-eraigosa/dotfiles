package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/config"
)

// TestSave_PrivateModes: config.json is the user's private state — the file is
// 0600 and the directory Save creates is 0700 (security-scan-green, G301/G306).
func TestSave_PrivateModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gsl", "config.json")
	if err := config.Save(path, config.Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	assertMode(t, path, 0o600)
	assertMode(t, filepath.Dir(path), 0o700)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o; want %o", path, got, want)
	}
}
