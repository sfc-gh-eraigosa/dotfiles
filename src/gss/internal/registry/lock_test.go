// White-box tests for the locked/atomic registry store per
// src/gss/docs/plan.md PR-18 (resolution #10): concurrent updates
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

	"github.com/wenlock/dotfiles/gss/internal/errors"
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
