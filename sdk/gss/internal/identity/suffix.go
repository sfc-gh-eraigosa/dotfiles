package identity

import (
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
)

// MaxSuffixRetries caps how many suffix words AllocateRef draws before
// failing loudly. With a 256-word pool the collision probability after 5
// draws is negligible even for dozens of workers sharing a user/purpose
// (design.md → "Suffix wordlist": (k/256)^5).
const MaxSuffixRetries = 5

// ErrSuffixExhausted is returned when MaxSuffixRetries draws all collide.
// It's a package-local sentinel (not part of the central errors catalog)
// because it signals an internal retry budget, not a user-facing contract.
var ErrSuffixExhausted = stderrors.New("identity: no free suffix after max draws")

// WorkerRef is the canonical worker identifier used everywhere a worker is
// named — flags, JSON, env vars, PR cross-links, tmux pane metadata
// (design.md → "Worker reference"). Higher packages pass WorkerRef, never
// a bare string.
//
//	worker_ref ::= <feature> "/" <user> "/" <purpose> [ "-" <suffix> ]
type WorkerRef struct {
	Feature string
	User    string
	Purpose string
	Suffix  string // optional; "" means no suffix
}

// Leaf returns the "purpose[-suffix]" tail of the ref — the part that,
// together with Feature and User, identifies a worker. Suffix may be "".
func (r WorkerRef) Leaf() string {
	if r.Suffix == "" {
		return r.Purpose
	}
	return r.Purpose + "-" + r.Suffix
}

// SameWorker reports whether r and other identify the same worker: the
// same Feature, the same User, and the same reconstructed Leaf.
//
// This is the ONLY correct way to ask "is this ref the same worker as that
// one" (dotfiles#258, and the PR #333 review that found five more call
// sites making the same mistake). ParseWorkerRef's Purpose/Suffix split is
// lossy: a purpose whose own trailing token happens to be a
// suffix-wordlist word re-splits differently depending on how it was
// entered, so two WorkerRef values naming the identical worker can
// disagree field-by-field on Purpose/Suffix while still joining to the
// same Leaf string — concatenation makes the split point irrelevant to the
// reconstruction. Every comparison that means "is this worker X" must call
// SameWorker; comparing Purpose and Suffix directly reintroduces the bug.
func (r WorkerRef) SameWorker(other WorkerRef) bool {
	return r.Feature == other.Feature && r.User == other.User && r.Leaf() == other.Leaf()
}

// String renders the canonical worker_ref.
func (r WorkerRef) String() string {
	return r.Feature + "/" + r.User + "/" + r.Leaf()
}

// wordSet indexes the built-in pool for suffix detection in ParseWorkerRef.
var wordSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[w] = struct{}{}
	}
	return m
}()

func isWordlistWord(s string) bool {
	_, ok := wordSet[s]
	return ok
}

// ParseWorkerRef parses a worker_ref string into its segments. It is a
// structural parse (3 non-empty segments split on "/"); full grammar
// validation of each segment lands in PR-08. The trailing "-<word>" of the
// purpose segment is treated as a suffix only when <word> is a built-in
// wordlist word, since suffixes are always drawn from that list — a
// purpose may otherwise contain hyphens.
func ParseWorkerRef(s string) (WorkerRef, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return WorkerRef{}, fmt.Errorf("%w: worker ref %q must be feature/user/purpose[-suffix]", errors.ErrInvalidIdent, s)
	}
	purpose, suffix := splitPurposeSuffix(parts[2])
	return WorkerRef{Feature: parts[0], User: parts[1], Purpose: purpose, Suffix: suffix}, nil
}

// splitPurposeSuffix separates a "purpose[-suffix]" segment. The suffix is
// the token after the final "-" iff it is a built-in wordlist word.
func splitPurposeSuffix(seg string) (purpose, suffix string) {
	if i := strings.LastIndex(seg, "-"); i > 0 && i < len(seg)-1 {
		if cand := seg[i+1:]; isWordlistWord(cand) {
			return seg[:i], cand
		}
	}
	return seg, ""
}

// AllocateRef returns a non-colliding WorkerRef for base's
// feature/user/purpose. Any Suffix on the input base is ignored — suffixes
// are only ever drawn from the pool, never caller-supplied (design.md →
// "Uniqueness": the --suffix flag is boolean and never takes a value).
//
// When forceSuffix is false, the suffix-less ref is tried first and
// returned if free. Otherwise (or if it collides) AllocateRef draws up to
// MaxSuffixRetries suffixes, returning the first whose ref is free, or
// wrapping ErrSuffixExhausted if all collide.
//
// taken reports whether a candidate ref is already in use (registry, disk,
// origin); a nil taken means nothing is taken.
func AllocateRef(rng RNG, base WorkerRef, forceSuffix bool, taken func(WorkerRef) bool) (WorkerRef, error) {
	if taken == nil {
		taken = func(WorkerRef) bool { return false }
	}
	if !forceSuffix {
		cand := WorkerRef{Feature: base.Feature, User: base.User, Purpose: base.Purpose}
		if !taken(cand) {
			return cand, nil
		}
	}
	for i := 0; i < MaxSuffixRetries; i++ {
		cand := WorkerRef{Feature: base.Feature, User: base.User, Purpose: base.Purpose, Suffix: rng.DrawWord()}
		if !taken(cand) {
			return cand, nil
		}
	}
	return WorkerRef{}, fmt.Errorf("%s/%s/%s: %w (%d draws)", base.Feature, base.User, base.Purpose, ErrSuffixExhausted, MaxSuffixRetries)
}
