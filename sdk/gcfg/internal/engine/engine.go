// Package engine runs the verbs over the families: export what is live,
// verify a file against it, plan the difference, and apply it — then read
// everything back, because a write GitHub accepted is not the same as a
// setting that changed (plan §3.2, design §7).
package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"gopkg.in/yaml.v3"
)

// ErrAllUnreadable is returned when the credential could not read a single
// declared family: the run has no information, so "no drift" would be a lie.
var ErrAllUnreadable = errors.New("no declared family could be read")

// Options are the flags a verb passes through.
type Options struct {
	Only   []string // --only a,b
	Org    bool     // --org: the org block instead of the repo block
	DryRun bool     // apply --dry-run
	Strict bool     // --strict: an unreadable family fails the run
}

// Report is what verify and apply return.
type Report struct {
	Target   string           `json:"target"`
	Families int              `json:"families"`
	Findings []family.Finding `json:"findings"`
	// Counts is how many findings of each kind, for the one-line headline.
	Counts map[family.Kind]int `json:"-"`
}

// Clean is true when nothing needs a human.
func (r Report) Clean() bool { return len(r.Findings) == 0 }

// Headline is the one-line summary the renderers put first.
func (r Report) Headline() string {
	if r.Clean() {
		return fmt.Sprintf("%s: clean (%d families checked)", r.Target, r.Families)
	}
	var parts []string
	for _, k := range []family.Kind{family.Drift, family.Unmanaged, family.NotHonoured, family.Unreadable} {
		if n := r.Counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	return fmt.Sprintf("%s: %s (%d families checked)", r.Target, strings.Join(parts, ", "), r.Families)
}

// Engine runs verbs against a registry of families.
type Engine struct {
	reg *family.Registry
}

// New builds an engine over a registry (family.Default in production).
func New(reg *family.Registry) *Engine { return &Engine{reg: reg} }

// scope is which block this run operates on.
func (o Options) scope() family.Scope {
	if o.Org {
		return family.ScopeOrg
	}
	return family.ScopeRepo
}

// declared returns the families to visit: those the registry knows, that
// --only selects, and that the file actually declares.
func (e *Engine) declared(f *schema.File, o Options) ([]family.Family, map[string]*yaml.Node, error) {
	selected, err := e.reg.Select(o.scope(), o.Only)
	if err != nil {
		return nil, nil, err
	}
	nodes, err := blockNodes(f, o.scope())
	if err != nil {
		return nil, nil, err
	}
	var out []family.Family
	for _, fam := range selected {
		if _, ok := nodes[fam.Name()]; ok {
			out = append(out, fam)
		}
	}
	return out, nodes, nil
}

// blockNodes re-encodes the typed file back to nodes so each family can read
// its own declared fragment, comments and all.
func blockNodes(f *schema.File, scope family.Scope) (map[string]*yaml.Node, error) {
	var block any
	if scope == family.ScopeOrg {
		if f.Org == nil {
			return map[string]*yaml.Node{}, nil
		}
		block = f.Org
	} else {
		if f.Repo == nil {
			return map[string]*yaml.Node{}, nil
		}
		block = f.Repo
	}
	b, err := yaml.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("reading the declared settings: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("reading the declared settings: %w", err)
	}
	out := map[string]*yaml.Node{}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return out, nil
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		out[m.Content[i].Value] = m.Content[i+1]
	}
	return out, nil
}

// unreadable turns a read failure into the finding verify reports.
func unreadable(fam family.Family, err error) family.Finding {
	return family.Finding{
		Family: fam.Name(),
		Key:    "*",
		Kind:   family.Unreadable,
		Reason: fmt.Sprintf("%v (needs %s)", err, fam.Permission()),
	}
}

// report assembles a Report from findings.
func report(target string, families int, findings []family.Finding) Report {
	counts := map[family.Kind]int{}
	for _, f := range findings {
		counts[f.Kind]++
	}
	return Report{Target: target, Families: families, Findings: findings, Counts: counts}
}

// diffAll reads every declared family and diffs it. applied carries the
// changes a preceding apply made, so a family that can tell the difference
// reports not_honoured instead of drift.
func (e *Engine) diffAll(ctx context.Context, c gh.Client, t family.Target, f *schema.File, o Options, applied map[string][]family.Change) ([]family.Finding, []family.Change, int, error) {
	fams, nodes, err := e.declared(f, o)
	if err != nil {
		return nil, nil, 0, err
	}
	var findings []family.Finding
	var changes []family.Change
	unread := 0
	for _, fam := range fams {
		live, err := fam.Read(ctx, c, t)
		if err != nil {
			unread++
			findings = append(findings, unreadable(fam, err))
			continue
		}
		own := ownership(f, nodes[fam.Name()])
		fs, cs := diff(fam, nodes[fam.Name()], live, own, applied[fam.Name()])
		findings = append(findings, fs...)
		changes = append(changes, cs...)
	}
	if len(fams) > 0 && unread == len(fams) {
		return findings, changes, len(fams), fmt.Errorf("%w on %s", ErrAllUnreadable, t)
	}
	return findings, changes, len(fams), nil
}

