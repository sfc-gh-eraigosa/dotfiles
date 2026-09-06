package identity

import (
	"crypto/rand"
	"math/big"
)

// RNG draws a single word uniformly from a fixed pool. The pool is bound
// at construction so the suffix logic (AllocateRef) stays pool-agnostic.
// design.md → "Test seams": production wires SystemRNG (crypto/rand);
// tests inject a deterministic fake to pin the draw sequence.
type RNG interface {
	DrawWord() string
}

// SystemRNG is the production RNG: a uniform crypto/rand draw over its
// pool. Construct with NewSystemRNG over the effective suffix pool
// (built-in Words() plus any config append/replace words).
type SystemRNG struct {
	pool []string
}

// NewSystemRNG copies pool so later mutation of the caller's slice cannot
// change the draw set.
func NewSystemRNG(pool []string) *SystemRNG {
	return &SystemRNG{pool: append([]string(nil), pool...)}
}

// DrawWord returns a uniformly-random word from the pool, or "" if the
// pool is empty. A crypto/rand read failure (effectively never) degrades
// to the first word rather than panicking inside a library.
func (r *SystemRNG) DrawWord() string {
	if len(r.pool) == 0 {
		return ""
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(r.pool))))
	if err != nil {
		return r.pool[0]
	}
	return r.pool[n.Int64()]
}

var _ RNG = (*SystemRNG)(nil)
