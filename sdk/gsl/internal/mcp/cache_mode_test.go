package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteCache_PrivateModes: the MCP active-count cache is private state —
// 0600 file in a 0700 directory, and no leftover temp file (security-scan-green,
// G301/G306/G404: the temp name comes from os.CreateTemp, not math/rand).
func TestWriteCache_PrivateModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	path := filepath.Join(dir, "ac.json")
	if err := writeCache(path, cacheEntry{Count: 1}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}
	for p, want := range map[string]os.FileMode{path: 0o600, dir: 0o700} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o; want %o", p, got, want)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("want only the cache file in %s, got %d entries", dir, len(entries))
	}
}

// TestWriteCache_RenameFailureLeavesNoTempFile: when the final rename cannot
// succeed (here: a directory already occupies the cache path), the 0600 temp
// file must not be left behind — the status line writes this cache on every
// render, so a leak would accumulate forever.
func TestWriteCache_RenameFailureLeavesNoTempFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	path := filepath.Join(dir, "ac.json")
	if err := os.MkdirAll(filepath.Join(path, "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeCache(path, cacheEntry{Count: 1}); err == nil {
		t.Fatal("writeCache over a non-empty directory should fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "ac.json" {
			t.Errorf("temp file left behind after a failed rename: %s", e.Name())
		}
	}
}
