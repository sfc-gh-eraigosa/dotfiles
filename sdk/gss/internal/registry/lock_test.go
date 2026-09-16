// White-box tests for the locked/atomic registry store per
// sdk/gss/docs/plan.md PR-18 (resolution #10): concurrent updates
// serialize, conflicting ops don't corrupt, fn errors leave the file
// untouched, writes are 0600, and a uid mismatch is refused. Package
// registry (not registry_test) so the injectable euid can be set.
//
// Note: `go test -race` cannot run on the aarch64 dev host (ThreadSanitizer
// VMA limitation); these run without it. x86 CI exercises -race.
package registry

import (
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
)

func tmpStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "registry.json"))
}

// addWorker appends a worker with the given purpose, creating the feature
// if needed.
func addWorker(r *Registry, purpose string) {
	if len(r.Features) == 0 {
		r.Features = append(r.Features, Feature{Name: "f", DefaultBaseBranch: "main"})
	}
	r.Features[0].Workers = append(r.Features[0].Workers, Worker{
		User: "u", Purpose: purpose, Branch: "b/" + purpose,
		Worktree: "/wt/" + purpose, BaseBranch: "main", Backend: "git",
		StartedAt: "t", Description: "d",
	})
}

func TestUpdate_CreatesFile0600(t *testing.T) {
	s := tmpStore(t)
	if err := s.Update(func(r *Registry) error { addWorker(r, "api"); return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	fi, err := os.Stat(s.Path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("registry.json mode = %o; want 0600", perm)
	}
	reg, _ := s.Load()
	if len(reg.Features) != 1 || len(reg.Features[0].Workers) != 1 {
		t.Errorf("expected 1 feature/1 worker; got %+v", reg)
	}
}

func TestConcurrentWorkerAdd(t *testing.T) {
	s := tmpStore(t)
	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			err := s.Update(func(r *Registry) error {
				addWorker(r, fmt.Sprintf("p%d", i))
				return nil
			})
			if err != nil {
				t.Errorf("Update %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	reg, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := 0
	if len(reg.Features) > 0 {
		got = len(reg.Features[0].Workers)
	}
	if got != n {
		t.Errorf("workers = %d; want %d (lost updates → lock not serializing)", got, n)
	}
	// All purposes distinct → no clobbered writes.
	seen := map[string]bool{}
	for _, w := range reg.Features[0].Workers {
		if seen[w.Purpose] {
			t.Errorf("duplicate worker purpose %q", w.Purpose)
		}
		seen[w.Purpose] = true
	}
}

func TestDoneRacingCheckpoint(t *testing.T) {
	s := tmpStore(t)
	if err := s.Update(func(r *Registry) error { addWorker(r, "api"); return nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	// "checkpoint": mutate the worker's description.
	go func() {
		defer wg.Done()
		_ = s.Update(func(r *Registry) error {
			if len(r.Features) > 0 && len(r.Features[0].Workers) > 0 {
				r.Features[0].Workers[0].Description = "updated"
			}
			return nil
		})
	}()
	// "done": remove the worker.
	go func() {
		defer wg.Done()
		_ = s.Update(func(r *Registry) error {
			if len(r.Features) > 0 {
				r.Features[0].Workers = nil
			}
			return nil
		})
	}()
	wg.Wait()

	// Whichever ran last, the registry must still parse and be coherent.
	reg, err := s.Load()
	if err != nil {
		t.Fatalf("Load after race: %v (file corrupted?)", err)
	}
	if len(reg.Features) != 1 {
		t.Errorf("feature count = %d; want 1", len(reg.Features))
	}
	if n := len(reg.Features[0].Workers); n > 1 {
		t.Errorf("worker count = %d; want 0 or 1", n)
	}
}

func TestUpdate_FnErrorLeavesFileUntouched(t *testing.T) {
	s := tmpStore(t)
	if err := s.Update(func(r *Registry) error { addWorker(r, "api"); return nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, _ := os.ReadFile(s.Path)

	wantErr := stderrors.New("abort")
	if err := s.Update(func(r *Registry) error {
		addWorker(r, "ui") // mutate, then abort
		return wantErr
	}); !stderrors.Is(err, wantErr) {
		t.Fatalf("Update err = %v; want abort", err)
	}

	after, _ := os.ReadFile(s.Path)
	if string(before) != string(after) {
		t.Error("aborted Update modified registry.json; want it untouched")
	}
	assertNoTempLeftover(t, filepath.Dir(s.Path))
}

func TestUpdate_NoTempLeftoverOnSuccess(t *testing.T) {
	s := tmpStore(t)
	if err := s.Update(func(r *Registry) error { addWorker(r, "api"); return nil }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertNoTempLeftover(t, filepath.Dir(s.Path))
}

func TestCheckOwner_RefusesUidMismatch(t *testing.T) {
	s := tmpStore(t)
	if err := s.Update(func(r *Registry) error { addWorker(r, "api"); return nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Pretend our effective uid differs from the file owner.
	s.euid = func() int { return os.Geteuid() + 99999 }

	if _, err := s.Load(); err == nil {
		t.Fatal("Load with uid mismatch: err = nil; want refusal")
	} else if !stderrors.Is(err, errors.ErrPermissionMode) {
		t.Errorf("err = %v; want wrapping ErrPermissionMode", err)
	}
}

// TestUpdate_ReturnsErrLockHeldOnContention pins the dotfiles#97 fix: the
// registry lock is advertised as failing fast with errors.ErrLockHeld /
// exit code 13 when contended, but Update called the blocking flock APIs
// (Lock/RLock) and never returned that sentinel — a second gss process
// contending for the registry (the exact multi-writer / multi-agent
// scenario the lock exists for) blocked forever instead. This holds the
// lock externally (simulating another process) and asserts Update returns
// promptly with ErrLockHeld rather than hanging.
func TestUpdate_ReturnsErrLockHeldOnContention(t *testing.T) {
	s := tmpStore(t)
	// Seed so the lock file exists at s.LockPath.
	if err := s.Update(func(r *Registry) error { addWorker(r, "api"); return nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	holder := flock.New(s.LockPath)
	if ok, err := holder.TryLock(); err != nil || !ok {
		t.Fatalf("test setup: could not hold the lock externally: ok=%v err=%v", ok, err)
	}
	defer func() { _ = holder.Unlock() }()

	s.LockTimeout = 100 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		done <- s.Update(func(r *Registry) error { addWorker(r, "contended"); return nil })
	}()
	select {
	case err := <-done:
		if !stderrors.Is(err, errors.ErrLockHeld) {
			t.Fatalf("err = %v; want ErrLockHeld", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Update blocked far longer than LockTimeout — still hangs forever on contention")
	}
}

// TestLoad_ReturnsErrLockHeldOnContention mirrors the above for the shared
// (RLock) path: an exclusive holder blocks a would-be reader too, and Load
// must fail fast the same way.
func TestLoad_ReturnsErrLockHeldOnContention(t *testing.T) {
	s := tmpStore(t)
	if err := s.Update(func(r *Registry) error { addWorker(r, "api"); return nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	holder := flock.New(s.LockPath)
	if ok, err := holder.TryLock(); err != nil || !ok {
		t.Fatalf("test setup: could not hold the lock externally: ok=%v err=%v", ok, err)
	}
	defer func() { _ = holder.Unlock() }()

	s.LockTimeout = 100 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		_, err := s.Load()
		done <- err
	}()
	select {
	case err := <-done:
		if !stderrors.Is(err, errors.ErrLockHeld) {
			t.Fatalf("err = %v; want ErrLockHeld", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Load blocked far longer than LockTimeout — still hangs forever on contention")
	}
}

// TestLoad_SharedLocksDoNotContend guards against an overcorrection: two
// concurrent readers must NOT fail each other with ErrLockHeld — only an
// exclusive holder should ever cause a timeout.
func TestLoad_SharedLocksDoNotContend(t *testing.T) {
	s := tmpStore(t)
	if err := s.Update(func(r *Registry) error { addWorker(r, "api"); return nil }); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s.LockTimeout = 200 * time.Millisecond
	var wg sync.WaitGroup
	errs := make([]error, 10)
	wg.Add(10)
	for i := range errs {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.Load()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent Load %d: %v; want nil (shared locks must not contend)", i, err)
		}
	}
}

// TestConcurrentWorkerAdd_StillSucceedsWithBoundedTimeout re-runs the
// existing high-contention same-process scenario with a short LockTimeout
// to confirm the fix's retry loop still lets fast, brief holders all
// succeed — the bounded wait must not turn ordinary same-process
// contention into spurious ErrLockHeld failures.
func TestConcurrentWorkerAdd_StillSucceedsWithBoundedTimeout(t *testing.T) {
	s := tmpStore(t)
	s.LockTimeout = 2 * time.Second
	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			if err := s.Update(func(r *Registry) error {
				addWorker(r, fmt.Sprintf("q%d", i))
				return nil
			}); err != nil {
				t.Errorf("Update %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	reg, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(reg.Features[0].Workers); got != n {
		t.Errorf("workers = %d; want %d", got, n)
	}
}

func assertNoTempLeftover(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
