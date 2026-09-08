package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gcfg.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const full = `
version: 1
ownership: declared
repo:
  general:
    description: "dotfiles"
    homepage: "https://example.com"
    topics: [dotfiles, shell]
    visibility: public
    default_branch: main
    features: {issues: true, projects: false, wiki: false, discussions: true}
    merge:
      squash: true
      merge_commit: false
      rebase: false
      auto_merge: true
      delete_branch_on_merge: true
      allow_update_branch: true
      squash_title: COMMIT_OR_PR_TITLE
      squash_message: COMMIT_MESSAGES
    web_commit_signoff_required: false
    allow_forking: true
  security:
    secret_scanning: true
    push_protection: true
    non_provider_patterns: true
    dependabot_alerts: true
    dependabot_security_updates: true
    private_vulnerability_reporting: false
    code_scanning_default_setup: not-configured
  actions:
    enabled: true
    allowed_actions: all
    sha_pinning_required: false
    default_workflow_permissions: read
    can_approve_pull_request_reviews: false
  rulesets:
    - name: main
      target: branch
      enforcement: active
      conditions: {ref_name: {include: ["~DEFAULT_BRANCH"], exclude: []}}
      bypass_actors: []
      rules:
        - type: pull_request
          parameters: {required_approving_review_count: 0, dismiss_stale_reviews_on_push: true}
        - type: non_fast_forward
        - type: deletion
  labels:
    ownership: full
    items:
      - {name: bug, color: d73a4a, description: "Something isn't working"}
  autolinks:
    - {key_prefix: "JIRA-", url_template: "https://example.com/<num>", is_alphanumeric: false}
  environments:
    - {name: production, wait_timer: 0, reviewers: [], deployment_branch_policy: protected_branches}
  secrets: {names: [GCFG_TOKEN]}
  webhooks:
    - {url: "https://example.com/hook", events: [push], active: true, content_type: json}
  collaborators:
    - {login: someone, permission: push}
  pages: {enabled: false}
org:
  profile: {description: "d", blog: "https://example.com", location: "here"}
  members: {default_repository_permission: none, members_can_create_repositories: false, two_factor_required: true}
  security_defaults: {secret_scanning_new_repos: true, push_protection_new_repos: true, dependabot_alerts_new_repos: true}
  actions: {allowed_actions: selected, default_workflow_permissions: read}
  rulesets:
    - name: org-main
      target: branch
      enforcement: evaluate
  apps:
    - {slug: mergify, repository_selection: all}
`

func TestLoadFullFile(t *testing.T) {
	f, warns, err := Load(write(t, full))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if f.Version != 1 || f.Ownership != Declared {
		t.Fatalf("version/ownership = %d %q", f.Version, f.Ownership)
	}
	g := f.Repo.General
	if *g.Description != "dotfiles" || *g.Visibility != "public" || (*g.Topics)[1] != "shell" {
		t.Errorf("general = %+v", g)
	}
	if !*g.Features.Issues || *g.Features.Wiki {
		t.Errorf("features = %+v", g.Features)
	}
	if *g.Merge.SquashTitle != "COMMIT_OR_PR_TITLE" || !*g.Merge.DeleteBranchOnMerge {
		t.Errorf("merge = %+v", g.Merge)
	}
	if !*f.Repo.Security.SecretScanning || *f.Repo.Security.CodeScanningDefaultSetup != "not-configured" {
		t.Errorf("security = %+v", f.Repo.Security)
	}
	if *f.Repo.Actions.AllowedActions != "all" || *f.Repo.Actions.DefaultWorkflowPermissions != "read" {
		t.Errorf("actions = %+v", f.Repo.Actions)
	}
	rs := f.Repo.Rulesets.Items
	if len(rs) != 1 || rs[0].Name != "main" || len(rs[0].Rules) != 3 || rs[0].Conditions.RefName.Include[0] != "~DEFAULT_BRANCH" {
		t.Errorf("rulesets = %+v", rs)
	}
	if rs[0].Rules[0].Parameters["required_approving_review_count"] != 0 {
		t.Errorf("rule parameters = %v", rs[0].Rules[0].Parameters)
	}
	if f.Repo.Labels.Ownership != Full || f.Repo.Labels.Items[0].Color != "d73a4a" {
		t.Errorf("labels = %+v", f.Repo.Labels)
	}
	if f.Repo.Autolinks.Items[0].KeyPrefix != "JIRA-" || f.Repo.Autolinks.Ownership != "" {
		t.Errorf("autolinks = %+v", f.Repo.Autolinks)
	}
	if f.Repo.Environments.Items[0].Name != "production" || f.Repo.Secrets.Names[0] != "GCFG_TOKEN" {
		t.Errorf("environments/secrets = %+v %+v", f.Repo.Environments, f.Repo.Secrets)
	}
	if f.Repo.Webhooks.Items[0].URL != "https://example.com/hook" || f.Repo.Collaborators.Items[0].Login != "someone" {
		t.Errorf("webhooks/collaborators")
	}
	if *f.Repo.Pages.Enabled {
		t.Errorf("pages = %+v", f.Repo.Pages)
	}
	if *f.Org.Profile.Description != "d" || !*f.Org.Members.TwoFactorRequired || !*f.Org.SecurityDefaults.SecretScanningNewRepos {
		t.Errorf("org = %+v", f.Org)
	}
	if *f.Org.Actions.AllowedActions != "selected" || f.Org.Rulesets.Items[0].Enforcement != "evaluate" || f.Org.Apps.Items[0].Slug != "mergify" {
		t.Errorf("org actions/rulesets/apps")
	}
}

