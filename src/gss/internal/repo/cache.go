package repo

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Cache is a tiny single-entry file cache for the resolved NWO, stored at
// <worktrees_root>/.nwo. It records the origin URL the NWO was resolved
// against, so a changed origin (a re-cloned or re-pointed remote) misses
// rather than returning a stale identity.
type Cache struct {
	Path string
}

// cacheEntry is the on-disk JSON shape.
type cacheEntry struct {
	OriginURL string `json:"origin_url"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
}

// NewCache returns a Cache writing to <root>/.nwo.
func NewCache(root string) *Cache {
	return &Cache{Path: filepath.Join(root, ".nwo")}
}

// Get returns the cached NWO if the cache file exists, parses, and was
// resolved against the same originURL. Any mismatch or read/parse error is
// a miss (ok=false), never an error — a bad cache must not block resolution.
func (c *Cache) Get(originURL string) (NWO, bool) {
	if c == nil {
		return NWO{}, false
	}
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return NWO{}, false
	}
	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return NWO{}, false
	}
	if e.OriginURL != originURL {
		return NWO{}, false
	}
	nwo := NWO{Owner: e.Owner, Repo: e.Repo}
	if !nwo.Valid() {
		return NWO{}, false
	}
	return nwo, true
}

// Put writes the NWO + originURL to the cache file, creating parent dirs.
func (c *Cache) Put(originURL string, nwo NWO) error {
	if c == nil {
		return nil
	}
	data, err := json.MarshalIndent(cacheEntry{OriginURL: originURL, Owner: nwo.Owner, Repo: nwo.Repo}, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(c.Path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(c.Path, data, 0o644)
}
