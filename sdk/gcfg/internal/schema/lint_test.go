package schema

import (
	"strings"
	"testing"
)

func lintBody(t *testing.T, body string, opts LintOpts) []Problem {
	t.Helper()
	f, _, err := Load(write(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return Lint(f, opts)
}

func problemStrings(ps []Problem) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString(p.String())
		b.WriteString("\n")
	}
	return b.String()
}

func TestLintCleanFile(t *testing.T) {
	if ps := lintBody(t, full, LintOpts{Owner: "sfc-gh-eraigosa", Repo: ".github"}); len(ps) != 0 {
		t.Fatalf("want no problems, got:\n%s", problemStrings(ps))
	}
}

func TestLintOrgBlockOnlyInDotGithubRepo(t *testing.T) {
	body := "version: 1\norg:\n  profile: {description: x}\n"
	ps := lintBody(t, body, LintOpts{Owner: "sfc-gh-eraigosa", Repo: "dotfiles"})
	if len(ps) != 1 || !strings.Contains(ps[0].Message, ".github") || ps[0].Path != "org" {
		t.Fatalf("want one org-placement problem, got:\n%s", problemStrings(ps))
	}
	if ps := lintBody(t, body, LintOpts{Owner: "sfc-gh-eraigosa", Repo: ".github"}); len(ps) != 0 {
		t.Fatalf("org block in .github must be fine, got:\n%s", problemStrings(ps))
	}
	// With no target known, placement cannot be judged: no problem.
	if ps := lintBody(t, body, LintOpts{}); len(ps) != 0 {
		t.Fatalf("unknown target: %s", problemStrings(ps))
	}
}

func TestLintDuplicateNames(t *testing.T) {
	ps := lintBody(t, `
version: 1
repo:
  labels: [{name: bug, color: aaaaaa}, {name: bug, color: bbbbbb}]
  rulesets:
    - {name: main, target: branch, enforcement: active}
    - {name: main, target: branch, enforcement: active}
  environments: [{name: prod}, {name: prod}]
`, LintOpts{})
	// Problems arrive in schema order (the order plan §3.1 lists the
	// families), not the order the YAML happens to use — so two files that
	// declare the same settings report identically.
	want := []string{"repo.rulesets[1].name", "repo.labels[1].name", "repo.environments[1].name"}
	if len(ps) != len(want) {
		t.Fatalf("want %d duplicate problems, got:\n%s", len(want), problemStrings(ps))
	}
	for i, w := range want {
		if ps[i].Path != w {
			t.Errorf("problem %d path = %q, want %q", i, ps[i].Path, w)
		}
		if !strings.Contains(ps[i].Message, "duplicate") {
			t.Errorf("not a duplicate problem: %s", ps[i])
		}
	}
}

func TestLintEnumValues(t *testing.T) {
	ps := lintBody(t, `
version: 1
repo:
  general:
    visibility: hidden
    merge: {squash_title: SHOUT, squash_message: WHISPER}
  security: {code_scanning_default_setup: maybe}
  actions: {allowed_actions: some, default_workflow_permissions: admin}
  rulesets: [{name: r, target: sideways, enforcement: sometimes}]
  collaborators: [{login: x, permission: godmode}]
`, LintOpts{})
	wantPaths := []string{
		"repo.general.visibility",
		"repo.general.merge.squash_title",
		"repo.general.merge.squash_message",
		"repo.security.code_scanning_default_setup",
		"repo.actions.allowed_actions",
		"repo.actions.default_workflow_permissions",
		"repo.rulesets[0].target",
		"repo.rulesets[0].enforcement",
		"repo.collaborators[0].permission",
	}
	if len(ps) != len(wantPaths) {
		t.Fatalf("want %d enum problems, got %d:\n%s", len(wantPaths), len(ps), problemStrings(ps))
	}
	for i, want := range wantPaths {
		if ps[i].Path != want {
			t.Errorf("problem %d path = %q, want %q", i, ps[i].Path, want)
		}
		if !strings.Contains(ps[i].Message, "must be one of") {
			t.Errorf("problem %d message = %q", i, ps[i].Message)
		}
	}
}

// G8: nothing in this file may carry a secret value. A value that looks like
// a token is a lint error wherever it appears — the file is committed.
func TestLintSecretShapedValues(t *testing.T) {
	tokenish := strings.Join([]string{"ghp", "0123456789abcdef0123456789abcdef0123"}, "_")
	ps := lintBody(t, "version: 1\nrepo:\n  general:\n    description: \""+tokenish+"\"\n", LintOpts{})
	if len(ps) != 1 || !strings.Contains(ps[0].Message, "secret") {
		t.Fatalf("want a secret-shaped problem, got:\n%s", problemStrings(ps))
	}
	if strings.Contains(problemStrings(ps), tokenish) {
		t.Fatal("the lint message must not echo the secret it found")
	}
	// Secrets are declared by name; a name is not a secret.
	if ps := lintBody(t, "version: 1\nrepo:\n  secrets: {names: [GCFG_TOKEN, GH_TOKEN]}\n", LintOpts{}); len(ps) != 0 {
		t.Fatalf("secret names must be fine, got:\n%s", problemStrings(ps))
	}
	// A webhook URL carrying userinfo credentials is caught too.
	creds := "user:" + strings.Join([]string{"hunter", "2"}, "")
	ps = lintBody(t, "version: 1\nrepo:\n  webhooks: [{url: \"https://"+creds+"@example.com/h\"}]\n", LintOpts{})
	if len(ps) != 1 || !strings.Contains(ps[0].Message, "credential") {
		t.Fatalf("want a webhook-credential problem, got:\n%s", problemStrings(ps))
	}
}

func TestLintRequiredFieldsInLists(t *testing.T) {
	ps := lintBody(t, `
version: 1
repo:
  labels: [{color: aaaaaa}]
  rulesets: [{target: branch, enforcement: active}]
  autolinks: [{key_prefix: "J-"}]
`, LintOpts{})
	if len(ps) != 3 {
		t.Fatalf("want 3 required-field problems, got:\n%s", problemStrings(ps))
	}
	for _, p := range ps {
		if !strings.Contains(p.Message, "required") {
			t.Errorf("not a required-field problem: %s", p)
		}
	}
}

func TestLintLabelColour(t *testing.T) {
	ps := lintBody(t, "version: 1\nrepo:\n  labels: [{name: bug, color: \"#d73a4a\"}]\n", LintOpts{})
	if len(ps) != 1 || !strings.Contains(ps[0].Message, "six hex") {
		t.Fatalf("want a colour-format problem, got:\n%s", problemStrings(ps))
	}
}

func TestProblemStringCarriesPathAndMessage(t *testing.T) {
	p := Problem{Path: "repo.general.visibility", Message: "must be one of public, private, internal"}
	if !strings.Contains(p.String(), "repo.general.visibility") || !strings.Contains(p.String(), "must be one of") {
		t.Fatalf("Problem.String() = %q", p.String())
	}
}
