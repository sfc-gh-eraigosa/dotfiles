package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// cache.go — the stale-while-revalidate MCP cache (plan F7).
//
// One JSON file per working directory, ${XDG_CACHE_HOME:-~/.cache}/gsl/mcp/
// <sha256(cwd)[:16]>.json. A render reads it with Get (never spawns); Refresh
// is the only writer and runs out of band, single-flight under a lockfile,
// with failures cached negatively under exponential backoff.

const (
	// defaultTTL is how long a probe stays fresh. The probe takes ~4 s and dials
	// every server, so it is not worth repeating more often than this.
	defaultTTL = 2 * time.Minute
	// probeTimeout bounds one `claude mcp list` (measured 3.4–4.1 s).
	probeTimeout = 30 * time.Second
	// backoffBase and backoffMax bound the negative cache: 30 s, 1 m, 2 m, …
	backoffBase = 30 * time.Second
	backoffMax  = 30 * time.Minute
)

// entry is one cwd's cache file.
type entry struct {
	Cwd       string    `json:"cwd"` // guards against a key collision
	Status    Status    `json:"status"`
	FetchedAt time.Time `json:"fetched_at"` // last successful probe; zero = never
	Failures  int       `json:"failures,omitempty"`
	RetryAt   time.Time `json:"retry_at,omitempty"` // no probe before this
	LastError string    `json:"last_error,omitempty"`
}

// backoff is the wait after the n-th consecutive failure.
func backoff(n int) time.Duration {
	d := backoffBase
	for i := 1; i < n && d < backoffMax; i++ {
		d *= 2
	}
	return min(d, backoffMax)
}

// key is the cache file stem for cwd: the first 16 hex digits of its sha256.
func key(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(sum[:])[:16]
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o Options) ttl() time.Duration {
	if o.TTL > 0 {
		return o.TTL
	}
	return defaultTTL
}

func (o Options) dir() (string, error) {
	if o.CacheDir != "" {
		return o.CacheDir, nil
	}
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "gsl", "mcp"), nil
}

// Get returns the MCP status for cwd from the CACHE ONLY.
//
// CONTRACT — the load-bearing invariant of the whole design:
//
//	Get MUST NEVER spawn a subprocess, and MUST NEVER block on the network.
//
// It is called on the render hot path, once per assistant turn, and `claude
// mcp list` takes 3.4–4.1 s. Configured is recomputed from the config files
// (ConfiguredServers: file reads only). A fresh entry comes back with
// Stale=false; an expired one still comes back — a slightly old count beats
// none — with Stale=true; a cold cache returns only Configured, Stale=true.
// RefreshDue tells the caller to start SpawnRefresh: stale, the backoff has
// passed, and no refresh is already running.
func Get(_ context.Context, cwd string, o Options) (Status, error) {
	if o.Fixed != nil {
		return *o.Fixed, nil
	}
	var s Status
	servers, cfgErr := ConfiguredServers(cwd)
	dir, err := o.dir()
	if err != nil {
		s.Configured, s.Stale = len(servers), true
		return s, err
	}
	now := o.now()
	e, ok := readEntry(filepath.Join(dir, key(cwd)+".json"))
	if ok && e.Cwd == cwd {
		s = e.Status
		s.FetchedAt = e.FetchedAt
	}
	s.Configured = len(servers)
	s.Stale = s.FetchedAt.IsZero() || now.Sub(s.FetchedAt) >= o.ttl()
	s.RefreshDue = s.Stale && !now.Before(e.RetryAt) && !lockHeld(filepath.Join(dir, key(cwd)+".lock"))
	return s, cfgErr
}

// Refresh runs the probe OUT OF BAND and rewrites cwd's cache entry.
//
// This is the ONLY function in the package that shells out; `gsl mcp refresh`
// calls it, and the render path starts that verb detached (SpawnRefresh) —
// never synchronously. The probe runs in the process's working directory, so
// callers run it from cwd (SpawnRefresh sets it).
//
// Single-flight: a lockfile per cwd; a second Refresh while one runs returns
// nil without probing. Negative caching: a failed probe keeps the last good
// status, records the error and backs off (30 s doubling to 30 m); Refresh
// during the backoff returns nil without probing unless o.Force.
func Refresh(ctx context.Context, r Runner, cwd string, o Options) error {
	dir, err := o.dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, key(cwd)+".json")
	release, ok := acquire(filepath.Join(dir, key(cwd)+".lock"))
	if !ok {
		return nil // another refresh is running
	}
	defer release()

	now := o.now()
	e, had := readEntry(path)
	if !had || e.Cwd != cwd {
		e = entry{Cwd: cwd}
	}
	if !o.Force && now.Before(e.RetryAt) {
		return nil // backing off after a failure
	}

	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, runErr := r.Run(pctx, "claude", "mcp", "list")
	st := ParseStatus(out)
	// `claude mcp list` may exit non-zero when some servers fail; a parsed
	// list is still an answer. Only no answer at all is a failure.
	if runErr != nil && len(st.Servers) == 0 {
		e.Failures++
		e.RetryAt = now.Add(backoff(e.Failures))
		e.LastError = runErr.Error()
		_ = writeCache(path, e)
		return runErr
	}
	// FetchedAt is when the answer arrived: the probe itself takes seconds.
	e = entry{Cwd: cwd, Status: st, FetchedAt: o.now()}
	return writeCache(path, e)
}

// acquire takes the single-flight lock (a kernel flock — see tryLock).
func acquire(path string) (release func(), ok bool) {
	f, ok := tryLock(path)
	if !ok {
		return nil, false
	}
	return func() {
		if f != nil {
			_ = f.Close() // closing drops the flock; the file stays, harmlessly
		}
	}, true
}

// lockHeld reports a refresh in flight: someone holds the flock.
func lockHeld(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	release, ok := acquire(path)
	if ok {
		release()
	}
	return !ok
}

// readEntry reads one cache file; ok=false when absent or malformed.
func readEntry(path string) (entry, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return entry{}, false
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return entry{}, false
	}
	return e, true
}

// writeCache serialises v and atomically writes it to path (0600 in a 0700
// directory), creating the directory as needed.
func writeCache(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// os.CreateTemp picks a unique name (concurrent gsl processes never
	// collide) and creates it 0600.
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	renamed := false
	// Whatever fails below, the temp file must not survive.
	defer func() {
		if !renamed {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	renamed = true
	return nil
}
