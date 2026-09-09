package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/gh"
)

// world stubs a whole repository so the verbs can run end to end without a
// network: the general and security families read these endpoints.
type world struct {
	fake *gh.Fake
	dir  string
	file string
}

const repoJSON = `{
  "full_name": "sfc-gh-eraigosa/dotfiles",
  "description": "dotfiles",
  "homepage": "",
  "topics": ["dotfiles"],
  "visibility": "public",
  "default_branch": "main",
  "has_issues": true, "has_projects": false, "has_wiki": false, "has_discussions": false,
  "allow_squash_merge": true, "allow_merge_commit": false, "allow_rebase_merge": false,
  "allow_auto_merge": true, "delete_branch_on_merge": true, "allow_update_branch": true,
  "squash_merge_commit_title": "COMMIT_OR_PR_TITLE", "squash_merge_commit_message": "COMMIT_MESSAGES",
  "web_commit_signoff_required": false, "allow_forking": true,
  "security_and_analysis": {
    "secret_scanning": {"status": "enabled"},
    "secret_scanning_push_protection": {"status": "enabled"},
    "secret_scanning_non_provider_patterns": {"status": "disabled"},
    "dependabot_security_updates": {"status": "enabled"}
  }
}`

func newWorld(t *testing.T) *world {
	t.Helper()
	isolateCreds(t)
	t.Setenv("GH_TOKEN", "t")
	f := gh.NewFake()
	f.Get("/repos/sfc-gh-eraigosa/dotfiles", 200, repoJSON)
	f.Get("/repos/sfc-gh-eraigosa/dotfiles/vulnerability-alerts", 204, "")
	f.Fail("GET", "/repos/sfc-gh-eraigosa/dotfiles/private-vulnerability-reporting", 404, "Not Found")
	w := &world{fake: f, dir: t.TempDir()}
	w.file = filepath.Join(w.dir, "gcfg.yaml")
	// Every verb resolves its client through this seam.
	old := newClient
	newClient = func(*Globals, Target) (gh.Client, gh.Source, error) { return f, gh.SourceEnv, nil }
	t.Cleanup(func() { newClient = old })
	return w
}

func (w *world) write(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(w.file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w *world) run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return run(append(args, "-R", "sfc-gh-eraigosa/dotfiles", "-f", w.file)...)
}

// UC1: export what is live, and the result verifies clean.
func TestExportThenVerifyIsClean(t *testing.T) {
	w := newWorld(t)
	out, _, err := w.run(t, "export")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	body, err := os.ReadFile(w.file)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version: 1", "repo:", "general:", "security:", "secret_scanning: true"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("exported file missing %q:\n%s", want, body)
		}
	}
	out, _, err = w.run(t, "verify")
	if err != nil {
		t.Fatalf("a file exported from live state must verify clean: %v\n%s", err, out)
	}
	if !strings.Contains(out, "clean") {
		t.Errorf("verify output = %q", out)
	}
}

func TestExportRefusesToOverwriteWithoutForce(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\n# hand-written\n")
	_, _, err := w.run(t, "export")
	if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("want a usage error pointing at --force, got %v", err)
	}
	if _, _, err := w.run(t, "export", "--force"); err != nil {
		t.Fatalf("--force: %v", err)
	}
	body, _ := os.ReadFile(w.file)
	if strings.Contains(string(body), "hand-written") {
		t.Error("--force should have replaced the file")
	}
}

func TestExportToStdout(t *testing.T) {
	w := newWorld(t)
	out, _, err := w.run(t, "export", "--out", "-")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "version: 1") {
		t.Errorf("stdout = %q", out)
	}
	if _, err := os.Stat(w.file); !os.IsNotExist(err) {
		t.Error("--out - must not write the file")
	}
}

// UC2: a setting flipped away from the file is drift, exit 1, and the key
// is named.
func TestVerifyReportsDriftWithExitOne(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    merge:\n      delete_branch_on_merge: false\n")
	out, _, err := w.run(t, "verify")
	if !errors.Is(err, ErrFindings) {
		t.Fatalf("want ErrFindings (exit 1), got %v", err)
	}
	if !strings.Contains(out, "merge.delete_branch_on_merge") {
		t.Errorf("verify must name the key:\n%s", out)
	}
	if !strings.Contains(out, "drift") {
		t.Errorf("verify must say what kind:\n%s", out)
	}
}

