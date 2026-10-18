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
	LockStale       = 10 * time.Second
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

// TryLock takes the refresher lock (O_EXCL). A lock older than LockStale is
// treated as left by a crashed refresher and replaced.
func (c Cache) TryLock() bool {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return false
	}
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(c.lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return true
		}
		fi, serr := os.Stat(c.lockPath())
		if serr != nil || c.now().Sub(fi.ModTime()) <= LockStale {
			return false
		}
		_ = os.Remove(c.lockPath())
	}
	return false
}

// Unlock releases the refresher lock.
func (c Cache) Unlock() { _ = os.Remove(c.lockPath()) }

// Get is the render-path entry point. It reads the cache and, when the data is
// stale, starts exactly one refresher via spawn (guarded by the lock) without
// waiting for it. It returns the last known reading; show=false means there is
// nothing to display (no data yet, or no GPU source on this host).
func (c Cache) Get(spawn func() error) (s Stats, show bool) {
	r, have := c.Read()
	if c.stale(r, have) && spawn != nil && c.TryLock() {
		if err := spawn(); err != nil {
			c.Unlock()
		}
	}
	if !have || !r.OK {
		return Stats{}, false
	}
	return r.Stats, true
}

// Refresh is what the detached refresher runs: collect, store, unlock.
func (c Cache) Refresh(col Collector) error {
	defer c.Unlock()
	return c.refresh(col)
}

func (c Cache) refresh(col Collector) error {
	ctx, cancel := context.WithTimeout(context.Background(), RefreshTimeout)
	defer cancel()
	s, err := col.Collect(ctx)
	return c.Store(s, err)
}
