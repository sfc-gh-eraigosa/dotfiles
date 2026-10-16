package usage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/usage"
)

// TestWrite_PrivateDirMode: the usage directory holds the user's rate-limit
// history; Write creates it 0700 (security-scan-green, G301).
func TestWrite_PrivateDirMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "usage")
	s := usage.Snapshot{Host: usage.HostClaude, CapturedAt: time.Now()}
	if err := usage.Write(dir, s); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Errorf("usage dir mode = %o; want 0700", got)
	}
}
