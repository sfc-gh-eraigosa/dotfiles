package gh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PRInfo holds the result of a `gh pr view` lookup.
type PRInfo struct {
	Number int
	State  string
	// URL is the PR's web URL, used by the render layer to emit an OSC 8
	// hyperlink. Empty when gh did not report one (older gh, or a cache entry
	// written before this field existed).
	URL string
}

// prCache is the on-disk JSON structure for the cache file.
type prCache struct {
	Number int       `json:"number"`
	State  string    `json:"state"`
	URL    string    `json:"url,omitempty"`
	TS     time.Time `json:"ts"`
	// TTLSeconds is this entry's OWN lifetime. It exists so a timeout-class
	// failure can expire faster than a conclusive answer (E19 edge). Zero means
	// "use cacheMaxAge" — which is what every cache file written before this
	// field existed will unmarshal to, so old files keep working.
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

// cacheMaxAge is the default entry lifetime: a conclusive answer from gh,
// positive ("PR #123 is OPEN") or negative ("this branch has no PR").
const cacheMaxAge = 60 * time.Second

// transientTTL is the lifetime of an INCONCLUSIVE entry — gh was still running
// when the deadline fired, so we never learned anything.
//
// It is cached at all so that a hanging gh is not re-dialled on every single
// render (each attempt costs the full 800 ms timeout on the hot path). It is
// cached BRIEFLY because the answer is unknown, not "no": a network blip must
// not blind the status line to a real PR for a full minute.
const transientTTL = 5 * time.Second

// prViewTimeout is the per-invocation timeout for `gh pr view`.
const prViewTimeout = 800 * time.Millisecond

// sanitizeBranch replaces characters that are unsafe in a filename.
// Primarily replaces forward-slashes used in git branch names.
func sanitizeBranch(branch string) string {
	return strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	).Replace(branch)
}

// repoRoot walks up from dir looking for a .git entry and returns the first
// directory that has one. It returns "" when dir is not inside a repository.
//
// This is a pure filesystem walk (a handful of stat calls) on purpose: it must
// NOT fork a `git rev-parse`, or it would re-introduce on the hot render path
// exactly the subprocess cost this workstream exists to remove.
func repoRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// repoScope returns a short, stable discriminator for the repository that `gh`
// will actually answer about.
//
// # Why the cache key cannot be the branch alone
//
// The cache lives in ONE global directory (~/.cache/gsl), but the key was just
// "pr-<branch>.json" — no repository component. Every repo on the machine that
// shares a branch name therefore shares a cache entry. Two ways that goes wrong:
//
//   - A repo with no PR on `main` caches a NEGATIVE entry under pr-main.json,
//     which then suppresses the real PR of any OTHER repo whose `main` has one.
//   - Conversely a positive entry leaks a PR number into an unrelated repo.
//
// The negative direction only became reachable when negative caching landed
// (E19/F13) — before that the no-PR path wrote nothing — and negative results
// are the COMMON case, so the poisoned key would be written constantly.
//
// The scope is derived from the PROCESS cwd, not the payload cwd, because the
// gh Runner does not set cmd.Dir: `gh pr view` inherits gsl's own cwd and
// resolves the repository from it. Keying on the process cwd's repo root is
// therefore keying on precisely the repo whose answer we are storing.
func repoScope() string {
	wd, err := os.Getwd()
	if err != nil {
		return "norepo"
	}
	root := repoRoot(wd)
	if root == "" {
		return "norepo"
	}
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:4])
}

// cacheFilePath returns the path to the cache file for the given branch,
// scoped to the repository gh will be asked about (see repoScope).
func cacheFilePath(branch string) string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	safe := sanitizeBranch(branch)
	return filepath.Join(base, "gsl", "pr-"+safe+"-"+repoScope()+".json")
}

// readCache attempts to load a valid (non-stale) cache entry for branch.
// Returns nil if the file is absent, unreadable, invalid, or past its TTL.
func readCache(path string) *prCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c prCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	ttl := cacheMaxAge
	if c.TTLSeconds > 0 {
		ttl = time.Duration(c.TTLSeconds) * time.Second
	}
	if time.Since(c.TS) >= ttl {
		return nil
	}
	return &c
}

