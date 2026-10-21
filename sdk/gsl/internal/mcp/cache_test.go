package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/mcp/fake"
)

// clock is a settable time source.
type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func testOpts(t *testing.T) (Options, *clock) {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // no real ~/.claude.json in Configured
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	c := &clock{t: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	return Options{CacheDir: t.TempDir(), TTL: time.Minute, Now: c.now}, c
}

func listOutput(connected, failed int) []byte {
	var b strings.Builder
	b.WriteString("Checking MCP server health…\n\n")
	for i := 0; i < connected; i++ {
		b.WriteString("c" + string(rune('a'+i)) + ": https://x.invalid - ✔ Connected\n")
	}
	for i := 0; i < failed; i++ {
		b.WriteString("f" + string(rune('a'+i)) + ": https://x.invalid - ✘ Failed to connect\n")
	}
	return []byte(b.String())
}

// E12: Get never spawns — not even on a cold cache. A `claude` on PATH that
// would leave a marker proves nothing ran.
func TestGet_NeverSpawns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stand-in")
	}
	o, _ := testOpts(t)
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	script := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	s, err := Get(context.Background(), t.TempDir(), o)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Stale || !s.RefreshDue {
		t.Errorf("cold cache: Stale=%v RefreshDue=%v, want both true", s.Stale, s.RefreshDue)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("Get ran claude")
	}
}

// E11: the cache is keyed by cwd — repo A's count is never served to repo B.
func TestStatus_CacheKeyedByCwd(t *testing.T) {
	o, _ := testOpts(t)
	a, b := t.TempDir(), t.TempDir()
	r := &fake.Runner{Script: []fake.Response{{Stdout: listOutput(5, 0)}, {Stdout: listOutput(1, 0)}}}
	if err := Refresh(context.Background(), r, a, o); err != nil {
		t.Fatal(err)
	}
	if err := Refresh(context.Background(), r, b, o); err != nil {
		t.Fatal(err)
	}
	sa, _ := Get(context.Background(), a, o)
	sb, _ := Get(context.Background(), b, o)
	if sa.Connected != 5 || sb.Connected != 1 {
		t.Fatalf("A=%d B=%d, want 5 and 1", sa.Connected, sb.Connected)
	}
	if sb.Stale || sb.RefreshDue {
		t.Errorf("fresh entry: Stale=%v RefreshDue=%v", sb.Stale, sb.RefreshDue)
	}
}

// An expired entry is still served (stale beats nothing) and asks for a refresh.
func TestGet_StaleServedAndDue(t *testing.T) {
	o, c := testOpts(t)
	cwd := t.TempDir()
	if err := Refresh(context.Background(), &fake.Runner{Default: fake.Response{Stdout: listOutput(3, 1)}}, cwd, o); err != nil {
		t.Fatal(err)
	}
	c.add(2 * time.Minute)
	s, _ := Get(context.Background(), cwd, o)
	if s.Connected != 3 || s.Failed != 1 || !s.Stale || !s.RefreshDue {
		t.Fatalf("stale entry = %+v", s)
	}
}

// Failures are cached negatively with exponential backoff: no refresh is due
// until the retry time, and each failure doubles it. The last good status is
// kept. Force (the `gsl mcp refresh` verb) ignores the backoff.
func TestRefresh_NegativeCacheBackoff(t *testing.T) {
	o, c := testOpts(t)
	cwd := t.TempDir()
	ctx := context.Background()
	if err := Refresh(ctx, &fake.Runner{Default: fake.Response{Stdout: listOutput(2, 0)}}, cwd, o); err != nil {
		t.Fatal(err)
	}
	c.add(2 * time.Minute) // stale
	bad := &fake.Runner{Default: fake.Response{Err: errors.New("claude: not found")}}
	if err := Refresh(ctx, bad, cwd, o); err == nil {
		t.Fatal("a failed probe must return its error")
	}
	s, _ := Get(ctx, cwd, o)
	if s.Connected != 2 || !s.Stale || s.RefreshDue {
		t.Fatalf("after a failure: %+v (want last good 2, stale, not due)", s)
	}
	c.add(backoff(1) + time.Second)
	if s, _ = Get(ctx, cwd, o); !s.RefreshDue {
		t.Fatal("refresh not due after the first backoff")
	}
	_ = Refresh(ctx, bad, cwd, o)
	c.add(backoff(1) + time.Second)
	if s, _ = Get(ctx, cwd, o); s.RefreshDue {
		t.Fatal("second failure did not double the backoff")
	}
	calls := len(bad.Calls)
	if err := Refresh(ctx, bad, cwd, o); err != nil || len(bad.Calls) != calls {
		t.Fatalf("Refresh during backoff ran the probe (err=%v)", err)
	}
	o.Force = true
	_ = Refresh(ctx, bad, cwd, o)
	if len(bad.Calls) != calls+1 {
		t.Fatal("Force did not bypass the backoff")
	}
	if backoff(1) != 30*time.Second || backoff(2) != time.Minute || backoff(20) != 30*time.Minute {
		t.Errorf("backoff = %v %v %v", backoff(1), backoff(2), backoff(20))
	}
}

// Single-flight: while a refresh holds the lock, another does not probe and no
// refresh is due; a lock left by a crashed refresh expires.
func TestRefresh_SingleFlight(t *testing.T) {
	o, _ := testOpts(t)
	cwd := t.TempDir()
	lock := filepath.Join(o.CacheDir, key(cwd)+".lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &fake.Runner{Default: fake.Response{Stdout: listOutput(1, 0)}}
	if err := Refresh(context.Background(), r, cwd, o); err != nil || len(r.Calls) != 0 {
		t.Fatalf("Refresh with the lock held: err=%v calls=%d", err, len(r.Calls))
	}
	if s, _ := Get(context.Background(), cwd, o); s.RefreshDue {
		t.Error("a refresh is in flight: none should be due")
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if err := Refresh(context.Background(), r, cwd, o); err != nil || len(r.Calls) != 1 {
		t.Fatalf("a stale lock must not block forever: err=%v calls=%d", err, len(r.Calls))
	}
	if _, err := os.Stat(lock); err == nil {
		t.Error("Refresh left its lock behind")
	}
}

// Configured comes from the config scan on every Get (cheap file reads), and
// the probe's servers survive the round trip.
func TestGet_ConfiguredAndServers(t *testing.T) {
	o, _ := testOpts(t)
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, ".mcp.json"), []byte(`{"mcpServers":{"a":{},"b":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile("testdata/mcp-list-real.txt")
	if err := Refresh(context.Background(), &fake.Runner{Default: fake.Response{Stdout: out}}, cwd, o); err != nil {
		t.Fatal(err)
	}
	s, _ := Get(context.Background(), cwd, o)
	if s.Configured != 2 || s.Connected != 13 || len(s.Servers) != 66 || s.FetchedAt.IsZero() {
		t.Fatalf("Get = configured %d connected %d servers %d fetched %v", s.Configured, s.Connected, len(s.Servers), s.FetchedAt)
	}
}

// The probe runs `claude mcp list`, exactly.
func TestRefresh_RunsClaudeMCPList(t *testing.T) {
	o, _ := testOpts(t)
	r := &fake.Runner{Default: fake.Response{Stdout: listOutput(1, 0)}}
	if err := Refresh(context.Background(), r, t.TempDir(), o); err != nil {
		t.Fatal(err)
	}
	if len(r.Calls) != 1 || r.Calls[0].Name != "claude" || strings.Join(r.Calls[0].Args, " ") != "mcp list" {
		t.Fatalf("calls = %+v", r.Calls)
	}
}
