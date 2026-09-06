package registry

import (
	stderrors "errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/gofrs/flock"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
)

// Store is a file-backed registry with advisory-locked, atomic
// read-modify-write semantics (design.md resolution #10): every mutation
// holds an exclusive flock on <dir>/.registry.lock, writes go through a
// temp file + rename(2), the file is mode 0600, and gss refuses to operate
// on a registry.json owned by a different user.
type Store struct {
	// Path is the registry.json location.
	Path string
	// LockPath is the advisory lock file; defaults to <dir>/.registry.lock.
	LockPath string
	// euid returns the effective uid to compare against the file owner;
	// nil uses os.Geteuid. Injected in tests to exercise the refusal path.
	euid func() int
}

// NewStore returns a Store for registry.json at path.
func NewStore(path string) *Store {
	return &Store{
		Path:     path,
		LockPath: filepath.Join(filepath.Dir(path), ".registry.lock"),
	}
}

func (s *Store) effectiveUID() int {
	if s.euid != nil {
		return s.euid()
	}
	return os.Geteuid()
}

// Update runs fn under an exclusive lock, passing the current registry
// (empty if the file is absent) and atomically persisting the mutated
// result. fn returning an error aborts the write, leaving registry.json
// untouched.
func (s *Store) Update(fn func(*Registry) error) error {
	lk := flock.New(s.LockPath)
	if err := lk.Lock(); err != nil {
		return fmt.Errorf("registry: acquire lock %s: %w", s.LockPath, err)
	}
	defer func() { _ = lk.Unlock() }()

	reg, err := s.readLocked()
	if err != nil {
		return err
	}
	if err := fn(&reg); err != nil {
		return err
	}
	return s.writeLocked(reg)
}

// Load reads the registry under a shared lock. A missing file yields an
// empty registry at the supported schema version.
func (s *Store) Load() (Registry, error) {
	lk := flock.New(s.LockPath)
	if err := lk.RLock(); err != nil {
		return Registry{}, fmt.Errorf("registry: acquire rlock %s: %w", s.LockPath, err)
	}
	defer func() { _ = lk.Unlock() }()
	return s.readLocked()
}

func (s *Store) readLocked() (Registry, error) {
	if err := s.checkOwner(); err != nil {
		return Registry{}, err
	}
	data, err := os.ReadFile(s.Path)
	if stderrors.Is(err, fs.ErrNotExist) {
		return Registry{SchemaVersion: SupportedSchemaVersion}, nil
	}
	if err != nil {
		return Registry{}, fmt.Errorf("registry: read %s: %w", s.Path, err)
	}
	return Unmarshal(data)
}

func (s *Store) writeLocked(reg Registry) error {
	if reg.SchemaVersion == 0 {
		reg.SchemaVersion = SupportedSchemaVersion
	}
	data, err := Marshal(reg)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("registry: mkdir %s: %w", dir, err)
		}
	}
	// Temp file in the same dir so rename(2) is atomic (same filesystem).
	tmp, err := os.CreateTemp(dir, ".registry-*.tmp")
	if err != nil {
		return fmt.Errorf("registry: create temp: %w", err)
	}
	tmpName := tmp.Name()
	// No-op after a successful rename; nothing to report if the temp file is
	// already gone.
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("registry: write temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("registry: chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("registry: close temp: %w", err)
	}
	if err := os.Rename(tmpName, s.Path); err != nil {
		return fmt.Errorf("registry: atomic rename: %w", err)
	}
	return nil
}

// checkOwner refuses (errors.ErrPermissionMode) when registry.json exists
// and is owned by a uid other than our effective uid. A missing file or a
// non-unix stat is allowed.
func (s *Store) checkOwner() error {
	fi, err := os.Stat(s.Path)
	if err != nil {
		if stderrors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("registry: stat %s: %w", s.Path, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil // non-unix platform; skip the uid guard
	}
	if int(st.Uid) != s.effectiveUID() {
		return fmt.Errorf("%w: registry.json owned by uid %d but effective uid is %d",
			errors.ErrPermissionMode, st.Uid, s.effectiveUID())
	}
	return nil
}
