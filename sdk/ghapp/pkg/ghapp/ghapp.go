// Package ghapp is the GitHub App credential toolkit: create an App by the
// manifest flow, keep its id + private key in a private store, sign App JWTs,
// and mint installation tokens. Contracts: docs/mbo/plans/gcfg.md §3.2.
//
// Nothing in this package ever logs or prints a token or key.
package ghapp

// App is one GitHub App the store knows about.
type App struct {
	ID            int64            `json:"id"`
	Slug          string           `json:"slug"`
	PEMPath       string           `json:"pem_path"`
	Installations map[string]int64 `json:"installations,omitempty"` // "owner" or "owner/repo" → installation id
}

// Apps is every stored App, keyed by slug.
type Apps map[string]App

// Store persists Apps. The file implementation lives in store.go.
type Store interface {
	Load() (Apps, error)
	Save(Apps) error
}
