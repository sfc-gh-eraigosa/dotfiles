package gh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGHHostsPathHonoursEachConfigConvention(t *testing.T) {
	clearEnv(t)
	t.Setenv("GH_CONFIG_DIR", "/gh-config")
	if got := ghHostsPath(); got != filepath.Join("/gh-config", "hosts.yml") {
		t.Errorf("GH_CONFIG_DIR: %q", got)
	}
	os.Unsetenv("GH_CONFIG_DIR")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := ghHostsPath(); got != filepath.Join("/xdg", "gh", "hosts.yml") {
		t.Errorf("XDG_CONFIG_HOME: %q", got)
	}
	os.Unsetenv("XDG_CONFIG_HOME")
	t.Setenv("HOME", "/home/someone")
	if got := ghHostsPath(); got != filepath.Join("/home/someone", ".config", "gh", "hosts.yml") {
		t.Errorf("HOME: %q", got)
	}
}

// A hosts.yml that is missing, unparsable, or for another host is simply
// "no credential here" — never an error that stops the chain.
func TestHostsFileTokenToleratesAnythingUnreadable(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", dir)
	if got := hostsFileToken(); got != "" {
		t.Errorf("missing file: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(":::not yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := hostsFileToken(); got != "" {
		t.Errorf("bad yaml: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte("ghe.example.com:\n    "+hostsKey+": elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := hostsFileToken(); got != "" {
		t.Errorf("another host: %q", got)
	}
}

func TestPathsSummarisesTheCallRecord(t *testing.T) {
	f := NewFake()
	f.Get("/a", 200, `{}`)
	if _, err := f.Do(t.Context(), "GET", "/a", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Do(t.Context(), "PATCH", "/a", map[string]any{"x": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Paths(), ", "); got != "GET /a, PATCH /a" {
		t.Fatalf("Paths() = %q", got)
	}
}

func TestWithPerPageLeavesAnExistingSetting(t *testing.T) {
	cases := map[string]string{
		"/labels":             "/labels?per_page=100",
		"/labels?state=open":  "/labels?state=open&per_page=100",
		"/labels?per_page=30": "/labels?per_page=30",
	}
	for in, want := range cases {
		if got := withPerPage(in); got != want {
			t.Errorf("withPerPage(%q) = %q, want %q", in, got, want)
		}
	}
}

// GitHub's richer error bodies carry a message plus field-level errors; the
// text gcfg shows must include both, and a body it cannot read must not
// crash the caller.
func TestMessageRendersGitHubErrorBodies(t *testing.T) {
	cases := map[string]string{
		`{"message":"Validation Failed","errors":[{"field":"name","code":"already_exists"}]}`: "Validation Failed (name already_exists)",
		`{"message":"Validation Failed","errors":[{"message":"custom text"}]}`:                "Validation Failed (custom text)",
		`{"message":"Not Found"}`:       "Not Found",
		`{"message":"X","errors":[{}]}`: "X",
		`not json at all`:               "(no message)",
		`{}`:                            "(no message)",
	}
	for body, want := range cases {
		if got := message([]byte(body)); got != want {
			t.Errorf("message(%s) = %q, want %q", body, got, want)
		}
	}
}