func TestVerifyJSONAndMarkdown(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    merge: {delete_branch_on_merge: false}\n")
	out, _, err := w.run(t, "verify", "--json")
	if !errors.Is(err, ErrFindings) {
		t.Fatalf("err = %v", err)
	}
	var rep struct {
		Clean    bool                                 `json:"clean"`
		Findings []struct{ Family, Key, Kind string } `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if rep.Clean || len(rep.Findings) != 1 || rep.Findings[0].Key != "merge.delete_branch_on_merge" {
		t.Fatalf("json = %+v", rep)
	}
	out, _, _ = w.run(t, "verify", "--markdown")
	if !strings.Contains(out, "| family | key | want | live | kind |") {
		t.Errorf("markdown = %q", out)
	}
}

func TestVerifyOnlyLimitsFamilies(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    merge: {delete_branch_on_merge: false}\n  security:\n    push_protection: false\n")
	out, _, err := w.run(t, "verify", "--only", "security")
	if !errors.Is(err, ErrFindings) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(out, "delete_branch_on_merge") {
		t.Errorf("--only security must not report general:\n%s", out)
	}
	if _, _, err := w.run(t, "verify", "--only", "nope"); !errors.Is(err, ErrUsage) {
		t.Fatalf("unknown family: want ErrUsage, got %v", err)
	}
}

func TestPlanShowsChangesAndWritesNothing(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    merge: {delete_branch_on_merge: false}\n")
	out, _, err := w.run(t, "plan")
	if !errors.Is(err, ErrFindings) {
		t.Fatalf("plan with drift exits 1: %v", err)
	}
	if !strings.Contains(out, "update") || !strings.Contains(out, "merge.delete_branch_on_merge") {
		t.Errorf("plan output = %q", out)
	}
	if writes := w.fake.Writes(); len(writes) != 0 {
		t.Fatalf("plan must not write: %v", writes)
	}
}

// UC3: apply the difference, then the re-read comes back clean.
func TestApplyWritesAndReVerifies(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    merge: {delete_branch_on_merge: false}\n")
	// After the write, reading the repo back shows the new value — which is
	// what makes apply's re-read meaningful.
	w.fake.AfterWrite("PATCH", "/repos/sfc-gh-eraigosa/dotfiles", "/repos/sfc-gh-eraigosa/dotfiles",
		strings.Replace(repoJSON, `"delete_branch_on_merge": true`, `"delete_branch_on_merge": false`, 1))
	out, _, err := w.run(t, "apply", "--yes")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	body := w.fake.Body("PATCH", "/repos/sfc-gh-eraigosa/dotfiles")
	if body["delete_branch_on_merge"] != false {
		t.Fatalf("apply body = %v", body)
	}
	if !strings.Contains(out, "clean") {
		t.Errorf("the re-read after a successful apply must be clean:\n%s", out)
	}
}

// A write GitHub accepts but ignores must not be reported as success: the
// re-read still disagrees, so apply exits 1 and says why.
func TestApplyReportsASettingGitHubIgnored(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  security:\n    non_provider_patterns: true\n")
	// No AfterWrite: the PATCH is accepted, the value never changes.
	out, _, err := w.run(t, "apply", "--yes")
	if !errors.Is(err, ErrFindings) {
		t.Fatalf("want exit 1, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "not_honoured") || !strings.Contains(out, "Secret Protection") {
		t.Errorf("apply must explain what GitHub ignored:\n%s", out)
	}
}

// Exit 2 and zero writes: a non-TTY apply without --yes must never guess.
func TestApplyWithoutYesInNonTTYIsExitTwoAndWritesNothing(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    merge: {delete_branch_on_merge: false}\n")
	out, _, err := w.run(t, "apply")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v\n%s", err, out)
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("the error must say how to proceed: %v", err)
	}
	if writes := w.fake.Writes(); len(writes) != 0 {
		t.Fatalf("nothing may be written: %v", writes)
	}
}

func TestApplyDryRunWritesNothing(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    merge: {delete_branch_on_merge: false}\n")
	if _, _, err := w.run(t, "apply", "--dry-run"); !errors.Is(err, ErrFindings) {
		t.Fatalf("a dry run with drift still exits 1: %v", err)
	}
	if writes := w.fake.Writes(); len(writes) != 0 {
		t.Fatalf("--dry-run must not write: %v", writes)
	}
}

func TestApplyCleanFileIsExitZeroAndNoWrites(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    merge: {delete_branch_on_merge: true}\n")
	if _, _, err := w.run(t, "apply", "--yes"); err != nil {
		t.Fatal(err)
	}
	if writes := w.fake.Writes(); len(writes) != 0 {
		t.Fatalf("nothing to do means no request: %v", writes)
	}
}

func TestInitWritesADefaultFile(t *testing.T) {
	w := newWorld(t)
	out, _, err := w.run(t, "init")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	body, err := os.ReadFile(w.file)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version: 1", "ownership: declared", "secret_scanning: true"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("init file missing %q:\n%s", want, body)
		}
	}
	// What init writes must lint clean.
	if _, _, err := w.run(t, "lint"); err != nil {
		t.Fatalf("the default file must lint clean: %v", err)
	}
	if _, _, err := w.run(t, "init"); !errors.Is(err, ErrUsage) {
		t.Fatal("init must refuse to overwrite")
	}
	if _, _, err := w.run(t, "init", "--force"); err != nil {
		t.Fatalf("--force: %v", err)
	}
}

// init --from copies another repository's file as the starting point.
func TestInitFromAnotherRepo(t *testing.T) {
	w := newWorld(t)
	w.fake.Get("/repos/other/repo/contents/.github/gcfg.yaml", 200,
		`{"content":"dmVyc2lvbjogMQpyZXBvOgogIGdlbmVyYWw6CiAgICB2aXNpYmlsaXR5OiBwdWJsaWMK","encoding":"base64"}`)
	if _, _, err := w.run(t, "init", "--from", "other/repo"); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(w.file)
	if !strings.Contains(string(body), "visibility: public") {
		t.Fatalf("init --from should copy the source file:\n%s", body)
	}
	if _, _, err := w.run(t, "init", "--from", "nope", "--force"); !errors.Is(err, ErrUsage) {
		t.Fatalf("a bad --from is a usage error, got %v", err)
	}
}

func TestVerifyMissingFileIsUsage(t *testing.T) {
	w := newWorld(t)
	_, _, err := w.run(t, "verify")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
	if !strings.Contains(err.Error(), "gcfg init") {
		t.Errorf("the error should point at init: %v", err)
	}
}

// No verb may print a credential, whatever the report contains.
func TestVerbsNeverPrintACredential(t *testing.T) {
	w := newWorld(t)
	w.write(t, "version: 1\nrepo:\n  general:\n    description: x\n")
	for _, args := range [][]string{{"verify"}, {"plan"}, {"export", "--out", "-"}, {"lint"}} {
		out, errb, _ := w.run(t, args...)
		if strings.Contains(out+errb, "ghs_") || strings.Contains(out+errb, "gho_") {
			t.Errorf("%v printed something token-shaped:\n%s%s", args, out, errb)
		}
	}
}
