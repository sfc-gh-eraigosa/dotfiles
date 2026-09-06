package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"gopkg.in/yaml.v3"
)

// ownerSpy records the ownership the engine hands each family — the whole
// point of the setting is that a family is told, not left to guess.
type ownerSpy struct {
	*fake
	seen []schema.Ownership
}

func (o *ownerSpy) Diff(n *yaml.Node, live family.Live, own schema.Ownership) ([]family.Finding, []family.Change) {
	o.seen = append(o.seen, own)
	return o.fake.Diff(n, live, own)
}

func TestOwnershipResolution(t *testing.T) {
	cases := []struct {
		name string
		body string
		want schema.Ownership
	}{
		{"unset defaults to declared", "version: 1\nrepo:\n  labels: [{name: bug, color: aaaaaa}]\n", schema.Declared},
		{"file level applies", "version: 1\nownership: full\nrepo:\n  labels: [{name: bug, color: aaaaaa}]\n", schema.Full},
		{"family overrides the file", "version: 1\nownership: full\nrepo:\n  labels:\n    ownership: declared\n    items: [{name: bug, color: aaaaaa}]\n", schema.Declared},
		{"family alone", "version: 1\nrepo:\n  labels:\n    ownership: full\n    items: [{name: bug, color: aaaaaa}]\n", schema.Full},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &ownerSpy{fake: &fake{name: "labels", scope: family.ScopeRepo}}
			if _, err := New(reg(spy)).Verify(context.Background(), gh.NewFake(), target, file(t, tc.body), Options{}); err != nil {
				t.Fatal(err)
			}
			if len(spy.seen) != 1 || spy.seen[0] != tc.want {
				t.Fatalf("ownership handed to the family = %v, want %q", spy.seen, tc.want)
			}
		})
	}
}

func TestHeadlineReadsAsASentence(t *testing.T) {
	clean := report("o/r", 3, nil)
	if got := clean.Headline(); got != "o/r: clean (3 families checked)" {
		t.Errorf("clean headline = %q", got)
	}
	dirty := report("o/r", 4, []family.Finding{
		{Kind: family.Drift}, {Kind: family.Drift}, {Kind: family.Unmanaged}, {Kind: family.Unreadable},
	})
	got := dirty.Headline()
	for _, want := range []string{"o/r", "2 drift", "1 unmanaged", "1 unreadable", "4 families checked"} {
		if !strings.Contains(got, want) {
			t.Errorf("headline missing %q: %q", want, got)
		}
	}
	// Kinds appear in severity order, not map order.
	if strings.Index(got, "drift") > strings.Index(got, "unmanaged") {
		t.Errorf("drift should come first: %q", got)
	}
}

// An org run against a file with no org block simply has nothing to do.
func TestBlockNodesToleratesAMissingBlock(t *testing.T) {
	p := &fake{name: "profile", scope: family.ScopeOrg}
	rep, err := New(reg(p)).Verify(context.Background(), gh.NewFake(), family.Target{Owner: "acme"}, file(t, "version: 1\nrepo:\n  general: {description: x}\n"), Options{Org: true})
	if err != nil || !rep.Clean() || rep.Families != 0 {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
	if p.reads != 0 {
		t.Errorf("nothing declared means nothing read, got %d", p.reads)
	}
}

// Plan surfaces a read failure the same way verify does, and an --only
// mistake before touching the network.
func TestPlanErrors(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo, readErr: errors.New("HTTP 403 nope")}
	if _, _, err := New(reg(g)).Plan(context.Background(), gh.NewFake(), target, file(t, "version: 1\nrepo:\n  general: {description: x}\n"), Options{}); !errors.Is(err, ErrAllUnreadable) {
		t.Fatalf("want ErrAllUnreadable, got %v", err)
	}
	g2 := &fake{name: "general", scope: family.ScopeRepo}
	if _, _, err := New(reg(g2)).Plan(context.Background(), gh.NewFake(), target, file(t, "version: 1\nrepo:\n  general: {description: x}\n"), Options{Only: []string{"nope"}}); err == nil {
		t.Fatal("an unknown --only name must fail before any request")
	}
	if g2.reads != 0 {
		t.Errorf("a bad --only must not read anything, got %d reads", g2.reads)
	}
}

// Export refuses an unknown --only too, and returns just `version: 1` when
// no family produced anything.
func TestExportEdges(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo}
	if _, _, err := New(reg(g)).Export(context.Background(), gh.NewFake(), target, Options{Only: []string{"nope"}}); err == nil {
		t.Fatal("want an error for an unknown family")
	}
	f, findings, err := New(reg(g)).Export(context.Background(), gh.NewFake(), target, Options{})
	if err != nil || len(findings) != 0 {
		t.Fatalf("f=%v findings=%v err=%v", f, findings, err)
	}
	b, err := yaml.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "repo:") {
		t.Errorf("a family that exports nothing must not leave an empty block:\n%s", b)
	}
	if _, _, err := schema.Parse(b, "export"); err != nil {
		t.Fatalf("even the empty export must load: %v\n%s", err, b)
	}
}

// Apply with nothing to change still re-reads and reports.
func TestApplyWithNoChangesStillVerifies(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo}
	rep, err := New(reg(g)).Apply(context.Background(), gh.NewFake(), target, file(t, "version: 1\nrepo:\n  general: {description: x}\n"), nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Errorf("rep = %+v", rep)
	}
	if len(g.applied) != 0 {
		t.Error("nothing to apply means no write")
	}
	if g.reads != 1 {
		t.Errorf("want the re-read, got %d reads", g.reads)
	}
}

// A family with no DiffAfterApply still works after an apply: it simply
// reports ordinary drift for anything that did not take.
func TestApplyFallsBackToPlainDiff(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo,
		changes:       []family.Change{{Family: "general", Key: "a", Op: family.OpUpdate, Want: true}},
		afterFindings: []family.Finding{{Family: "general", Key: "a", Kind: family.Drift}}}
	rep, err := New(reg(g)).Apply(context.Background(), gh.NewFake(), target,
		file(t, "version: 1\nrepo:\n  general: {description: x}\n"),
		[]family.Change{{Family: "general", Key: "a", Op: family.OpUpdate, Want: true}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != family.Drift {
		t.Fatalf("rep = %+v", rep.Findings)
	}
}
