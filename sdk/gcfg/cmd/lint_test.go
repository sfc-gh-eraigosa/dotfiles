package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLintCleanFileIsExitZero(t *testing.T) {
	p := writeFile(t, "gcfg.yaml", "version: 1\nrepo:\n  general:\n    visibility: public\n")
	out, _, err := run("lint", "-f", p, "-R", "o/r")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("want an ok line, got %q", out)
	}
}

func TestLintProblemsAreUsageErrors(t *testing.T) {
	p := writeFile(t, "gcfg.yaml", "version: 1\nrepo:\n  general:\n    visibility: hidden\n")
	out, _, err := run("lint", "-f", p, "-R", "o/r")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
	if !strings.Contains(out, "repo.general.visibility") || !strings.Contains(out, "must be one of") {
		t.Errorf("want the problem on stdout, got %q", out)
	}
}

func TestLintUnknownKeyIsAUsageError(t *testing.T) {
	p := writeFile(t, "gcfg.yaml", "version: 1\nrepo:\n  genral: {}\n")
	_, _, err := run("lint", "-f", p)
	if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "genral") {
		t.Fatalf("want a usage error naming the key, got %v", err)
	}
}

func TestLintMissingFileIsAUsageError(t *testing.T) {
	_, _, err := run("lint", "-f", filepath.Join(t.TempDir(), "nope.yaml"))
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
}

func TestLintWarnsOnAnEmptyFile(t *testing.T) {
	p := writeFile(t, "gcfg.yaml", "version: 1\n")
	out, _, err := run("lint", "-f", p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "declares no settings") {
		t.Errorf("want the warning, got %q", out)
	}
}

func TestLintJSON(t *testing.T) {
	p := writeFile(t, "gcfg.yaml", "version: 1\nrepo:\n  general:\n    visibility: hidden\n")
	out, _, err := run("lint", "-f", p, "--json")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
	var got struct {
		OK       bool                             `json:"ok"`
		Problems []struct{ Path, Message string } `json:"problems"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if got.OK || len(got.Problems) != 1 || got.Problems[0].Path != "repo.general.visibility" {
		t.Fatalf("json = %+v", got)
	}
}

// The org-placement rule needs the target; -R supplies it.
func TestLintOrgPlacementUsesTheTarget(t *testing.T) {
	p := writeFile(t, "gcfg.yaml", "version: 1\norg:\n  profile: {description: x}\n")
	if _, _, err := run("lint", "-f", p, "-R", "acme/.github"); err != nil {
		t.Fatalf("org file in .github should lint clean: %v", err)
	}
	out, _, err := run("lint", "-f", p, "-R", "acme/website")
	if !errors.Is(err, ErrUsage) || !strings.Contains(out, ".github") {
		t.Fatalf("want a placement problem, got %v / %q", err, out)
	}
}

func TestSchemaWritesTheJSONSchema(t *testing.T) {
	out, _, err := run("schema")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if s["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("$schema = %v", s["$schema"])
	}
	dst := filepath.Join(t.TempDir(), "gcfg.schema.json")
	if _, _, err := run("schema", "--out", dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != out {
		t.Error("--out must write exactly what stdout prints")
	}
	if _, _, err := run("schema", "--out", filepath.Join(t.TempDir(), "no-such-dir", "s.json")); err == nil {
		t.Error("unwritable --out: want an error")
	}
}

func TestBadTargetIsAUsageError(t *testing.T) {
	p := writeFile(t, "gcfg.yaml", "version: 1\n")
	if _, _, err := run("lint", "-f", p, "-R", "notaslug"); !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
}
