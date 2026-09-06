// Package family is the unit gcfg manages: one coherent group of GitHub
// settings that can be read, exported, diffed, and applied on its own
// (plan §3.2). One family breaking never stops the rest — a family that
// cannot be read becomes a finding, not an error.
package family

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"gopkg.in/yaml.v3"
)

// Scope says whether a family lives on a repository or an organization.
type Scope int

// The two scopes.
const (
	ScopeRepo Scope = iota
	ScopeOrg
)

func (s Scope) String() string {
	if s == ScopeOrg {
		return "org"
	}
	return "repo"
}

// Target is what a run acts on. An empty Repo means the organization.
type Target struct {
	Owner string
	Repo  string
}

func (t Target) String() string {
	if t.Repo == "" {
		return t.Owner
	}
	return t.Owner + "/" + t.Repo
}

// IsOrg is true for an organization target.
func (t Target) IsOrg() bool { return t.Repo == "" }

// Live is whatever a family read from GitHub. Only that family interprets it.
type Live any

// Kind classifies a finding.
type Kind int

// The four things verify can say about a setting.
const (
	// Drift: declared and live disagree.
	Drift Kind = iota
	// Unmanaged: live has something the file does not declare (reported
	// under `declared`, removed under `full`).
	Unmanaged
	// Unreadable: the credential cannot read this family. Reported, never
	// fatal, unless --strict.
	Unreadable
	// NotHonoured: a write GitHub accepted did not change anything — the
	// plan or product does not support it.
	NotHonoured
)

func (k Kind) String() string {
	switch k {
	case Unmanaged:
		return "unmanaged"
	case Unreadable:
		return "unreadable"
	case NotHonoured:
		return "not_honoured"
	default:
		return "drift"
	}
}

// Finding is one thing verify wants a human to know.
type Finding struct {
	Family string `json:"family"`
	Key    string `json:"key"`
	Op     string `json:"op,omitempty"`
	Kind   Kind   `json:"-"`
	Want   any    `json:"want,omitempty"`
	Live   any    `json:"live,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// MarshalJSON renders Kind as its name.
func (f Finding) String() string {
	s := fmt.Sprintf("%s.%s: %s", f.Family, f.Key, f.Kind)
	switch f.Kind {
	case Drift, NotHonoured:
		s += fmt.Sprintf(" (want %v, live %v)", f.Want, f.Live)
	case Unmanaged:
		s += fmt.Sprintf(" (live %v, not declared)", f.Live)
	}
	if f.Reason != "" {
		s += ": " + f.Reason
	}
	return s
}

// Op is what a change does.
type Op int

// The three operations apply can perform.
const (
	OpUpdate Op = iota
	OpCreate
	OpDelete
)

func (o Op) String() string {
	switch o {
	case OpCreate:
		return "create"
	case OpDelete:
		return "delete"
	default:
		return "update"
	}
}

// Change is one edit apply would make. PreImage is what was there before,
// so a delete under `full` can be reported and reconstructed.
type Change struct {
	Family   string `json:"family"`
	Key      string `json:"key"`
	Op       Op     `json:"-"`
	Want     any    `json:"want,omitempty"`
	Live     any    `json:"live,omitempty"`
	PreImage any    `json:"pre_image,omitempty"`
}

func (c Change) String() string {
	return fmt.Sprintf("%s %s.%s: %v → %v", c.Op, c.Family, c.Key, c.Live, c.Want)
}

// Family is one manageable group of settings.
type Family interface {
	// Name is the key under repo:/org: in gcfg.yaml.
	Name() string
	// Scope says which block the family belongs to.
	Scope() Scope
	// Permission is the token permission its writes need, quoted in the
	// error when GitHub answers 403.
	Permission() string
	// Read fetches the live state.
	Read(ctx context.Context, c gh.Client, t Target) (Live, error)
	// Export renders live state as the YAML node that belongs in the file.
	Export(live Live) (*yaml.Node, error)
	// Diff compares the declared node with live state under an ownership
	// rule, returning what to report and what apply would change.
	Diff(desired *yaml.Node, live Live, own schema.Ownership) ([]Finding, []Change)
	// Apply performs the changes; it never re-reads (the engine does).
	Apply(ctx context.Context, c gh.Client, t Target, changes []Change) error
}

// Registry holds the families a build knows about.
type Registry struct {
	byScope map[Scope]map[string]Family
}

// NewRegistry returns an empty registry; the default one is Default.
func NewRegistry() *Registry {
	return &Registry{byScope: map[Scope]map[string]Family{}}
}

// Default is the registry the CLI uses; families register into it from
// their own package's init.
var Default = NewRegistry()

// Register adds a family. A duplicate is a programming mistake, so it
// panics at startup rather than silently shadowing.
func (r *Registry) Register(f Family) {
	if r.byScope[f.Scope()] == nil {
		r.byScope[f.Scope()] = map[string]Family{}
	}
	if _, dup := r.byScope[f.Scope()][f.Name()]; dup {
		panic(fmt.Sprintf("gcfg: family %q registered twice for scope %s", f.Name(), f.Scope()))
	}
	r.byScope[f.Scope()][f.Name()] = f
}

// Register adds a family to the default registry.
func Register(f Family) { Default.Register(f) }

// All returns every family in a scope, name-ordered so output is stable.
func (r *Registry) All(s Scope) []Family {
	fs := make([]Family, 0, len(r.byScope[s]))
	for _, f := range r.byScope[s] {
		fs = append(fs, f)
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].Name() < fs[j].Name() })
	return fs
}

// All returns every family in a scope from the default registry.
func All(s Scope) []Family { return Default.All(s) }

// Lookup finds one family by scope and name.
func (r *Registry) Lookup(s Scope, name string) (Family, bool) {
	f, ok := r.byScope[s][name]
	return f, ok
}

// Select resolves --only. An empty selection means every family; an
// unknown name is an error that lists what does exist.
func (r *Registry) Select(s Scope, only []string) ([]Family, error) {
	all := r.All(s)
	if len(only) == 0 {
		return all, nil
	}
	var out []Family
	var unknown []string
	for _, name := range only {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if f, ok := r.Lookup(s, name); ok {
			out = append(out, f)
			continue
		}
		unknown = append(unknown, name)
	}
	if len(unknown) > 0 {
		var have []string
		for _, f := range all {
			have = append(have, f.Name())
		}
		return nil, fmt.Errorf("unknown %s family %s (have: %s)", s, strings.Join(unknown, ", "), strings.Join(have, ", "))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

// Select resolves --only against the default registry.
func Select(s Scope, only []string) ([]Family, error) { return Default.Select(s, only) }
