package repo

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

// testdataPath returns the absolute path to testdata/<name>.
func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	return filepath.Join(dir, "testdata", name)
}

// ---------------------------------------------------------------------------
// LoadRegistry tests
// ---------------------------------------------------------------------------

func TestLoadRegistry_Success(t *testing.T) {
	reg, err := LoadRegistry(testdataPath("registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry: unexpected error: %v", err)
	}
	if reg == nil {
		t.Fatal("LoadRegistry: expected non-nil Registry")
	}
	if len(reg.Features) != 2 {
		t.Errorf("Features: want 2, got %d", len(reg.Features))
	}
}

func TestLoadRegistry_NotFound(t *testing.T) {
	_, err := LoadRegistry("/nonexistent/path/to/registry.json")
	if !errors.Is(err, ErrRegistryNotFound) {
		t.Errorf("want ErrRegistryNotFound, got %v", err)
	}
}

func TestLoadRegistry_UnsupportedSchema(t *testing.T) {
	_, err := LoadRegistry(testdataPath("registry_v2.json"))
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Errorf("want ErrUnsupportedSchema, got %v", err)
	}
}

func TestLoadRegistry_MalformedJSON(t *testing.T) {
	reg, err := LoadRegistry(testdataPath("registry_malformed.json"))
	if err == nil {
		t.Errorf("want error for malformed JSON, got nil (reg=%v)", reg)
	}
}

// ---------------------------------------------------------------------------
// parsePRNumber tests
// ---------------------------------------------------------------------------

func TestParsePRNumber(t *testing.T) {
	cases := []struct {
		url  string
		want int
	}{
		{"https://github.com/owner/repo/pull/21", 21},
		{"https://github.com/owner/repo/pull/42", 42},
		{"https://github.com/owner/repo/pull/1", 1},
		{"https://github.com/owner/repo/pull/21/", 21}, // trailing slash
		{"", 0},
		{"not-a-url", 0},
		{"https://github.com/owner/repo/pull/", 0},
		{"https://github.com/owner/repo/pull/abc", 0},
	}
	for _, tc := range cases {
		got := parsePRNumber(tc.url)
		if got != tc.want {
			t.Errorf("parsePRNumber(%q): want %d, got %d", tc.url, tc.want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Match tests
// ---------------------------------------------------------------------------

func loadTestRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := LoadRegistry(testdataPath("registry.json"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return reg
}

func TestMatch_ByToplevel_WithPR(t *testing.T) {
	reg := loadTestRegistry(t)
	toplevel := "/home/wenlock/.config/gss/worktrees/sfc-gh-eraigosa/dotfiles/gsl/edward-raigosa/impl"
	m, ok := Match(reg, toplevel, "some-other-branch")
	if !ok {
		t.Fatal("Match: expected a match by toplevel")
	}
	if m.Feature != "gsl" {
		t.Errorf("Feature: want gsl, got %q", m.Feature)
	}
	if !m.HasPR {
		t.Error("HasPR: want true")
	}
	if m.PRNumber != 21 {
		t.Errorf("PRNumber: want 21, got %d", m.PRNumber)
	}
	if m.PRState != "OPEN" {
		t.Errorf("PRState: want OPEN, got %q", m.PRState)
	}
}

func TestMatch_ByBranch_WithPR(t *testing.T) {
	reg := loadTestRegistry(t)
	// Use a different toplevel so it doesn't match by path; use the real branch
	m, ok := Match(reg, "/no/such/path", "feature/gsl/edward-raigosa/impl")
	if !ok {
		t.Fatal("Match: expected a match by branch")
	}
	if m.Feature != "gsl" {
		t.Errorf("Feature: want gsl, got %q", m.Feature)
	}
	if !m.HasPR {
		t.Error("HasPR: want true")
	}
	if m.PRNumber != 21 {
		t.Errorf("PRNumber: want 21, got %d", m.PRNumber)
	}
}

func TestMatch_ByBranch_NoPR(t *testing.T) {
	reg := loadTestRegistry(t)
	// The second worker ("another-user/review") has no pr_url.
	m, ok := Match(reg, "/no/such/path", "feature/gsl/another-user/review")
	if !ok {
		t.Fatal("Match: expected a match by branch")
	}
	if m.Feature != "gsl" {
		t.Errorf("Feature: want gsl, got %q", m.Feature)
	}
	if m.HasPR {
		t.Error("HasPR: want false (no pr_url in fixture)")
	}
	if m.PRNumber != 0 {
		t.Errorf("PRNumber: want 0, got %d", m.PRNumber)
	}
}

func TestMatch_ToplevelPrecedenceOverBranch(t *testing.T) {
	// Register a registry where a branch matches one feature but the toplevel
	// matches a different worker.  Toplevel should win.
	reg := &Registry{
		Features: []Feature{
			{
				Name: "alpha",
				Workers: []Worker{
					{Branch: "feat/alpha", Worktree: "/path/alpha", PRUrl: "https://github.com/o/r/pull/10", PRState: "OPEN"},
				},
			},
			{
				Name: "beta",
				Workers: []Worker{
					{Branch: "feat/alpha", Worktree: "/path/beta", PRUrl: "https://github.com/o/r/pull/20", PRState: "DRAFT"},
				},
			},
		},
	}
	// toplevel matches "alpha", branch also matches "beta" (second feature) but
	// would also match "alpha". Toplevel scan is first, so "alpha" wins.
	m, ok := Match(reg, "/path/alpha", "feat/alpha")
	if !ok {
		t.Fatal("expected a match")
	}
	if m.Feature != "alpha" {
		t.Errorf("Feature: want alpha (toplevel wins), got %q", m.Feature)
	}
	if m.PRNumber != 10 {
		t.Errorf("PRNumber: want 10, got %d", m.PRNumber)
	}
}

func TestMatch_NoMatch(t *testing.T) {
	reg := loadTestRegistry(t)
	_, ok := Match(reg, "/no/such/path", "no-such-branch")
	if ok {
		t.Error("Match: expected no match, got one")
	}
}

func TestMatch_NilRegistry(t *testing.T) {
	_, ok := Match(nil, "/any", "any")
	if ok {
		t.Error("Match(nil): expected false")
	}
}

func TestMatch_OtherFeaturePR42(t *testing.T) {
	reg := loadTestRegistry(t)
	m, ok := Match(reg, "/home/bob/.config/gss/worktrees/sfc-gh-bob/dotfiles/other-feature/bob/impl", "")
	if !ok {
		t.Fatal("expected match for other-feature worker")
	}
	if m.Feature != "other-feature" {
		t.Errorf("Feature: want other-feature, got %q", m.Feature)
	}
	if m.PRNumber != 42 {
		t.Errorf("PRNumber: want 42, got %d", m.PRNumber)
	}
	if m.PRState != "MERGED" {
		t.Errorf("PRState: want MERGED, got %q", m.PRState)
	}
}
