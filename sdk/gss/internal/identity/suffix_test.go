// Package identity_test verifies the worker_ref type and suffix-draw logic
// per sdk/gss/docs/plan.md PR-07: RNG injection pins the draw sequence;
// caller-supplied suffixes are rejected (only drawn words are used);
// collision retry caps at 5; worker_ref formatter/parser round-trips.
package identity_test

import (
	stderrors "errors"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
)

// scriptRNG returns words in a fixed order so tests pin the draw sequence.
// It records how many times it was called.
type scriptRNG struct {
	words []string
	calls int
}

func (s *scriptRNG) DrawWord() string {
	w := s.words[s.calls%len(s.words)]
	s.calls++
	return w
}

func TestWorkerRef_String(t *testing.T) {
	if got := (identity.WorkerRef{Feature: "login", User: "erai", Purpose: "refactor"}).String(); got != "login/erai/refactor" {
		t.Errorf("no-suffix String() = %q", got)
	}
	if got := (identity.WorkerRef{Feature: "login", User: "erai", Purpose: "refactor", Suffix: "moss"}).String(); got != "login/erai/refactor-moss" {
		t.Errorf("suffix String() = %q", got)
	}
}

func TestParseWorkerRef_RoundTrip(t *testing.T) {
	for _, s := range []string{"login/erai/refactor", "login/erai/refactor-moss"} {
		ref, err := identity.ParseWorkerRef(s)
		if err != nil {
			t.Fatalf("ParseWorkerRef(%q): %v", s, err)
		}
		if ref.String() != s {
			t.Errorf("round-trip: %q -> %+v -> %q", s, ref, ref.String())
		}
	}
	// Field-level: the suffix must be split out from the purpose.
	ref, _ := identity.ParseWorkerRef("login/erai/refactor-moss")
	if ref.Purpose != "refactor" || ref.Suffix != "moss" {
		t.Errorf("split wrong: purpose=%q suffix=%q; want refactor/moss", ref.Purpose, ref.Suffix)
	}
	// A trailing token NOT in the wordlist stays part of the purpose.
	ref2, _ := identity.ParseWorkerRef("login/erai/multi-stage")
	if ref2.Purpose != "multi-stage" || ref2.Suffix != "" {
		t.Errorf("non-wordlist trailing token mis-split: purpose=%q suffix=%q", ref2.Purpose, ref2.Suffix)
	}
}

func TestParseWorkerRef_Invalid(t *testing.T) {
	for _, s := range []string{"", "a/b", "a/b/c/d", "a//c", "/b/c", "a/b/"} {
		_, err := identity.ParseWorkerRef(s)
		if err == nil {
			t.Errorf("ParseWorkerRef(%q): err = nil; want invalid-ident error", s)
			continue
		}
		if !stderrors.Is(err, errors.ErrInvalidIdent) {
			t.Errorf("ParseWorkerRef(%q): err = %v; want wrapping ErrInvalidIdent", s, err)
		}
	}
}

func TestAllocateRef_NoSuffixWhenFree(t *testing.T) {
	rng := &scriptRNG{words: []string{"moss"}}
	base := identity.WorkerRef{Feature: "login", User: "erai", Purpose: "refactor"}
	ref, err := identity.AllocateRef(rng, base, false, func(identity.WorkerRef) bool { return false })
	if err != nil {
		t.Fatalf("AllocateRef: %v", err)
	}
	if ref.Suffix != "" {
		t.Errorf("free ref should have no suffix; got %q", ref.Suffix)
	}
	if rng.calls != 0 {
		t.Errorf("rng should not be drawn when the suffix-less ref is free; calls=%d", rng.calls)
	}
}

func TestAllocateRef_ForceSuffixDrawsEvenWhenFree(t *testing.T) {
	rng := &scriptRNG{words: []string{"moss"}}
	base := identity.WorkerRef{Feature: "login", User: "erai", Purpose: "refactor"}
	ref, err := identity.AllocateRef(rng, base, true, func(identity.WorkerRef) bool { return false })
	if err != nil {
		t.Fatalf("AllocateRef: %v", err)
	}
	if ref.Suffix != "moss" {
		t.Errorf("forceSuffix should draw a suffix; got %q", ref.Suffix)
	}
}

func TestAllocateRef_IgnoresCallerSuppliedSuffix(t *testing.T) {
	rng := &scriptRNG{words: []string{"moss"}}
	// base carries a suffix, simulating a caller trying to supply one.
	base := identity.WorkerRef{Feature: "login", User: "erai", Purpose: "refactor", Suffix: "evil"}
	ref, err := identity.AllocateRef(rng, base, false, func(identity.WorkerRef) bool { return false })
	if err != nil {
		t.Fatalf("AllocateRef: %v", err)
	}
	if ref.Suffix == "evil" {
		t.Error("caller-supplied suffix must be ignored, never used")
	}
}

func TestAllocateRef_CollisionRetry(t *testing.T) {
	rng := &scriptRNG{words: []string{"moss", "fern", "kelp"}}
	base := identity.WorkerRef{Feature: "login", User: "erai", Purpose: "refactor"}
	// The suffix-less ref and the first two drawn suffixes collide; "kelp" is free.
	taken := func(r identity.WorkerRef) bool {
		switch r.Suffix {
		case "", "moss", "fern":
			return true
		default:
			return false
		}
	}
	ref, err := identity.AllocateRef(rng, base, false, taken)
	if err != nil {
		t.Fatalf("AllocateRef: %v", err)
	}
	if ref.Suffix != "kelp" {
		t.Errorf("expected first free suffix kelp; got %q", ref.Suffix)
	}
	if rng.calls != 3 {
		t.Errorf("expected 3 draws (moss, fern, kelp); got %d", rng.calls)
	}
}

func TestAllocateRef_Exhausted(t *testing.T) {
	rng := &scriptRNG{words: []string{"moss", "fern", "kelp", "sage", "pine", "dune"}}
	base := identity.WorkerRef{Feature: "login", User: "erai", Purpose: "refactor"}
	_, err := identity.AllocateRef(rng, base, false, func(identity.WorkerRef) bool { return true })
	if err == nil {
		t.Fatal("all-taken: err = nil; want ErrSuffixExhausted")
	}
	if !stderrors.Is(err, identity.ErrSuffixExhausted) {
		t.Errorf("err = %v; want wrapping ErrSuffixExhausted", err)
	}
	if rng.calls != identity.MaxSuffixRetries {
		t.Errorf("draws = %d; want exactly MaxSuffixRetries=%d", rng.calls, identity.MaxSuffixRetries)
	}
}
