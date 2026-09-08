package general

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/schema"
	"gopkg.in/yaml.v3"
)

var target = family.Target{Owner: "sfc-gh-eraigosa", Repo: "dotfiles"}

func fixture(t *testing.T) (*gh.Fake, family.Live) {
	t.Helper()
	f := gh.NewFake()
	f.GetFile(t, "/repos/sfc-gh-eraigosa/dotfiles", filepath.Join("testdata", "repo.json"))
	live, err := New().Read(context.Background(), f, target)
	if err != nil {
		t.Fatal(err)
	}
	return f, live
}

// desired parses a YAML fragment the way the engine hands it to a family.
func desired(t *testing.T, body string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func TestReadAndExportRoundTrip(t *testing.T) {
	_, live := fixture(t)
	node, err := New().Export(live)
	if err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"description: My dotfiles and lab tooling",
		"homepage: https://example.com",
		"visibility: public",
		"default_branch: main",
		"issues: true",
		"wiki: false",
		"discussions: true",
		"squash: true",
		"merge_commit: false",
		"delete_branch_on_merge: true",
		"squash_title: COMMIT_OR_PR_TITLE",
		"squash_message: COMMIT_MESSAGES",
		"allow_forking: true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export missing %q:\n%s", want, got)
		}
	}
	// What is exported must load back into the schema unchanged.
	f, _, err := schema.Parse([]byte("version: 1\nrepo:\n  general:\n"+indent(got, 4)), "export")
	if err != nil {
		t.Fatalf("exported YAML does not load: %v\n%s", err, got)
	}
	g := f.Repo.General
	if *g.Description != "My dotfiles and lab tooling" || *g.Visibility != "public" || !*g.Features.Issues || *g.Features.Wiki {
		t.Errorf("round-trip lost information: %+v", g)
	}
	if (*g.Topics)[0] != "dotfiles" || len(*g.Topics) != 3 {
		t.Errorf("topics = %v", *g.Topics)
	}
}

func TestDiffCleanFileHasNoFindings(t *testing.T) {
	_, live := fixture(t)
	node := desired(t, `
description: My dotfiles and lab tooling
visibility: public
features: {issues: true, wiki: false}
merge: {squash: true, merge_commit: false, delete_branch_on_merge: true}
`)
	fs, cs := New().Diff(node, live, schema.Declared)
	if len(fs) != 0 || len(cs) != 0 {
		t.Fatalf("want nothing to report or change, got %v / %v", fs, cs)
	}
}

func TestDiffReportsDriftPerKeyAndPlansTheChange(t *testing.T) {
	_, live := fixture(t)
	node := desired(t, `
description: Something else
features: {wiki: true}
merge: {delete_branch_on_merge: false}
`)
	fs, cs := New().Diff(node, live, schema.Declared)
	if len(fs) != 3 || len(cs) != 3 {
		t.Fatalf("want 3 findings and 3 changes, got %d/%d: %v", len(fs), len(cs), fs)
	}
	byKey := map[string]family.Finding{}
	for _, f := range fs {
		byKey[f.Key] = f
		if f.Kind != family.Drift || f.Family != "general" {
			t.Errorf("%s: kind=%v family=%q", f.Key, f.Kind, f.Family)
		}
	}
	if got := byKey["description"]; got.Want != "Something else" || got.Live != "My dotfiles and lab tooling" {
		t.Errorf("description finding = %+v", got)
	}
	if got := byKey["features.wiki"]; got.Want != true || got.Live != false {
		t.Errorf("wiki finding = %+v", got)
	}
	if _, ok := byKey["merge.delete_branch_on_merge"]; !ok {
		t.Errorf("keys = %v", keys(byKey))
	}
	for _, c := range cs {
		if c.Op != family.OpUpdate || c.Family != "general" {
			t.Errorf("change = %+v", c)
		}
	}
}

// Undeclared keys are unmanaged under `declared` — and stay unmanaged under
// `full` too: this family is a fixed set of scalars, so "extra" has no
// meaning. (Extras matter for list families: labels, rulesets.)
func TestDiffUndeclaredKeysAreNeverTouched(t *testing.T) {
	_, live := fixture(t)
	node := desired(t, "description: My dotfiles and lab tooling\n")
	for _, own := range []schema.Ownership{schema.Declared, schema.Full} {
		fs, cs := New().Diff(node, live, own)
		if len(cs) != 0 {
			t.Errorf("%s: undeclared keys must never be changed, got %v", own, cs)
		}
		if len(fs) != 0 {
			t.Errorf("%s: undeclared keys must not be reported for this family, got %v", own, fs)
		}
	}
}