// Every key is optional: a file with only `version` loads, and absent keys
// stay nil so the engine can tell "unmanaged" from "declared false".
func TestLoadEverythingOptional(t *testing.T) {
	f, warns, err := Load(write(t, "version: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Repo != nil || f.Org != nil {
		t.Fatalf("want nil sections, got %+v", f)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "declares no settings") {
		t.Fatalf("want an empty-file warning, got %v", warns)
	}
	f, _, err = Load(write(t, "version: 1\nrepo:\n  general:\n    description: x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Repo.General.Homepage != nil || f.Repo.Security != nil {
		t.Fatalf("absent keys must stay nil: %+v", f.Repo.General)
	}
}

func TestLoadUnknownKeyNamesThePath(t *testing.T) {
	cases := map[string]string{
		"top level":  "version: 1\nrepoo: {}\n",
		"under repo": "version: 1\nrepo:\n  genral: {}\n",
		"deep":       "version: 1\nrepo:\n  general:\n    merge:\n      squash_titel: PR_TITLE\n",
		"in a list":  "version: 1\nrepo:\n  labels:\n    - {name: bug, colour: red}\n",
	}
	wants := map[string]string{
		"top level":  "repoo",
		"under repo": "genral",
		"deep":       "squash_titel",
		"in a list":  "colour",
	}
	for name, body := range cases {
		_, _, err := Load(write(t, body))
		if err == nil {
			t.Errorf("%s: want error, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), wants[name]) {
			t.Errorf("%s: error should name the key %q, got: %v", name, wants[name], err)
		}
		if !strings.Contains(err.Error(), "line") {
			t.Errorf("%s: error should carry the line, got: %v", name, err)
		}
	}
}

func TestLoadVersionRules(t *testing.T) {
	if _, _, err := Load(write(t, "repo: {}\n")); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("missing version: want error naming it, got %v", err)
	}
	if _, _, err := Load(write(t, "version: 2\n")); err == nil || !strings.Contains(err.Error(), "2") {
		t.Fatalf("unsupported version: want error, got %v", err)
	}
}

func TestLoadPerFamilyOwnership(t *testing.T) {
	f, _, err := Load(write(t, "version: 1\nownership: full\nrepo:\n  general:\n    ownership: declared\n    description: x\n  labels:\n    items: [{name: bug, color: aaaaaa}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Ownership != Full || f.Repo.General.Ownership != Declared {
		t.Fatalf("ownership = %q / %q", f.Ownership, f.Repo.General.Ownership)
	}
	// Effective ownership falls back to the file's.
	if got := f.Repo.Labels.Own(f.Ownership); got != Full {
		t.Fatalf("labels effective ownership = %q, want full", got)
	}
	if got := f.Repo.General.Own(f.Ownership); got != Declared {
		t.Fatalf("general effective ownership = %q, want declared", got)
	}
}

func TestLoadListFamilyAcceptsBothForms(t *testing.T) {
	bare, _, err := Load(write(t, "version: 1\nrepo:\n  labels: [{name: bug, color: aaaaaa}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	mapped, _, err := Load(write(t, "version: 1\nrepo:\n  labels:\n    ownership: full\n    items: [{name: bug, color: aaaaaa}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(bare.Repo.Labels.Items) != 1 || bare.Repo.Labels.Ownership != "" {
		t.Errorf("bare list = %+v", bare.Repo.Labels)
	}
	if len(mapped.Repo.Labels.Items) != 1 || mapped.Repo.Labels.Ownership != Full {
		t.Errorf("mapped list = %+v", mapped.Repo.Labels)
	}
	if _, _, err := Load(write(t, "version: 1\nrepo:\n  labels: 3\n")); err == nil {
		t.Error("scalar for a list family: want error")
	}
	if _, _, err := Load(write(t, "version: 1\nrepo:\n  labels:\n    ownership: nonsense\n    items: []\n")); err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("bad ownership: want error naming it, got %v", err)
	}
}

func TestLoadReadErrors(t *testing.T) {
	if _, _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("missing file: want error")
	}
	if _, _, err := Load(write(t, "version: 1\nrepo: [not a map]\n")); err == nil {
		t.Error("type mismatch: want error")
	}
	if _, _, err := Load(write(t, "::: not yaml\n")); err == nil {
		t.Error("bad yaml: want error")
	}
}

// Default() is what `gcfg init` writes, and it must load cleanly.
func TestDefaultRoundTrips(t *testing.T) {
	b, err := Default().Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "version: 1") {
		t.Fatalf("default file missing version:\n%s", b)
	}
	f, _, err := Load(write(t, string(b)))
	if err != nil {
		t.Fatalf("default file does not load: %v\n%s", err, b)
	}
	if f.Repo == nil || f.Repo.General == nil {
		t.Fatalf("default file should declare general settings:\n%s", b)
	}
}

// The package's errors carry no program prefix: the CLI adds "gcfg: " once,
// and a doubled prefix reads like a bug in the tool.
func TestErrorsCarryNoProgramPrefix(t *testing.T) {
	for _, body := range []string{"nope: 1\n", "version: 9\n", "version: 1\nrepo:\n  genral: {}\n"} {
		_, _, err := Load(write(t, body))
		if err == nil {
			t.Fatalf("want error for %q", body)
		}
		if strings.HasPrefix(err.Error(), "gcfg:") {
			t.Errorf("error should not prefix the program name: %v", err)
		}
	}
	if _, _, err := Load("/no/such/file.yaml"); err == nil || strings.HasPrefix(err.Error(), "gcfg:") {
		t.Errorf("read error = %v", err)
	}
}
