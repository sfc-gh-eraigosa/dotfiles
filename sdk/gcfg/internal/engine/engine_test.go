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

var target = family.Target{Owner: "o", Repo: "r"}

// fake is a Family whose behaviour each test dictates: what it reads, what
// it reports, and what it records when applied.
type fake struct {
	name     string
	scope    family.Scope
	live     any
	readErr  error
	export   *yaml.Node
	findings []family.Finding
	changes  []family.Change
	// after is what Diff returns once Apply has run (the re-read).
	afterFindings []family.Finding
	applied       [][]family.Change
	applyErr      error
	reads         int
}

func (f *fake) Name() string        { return f.name }
func (f *fake) Scope() family.Scope { return f.scope }
func (f *fake) Permission() string  { return "repo:Administration:write" }

func (f *fake) Read(context.Context, gh.Client, family.Target) (family.Live, error) {
	f.reads++
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.live, nil
}

func (f *fake) Export(family.Live) (*yaml.Node, error) { return f.export, nil }

func (f *fake) Diff(*yaml.Node, family.Live, schema.Ownership) ([]family.Finding, []family.Change) {
	if len(f.applied) > 0 {
		return f.afterFindings, nil
	}
	return f.findings, f.changes
}

func (f *fake) Apply(_ context.Context, _ gh.Client, _ family.Target, cs []family.Change) error {
	if f.applyErr != nil {
		return f.applyErr
	}
	f.applied = append(f.applied, cs)
	return nil
}

func node(t *testing.T, body string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func file(t *testing.T, body string) *schema.File {
	t.Helper()
	f, _, err := schema.Parse([]byte(body), "test")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func reg(fs ...family.Family) *family.Registry {
	r := family.NewRegistry()
	for _, f := range fs {
		r.Register(f)
	}
	return r
}

func TestVerifyCleanIsNoFindings(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo}
	e := New(reg(g))
	rep, err := e.Verify(context.Background(), gh.NewFake(), target, file(t, "version: 1\nrepo:\n  general:\n    description: x\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 || !rep.Clean() {
		t.Fatalf("want a clean report, got %+v", rep)
	}
	if g.reads != 1 {
		t.Errorf("want one read, got %d", g.reads)
	}
}

func TestVerifyCollectsFindingsFromEveryFamily(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo, findings: []family.Finding{{Family: "general", Key: "a", Kind: family.Drift}}}
	s := &fake{name: "security", scope: family.ScopeRepo, findings: []family.Finding{{Family: "security", Key: "b", Kind: family.Drift}}}
	rep, err := New(reg(g, s)).Verify(context.Background(), gh.NewFake(), target,
		file(t, "version: 1\nrepo:\n  general: {description: x}\n  security: {secret_scanning: true}\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 2 || rep.Clean() {
		t.Fatalf("findings = %v", rep.Findings)
	}
	if rep.Findings[0].Family != "general" || rep.Findings[1].Family != "security" {
		t.Errorf("findings must stay in family order: %v", rep.Findings)
	}
	if rep.Counts[family.Drift] != 2 {
		t.Errorf("counts = %v", rep.Counts)
	}
}

// A family that cannot be read is a finding, never a fatal error — one
// missing permission must not blind the whole run.
func TestVerifyUnreadableFamilyIsAFindingNotAnError(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo}
	s := &fake{name: "security", scope: family.ScopeRepo, readErr: errors.New("HTTP 403 Resource not accessible by personal access token")}
	rep, err := New(reg(g, s)).Verify(context.Background(), gh.NewFake(), target,
		file(t, "version: 1\nrepo:\n  general: {description: x}\n  security: {secret_scanning: true}\n"), Options{})
	if err != nil {
		t.Fatalf("an unreadable family must not fail the run: %v", err)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != family.Unreadable {
		t.Fatalf("findings = %v", rep.Findings)
	}
	if !strings.Contains(rep.Findings[0].Reason, "403") {
		t.Errorf("the reason should carry what GitHub said: %q", rep.Findings[0].Reason)
	}
	if !strings.Contains(rep.Findings[0].Reason, "Administration") {
		t.Errorf("the reason should name the permission it needs: %q", rep.Findings[0].Reason)
	}
	if rep.Clean() {
		t.Error("a report with an unreadable family is not clean")
	}
}

// Every family unreadable means the answer is meaningless: that is exit 2,
// not "no drift".
func TestVerifyAllUnreadableIsAnError(t *testing.T) {
	boom := errors.New("HTTP 403 nope")
	g := &fake{name: "general", scope: family.ScopeRepo, readErr: boom}
	_, err := New(reg(g)).Verify(context.Background(), gh.NewFake(), target,
		file(t, "version: 1\nrepo:\n  general: {description: x}\n"), Options{})
	if err == nil || !errors.Is(err, ErrAllUnreadable) {
		t.Fatalf("want ErrAllUnreadable, got %v", err)
	}
}

// Only declared families are visited: an absent block is not "set it to
// empty", it is "gcfg does not manage this".
func TestVerifySkipsUndeclaredFamilies(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo}
	s := &fake{name: "security", scope: family.ScopeRepo}
	if _, err := New(reg(g, s)).Verify(context.Background(), gh.NewFake(), target,
		file(t, "version: 1\nrepo:\n  general: {description: x}\n"), Options{}); err != nil {
		t.Fatal(err)
	}
	if g.reads != 1 || s.reads != 0 {
		t.Fatalf("reads: general=%d security=%d", g.reads, s.reads)
	}
}

func TestVerifyOnlySelectsFamilies(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo}
	s := &fake{name: "security", scope: family.ScopeRepo}
	f := file(t, "version: 1\nrepo:\n  general: {description: x}\n  security: {secret_scanning: true}\n")
	if _, err := New(reg(g, s)).Verify(context.Background(), gh.NewFake(), target, f, Options{Only: []string{"security"}}); err != nil {
		t.Fatal(err)
	}
	if g.reads != 0 || s.reads != 1 {
		t.Fatalf("--only should visit security alone: general=%d security=%d", g.reads, s.reads)
	}
	if _, err := New(reg(g, s)).Verify(context.Background(), gh.NewFake(), target, f, Options{Only: []string{"nope"}}); err == nil {
		t.Fatal("an unknown --only name must be an error")
	}
}

