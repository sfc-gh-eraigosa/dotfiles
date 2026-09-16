package feature

// White-box test: findWorker is package-private, exercised directly here
// (rather than through a verb like Checkpoint/Done) to pin the fix for
// dotfiles#258 with no git/gh fakes needed.

import (
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/identity"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// TestFindWorker_PurposeEndingInWordlistWord reproduces the dotfiles#258
// repro exactly: a worker stored with a whole purpose ("apt-pin") that
// happens to end in a suffix-wordlist word must still resolve for the
// worker_ref its own `--json` output would print. Before the fix,
// ParseWorkerRef re-split "apt-pin" into purpose="apt" + suffix="pin" (since
// "pin" is a wordlist word), and findWorker's component-wise comparison
// against the stored (purpose="apt-pin", suffix="") row missed — the exact
// "no such worker" for a worker `list` shows.
func TestFindWorker_PurposeEndingInWordlistWord(t *testing.T) {
	reg := registry.Registry{Features: []registry.Feature{{
		Name: "gh-latest",
		Workers: []registry.Worker{
			{User: "edward-raigosa", Purpose: "apt-pin", Suffix: ""},
		},
	}}}
	ref, err := identity.ParseWorkerRef("gh-latest/edward-raigosa/apt-pin")
	if err != nil {
		t.Fatalf("ParseWorkerRef: %v", err)
	}
	fi, wi := findWorker(reg, ref)
	if fi < 0 || wi < 0 {
		t.Fatalf("findWorker(%+v) = (%d, %d); want a match — the worker_ref round-trips to a "+
			"different split than what's stored", ref, fi, wi)
	}
}

// TestFindWorker_StillDistinguishesRealSuffixes guards against a
// leaf-string-only fix being too loose: two workers with the same purpose
// but different (or no) suffix must remain distinguishable.
func TestFindWorker_StillDistinguishesRealSuffixes(t *testing.T) {
	reg := registry.Registry{Features: []registry.Feature{{
		Name: "auth",
		Workers: []registry.Worker{
			{User: "erai", Purpose: "refactor", Suffix: ""},
			{User: "erai", Purpose: "refactor", Suffix: "moss"},
		},
	}}}
	ref, err := identity.ParseWorkerRef("auth/erai/refactor-moss")
	if err != nil {
		t.Fatalf("ParseWorkerRef: %v", err)
	}
	fi, wi := findWorker(reg, ref)
	if fi < 0 || wi != 1 {
		t.Fatalf("findWorker(%+v) = (%d, %d); want the suffixed worker at index 1", ref, fi, wi)
	}
}

func TestFindWorker_NoMatch(t *testing.T) {
	reg := registry.Registry{Features: []registry.Feature{{
		Name:    "auth",
		Workers: []registry.Worker{{User: "erai", Purpose: "api"}},
	}}}
	ref, _ := identity.ParseWorkerRef("auth/erai/ghost")
	fi, wi := findWorker(reg, ref)
	if fi >= 0 || wi >= 0 {
		t.Fatalf("findWorker(%+v) = (%d, %d); want (-1, -1)", ref, fi, wi)
	}
}
