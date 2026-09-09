package updplan

import (
	"strings"
	"testing"
)

func TestWithRefTargetsDotfilesByName(t *testing.T) {
	yaml := `version: 1
update:
  repos:
    dotfiles: { path: dotfiles, branches: [main] }
    other: { path: other, branches: [main] }
  steps: []
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	out, err := p.WithRef("staging")
	if err != nil {
		t.Fatalf("WithRef() error: %v", err)
	}
	if got := out.Repos["dotfiles"].Branches[0]; got != "staging" {
		t.Errorf("dotfiles.Branches[0] = %q, want staging", got)
	}
	if got := out.Repos["other"].Branches[0]; got != "main" {
		t.Errorf("other.Branches[0] = %q, want main (untouched)", got)
	}
}

func TestWithRefTargetsTheSoleRepo(t *testing.T) {
	yaml := `version: 1
update:
  repos:
    work: { path: work, branches: [main] }
  steps: []
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	out, err := p.WithRef("staging")
	if err != nil {
		t.Fatalf("WithRef() error: %v", err)
	}
	if got := out.Repos["work"].Branches[0]; got != "staging" {
		t.Errorf("work.Branches[0] = %q, want staging", got)
	}
}

func TestWithRefIsAmbiguousWithManyRepos(t *testing.T) {
	yaml := `version: 1
update:
  repos:
    work: { path: work, branches: [main] }
    other: { path: other, branches: [main] }
  steps: []
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	_, err = p.WithRef("staging")
	if err == nil {
		t.Fatal("WithRef() = nil error, want ambiguous error")
	}
	for _, want := range []string{"work", "other", "repo=branch"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestWithRefRepoEqualsBranch(t *testing.T) {
	yaml := `version: 1
update:
  repos:
    work: { path: work, branches: [main] }
    other: { path: other, branches: [main] }
  steps: []
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	out, err := p.WithRef("work=staging")
	if err != nil {
		t.Fatalf("WithRef() error: %v", err)
	}
	if got := out.Repos["work"].Branches[0]; got != "staging" {
		t.Errorf("work.Branches[0] = %q, want staging", got)
	}
	if got := out.Repos["other"].Branches[0]; got != "main" {
		t.Errorf("other.Branches[0] = %q, want main (untouched)", got)
	}
}

func TestWithRefRejectsShellInjection(t *testing.T) {
	yaml := `version: 1
update:
  repos:
    dotfiles: { path: dotfiles, branches: [main] }
  steps: []
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	for _, good := range []string{"main", "feature/fleet/build", "v0.1.0", "release-1.2"} {
		if _, err := p.WithRef(good); err != nil {
			t.Errorf("WithRef(%q) error: %v, want nil", good, err)
		}
	}
	for _, bad := range []string{"", "main; rm -rf ~", "main && echo pwned", "$(whoami)", "a b", "main`id`"} {
		if _, err := p.WithRef(bad); err == nil {
			t.Errorf("WithRef(%q) = nil error, want rejection", bad)
		}
	}
}

func TestWithRefDropsADuplicateExtra(t *testing.T) {
	yaml := `version: 1
update:
  repos:
    dotfiles: { path: dotfiles, branches: [main, staging] }
  steps: []
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	out, err := p.WithRef("staging")
	if err != nil {
		t.Fatalf("WithRef() error: %v", err)
	}
	got := out.Repos["dotfiles"].Branches
	want := []string{"staging"}
	if len(got) != len(want) {
		t.Fatalf("Branches = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Branches = %v, want %v", got, want)
		}
	}
}

func TestRepoPathResolvesUnderRoot(t *testing.T) {
	yaml := `version: 1
update:
  root: ~/git
  repos:
    dotfiles: { branches: [main] }
    scripts: { path: work/scripts, branches: [main] }
    srv: { path: /srv/x, branches: [main] }
    home: { path: "~/x", branches: [main] }
  steps: []
`
	p, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	cases := map[string]string{
		"dotfiles": "~/git/dotfiles",
		"scripts":  "~/git/work/scripts",
		"srv":      "/srv/x",
		"home":     "~/x",
	}
	for name, want := range cases {
		got := p.Repos[name].Path
		if got != want {
			t.Errorf("Repos[%q].Path = %q, want %q", name, got, want)
		}
	}
}