// writeCache persists the PRInfo to the cache file at path with an explicit TTL.
// Errors are silently ignored — a failed write is non-fatal (we just re-probe).
func writeCache(path string, info *PRInfo, ttl time.Duration) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	c := prCache{
		Number:     info.Number,
		State:      info.State,
		URL:        info.URL,
		TS:         time.Now(),
		TTLSeconds: int(ttl.Seconds()),
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// cacheNegative remembers "there is no PR for this branch" for ttl.
func cacheNegative(path string, ttl time.Duration) {
	writeCache(path, &PRInfo{}, ttl)
}

// inconclusive reports whether the call failed because we RAN OUT OF TIME
// rather than because gh gave us an answer. Those two get different TTLs:
// an answer is good for a minute; a non-answer for five seconds.
func inconclusive(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// PR returns the PR number and state for the given branch.
//
// Contract: when there is no PR, or gh is absent / unreachable, PR returns
// (nil, nil) — the caller must treat nil PRInfo as "omit the PR# field".
// A non-nil error is returned only for internal logic failures that the caller
// cannot work around by omitting output (currently none in this path).
//
// # Caching (F13 / E19)
//
// A file at ${XDG_CACHE_HOME:-$HOME/.cache}/gsl/pr-<branch>.json is consulted
// first. On a live entry the Runner is NOT called; an entry whose Number is 0
// is a remembered "no PR" and yields (nil, nil) with no gh invocation.
//
// EVERY outcome is cached, and that is the point. The shipped code cached only
// the `{"number":0}` shape — but that is not how gh reports "no PR". gh EXITS 1
// and writes "no pull requests found for branch" to stderr, so the real no-PR
// path took the `err != nil` branch and returned WITHOUT writing the cache.
// Nothing was ever cached for a PR-less branch, so `gh pr view` — a ~760 ms
// NETWORK call that forks three more git subprocesses of its own — re-ran on
// EVERY render, forever. On `main`, which normally has no PR, that was the
// single most expensive thing gsl did, on every assistant turn.
//
// TTLs differ by how much we learned:
//   - a conclusive answer (a PR, or "no PR")      → cacheMaxAge  (60 s)
//   - inconclusive (deadline fired, we know zip)  → transientTTL  (5 s)
//
// A nil Runner is a no-op returning (nil, nil): it used to nil-deref and panic,
// and because render's Detect recovers from segment panics, the panic showed up
// as nothing worse than a silently missing segment — a test could stay green on
// a panicking path.
func PR(ctx context.Context, runner Runner, branch string) (*PRInfo, error) {
	path := cacheFilePath(branch)

	// Cache hit — return without touching the Runner.
	if c := readCache(path); c != nil {
		// A cached Number of 0 is a remembered "no PR" result: honor the
		// (nil, nil) omit contract instead of fabricating a PRInfo{Number:0}.
		if c.Number == 0 {
			return nil, nil
		}
		return &PRInfo{Number: c.Number, State: c.State, URL: c.URL}, nil
	}

	// No runner (Deps{GH: nil}): nothing to ask, nothing learned. Do NOT cache
	// — the on-disk cache is shared with processes that DO have a gh.
	if runner == nil {
		return nil, nil
	}

	// Apply a per-call timeout so we never block the status line for long.
	tctx, cancel := context.WithTimeout(ctx, prViewTimeout)
	defer cancel()

	// Forward the branch as a positional arg so gh resolves the PR for the
	// requested branch (matching the cache key) rather than the cwd branch.
	out, err := runner.Run(tctx, "pr", "view", branch, "--json", "number,state,url")
	if err != nil {
		// gh exited non-zero (the NORMAL "no PR for this branch" signal), gh is
		// absent, or the deadline fired. All map to "omit" — but they are now
		// all REMEMBERED, at a TTL that reflects how conclusive they were.
		if inconclusive(tctx, err) {
			cacheNegative(path, transientTTL)
		} else {
			cacheNegative(path, cacheMaxAge)
		}
		return nil, nil //nolint:nilerr
	}

	// gh exited 0 but said nothing. Conclusive enough: there is no PR.
	if len(out) == 0 {
		cacheNegative(path, cacheMaxAge)
		return nil, nil
	}

	var payload struct {
		Number int    `json:"number"`
		State  string `json:"state"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		// gh answered in a shape we don't understand. Don't re-dial it every
		// render, but expire fast: a gh upgrade must not blind us for a minute.
		cacheNegative(path, transientTTL)
		return nil, nil //nolint:nilerr
	}

	// Number == 0 generally means no PR was found. Cache the negative result
	// so PR-less repos don't pay the full gh invocation every render.
	if payload.Number == 0 {
		writeCache(path, &PRInfo{Number: 0, State: payload.State}, cacheMaxAge)
		return nil, nil
	}

	info := &PRInfo{Number: payload.Number, State: payload.State, URL: payload.URL}
	writeCache(path, info, cacheMaxAge)
	return info, nil
}
