// Package flags resolves the gff feature flags gsl consults at render time.
//
// Every lookup is fail-open: an error, an unregistered source, a missing gff
// schema, or a slow answer all mean "on" — a flag can only ever turn a link
// family OFF. This mirrors install.sh's gff_on gate, which fails open for the
// same reason (a fresh machine must render exactly as if gff were absent).
//
// The package is the only place gsl touches the gff SDK, and it is called from
// cmd, never from render (render stays free of subprocess-shaped dependencies).
package flags

import (
	"context"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"
)

// Namespace is the dotfiles repo's gff namespace (.github/gff/features.yaml).
// `gff install` from the checkout registers it on a host; install.sh does that.
const Namespace = "com.github.sfc-gh-eraigosa.dotfiles"

// The five flags. Enabled is the master; the rest are link families.
const (
	KeyEnabled = "gsl.links.enabled"
	KeyRepo    = "gsl.links.repo"
	KeyDirGit  = "gsl.links.dirgit"
	KeyAI      = "gsl.links.ai"
	KeyTime    = "gsl.links.time"
)

// Links is the resolved flag set. The zero value is NOT the default — use
// Resolve, which starts from all-true.
type Links struct{ Enabled, Repo, DirGit, AI, Time bool }

// Lookup answers one flag key. It is the seam tests replace with a fake.
type Lookup func(key string) (bool, error)

// Resolve evaluates the five keys concurrently and returns when all have
// answered or ctx is done; keys that errored or never answered read true.
// The goroutines of a timed-out lookup are abandoned, not joined — the render
// must not wait on them.
func Resolve(ctx context.Context, look Lookup) Links {
	out := Links{Enabled: true, Repo: true, DirGit: true, AI: true, Time: true}
	if look == nil {
		return out
	}
	type answer struct {
		key string
		val bool
	}
	keys := []string{KeyEnabled, KeyRepo, KeyDirGit, KeyAI, KeyTime}
	ch := make(chan answer, len(keys))
	for _, k := range keys {
		go func(k string) {
			v, err := look(k)
			if err != nil {
				v = true
			}
			ch <- answer{k, v}
		}(k)
	}
	for range keys {
		select {
		case a := <-ch:
			switch a.key {
			case KeyEnabled:
				out.Enabled = a.val
			case KeyRepo:
				out.Repo = a.val
			case KeyDirGit:
				out.DirGit = a.val
			case KeyAI:
				out.AI = a.val
			case KeyTime:
				out.Time = a.val
			}
		case <-ctx.Done():
			return out
		}
	}
	return out
}

// GFFLookup resolves through the registered namespace first and, when that
// source is unknown on this host, through the checkout named by $DOTFILES_DIR
// (a path source). env is injected for tests; nil means os.Getenv.
func GFFLookup(env func(string) string) Lookup {
	if env == nil {
		env = os.Getenv
	}
	return func(key string) (bool, error) {
		v, err := gff.Bool(key, gff.WithSource(Namespace))
		if err == nil {
			return v, nil
		}
		if dir := env("DOTFILES_DIR"); dir != "" {
			return gff.Bool(key, gff.WithSource(dir))
		}
		return true, err
	}
}
