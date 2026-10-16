package repo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/repo"
)

// TestCachePut_PrivateModes: the origin→NWO cache is private state — 0600 file
// in a 0700 directory (security-scan-green, G301/G306).
func TestCachePut_PrivateModes(t *testing.T) {
	c := &repo.Cache{Path: filepath.Join(t.TempDir(), "gss", "nwo.json")}
	if err := c.Put("git@github.com:o/r.git", repo.NWO{Owner: "o", Repo: "r"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	for p, want := range map[string]os.FileMode{c.Path: 0o600, filepath.Dir(c.Path): 0o700} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o; want %o", p, got, want)
		}
	}
}
