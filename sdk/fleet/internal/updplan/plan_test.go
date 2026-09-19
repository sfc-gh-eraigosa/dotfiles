package updplan

import (
	"reflect"
	"strings"
	"testing"
)

// TestDefaultPlanIsTodaysUpdate pins Default() to exactly today's
// `fleet update` behaviour: one repo (dotfiles, under ~/git, branch main,
// local skip, restore true) synced then installed via ./install.sh.
func TestDefaultPlanIsTodaysUpdate(t *testing.T) {
	p := Default()

	if p.Root != "~/git" {
		t.Errorf("Root = %q, want ~/git", p.Root)
	}
	if len(p.Repos) != 1 {
		t.Fatalf("len(Repos) = %d, want 1", len(p.Repos))
	}
	r, ok := p.Repos["dotfiles"]
	if !ok {
		t.Fatal("missing repo \"dotfiles\"")
	}
	if r.Name != "dotfiles" {
		t.Errorf("repo.Name = %q, want dotfiles", r.Name)
	}
	if r.Path != "~/git/dotfiles" {
		t.Errorf("repo.Path = %q, want ~/git/dotfiles", r.Path)
	}
	if len(r.Branches) != 1 || r.Branches[0] != "main" {
		t.Errorf("repo.Branches = %v, want [main]", r.Branches)
	}
	if r.Local != LocalSkip {
		t.Errorf("repo.Local = %q, want skip", r.Local)
	}
	if !r.Restore {
		t.Error("repo.Restore = false, want true")
	}

	if len(p.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(p.Steps))
	}

	sync := p.Steps[0]
	if sync.ID != "dotfiles.sync" {
		t.Errorf("Steps[0].ID = %q, want dotfiles.sync", sync.ID)
	}
	if sync.Kind != KindSync {
		t.Errorf("Steps[0].Kind = %q, want sync", sync.Kind)
	}
	if sync.Repo != "dotfiles" {
		t.Errorf("Steps[0].Repo = %q, want dotfiles", sync.Repo)
	}

	install := p.Steps[1]
	if install.ID != "dotfiles.install" {
		t.Errorf("Steps[1].ID = %q, want dotfiles.install", install.ID)
	}
	if install.Kind != KindRun {
		t.Errorf("Steps[1].Kind = %q, want run", install.Kind)
	}
	if install.Run != "./install.sh" {
		t.Errorf("Steps[1].Run = %q, want ./install.sh", install.Run)
	}
	if !install.Interactive {
		t.Error("Steps[1].Interactive = false, want true")
	}
	if len(install.Needs) != 1 || install.Needs[0] != "dotfiles.sync" {
		t.Errorf("Steps[1].Needs = %v, want [dotfiles.sync]", install.Needs)
	}
}

// TestDefaultYAMLRoundTripsToDefault pins the built-in starter YAML
// (`fleet update init` writes it) to parse to exactly Default(), modulo
// Source (which the caller, not Parse, sets on the built-in plan).
func TestDefaultYAMLRoundTripsToDefault(t *testing.T) {
	got, err := Parse([]byte(DefaultYAML))
	if err != nil {
		t.Fatalf("Parse(DefaultYAML) error: %v", err)
	}
	want := Default()

	got.Source = ""
	want.Source = ""

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(DefaultYAML) = %+v, want %+v", got, want)
	}
}

// --- baseline + stamp: the status column follows the PLAN's repo (#351 F2) --

func TestBaselineAndStampParse(t *testing.T) {
	p, err := Parse([]byte(`
version: 1
update:
  baseline: playground
  repos:
    dotfiles:
      path: ~/git/dotfiles
      stamp: ~/.local/state/dotfiles/install-stamp
    playground:
      path: ~/github/org/playground
      url: git@github.com:org/playground.git
      stamp: ~/.local/state/playground/install-stamp
  steps:
    - id: s
      kind: sync
      repo: playground
`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Baseline != "playground" {
		t.Fatalf("Baseline = %q, want playground", p.Baseline)
	}
	if got := p.Repos["playground"].Stamp; got != "~/.local/state/playground/install-stamp" {
		t.Fatalf("stamp = %q", got)
	}
	r, ok := p.BaselineRepo()
	if !ok || r.Name != "playground" {
		t.Fatalf("BaselineRepo() = %v %v, want playground", r.Name, ok)
	}
}

func TestBaselineMustNameADeclaredRepo(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
update:
  baseline: nosuch
  repos:
    dotfiles: {path: ~/git/dotfiles}
  steps:
    - {id: s, kind: sync, repo: dotfiles}
`))
	if err == nil {
		t.Fatal("a baseline naming an undeclared repo was accepted")
	}
	if !strings.Contains(err.Error(), "baseline") {
		t.Fatalf("error %q does not name the offending field", err)
	}
}

func TestBaselineRepoFallsBackPredictably(t *testing.T) {
	// No `baseline:` — a plan written before this existed, or a one-repo plan.
	one, err := Parse([]byte(`
version: 1
update:
  repos:
    solo: {path: ~/x, stamp: ~/.local/state/x/install-stamp}
  steps:
    - {id: s, kind: sync, repo: solo}
`))
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := one.BaselineRepo(); !ok || r.Name != "solo" {
		t.Fatalf("a single-repo plan should baseline on it, got %v %v", r.Name, ok)
	}

	// Several repos, one of them dotfiles: keep tracking dotfiles, which is
	// what every plan written before this feature meant.
	multi, err := Parse([]byte(`
version: 1
update:
  repos:
    dotfiles: {path: ~/git/dotfiles}
    other:    {path: ~/x}
  steps:
    - {id: s, kind: sync, repo: other}
`))
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := multi.BaselineRepo(); !ok || r.Name != "dotfiles" {
		t.Fatalf("a multi-repo plan with dotfiles should baseline on it, got %v %v", r.Name, ok)
	}

	// Several repos, no dotfiles, no baseline: refuse to guess.
	amb, err := Parse([]byte(`
version: 1
update:
  repos:
    a: {path: ~/a}
    b: {path: ~/b}
  steps:
    - {id: s, kind: sync, repo: a}
`))
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := amb.BaselineRepo(); ok {
		t.Fatalf("an ambiguous plan should have no baseline, got %v", r.Name)
	}
}

func TestBuiltInPlanDeclaresTheDotfilesStamp(t *testing.T) {
	// Today's output must not change: the built-in plan carries what used to
	// be the hardcoded consts, so status has one code path, not two.
	p := Default()
	r, ok := p.BaselineRepo()
	if !ok {
		t.Fatal("the built-in plan has no baseline repo")
	}
	if r.Stamp != "~/.local/state/dotfiles/install-stamp" {
		t.Fatalf("built-in stamp = %q", r.Stamp)
	}
	if r.Path != "~/git/dotfiles" {
		t.Fatalf("built-in baseline path = %q", r.Path)
	}
}
