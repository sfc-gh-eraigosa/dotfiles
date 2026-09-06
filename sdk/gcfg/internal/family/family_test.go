package family

import (
	"context"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"gopkg.in/yaml.v3"
)

// stub is a minimal Family so the registry can be tested on its own.
type stub struct {
	name  string
	scope Scope
}

func (s stub) Name() string                                                  { return s.name }
func (s stub) Scope() Scope                                                  { return s.scope }
func (s stub) Permission() string                                            { return "repo:Administration:read" }
func (s stub) Read(context.Context, gh.Client, Target) (Live, error)         { return nil, nil }
func (s stub) Export(Live) (*yaml.Node, error)                               { return nil, nil }
func (s stub) Diff(*yaml.Node, Live, schema.Ownership) ([]Finding, []Change) { return nil, nil }
func (s stub) Apply(context.Context, gh.Client, Target, []Change) error      { return nil }

func TestRegistryKeepsScopesApartAndStaysOrdered(t *testing.T) {
	r := NewRegistry()
	r.Register(stub{"security", ScopeRepo})
	r.Register(stub{"general", ScopeRepo})
	r.Register(stub{"profile", ScopeOrg})

	repo := names(r.All(ScopeRepo))
	if strings.Join(repo, ",") != "general,security" {
		t.Fatalf("repo families = %v (registration order must not leak; they are sorted)", repo)
	}
	if org := names(r.All(ScopeOrg)); strings.Join(org, ",") != "profile" {
		t.Fatalf("org families = %v", org)
	}
	f, ok := r.Lookup(ScopeRepo, "general")
	if !ok || f.Name() != "general" {
		t.Fatalf("lookup = %v %v", f, ok)
	}
	if _, ok := r.Lookup(ScopeOrg, "general"); ok {
		t.Fatal("a repo family must not answer an org lookup")
	}
	if _, ok := r.Lookup(ScopeRepo, "nope"); ok {
		t.Fatal("unknown family must not resolve")
	}
}

func TestRegistryRejectsADuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(stub{"general", ScopeRepo})
	defer func() {
		if recover() == nil {
			t.Fatal("registering the same family twice must panic — it is a build-time mistake")
		}
	}()
	r.Register(stub{"general", ScopeRepo})
}

// --only selects a subset, and names nothing it cannot find.
func TestSelect(t *testing.T) {
	r := NewRegistry()
	r.Register(stub{"general", ScopeRepo})
	r.Register(stub{"security", ScopeRepo})

	got, err := r.Select(ScopeRepo, []string{"security"})
	if err != nil || len(got) != 1 || got[0].Name() != "security" {
		t.Fatalf("got=%v err=%v", names(got), err)
	}
	if got, err := r.Select(ScopeRepo, nil); err != nil || len(got) != 2 {
		t.Fatalf("empty selection means everything: %v %v", names(got), err)
	}
	_, err = r.Select(ScopeRepo, []string{"security", "nope"})
	if err == nil || !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "general, security") {
		t.Fatalf("want an error naming the bad one and listing the real ones, got %v", err)
	}
}

func TestTargetString(t *testing.T) {
	if got := (Target{Owner: "o", Repo: "r"}).String(); got != "o/r" {
		t.Errorf("repo target = %q", got)
	}
	if got := (Target{Owner: "o"}).String(); got != "o" {
		t.Errorf("org target = %q", got)
	}
	if !(Target{Owner: "o"}).IsOrg() || (Target{Owner: "o", Repo: "r"}).IsOrg() {
		t.Error("IsOrg is wrong")
	}
}

func TestFindingAndChangeRender(t *testing.T) {
	f := Finding{Family: "general", Key: "merge.squash", Kind: Drift, Want: true, Live: false}
	s := f.String()
	if !strings.Contains(s, "general") || !strings.Contains(s, "merge.squash") || !strings.Contains(s, "drift") {
		t.Errorf("Finding.String() = %q", s)
	}
	for kind, want := range map[Kind]string{Drift: "drift", Unmanaged: "unmanaged", Unreadable: "unreadable", NotHonoured: "not_honoured"} {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d) = %q, want %q", kind, got, want)
		}
	}
	c := Change{Family: "general", Key: "merge.squash", Op: OpUpdate, Want: true, Live: false}
	if !strings.Contains(c.String(), "update") || !strings.Contains(c.String(), "merge.squash") {
		t.Errorf("Change.String() = %q", c.String())
	}
	for op, want := range map[Op]string{OpUpdate: "update", OpCreate: "create", OpDelete: "delete"} {
		if got := op.String(); got != want {
			t.Errorf("Op(%d) = %q, want %q", op, got, want)
		}
	}
}

func names(fs []Family) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Name())
	}
	return out
}
