package config_test

import (
	"testing"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/config"
)

// TestSystemClock_Now — the production clock returns a time at or after the
// moment just before the call (a sanity bound, not a precise assertion).
func TestSystemClock_Now(t *testing.T) {
	before := time.Now()
	got := config.SystemClock{}.Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("SystemClock.Now() = %v; want within [%v, %v]", got, before, after)
	}
}

// TestSystemClock_ImplementsClock — compile-time + value-level assertion
// that SystemClock satisfies the injectable Clock seam.
func TestSystemClock_ImplementsClock(t *testing.T) {
	var c config.Clock = config.SystemClock{}
	if c.Now().IsZero() {
		t.Error("SystemClock.Now() returned the zero time")
	}
}
