package featflag

import (
	"errors"
	"fmt"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"
)

func TestGFFFallsBackToUnscopedOnUnknownSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	g := &GFF{}

	calls := 0
	g.boolFn = func(key string, opts ...gff.Option) (bool, error) {
		calls++
		if calls == 1 {
			return false, fmt.Errorf("scoped: %w", gff.ErrUnknownSource)
		}
		if calls != 2 {
			t.Fatalf("boolFn called %d times, want at most 2", calls)
		}
		return true, nil
	}

	got, err := g.Bool(KeyEnabled)

	if err != nil {
		t.Fatalf("Bool: unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("Bool = false, want true from the unscoped fallback")
	}
	if calls != 2 {
		t.Fatalf("boolFn called %d times, want exactly 2 (scoped then unscoped)", calls)
	}
}

func TestGFFDoesNotRetryOtherErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	g := &GFF{}
	wantErr := errors.New("some other failure")

	calls := 0
	g.boolFn = func(key string, opts ...gff.Option) (bool, error) {
		calls++
		return false, wantErr
	}

	_, err := g.Bool(KeyEnabled)

	if !errors.Is(err, wantErr) {
		t.Fatalf("Bool err = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("boolFn called %d times, want exactly 1 (no retry)", calls)
	}
}

func TestGFFStringsFallsBackToUnscopedOnUnknownSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	g := &GFF{}

	calls := 0
	g.stringsFn = func(key string, opts ...gff.Option) ([]string, error) {
		calls++
		if calls == 1 {
			return nil, fmt.Errorf("scoped: %w", gff.ErrUnknownSource)
		}
		if calls != 2 {
			t.Fatalf("stringsFn called %d times, want at most 2", calls)
		}
		return []string{"home"}, nil
	}

	got, err := g.Strings(KeyConfig)

	if err != nil {
		t.Fatalf("Strings: unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "home" {
		t.Fatalf("Strings = %v, want [home]", got)
	}
	if calls != 2 {
		t.Fatalf("stringsFn called %d times, want exactly 2", calls)
	}
}

func TestGFFStringsDoesNotRetryOtherErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	g := &GFF{}
	wantErr := errors.New("some other failure")

	calls := 0
	g.stringsFn = func(key string, opts ...gff.Option) ([]string, error) {
		calls++
		return nil, wantErr
	}

	_, err := g.Strings(KeyConfig)

	if !errors.Is(err, wantErr) {
		t.Fatalf("Strings err = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("stringsFn called %d times, want exactly 1 (no retry)", calls)
	}
}

func TestGFFUsesTheRealSDKByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	g := &GFF{Opts: []gff.Option{gff.WithRoot(t.TempDir())}}

	// No boolFn/stringsFn injected: this exercises the real gff.Bool/StringValues
	// call paths. The key is unregistered in this scratch root, so we only
	// assert an error comes back (not what shape) - the point is coverage of
	// the un-injected default, not resolver behaviour (covered in sdk/gff).
	if _, err := g.Bool(KeyEnabled); err == nil {
		t.Fatalf("Bool: want an error resolving an unregistered key against a scratch root")
	}
	if _, err := g.Strings(KeyConfig); err == nil {
		t.Fatalf("Strings: want an error resolving an unregistered key against a scratch root")
	}
}

func TestResolveWithGFFUnknownKeyIsFailOpen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := Static{Err: fmt.Errorf("x: %w", gff.ErrUnknownKey)}

	got := Resolve(src, t.TempDir(), t.TempDir())

	if !got.Enabled {
		t.Fatalf("Enabled = false, want true (fail open on gff.ErrUnknownKey)")
	}
}
