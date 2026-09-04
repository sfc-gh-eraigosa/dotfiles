package featflag

import (
	"errors"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"
)

// GFF adapts the gff SDK to the featflag.Source interface. It is the only
// file in this module that imports sdk/gff/pkg/gff.
//
// Each accessor first resolves scoped to Namespace (via gff.WithSource) and,
// only when that scoped lookup fails with gff.ErrUnknownSource (this repo's
// namespace isn't registered with the local gff install), retries once
// without the source scope. Any other error is returned as-is - no retry.
type GFF struct {
	// Opts are additional gff.Option values appended after the source scope
	// (e.g. gff.WithRoot in tests).
	Opts []gff.Option

	// boolFn/stringsFn are injectable for tests; they default to the real SDK
	// calls in NewGFF/zero-value use via the accessor methods below.
	boolFn    func(key string, opts ...gff.Option) (bool, error)
	stringsFn func(key string, opts ...gff.Option) ([]string, error)
}

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
	return gff.StringValues
}

// Bool implements Source.
func (g *GFF) Bool(key string) (bool, error) {
	call := g.boolCall()

	scoped := append([]gff.Option{gff.WithSource(Namespace)}, g.Opts...)
	v, err := call(key, scoped...)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, gff.ErrUnknownSource) {
		return false, err
	}
	return call(key, g.Opts...)
}

// Strings implements Source.
func (g *GFF) Strings(key string) ([]string, error) {
	call := g.stringsCall()

	scoped := append([]gff.Option{gff.WithSource(Namespace)}, g.Opts...)
	v, err := call(key, scoped...)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, gff.ErrUnknownSource) {
		return nil, err
	}
	return call(key, g.Opts...)
}
