package featflag

import (
	"errors"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"
)

// GFF adapts the gff SDK to the featflag.Source interface. It is the only
// file in this module that imports sdk/gff/pkg/gff.
//
// Every lookup is scoped to the dotfiles checkout named by Repo (the CLI's
// --repo path) via gff.WithSource(path), which the SDK resolves to that
// checkout's LIVE feature file. That is what makes the flags independent of
// the process cwd and of any stale registered snapshot — review finding on
// PR #270: scoping by namespace read a snapshot nothing ever refreshed, and
// the unscoped fallback bound the live layer to wherever `fleet` was run.
// Only when the path is not a git repository (gff.ErrUnknownSource) does the
// adapter retry once unscoped (cwd discovery). Any other error is returned
// as-is — no retry.
//
// Strings returns the selected option IDs (gff.Selected), which is what
// `gff set <key> <id>` stores, so Resolve's "home"/"repo" switch and the
// features.yaml option ids are one contract rather than two that happen to
// coincide.
type GFF struct {
	// Repo is the local dotfiles checkout whose feature file is authoritative.
	Repo string
	// Opts are additional gff.Option values appended after the source scope
	// (e.g. gff.WithRoot in tests).
	Opts []gff.Option

	// boolFn/stringsFn are injectable for tests; nil means the real SDK.
	boolFn    func(key string, opts ...gff.Option) (bool, error)
	stringsFn func(key string, opts ...gff.Option) ([]string, error)
}

// ErrNoSource is returned by a nil *GFF: the "gff unavailable" case, which
// Resolve treats as fail-open exactly like any other error.
var ErrNoSource = errors.New("featflag: no gff source")

func (g *GFF) boolCall() func(key string, opts ...gff.Option) (bool, error) {
	if g.boolFn != nil {
		return g.boolFn
	}
	return gff.Bool
}

func (g *GFF) stringsCall() func(key string, opts ...gff.Option) ([]string, error) {
	if g.stringsFn != nil {
		return g.stringsFn
	}
	return gff.Selected
}

// scopedThenUnscoped runs call scoped to g.Repo and retries unscoped only
// when that path is not a usable source.
func scopedThenUnscoped[T any](g *GFF, key string, call func(string, ...gff.Option) (T, error)) (T, error) {
	scoped := append([]gff.Option{gff.WithSource(g.Repo)}, g.Opts...)
	v, err := call(key, scoped...)
	if err == nil || !errors.Is(err, gff.ErrUnknownSource) {
		return v, err
	}
	return call(key, g.Opts...)
}

// Bool implements Source. Safe on a nil receiver.
func (g *GFF) Bool(key string) (bool, error) {
	if g == nil {
		return false, ErrNoSource
	}
	return scopedThenUnscoped(g, key, g.boolCall())
}

// Strings implements Source: the selected option IDs. Safe on a nil receiver.
func (g *GFF) Strings(key string) ([]string, error) {
	if g == nil {
		return nil, ErrNoSource
	}
	return scopedThenUnscoped(g, key, g.stringsCall())
}
