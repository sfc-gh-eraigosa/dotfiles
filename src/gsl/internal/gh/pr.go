package gh

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PRInfo holds the result of a `gh pr view` lookup.
type PRInfo struct {
	Number int
	State  string
}

// prCache is the on-disk JSON structure for the cache file.
type prCache struct {
	Number int       `json:"number"`
	State  string    `json:"state"`
	TS     time.Time `json:"ts"`
}

// cacheMaxAge is the maximum age of a cache entry before it is considered stale.
const cacheMaxAge = 60 * time.Second

// prViewTimeout is the per-invocation timeout for `gh pr view`.
const prViewTimeout = 800 * time.Millisecond

// sanitizeBranch replaces characters that are unsafe in a filename.
// Primarily replaces forward-slashes used in git branch names.
func sanitizeBranch(branch string) string {
	return strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	).Replace(branch)
}

// cacheFilePath returns the path to the cache file for the given branch.
func cacheFilePath(branch string) string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	safe := sanitizeBranch(branch)
	return filepath.Join(base, "gsl", "pr-"+safe+".json")
}

// readCache attempts to load a valid (non-stale) cache entry for branch.
// Returns nil if the file is absent, unreadable, invalid, or older than cacheMaxAge.
func readCache(path string) *prCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c prCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	if time.Since(c.TS) >= cacheMaxAge {
		return nil
	}
	return &c
}

// writeCache persists the PRInfo to the cache file at path.
// Errors are silently ignored — a failed write is non-fatal.
func writeCache(path string, info *PRInfo) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	c := prCache{
		Number: info.Number,
		State:  info.State,
		TS:     time.Now(),
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// PR returns the PR number and state for the given branch.
//
// Contract: when there is no PR, or gh is absent / unreachable, PR returns
// (nil, nil) — the caller must treat nil PRInfo as "omit the PR# field".
// A non-nil error is returned only for internal logic failures that the caller
// cannot work around by omitting output (currently none in this path).
//
// Cache: a file at ${XDG_CACHE_HOME:-$HOME/.cache}/gsl/pr-<branch>.json is
// consulted first. If it exists and is < 60 s old the Runner is NOT called.
// Otherwise `gh pr view --json number,state` is invoked with an 800 ms
// timeout; a fresh cache file is written on success.
func PR(ctx context.Context, runner Runner, branch string) (*PRInfo, error) {
	path := cacheFilePath(branch)

	// Cache hit — return without touching the Runner.
	if c := readCache(path); c != nil {
		return &PRInfo{Number: c.Number, State: c.State}, nil
	}

	// Apply a per-call timeout so we never block the status line for long.
	tctx, cancel := context.WithTimeout(ctx, prViewTimeout)
	defer cancel()

	out, err := runner.Run(tctx, "pr", "view", "--json", "number,state")
	if err != nil {
		// gh absent, no remote, no PR, timeout — all map to "omit".
		return nil, nil //nolint:nilerr
	}

	if len(out) == 0 {
		return nil, nil
	}

	var payload struct {
		Number int    `json:"number"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, nil //nolint:nilerr
	}

	// Number == 0 generally means no PR was found.
	if payload.Number == 0 {
		return nil, nil
	}

	info := &PRInfo{Number: payload.Number, State: payload.State}
	writeCache(path, info)
	return info, nil
}

