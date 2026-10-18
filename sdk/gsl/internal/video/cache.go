package video

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Cache timing. FreshFor is how long a reading is served without a refresh;
// NoSourceBackoff is the (longer) window after a refresh found no source at
// all, so hosts without a GPU probe (macOS, Jetson) spawn at most one
// refresher per window instead of one per render.
const (
	FreshFor        = time.Second
	NoSourceBackoff = 30 * time.Second
	RefreshTimeout  = 5 * time.Second
)

// Record is the on-disk cache payload ($XDG_STATE_HOME/gsl/video.json).
type Record struct {
	OK      bool  `json:"ok"` // false: refresh ran and no source exists
	Stats         // embedded
	Updated int64 `json:"updated_unix_ms"`
}

// Dir is ${XDG_STATE_HOME:-$HOME/.local/state}/gsl, gsl's own state dir
// (the same base internal/usage uses).
func Dir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "gsl"), nil
}

// Cache reads/writes the data file and arbitrates the refresher lock.
type Cache struct {
	Dir string
	Now func() time.Time // nil => time.Now
}

func (c Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c Cache) dataPath() string { return filepath.Join(c.Dir, "video.json") }
func (c Cache) lockPath() string { return filepath.Join(c.Dir, "video.lock") }

// Read returns the cached record, ok=false when absent or corrupt.
func (c Cache) Read() (Record, bool) {
	b, err := os.ReadFile(c.dataPath())
	if err != nil {
		return Record{}, false
	}
	var r Record
	if json.Unmarshal(b, &r) != nil || r.Updated == 0 {
		return Record{}, false
	}
	return r, true
}

// Write stores a record atomically (temp file + rename).
func (c Cache) Write(r Record) error {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.Dir, "video-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), c.dataPath()); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Store stamps and writes the outcome of one collection.
func (c Cache) Store(s Stats, err error) error {
	r := Record{OK: err == nil, Updated: c.now().UnixMilli()}
	if err == nil {
		r.Stats = s
	}
	return c.Write(r)
}

func (c Cache) stale(r Record, have bool) bool {
	if !have {
		return true
	}
	age := c.now().Sub(time.UnixMilli(r.Updated))
	if r.OK {
		return age > FreshFor
	}
	return age > NoSourceBackoff
}

// SpawnFunc starts a refresher that inherits lock. The refresher holds the
// lock until it exits; the caller closes its own copy right after.
type SpawnFunc func(lock *os.File) error

// TryLock takes the refresher lock without waiting. It is a kernel lock
// (flock) on video.lock, so it is released when the last process holding it
// exits, crashes included: there is no stale lock to guess about. Two renders
// can never both take it. On platforms without flock it always fails, which
// leaves the segment hidden there.
func (c Cache) TryLock() (*os.File, bool) {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return nil, false
	}
	return tryLockFile(c.lockPath())
}

// Get is the render-path entry point. It reads the cache and, when the data is
// stale and no refresher holds the lock, starts one via spawn, handing it the
// lock, without waiting for it. It returns the last known reading; show=false
// means there is nothing to display (no data yet, or no GPU source here).
func (c Cache) Get(spawn SpawnFunc) (s Stats, show bool) {
	r, have := c.Read()
	if c.stale(r, have) && spawn != nil {
		if lock, ok := c.TryLock(); ok {
			_ = spawn(lock) // on error nobody inherited the lock; Close frees it
			_ = lock.Close()
		}
	}
	if !have || !r.OK {
		return Stats{}, false
	}
	return r.Stats, true
}

// Refresh is what the detached refresher runs: collect and store. The lock it
// inherited is released when the refresher process exits.
func (c Cache) Refresh(col Collector) error {
	return c.refresh(col)
}

func (c Cache) refresh(col Collector) error {
	ctx, cancel := context.WithTimeout(context.Background(), RefreshTimeout)
	defer cancel()
	s, err := col.Collect(ctx)
	return c.Store(s, err)
}