// afterApplyDiffer is implemented by a family that can tell a write GitHub
// ignored from ordinary drift.
type afterApplyDiffer interface {
	DiffAfterApply(desired *yaml.Node, live family.Live, own schema.Ownership, applied []family.Change) ([]family.Finding, []family.Change)
}

func diff(fam family.Family, desired *yaml.Node, live family.Live, own schema.Ownership, applied []family.Change) ([]family.Finding, []family.Change) {
	if len(applied) > 0 {
		if d, ok := fam.(afterApplyDiffer); ok {
			return d.DiffAfterApply(desired, live, own, applied)
		}
	}
	return fam.Diff(desired, live, own)
}

// ownership is the family's own setting, else the file's, else declared.
func ownership(f *schema.File, node *yaml.Node) schema.Ownership {
	if own, ok := family.Str(node, "ownership"); ok && own != "" {
		return schema.Ownership(own)
	}
	if f.Ownership != "" {
		return f.Ownership
	}
	return schema.Declared
}

// Verify reads every declared family and reports what disagrees.
func (e *Engine) Verify(ctx context.Context, c gh.Client, t family.Target, f *schema.File, o Options) (Report, error) {
	findings, _, n, err := e.diffAll(ctx, c, t, f, o, nil)
	if err != nil {
		return report(t.String(), n, findings), err
	}
	return report(t.String(), n, findings), nil
}

// Plan is Verify plus the changes apply would make. It writes nothing.
func (e *Engine) Plan(ctx context.Context, c gh.Client, t family.Target, f *schema.File, o Options) ([]family.Change, Report, error) {
	findings, changes, n, err := e.diffAll(ctx, c, t, f, o, nil)
	rep := report(t.String(), n, findings)
	if err != nil {
		return nil, rep, err
	}
	return changes, rep, nil
}

// Apply performs the changes and then re-reads everything, returning the
// report for the state that actually resulted. A clean report is the only
// proof an apply worked.
func (e *Engine) Apply(ctx context.Context, c gh.Client, t family.Target, f *schema.File, changes []family.Change, o Options) (Report, error) {
	byFamily := map[string][]family.Change{}
	for _, ch := range changes {
		byFamily[ch.Family] = append(byFamily[ch.Family], ch)
	}
	if o.DryRun {
		findings, _, n, err := e.diffAll(ctx, c, t, f, o, nil)
		return report(t.String(), n, findings), err
	}
	fams, _, err := e.declared(f, o)
	if err != nil {
		return Report{}, err
	}
	for _, fam := range fams {
		cs := byFamily[fam.Name()]
		if len(cs) == 0 {
			continue
		}
		if err := fam.Apply(ctx, c, t, cs); err != nil {
			return Report{}, fmt.Errorf("applying %s: %w", fam.Name(), err)
		}
	}
	// The re-read is the point: it catches a write that was accepted and
	// ignored, and it is what the caller reports.
	findings, _, n, err := e.diffAll(ctx, c, t, f, o, byFamily)
	return report(t.String(), n, findings), err
}

// Export reads every family in scope and builds a file from live state.
// A family that cannot be read is reported and skipped, never fatal.
func (e *Engine) Export(ctx context.Context, c gh.Client, t family.Target, o Options) (*schema.File, []family.Finding, error) {
	fams, err := e.reg.Select(o.scope(), o.Only)
	if err != nil {
		return nil, nil, err
	}
	var findings []family.Finding
	pairs := map[string]*yaml.Node{}
	for _, fam := range fams {
		live, err := fam.Read(ctx, c, t)
		if err != nil {
			findings = append(findings, unreadable(fam, err))
			continue
		}
		node, err := fam.Export(live)
		if err != nil {
			findings = append(findings, family.Finding{Family: fam.Name(), Key: "*", Kind: family.Unreadable, Reason: err.Error()})
			continue
		}
		if node != nil {
			pairs[fam.Name()] = node
		}
	}
	f, err := buildFile(pairs, o.scope())
	if err != nil {
		return nil, findings, err
	}
	return f, findings, nil
}

// buildFile turns per-family nodes into a typed File, so what export writes
// is exactly what load accepts.
func buildFile(pairs map[string]*yaml.Node, scope family.Scope) (*schema.File, error) {
	block := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	names := make([]string, 0, len(pairs))
	for name := range pairs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		block.Content = append(block.Content, family.Scalar(name), pairs[name])
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map",
		Content: []*yaml.Node{family.Scalar("version"), family.Scalar(schema.Version)}}
	key := "repo"
	if scope == family.ScopeOrg {
		key = "org"
	}
	if len(block.Content) > 0 {
		root.Content = append(root.Content, family.Scalar(key), block)
	}
	b, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("building the exported file: %w", err)
	}
	f, _, err := schema.Parse(b, "export")
	if err != nil {
		return nil, fmt.Errorf("the exported file does not satisfy the schema: %w", err)
	}
	return f, nil
}
