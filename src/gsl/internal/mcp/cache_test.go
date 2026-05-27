package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wenlock/dotfiles/gsl/internal/mcp"
	"github.com/wenlock/dotfiles/gsl/internal/mcp/fake"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// writeCacheFile writes a cache file with the given count and timestamp age.
// A negative age means the entry is that many seconds in the past (stale).
func writeCacheFile(t *testing.T, path string, count int, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ts := time.Now().Add(-age).Unix()
	data, _ := json.Marshal(map[string]any{"count": count, "ts": ts})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// opts returns an ActiveCountOptions pre-configured for a test:
//   - CacheFile points to a temp directory so tests don't share state.
//   - Now is real time.Now() unless overridden by the caller.
func opts(t *testing.T) (mcp.ActiveCountOptions, string) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "gsl", "mcp.json")
	return mcp.ActiveCountOptions{CacheFile: cacheFile}, cacheFile
}

// ─── parsing ──────────────────────────────────────────────────────────────────

func TestParseConnectedCount_Mixed(t *testing.T) {
	// We test via ActiveCount by injecting scripted Runner output.
	o, _ := opts(t)
	r := &fake.Runner{
		Script: []fake.Response{
			{
				// 3 connected, 2 failed — as produced by `claude mcp list`.
				Stdout: []byte(
					"claude.ai Slack: https://mcp.slack.com/mcp - ✓ Connected\n" +
						"claude.ai Linear: https://mcp.linear.app/mcp - ✓ Connected\n" +
						"plugin:github:github: https://api.githubcopilot.com/mcp/ (HTTP) - ✗ Failed to connect\n" +
						"plugin:deploy-on-aws:awsiac: uvx awslabs.aws-iac-mcp-server@latest - ✗ Failed to connect\n" +
						"claude.ai Google Drive: https://drivemcp.googleapis.com/mcp/v1 - ✓ Connected\n",
				),
			},
		},
	}
	count, err := mcp.ActiveCount(context.Background(), r, o)
	if err != nil {
		t.Fatalf("ActiveCount: unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("ActiveCount = %d; want 3 connected", count)
	}
}

// TestParseConnectedCount_CheckmarkInName guards against the false-positive
// where a "✓" embedded in a server name (or any non-status position) inflates
// the count. Only the "✓ Connected" status token in the status position counts.
func TestParseConnectedCount_CheckmarkInName(t *testing.T) {
	o, _ := opts(t)
	r := &fake.Runner{
		Script: []fake.Response{
			{
				// First server has a "✓" in its NAME but is actually FAILED.
				// Second server is genuinely connected. Want count == 1.
				Stdout: []byte(
					"my✓server: https://example.com/mcp - ✗ Failed to connect\n" +
						"real-server: https://example.com/mcp - ✓ Connected\n",
				),
			},
		},
	}
	count, err := mcp.ActiveCount(context.Background(), r, o)
	if err != nil {
		t.Fatalf("ActiveCount: unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("ActiveCount = %d; want 1 (✓ in a name must not be counted)", count)
	}
}

func TestActiveCount_AllConnected(t *testing.T) {
	o, _ := opts(t)
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte("server-a: url-a - ✓ Connected\nserver-b: url-b - ✓ Connected\n")},
		},
	}
	count, err := mcp.ActiveCount(context.Background(), r, o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d; want 2", count)
	}
}

func TestActiveCount_AllFailed(t *testing.T) {
	o, _ := opts(t)
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte("server-a: url-a - ✗ Failed to connect\nserver-b: url-b - ✗ Failed to connect\n")},
		},
	}
	count, err := mcp.ActiveCount(context.Background(), r, o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d; want 0", count)
	}
}

// ─── cache hit (spy test) ─────────────────────────────────────────────────────

