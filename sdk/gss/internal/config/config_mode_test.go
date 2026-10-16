package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/config"
)

// TestWriteStubIfMissing_PrivateModes: the config stub is the user's private
// state — 0600 file in a 0700 directory (security-scan-green, G301/G306).
func TestWriteStubIfMissing_PrivateModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gss", "config.yaml")
	clock := fixedClock{t: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)}
	if _, err := config.WriteStubIfMissing(path, clock); err != nil {
		t.Fatalf("WriteStubIfMissing: %v", err)
	}
	for p, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o; want %o", p, got, want)
		}
	}
}
