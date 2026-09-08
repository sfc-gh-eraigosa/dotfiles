// Package ghapp is the GitHub App credential toolkit: create an App by the
// manifest flow, keep its id + private key in a private store, sign App JWTs,
// and mint installation tokens. Contracts: docs/mbo/plans/gcfg.md §3.2.
//
// Nothing in this package ever logs or prints a token or key.
package ghapp

import (
	"crypto/rsa"
	"net/http"
	"sync"
	"time"
)

// App is one GitHub App the store knows about.
//
// Plan §3.2 names the map field `Installations`; Go cannot carry a field and
// a method of the same name, so the field is `Installs` (JSON key unchanged)
// and the method keeps the contract name. Recorded in TRACKING §4.
type App struct {
	ID       int64            `json:"id"`
	Slug     string           `json:"slug"`
	PEMPath  string           `json:"pem_path"`
	Installs map[string]int64 `json:"installations,omitempty"` // "owner" or "owner/repo" → installation id

	deps *deps
}

// Apps is every stored App, keyed by slug.
type Apps map[string]App

// Store persists Apps. The file implementation lives in store.go.
type Store interface {
	Load() (Apps, error)
	Save(Apps) error
}

// Options are the injectable dependencies of an App's API calls. Zero values
// mean: key loaded from PEMPath on first use, http.DefaultClient,
// https://api.github.com, time.Now.
type Options struct {
	Key     *rsa.PrivateKey
	HTTP    *http.Client
	BaseURL string
	Now     func() time.Time
}

// deps is shared by every copy of an App returned from With, so the token
// cache survives value-receiver calls.
type deps struct {
	key     *rsa.PrivateKey
	http    *http.Client
	baseURL string
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]Token
}

// With returns a copy of the App wired to o.
func (a App) With(o Options) App {
	d := &deps{key: o.Key, http: o.HTTP, baseURL: o.BaseURL, now: o.Now, cache: map[string]Token{}}
	if d.http == nil {
		d.http = http.DefaultClient
	}
	if d.baseURL == "" {
		d.baseURL = "https://api.github.com"
	}
	if d.now == nil {
		d.now = time.Now
	}
	a.deps = d
	return a
}
