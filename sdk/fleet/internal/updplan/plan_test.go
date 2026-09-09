package updplan

import (
	"reflect"
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
