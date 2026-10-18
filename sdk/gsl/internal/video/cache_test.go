//go:build !windows

package video

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// cacheClock is a mutable clock for Cache.Now, starting at real time.
type cacheClock struct{ t time.Time }

func (c *cacheClock) now() time.Time { return c.t }

func newTestCache(t *testing.T) (Cache, *cacheClock) {
	t.Helper()
	clk := &cacheClock{t: time.Now()}
	return Cache{Dir: t.TempDir(), Now: clk.now}, clk
}

// spyCounter counts spawn calls. It never starts a process. Like a real
// refresher it keeps the lock it was handed (a dup of the descriptor, which
// shares the flock) until release, so later Gets see a running refresher.
type spyCounter struct {
	n    int
	held []*os.File
}

func (s *spyCounter) spawn(lock *os.File) error {
	s.n++
	fd, err := syscall.Dup(int(lock.Fd()))
	if err != nil {
		return err
	}
	s.held = append(s.held, os.NewFile(uintptr(fd), "lock-dup"))
	return nil
}

// release is the refresher exiting.
func (s *spyCounter) release() {
	for _, f := range s.held {
		_ = f.Close()
	}
	s.held = nil
}

func okRecord(at time.Time, u int) Record {
	return Record{
		OK:      true,
		Stats:   Stats{Util: u, UsedBytes: 2069835776, TotalBytes: 8319870976, Source: "nvidia"},
		Updated: at.UnixMilli(),
	}
}

func mustWrite(t *testing.T, c Cache, r Record) {
	t.Helper()
	if err := c.Write(r); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

// lockHeld reports whether some descriptor holds the refresher lock.
func lockHeld(c Cache) bool {
	f, ok := c.TryLock()
	if ok {
		_ = f.Close()
	}
	return !ok
}

func TestCacheGet_FreshRecordDoesNotSpawn(t *testing.T) {
	c, clk := newTestCache(t)
	mustWrite(t, c, okRecord(clk.now(), 6))

	var spy spyCounter
	s, show := c.Get(spy.spawn)
	if !show || s.Util != 6 {
		t.Fatalf("Get = (%+v, %v), want util 6 shown", s, show)
	}
	if spy.n != 0 {
		t.Fatalf("spawn called %d times for a fresh record, want 0", spy.n)
	}
	if lockHeld(c) {
		t.Fatal("lock taken for a fresh record")
	}
}

func TestCacheGet_StaleOKSpawnsOnceAcrossTenCalls(t *testing.T) {
	c, clk := newTestCache(t)
	mustWrite(t, c, okRecord(clk.now().Add(-5*time.Second), 6))

	// The spy keeps the lock, like a refresher that is still running: the lock
	// must suppress every later spawn until it exits.
	var spy spyCounter
	defer spy.release()
	for i := 0; i < 10; i++ {
		s, show := c.Get(spy.spawn)
		if !show || s.Util != 6 {
			t.Fatalf("call %d: Get = (%+v, %v), want last known reading", i, s, show)
		}
	}
	if spy.n != 1 {
		t.Fatalf("spawn called %d times over 10 Gets, want exactly 1", spy.n)
	}
	if !lockHeld(c) {
		t.Fatal("lock not held while the refresher runs")
	}
	spy.release()
	if lockHeld(c) {
		t.Fatal("lock still held after the refresher exited")
	}
	c.Get(spy.spawn)
	if spy.n != 2 {
		t.Fatalf("after the refresher exited: spawn calls = %d, want 2", spy.n)
	}
}

func TestCacheGet_StaleNoSourceBackoff(t *testing.T) {
	c, clk := newTestCache(t)
	mustWrite(t, c, Record{OK: false, Updated: clk.now().Add(-10 * time.Second).UnixMilli()})

	var spy spyCounter
	if _, show := c.Get(spy.spawn); show {
		t.Fatal("no-source record must not be shown")
	}
	if spy.n != 0 {
		t.Fatalf("spawn called %d times inside NoSourceBackoff, want 0", spy.n)
	}

	// Move past the backoff window: one refresher is allowed again.
	clk.t = clk.t.Add(NoSourceBackoff)
	c.Get(spy.spawn)
	spy.release()
	if spy.n != 1 {
		t.Fatalf("spawn called %d times after NoSourceBackoff, want 1", spy.n)
	}
}

func TestCacheGet_SpawnErrorReleasesLock(t *testing.T) {
	c, clk := newTestCache(t)
	mustWrite(t, c, okRecord(clk.now().Add(-5*time.Second), 6))

	calls := 0
	failing := func(*os.File) error { calls++; return errors.New("exec refused") }
	c.Get(failing)
	if calls != 1 {
		t.Fatalf("failing spawn called %d times, want 1", calls)
	}
	if lockHeld(c) {
		t.Fatal("lock still held after spawn error")
	}

	// The lock is free again, so the next Get may retry.
	c.Get(failing)
	if calls != 2 {
		t.Fatalf("retry after spawn error: calls = %d, want 2", calls)
	}
}

func TestCacheRefresh_Stores(t *testing.T) {
	cases := []struct {
		name      string
		runner    *fakeRunner
		wantOK    bool
		wantUtil  int
		wantSrc   string
		wantStats bool
	}{
		{
			name:      "source answers: record stored OK",
			runner:    only("nvidia-smi", "41, 1024, 8192\n", nil),
			wantOK:    true,
			wantUtil:  41,
			wantSrc:   "nvidia",
			wantStats: true,
		},
		{
			name:   "no source: record stored as no-source",
			runner: only("none", "", errBoom),
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, clk := newTestCache(t)
			// Stale record so the refresh is the thing under test.
			mustWrite(t, c, okRecord(clk.now().Add(-time.Minute), 1))
			col := Collector{Runner: tc.runner, DRMRoot: t.TempDir(), IsWSL: func() bool { return false }}
			if err := c.Refresh(col); err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			r, ok := c.Read()
			if !ok {
				t.Fatal("Refresh did not store a record")
			}
			if r.OK != tc.wantOK {
				t.Fatalf("stored OK = %v, want %v", r.OK, tc.wantOK)
			}
			if tc.wantStats && (r.Util != tc.wantUtil || r.Source != tc.wantSrc) {
				t.Fatalf("stored stats = %+v, want util %d source %q", r.Stats, tc.wantUtil, tc.wantSrc)
			}
			if r.Updated != clk.now().UnixMilli() {
				t.Fatalf("stored Updated = %d, want injected clock %d", r.Updated, clk.now().UnixMilli())
			}
		})
	}
}