func TestPlanReturnsChangesWithoutWriting(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo,
		findings: []family.Finding{{Key: "a", Kind: family.Drift}},
		changes:  []family.Change{{Family: "general", Key: "a", Op: family.OpUpdate, Want: true}}}
	changes, rep, err := New(reg(g)).Plan(context.Background(), gh.NewFake(), target,
		file(t, "version: 1\nrepo:\n  general: {description: x}\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || len(rep.Findings) != 1 {
		t.Fatalf("changes=%v findings=%v", changes, rep.Findings)
	}
	if len(g.applied) != 0 {
		t.Fatal("plan must not apply anything")
	}
}

// Apply writes, then re-reads and re-diffs. The report it returns is the
// state after the write, which is the only honest answer.
func TestApplyWritesThenReReads(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo,
		findings: []family.Finding{{Key: "a", Kind: family.Drift}},
		changes:  []family.Change{{Family: "general", Key: "a", Op: family.OpUpdate, Want: true}}}
	e := New(reg(g))
	f := file(t, "version: 1\nrepo:\n  general: {description: x}\n")
	changes, _, err := e.Plan(context.Background(), gh.NewFake(), target, f, Options{})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := e.Apply(context.Background(), gh.NewFake(), target, f, changes, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.applied) != 1 || len(g.applied[0]) != 1 {
		t.Fatalf("apply received %v", g.applied)
	}
	// Two reads, not three: apply trusts the plan it was handed and spends
	// its second read on the re-read, which is the one that matters.
	if g.reads != 2 {
		t.Errorf("want one read for the plan and one to re-read after writing, got %d", g.reads)
	}
	if !rep.Clean() {
		t.Errorf("the re-read reported nothing, so the report is clean: %v", rep.Findings)
	}
}

