// Package identity_test verifies the embedded suffix wordlist per
// sdk/gss/docs/plan.md PR-06: exactly 256 entries, each 3-5 lowercase
// ASCII letters, no duplicates. These are the per-word constraints the
// design enforces on both the built-in pool and any user-supplied words.
package identity_test

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
)

func TestWords_Count(t *testing.T) {
	if got := len(identity.Words()); got != 256 {
		t.Errorf("len(Words()) = %d; want exactly 256", got)
	}
}

func TestWords_LengthBounds(t *testing.T) {
	for _, w := range identity.Words() {
		if len(w) < 3 || len(w) > 5 {
			t.Errorf("word %q has length %d; want 3..5", w, len(w))
		}
	}
}

func TestWords_LowercaseASCIIOnly(t *testing.T) {
	for _, w := range identity.Words() {
		for i := 0; i < len(w); i++ {
			if c := w[i]; c < 'a' || c > 'z' {
				t.Errorf("word %q contains non-lowercase-ASCII byte %q", w, c)
				break
			}
		}
	}
}

func TestWords_NoDuplicates(t *testing.T) {
	seen := make(map[string]int, 256)
	for i, w := range identity.Words() {
		if first, dup := seen[w]; dup {
			t.Errorf("duplicate word %q at indices %d and %d", w, first, i)
		}
		seen[w] = i
	}
	if len(seen) != 256 {
		t.Errorf("unique words = %d; want 256", len(seen))
	}
}

// TestWords_ReturnsCopy — mutating the returned slice must not corrupt the
// shared pool seen by the next caller.
func TestWords_ReturnsCopy(t *testing.T) {
	a := identity.Words()
	if len(a) == 0 {
		t.Fatal("Words() empty")
	}
	orig := a[0]
	a[0] = "ZZZ"
	b := identity.Words()
	if b[0] != orig {
		t.Errorf("Words() returned a shared slice: mutation leaked (got %q, want %q)", b[0], orig)
	}
}
