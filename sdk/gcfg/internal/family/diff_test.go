package family

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"gopkg.in/yaml.v3"
)

// Every family reports through Differ, so its rules are the rules: an
// undeclared key is silent, a declared key that matches is silent, and a
// declared key that disagrees is one finding plus one change carrying the
// pre-image.
func TestDifferOnlyReportsDeclaredDisagreements(t *testing.T) {
	d := NewDiffer("general")
	d.Scalar("a", false, "want", "live") // undeclared
	d.Scalar("b", true, "same", "same")  // agrees
	d.Scalar("c", true, true, false)     // drift
	d.List("t1", false, []string{"x"}, nil)
	d.List("t2", true, []string{"a", "b"}, []string{"b", "a"}) // order-insensitive
	d.List("t3", true, []string{"a"}, []string{"b"})

	fs, cs := d.Result()
	if len(fs) != 2 || len(cs) != 2 {
		t.Fatalf("want 2 findings/changes, got %d/%d: %v", len(fs), len(cs), fs)
	}
	if fs[0].Key != "c" || fs[0].Kind != Drift || fs[0].Family != "general" {
		t.Errorf("finding 0 = %+v", fs[0])
	}
	if cs[0].Op != OpUpdate || cs[0].PreImage != false {
		t.Errorf("change 0 must carry the pre-image: %+v", cs[0])
	}
	if fs[1].Key != "t3" {
		t.Errorf("finding 1 = %+v", fs[1])
	}
}

func TestDifferReportAndAddStampTheFamily(t *testing.T) {
	d := NewDiffer("labels")
	d.Report(Finding{Key: "chore", Kind: Unmanaged, Live: "chore"})
	d.Add(Change{Key: "chore", Op: OpDelete, PreImage: "chore"})
	fs, cs := d.Result()
	if len(fs) != 1 || fs[0].Family != "labels" || fs[0].Kind != Unmanaged {
		t.Fatalf("findings = %v", fs)
	}
	if len(cs) != 1 || cs[0].Family != "labels" || cs[0].Op != OpDelete {
		t.Fatalf("changes = %v", cs)
	}
}

func TestManagedFollowsOwnership(t *testing.T) {
	if Managed(schema.Declared) {
		t.Error("declared ownership does not manage extras")
	}
	if !Managed(schema.Full) {
		t.Error("full ownership manages extras")
	}
	if Managed("") {
		t.Error("an unset ownership must default to declared")
	}
}

func TestScopeString(t *testing.T) {
	if ScopeRepo.String() != "repo" || ScopeOrg.String() != "org" {
		t.Errorf("scopes = %q %q", ScopeRepo, ScopeOrg)
	}
}

func TestFindingStringCoversEveryKind(t *testing.T) {
	cases := []struct {
		f    Finding
		want []string
	}{
		{Finding{Family: "security", Key: "push_protection", Kind: Drift, Want: true, Live: false}, []string{"drift", "want true", "live false"}},
		{Finding{Family: "labels", Key: "chore", Kind: Unmanaged, Live: "chore"}, []string{"unmanaged", "not declared"}},
		{Finding{Family: "actions", Key: "*", Kind: Unreadable, Reason: "403"}, []string{"unreadable", "403"}},
		{Finding{Family: "security", Key: "non_provider_patterns", Kind: NotHonoured, Want: true, Live: false, Reason: "needs Secret Protection"}, []string{"not_honoured", "Secret Protection"}},
	}
	for _, tc := range cases {
		got := tc.f.String()
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Errorf("%v: missing %q in %q", tc.f.Kind, want, got)
			}
		}
	}
}

// The node helpers are the only way families read the file, so their
// tolerance for a missing or mistyped key is what keeps a family simple.
func TestNodeHelpers(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("a: true\nb: text\nc: [x, y]\nd: {e: 1}\n"), &doc); err != nil {
		t.Fatal(err)
	}
	n := doc.Content[0]

	if v, ok := Bool(n, "a"); !ok || !v {
		t.Errorf("Bool(a) = %v %v", v, ok)
	}
	if _, ok := Bool(n, "b"); ok {
		t.Error("Bool on a string must not claim success")
	}
	if _, ok := Bool(n, "missing"); ok {
		t.Error("Bool on a missing key must not claim success")
	}
	if v, ok := Str(n, "b"); !ok || v != "text" {
		t.Errorf("Str(b) = %q %v", v, ok)
	}
	if _, ok := Str(n, "c"); ok {
		t.Error("Str on a list must not claim success")
	}
	if v, ok := Strings(n, "c"); !ok || strings.Join(v, ",") != "x,y" {
		t.Errorf("Strings(c) = %v %v", v, ok)
	}
	if _, ok := Strings(n, "b"); ok {
		t.Error("Strings on a scalar must not claim success")
	}
	if sub, ok := Field(n, "d"); !ok || sub.Kind != yaml.MappingNode {
		t.Errorf("Field(d) = %v %v", sub, ok)
	}
	if _, ok := Field(nil, "a"); ok {
		t.Error("Field on a nil node must be silent")
	}
	if _, ok := Field(n.Content[1], "a"); ok {
		t.Error("Field on a scalar must be silent")
	}
}

func TestMapSkipsAbsentValuesAndScalarsCarryTags(t *testing.T) {
	if got := Map("a", nil); got != nil {
		t.Errorf("a map with nothing in it is absent, got %v", got)
	}
	m := Map("a", Scalar(true), "b", nil, "c", Scalar("x"))
	if len(m.Content) != 4 {
		t.Fatalf("want two pairs, got %d nodes", len(m.Content))
	}
	if m.Content[0].Value != "a" || m.Content[1].Tag != "!!bool" || m.Content[1].Value != "true" {
		t.Errorf("bool pair = %v %v", m.Content[0], m.Content[1])
	}
	if m.Content[2].Value != "c" || m.Content[3].Tag != "!!str" {
		t.Errorf("string pair = %v %v", m.Content[2], m.Content[3])
	}
	if got := Scalar(7); got.Tag != "!!int" || got.Value != "7" {
		t.Errorf("int scalar = %+v", got)
	}
	if got := Scalar(1.5); got.Tag != "!!str" {
		t.Errorf("unknown kinds fall back to a string: %+v", got)
	}
	seq := Seq([]string{"a", "b"})
	if seq.Kind != yaml.SequenceNode || len(seq.Content) != 2 {
		t.Errorf("seq = %+v", seq)
	}
	if empty := Seq(nil); empty.Kind != yaml.SequenceNode || len(empty.Content) != 0 {
		t.Errorf("a declared-empty list still renders: %+v", empty)
	}
}

func TestMapPanicsOnAnOddArgumentList(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("an odd pair list is a programming mistake and must panic")
		}
	}()
	Map("a")
}

func TestSameStrings(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{nil, nil, true},
		{[]string{"a"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{[]string{"a"}, []string{"a", "b"}, false},
		{[]string{"a", "a"}, []string{"a", "b"}, false},
	}
	for _, tc := range cases {
		if got := SameStrings(tc.a, tc.b); got != tc.want {
			t.Errorf("SameStrings(%v, %v) = %v", tc.a, tc.b, got)
		}
	}
}

// The package-level wrappers are what cmd uses.
func TestPackageLevelRegistry(t *testing.T) {
	if len(All(ScopeRepo)) == 0 {
		t.Skip("no families registered in this test binary")
	}
}
