package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPathPrecedence(t *testing.T) {
	t.Setenv("GDASH_CONFIG", "/x/gdash.json")
	if p, _ := Path(); p != "/x/gdash.json" {
		t.Fatalf("GDASH_CONFIG ignored: %s", p)
	}
	t.Setenv("GDASH_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if p, _ := Path(); p != "/xdg/gdash/config.json" {
		t.Fatalf("XDG path = %s", p)
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || len(c.Profiles) != 0 || len(c.Views) != 0 {
		t.Fatalf("missing file = %+v %v", c, err)
	}
}

func TestSaveModesAndRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gdash")
	p := filepath.Join(dir, "config.json")
	c := Config{
		Profiles:       map[string]Profile{"lab": {URL: "http://192.0.2.128:30300", Auth: Auth{File: "~/.config/gdash/lab.cred"}}},
		DefaultProfile: "lab",
		Views:          map[string]View{"ci": {Dashboard: "ci-lab", Refresh: "30s"}},
	}
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", st.Mode().Perm())
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", st.Mode().Perm())
	}
	back, err := Load(p)
	if err != nil || back.Views["ci"].Dashboard != "ci-lab" || back.Profiles["lab"].Auth.File == "" {
		t.Fatalf("round trip = %+v %v", back, err)
	}
}

func TestValidate(t *testing.T) {
	ok := Profile{URL: "http://192.0.2.128:30300", Auth: Auth{Env: "GDASH_CRED"}}
	cases := []struct {
		name string
		c    Config
		want string // "" = valid; else a substring of the error
	}{
		{"valid", Config{Profiles: map[string]Profile{"lab": ok}, DefaultProfile: "lab", Views: map[string]View{"ci": {Dashboard: "ci-lab"}}}, ""},
		{"no url", Config{Profiles: map[string]Profile{"lab": {Auth: Auth{Env: "X"}}}}, "profiles.lab.url"},
		{"password in url", Config{Profiles: map[string]Profile{"lab": {URL: "http://admin:pw@h", Auth: Auth{Env: "X"}}}}, "auth.user"},
		{"bad scheme", Config{Profiles: map[string]Profile{"lab": {URL: "ftp://h", Auth: Auth{Env: "X"}}}}, "profiles.lab.url"},
		{"no auth source", Config{Profiles: map[string]Profile{"lab": {URL: "http://h"}}}, "profiles.lab.auth"},
		{"two auth sources", Config{Profiles: map[string]Profile{"lab": {URL: "http://h", Auth: Auth{Env: "X", Cmd: "lpass show"}}}}, "exactly one"},
		{"view without dashboard", Config{Profiles: map[string]Profile{"lab": ok}, DefaultProfile: "lab", Views: map[string]View{"ci": {}}}, "views.ci.dashboard"},
		{"view names unknown profile", Config{Profiles: map[string]Profile{"lab": ok}, Views: map[string]View{"ci": {Dashboard: "d", Profile: "nope"}}}, "views.ci.profile"},
		{"view with no profile to use", Config{Profiles: map[string]Profile{"lab": ok}, Views: map[string]View{"ci": {Dashboard: "d"}}}, "default_profile"},
		{"bad refresh", Config{Profiles: map[string]Profile{"lab": ok}, DefaultProfile: "lab", Views: map[string]View{"ci": {Dashboard: "d", Refresh: "soon"}}}, "views.ci.refresh"},
		{"bad range", Config{Profiles: map[string]Profile{"lab": ok}, DefaultProfile: "lab", Views: map[string]View{"ci": {Dashboard: "d", Range: "-1h"}}}, "views.ci.range"},
	}
	for _, tc := range cases {
		err := tc.c.Validate()
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: error %v, want it to name %q", tc.name, err, tc.want)
		}
	}
}

func TestParseDurationAcceptsDays(t *testing.T) {
	for in, want := range map[string]time.Duration{"30s": 30 * time.Second, "1h": time.Hour, "7d": 7 * 24 * time.Hour} {
		if got, err := ParseDuration(in); err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v %v, want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "-1h", "0s", "soon"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("ParseDuration(%q) accepted", bad)
		}
	}
}

func TestViewResolvesDefaults(t *testing.T) {
	c := Config{Profiles: map[string]Profile{"lab": {URL: "http://h", Auth: Auth{Env: "X"}}}, DefaultProfile: "lab",
		Views: map[string]View{"ci": {Dashboard: "ci-lab"}}}
	v, p, err := c.Resolve("ci")
	if err != nil || p.URL != "http://h" || v.Range != "1h" || v.Refresh != "30s" {
		t.Fatalf("Resolve = %+v %+v %v", v, p, err)
	}
	if _, _, err := c.Resolve("nope"); err == nil || !strings.Contains(err.Error(), "ci") {
		t.Fatalf("unknown view error should list the known ones: %v", err)
	}
}

func TestGetSetDotted(t *testing.T) {
	var c Config
	steps := [][2]string{
		{"profiles.lab.url", "http://192.0.2.128:30300"},
		{"profiles.lab.auth.env", "GDASH_CRED"},
		{"default_profile", "lab"},
		{"views.ci.dashboard", "ci-lab"},
		{"views.ci.vars.node", "All"},
		{"views.ci.panels", "Queue,Runtime"},
	}
	for _, s := range steps {
		if err := c.Set(s[0], s[1]); err != nil {
			t.Fatalf("Set(%s): %v", s[0], err)
		}
	}
	if v, _ := c.Get("views.ci.panels"); v != "Queue,Runtime" {
		t.Fatalf("panels = %q", v)
	}
	if v, _ := c.Get("views.ci.vars.node"); v != "All" {
		t.Fatalf("vars = %q", v)
	}
	if _, err := c.Get("views.nope.dashboard"); err == nil {
		t.Fatal("Get on a missing view should fail")
	}
	if err := c.Set("views.ci.colour", "x"); err == nil {
		t.Fatal("unknown field accepted")
	}
}

// E13: a value for any key named like a credential is refused, so a secret
// can never be written inline.
func TestSetRefusesInlineCredentials(t *testing.T) {
	var c Config
	for _, key := range []string{"profiles.lab.auth.token", "profiles.lab.password", "profiles.lab.auth.secret"} {
		err := c.Set(key, "x")
		if err == nil || !strings.Contains(err.Error(), "auth.file") {
			t.Errorf("Set(%s) = %v, want a refusal pointing at auth.file/env/cmd", key, err)
		}
	}
}

func TestSetAuthFileMustBe0600(t *testing.T) {
	var c Config
	f := filepath.Join(t.TempDir(), "lab.cred")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("profiles.lab.auth.file", f); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("0644 file accepted: %v", err)
	}
	if err := os.Chmod(f, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("profiles.lab.auth.file", f); err != nil {
		t.Fatalf("0600 file refused: %v", err)
	}
}
