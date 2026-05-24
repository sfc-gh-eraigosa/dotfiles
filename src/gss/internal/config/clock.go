package config

import "time"

// Clock abstracts "what time is it now" so callers can inject a
// deterministic clock in tests. The Now() signature is pinned by
// design.md → "Test seams"; production wires SystemClock, tests wire a
// fake. It lives in internal/config because config is the first package to
// need an injectable clock (first-run stub stamping), and the higher
// internal/feature/* orchestrators take a Clock as a constructor parameter.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock, backed by time.Now.
type SystemClock struct{}

// Now returns the current local time.
func (SystemClock) Now() time.Time { return time.Now() }

// Compile-time assertion.
var _ Clock = SystemClock{}