func TestApplySendsOnePatchWithOnlyTheChangedKeys(t *testing.T) {
	f, live := fixture(t)
	node := desired(t, "description: New text\nfeatures: {wiki: true}\nmerge: {squash_title: PR_TITLE}\n")
	_, cs := New().Diff(node, live, schema.Declared)
	if err := New().Apply(context.Background(), f, target, cs); err != nil {
		t.Fatal(err)
	}
	writes := f.Writes()
	if len(writes) != 1 || writes[0].Method != "PATCH" || writes[0].Path != "/repos/sfc-gh-eraigosa/dotfiles" {
		t.Fatalf("want one PATCH to the repo, got %+v", writes)
	}
	body := writes[0].Body
	if body["description"] != "New text" || body["has_wiki"] != true || body["squash_merge_commit_title"] != "PR_TITLE" {
		t.Fatalf("body = %v", body)
	}
	// Nothing that did not change may appear: a PATCH is a whole-object
	// write, and sending a stale value would clobber a concurrent edit.
	for _, unexpected := range []string{"homepage", "has_issues", "allow_squash_merge", "visibility", "default_branch"} {
		if _, present := body[unexpected]; present {
			t.Errorf("body carries the unchanged key %q: %v", unexpected, body)
		}
	}
}

// Visibility and the default branch are separate endpoints on GitHub; the
// family must not smuggle them into the repo PATCH.
func TestApplySplitsVisibilityAndDefaultBranch(t *testing.T) {
	f, live := fixture(t)
	node := desired(t, "visibility: private\ndefault_branch: trunk\n")
	_, cs := New().Diff(node, live, schema.Declared)
	if err := New().Apply(context.Background(), f, target, cs); err != nil {
		t.Fatal(err)
	}
	body := f.Body("PATCH", "/repos/sfc-gh-eraigosa/dotfiles")
	if body["visibility"] != "private" || body["default_branch"] != "trunk" {
		t.Fatalf("both go in the repo PATCH GitHub accepts: %v", body)
	}
}

func TestApplyTopicsUseTheirOwnEndpoint(t *testing.T) {
	f, live := fixture(t)
	node := desired(t, "topics: [dotfiles, lab]\n")
	fs, cs := New().Diff(node, live, schema.Declared)
	if len(fs) != 1 || fs[0].Key != "topics" {
		t.Fatalf("findings = %v", fs)
	}
	if err := New().Apply(context.Background(), f, target, cs); err != nil {
		t.Fatal(err)
	}
	body := f.Body("PUT", "/repos/sfc-gh-eraigosa/dotfiles/topics")
	if body == nil {
		t.Fatalf("topics need PUT /topics, calls were %v", f.Paths())
	}
	got, _ := body["names"].([]any)
	if len(got) != 2 || got[0] != "dotfiles" || got[1] != "lab" {
		t.Fatalf("topics body = %v", body)
	}
	if b := f.Body("PATCH", "/repos/sfc-gh-eraigosa/dotfiles"); b != nil {
		if _, present := b["topics"]; present {
			t.Error("topics must not ride along in the repo PATCH")
		}
	}
}

func TestApplyWithNothingToDoMakesNoRequest(t *testing.T) {
	f, _ := fixture(t)
	before := len(f.Calls())
	if err := New().Apply(context.Background(), f, target, nil); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls()) != before {
		t.Fatalf("an empty change set must not call GitHub: %v", f.Paths())
	}
}

func TestReadSurfacesTheFailure(t *testing.T) {
	f := gh.NewFake()
	f.Fail("GET", "/repos/sfc-gh-eraigosa/dotfiles", 403, "Resource not accessible by personal access token")
	_, err := New().Read(context.Background(), f, target)
	if err == nil || !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("want the read failure, got %v", err)
	}
}

func TestIdentity(t *testing.T) {
	g := New()
	if g.Name() != "general" || g.Scope() != family.ScopeRepo {
		t.Errorf("identity = %q %v", g.Name(), g.Scope())
	}
	if !strings.Contains(g.Permission(), "Administration") {
		t.Errorf("permission = %q", g.Permission())
	}
}

func keys(m map[string]family.Finding) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// indent shifts a YAML block so it can be nested under a key.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString(pad + line + "\n")
	}
	return b.String()
}
