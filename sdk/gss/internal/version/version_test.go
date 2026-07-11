// Package version_test verifies the build-metadata single source of truth
// per sdk/gss/docs/plan.md PR-04. TDD-first proof: this file fails to
// compile until version.go declares the four vars and Get().
//
// The exported vars are set by `build.sh` via -X ldflags; in any binary
// built without those flags (go test, go run, plain go install) they are
// the empty string. Get() layers display fallbacks on top so a command
// rendering version info shows "dev"/"none"/… rather than blanks.
package version_test

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/version"
)

// TestVarsEmptyByDefault — in this (unstamped) test binary the linker never
// set the vars, so they must be empty. This is the contract build.sh
// relies on: an unset field is "" and Get() supplies the fallback.
func TestVarsEmptyByDefault(t *testing.T) {
	if version.Version != "" || version.Commit != "" || version.BuildDate != "" || version.Dirty != "" {
		t.Errorf("expected all build vars empty in an unstamped binary; got Version=%q Commit=%q BuildDate=%q Dirty=%q",
			version.Version, version.Commit, version.BuildDate, version.Dirty)
	}
}

// TestGetDefaultsWhenUnset — with the vars empty, Get() applies the display
// fallbacks (preserving the historical cmd defaults).
func TestGetDefaultsWhenUnset(t *testing.T) {
	got := version.Get()
	want := version.Info{Version: "dev", Commit: "none", BuildDate: "unknown", Dirty: "false"}
	if got != want {
		t.Errorf("Get() with empty vars = %+v; want %+v", got, want)
	}
}

// TestGetReflectsStampedValues — when the linker has set the vars (simulated
// here by assigning them), Get() returns them verbatim, not the fallbacks.
func TestGetReflectsStampedValues(t *testing.T) {
	restore := stub(t)
	defer restore()

	version.Version = "1.4.0"
	version.Commit = "deadbee"
	version.BuildDate = "2026-05-21T00:00:00Z"
	version.Dirty = "true"

	got := version.Get()
	want := version.Info{Version: "1.4.0", Commit: "deadbee", BuildDate: "2026-05-21T00:00:00Z", Dirty: "true"}
	if got != want {
		t.Errorf("Get() with stamped vars = %+v; want %+v", got, want)
	}
}

// TestGetPartial — a partially-stamped binary (e.g. Version set, Commit not)
// fills only the empty fields from the fallback table.
func TestGetPartial(t *testing.T) {
	restore := stub(t)
	defer restore()

	version.Version = "2.0.0" // others left empty

	got := version.Get()
	want := version.Info{Version: "2.0.0", Commit: "none", BuildDate: "unknown", Dirty: "false"}
	if got != want {
		t.Errorf("Get() partial = %+v; want %+v", got, want)
	}
}

// stub snapshots the current build vars and returns a restorer, so a test
// that assigns them cannot leak into the empty-by-default expectations of
// the other tests.
func stub(t *testing.T) func() {
	t.Helper()
	v, c, d, dirty := version.Version, version.Commit, version.BuildDate, version.Dirty
	return func() {
		version.Version, version.Commit, version.BuildDate, version.Dirty = v, c, d, dirty
	}
}
