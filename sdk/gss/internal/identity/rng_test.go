package identity_test

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
)

func TestSystemRNG_DrawsFromPool(t *testing.T) {
	pool := []string{"moss", "fern", "kelp"}
	rng := identity.NewSystemRNG(pool)
	inPool := map[string]bool{"moss": true, "fern": true, "kelp": true}
	for i := 0; i < 100; i++ {
		w := rng.DrawWord()
		if !inPool[w] {
			t.Fatalf("DrawWord() = %q; not in pool %v", w, pool)
		}
	}
}

func TestSystemRNG_EmptyPool(t *testing.T) {
	if got := identity.NewSystemRNG(nil).DrawWord(); got != "" {
		t.Errorf("empty-pool DrawWord() = %q; want \"\"", got)
	}
}

func TestSystemRNG_CopiesPool(t *testing.T) {
	pool := []string{"moss", "fern"}
	rng := identity.NewSystemRNG(pool)
	pool[0] = "MUTATED"
	for i := 0; i < 20; i++ {
		if rng.DrawWord() == "MUTATED" {
			t.Fatal("SystemRNG drew a value mutated in the caller's slice; pool not copied")
		}
	}
}

// TestSystemRNG_OverPool draws many times over the real 256-word pool and
// confirms it only ever returns pool members and exercises more than one.
func TestSystemRNG_OverWordlist(t *testing.T) {
	rng := identity.NewSystemRNG(identity.Words())
	seen := map[string]bool{}
	valid := map[string]bool{}
	for _, w := range identity.Words() {
		valid[w] = true
	}
	for i := 0; i < 500; i++ {
		w := rng.DrawWord()
		if !valid[w] {
			t.Fatalf("drew %q not in the wordlist", w)
		}
		seen[w] = true
	}
	if len(seen) < 2 {
		t.Errorf("500 draws produced only %d distinct words; expected variety", len(seen))
	}
}