func TestCacheRead_CorruptJSONIsAbsent(t *testing.T) {
	for _, body := range []string{"{not json", "", `{"ok":true}`} {
		c, _ := newTestCache(t)
		if err := os.WriteFile(c.dataPath(), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := c.Read(); ok {
			t.Fatalf("Read(%q) ok=true, want absent", body)
		}
		// Corrupt data behaves like no data: the render path spawns and hides.
		var spy spyCounter
		_, show := c.Get(spy.spawn)
		spy.release()
		if show {
			t.Fatalf("corrupt %q shown", body)
		}
		if spy.n != 1 {
			t.Fatalf("corrupt %q: spawn called %d times, want 1", body, spy.n)
		}
	}
}

func TestCacheWrite_RoundTripJSON(t *testing.T) {
	c, clk := newTestCache(t)
	want := okRecord(clk.now(), 17)
	mustWrite(t, c, want)

	b, err := os.ReadFile(filepath.Join(c.Dir, "video.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got Record
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("stored file is not JSON: %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

// TestTryLock_OneWinnerUnderContention: renders racing for the lock produce
// exactly one holder. (The earlier mtime-based stale-lock takeover let two or
// three through.)
func TestTryLock_OneWinnerUnderContention(t *testing.T) {
	for round := 0; round < 50; round++ {
		dir := t.TempDir()
		var wg sync.WaitGroup
		var wins atomic.Int32
		var mu sync.Mutex
		var held []*os.File
		start := make(chan struct{})
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if f, ok := (Cache{Dir: dir}).TryLock(); ok {
					wins.Add(1)
					mu.Lock()
					held = append(held, f)
					mu.Unlock()
				}
			}()
		}
		close(start)
		wg.Wait()
		for _, f := range held {
			_ = f.Close()
		}
		if w := wins.Load(); w != 1 {
			t.Fatalf("round %d: %d renders took the lock, want exactly 1", round, w)
		}
	}
}

// TestTryLock_HeldByChildUntilItExits: the render hands the lock to a child and
// closes its own copy; the lock must stay held for exactly as long as the child
// lives. The child is /bin/sh sleeping, never this test binary.
func TestTryLock_HeldByChildUntilItExits(t *testing.T) {
	c := Cache{Dir: t.TempDir()}
	lock, ok := c.TryLock()
	if !ok {
		t.Fatal("initial lock failed")
	}
	child := exec.Command("/bin/sh", "-c", "sleep 0.5")
	child.ExtraFiles = []*os.File{lock}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	if !lockHeld(c) {
		t.Fatal("lock released while the child holding it is alive")
	}
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	if lockHeld(c) {
		t.Fatal("lock still held after the child exited")
	}
}
