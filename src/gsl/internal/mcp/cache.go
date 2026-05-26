package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cacheTTL is how long a written cache entry is considered fresh.
const cacheTTL = 60 * time.Second

// runnerTimeout caps how long we wait for `claude mcp list`.
const runnerTimeout = 500 * time.Millisecond

// cacheEntry is the JSON structure written to the cache file.
type cacheEntry struct {
	Count int   `json:"count"`
	TS    int64 `json:"ts"` // Unix timestamp (seconds) of when the entry was written
}

// ErrNoCache is returned when ActiveCount has no cached value to fall back to
// and the Runner call fails (timeout or other error).
var ErrNoCache = errors.New("mcp: no cache and runner failed")

// ActiveCountOptions configures ActiveCount.  Zero value uses system defaults.
type ActiveCountOptions struct {
	// CacheFile overrides the default cache path
	// (${XDG_CACHE_HOME:-$HOME/.cache}/gsl/mcp.json).
	CacheFile string

	// Now, if non-nil, replaces time.Now() for cache-age calculations.
	// Useful in tests.
	Now func() time.Time
}

// defaultCacheFile returns the default path for the gsl MCP cache.
func defaultCacheFile() (string, error) {
	cacheBase := os.Getenv("XDG_CACHE_HOME")
	if cacheBase == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cacheBase = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheBase, "gsl", "mcp.json"), nil
}

// ActiveCount returns the number of MCP servers that are currently reporting
// as connected/healthy.
//
// Caching: the count is written to ${XDG_CACHE_HOME:-$HOME/.cache}/gsl/mcp.json
// (or opts.CacheFile if set).  If a fresh entry exists (age < 60 s) the
// cached value is returned immediately and the Runner is NOT called.
//
// Subprocess: when the cache is absent or stale, Run(ctx, "claude", "mcp",
// "list") is called with a ~500 ms timeout.  The output is parsed line by line;
// a line is counted as connected when it contains the Unicode check-mark "✓".
// Lines containing "✗" (or that don't contain "✓") are counted as failed and
// excluded.  The format `<name>: <url> - ✓ Connected` is tolerated, as are
// minor variations, because only the presence/absence of "✓" is checked.
//
// Error handling:
//   - Runner timeout or error with a non-stale cached value: returns the last
//     cached count (stale but better than nothing).
//   - Runner timeout or error with no cached value at all: returns (0,
//     ErrNoCache) so the caller can decide (e.g. fall back to ConfiguredCount).
//   - Fresh cache hit: returns the cached count and nil error.
func ActiveCount(ctx context.Context, r Runner, opts ActiveCountOptions) (int, error) {
	cacheFile := opts.CacheFile
	if cacheFile == "" {
		cf, err := defaultCacheFile()
		if err != nil {
			return 0, err
		}
		cacheFile = cf
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}

	// ── Try cache ─────────────────────────────────────────────────────────────
	entry, hasCached := readCache(cacheFile)
	if hasCached {
		age := now().Sub(time.Unix(entry.TS, 0))
		if age < cacheTTL {
			// Fresh cache hit — do NOT invoke the Runner.
			return entry.Count, nil
		}
	}

	// ── Call Runner ───────────────────────────────────────────────────────────
	tctx, cancel := context.WithTimeout(ctx, runnerTimeout)
	defer cancel()

	out, runErr := r.Run(tctx, "claude", "mcp", "list")
	if runErr != nil {
		// Runner failed — fall back to stale cache if available.
		if hasCached {
			return entry.Count, nil
		}
		return 0, ErrNoCache
	}

	count := parseConnectedCount(out)

	// ── Write cache ───────────────────────────────────────────────────────────
	_ = writeCache(cacheFile, cacheEntry{Count: count, TS: now().Unix()})

	return count, nil
}

// parseConnectedCount scans the output of `claude mcp list` and counts lines
// that contain the Unicode check mark "✓", which marks a connected server.
// Lines with "✗" (or no check mark at all) are treated as failed/disconnected.
//
// The full line format observed in the wild is:
//
//	<name>: <url> - ✓ Connected
//	<name>: <url> - ✗ Failed to connect
//
// Only the presence of the "✓" rune is required; the surrounding text is
// intentionally not validated so minor Claude CLI version differences don't
// break counting.
func parseConnectedCount(output []byte) int {
	count := 0
	sc := bufio.NewScanner(bytes.NewReader(output))
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "✓") {
			count++
		}
	}
	return count
}

// readCache reads and deserialises the cache file.  Returns (entry, true) on
// success, or (zero, false) if the file is absent, empty, or malformed.
func readCache(path string) (cacheEntry, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return cacheEntry{}, false
	}
	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return cacheEntry{}, false
	}
	return e, true
}

// writeCache serialises the entry and atomically-ish writes it to path,
// creating parent directories as needed.
func writeCache(path string, e cacheEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	// Write via a temp file in the same directory for an atomic rename.
	// Include the PID and a random suffix to avoid collisions when multiple
	// gsl processes write the cache concurrently.
	tmp := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), rand.Int63()) //nolint:gosec
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
