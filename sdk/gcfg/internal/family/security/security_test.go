package security

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

// stubRepo wires the three endpoints this family reads: the repository
// itself, the Dependabot alerts flag (204/404, no body), and private
// vulnerability reporting.
func stubRepo(t *testing.T, alerts, pvr bool) *gh.Fake {
	t.Helper()
	f := gh.NewFake()
	f.GetFile(t, "/repos/sfc-gh-eraigosa/dotfiles", filepath.Join("testdata", "repo.json"))
	if alerts {
		f.Get("/repos/sfc-gh-eraigosa/dotfiles/vulnerability-alerts", 204, "")
	} else {
		f.Fail("GET", "/repos/sfc-gh-eraigosa/dotfiles/vulnerability-alerts", 404, "Not Found")
	}
	if pvr {
		f.Get("/repos/sfc-gh-eraigosa/dotfiles/private-vulnerability-reporting", 204, "")
	} else {
		f.Fail("GET", "/repos/sfc-gh-eraigosa/dotfiles/private-vulnerability-reporting", 404, "Not Found")
	}
	return f
}

func read(t *testing.T, f *gh.Fake) family.Live {
	t.Helper()
	live, err := New().Read(context.Background(), f, target)
	if err != nil {
		t.Fatal(err)
	}
	return live
}

func desired(t *testing.T, body string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func TestReadCombinesTheThreeEndpoints(t *testing.T) {
	f := stubRepo(t, true, false)
	live := read(t, f)
	node, err := New().Export(live)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := yaml.Marshal(node)
	got := string(out)
	for _, want := range []string{
		"secret_scanning: true",
		"push_protection: true",
		"non_provider_patterns: false",
		"dependabot_alerts: true",
		"dependabot_security_updates: true",
		"private_vulnerability_reporting: false",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export missing %q:\n%s", want, got)
		}
	}
	// The exported block must load back into the schema.
	if _, _, err := schema.Parse([]byte("version: 1\nrepo:\n  security:\n"+indent(got, 4)), "export"); err != nil {
		t.Fatalf("exported YAML does not load: %v\n%s", err, got)
	}
}

// A 404 on the alerts endpoint means "off", not "broken".
func TestReadTreatsAlerts404AsDisabled(t *testing.T) {
	live := read(t, stubRepo(t, false, false))
	node, _ := New().Export(live)
	out, _ := yaml.Marshal(node)
	if !strings.Contains(string(out), "dependabot_alerts: false") {
		t.Fatalf("alerts should read as false:\n%s", out)
	}
}

func TestDiffAndApplyUseTheSecurityAnalysisBlock(t *testing.T) {
	f := stubRepo(t, true, false)
	live := read(t, f)
	node := desired(t, "secret_scanning: true\npush_protection: false\nnon_provider_patterns: true\n")
	fs, cs := New().Diff(node, live, schema.Declared)
	if len(fs) != 2 || len(cs) != 2 {
		t.Fatalf("want 2 findings/changes, got %d/%d: %v", len(fs), len(cs), fs)
	}
	if err := New().Apply(context.Background(), f, target, cs); err != nil {
		t.Fatal(err)
	}
	body := f.Body("PATCH", "/repos/sfc-gh-eraigosa/dotfiles")
	sa, _ := body["security_and_analysis"].(map[string]any)
	if sa == nil {
		t.Fatalf("want a security_and_analysis PATCH, got %v", body)
	}
	pp, _ := sa["secret_scanning_push_protection"].(map[string]any)
	npp, _ := sa["secret_scanning_non_provider_patterns"].(map[string]any)
	if pp["status"] != "disabled" || npp["status"] != "enabled" {
		t.Fatalf("security_and_analysis = %v", sa)
	}
	if _, present := sa["secret_scanning"]; present {
		t.Error("an unchanged setting must not be re-sent")
	}
}

// Dependabot alerts are a PUT/DELETE pair, not part of the PATCH.
func TestApplyDependabotAlertsUseTheirOwnEndpoint(t *testing.T) {
	f := stubRepo(t, true, false)
	live := read(t, f)
	_, cs := New().Diff(desired(t, "dependabot_alerts: false\n"), live, schema.Declared)
	if err := New().Apply(context.Background(), f, target, cs); err != nil {
		t.Fatal(err)
	}
	if got := f.Paths(); !contains(got, "DELETE /repos/sfc-gh-eraigosa/dotfiles/vulnerability-alerts") {
		t.Fatalf("want a DELETE on the alerts endpoint, calls: %v", got)
	}
	f2 := stubRepo(t, false, false)
	live2 := read(t, f2)
	_, cs2 := New().Diff(desired(t, "dependabot_alerts: true\n"), live2, schema.Declared)
	if err := New().Apply(context.Background(), f2, target, cs2); err != nil {
		t.Fatal(err)
	}
	if got := f2.Paths(); !contains(got, "PUT /repos/sfc-gh-eraigosa/dotfiles/vulnerability-alerts") {
		t.Fatalf("want a PUT on the alerts endpoint, calls: %v", got)
	}
}

func TestApplyPrivateVulnerabilityReportingUsesItsOwnEndpoint(t *testing.T) {
	f := stubRepo(t, true, false)
	live := read(t, f)
	_, cs := New().Diff(desired(t, "private_vulnerability_reporting: true\n"), live, schema.Declared)
	if err := New().Apply(context.Background(), f, target, cs); err != nil {
		t.Fatal(err)
	}
	if got := f.Paths(); !contains(got, "PUT /repos/sfc-gh-eraigosa/dotfiles/private-vulnerability-reporting") {
		t.Fatalf("calls: %v", got)
	}
}

// The design's live finding: GitHub answers 200 to a non-provider-patterns
// write, then leaves it disabled without Secret Protection. Verify must say
// so rather than reporting drift forever with no explanation.
func TestNotHonouredIsItsOwnFinding(t *testing.T) {
	f := stubRepo(t, true, false)
	live := read(t, f)
	node := desired(t, "non_provider_patterns: true\n")

	fs, cs := New().Diff(node, live, schema.Declared)
	if len(fs) != 1 || fs[0].Kind != family.Drift {
		t.Fatalf("before apply this is plain drift: %v", fs)
	}
	// After an apply that GitHub accepted, a re-read that still disagrees is
	// not_honoured — the engine passes AfterApply so the family can say it.
	fs2, _ := New().DiffAfterApply(node, live, schema.Declared, cs)
	if len(fs2) != 1 || fs2[0].Kind != family.NotHonoured {
		t.Fatalf("after apply it must be not_honoured: %v", fs2)
	}
	if !strings.Contains(fs2[0].Reason, "Secret Protection") {
		t.Errorf("the reason should name what is missing: %q", fs2[0].Reason)
	}
}

func TestReadSurfacesARepoFailure(t *testing.T) {
	f := gh.NewFake()
	f.Fail("GET", "/repos/sfc-gh-eraigosa/dotfiles", 403, "Resource not accessible by personal access token")
	if _, err := New().Read(context.Background(), f, target); err == nil {
		t.Fatal("want the read failure")
	}
}

func TestIdentity(t *testing.T) {
	s := New()
	if s.Name() != "security" || s.Scope() != family.ScopeRepo {
		t.Errorf("identity = %q %v", s.Name(), s.Scope())
	}
	if !strings.Contains(s.Permission(), "Administration") {
		t.Errorf("permission = %q", s.Permission())
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString(pad + line + "\n")
	}
	return b.String()
}
