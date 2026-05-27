package gh_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wenlock/dotfiles/gsl/internal/gh"
	"github.com/wenlock/dotfiles/gsl/internal/gh/fake"
)

// setupCacheDir creates a temp dir, sets XDG_CACHE_HOME to it, and returns a
// cleanup function that restores the original env-var and removes the dir.
func setupCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	return dir
}

// writeFreshCache manually writes a cache entry that is < 60 s old.
func writeFreshCache(t *testing.T, cacheDir, branch string, info gh.PRInfo) {
	t.Helper()
	safe := branch
	for _, ch := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		safe = replaceAll(safe, ch, "-")
	}
	dir := filepath.Join(cacheDir, "gsl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	type cacheEntry struct {
		Number int       `json:"number"`
		State  string    `json:"state"`
		TS     time.Time `json:"ts"`
	}
	entry := cacheEntry{Number: info.Number, State: info.State, TS: time.Now()}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, "pr-"+safe+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
}

// replaceAll is a minimal strings.ReplaceAll stand-in using stdlib only.
func replaceAll(s, old, new string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			out = append(out, new...)
			i += len(old)
		} else {
			out = append(out, s[i])
			i++
		}
	}
	return string(out)
}

// --- Tests ---

// TestPROpenState verifies that a successful gh pr view response is parsed
// into a non-nil PRInfo with the correct Number and State.
func TestPROpenState(t *testing.T) {
	setupCacheDir(t)
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(`{"number":21,"state":"OPEN"}`)},
		},
	}
	info, err := gh.PR(context.Background(), r, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil PRInfo, got nil")
	}
	if info.Number != 21 {
		t.Errorf("Number = %d; want 21", info.Number)
	}
	if info.State != "OPEN" {
		t.Errorf("State = %q; want OPEN", info.State)
	}
}

// TestPRDraftState verifies that a DRAFT PR is parsed correctly.
func TestPRDraftState(t *testing.T) {
	setupCacheDir(t)
	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(`{"number":7,"state":"DRAFT"}`)},
		},
	}
	info, err := gh.PR(context.Background(), r, "feature/draft")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil PRInfo for DRAFT, got nil")
	}
	if info.Number != 7 {
		t.Errorf("Number = %d; want 7", info.Number)
	}
	if info.State != "DRAFT" {
		t.Errorf("State = %q; want DRAFT", info.State)
	}
}

// TestPRNoPR verifies that when gh returns an error (no PR found), PR()
// returns nil PRInfo and nil error — the "omit" contract.
func TestPRNoPR(t *testing.T) {
	setupCacheDir(t)
	r := &fake.Runner{
		Default: fake.Response{
			Err: errors.New("no pull requests found for branch \"main\""),
		},
	}
	info, err := gh.PR(context.Background(), r, "main")
	if err != nil {
		t.Fatalf("expected nil error for no-PR case, got: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil PRInfo for no-PR case, got: %+v", info)
	}
}

// TestPREmptyOutput verifies that empty gh output returns nil PRInfo, nil error.
func TestPREmptyOutput(t *testing.T) {
	setupCacheDir(t)
	r := &fake.Runner{
		Default: fake.Response{Stdout: nil, Err: nil},
	}
	info, err := gh.PR(context.Background(), r, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil PRInfo for empty output, got: %+v", info)
	}
}

// TestPRCacheHit is the SPY test: when a fresh cache file exists the Runner
// must NOT be called (call count stays at 0).
func TestPRCacheHit(t *testing.T) {
	cacheDir := setupCacheDir(t)

	// Pre-write a fresh cache entry.
	writeFreshCache(t, cacheDir, "feature/my-branch", gh.PRInfo{Number: 99, State: "OPEN"})

	spy := &fake.Runner{}
	info, err := gh.PR(context.Background(), spy, "feature/my-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil PRInfo from cache, got nil")
	}
	if info.Number != 99 {
		t.Errorf("Number = %d; want 99", info.Number)
	}
	if info.State != "OPEN" {
		t.Errorf("State = %q; want OPEN", info.State)
	}

	// SPY assertion — the runner must have been called ZERO times.
	if got := spy.CallCount(); got != 0 {
		t.Errorf("Runner.CallCount() = %d; want 0 (cache should have been used)", got)
	}
}

// TestPRTimeout verifies that a context deadline / timeout from the runner
// returns nil PRInfo and nil error (omit — never hang, never error-out).
func TestPRTimeout(t *testing.T) {
	setupCacheDir(t)

	// Simulate a timeout by providing a context that is already cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done

	r := &fake.Runner{}
	info, err := gh.PR(ctx, r, "main")
	if err != nil {
		t.Fatalf("expected nil error for timeout case, got: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil PRInfo for timeout case, got: %+v", info)
	}
}

