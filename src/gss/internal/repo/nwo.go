// Package repo resolves a repository's name-with-owner (NWO) — the
// canonical <owner>/<repo> identity gss keys worktrees and registry
// entries by (design.md → "Repo identity"; resolution #12).
//
// Resolution order (when no --repo override is given):
//
//  1. file cache at <worktrees_root>/.nwo, keyed by the current origin URL
//     (a changed origin invalidates the entry);
//  2. gh.Client.RepoView → nameWithOwner;
//  3. fall back to parsing `git remote get-url origin`.
//
// If none resolve, Resolve refuses and tells the caller to pass --repo.
package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/errors"
	"github.com/wenlock/dotfiles/gss/internal/gh"
	"github.com/wenlock/dotfiles/gss/internal/git"
)

// NWO is a name-with-owner: the canonical <owner>/<repo> repo identity.
type NWO struct {
	Owner string
	Repo  string
}

// String renders "<owner>/<repo>".
func (n NWO) String() string { return n.Owner + "/" + n.Repo }

// Valid reports whether both segments are present.
func (n NWO) Valid() bool { return n.Owner != "" && n.Repo != "" }

// ParseNWO parses an "<owner>/<repo>" string (the --repo override form). A
// trailing ".git" on the repo is stripped.
func ParseNWO(s string) (NWO, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return NWO{}, fmt.Errorf("%w: repo %q must be <owner>/<repo>", errors.ErrInvalidIdent, s)
	}
	return NWO{Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git")}, nil
}

// ParseRemoteURL extracts an NWO from a git remote URL, handling the
// scp-like (git@host:owner/repo.git), https, and ssh:// forms.
func ParseRemoteURL(url string) (NWO, error) {
	u := strings.TrimSuffix(strings.TrimSpace(url), ".git")
	switch {
	case strings.Contains(u, "://"):
		// scheme://[user@]host/owner/repo — drop scheme + host.
		u = u[strings.Index(u, "://")+3:]
		if s := strings.IndexByte(u, '/'); s >= 0 {
			u = u[s+1:]
		}
	case strings.Contains(u, "@") && strings.Contains(u, ":"):
		// scp-like git@host:owner/repo — keep the part after the colon.
		u = u[strings.LastIndex(u, ":")+1:]
	}
	parts := strings.Split(strings.Trim(u, "/"), "/")
	if len(parts) < 2 {
		return NWO{}, fmt.Errorf("%w: cannot parse owner/repo from remote URL %q", errors.ErrInvalidIdent, url)
	}
	owner, repoName := parts[len(parts)-2], parts[len(parts)-1]
	if owner == "" || repoName == "" {
		return NWO{}, fmt.Errorf("%w: empty owner/repo in remote URL %q", errors.ErrInvalidIdent, url)
	}
	return NWO{Owner: owner, Repo: repoName}, nil
}

// Resolver resolves a repo's NWO via gh (with an origin-URL fallback) and
// caches the result. A nil Cache disables caching.
type Resolver struct {
	GH    gh.Client
	Git   git.Runner
	Cache *Cache
}

// NewResolver wires the dependencies.
func NewResolver(ghc gh.Client, gitr git.Runner, cache *Cache) *Resolver {
	return &Resolver{GH: ghc, Git: gitr, Cache: cache}
}

// Resolve returns the NWO. A non-empty override is parsed and returned
// directly as a read-only shadow — gh, origin, and the cache are not
// touched and the result is not cached.
func (r *Resolver) Resolve(ctx context.Context, override string) (NWO, error) {
	if override != "" {
		return ParseNWO(override)
	}

	originURL := r.originURL(ctx)
	if r.Cache != nil && originURL != "" {
		if nwo, ok := r.Cache.Get(originURL); ok {
			return nwo, nil
		}
	}

	nwo := r.viaGH(ctx)
	if !nwo.Valid() && originURL != "" {
		if parsed, err := ParseRemoteURL(originURL); err == nil {
			nwo = parsed
		}
	}
	if !nwo.Valid() {
		return NWO{}, fmt.Errorf("repo: could not resolve name-with-owner from gh or origin; pass --repo <owner>/<repo>")
	}

	if r.Cache != nil && originURL != "" {
		_ = r.Cache.Put(originURL, nwo)
	}
	return nwo, nil
}

// originURL returns `git remote get-url origin`, or "" if unavailable.
func (r *Resolver) originURL(ctx context.Context) string {
	if r.Git == nil {
		return ""
	}
	out, err := r.Git.Run(ctx, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// viaGH resolves the NWO from gh.RepoView, returning an invalid NWO on any
// failure so Resolve falls through to the origin parse.
func (r *Resolver) viaGH(ctx context.Context) NWO {
	if r.GH == nil {
		return NWO{}
	}
	info, err := r.GH.RepoView(ctx)
	if err != nil {
		return NWO{}
	}
	if nwo, err := ParseNWO(info.NameWithOwner); err == nil {
		return nwo
	}
	return NWO{}
}
