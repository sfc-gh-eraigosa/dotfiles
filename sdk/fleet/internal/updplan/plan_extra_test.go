package updplan

import (
	"testing"
	"time"
)

// TestRepoOfReturnsTheSteppedRepo and TestRepoOfNoRepoIsFalse close the
// coverage on Plan.RepoOf, exercised by the executor (leaf B) but pure
// enough to pin here.
func TestRepoOfReturnsTheSteppedRepo(t *testing.T) {
	p, err := Parse([]byte(validBase))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	st, ok := p.Step("dotfiles.sync")
	if !ok {
		t.Fatal("missing step dotfiles.sync")
	}
	r, ok := p.RepoOf(st)
	if !ok {
		t.Fatal("RepoOf() not found")
	}
	if r.Name != "dotfiles" {
		t.Errorf("RepoOf().Name = %q, want dotfiles", r.Name)
	}
}

func TestRepoOfNoRepoIsFalse(t *testing.T) {
	p, err := Parse([]byte(`version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: gh-auth
`))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	st, _ := p.Step("a")
	if _, ok := p.RepoOf(st); ok {
		t.Error("RepoOf() = true for a gh-auth step, want false")
	}
}

func TestStepUnknownIDIsFalse(t *testing.T) {
	p := Default()
	if _, ok := p.Step("nope"); ok {
		t.Error("Step(nope) = true, want false")
	}
}

func TestWithRefsAppliesEachSpecInTurn(t *testing.T) {
	p, err := Parse([]byte(`version: 1
update:
  repos:
    dotfiles: { path: dotfiles, branches: [main] }
    other: { path: other, branches: [main] }
  steps: []
`))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	out, err := p.WithRefs([]string{"dotfiles=staging", "other=release"})
	if err != nil {
		t.Fatalf("WithRefs() error: %v", err)
	}
	if got := out.Repos["dotfiles"].Branches[0]; got != "staging" {
		t.Errorf("dotfiles.Branches[0] = %q, want staging", got)
	}
	if got := out.Repos["other"].Branches[0]; got != "release" {
		t.Errorf("other.Branches[0] = %q, want release", got)
	}
}

func TestWithRefsPropagatesAnError(t *testing.T) {
	p := Default()
	if _, err := p.WithRefs([]string{"main", "bad ref"}); err == nil {
		t.Fatal("WithRefs() = nil error, want propagated rejection")
	}
}

func TestValidHostname(t *testing.T) {
	for _, good := range []string{"github.com", "gh.example-corp.internal"} {
		if !ValidHostname(good) {
			t.Errorf("ValidHostname(%q) = false, want true", good)
		}
	}
	for _, bad := range []string{"", "gh$(id)", "gh; id", "gh host"} {
		if ValidHostname(bad) {
			t.Errorf("ValidHostname(%q) = true, want false", bad)
		}
	}
}

func TestValidSHA(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"[:40]
	if !ValidSHA(sha) {
		t.Errorf("ValidSHA(%q) = false, want true", sha)
	}
	for _, bad := range []string{"", "abc", sha[:39], sha + "0", "g" + sha[1:]} {
		if ValidSHA(bad) {
			t.Errorf("ValidSHA(%q) = true, want false", bad)
		}
	}
}

func TestParseBackoffOverridesEachFieldIndependently(t *testing.T) {
	yaml := `version: 1
update:
  repos: {}
  steps:
    - id: a
      kind: run
      run: echo hi
      retry: { backoff: { initial: 1s, max: 1h, factor: 3, jitter: false } }
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	st, _ := p.Step("a")
	want := Backoff{Initial: time.Second, Max: time.Hour, Factor: 3, Jitter: false}
	if st.Retry.Backoff != want {
		t.Errorf("Retry.Backoff = %+v, want %+v", st.Retry.Backoff, want)
	}
}

func TestMustParsePanicsOnInvalidYAML(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("mustParse() did not panic on invalid input")
		}
	}()
	mustParse("version: 2\nupdate: {}\n")
}
