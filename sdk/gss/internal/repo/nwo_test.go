// Package repo_test verifies NWO resolution + caching per
// sdk/gss/docs/plan.md PR-09: resolve via gh.Client.RepoView (fake), cache
// hit on the second call, cache invalidation when origin diverges, --repo
// as a read-only shadow, origin-URL fallback, and the refusal path.
package repo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh"
	ghfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/gh/fake"
	gitfake "github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/git/fake"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/repo"
)

func gitWithOrigin(url string) *gitfake.Runner {
	return &gitfake.Runner{Default: gitfake.Response{Stdout: []byte(url + "\n")}}
}

func ghWithRepo(nwo string) *ghfake.Client {
	c := ghfake.NewClient()
	c.SetRepo(gh.RepoInfo{NameWithOwner: nwo})
	return c
}

func repoViewCalls(c *ghfake.Client) int {
	n := 0
	for _, call := range c.Calls() {
		if call.Verb == ghfake.VerbRepoView {
			n++
		}
	}
	return n
}

func TestResolve_ViaGH(t *testing.T) {
	r := repo.NewResolver(ghWithRepo("octo/proj"), gitWithOrigin("git@github.com:octo/proj.git"), repo.NewCache(t.TempDir()))
	nwo, err := r.Resolve(context.Background(), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if nwo.String() != "octo/proj" {
		t.Errorf("nwo = %s; want octo/proj", nwo)
	}
}

func TestResolve_CacheHitSecondCall(t *testing.T) {
	ghc := ghWithRepo("octo/proj")
	r := repo.NewResolver(ghc, gitWithOrigin("git@github.com:octo/proj.git"), repo.NewCache(t.TempDir()))
	ctx := context.Background()

	if _, err := r.Resolve(ctx, ""); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	first := repoViewCalls(ghc)

	nwo, err := r.Resolve(ctx, "")
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if nwo.String() != "octo/proj" {
		t.Errorf("cached nwo = %s; want octo/proj", nwo)
	}
	if repoViewCalls(ghc) != first {
		t.Errorf("second Resolve hit gh again (%d→%d); want cache hit", first, repoViewCalls(ghc))
	}
}

func TestResolve_CacheInvalidatedOnOriginChange(t *testing.T) {
	cache := repo.NewCache(t.TempDir())
	ctx := context.Background()

	// First resolve caches octo/proj against origin A.
	r1 := repo.NewResolver(ghWithRepo("octo/proj"), gitWithOrigin("git@github.com:octo/proj.git"), cache)
	if _, err := r1.Resolve(ctx, ""); err != nil {
		t.Fatalf("seed Resolve: %v", err)
	}

	// Origin now points elsewhere and gh reports a different repo; the
	// stale cache entry (keyed by origin A) must be ignored.
	r2 := repo.NewResolver(ghWithRepo("other/fork"), gitWithOrigin("git@github.com:other/fork.git"), cache)
	nwo, err := r2.Resolve(ctx, "")
	if err != nil {
		t.Fatalf("Resolve after origin change: %v", err)
	}
	if nwo.String() != "other/fork" {
		t.Errorf("nwo = %s; want other/fork (stale cache must not win)", nwo)
	}
}

func TestResolve_OverrideIsReadOnlyShadow(t *testing.T) {
	gitr := gitWithOrigin("git@github.com:octo/proj.git")
	r := repo.NewResolver(ghWithRepo("octo/proj"), gitr, repo.NewCache(t.TempDir()))
	nwo, err := r.Resolve(context.Background(), "manual/override")
	if err != nil {
		t.Fatalf("Resolve(override): %v", err)
	}
	if nwo.String() != "manual/override" {
		t.Errorf("nwo = %s; want manual/override", nwo)
	}
	if gitr.CallCount() != 0 {
		t.Errorf("override should not touch git; CallCount=%d", gitr.CallCount())
	}
}

func TestResolve_OriginFallback(t *testing.T) {
	// gh has no repo (RepoView returns empty NWO) → fall back to origin.
	r := repo.NewResolver(ghfake.NewClient(), gitWithOrigin("https://github.com/foo/bar.git"), repo.NewCache(t.TempDir()))
	nwo, err := r.Resolve(context.Background(), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if nwo.String() != "foo/bar" {
		t.Errorf("nwo = %s; want foo/bar (origin fallback)", nwo)
	}
}

func TestResolve_Refusal(t *testing.T) {
	// gh empty + origin lookup fails → refuse.
	gitErr := &gitfake.Runner{Default: gitfake.Response{Err: errors.New("no origin remote")}}
	r := repo.NewResolver(ghfake.NewClient(), gitErr, repo.NewCache(t.TempDir()))
	if _, err := r.Resolve(context.Background(), ""); err == nil {
		t.Fatal("Resolve with no gh/origin: err = nil; want refusal")
	}
}

func TestParseNWO(t *testing.T) {
	ok, err := repo.ParseNWO("owner/repo")
	if err != nil || ok.Owner != "owner" || ok.Repo != "repo" {
		t.Errorf("ParseNWO(owner/repo) = %+v, %v", ok, err)
	}
	if g, _ := repo.ParseNWO("owner/repo.git"); g.Repo != "repo" {
		t.Errorf("trailing .git not stripped: %+v", g)
	}
	for _, bad := range []string{"", "noslash", "a/b/c", "/b", "a/"} {
		if _, err := repo.ParseNWO(bad); err == nil {
			t.Errorf("ParseNWO(%q): want error", bad)
		}
	}
}

func TestParseRemoteURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:octo/proj.git":         "octo/proj",
		"https://github.com/octo/proj.git":     "octo/proj",
		"https://github.com/octo/proj":         "octo/proj",
		"ssh://git@github.com/octo/proj.git":   "octo/proj",
		"git@gitlab.example.com:group/sub.git": "group/sub",
	}
	for url, want := range cases {
		got, err := repo.ParseRemoteURL(url)
		if err != nil {
			t.Errorf("ParseRemoteURL(%q): %v", url, err)
			continue
		}
		if got.String() != want {
			t.Errorf("ParseRemoteURL(%q) = %s; want %s", url, got, want)
		}
	}
	if _, err := repo.ParseRemoteURL("garbage"); err == nil {
		t.Error("ParseRemoteURL(garbage): want error")
	}
}

func TestCache_RoundTripAndDivergence(t *testing.T) {
	c := repo.NewCache(t.TempDir())
	if _, ok := c.Get("urlA"); ok {
		t.Error("empty cache should miss")
	}
	if err := c.Put("urlA", repo.NWO{Owner: "o", Repo: "r"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got, ok := c.Get("urlA"); !ok || got.String() != "o/r" {
		t.Errorf("Get(urlA) = %s, %v; want o/r, true", got, ok)
	}
	if _, ok := c.Get("urlB"); ok {
		t.Error("Get with diverged origin should miss")
	}
}