// TestPRCacheWrittenAfterFetch verifies that after a successful gh call the
// result is written to the cache, so a subsequent call hits the cache.
func TestPRCacheWrittenAfterFetch(t *testing.T) {
	setupCacheDir(t)

	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(`{"number":55,"state":"MERGED"}`)},
		},
	}

	// First call — should invoke the runner and write the cache.
	info1, err := gh.PR(context.Background(), r, "release/v1")
	if err != nil || info1 == nil {
		t.Fatalf("first call: got (%v, %v); want non-nil PRInfo", info1, err)
	}
	if r.CallCount() != 1 {
		t.Errorf("first call: Runner.CallCount() = %d; want 1", r.CallCount())
	}

	// Second call — cache should be fresh, runner should NOT be called again.
	info2, err := gh.PR(context.Background(), r, "release/v1")
	if err != nil || info2 == nil {
		t.Fatalf("second call: got (%v, %v); want non-nil PRInfo", info2, err)
	}
	if r.CallCount() != 1 {
		t.Errorf("second call: Runner.CallCount() = %d; want still 1 (cache hit)", r.CallCount())
	}
	if info2.Number != 55 || info2.State != "MERGED" {
		t.Errorf("second call: got %+v; want {55 MERGED}", info2)
	}
}

// TestPRNoPRCachedNegative verifies Finding #3: a "no PR" result (Number 0)
// is cached, so a second call within the TTL does NOT invoke the Runner and
// still returns (nil, nil) per the omit contract.
func TestPRNoPRCachedNegative(t *testing.T) {
	setupCacheDir(t)

	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(`{"number":0,"state":""}`)},
		},
	}

	// First call — runner invoked, returns nil PRInfo, writes negative cache.
	info1, err := gh.PR(context.Background(), r, "no-pr-branch")
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if info1 != nil {
		t.Fatalf("first call: expected nil PRInfo, got: %+v", info1)
	}
	if r.CallCount() != 1 {
		t.Fatalf("first call: CallCount() = %d; want 1", r.CallCount())
	}

	// Second call — cache hit on the negative result; runner NOT invoked.
	info2, err := gh.PR(context.Background(), r, "no-pr-branch")
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if info2 != nil {
		t.Fatalf("second call: expected nil PRInfo (cached negative), got: %+v", info2)
	}
	if r.CallCount() != 1 {
		t.Errorf("second call: CallCount() = %d; want still 1 (negative cache hit)", r.CallCount())
	}
}

// TestPRBranchForwarded verifies that the branch is forwarded to gh as a
// positional argument (`gh pr view <branch> ...`), so the query matches the
// cache key instead of resolving the process cwd's current branch.
func TestPRBranchForwarded(t *testing.T) {
	setupCacheDir(t)

	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(`{"number":42,"state":"OPEN"}`)},
		},
	}
	if _, err := gh.PR(context.Background(), r, "feature/widget"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.Calls))
	}
	call := r.Calls[0]
	if call.Name != "pr" {
		t.Errorf("Name = %q; want pr", call.Name)
	}
	// args must be: view <branch> --json number,state
	if len(call.Args) < 2 || call.Args[0] != "view" {
		t.Fatalf("Args = %v; want [view <branch> ...]", call.Args)
	}
	if call.Args[1] != "feature/widget" {
		t.Errorf("branch positional arg = %q; want feature/widget (Args=%v)", call.Args[1], call.Args)
	}
}

// TestPRBranchSanitization verifies that branch names with slashes do not
// result in nested directories or path-traversal issues.
func TestPRBranchSanitization(t *testing.T) {
	cacheDir := setupCacheDir(t)

	r := &fake.Runner{
		Script: []fake.Response{
			{Stdout: []byte(`{"number":3,"state":"OPEN"}`)},
		},
	}
	_, err := gh.PR(context.Background(), r, "feature/foo/bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The cache file must be a single flat file inside cacheDir/gsl/ with no
	// subdirectories corresponding to the slash segments.
	entries, err := os.ReadDir(filepath.Join(cacheDir, "gsl"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 cache file, got %d entries", len(entries))
	}
	name := entries[0].Name()
	if name != "pr-feature-foo-bar.json" {
		t.Errorf("cache filename = %q; want pr-feature-foo-bar.json", name)
	}
}
