package gh

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWriteCache_PrivateModes: the PR cache is private state — 0600 file in a
// 0700 directory (security-scan-green, G301/G306).
func TestWriteCache_PrivateModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache", "pr.json")
	writeCache(path, &PRInfo{Number: 7}, time.Minute)
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