// TestActiveCount_CacheHit_NoSubprocess is the key spy test:
// when a fresh cache entry exists (age < 60s), ActiveCount MUST NOT call the
// Runner at all.  We assert Runner.CallCount() == 0.
func TestActiveCount_CacheHit_NoSubprocess(t *testing.T) {
	o, cacheFile := opts(t)

	// Pre-write a FRESH cache entry (5 seconds old, well under 60s TTL).
	writeCacheFile(t, cacheFile, 7, 5*time.Second)

	r := &fake.Runner{} // scripted to return nothing; any call would panic the test

	count, err := mcp.ActiveCount(context.Background(), r, o)
	if err != nil {
		t.Fatalf("ActiveCount with fresh cache: unexpected error: %v", err)
	}
	if count != 7 {
		t.Errorf("count = %d; want 7 (from cache)", count)
	}

	// THE CRITICAL SPY ASSERTION: zero subprocess calls.
	if calls := r.CallCount(); calls != 0 {
		t.Errorf("Runner.CallCount() = %d; want 0 (cache hit must not call the Runner)", calls)
	}
}

// ─── stale cache ─────────────────────────────────────────────────────────────

func TestActiveCount_StaleCache_CallsRunner(t *testing.T) {
	o, cacheFile := opts(t)

	// Write a STALE cache entry (90 seconds old, over 60s TTL).
	writeCacheFile(t, cacheFile, 42, 90*time.Second)

	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte("server: url - ✓ Connected\n")},
		},
	}

	count, err := mcp.ActiveCount(context.Background(), r, o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d; want 1 (fresh from runner)", count)
	}
	if calls := r.CallCount(); calls != 1 {
		t.Errorf("Runner.CallCount() = %d; want 1 (stale cache must invoke runner)", calls)
	}
}

// ─── runner timeout / error ───────────────────────────────────────────────────

// TestActiveCount_RunnerTimeout_FallbackToCachedValue: when the runner times
// out and there IS a cached (even stale) value, return that value, no error.
func TestActiveCount_RunnerTimeout_FallbackToCachedValue(t *testing.T) {
	o, cacheFile := opts(t)

	// Write a stale-but-present cache entry.
	writeCacheFile(t, cacheFile, 5, 120*time.Second)

	// Runner is scripted to return an error (simulating timeout).
	r := &fake.Runner{
		Script: []fake.Response{
			{Err: errors.New("context deadline exceeded")},
		},
	}

	count, err := mcp.ActiveCount(context.Background(), r, o)
	if err != nil {
		t.Fatalf("expected fallback to cached value on runner error, got error: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d; want 5 (stale cache fallback)", count)
	}
}

// TestActiveCount_RunnerTimeout_NoCache_ReturnsError: when the runner fails
// and there is NO cached value at all, return ErrNoCache.
func TestActiveCount_RunnerTimeout_NoCache_ReturnsError(t *testing.T) {
	o, _ := opts(t) // no cache file written

	r := &fake.Runner{
		Script: []fake.Response{
			{Err: errors.New("context deadline exceeded")},
		},
	}

	_, err := mcp.ActiveCount(context.Background(), r, o)
	if !errors.Is(err, mcp.ErrNoCache) {
		t.Errorf("err = %v; want mcp.ErrNoCache", err)
	}
}

// ─── cache persistence ────────────────────────────────────────────────────────

// TestActiveCount_WritesCache verifies that a successful runner call writes a
// cache file that a subsequent call reads (proving the write path works).
func TestActiveCount_WritesCache(t *testing.T) {
	o, _ := opts(t)

	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte("s1: u1 - ✓ Connected\ns2: u2 - ✓ Connected\n")},
		},
	}

	// First call: cache cold → runner invoked → cache written.
	count, err := mcp.ActiveCount(context.Background(), r, o)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if count != 2 {
		t.Errorf("first call count = %d; want 2", count)
	}

	// Second call with a fresh runner (no scripted responses): must hit cache.
	r2 := &fake.Runner{}
	count2, err := mcp.ActiveCount(context.Background(), r2, o)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if count2 != 2 {
		t.Errorf("second call count = %d; want 2 (from cache)", count2)
	}
	if calls := r2.CallCount(); calls != 0 {
		t.Errorf("second call Runner.CallCount() = %d; want 0 (should be cache hit)", calls)
	}
}