// A setting GitHub accepted but ignored survives the re-read, and that is
// what apply must report.
func TestApplyReportsWhatSurvivedTheReRead(t *testing.T) {
	g := &fake{name: "security", scope: family.ScopeRepo,
		findings:      []family.Finding{{Key: "non_provider_patterns", Kind: family.Drift}},
		changes:       []family.Change{{Family: "security", Key: "non_provider_patterns", Op: family.OpUpdate, Want: true}},
		afterFindings: []family.Finding{{Family: "security", Key: "non_provider_patterns", Kind: family.NotHonoured, Reason: "needs Secret Protection"}}}
	e := New(reg(g))
	f := file(t, "version: 1\nrepo:\n  security: {non_provider_patterns: true}\n")
	changes, _, err := e.Plan(context.Background(), gh.NewFake(), target, f, Options{})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := e.Apply(context.Background(), gh.NewFake(), target, f, changes, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean() || len(rep.Findings) != 1 || rep.Findings[0].Kind != family.NotHonoured {
		t.Fatalf("apply must report what survived: %+v", rep.Findings)
	}
}

func TestApplyDryRunWritesNothing(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo,
		changes: []family.Change{{Family: "general", Key: "a", Op: family.OpUpdate, Want: true}}}
	e := New(reg(g))
	f := file(t, "version: 1\nrepo:\n  general: {description: x}\n")
	changes, _, _ := e.Plan(context.Background(), gh.NewFake(), target, f, Options{})
	if _, err := e.Apply(context.Background(), gh.NewFake(), target, f, changes, Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if len(g.applied) != 0 {
		t.Fatal("--dry-run must not write")
	}
}

func TestApplyFailureIsReported(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo,
		changes:  []family.Change{{Family: "general", Key: "a", Op: family.OpUpdate}},
		applyErr: errors.New("HTTP 403 Resource not accessible"),
	}
	e := New(reg(g))
	f := file(t, "version: 1\nrepo:\n  general: {description: x}\n")
	changes, _, _ := e.Plan(context.Background(), gh.NewFake(), target, f, Options{})
	_, err := e.Apply(context.Background(), gh.NewFake(), target, f, changes, Options{})
	if err == nil || !strings.Contains(err.Error(), "general") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want an error naming the family and the cause, got %v", err)
	}
}

func TestExportBuildsAFileFromLiveState(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo, export: node(t, "description: live text\n")}
	s := &fake{name: "security", scope: family.ScopeRepo, export: node(t, "secret_scanning: true\n")}
	f, findings, err := New(reg(g, s)).Export(context.Background(), gh.NewFake(), target, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v", findings)
	}
	b, err := yaml.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{"version: 1", "repo:", "general:", "description: live text", "security:", "secret_scanning: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q:\n%s", want, out)
		}
	}
	// What export produces must load back through the strict loader.
	if _, _, err := schema.Parse(b, "export"); err != nil {
		t.Fatalf("exported file does not load: %v\n%s", err, out)
	}
}

// Export never fails because one family cannot be read: it reports and
// carries on, so a partial file is still worth having.
func TestExportReportsUnreadableFamilies(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo, export: node(t, "description: x\n")}
	s := &fake{name: "security", scope: family.ScopeRepo, readErr: errors.New("HTTP 403 nope")}
	f, findings, err := New(reg(g, s)).Export(context.Background(), gh.NewFake(), target, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Kind != family.Unreadable {
		t.Fatalf("findings = %v", findings)
	}
	b, _ := yaml.Marshal(f)
	if strings.Contains(string(b), "security") {
		t.Errorf("an unreadable family must not appear in the file:\n%s", b)
	}
}

func TestOrgScopeUsesTheOrgBlock(t *testing.T) {
	p := &fake{name: "profile", scope: family.ScopeOrg}
	g := &fake{name: "general", scope: family.ScopeRepo}
	f := file(t, "version: 1\nrepo:\n  general: {description: x}\norg:\n  profile: {description: y}\n")
	if _, err := New(reg(p, g)).Verify(context.Background(), gh.NewFake(), family.Target{Owner: "acme"}, f, Options{Org: true}); err != nil {
		t.Fatal(err)
	}
	if p.reads != 1 || g.reads != 0 {
		t.Fatalf("org run must visit org families only: profile=%d general=%d", p.reads, g.reads)
	}
}

func TestVerifyWithNothingDeclaredIsCleanAndSaysSo(t *testing.T) {
	g := &fake{name: "general", scope: family.ScopeRepo}
	rep, err := New(reg(g)).Verify(context.Background(), gh.NewFake(), target, file(t, "version: 1\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() || rep.Families != 0 {
		t.Fatalf("report = %+v", rep)
	}
}
